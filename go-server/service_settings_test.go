package main

import (
	"context"
	"encoding/json"
	mysql "github.com/go-sql-driver/mysql"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestConnectionConfigAccessPersistenceAndLease(t *testing.T) {
	h := systemHarness(t)
	cookie := h.cookie
	h.cookie = ""
	for _, route := range []string{"/api/admin/system/connections", "/api/admin/system/connections/test"} {
		method := "GET"
		if strings.HasSuffix(route, "test") {
			method = "POST"
		}
		if w := h.req(method, route, nil, ""); w.Code != 401 {
			t.Fatal(w.Code, w.Body)
		}
	}
	h.cookie = cookie
	v := payload(t, h.req("GET", "/api/admin/system/connections", nil, ""))
	revision := v["revision"]
	if v["restartRequired"] != false {
		t.Fatal(v)
	}
	input := M{"section": "http", "revision": revision, "values": httpSettings{8, 80, 16, 32, 45}}
	w := h.req("PUT", "/api/admin/system/connections", input, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	v = payload(t, w)
	if v["restartRequired"] != true || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal(v)
	}
	c, e := loadConfig(h.a.configPath)
	if e != nil || c.HTTP.TimeoutSeconds != 8 || h.a.config.HTTP != nil {
		t.Fatal("save must persist without mutating running client", e)
	}
	if w = h.req("PUT", "/api/admin/system/connections", input, ""); w.Code != 409 {
		t.Fatal("stale write accepted", w.Code)
	}
	input["revision"] = v["revision"]
	input["values"] = M{"timeoutSeconds": 0}
	if w = h.req("PUT", "/api/admin/system/connections", input, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	input["values"] = effectiveHTTP(nil)
	input["unexpected"] = "x"
	if w = h.req("PUT", "/api/admin/system/connections", input, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = h.req("POST", "/api/admin/system/connections/test", M{"section": "database", "values": effectiveDatabase(nil)}, ""); w.Code != 409 {
		t.Fatal("file mode must not fake MySQL", w.Code)
	}
	if w = h.req("POST", "/api/admin/system/connections/test", M{"section": "http", "values": effectiveHTTP(nil)}, ""); w.Code != 409 {
		t.Fatal("unconfigured CMS must not pass", w.Code)
	}
}
func TestConnectionRedisSecretLifecycle(t *testing.T) {
	h := systemHarness(t)
	c := h.a.config
	c.Redis = &redisConfig{Address: "127.0.0.1:6379", Password: "SECRET_REDIS", PoolSize: 10, MaxIdle: 5, TTLSeconds: 60}
	raw, _ := json.Marshal(c)
	if e := atomicFile(h.a.configPath, raw); e != nil {
		t.Fatal(e)
	}
	get := func() M {
		w := h.req("GET", "/api/admin/system/connections", nil, "")
		if strings.Contains(w.Body.String(), "SECRET") {
			t.Fatal("secret exposed")
		}
		return payload(t, w)
	}
	values := M{"address": "127.0.0.1:6379", "db": 1, "poolSize": 10, "minIdle": 0, "maxIdle": 5, "ttlSeconds": 60, "password": "INJECTED"}
	for _, action := range []string{"keep", "replace", "clear"} {
		v := get()
		w := h.req("PUT", "/api/admin/system/connections", M{"section": "redis", "values": values, "revision": v["revision"], "passwordAction": action}, "")
		if w.Code != 200 || strings.Contains(w.Body.String(), "INJECTED") || strings.Contains(w.Body.String(), "SECRET") {
			t.Fatal(w.Code, w.Body)
		}
		disk, e := loadConfig(h.a.configPath)
		if e != nil {
			t.Fatal(e)
		}
		expected := map[string]string{"keep": "SECRET_REDIS", "replace": "INJECTED", "clear": ""}[action]
		if disk.Redis.Password != expected {
			t.Fatal("password action failed", action)
		}
	}
}
func TestConnectionHTTPRealProbeAndTransport(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := systemHarness(t)
	if e := h.a.change(context.Background(), "cms.test", func(v *state) error { v.CMSBase = cms.URL; return nil }); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(h.a.configPath)
	values := httpSettings{4, 40, 8, 16, 25}
	w := h.req("POST", "/api/admin/system/connections/test", M{"section": "http", "values": values}, "")
	if w.Code != 200 || payload(t, w)["ok"] != true {
		t.Fatal(w.Code, w.Body)
	}
	after, _ := os.ReadFile(h.a.configPath)
	if string(before) != string(after) {
		t.Fatal("test persisted candidate")
	}
	c := configuredCMSClient(config{HTTP: &values, AllowPrivateCMS: true})
	defer c.CloseIdleConnections()
	transport := c.Transport.(*http.Transport)
	if c.Timeout != 4*time.Second || transport.MaxIdleConns != 40 || transport.MaxIdleConnsPerHost != 8 || transport.MaxConnsPerHost != 16 || transport.IdleConnTimeout != 25*time.Second {
		t.Fatal("HTTP values not wired")
	}
	// The same probe must retain the production private-address boundary.
	h.a.config.AllowPrivateCMS = false
	raw, _ := json.Marshal(h.a.config)
	_ = atomicFile(h.a.configPath, raw)
	w = h.req("POST", "/api/admin/system/connections/test", M{"section": "http", "values": values}, "")
	if w.Code != 200 || payload(t, w)["ok"] != false {
		t.Fatal("private network guard bypassed", w.Code, w.Body)
	}
}
func TestDatabaseTuningDSNAndEffectiveDefaults(t *testing.T) {
	s := databaseSettings{3, 7, 9, 12, 4, 60, 20}
	dsn, e := databaseDSN("user:password@tcp(127.0.0.1:3306)/sample?parseTime=true", s)
	if e != nil {
		t.Fatal(e)
	}
	c, e := mysql.ParseDSN(dsn)
	if e != nil || c.Timeout != 3*time.Second || c.ReadTimeout != 7*time.Second || c.WriteTimeout != 9*time.Second || !c.ParseTime {
		t.Fatal("DSN settings not wired", e)
	}
	d := effectiveDatabase(nil)
	v := effectiveHTTP(nil)
	if connectionPending(config{}, config{Database: &d, HTTP: &v}) {
		t.Fatal("effective defaults should match")
	}
	s.MaxIdle = s.MaxOpen + 1
	if s.validate() == nil {
		t.Fatal("invalid pool accepted")
	}
}
func TestConnectionRedisRealProbe(t *testing.T) {
	addr := os.Getenv("HETUN_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("real Redis address not configured")
	}
	h := systemHarness(t)
	before, _ := os.ReadFile(h.a.configPath)
	values := redisConfig{Address: addr, DB: 0, PoolSize: 8, MaxIdle: 4, TTLSeconds: 30}
	w := h.req("POST", "/api/admin/system/connections/test", M{"section": "redis", "values": values, "passwordAction": "clear"}, "")
	if w.Code != 200 || payload(t, w)["ok"] != true {
		t.Fatal(w.Code, w.Body)
	}
	after, _ := os.ReadFile(h.a.configPath)
	if string(before) != string(after) {
		t.Fatal("probe persisted candidate")
	}
	values.Address = "127.0.0.1:1"
	w = h.req("POST", "/api/admin/system/connections/test", M{"section": "redis", "values": values, "passwordAction": "clear"}, "")
	if w.Code != 200 || payload(t, w)["ok"] != false {
		t.Fatal("closed port falsely succeeded", w.Code, w.Body)
	}
}
