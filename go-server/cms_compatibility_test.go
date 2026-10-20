package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCMSCategoryWithoutList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ac") == "list" {
			jsonResponse(w, 200, M{"code": 1, "class": []any{M{"type_id": 1, "type_name": "电影"}}})
			return
		}
		jsonResponse(w, 200, M{"code": 1, "total": 0, "list": []any{}})
	}))
	defer server.Close()
	h := newHarness(t, "")
	report := h.a.checkCMS(context.Background(), server.URL)
	if report["ok"] != true || report["status"] != "empty" {
		t.Fatal(report)
	}
	checks := report["checks"].([]any)
	if !strings.Contains(str(obj(checks[0])["url"]), "ac=list") {
		t.Fatal(checks)
	}
	root := t.TempDir()
	h.a.config.CMSRoots = []string{root}
	fakeCMSSite(t, root, "cms", server.URL)
	if result := h.a.discoverAndConnect(context.Background()); result["connected"] != true {
		t.Fatal(result)
	}
}

func TestCMSExplainsInvalidResponseAndDoesNotSave(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"closed", "不能仅凭此响应确定原因"},
		{"<html>login</html>", "网页而非 JSON"},
		{"broken", "未返回兼容 JSON"},
		{"null", "缺少 class 数组"},
		{`{"code":0,"msg":"接口禁止访问","class":[],"list":[]}`, "接口禁止访问"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			h := systemHarness(t)
			report := h.a.checkCMS(context.Background(), server.URL)
			if report["ok"] != false || !strings.Contains(str(report["message"]), tc.want) {
				t.Fatal(report)
			}
			w := h.req("PUT", "/api/admin/cms", M{"base": server.URL}, "")
			if w.Code != 400 || !strings.Contains(w.Body.String(), tc.want) || h.a.readState().CMSBase != "" {
				t.Fatal(w.Code, w.Body, h.a.readState().CMSBase)
			}
		})
	}
}

func TestUnconfiguredCMSContentIsExplicit(t *testing.T) {
	h := newHarness(t, "")
	for _, route := range []string{"/api/catalog/categories", "/api/catalog?page=1"} {
		w := h.req("GET", route, nil, "")
		result := payload(t, w)
		if w.Code != 409 || result["code"] != "CMS_NOT_CONFIGURED" {
			t.Fatalf("%s: %d %s", route, w.Code, w.Body.String())
		}
	}
	w := h.req("GET", "/api/service-directory", nil, "")
	if w.Code != 200 {
		t.Fatalf("directory must remain available: %d %s", w.Code, w.Body.String())
	}
}
