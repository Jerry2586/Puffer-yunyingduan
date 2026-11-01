package main

import (
	"context"
	"testing"
)

func shortcutDraft() M {
	d := pageDraft()
	d["categories"] = []any{
		M{"id": "movie-nav", "label": "电影", "page": "home", "pageId": "movies", "categoryId": "1", "enabled": true},
		M{"id": "second-nav", "label": "精选电影", "page": "home", "pageId": "movies", "categoryId": "1", "enabled": true},
	}
	d["categoryShortcuts"] = []any{
		M{"entryId": "movie-nav", "enabled": true, "layout": "grid", "columns": float64(2), "background": "should-not-cross-boundary", "items": []any{
			M{"id": "schedule", "label": "动漫排期", "enabled": true, "display": "both", "iconKey": "schedule", "icon": "unowned.svg"},
			M{"id": "ranking", "label": "动漫榜", "enabled": true, "display": "text"},
		}},
		M{"entryId": "second-nav", "enabled": false, "layout": "scroll", "columns": float64(5), "items": []any{
			M{"id": "search", "label": "找电影", "enabled": true, "display": "icon", "iconKey": "search"},
		}},
	}
	return d
}

func TestShortcutLayoutsSavePublishAndRestart(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	w := h.req("PUT", "/api/admin/draft", M{"version": 1, "theme": shortcutDraft(), "template": "runtime-mobile"}, "")
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
	response, err := bootstrap(restarted.readState(), "mobile", "web", "", h.origin)
	if err != nil {
		t.Fatal(err)
	}
	groups := arr(obj(response["config"])["categoryShortcuts"])
	if len(groups) != 2 {
		t.Fatalf("lost shortcut groups: %v", groups)
	}
	first, second := obj(groups[0]), obj(groups[1])
	if first["entryId"] != "movie-nav" || first["enabled"] != true || first["layout"] != "grid" || first["columns"] != float64(2) {
		t.Fatal("first group changed", first)
	}
	if second["entryId"] != "second-nav" || second["enabled"] != false || second["layout"] != "scroll" || second["columns"] != float64(5) {
		t.Fatal("second group changed", second)
	}
	rows := arr(first["items"])
	if len(rows) != 2 || obj(rows[0])["id"] != "schedule" || obj(rows[1])["id"] != "ranking" || obj(rows[0])["iconKey"] != "schedule" {
		t.Fatal("entry order or slot lost", rows)
	}
	if len(arr(second["items"])) != 1 || obj(arr(second["items"])[0])["enabled"] != true {
		t.Fatal("disabled group lost entries", second)
	}
	if _, exists := first["background"]; exists {
		t.Fatal("theme-owned group field crossed boundary")
	}
	if _, exists := obj(rows[0])["icon"]; exists {
		t.Fatal("theme-owned artwork crossed boundary")
	}
}

func TestShortcutLayoutValidation(t *testing.T) {
	cases := []struct {
		name, key string
		value     any
	}{
		{"string switch", "enabled", "true"}, {"null switch", "enabled", nil},
		{"unknown layout", "layout", "rows"}, {"numeric layout", "layout", 2}, {"null layout", "layout", nil},
		{"string columns", "columns", "2"}, {"fractional columns", "columns", 2.5}, {"zero columns", "columns", 0},
		{"one column", "columns", 1}, {"too many columns", "columns", 7}, {"null columns", "columns", nil},
	}
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := shortcutDraft()
			group := obj(arr(d["categoryShortcuts"])[0])
			group[tc.key] = tc.value
			w := h.req("PUT", "/api/admin/draft", M{"version": 1, "theme": d, "template": "runtime-mobile"}, "")
			if w.Code != 400 {
				t.Fatalf("expected validation rejection: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestLegacyShortcutGroupsRemainCompatible(t *testing.T) {
	d := shortcutDraft()
	obj(arr(d["categories"])[1])["categoryId"] = "2"
	group := obj(arr(d["categoryShortcuts"])[0])
	delete(group, "entryId")
	group["categoryId"] = "1"
	delete(group, "enabled")
	delete(group, "layout")
	delete(group, "columns")
	cfg, err := operationConfig(d)
	if err != nil {
		t.Fatal(err)
	}
	old := obj(arr(cfg["categoryShortcuts"])[0])
	if old["entryId"] != "movie-nav" {
		t.Fatal("legacy owner not fixed", old)
	}
	for _, key := range []string{"categoryId", "enabled", "layout", "columns"} {
		if _, exists := old[key]; exists {
			t.Fatalf("unexpected legacy field %s", key)
		}
	}
	if obj(arr(old["items"])[0])["enabled"] != true {
		t.Fatal("legacy entry disabled")
	}
}

func TestLegacyShortcutOwnershipCannotRevive(t *testing.T) {
	for _, name := range []string{"ambiguous", "deleted", "explicit wins", "explicit wins first"} {
		t.Run(name, func(t *testing.T) {
			d := shortcutDraft()
			groups := arr(d["categoryShortcuts"])
			legacy := obj(groups[0])
			delete(legacy, "entryId")
			legacy["categoryId"] = "1"
			switch name {
			case "deleted":
				legacy["categoryId"] = "removed-source"
			case "explicit wins", "explicit wins first":
				obj(arr(d["categories"])[0])["categoryId"] = "2"
				if name == "explicit wins first" {
					d["categoryShortcuts"] = []any{groups[1], groups[0]}
				}
			}
			cfg, err := operationConfig(d)
			if err != nil {
				t.Fatal(err)
			}
			clean := arr(cfg["categoryShortcuts"])
			if len(clean) != 1 || obj(clean[0])["entryId"] != "second-nav" || obj(clean[0])["enabled"] != false {
				t.Fatal("ambiguous or superseded legacy group survived", clean)
			}
			// A later new entry binding the same CMS source must not inherit discarded navigation.
			cfg["categories"] = []any{M{"id": "replacement", "label": "新栏目", "page": "category", "categoryId": "1", "enabled": true}, arr(cfg["categories"])[1]}
			if _, err := operationConfig(cfg); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShortcutOwnerValidation(t *testing.T) {
	for _, tc := range []string{"unknown", "duplicate", "mixed", "missing", "number"} {
		t.Run(tc, func(t *testing.T) {
			d := shortcutDraft()
			groups := arr(d["categoryShortcuts"])
			g := obj(groups[0])
			switch tc {
			case "unknown":
				g["entryId"] = "absent"
			case "duplicate":
				obj(groups[1])["entryId"] = g["entryId"]
			case "mixed":
				g["categoryId"] = "1"
			case "missing":
				delete(g, "entryId")
			case "number":
				g["entryId"] = 1
			}
			if _, err := operationConfig(d); err == nil {
				t.Fatal("invalid owner accepted", tc)
			}
		})
	}
}
