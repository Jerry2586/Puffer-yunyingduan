package main

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func carouselDraft() M {
	d := shortcutDraft()
	d["carouselGroups"] = []any{
		M{"id": "movie-hero", "name": "电影轮播", "enabled": true, "entryIds": []any{"movie-nav"}, "height": 999, "slides": []any{
			M{"id": "feature", "title": "精选", "image": "https://images.example/hero.jpg", "sourceVodId": "12", "enabled": true, "target": M{"type": "vod", "vodId": "34"}, "background": "blocked"},
			M{"id": "campaign", "title": "活动", "image": "/api/assets/" + strings.Repeat("a", 64) + ".webp", "sourceVodId": "", "enabled": true, "target": M{"type": "page", "pageId": "summer"}},
		}},
		M{"id": "second-hero", "name": "精选轮播", "enabled": true, "entryIds": []any{"second-nav"}, "slides": []any{
			M{"id": "external", "title": "专题", "image": "https://images.example/event.png", "sourceVodId": "", "enabled": true, "target": M{"type": "external", "url": "https://example.com/event"}},
			M{"id": "function", "title": "总排期", "image": "https://images.example/schedule.png", "sourceVodId": "", "enabled": true, "target": M{"type": "system", "page": "schedule"}},
		}},
	}
	return d
}

func TestCarouselsApplyRestartBootstrap(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	before := h.a.readState()
	w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 1, "theme": carouselDraft()}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	renamed := carouselDraft()
	obj(arr(renamed["carouselGroups"])[0])["name"] = "首页 活动"
	w = h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 2, "theme": renamed}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	after := h.a.readState()
	if !reflect.DeepEqual(before.Operations["tablet"], after.Operations["tablet"]) {
		t.Fatal("changed another client")
	}
	restarted, err := newApp(context.Background(), h.a.config, h.a.store)
	if err != nil {
		t.Fatal(err)
	}
	response, err := bootstrap(restarted.readState(), "mobile", "android", "", h.origin)
	if err != nil {
		t.Fatal(err)
	}
	cfg := obj(response["config"])
	groups := arr(cfg["carouselGroups"])
	if len(groups) != 2 {
		t.Fatal("lost groups", groups)
	}
	first, second := obj(groups[0]), obj(groups[1])
	if first["name"] != "首页 活动" || obj(arr(after.Operations["mobile"].Draft["carouselGroups"])[0])["name"] != "首页 活动" {
		t.Fatal("rename lost after apply or restart", first)
	}
	if arr(first["entryIds"])[0] != "movie-nav" || arr(second["entryIds"])[0] != "second-nav" {
		t.Fatal("confused entries sharing CMS category")
	}
	if _, exists := first["height"]; exists {
		t.Fatal("theme geometry crossed boundary")
	}
	slide := obj(arr(first["slides"])[0])
	if _, exists := slide["background"]; exists {
		t.Fatal("theme artwork styling crossed boundary")
	}
	if slide["sourceVodId"] != "12" || obj(slide["target"])["vodId"] != "34" {
		t.Fatal("display and target merged", slide)
	}
	if obj(arr(obj(cfg["carousel"])["slides"])[0])["id"] != "summer-hero" {
		t.Fatal("legacy carousel changed")
	}
	if !reflect.DeepEqual(cfg, after.Operations["mobile"].Published) {
		t.Fatal("restart changed published configuration")
	}
}

