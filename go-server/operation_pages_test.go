package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func pageDraft() M {
	d := clone(defaults)
	d["contentPages"] = []any{
		M{"id": "anime", "title": "动漫", "kind": "category", "categoryId": "1", "enabled": true, "carouselEnabled": true},
		M{"id": "movies", "title": "电影", "kind": "category", "categoryId": "2", "enabled": true, "carouselEnabled": false},
		M{"id": "summer", "title": "暑期活动", "kind": "campaign", "categoryId": "", "enabled": true, "carouselEnabled": true},
		M{"id": "empty", "title": "空白页", "kind": "blank", "categoryId": "", "enabled": false, "carouselEnabled": false},
	}
	d["navigation"] = []any{M{"id": "anime-nav", "label": "动漫", "page": "home", "pageId": "anime", "categoryId": "wrong", "enabled": true}}
	d["categories"] = []any{M{"id": "movie-nav", "label": "电影", "page": "home", "pageId": "movies", "categoryId": "cms-primary", "display": "icon", "iconKey": "legacy-icon", "enabled": true}}
	d["sections"] = []any{
		M{"id": "anime-list", "title": "动漫新番", "page": "home", "pageId": "anime", "source": "category", "categoryId": "1", "limit": float64(6), "orderBy": "latest", "enabled": true},
		M{"id": "movie-list", "title": "电影精选", "page": "home", "pageId": "movies", "source": "category", "categoryId": "2", "limit": float64(9), "orderBy": "score", "enabled": true},
		M{"id": "summer-list", "title": "活动片单", "page": "home", "pageId": "summer", "source": "latest", "categoryId": "", "limit": float64(3), "orderBy": "manual", "manualIds": "2,1", "enabled": true},
	}
	d["carousel"] = M{"slides": []any{M{"id": "summer-hero", "pageId": "summer", "page": "home", "vodId": "2", "image": "", "video": "", "title": "暑期精选"}}}
	d["navigationStyle"] = M{"fontSize": float64(17), "color": "#999999", "activeColor": "#FfAA00", "background": "BLOCKED_VISUAL"}
	d["colors"] = M{"background": "BLOCKED_VISUAL"}
	return d
}

func assertPageConfig(t *testing.T, cfg M) {
	t.Helper()
	if len(arr(cfg["contentPages"])) != 4 || len(arr(cfg["sections"])) != 3 {
		t.Fatalf("lost page configuration: %v", cfg)
	}
	nav := obj(arr(cfg["navigation"])[0])
	if nav["pageId"] != "anime" || nav["page"] != "category" || nav["categoryId"] != "1" {
		t.Fatalf("navigation reference not canonical: %v", nav)
	}
	category := obj(arr(cfg["categories"])[0])
	if category["pageId"] != "movies" || category["categoryId"] != "cms-primary" || category["display"] != "text" {
		t.Fatalf("primary category binding changed with its page: %v", category)
	}
	if _, exists := category["iconKey"]; exists {
		t.Fatal("primary category retained an icon slot")
	}
	if obj(arr(cfg["sections"])[1])["pageId"] != "movies" || obj(arr(cfg["sections"])[2])["manualIds"] != "2,1" {
		t.Fatal("page contents mixed")
	}
	if obj(arr(obj(cfg["carousel"])["slides"])[0])["pageId"] != "summer" {
		t.Fatal("carousel lost its owning page")
	}
	style := obj(cfg["navigationStyle"])
	if number(style["fontSize"]) != 17 || style["activeColor"] != "#FfAA00" || len(style) != 3 {
		t.Fatalf("style boundary: %v", style)
	}
	if _, ok := cfg["colors"]; ok {
		t.Fatal("theme visuals leaked into operational configuration")
	}
}

