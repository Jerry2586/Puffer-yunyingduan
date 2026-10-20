package main

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientCacheIsolationInvalidationAndAuth(t *testing.T) {
	var calls atomic.Int32
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		jsonResponse(w, 200, M{"code": 1, "class": []any{M{"type_id": 1, "type_name": "Movies"}}, "list": []any{M{"vod_id": 1, "vod_name": "Film"}}})
	}))
	defer cms.Close()
	h := newHarness(t, "")
	h.a.state.CMSBase = cms.URL
	redisServer := miniredis.RunT(t)
	cfg := &redisConfig{Address: redisServer.Addr(), TTLSeconds: 60}
	if err := cfg.normalize(); err != nil {
		t.Fatal(err)
	}
	h.a.cache = newContentCache(cfg, h.a.config.DataDir)
	defer h.a.cache.close()
	other := newContentCache(cfg, t.TempDir())
	defer other.close()
	redisServer.Set("other-app", "keep")
	redisServer.Set(other.prefix+"catalog:v2:mobile:0:other", "keep")
	for _, p := range platforms {
		for n := 0; n < 2; n++ {
			w := h.req("GET", "/api/catalog?platform="+p, nil, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
		}
		status := payload(t, h.req("GET", "/api/admin/client-cache?platform="+p, nil, ""))
		if status["available"] != true || status["entries"] != float64(1) {
			t.Fatal(status)
		}
	}
	if calls.Load() != 4 {
		t.Fatal("platforms share data cache", calls.Load())
	}
	w := h.req("POST", "/api/admin/client-cache/clear?platform=tv", M{"kind": "catalog"}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	result := payload(t, w)
	if result["deleted"] != float64(1) || result["invalidated"] != true || result["auditRecorded"] != true {
		t.Fatal(result)
	}
	for _, p := range platforms {
		w = h.req("GET", "/api/catalog?platform="+p, nil, "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	if calls.Load() != 5 {
		t.Fatal("clear escaped platform", calls.Load())
	}
	if value, _ := redisServer.Get("other-app"); value != "keep" {
		t.Fatal("other application touched")
	}
	if !redisServer.Exists(other.prefix + "catalog:v2:mobile:0:other") {
		t.Fatal("other installation touched")
	}
	if w = h.req("GET", "/api/admin/client-cache", nil, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = h.req("GET", "/api/admin/client-cache?platform=invalid", nil, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = h.req("POST", "/api/admin/client-cache/clear?platform=mobile", M{"kind": "unsupported"}, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = h.req("GET", "/api/admin/client-cache/clear?platform=mobile", nil, ""); w.Code != 405 {
		t.Fatal(w.Code)
	}
	h.cookie = ""
	if w = h.req("POST", "/api/admin/client-cache/clear?platform=mobile", M{"kind": "catalog"}, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestClientCacheMissingRedisAndFailure(t *testing.T) {
	h := newHarness(t, "")
	w := h.req("GET", "/api/admin/client-cache?platform=tablet", nil, "")
	if w.Code != 200 || payload(t, w)["configured"] != false {
		t.Fatal(w.Code, w.Body)
	}
	w = h.req("POST", "/api/admin/client-cache/clear?platform=tablet", M{"kind": "catalog"}, "")
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body)
	}
	redisServer := miniredis.RunT(t)
	cfg := &redisConfig{Address: redisServer.Addr()}
	_ = cfg.normalize()
	h.a.cache = newContentCache(cfg, h.a.config.DataDir)
	defer h.a.cache.close()
	redisServer.RequireAuth("unavailable-secret")
	w = h.req("GET", "/api/admin/client-cache?platform=tablet", nil, "")
	if w.Code != 200 || payload(t, w)["available"] != false {
		t.Fatal(w.Code, w.Body)
	}
	w = h.req("POST", "/api/admin/client-cache/clear?platform=tablet", M{"kind": "catalog"}, "")
	if w.Code != 503 || strings.Contains(w.Body.String(), "unavailable-secret") {
		t.Fatal(w.Code, w.Body)
	}
}

func TestClientCacheLateWriteCannotResurrect(t *testing.T) {
	h := newHarness(t, "")
	redisServer := miniredis.RunT(t)
	cfg := &redisConfig{Address: redisServer.Addr()}
	_ = cfg.normalize()
	h.a.cache = newContentCache(cfg, h.a.config.DataDir)
	defer h.a.cache.close()
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		jsonResponse(w, 200, M{"code": 1, "list": []any{M{"vod_id": 1}}})
	}))
	defer cms.Close()
	done := make(chan error, 1)
	go func() {
		_, err := h.a.cachedCatalog(context.WithValue(context.Background(), cachePlatformKey{}, "mobile"), cms.URL, url.Values{"ac": {"detail"}})
		done <- err
	}()
	<-started
	w := h.req("POST", "/api/admin/client-cache/clear?platform=mobile", M{"kind": "catalog"}, "")
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if _, err := h.a.cachedCatalog(context.Background(), cms.URL, url.Values{"ac": {"detail"}}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("late response reused after clear", calls.Load())
	}
}

func TestClientCacheRejectsConflictingPlatformHeader(t *testing.T) {
	h := newHarness(t, "")
	for _, path := range []string{"/api/admin/client-cache?platform=tv", "/api/catalog?platform=tv", "/api/catalog/categories?platform=tv"} {
		r := httptest.NewRequest("GET", h.origin+path, nil)
		r.Header.Set("X-Hetun-Platform", "mobile")
		r.AddCookie(&http.Cookie{Name: "hetun_session", Value: h.cookie})
		w := httptest.NewRecorder()
		h.a.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestClientCacheStatusTotalsAllKindsAndIsolation(t *testing.T) {
	h := newHarness(t, "")
	redisServer := miniredis.RunT(t)
	cfg := &redisConfig{Address: redisServer.Addr(), TTLSeconds: 60}
	if err := cfg.normalize(); err != nil {
		t.Fatal(err)
	}
	h.a.cache = newContentCache(cfg, h.a.config.DataDir)
	defer h.a.cache.close()
	load := func() (M, error) { return M{"ok": true}, nil }
	for _, kind := range []string{"config", "users", "catalog"} {
		if _, err := h.a.cacheObject(context.Background(), "mobile", kind, "mobile-"+kind, load); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.a.cacheObject(context.Background(), "tv", "catalog", "tv-catalog", load); err != nil {
		t.Fatal(err)
	}
	status := payload(t, h.req("GET", "/api/admin/client-cache?platform=mobile", nil, ""))
	if status["available"] != true || status["entries"] != float64(3) || status["countComplete"] != true {
		t.Fatal(status)
	}
	kinds := obj(status["kinds"])
	for _, kind := range []string{"config", "users", "catalog"} {
		entry := obj(kinds[kind])
		if entry["entries"] != float64(1) || entry["countComplete"] != true {
			t.Fatalf("%s: %v", kind, entry)
		}
	}
	tv := payload(t, h.req("GET", "/api/admin/client-cache?platform=tv", nil, ""))
	if tv["entries"] != float64(1) {
		t.Fatal("platform cache count crossed scope", tv)
	}
}
