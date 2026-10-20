package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func releaseInput(p, os, ver string) M {
	return M{"platform": p, "os": os, "version": ver, "sizeMB": 35.8, "downloadUrl": "https://downloads.example.test/" + os + "/app", "browserUrl": "https://downloads.example.test/guide", "notes": "修复播放问题，优化启动速度。", "force": false, "enabled": true}
}
func releaseItem(t *testing.T, h *harness, in M, method string) appRelease {
	t.Helper()
	w := h.req(method, "/api/admin/releases", in, "")
	if w.Code != 200 {
		t.Fatalf("save %d %s", w.Code, w.Body)
	}
	var list struct {
		Items []appRelease `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, r := range list.Items {
		if r.Platform == in["platform"] && r.OS == in["os"] && r.Version == canonicalReleaseVersion(str(in["version"])) {
			return r
		}
	}
	t.Fatal("saved release absent")
	return appRelease{}
}
func TestAppReleasesPlatformVersionAndDurability(t *testing.T) {
	h := systemHarness(t)
	first := releaseItem(t, h, releaseInput("mobile", "android", "1.9.0"), "POST")
	in := releaseInput("mobile", "android", "1.10.0")
	in["force"] = true
	latest := releaseItem(t, h, in, "POST")
	releaseItem(t, h, releaseInput("mobile", "ios", "2.0.0"), "POST")
	releaseItem(t, h, releaseInput("tablet", "android", "3.0.0"), "POST")
	releaseItem(t, h, releaseInput("tv", "android", "4.0.0"), "POST")
	releaseItem(t, h, releaseInput("tablet", "ios", "5.0.0"), "POST")
	releaseItem(t, h, releaseInput("web", "windows", "6.0.0"), "POST")
	for _, tc := range []struct {
		p, os, ver string
		required   bool
	}{{"mobile", "android", "1.10.0", true}, {"mobile", "ios", "2.0.0", false}, {"tablet", "android", "3.0.0", false}, {"tablet", "ios", "5.0.0", false}, {"tv", "android", "4.0.0", false}, {"web", "windows", "6.0.0", false}} {
		w := h.req("GET", "/api/client/bootstrap?platform="+tc.p+"&os="+tc.os+"&version=1.0.0", nil, "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		out := payload(t, w)
		u := obj(out["update"])
		if u["version"] != tc.ver || u["available"] != true || u["required"] != tc.required || u["sizeMB"] != 35.8 || u["browserUrl"] != "https://downloads.example.test/guide" {
			t.Fatalf("%s/%s: %+v", tc.p, tc.os, u)
		}
		settings := obj(obj(out["config"])["clientSettings"])
		if settings["latestVersion"] != tc.ver || settings["notifyOnLaunch"] != true {
			t.Fatal("legacy config not projected", settings)
		}
	}
	for _, ver := range []string{"1.10.0", "9.0.0"} {
		w := h.req("GET", "/api/client/bootstrap?platform=mobile&os=android&version="+ver, nil, "")
		u := obj(payload(t, w)["update"])
		if u["available"] != false || u["required"] != false {
			t.Fatal("non-older client forced", u)
		}
	}
	before := h.a.readState()
	if before.Operations["mobile"].PublishedVersion <= 1 {
		t.Fatal("client snapshot revision did not change")
	}
	stored, e := h.a.store.Load(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if got := latestAppRelease(stored, "mobile", "android"); got == nil || got.ID != latest.ID {
		t.Fatal("not durable")
	}
	in = releaseInput("mobile", "android", latest.Version)
	in["id"] = latest.ID
	in["revision"] = latest.Revision
	in["enabled"] = false
	releaseItem(t, h, in, "PUT")
	if got := latestAppRelease(h.a.readState(), "mobile", "android"); got == nil || got.ID != first.ID {
		t.Fatal("disabled newest did not fall back")
	}
	if w := h.req("PUT", "/api/admin/releases", in, ""); w.Code != 409 {
		t.Fatal("lost update not rejected", w.Code, w.Body)
	}
}
func TestAppReleaseValidationAndAuthentication(t *testing.T) {
	h := systemHarness(t)
	cookie := h.cookie
	h.cookie = ""
	for _, method := range []string{"GET", "POST", "PUT"} {
		if w := h.req(method, "/api/admin/releases", releaseInput("mobile", "android", "1.0.0"), ""); w.Code != 401 {
			t.Fatal("unprotected release endpoint", w.Code)
		}
	}
	h.cookie = cookie
	r := httptest.NewRequest("POST", h.origin+"/api/admin/releases", strings.NewReader(`{}`))
	r.Header.Set("Origin", "https://evil.test")
	r.AddCookie(&http.Cookie{Name: "hetun_session", Value: cookie})
	w := httptest.NewRecorder()
	h.a.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin write allowed", w.Code)
	}
	for _, tc := range []struct {
		k string
		v any
	}{{"version", "1.2"}, {"version", "1000000000.0.0"}, {"sizeMB", 0.0}, {"sizeMB", -1.0}, {"sizeMB", "20"}, {"downloadUrl", "http://example.test/a.apk"}, {"downloadUrl", "https://user:secret@example.test/a.apk"}, {"browserUrl", "javascript:alert(1)"}, {"notes", ""}, {"notes", strings.Repeat("字", 501)}, {"platform", "other"}, {"os", "mac"}, {"force", "yes"}, {"enabled", "yes"}} {
		in := releaseInput("mobile", "android", "1.0.0")
		in[tc.k] = tc.v
		if w := h.req("POST", "/api/admin/releases", in, ""); w.Code != 400 {
			t.Fatalf("%s=%v accepted: %d %s", tc.k, tc.v, w.Code, w.Body)
		}
	}
	first := releaseItem(t, h, releaseInput("mobile", "android", "01.00.0"), "POST")
	if w := h.req("POST", "/api/admin/releases", releaseInput("mobile", "android", "1.0.0"), ""); w.Code != 409 {
		t.Fatal("canonical duplicate accepted", w.Code)
	}
	in := releaseInput("tablet", "android", first.Version)
	in["id"] = first.ID
	in["revision"] = first.Revision
	if w := h.req("PUT", "/api/admin/releases", in, ""); w.Code != 400 {
		t.Fatal("platform moved", w.Code)
	}
	if w := h.req("POST", "/api/admin/releases/delete", M{"id": first.ID, "revision": 0}, ""); w.Code != 409 {
		t.Fatal("stale delete accepted", w.Code)
	}
	if w := h.req("POST", "/api/admin/releases/delete", M{"id": first.ID, "revision": first.Revision}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if u := obj(payload(t, h.req("GET", "/api/client/bootstrap?platform=mobile&os=android&version=0.0.0", nil, ""))["update"]); u["available"] != false || u["required"] != false {
		t.Fatal("deleted update still offered", u)
	}
}
func TestAppReleaseLegacyMigrationAndDeleteDoNotResurrect(t *testing.T) {
	v := initialState()
	settings := obj(v.Operations["mobile"].Published["clientSettings"])
	settings["latestVersion"] = "2.1.0"
	settings["androidUrl"] = "https://example.test/a.apk"
	settings["iosUrl"] = "https://apps.apple.com/app/id1"
	settings["forceUpdate"] = true
	settings["updateNotes"] = "真实旧版更新说明"
	migrateAppReleases(&v)
	migrateAppReleases(&v)
	if len(v.AppReleases) != 2 {
		t.Fatal("migration duplicate or missing", v.AppReleases)
	}
	r := latestAppRelease(v, "mobile", "android")
	if r == nil || !r.Legacy || !r.Force {
		t.Fatal("legacy settings lost")
	}
	if e := deleteAppRelease(&v, M{"id": r.ID, "revision": r.Revision}); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(v)
	var restored state
	_ = json.Unmarshal(b, &restored)
	migrateAppReleases(&restored)
	if latestAppRelease(restored, "mobile", "android") != nil {
		t.Fatal("deleted legacy release resurrected")
	}
	out, e := bootstrap(restored, "mobile", "android", "1.0.0", "https://example.test")
	if e != nil {
		t.Fatal(e)
	}
	if obj(out["update"])["available"] != false || obj(obj(out["config"])["clientSettings"])["latestVersion"] != "" {
		t.Fatal("legacy fallback resurrected deleted update", out)
	}
	if got := obj(v.Operations["mobile"].Published["clientSettings"])["latestVersion"]; got != "2.1.0" {
		t.Fatal("bootstrap mutated stored config")
	}
}
