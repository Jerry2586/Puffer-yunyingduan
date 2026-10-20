package main

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func enableObjectCache(t *testing.T, h *harness) *miniredis.Miniredis {
	t.Helper()
	r := miniredis.RunT(t)
	cfg := &redisConfig{Address: r.Addr(), TTLSeconds: 60}
	if e := cfg.normalize(); e != nil {
		t.Fatal(e)
	}
	h.a.cache = newContentCache(cfg, h.a.config.DataDir)
	t.Cleanup(func() { h.a.cache.close() })
	return r
}
func TestObjectCacheScopesAndSourceChanges(t *testing.T) {
	h := newHarness(t, "")
	r := enableObjectCache(t, h)
	calls := 0
	load := func() (M, error) { calls++; return M{"value": calls}, nil }
	for _, p := range platforms {
		for _, kind := range []string{"config", "users", "catalog"} {
			for n := 0; n < 2; n++ {
				if _, e := h.a.cacheObject(context.Background(), p, kind, "private-token", load); e != nil {
					t.Fatal(e)
				}
			}
		}
	}
	if calls != 12 {
		t.Fatal("cache hit failed", calls)
	}
	for _, key := range r.Keys() {
		if strings.Contains(key, "private-token") {
			t.Fatal("plaintext token leaked")
		}
	}
	w := h.req("POST", "/api/admin/client-cache/clear?platform=tv", M{"kind": "users"}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	for _, p := range platforms {
		for _, kind := range []string{"config", "users", "catalog"} {
			h.a.cacheObject(context.Background(), p, kind, "private-token", load)
		}
	}
	if calls != 13 {
		t.Fatal("clear crossed scope", calls)
	}
	h.a.cacheObject(context.Background(), "mobile", "config", "changed-publication", load)
	if calls != 14 {
		t.Fatal("source revision ignored")
	}
	r.FastForward(31 * time.Second)
	h.a.cacheObject(context.Background(), "mobile", "users", "private-token", load)
	if calls != 15 {
		t.Fatal("profile TTL exceeded")
	}
}
func TestProfileCacheNeverBypassesAuthentication(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	enableObjectCache(t, h)
	login := h.req("POST", "/api/member/login", M{"username": "alice", "password": "correct"}, "")
	token := str(payload(t, login)["token"])
	if token == "" {
		t.Fatal(login.Body)
	}
	for n := 0; n < 2; n++ {
		w := h.req("GET", "/api/member/profile?platform=mobile", nil, token)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	if w := h.req("POST", "/api/admin/client-cache/clear?platform=mobile", M{"kind": "users"}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := h.req("GET", "/api/member/profile?platform=mobile", nil, token); w.Code != 200 {
		t.Fatal("clear revoked login", w.Body)
	}
	h.req("POST", "/api/member/logout", M{}, token)
	if w := h.req("GET", "/api/member/profile?platform=mobile", nil, token); w.Code != 401 {
		t.Fatal("cache bypassed logout", w.Code, w.Body)
	}
}
func TestSearchIndexRealQueriesAndFailedRebuild(t *testing.T) {
	var fail atomic.Bool
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			jsonResponse(w, 200, M{"code": 1, "pagecount": 1, "total": 2, "list": []any{M{"vod_id": 1, "vod_name": "Alpha"}}})
			return
		}
		jsonResponse(w, 200, M{"code": 1, "pagecount": 1, "total": 1, "list": []any{M{"vod_id": 1, "vod_name": "Alpha", "vod_play_url": "secret-playback"}}})
	}))
	defer cms.Close()
	h := newHarness(t, "")
	h.a.state.CMSBase = cms.URL
	build := func() {
		t.Helper()
		w := h.req("POST", "/api/admin/client-cache/search/rebuild?platform=tv", M{}, "")
		if w.Code != 202 {
			t.Fatal(w.Code, w.Body)
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if h.a.searchStatus("tv")["phase"] != "running" {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("build timeout")
	}
	build()
	if h.a.searchStatus("tv")["ready"] != true {
		t.Fatal(h.a.searchStatus("tv"))
	}
	w := h.req("GET", "/api/catalog?platform=tv&keyword=Alpha", nil, "")
	m := payload(t, w)
	if w.Code != 200 || m["searchSource"] != "local-index" || strings.Contains(w.Body.String(), "secret-playback") {
		t.Fatal(w.Code, w.Body)
	}
	if h.a.searchStatus("mobile")["ready"] == true {
		t.Fatal("platform index shared")
	}
	fail.Store(true)
	build()
	s := h.a.searchStatus("tv")
	if s["phase"] != "failed" || s["ready"] != true {
		t.Fatal("partial index replaced valid snapshot", s)
	}
	h.a.searchMu.Lock()
	h.a.searchIndexes["tv"].BuiltAt = time.Now().Add(-searchIndexLifetime)
	h.a.searchMu.Unlock()
	w = h.req("GET", "/api/catalog?platform=tv&keyword=Alpha", nil, "")
	if payload(t, w)["searchSource"] == "local-index" {
		t.Fatal("expired index used")
	}
	h.a.state.CMSBase = ""
	if w = h.req("POST", "/api/admin/client-cache/search/rebuild?platform=tv", M{}, ""); w.Code != 409 {
		t.Fatal(w.Code, w.Body)
	}
	h.cookie = ""
	if w = h.req("POST", "/api/admin/client-cache/search/rebuild?platform=tv", M{}, ""); w.Code != 401 {
		t.Fatal(w.Code, w.Body)
	}
}
