package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceCheckRealEndpointsAndContentFailure(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	server := httptest.NewServer(h.a)
	defer server.Close()
	h.a.config.PublicOrigin = server.URL
	h.origin = server.URL
	w := h.req("POST", "/api/admin/service-addresses/check", M{"address": server.URL}, "")
	v := payload(t, w)
	if w.Code != 200 || v["ok"] != true || v["connectionOK"] != true || v["contentOK"] != true || len(arr(v["checks"])) != 10 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if err := h.a.change(context.Background(), "test.cms.remove", func(v *state) error { v.CMSBase = ""; return nil }); err != nil {
		t.Fatal(err)
	}
	w = h.req("POST", "/api/admin/service-addresses/check", M{"address": server.URL}, "")
	v = payload(t, w)
	if v["connectionOK"] != true || v["contentOK"] != false || v["ok"] != false {
		t.Fatal(w.Body)
	}
	for _, item := range arr(v["checks"]) {
		row := obj(item)
		if row["content"] == true && (row["ok"] != false || row["status"].(float64) < 400) {
			t.Fatal(row)
		}
	}
	h.cookie = ""
	if w := h.req("POST", "/api/admin/service-addresses/check", M{"address": server.URL}, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestServiceProbeIdentityAndNetworkBoundaries(t *testing.T) {
	h := newHarness(t, "")
	for _, address := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com/api", "https://example.com/?key=x"} {
		if _, _, err := h.a.probeAddress(address); err == nil {
			t.Fatal(address)
		}
	}
	if _, local, err := h.a.probeAddress(h.origin); err != nil || !local {
		t.Fatalf("preview %v", err)
	}
	// A different loopback service must not receive the privileged local-network bypass.
	if _, local, err := h.a.probeAddress("https://127.0.0.1:9999"); err != nil || local {
		t.Fatalf("bypass %v %v", local, err)
	}
	other := newHarness(t, "")
	w := other.req("GET", "/api/service-directory", nil, "")
	if h.a.validProbeDirectory(payload(t, w), other.origin) {
		t.Fatal("accepted another server identity")
	}
	for _, tc := range []struct {
		status int
		body   string
	}{{502, `{}`}, {200, `<html>login</html>`}, {200, `null`}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		_, status, err := probeJSON(context.Background(), cmsClient(true), server.URL)
		server.Close()
		if err == nil || status != tc.status {
			t.Fatal(tc, status, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("private service was accessed") }))
	defer server.Close()
	if _, _, err := probeJSON(context.Background(), cmsClient(false), server.URL); err == nil {
		t.Fatal("private request accepted")
	}
}
func TestSystemConfigSectionsAndAdminRestart(t *testing.T) {
	h := systemHarness(t)
	active := h.a.config.AdminPath
	w := h.req("PUT", "/api/admin/system/config", M{"adminPath": "/abc123"}, "")
	v := payload(t, w)
	if w.Code != 200 || v["restartRequired"] != true || obj(v["active"])["adminPath"] != shortAdminPath(active) || obj(v["saved"])["adminPath"] != "/abc123" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	saved, err := loadConfig(h.a.configPath)
	if err != nil || saved.Listen != h.a.config.Listen || saved.AdminPath != "/abc123" || h.a.config.AdminPath != active {
		t.Fatalf("%v %+v", err, saved)
	}
	w = h.req("PUT", "/api/admin/system/config", M{"listen": "127.0.0.1:9991"}, "")
	if w.Code != 200 || obj(payload(t, w)["saved"])["adminPath"] != "/abc123" {
		t.Fatal(w.Body)
	}
	for _, value := range []any{"", "/", "abc123", "/ABC123", "/abc1234", "/../abc", nil, 42} {
		if w := h.req("PUT", "/api/admin/system/config", M{"adminPath": value}, ""); w.Code != 400 {
			t.Fatal(value, w.Code)
		}
	}
	if w := h.req("PUT", "/api/admin/system/config", M{}, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = h.req("GET", "/api/admin/system/config", nil, "")
	v = payload(t, w)
	if len(str(obj(v["identity"])["fingerprint"])) != 64 || !strings.HasSuffix(str(v["auditPath"]), "audit.jsonl") || strings.Contains(w.Body.String(), "PRIVATE KEY") {
		t.Fatal(w.Body)
	}
}
