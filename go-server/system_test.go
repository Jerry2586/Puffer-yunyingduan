package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func systemHarness(t *testing.T) *harness {
	h := newHarness(t, "")
	if err := h.a.change(context.Background(), "setup.test", func(v *state) error { v.Setup = M{"completedAt": now()}; return nil }); err != nil {
		t.Fatal(err)
	}
	h.a.config.Listen = "127.0.0.1:8787"
	h.a.config.Storage = "file"
	h.a.config.MySQLDSN = "SECRET_NEVER_RETURN"
	h.a.configPath = filepath.Join(t.TempDir(), "config.json")
	b, _ := json.Marshal(h.a.config)
	if err := atomicFile(h.a.configPath, b); err != nil {
		t.Fatal(err)
	}
	return h
}
func TestSystemAuthenticationAndTruthfulOverview(t *testing.T) {
	h := systemHarness(t)
	cookie := h.cookie
	h.cookie = ""
	for _, route := range []string{"overview", "config", "logs"} {
		if w := h.req("GET", "/api/admin/system/"+route, nil, ""); w.Code != 401 {
			t.Fatalf("%s auth %d", route, w.Code)
		}
	}
	h.cookie = cookie
	w := h.req("GET", "/api/admin/system/overview", nil, "")
	v := payload(t, w)
	if w.Code != 200 || v["version"] != buildVersion || obj(v["redis"])["configured"] != false || v["cmsBase"] != "" {
		t.Fatalf("overview %d %s", w.Code, w.Body)
	}
	db := v["database"].(map[string]any)
	if db["mode"] != "file" || db["ok"] != true || db["pool"] != nil {
		t.Fatal(db)
	}
	if v["heapBytes"].(float64) <= 0 || v["cpuCores"].(float64) < 1 {
		t.Fatal(v)
	}
	if w := h.req("POST", "/api/admin/system/overview", M{}, ""); w.Code != 405 {
		t.Fatal(w.Code)
	}
}
func TestSystemConfigPersistenceAndRedaction(t *testing.T) {
	h := systemHarness(t)
	input := M{"listen": "127.0.0.1:9876", "publicOrigin": h.origin, "clientOrigins": []any{"https://client.example"}}
	w := h.req("PUT", "/api/admin/system/config", input, "")
	v := payload(t, w)
	if w.Code != 200 || v["restartRequired"] != true || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatalf("save %d %s", w.Code, w.Body)
	}
	saved, err := loadConfig(h.a.configPath)
	if err != nil || saved.Listen != "127.0.0.1:9876" || saved.MySQLDSN != "SECRET_NEVER_RETURN" || h.a.config.Listen != "127.0.0.1:8787" {
		t.Fatalf("disk/active mismatch %v", err)
	}
	for _, port := range []string{"abc", "0", "65536"} {
		input["listen"] = "127.0.0.1:" + port
		if w := h.req("PUT", "/api/admin/system/config", input, ""); w.Code != 400 {
			t.Fatal(port, w.Code)
		}
	}
	input["listen"] = "127.0.0.1:9876"
	input["mysqlDSN"] = "overwrite"
	if w := h.req("PUT", "/api/admin/system/config", input, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	saved, err = loadConfig(h.a.configPath)
	if err != nil || saved.MySQLDSN != "SECRET_NEVER_RETURN" {
		t.Fatal("secret changed")
	}
	w = h.req("GET", "/api/admin/system/config", nil, "")
	if strings.Contains(w.Body.String(), "SECRET") {
		t.Fatal("secret leaked")
	}
}
func TestSystemLogsBoundedFilteredAndSanitized(t *testing.T) {
	h := systemHarness(t)
	f, err := os.Create(filepath.Join(h.a.config.DataDir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 700; i++ {
		_ = json.NewEncoder(f).Encode(M{"at": now(), "event": "system.config.saved", "revision": i, "password": "SECRET"})
	}
	_ = f.Close()
	w := h.req("GET", "/api/admin/system/logs?filter=system", nil, "")
	v := payload(t, w)
	if w.Code != 200 || v["truncated"] != true || len(v["items"].([]any)) != 500 || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatalf("logs %d", w.Code)
	}
	rows := v["items"].([]any)
	if rows[0].(map[string]any)["revision"] != float64(699) {
		t.Fatal("newest first")
	}
	w = h.req("GET", "/api/admin/system/logs?filter=account", nil, "")
	if len(payload(t, w)["items"].([]any)) != 0 {
		t.Fatal("filter")
	}
	if w := h.req("GET", "/api/admin/system/logs?filter=../../config.json", nil, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestSystemOverviewProbesCMSAndTracksRealRequests(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := systemHarness(t)
	if err := h.a.change(context.Background(), "cms.test", func(v *state) error { v.CMSBase = cms.URL; return nil }); err != nil {
		t.Fatal(err)
	}
	v := payload(t, h.req("GET", "/api/admin/system/overview", nil, ""))
	report := v["cms"].(map[string]any)
	if report["ok"] != true || len(report["checks"].([]any)) != 2 {
		t.Fatal(report)
	}
	before := v["requests"].(map[string]any)
	h.req("GET", "/api/health", nil, "")
	h.req("GET", "/api/does-not-exist", nil, "")
	v = payload(t, h.req("GET", "/api/admin/system/overview", nil, ""))
	after := v["requests"].(map[string]any)
	if after["total"].(float64) != before["total"].(float64)+2 || after["failed"].(float64) != before["failed"].(float64)+1 || after["active"] != float64(0) {
		t.Fatal(before, after)
	}
	if after["averageMs"].(float64) < 0 || v["pid"].(float64) <= 0 {
		t.Fatal(v)
	}
	// Stop the real fixture: a saved URL must not remain marked connected.
	cms.Close()
	v = payload(t, h.req("GET", "/api/admin/system/overview", nil, ""))
	report = v["cms"].(map[string]any)
	if report["ok"] != false || report["status"] != "error" {
		t.Fatal(report)
	}
}
func TestSystemMemoryUsesOperatingSystem(t *testing.T) {
	v := systemMemory()
	if v["available"] != true {
		t.Fatal("supported test host did not report memory", v)
	}
	total := v["totalBytes"].(uint64)
	used := v["usedBytes"].(uint64)
	if total == 0 || used > total || v["rssBytes"].(uint64) == 0 {
		t.Fatal(v)
	}
}
