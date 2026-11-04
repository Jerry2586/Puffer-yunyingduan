package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func announcementFixture(title, text string, enabled bool) M {
	cfg := clone(obj(defaults["announcement"]))
	cfg["title"], cfg["text"], cfg["enabled"], cfg["style"] = title, text, enabled, "modal"
	return cfg
}

func TestAnnouncementsMigrationPreservesDistinctLegacyDocuments(t *testing.T) {
	v := initialState()
	v.Announcements = nil
	shared := announcementFixture("共用公告", "已发布内容", true)
	v.Operations["mobile"].Published["announcement"] = clone(shared)
	shared["enabled"] = false
	v.Operations["mobile"].Draft["announcement"] = clone(shared)
	shared["enabled"] = true
	v.Operations["tv"].Published["announcement"] = clone(shared)
	v.Operations["tablet"].Published["announcement"] = announcementFixture("第二条", "另一个已发布公告", true)
	v.Operations["tablet"].Draft["announcement"] = announcementFixture("未发布", "仅存在草稿中", true)
	metadata := clone(obj(defaults["announcement"]))
	metadata["url"] = "https://example.test/legacy"
	v.Operations["web"].Draft["announcement"] = metadata
	before := clone(v)
	// Read-only fallbacks may project the migration, but may not mutate old state.
	report := announcementsReport(v)
	projected := currentAnnouncement(v)
	if len(report.Items) != 4 || projected["title"] != "共用公告" || !reflect.DeepEqual(v, before) {
		t.Fatal("read fallback lost content or mutated legacy state")
	}
	if !migrateAnnouncements(&v) || len(v.Announcements.Items) != 4 {
		t.Fatal("legacy announcements were lost or duplicated")
	}
	active := 0
	for _, entry := range v.Announcements.Items {
		if entry.Config["enabled"] == true {
			active++
			if entry.Config["title"] != "共用公告" {
				t.Fatal("wrong published announcement activated")
			}
		}
		if entry.Config["title"] == "共用公告" {
			want := []string{"手机 · 已发布", "手机 · 草稿", "电视 · 已发布"}
			if !reflect.DeepEqual(entry.Sources, want) {
				t.Fatal("source labels lost", entry.Sources)
			}
		}
	}
	if active != 1 {
		t.Fatal("migration must retain only one active published announcement", active)
	}
	for _, p := range platforms {
		old, next := before.Operations[p], v.Operations[p]
		if !reflect.DeepEqual(old.Draft, next.Draft) || !reflect.DeepEqual(old.Published, next.Published) || old.Version != next.Version {
			t.Fatal("migration modified original platform documents", p)
		}
		if next.PublishedVersion != old.PublishedVersion+1 {
			t.Fatal("client version not advanced", p)
		}
	}
	migrated := clone(v)
	if migrateAnnouncements(&v) || !reflect.DeepEqual(v, migrated) {
		t.Fatal("migration is not idempotent")
	}
	if err := validateState(v); err != nil {
		t.Fatal(err)
	}
}

func TestAnnouncementsMigrationNeverPublishesDraftOnlyNotice(t *testing.T) {
	v := initialState()
	v.Announcements = nil
	v.Operations["mobile"].Draft["announcement"] = announcementFixture("未发布草稿", "不可自动上线", true)
	migrateAnnouncements(&v)
	if len(v.Announcements.Items) != 1 || v.Announcements.Items[0].Config["enabled"] != false || currentAnnouncement(v)["text"] != "" {
		t.Fatal("draft announcement was activated")
	}
	empty := initialState()
	empty.Announcements = nil
	migrateAnnouncements(&empty)
	if len(empty.Announcements.Items) != 0 {
		t.Fatal("untouched default imported as user content")
	}
}

func TestAnnouncementsMigrationIsDurableBeforeStartup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := &fileStore{filepath.Join(dir, "state.json")}
	v := initialState()
	v.Announcements = nil
	v.Operations["mobile"].Published["announcement"] = announcementFixture("迁移公告", "重启后保留", true)
	if err := s.Save(ctx, v, 0); err != nil {
		t.Fatal(err)
	}
	c := config{PublicOrigin: "http://127.0.0.1:8787", DataDir: dir}
	a, err := newApp(ctx, c, s)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := s.Load(ctx)
	if err != nil || stored.Announcements == nil || stored.Revision != v.Revision+1 || !reflect.DeepEqual(stored, a.readState()) {
		t.Fatal("startup returned before migration was durable", err)
	}
	restarted, err := newApp(ctx, c, s)
	if err != nil || !reflect.DeepEqual(stored, restarted.readState()) {
		t.Fatal("restart changed migrated state", err)
	}
	failDir := t.TempDir()
	failStore := &fileStore{filepath.Join(failDir, "state.json")}
	if err := failStore.Save(ctx, v, 0); err != nil {
		t.Fatal(err)
	}
	c.DataDir = failDir
	if _, err := newApp(ctx, c, &applyFailStore{failStore}); err == nil {
		t.Fatal("startup accepted a failed migration save")
	}
	untouched, err := failStore.Load(ctx)
	if err != nil || !reflect.DeepEqual(untouched, v) {
		t.Fatal("failed startup changed last good data", err)
	}
}

