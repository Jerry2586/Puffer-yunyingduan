package main

import (
	"context"
	"testing"
)

func menuTargetDraft() M {
	d := pageDraft()
	d["serviceMenuItems"] = []any{
		M{"id": "legacy", "label": "旧收藏", "page": "library", "enabled": true, "categoryId": ""},
		M{"id": "event", "label": "活动", "targetType": "page", "page": "home", "pageId": "summer", "enabled": true},
		M{"id": "external", "label": "合作活动", "targetType": "external", "page": "home", "url": "https://example.com/event?a=1#offer", "enabled": true, "textColor": "#123456", "iconKey": "gift", "display": "both"},
	}
	return d
}

func TestMenuTargetsSavePublishAndRestart(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	w := h.req("PUT", "/api/admin/draft", M{"version": 1, "theme": menuTargetDraft(), "template": "runtime-mobile"}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = h.req("POST", "/api/admin/publish", M{"version": 2, "template": "runtime-mobile"}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	restarted, err := newApp(context.Background(), h.a.config, h.a.store)
	if err != nil {
		t.Fatal(err)
	}
	state := restarted.readState()
	response, err := bootstrap(state, "mobile", "web", "", h.origin)
	if err != nil {
		t.Fatal(err)
	}
	config := obj(response["config"])
	rows := arr(config["serviceMenuItems"])
	if len(rows) != 3 {
		t.Fatalf("menu lost on restart: %v", rows)
	}
	legacy, page, external := obj(rows[0]), obj(rows[1]), obj(rows[2])
	if legacy["targetType"] != "system" || legacy["page"] != "library" {
		t.Fatal("legacy destination changed", legacy)
	}
	if page["targetType"] != "page" || page["pageId"] != "summer" {
		t.Fatal("page destination changed", page)
	}
	if external["targetType"] != "external" || external["url"] != "https://example.com/event?a=1#offer" || external["textColor"] != "#123456" || external["iconKey"] != "gift" {
		t.Fatal("external destination changed", external)
	}
	if _, exists := external["pageId"]; exists {
		t.Fatal("external menu unexpectedly references a page")
	}
}

func TestMenuTargetValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(M)
	}{
		{"unknown type", func(m M) { m["targetType"] = "unknown" }},
		{"nonstring type", func(m M) { m["targetType"] = true }},
		{"empty address", func(m M) { m["url"] = "" }},
		{"missing address", func(m M) { delete(m, "url") }},
		{"nonstring address", func(m M) { m["url"] = 123 }},
		{"script", func(m M) { m["url"] = "javascript:alert(1)" }},
		{"relative", func(m M) { m["url"] = "//example.com/activity" }},
		{"missing host", func(m M) { m["url"] = "https://" }},
		{"invalid port", func(m M) { m["url"] = "https://example.com:999999/" }},
		{"credentials", func(m M) { m["url"] = "https://user:pass@example.com/" }},
		{"space", func(m M) { m["url"] = "https://example.com/a b" }},
		{"backslash", func(m M) { m["url"] = "https://example.com/a\\b" }},
		{"conflicting page", func(m M) { m["pageId"] = "summer" }},
		{"conflicting CMS", func(m M) { m["categoryId"] = "1" }},
		{"system with URL", func(m M) { m["targetType"] = "system" }},
		{"unselected page", func(m M) { delete(m, "url"); m["targetType"] = "page" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := menuTargetDraft()
			tc.change(obj(arr(d["serviceMenuItems"])[2]))
			if _, err := operationConfig(d); err == nil {
				t.Fatal("invalid menu accepted")
			}
		})
	}
	// Menu destinations cannot add external navigation to primary/bottom entries.
	d := menuTargetDraft()
	n := obj(arr(d["categories"])[0])
	n["url"] = "https://example.com"
	n["targetType"] = "external"
	clean, err := operationConfig(d)
	if err != nil {
		t.Fatal(err)
	}
	n = obj(arr(clean["categories"])[0])
	if _, ok := n["url"]; ok {
		t.Fatal("external URL crossed into primary category")
	}
	if _, ok := n["targetType"]; ok {
		t.Fatal("menu target type crossed into primary category")
	}
}