func TestCarouselsInvalidApplyIsAtomic(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	before := h.a.readState()
	for _, tc := range []struct {
		name string
		edit func(M)
	}{
		{"multiple entries", func(d M) {
			d["carouselGroups"] = arr(d["carouselGroups"])[:1]
			obj(arr(d["carouselGroups"])[0])["entryIds"] = []any{"movie-nav", "second-nav"}
		}},
		{"disabled multiple entries", func(d M) {
			d["carouselGroups"] = arr(d["carouselGroups"])[:1]
			g := obj(arr(d["carouselGroups"])[0])
			g["enabled"] = false
			g["entryIds"] = []any{"movie-nav", "second-nav"}
		}},
		{"duplicate owner", func(d M) { obj(arr(d["carouselGroups"])[1])["entryIds"] = []any{"movie-nav"} }},
		{"unknown entry", func(d M) { obj(arr(d["carouselGroups"])[0])["entryIds"] = []any{"1"} }},
		{"invalid name mode", func(d M) { obj(arr(d["carouselGroups"])[0])["nameMode"] = "invalid" }},
		{"non-string name mode", func(d M) { obj(arr(d["carouselGroups"])[0])["nameMode"] = M{"auto": true} }},
		{"stale page", func(d M) { carouselTestSlide(d)["target"] = M{"type": "page", "pageId": "deleted"} }},
		{"mixed target", func(d M) {
			carouselTestSlide(d)["target"] = M{"type": "vod", "vodId": "1", "url": "https://example.com"}
		}},
		{"script target", func(d M) { carouselTestSlide(d)["target"] = M{"type": "external", "url": "javascript:alert(1)"} }},
		{"credential URL", func(d M) { carouselTestSlide(d)["image"] = "https://user:pass@example.com/a.png" }},
		{"script image", func(d M) { carouselTestSlide(d)["image"] = "data:image/png,abc" }},
		{"video asset", func(d M) { carouselTestSlide(d)["image"] = "/api/assets/" + strings.Repeat("a", 64) + ".mp4" }},
		{"invalid source", func(d M) { carouselTestSlide(d)["sourceVodId"] = "abc" }},
		{"oversized source", func(d M) { carouselTestSlide(d)["sourceVodId"] = strings.Repeat("1", 21) }},
		{"invalid function", func(d M) { carouselTestSlide(d)["target"] = M{"type": "system", "page": "unknown"} }},
		{"no image", func(d M) { carouselTestSlide(d)["image"] = "" }},
		{"no target", func(d M) { carouselTestSlide(d)["target"] = M{"type": "vod", "vodId": ""} }},
		{"no binding", func(d M) { obj(arr(d["carouselGroups"])[0])["entryIds"] = []any{} }},
		{"too many slides", func(d M) { g := obj(arr(d["carouselGroups"])[0]); g["slides"] = make([]any, 21) }},
		{"too many groups", func(d M) { d["carouselGroups"] = make([]any, 41) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := carouselDraft()
			tc.edit(d)
			w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 1, "theme": d}, "")
			if w.Code != 400 {
				t.Fatal(w.Code, w.Body)
			}
			if !reflect.DeepEqual(before, h.a.readState()) {
				t.Fatal("rejected input changed state")
			}
		})
	}
}
func carouselTestSlide(d M) M { return obj(arr(obj(arr(d["carouselGroups"])[0])["slides"])[0]) }

func TestCarouselsIncompleteDraftAndLegacy(t *testing.T) {
	for _, target := range []M{{"type": "vod", "vodId": ""}, {"type": "page", "pageId": ""}, {"type": "external", "url": ""}} {
		d := shortcutDraft()
		d["carouselGroups"] = []any{M{"id": "draft", "name": "未完成", "enabled": false, "entryIds": []any{}, "slides": []any{M{"id": "new", "title": "", "image": "", "sourceVodId": "", "enabled": false, "target": target}}}}
		if _, err := operationConfig(d); err != nil {
			t.Fatal(err)
		}
	}
	out, err := operationConfig(shortcutDraft())
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := out["carouselGroups"]; exists {
		t.Fatal("added phantom carousel groups to legacy config")
	}
}

func TestCMSCarouselArtworkDoesNotExposePlayback(t *testing.T) {
	v := normalizeVod(M{"vod_id": "12", "vod_name": "film", "vod_pic": "/poster.jpg", "vod_pic_slide": "/hero.jpg", "vod_play_url": "ep$https://secret.example/video.m3u8"}, "https://cms.example/api.php/provide/vod/")
	if v["carouselImage"] != "https://cms.example/hero.jpg" || v["poster"] != "https://cms.example/poster.jpg" {
		t.Fatal(v)
	}
	encoded, _ := json.Marshal(v)
	if strings.Contains(string(encoded), "secret.example") {
		t.Fatal("catalog leaked playback URL")
	}
	old := normalizeVod(M{"vod_id": "1", "vod_pic": "/poster.jpg"}, "https://cms.example/")
	if old["carouselImage"] != "" {
		t.Fatal("fabricated legacy artwork")
	}
}

