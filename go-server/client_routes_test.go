package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestScopedOperationsDurabilityAndLegacyParity(t *testing.T) {
	h := systemHarness(t)
	for _, p := range platforms {
		name := clientName(p)
		base := "/api/admin/clients/" + name
		draft := clone(defaults)
		obj(arr(draft["sections"])[0])["title"] = "only-" + p
		w := h.req("PUT", base+"/draft", M{"version": 1, "theme": draft}, "")
		if w.Code != 200 {
			t.Fatal(name, w.Code, w.Body)
		}
		if w = h.req("POST", base+"/publish", M{"version": 2}, ""); w.Code != 200 {
			t.Fatal(name, w.Code, w.Body)
		}
		public := "/api/v1/" + name + "/bootstrap?os=android"
		if p == "web" {
			public = "/api/v1/desktop/bootstrap?os=windows"
		}
		w = h.req("GET", public, nil, "")
		old := h.req("GET", "/api/client/bootstrap?platform="+p+"&os="+map[bool]string{true: "windows", false: "android"}[p == "web"], nil, "")
		if w.Code != 200 || old.Code != 200 || !reflect.DeepEqual(payload(t, w), payload(t, old)) {
			t.Fatal(name, w.Code, w.Body, old.Body)
		}
		if !strings.Contains(w.Body.String(), "only-"+p) {
			t.Fatal("wrong config", w.Body)
		}
		if p != "mobile" && strings.Contains(w.Body.String(), "only-mobile") {
			t.Fatal("platform bleed", w.Body)
		}
		scopedState := h.req("GET", base+"/state", nil, "")
		oldState := h.req("GET", "/api/admin/state?template=runtime-"+p, nil, "")
		if scopedState.Code != 200 || !reflect.DeepEqual(payload(t, scopedState), payload(t, oldState)) {
			t.Fatal("state alias mismatch", scopedState.Body, oldState.Body)
		}
		if w = h.req("PUT", base+"/draft", M{"version": 1, "theme": draft}, ""); w.Code != 409 {
			t.Fatal("stale write accepted", w.Code, w.Body)
		}
	}
	restored, err := h.a.store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range platforms {
		raw, _ := json.Marshal(restored.Operations[p].Published)
		if !strings.Contains(string(raw), "only-"+p) {
			t.Fatal("not durable", p, string(raw))
		}
	}
}

func TestScopedRoutesRejectConflictsAndPreserveAdminBoundary(t *testing.T) {
	h := systemHarness(t)
	for _, route := range []string{
		"/api/v1/tv/bootstrap?platform=mobile", "/api/v1/tv/bootstrap?platform=tv&platform=tv",
		"/api/v1/tv/bootstrap?platform=", "/api/v1/tv/bootstrap?x=%ZZ",
		"/api/admin/clients/tv/state?template=mobile", "/api/admin/clients/mobile/state?template=",
		"/api/admin/clients/tv/state?template=tv&template=tv",
	} {
		if w := h.req("GET", route, nil, ""); w.Code != 400 {
			t.Fatal(route, w.Code, w.Body)
		}
	}
	for _, route := range []string{"/api/v1/web/bootstrap", "/api/v1/invalid/bootstrap", "/api/v1/tv/system/config", "/api/admin/clients/tv/cms", "/api/v1/tv/ads/callback/admob"} {
		if w := h.req("GET", route, nil, ""); w.Code != 404 {
			t.Fatal(route, w.Code, w.Body)
		}
	}
	for _, template := range []any{"mobile", "", 42} {
		if w := h.req("PUT", "/api/admin/clients/tv/draft", M{"template": template, "theme": defaults, "version": 1}, ""); w.Code != 400 {
			t.Fatal(template, w.Code, w.Body)
		}
	}
	for _, headers := range [][]string{{"mobile"}, {"tv", "tv"}, {""}} {
		r := httptest.NewRequest("GET", h.origin+"/api/v1/tv/bootstrap", nil)
		for _, value := range headers {
			r.Header.Add("X-Hetun-Platform", value)
		}
		w := httptest.NewRecorder()
		h.a.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(headers, w.Code, w.Body)
		}
	}
	cookie := h.cookie
	h.cookie = ""
	if w := h.req("GET", "/api/admin/clients/tv/state", nil, ""); w.Code != 401 {
		t.Fatal("admin auth bypass", w.Code)
	}
	h.cookie = cookie
	r := httptest.NewRequest("PUT", h.origin+"/api/admin/clients/tv/draft", strings.NewReader(`{"version":1}`))
	r.AddCookie(&http.Cookie{Name: "hetun_session", Value: cookie})
	r.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	h.a.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("origin bypass", w.Code, w.Body)
	}
}

