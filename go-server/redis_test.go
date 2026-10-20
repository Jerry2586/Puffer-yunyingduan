package main

import (
	"context"
	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRedisProbeIsolationAndRedaction(t *testing.T) {
	r := miniredis.RunT(t)
	r.RequireAuth("private-secret")
	r.Set("other-app", "keep")
	cfg := &redisConfig{Address: r.Addr(), Password: "private-secret"}
	if e := cfg.normalize(); e != nil {
		t.Fatal(e)
	}
	c := newContentCache(cfg, t.TempDir())
	defer c.close()
	if e := c.probe(context.Background()); e != nil {
		t.Fatal(e)
	}
	if keys := r.Keys(); len(keys) != 1 || keys[0] != "other-app" {
		t.Fatal(keys)
	}
	b, _ := json.Marshal(c.overview(context.Background()))
	if strings.Contains(string(b), cfg.Password) {
		t.Fatal("secret leaked")
	}
	cfg.Password = "wrong-secret"
	bad := newContentCache(cfg, t.TempDir())
	defer bad.close()
	if e := bad.probe(context.Background()); e == nil {
		t.Fatal("auth must fail")
	}
	b, _ = json.Marshal(bad.overview(context.Background()))
	if strings.Contains(string(b), cfg.Password) || !strings.Contains(string(b), `"ok":false`) {
		t.Fatal(string(b))
	}
}
func TestRedisCacheExpiryIsolationAndFallback(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		jsonResponse(w, 200, M{"code": 1, "list": []any{M{"vod_id": 1}}})
	}))
	defer srv.Close()
	r := miniredis.RunT(t)
	cfg := &redisConfig{Address: r.Addr(), TTLSeconds: 5}
	if e := cfg.normalize(); e != nil {
		t.Fatal(e)
	}
	c := newContentCache(cfg, t.TempDir())
	defer c.close()
	a := &app{cache: c, cms: cmsClient(true)}
	q := url.Values{"ac": {"detail"}}
	fetch := func(base string, p url.Values) {
		t.Helper()
		if _, e := a.cachedCatalog(context.Background(), base, p); e != nil {
			t.Fatal(e)
		}
	}
	fetch(srv.URL, q)
	fetch(srv.URL, q)
	if calls.Load() != 1 || c.hits.Load() != 1 {
		t.Fatal("cache miss", calls.Load())
	}
	fetch(srv.URL, url.Values{"pg": {"2"}})
	fetch(srv.URL+"/other", q)
	if calls.Load() != 3 {
		t.Fatal("key collision")
	}
	r.FastForward(6 * time.Second)
	fetch(srv.URL, q)
	if calls.Load() != 4 {
		t.Fatal("expiry ignored")
	}
	if _, e := a.catalogRaw(context.Background(), srv.URL, q); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 5 {
		t.Fatal("probe cached")
	}
	fail.Store(true)
	for n := 0; n < 2; n++ {
		if _, e := a.cachedCatalog(context.Background(), srv.URL, url.Values{"wd": {"error"}}); e == nil {
			t.Fatal("error ignored")
		}
	}
	if calls.Load() != 7 {
		t.Fatal("error cached")
	}
	fail.Store(false)
	r.Close()
	fetch(srv.URL, q)
	if calls.Load() != 8 || c.errors.Load() == 0 {
		t.Fatal("outage fallback failed")
	}
}
func TestInstallerRedisFailureBeforeDatabase(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	i, e := newInstaller(file, "127.0.0.1:8787")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	touched := false
	i.probe = func(context.Context, string) error { touched = true; return nil }
	i.connect = func(string) (store, error) { touched = true; return nil, nil }
	r := miniredis.RunT(t)
	r.RequireAuth("different")
	v := installInput{PublicOrigin: "http://127.0.0.1:8787", Host: "127.0.0.1", Port: 3306, Database: "isolated", Username: "owner", Admin: "owner", AdminPassword: "1", Redis: &redisConfig{Address: r.Addr(), Password: "no-leak"}}
	for _, action := range []string{"test", "complete"} {
		w := installRequest(i, "POST", i.route+"/"+action, v, v.PublicOrigin)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "redis_check_failed") || strings.Contains(w.Body.String(), "no-leak") {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	if touched {
		t.Fatal("database touched")
	}
	if _, e := os.Stat(file); !os.IsNotExist(e) {
		t.Fatal("config written")
	}
	v.Redis = nil
	if _, e := i.configuration(v); e == nil {
		t.Fatal("Redis required")
	}
}
func TestOutboundActiveCompletionAndReuse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	defer s.Close()
	base := http.DefaultTransport.(*http.Transport).Clone()
	defer base.CloseIdleConnections()
	m := &outboundMonitor{}
	client := &http.Client{Transport: &trackedTransport{base: base, metrics: m}}
	for n := 0; n < 2; n++ {
		res, e := client.Get(s.URL)
		if e != nil {
			t.Fatal(e)
		}
		if m.snapshot()["active"] != int64(1) {
			t.Fatal("missing active")
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		res.Body.Close()
	}
	v := m.snapshot()
	if v["active"] != int64(0) || v["total"] != int64(2) || v["reusePercent"].(float64) <= 0 {
		t.Fatal(v)
	}
}
func TestRealRedisOptional(t *testing.T) {
	addr := os.Getenv("HETUN_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("isolated real Redis address not supplied")
	}
	cfg := &redisConfig{Address: addr, Password: os.Getenv("HETUN_TEST_REDIS_PASSWORD")}
	if e := cfg.normalize(); e != nil {
		t.Fatal(e)
	}
	c := newContentCache(cfg, t.TempDir())
	defer c.close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := c.probe(ctx); e != nil {
		t.Fatal("real Redis probe failed")
	}
	v := c.overview(ctx)
	if v["ok"] != true || v["version"] == nil {
		t.Fatal("INFO unavailable")
	}
}