func TestCarouselsEnabledWithoutActiveSlidesPersist(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	for i, slides := range [][]any{{}, {M{"id": "draft-slide", "title": "", "image": "", "sourceVodId": "", "enabled": false, "target": M{"type": "vod", "vodId": ""}}}, {}} {
		d := carouselDraft()
		g := obj(arr(d["carouselGroups"])[0])
		g["slides"] = slides
		w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": i + 1, "theme": d}, "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		restarted, err := newApp(context.Background(), h.a.config, h.a.store)
		if err != nil {
			t.Fatal(err)
		}
		response, err := bootstrap(restarted.readState(), "mobile", "android", "", h.origin)
		if err != nil {
			t.Fatal(err)
		}
		saved := obj(arr(obj(response["config"])["carouselGroups"])[0])
		if saved["enabled"] != true || len(arr(saved["slides"])) != len(slides) {
			t.Fatal("empty group switch lost", saved)
		}
	}
}

func TestCarouselsNamesFollowBindingAndPreserveCustomAcrossRestart(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	d := carouselDraft()
	first := obj(arr(d["carouselGroups"])[0])
	first["name"], first["nameMode"] = "新轮播组", "auto"
	canonical, err := operationConfig(d)
	if err != nil {
		t.Fatal(err)
	}
	originalSlides := clone(obj(arr(canonical["carouselGroups"])[0])["slides"])
	for i, tc := range []struct {
		name string
		mode string
		edit func(M)
	}{
		{"电影轮播组", "auto", func(M) {}},
		{"首页轮播组", "auto", func(d M) { obj(arr(d["categories"])[0])["label"] = "首页" }},
		{"精选电影轮播组", "auto", func(d M) {
			second := obj(arr(d["carouselGroups"])[1])
			second["entryIds"], second["enabled"] = []any{}, false
			obj(arr(d["carouselGroups"])[0])["entryIds"] = []any{"second-nav"}
		}},
		{"本周精选", "custom", func(d M) { g := obj(arr(d["carouselGroups"])[0]); g["name"], g["nameMode"] = "本周精选", "custom" }},
		{"本周精选", "custom", func(d M) { obj(arr(d["carouselGroups"])[0])["entryIds"] = []any{"movie-nav"} }},
		{"本周精选", "custom", func(d M) { g := obj(arr(d["carouselGroups"])[0]); g["entryIds"], g["enabled"] = []any{}, false }},
	} {
		tc.edit(d)
		w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": i + 1, "theme": d}, "")
		if w.Code != 200 {
			t.Fatal(i, w.Code, w.Body)
		}
		restarted, err := newApp(context.Background(), h.a.config, h.a.store)
		if err != nil {
			t.Fatal(err)
		}
		response, err := bootstrap(restarted.readState(), "mobile", "android", "", h.origin)
		if err != nil {
			t.Fatal(err)
		}
		saved := obj(arr(obj(response["config"])["carouselGroups"])[0])
		if saved["name"] != tc.name || saved["nameMode"] != tc.mode {
			t.Fatal(i, "name lost after apply/restart", saved)
		}
		if !reflect.DeepEqual(saved["slides"], originalSlides) {
			t.Fatal("name change altered slides")
		}
		d = clone(h.a.readState().Operations["mobile"].Draft)
	}
}

func TestCarouselsLegacyNameModesAndLongLabels(t *testing.T) {
	for _, tc := range []struct{ name, mode, expected, expectedMode, label string }{
		{"新轮播组", "", "电影轮播组", "auto", "电影"},
		{"电影轮播组", "", "电影轮播组", "auto", "电影"},
		{"自定义专题", "", "自定义专题", "custom", "电影"},
		{"电影轮播组", "custom", "电影轮播组", "custom", "电影"},
		{"新轮播组", "auto", strings.Repeat("影", 77) + "轮播组", "auto", strings.Repeat("影", 100)},
		{"新轮播组", "auto", strings.Repeat("𠮷", 57) + "轮播组", "auto", strings.Repeat("𠮷", 100)},
	} {
		d := carouselDraft()
		obj(arr(d["categories"])[0])["label"] = tc.label
		g := obj(arr(d["carouselGroups"])[0])
		g["name"] = tc.name
		if tc.mode != "" {
			g["nameMode"] = tc.mode
		}
		out, err := operationConfig(d)
		if err != nil {
			t.Fatal(err)
		}
		saved := obj(arr(out["carouselGroups"])[0])
		if saved["name"] != tc.expected || saved["nameMode"] != tc.expectedMode || len(str(saved["name"])) > 240 {
			t.Fatal("incorrect name migration", saved)
		}
	}
}