func TestScopedReleasesIsolationAndMutationGuards(t *testing.T) {
	h := systemHarness(t)
	var mobile appRelease
	for _, p := range platforms {
		os := "android"
		if p == "web" {
			os = "windows"
		}
		w := h.req("POST", "/api/admin/clients/"+clientName(p)+"/releases", releaseInput(p, os, "1.0.0"), "")
		if w.Code != 200 {
			t.Fatal(p, w.Code, w.Body)
		}
		rows := arr(payload(t, w)["items"])
		if len(rows) != 1 || obj(rows[0])["platform"] != p {
			t.Fatal("unfiltered response", w.Body)
		}
		if p == "mobile" {
			raw, _ := json.Marshal(rows[0])
			if err := json.Unmarshal(raw, &mobile); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, p := range platforms {
		w := h.req("GET", "/api/admin/clients/"+clientName(p)+"/releases", nil, "")
		if len(arr(payload(t, w)["items"])) != 1 {
			t.Fatal("cross-client list", w.Body)
		}
	}
	in := releaseInput("tv", "android", "1.0.1")
	in["id"] = mobile.ID
	in["revision"] = mobile.Revision
	for _, tc := range []struct {
		route, method string
		body          M
		code          int
	}{
		{"releases", "PUT", in, 404}, {"releases/delete", "POST", M{"id": mobile.ID, "revision": mobile.Revision}, 404},
		{"releases", "POST", releaseInput("mobile", "android", "2.0.0"), 400},
	} {
		if w := h.req(tc.method, "/api/admin/clients/tv/"+tc.route, tc.body, ""); w.Code != tc.code {
			t.Fatal(w.Code, w.Body)
		}
	}
	in = releaseInput("mobile", "android", "1.0.1")
	in["id"] = mobile.ID
	in["revision"] = mobile.Revision
	if w := h.req("PUT", "/api/admin/clients/mobile/releases", in, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := h.req("PUT", "/api/admin/clients/mobile/releases", in, ""); w.Code != 409 {
		t.Fatal("stale release accepted", w.Code, w.Body)
	}
	in = M{"id": mobile.ID, "revision": mobile.Revision + 1}
	if w := h.req("POST", "/api/admin/clients/mobile/releases/delete", in, ""); w.Code != 200 || len(arr(payload(t, w)["items"])) != 0 {
		t.Fatal(w.Code, w.Body)
	}
	stored, err := h.a.store.Load(context.Background())
	if err != nil || len(stored.AppReleases) != 3 {
		t.Fatal("release persistence", err, stored.AppReleases)
	}
	if w := h.req("GET", "/api/admin/releases", nil, ""); len(arr(payload(t, w)["items"])) != 3 {
		t.Fatal("legacy list broken", w.Body)
	}
}

func TestScopedAdsAndCacheUseSamePersistentBusiness(t *testing.T) {
	h := systemHarness(t)
	report := payload(t, h.req("GET", "/api/admin/clients/tv/ads", nil, ""))
	w := h.req("PUT", "/api/admin/clients/tv/ads", M{"version": report["version"], "config": adFixture("splash", "android")}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = h.req("POST", "/api/admin/clients/tv/ads/publish", M{"version": payload(t, w)["version"]}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	cfg := payload(t, h.req("GET", "/api/v1/tv/ads/config", nil, ""))
	if cfg["decisionUrl"] != "/api/v1/tv/ads/decision" || cfg["eventUrl"] != "/api/v1/tv/ads/events" {
		t.Fatal(cfg)
	}
	input := M{"slot": "splash", "os": "android", "deviceId": "scoped-device-123456"}
	w = h.req("POST", "/api/v1/tv/ads/decision", input, "")
	out := payload(t, w)
	if w.Code != 200 || out["fill"] != true {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("POST", "/api/v1/mobile/ads/events", M{"decisionId": out["decisionId"], "type": "impression"}, ""); w.Code != 404 {
		t.Fatal("foreign decision accepted", w.Code, w.Body)
	}
	if w = h.req("POST", "/api/v1/tv/ads/events", M{"decisionId": out["decisionId"], "type": "impression"}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("POST", "/api/v1/mobile/ads/decision", input, ""); payload(t, w)["fill"] != false {
		t.Fatal("ad bleed", w.Body)
	}
	stored, err := h.a.store.Load(context.Background())
	if err != nil || stored.Advertising["tv"].PublishedVersion < 1 {
		t.Fatal("ad persistence", err)
	}
	if w = h.req("POST", "/api/admin/clients/tablet/cache/clear", M{"kind": "catalog"}, ""); w.Code != 409 {
		t.Fatal("fake cache success", w.Code, w.Body)
	}
	redisServer := miniredis.RunT(t)
	cfgRedis := &redisConfig{Address: redisServer.Addr()}
	_ = cfgRedis.normalize()
	h.a.cache = newContentCache(cfgRedis, h.a.config.DataDir)
	defer h.a.cache.close()
	redisServer.Set("other-application", "keep")
	if w = h.req("POST", "/api/admin/clients/tablet/cache/clear", M{"kind": "catalog"}, ""); w.Code != 200 || payload(t, w)["platform"] != "tablet" {
		t.Fatal(w.Code, w.Body)
	}
	if value, _ := redisServer.Get("other-application"); value != "keep" {
		t.Fatal("unrelated redis touched")
	}
	redisServer.RequireAuth("do-not-expose")
	if w = h.req("POST", "/api/admin/clients/tablet/cache/clear", M{"kind": "catalog"}, ""); w.Code != 503 {
		t.Fatal("failed redis reported success", w.Code, w.Body)
	}
}

func TestScopedCMSAndSharedMemberSession(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	for _, p := range platforms {
		route := "/api/v1/" + clientName(p)
		for _, endpoint := range []string{"catalog", "catalog/categories"} {
			w := h.req("GET", route+"/"+endpoint, nil, "")
			old := h.req("GET", "/api/"+endpoint+"?platform="+p, nil, "")
			if w.Code != 200 || !reflect.DeepEqual(payload(t, w), payload(t, old)) {
				t.Fatal(route, endpoint, w.Code, w.Body, old.Body)
			}
		}
	}
	w := h.req("POST", "/api/v1/mobile/member/login", M{"username": "viewer", "password": "correct"}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	token := str(payload(t, w)["token"])
	for _, name := range []string{"mobile", "tablet", "tv", "desktop"} {
		w = h.req("GET", "/api/v1/"+name+"/member/profile", nil, token)
		if w.Code != 200 || obj(payload(t, w)["profile"])["user_name"] != "viewer" {
			t.Fatal(name, w.Code, w.Body)
		}
	}
	w = h.req("POST", "/api/v1/tv/playback", M{"id": "1", "sid": 1, "nid": 1}, token)
	if w.Code != 200 || payload(t, w)["url"] != "https://video.test/1.m3u8" {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("POST", "/api/v1/desktop/member/logout", M{}, token); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("GET", "/api/v1/mobile/member/profile", nil, token); w.Code != 401 {
		t.Fatal("logout did not revoke shared session", w.Code, w.Body)
	}
}
