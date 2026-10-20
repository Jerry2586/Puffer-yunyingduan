package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// This opt-in test uses a real Redis process, HTTP handlers and a controlled CMS.
// It never flushes the database: all keys belong to a temporary install namespace.
func TestRealRedisClientCaches(t *testing.T) {
	addr := os.Getenv("HETUN_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("HETUN_TEST_REDIS_ADDR not set")
	}
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	cfg := &redisConfig{Address: addr, Password: os.Getenv("HETUN_TEST_REDIS_PASSWORD"), TTLSeconds: 60}
	if err := cfg.normalize(); err != nil {
		t.Fatal(err)
	}
	c := newContentCache(cfg, h.a.config.DataDir)
	h.a.cache = c
	ctx := context.Background()
	marker := c.prefix + "unrelated-sentinel"
	if err := c.client.Set(ctx, marker, "keep", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		keys, err := c.client.Keys(ctx, c.prefix+"*").Result()
		if err == nil && len(keys) > 0 {
			_ = c.client.Del(ctx, keys...).Err()
		}
		c.close()
	}()
	login := h.req("POST", "/api/v1/mobile/member/login", M{"username": "alice", "password": "correct"}, "")
	if login.Code != 200 {
		t.Fatal(login.Code, login.Body)
	}
	token := str(payload(t, login)["token"])
	if token == "" {
		t.Fatal("missing member token")
	}
	kinds := []string{"config", "users", "catalog"}
	routes := map[string]string{"config": "bootstrap", "users": "member/profile", "catalog": "catalog"}
	warm := func(p, kind string) {
		t.Helper()
		w := h.req("GET", "/api/v1/"+clientName(p)+"/"+routes[kind], nil, token)
		if w.Code != 200 {
			t.Fatal(p, kind, w.Code, w.Body)
		}
	}
	assertCounts := func(clearedP, clearedKind string) {
		t.Helper()
		for _, p := range platforms {
			w := h.req("GET", "/api/admin/clients/"+clientName(p)+"/cache", nil, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
			status := payload(t, w)
			wantTotal := float64(3)
			if p == clearedP {
				wantTotal--
			}
			if status["available"] != true || status["entries"] != wantTotal || status["countComplete"] != true {
				t.Fatal(p, status)
			}
			for _, kind := range kinds {
				want := float64(1)
				if p == clearedP && kind == clearedKind {
					want = 0
				}
				if obj(obj(status["kinds"])[kind])["entries"] != want {
					t.Fatal(p, kind, status)
				}
			}
		}
	}
	for _, p := range platforms {
		for _, kind := range kinds {
			warm(p, kind)
		}
	}
	hitsBefore := c.hits.Load()
	for _, p := range platforms {
		for _, kind := range kinds {
			warm(p, kind)
		}
	}
	if c.hits.Load()-hitsBefore != 12 {
		t.Fatal("second requests did not hit all twelve caches")
	}
	assertCounts("", "")
	for _, p := range platforms {
		for _, kind := range kinds {
			gen, err := c.generation(ctx, p, kind)
			if err != nil {
				t.Fatal(err)
			}
			keys, complete, err := c.kindKeys(ctx, p, kind, gen)
			if err != nil || !complete || len(keys) != 1 {
				t.Fatal(keys, complete, err)
			}
			if strings.Contains(keys[0], token) {
				t.Fatal("member token leaked in key")
			}
			ttl, err := c.client.TTL(ctx, keys[0]).Result()
			if err != nil || ttl <= 0 || ttl > time.Duration(c.kindTTL(kind))*time.Second {
				t.Fatal("invalid TTL", kind, ttl, err)
			}
			w := h.req("POST", "/api/admin/clients/"+clientName(p)+"/cache/clear", M{"kind": kind}, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
			result := payload(t, w)
			if result["deleted"] != float64(1) || result["invalidated"] != true || result["cleanupComplete"] != true || result["auditRecorded"] != true {
				t.Fatal(result)
			}
			assertCounts(p, kind)
			warm(p, kind)
			assertCounts("", "")
		}
	}
	if value, err := c.client.Get(ctx, marker).Result(); err != nil || value != "keep" {
		t.Fatal("unrelated key changed", value, err)
	}
	w := h.req("POST", "/api/v1/mobile/member/logout", M{}, token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	for _, p := range platforms {
		w = h.req("GET", "/api/v1/"+clientName(p)+"/member/profile", nil, token)
		if w.Code != 401 {
			t.Fatal("cached profile bypassed logout", p, w.Code, w.Body)
		}
	}
}