func postAnnouncement(t *testing.T, h *harness, input M) *announcementState {
	t.Helper()
	if _, ok := input["version"]; !ok {
		input["version"] = h.a.readState().Announcements.Version
	}
	w := h.req("POST", "/api/admin/announcements", input, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var result announcementState
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return &result
}

func assertSharedAnnouncement(t *testing.T, h *harness, want M) {
	t.Helper()
	for _, p := range platforms {
		name := p
		if p == "web" {
			name = "desktop"
		}
		for _, path := range []string{"/api/v1/" + name + "/bootstrap", "/api/client/bootstrap?platform=" + p} {
			w := h.req("GET", path, nil, "")
			out := payload(t, w)
			if w.Code != 200 || !reflect.DeepEqual(obj(obj(out["config"])["announcement"]), want) || number(out["pollSeconds"]) != 15 {
				t.Fatal("clients do not share active announcement", path, w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "sources") || strings.Contains(w.Body.String(), "createdAt") || strings.Contains(w.Body.String(), "notice-") {
				t.Fatal("private announcement collection leaked to public bootstrap", path)
			}
		}
	}
}

func TestAnnouncementsCRUDSynchronizesFourClientsAndCache(t *testing.T) {
	h := newHarness(t, "")
	enableObjectCache(t, h)
	before := h.a.readState()
	// Warm Redis before changing announcements, so stale source keys are detected.
	assertSharedAnnouncement(t, h, currentAnnouncement(before))
	first := announcementFixture("全端公告", "手机平板电视电脑一致", true)
	first["style"] = "ticker"
	first["url"] = "https://example.test/activity"
	first["image"] = "https://example.test/banner.png"
	result := postAnnouncement(t, h, M{"action": "create", "config": first})
	id := result.Items[0].ID
	if result.Version != 2 || !reflect.DeepEqual(result.Items[0].Config, first) {
		t.Fatal("create contract mismatch")
	}
	assertSharedAnnouncement(t, h, first)
	for _, p := range platforms {
		after := h.a.readState().Operations[p]
		if after.PublishedVersion != before.Operations[p].PublishedVersion+1 || after.Version != before.Operations[p].Version || !reflect.DeepEqual(after.Draft, before.Operations[p].Draft) {
			t.Fatal("shared announcement changed platform draft or failed to advance public version", p)
		}
	}
	second := announcementFixture("第二条公告", "启用后替换上一条", true)
	result = postAnnouncement(t, h, M{"action": "create", "config": second})
	secondID := result.Items[1].ID
	if result.Items[0].Config["enabled"] != false {
		t.Fatal("old announcement still active")
	}
	assertSharedAnnouncement(t, h, second)
	second["text"] = "编辑后自动保存的正文"
	postAnnouncement(t, h, M{"action": "update", "id": secondID, "config": second})
	assertSharedAnnouncement(t, h, second)
	postAnnouncement(t, h, M{"action": "toggle", "id": secondID, "enabled": false})
	assertSharedAnnouncement(t, h, currentAnnouncement(h.a.readState()))
	postAnnouncement(t, h, M{"action": "toggle", "id": id, "enabled": true})
	assertSharedAnnouncement(t, h, first)
	postAnnouncement(t, h, M{"action": "delete", "id": id})
	if currentAnnouncement(h.a.readState())["enabled"] != false || currentAnnouncement(h.a.readState())["text"] != "" {
		t.Fatal("deleted active announcement still exposed")
	}
	assertSharedAnnouncement(t, h, currentAnnouncement(h.a.readState()))
	durable, err := h.a.store.Load(context.Background())
	if err != nil || !reflect.DeepEqual(durable.Announcements, h.a.readState().Announcements) {
		t.Fatal("CRUD was not durable", err)
	}
}

func TestAnnouncementsRejectInvalidWritesAndKeepLastGoodState(t *testing.T) {
	h := newHarness(t, "")
	result := postAnnouncement(t, h, M{"action": "create", "config": announcementFixture("有效公告", "有效内容", true)})
	id, version := result.Items[0].ID, result.Version
	before := h.a.readState()
	cases := []struct {
		name   string
		input  M
		status int
	}{
		{"stale version", M{"version": version - 1, "action": "delete", "id": id}, 409},
		{"string version", M{"version": "2", "action": "delete", "id": id}, 409},
		{"missing item", M{"version": version, "action": "delete", "id": "notice-missing"}, 404},
		{"bad toggle", M{"version": version, "action": "toggle", "id": id, "enabled": "true"}, 400},
		{"bad config", M{"version": version, "action": "update", "id": id, "config": "invalid"}, 400},
		{"bad action", M{"version": version, "action": "invalid", "id": id}, 400},
	}
	for _, change := range []struct {
		name, field string
		value       any
	}{
		{"empty title", "title", " "}, {"enabled blank text", "text", " "}, {"numeric text", "text", 123},
		{"invalid style", "style", "html"}, {"invalid frequency", "frequency", "hourly"}, {"invalid page", "page", "unknown"},
		{"script URL", "url", "javascript:alert(1)"}, {"relative URL", "url", "/api/assets/abcdef.png"},
		{"credential URL", "url", "https://user:pass@example.test"}, {"whitespace URL", "url", "https://example.test/a b"},
		{"backslash URL", "url", "https://example.test/a\\b"}, {"oversize title", "title", strings.Repeat("字", 121)},
		{"oversize text", "text", strings.Repeat("字", 8001)}, {"oversize action", "actionLabel", strings.Repeat("字", 41)},
		{"oversize URL", "url", "https://example.test/" + strings.Repeat("a", 2048)},
	} {
		cfg := announcementFixture("有效公告", "有效内容", true)
		cfg[change.field] = change.value
		cases = append(cases, struct {
			name   string
			input  M
			status int
		}{change.name, M{"version": version, "action": "update", "id": id, "config": cfg}, 400})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := h.req("POST", "/api/admin/announcements", tc.input, "")
			if w.Code != tc.status || !reflect.DeepEqual(before, h.a.readState()) {
				t.Fatal("invalid write changed last good state", w.Code, w.Body)
			}
		})
	}
	original := h.a.store
	h.a.store = &applyFailStore{original}
	w := h.req("POST", "/api/admin/announcements", M{"version": version, "action": "delete", "id": id}, "")
	h.a.store = original
	if w.Code < 500 || !reflect.DeepEqual(before, h.a.readState()) {
		t.Fatal("failed save changed memory", w.Code, w.Body)
	}
	durable, err := original.Load(context.Background())
	if err != nil || !reflect.DeepEqual(durable, before) {
		t.Fatal("failed save changed durable state", err)
	}
	cookie := h.cookie
	h.cookie = ""
	if w = h.req("GET", "/api/admin/announcements", nil, ""); w.Code != 401 {
		t.Fatal("anonymous list accepted", w.Code)
	}
	if w = h.req("POST", "/api/admin/announcements", M{"version": version, "action": "delete", "id": id}, ""); w.Code != 401 {
		t.Fatal("anonymous write accepted", w.Code)
	}
	h.cookie = cookie
	for _, tc := range []struct{ name, origin, client, body string }{
		{"cross origin", "https://evil.test", "studio", "{}"},
		{"missing client", h.origin, "", "{}"},
		{"oversize body", h.origin, "studio", "{\"padding\":\"" + strings.Repeat("x", 65*1024) + "\"}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", h.origin+"/api/admin/announcements", strings.NewReader(tc.body))
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("X-Hetun-Client", tc.client)
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(&http.Cookie{Name: "hetun_session", Value: cookie})
			w := httptest.NewRecorder()
			h.a.ServeHTTP(w, r)
			if w.Code < 400 || !reflect.DeepEqual(before, h.a.readState()) {
				t.Fatal("untrusted/oversized request accepted", w.Code, w.Body)
			}
		})
	}
}