func TestIndependentPagesPublishRestartRollbackAndPlatforms(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	for _, p := range platforms {
		t.Run(p, func(t *testing.T) {
			draft := pageDraft()
			obj(arr(draft["contentPages"])[0])["title"] = "动漫-" + p
			w := h.req("PUT", "/api/admin/draft", M{"version": 1, "theme": draft, "template": "runtime-" + p}, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
			// A saved draft must not become public before publication.
			w = h.req("GET", "/api/client/bootstrap?platform="+p, nil, "")
			if len(arr(obj(payload(t, w)["config"])["contentPages"])) != 0 {
				t.Fatal("unpublished draft became public")
			}
			w = h.req("POST", "/api/admin/publish", M{"version": 2, "template": "runtime-" + p}, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
			w = h.req("GET", "/api/client/bootstrap?platform="+p, nil, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
			assertPageConfig(t, obj(payload(t, w)["config"]))
		})
	}
	restarted, err := newApp(context.Background(), h.a.config, h.a.store)
	if err != nil {
		t.Fatal(err)
	}
	state := restarted.readState()
	for _, p := range platforms {
		assertPageConfig(t, state.Operations[p].Draft)
		response, err := bootstrap(state, p, "web", "", h.origin)
		if err != nil {
			t.Fatal(err)
		}
		cfg := obj(response["config"])
		assertPageConfig(t, cfg)
		if obj(arr(cfg["contentPages"])[0])["title"] != "动漫-"+p {
			t.Fatal("platform configuration crossed boundaries")
		}
	}
	w := h.req("POST", "/api/admin/rollback", M{"version": 3, "targetVersion": 1, "template": "runtime-tv"}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	tv := payload(t, h.req("GET", "/api/client/bootstrap?platform=tv", nil, ""))
	if len(arr(obj(tv["config"])["contentPages"])) != 0 {
		t.Fatal("rollback retained pages from a newer publication")
	}
	for _, p := range []string{"mobile", "tablet", "web"} {
		assertPageConfig(t, obj(payload(t, h.req("GET", "/api/client/bootstrap?platform="+p, nil, ""))["config"]))
	}
}

func TestIndependentPageValidationAndLegacyCompatibility(t *testing.T) {
	if _, err := operationConfig(clone(defaults)); err != nil {
		t.Fatal("legacy config rejected", err)
	}
	tests := []struct {
		name   string
		change func(M)
	}{
		{"duplicate ID", func(d M) { a := arr(d["contentPages"]); obj(a[1])["id"] = "anime" }},
		{"invalid ID", func(d M) { obj(arr(d["contentPages"])[0])["id"] = "../page" }},
		{"unknown kind", func(d M) { obj(arr(d["contentPages"])[0])["kind"] = "executable" }},
		{"empty title", func(d M) { obj(arr(d["contentPages"])[0])["title"] = "" }},
		{"oversized category", func(d M) { obj(arr(d["contentPages"])[0])["categoryId"] = strings.Repeat("x", 61) }},
		{"invalid category type", func(d M) { obj(arr(d["contentPages"])[0])["categoryId"] = 1 }},
		{"invalid enabled", func(d M) { obj(arr(d["contentPages"])[0])["enabled"] = "true" }},
		{"invalid carousel switch", func(d M) { obj(arr(d["contentPages"])[0])["carouselEnabled"] = "false" }},
		{"campaign category binding", func(d M) { obj(arr(d["contentPages"])[2])["categoryId"] = "1" }},
		{"blank modules", func(d M) { obj(arr(d["sections"])[0])["pageId"] = "empty" }},
		{"invalid reference type", func(d M) { obj(arr(d["navigation"])[0])["pageId"] = false }},
		{"bad style type", func(d M) { d["navigationStyle"] = []any{} }},
		{"font too small", func(d M) { obj(d["navigationStyle"])["fontSize"] = float64(9) }},
		{"font too large", func(d M) { obj(d["navigationStyle"])["fontSize"] = float64(29) }},
		{"font decimal", func(d M) { obj(d["navigationStyle"])["fontSize"] = 17.5 }},
		{"unsafe color", func(d M) { obj(d["navigationStyle"])["color"] = "url(https://example.test)" }},
		{"too many pages", func(d M) {
			a := []any{}
			for i := 0; i < 41; i++ {
				a = append(a, M{"id": fmt.Sprint("p", i), "title": "Page", "kind": "blank"})
			}
			d["contentPages"] = a
		}},
	}
	for _, field := range []string{"navigation", "categories", "sections", "carousel"} {
		key := field
		tests = append(tests, struct {
			name   string
			change func(M)
		}{"dangling " + key, func(d M) {
			a := arr(d[key])
			if key == "carousel" {
				a = arr(obj(d[key])["slides"])
			}
			obj(a[0])["pageId"] = "missing"
		}})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := pageDraft()
			tc.change(d)
			if _, err := operationConfig(d); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	d := pageDraft()
	p := obj(arr(d["contentPages"])[0])
	delete(p, "enabled")
	delete(p, "carouselEnabled")
	delete(p, "categoryId")
	out, err := operationConfig(d)
	if err != nil {
		t.Fatal(err)
	}
	p = obj(arr(out["contentPages"])[0])
	if p["enabled"] != true || p["carouselEnabled"] != true || p["categoryId"] != "" {
		t.Fatal("missing field defaults", p)
	}
	for _, size := range []float64{10, 28} {
		d = pageDraft()
		obj(d["navigationStyle"])["fontSize"] = size
		if _, err := operationConfig(d); err != nil {
			t.Fatal("valid font boundary rejected", err)
		}
	}
}

// Primary navigation owns its CMS source even when its destination page changes.
func TestPrimaryCategorySourceIndependentOfPage(t *testing.T) {
	for _, pageID := range []string{"movies", "summer", "empty", ""} {
		for _, source := range []string{"cms-primary", ""} {
			d := pageDraft()
			entry := obj(arr(d["categories"])[0])
			entry["categoryId"] = source
			entry["pageId"] = pageID
			entry["textColor"] = "#ff6688"
			for round := 0; round < 2; round++ {
				cfg, err := operationConfig(d)
				if err != nil {
					t.Fatal(pageID, source, err)
				}
				got := obj(arr(cfg["categories"])[0])
				if got["categoryId"] != source || str(got["pageId"]) != pageID || got["textColor"] != "#ff6688" || got["display"] != "text" {
					t.Fatalf("binding changed for page %q source %q: %v", pageID, source, got)
				}
				if obj(arr(cfg["navigation"])[0])["categoryId"] != "1" {
					t.Fatal("other navigation behavior changed")
				}
				d = mergeOperations(cfg)
			}
		}
	}
}
