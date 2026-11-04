package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// This is a synthetic test key, never an installation credential.
func managementFixture(t *testing.T, respond func(http.ResponseWriter, M)) (*harness, *atomic.Int32) {
	t.Helper()
	catalog := fixtureCMS(t)
	t.Cleanup(catalog.Close)
	calls := &atomic.Int32{}
	key := strings.Repeat("a", 64)
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index.php/hetunops/gateway" {
			catalog.Config.Handler.ServeHTTP(w, r)
			return
		}
		calls.Add(1)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error("gateway request unreadable")
			w.WriteHeader(400)
			return
		}
		stamp, err := strconv.ParseInt(r.Header.Get("X-Hetun-Time"), 10, 64)
		nonce := r.Header.Get("X-Hetun-Nonce")
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(r.Header.Get("X-Hetun-Time") + "\n" + nonce + "\n"))
		mac.Write(raw)
		sig, sigErr := hex.DecodeString(r.Header.Get("X-Hetun-Signature"))
		if r.Method != "POST" || err != nil || time.Since(time.Unix(stamp, 0)).Abs() > time.Minute || nonce == "" || sigErr != nil || !hmac.Equal(sig, mac.Sum(nil)) {
			t.Error("gateway signature or request envelope invalid")
			w.WriteHeader(403)
			return
		}
		var input M
		if json.Unmarshal(raw, &input) != nil {
			t.Error("gateway JSON invalid")
			w.WriteHeader(400)
			return
		}
		respond(w, input)
	}))
	t.Cleanup(cms.Close)
	h := newHarness(t, cms.URL)
	w := h.req("POST", "/api/admin/manage/connection", M{"source": managementBase(cms.URL), "key": key}, "")
	if w.Code != 200 {
		t.Fatalf("connection status %d", w.Code)
	}
	return h, calls
}
func managementFixtureResponse(w http.ResponseWriter, input M) {
	switch str(input["action"]) {
	case "meta":
		jsonResponse(w, 200, M{"code": 1, "data": M{"adapterVersion": 1, "categories": []any{M{"id": 1, "name": "电影", "parentId": 0}}, "groups": []any{M{"id": 2, "name": "会员"}}}})
	case "videos.list":
		jsonResponse(w, 200, M{"code": 1, "data": M{"items": []any{M{"id": 1, "name": "影片", "status": 1, "cover": "", "categoryId": 1, "categoryName": "电影", "sources": "线路", "year": "2026", "area": "中国", "level": 0}}, "total": 1, "pageSize": 20, "page": 1, "pages": 1}})
	default:
		jsonResponse(w, 200, M{"code": 1, "data": M{"ok": true, "affected": 1}})
	}
}
func TestManagementAdminBoundaryAndSignedGateway(t *testing.T) {
	h, calls := managementFixture(t, managementFixtureResponse)
	for _, route := range []string{"/api/admin/manage/connection", "/api/admin/manage/videos", "/api/admin/manage/adapter", "/api/admin/manage/moderation"} {
		req := httptest.NewRequest("GET", h.origin+route, nil)
		w := httptest.NewRecorder()
		h.a.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("anonymous management status %d", w.Code)
		}
	}
	req := httptest.NewRequest("POST", h.origin+"/api/admin/manage/videos/action", strings.NewReader("{}"))
	req.Header.Set("Origin", "https://untrusted.test")
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "hetun_session", Value: h.cookie})
	blocked := httptest.NewRecorder()
	h.a.ServeHTTP(blocked, req)
	if blocked.Code != 403 || calls.Load() != 0 {
		t.Fatal("cross-origin write reached adapter")
	}
	for _, route := range []string{"/api/admin/manage/connection", "/api/admin/state"} {
		w := h.req("GET", route, nil, "")
		if w.Code != 200 || strings.Contains(w.Body.String(), strings.Repeat("a", 64)) || strings.Contains(w.Body.String(), "managementConnection") {
			t.Fatal("admin response leaked management credential")
		}
	}
	for _, route := range []string{"meta", "videos?pageSize=20"} {
		w := h.req("GET", "/api/admin/manage/"+route, nil, "")
		if w.Code != 200 || str(payload(t, w)["source"]) != managementBase(h.a.readState().CMSBase) {
			t.Fatalf("signed read status %d", w.Code)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected gateway requests")
	}
}
func TestManagementMutationGuardsAndUnconfirmedWrite(t *testing.T) {
	h, calls := managementFixture(t, func(w http.ResponseWriter, _ M) { _, _ = w.Write([]byte("{broken")) })
	base := managementBase(h.a.readState().CMSBase)
	for _, data := range []M{
		{"source": base + "/old", "action": "status", "ids": []any{1}, "value": 0},
		{"source": base, "action": "status", "ids": []any{1.5}, "value": 0},
		{"source": base, "action": "edit", "ids": []any{1, 2}, "fields": M{}},
		{"source": base, "action": "delete", "ids": []any{1}},
	} {
		w := h.req("POST", "/api/admin/manage/videos/action", data, "")
		if w.Code != 400 && w.Code != 409 {
			t.Fatalf("invalid mutation status %d", w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid mutation forwarded")
	}
	w := h.req("POST", "/api/admin/manage/videos/action", M{"source": base, "action": "status", "ids": []any{1}, "value": 0}, "")
	if w.Code != 502 || str(payload(t, w)["code"]) != "CMS_WRITE_UNCONFIRMED" || calls.Load() != 1 {
		t.Fatal("uncertain mutation must not retry or report success")
	}
}
func TestManagementRejectsMalformedSuccess(t *testing.T) {
	fixtures := []struct {
		name, action string
		data         M
	}{
		{"missing-result", "videos.status", M{}},
		{"missing-confirmation", "videos.status", M{"affected": 1}},
		{"fractional-affected", "users.edit", M{"ok": true, "affected": 1.2}},
		{"unsupported-adapter", "meta", M{"adapterVersion": 2, "categories": []any{}, "groups": []any{}}},
		{"invalid-pagination", "videos.list", M{"items": []any{}, "total": 1, "page": 1, "pages": 4, "pageSize": 20}},
		{"missing-user-fields", "users.list", M{"items": []any{M{"id": 1, "name": "alice", "status": 1}}, "total": 1, "page": 1, "pages": 1, "pageSize": 20}},
		{"fractional-id", "videos.list", M{"items": []any{M{"id": 1.5, "name": "影片", "status": 1}}, "total": 1, "page": 1, "pages": 1, "pageSize": 20}},
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.data)
			var normalized M
			_ = json.Unmarshal(raw, &normalized)
			if managementResultValid(tc.action, normalized) {
				t.Fatal("malformed adapter result accepted")
			}
		})
	}
	h, calls := managementFixture(t, func(w http.ResponseWriter, _ M) {
		jsonResponse(w, 200, M{"code": 1, "data": M{"items": []any{}, "total": 10, "page": 1, "pages": 1, "pageSize": 20}})
	})
	w := h.req("GET", "/api/admin/manage/videos", nil, "")
	// Empty results with a nonzero total are valid for a concurrent deletion, but not bogus page counts.
	if w.Code != 200 || calls.Load() != 1 {
		t.Fatal("valid empty page rejected")
	}
}
func TestManagementConfirmedWriteAndSelectiveSessions(t *testing.T) {
	h, _ := managementFixture(t, managementFixtureResponse)
	base := managementBase(h.a.readState().CMSBase)
	same := &memberSession{UserID: 7, Base: base}
	other := &memberSession{UserID: 8, Base: base}
	remote := &memberSession{UserID: 7, Base: "https://other.test"}
	h.a.members["target"] = same
	h.a.members["other"] = other
	h.a.members["remote"] = remote
	w := h.req("POST", "/api/admin/manage/users/action", M{"source": base, "action": "edit", "ids": []any{7}, "fields": M{"nickname": "新昵称"}}, "")
	if w.Code != 200 || len(h.a.members) != 3 || same.ProfileVersion.Load() != 1 || other.ProfileVersion.Load() != 1 || remote.ProfileVersion.Load() != 0 {
		t.Fatal("ordinary edit revoked sessions or failed to invalidate profiles")
	}
	w = h.req("POST", "/api/admin/manage/users/action", M{"source": base, "action": "status", "ids": []any{7}, "value": 0}, "")
	if w.Code != 200 || h.a.members["target"] != nil || h.a.members["other"] == nil || h.a.members["remote"] == nil {
		t.Fatal("disable must revoke only matching source and user ID")
	}
	h.a.searchIndexes = map[string]*searchIndex{"mobile": {Source: "old"}}
	h.a.searchJobs = map[string]*searchJob{"mobile": {Phase: "running"}}
	audit := filepath.Join(h.a.config.DataDir, "audit.jsonl")
	if err := os.Remove(audit); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(audit, 0700); err != nil {
		t.Fatal(err)
	}
	w = h.req("POST", "/api/admin/manage/videos/action", M{"source": base, "action": "status", "ids": []any{1}, "value": 1}, "")
	out := payload(t, w)
	if w.Code != 200 || out["ok"] != true || out["auditRecorded"] != false || len(arr(out["warnings"])) == 0 {
		t.Fatal("confirmed write was lost after audit failure")
	}
	if len(h.a.searchIndexes) != 0 || h.a.searchJobs["mobile"].Phase != "failed" {
		t.Fatal("video changes left stale search work active")
	}
}
func managementLocalFixture(t *testing.T) (*harness, string) {
	t.Helper()
	cms := fixtureCMS(t)
	t.Cleanup(cms.Close)
	h := newHarness(t, cms.URL)
	boundary := t.TempDir()
	root := fakeCMSSite(t, boundary, "site", cms.URL)
	if err := os.MkdirAll(filepath.Join(root, "application", "index", "controller"), 0700); err != nil {
		t.Fatal(err)
	}
	body := []byte("<?php return ['type'=>'mysql','hostname'=>'localhost','database'=>'fixture','username'=>'fixture','password'=>'','prefix'=>'mac_'];")
	if err := os.WriteFile(filepath.Join(root, "application", "database.php"), body, 0600); err != nil {
		t.Fatal(err)
	}
	h.a.config.CMSRoots = []string{boundary}
	if err := h.a.change(context.Background(), "fixture.local", func(s *state) error { s.CMSLocalRoot = root; return nil }); err != nil {
		t.Fatal(err)
	}
	return h, root
}
func TestManagementInstallerPreflightAndRollback(t *testing.T) {
	t.Run("conflicting-second-file-does-not-create-first", func(t *testing.T) {
		h, root := managementLocalFixture(t)
		second := filepath.Join(root, "application", "extra", "hetun_operations.php")
		if err := os.WriteFile(second, []byte("existing-user-file"), 0600); err != nil {
			t.Fatal(err)
		}
		w := h.req("POST", "/api/admin/manage/install", M{"source": managementBase(h.a.readState().CMSBase)}, "")
		if w.Code != 409 {
			t.Fatalf("conflict status %d", w.Code)
		}
		if _, err := os.Stat(filepath.Join(root, "application", "index", "controller", "Hetunops.php")); !os.IsNotExist(err) {
			t.Fatal("preflight wrote first target")
		}
		body, _ := os.ReadFile(second)
		if string(body) != "existing-user-file" {
			t.Fatal("existing CMS config overwritten")
		}
	})
	for _, preserve := range []bool{false, true} {
		t.Run(strconv.FormatBool(preserve), func(t *testing.T) {
			h, root := managementLocalFixture(t)
			first := filepath.Join(root, "application", "index", "controller", "Hetunops.php")
			if preserve {
				if err := os.WriteFile(first, managementAdapter, 0600); err != nil {
					t.Fatal(err)
				}
			}
			h.a.store = &applyFailStore{h.a.store}
			w := h.req("POST", "/api/admin/manage/install", M{"source": managementBase(h.a.readState().CMSBase)}, "")
			if w.Code == 200 {
				t.Fatal("installation reported success after persistence failure")
			}
			if _, err := os.Stat(filepath.Join(root, "application", "extra", "hetun_operations.php")); !os.IsNotExist(err) {
				t.Fatal("uncommitted config left behind")
			}
			b, err := os.ReadFile(first)
			if preserve {
				if err != nil || string(b) != string(managementAdapter) {
					t.Fatal("rollback removed pre-existing adapter")
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("uncommitted controller left behind")
			}
			if h.a.readState().ManagementConnection != nil {
				t.Fatal("failed installation changed live state")
			}
		})
	}
	t.Run("idempotent-success", func(t *testing.T) {
		h, _ := managementLocalFixture(t)
		for i := 0; i < 2; i++ {
			w := h.req("POST", "/api/admin/manage/install", M{"source": managementBase(h.a.readState().CMSBase)}, "")
			if w.Code != 200 {
				t.Fatalf("install status %d", w.Code)
			}
		}
		if h.a.readState().ManagementConnection == nil {
			t.Fatal("installed connection missing")
		}
	})
}