func TestAnnouncementsSurviveStalePlatformApplyPublishAndRollback(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	shared := announcementFixture("统一维护的公告", "不可被旧草稿覆盖", true)
	postAnnouncement(t, h, M{"action": "create", "config": shared})
	for _, p := range platforms {
		name := p
		if p == "web" {
			name = "desktop"
		}
		base := "/api/admin/clients/" + name
		stale := clone(defaults)
		stale["announcement"] = announcementFixture("旧端公告", "不应重新出现", true)
		for _, action := range []string{"draft", "apply", "publish", "rollback"} {
			record := h.a.readState().Operations[p]
			input := M{"version": record.Version}
			method := "POST"
			if action == "draft" || action == "apply" {
				method = "PUT"
				input["theme"] = stale
			}
			if action == "rollback" {
				input["targetVersion"] = record.History[len(record.History)-1].Version
			}
			w := h.req(method, base+"/"+action, input, "")
			if w.Code != 200 {
				t.Fatal(p, action, w.Code, w.Body)
			}
			updated := h.a.readState()
			if !reflect.DeepEqual(currentAnnouncement(updated), shared) || !reflect.DeepEqual(obj(updated.Operations[p].Draft["announcement"]), shared) {
				t.Fatal("platform operation overwrote shared announcement", p, action)
			}
			stateResponse := payload(t, h.req("GET", base+"/state", nil, ""))
			if !reflect.DeepEqual(obj(obj(stateResponse["draft"])["announcement"]), shared) || !reflect.DeepEqual(obj(obj(stateResponse["published"])["announcement"]), shared) {
				t.Fatal("studio exposed old platform notice", p, action)
			}
		}
	}
	assertSharedAnnouncement(t, h, shared)
}
