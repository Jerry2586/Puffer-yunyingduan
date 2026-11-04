package main

import (
	"context"
	"math"
	"reflect"
	"testing"
)

func TestPageCarouselAppearanceValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"null", nil}, {"array", []any{}}, {"string", "full"},
		{"unknown style", M{"style": "script"}}, {"theme video style", M{"style": "video"}},
		{"invalid fit", M{"fit": "fill"}}, {"numeric string", M{"height": "240"}},
		{"fraction", M{"radius": 12.5}}, {"too narrow", M{"widthPercent": 39.0}},
		{"too wide", M{"widthPercent": 101.0}}, {"too short", M{"height": 119.0}},
		{"too tall", M{"height": 601.0}}, {"negative margin", M{"margin": -1.0}},
		{"large margin", M{"margin": 49.0}}, {"large radius", M{"radius": 301.0}},
		{"NaN", M{"height": math.NaN()}}, {"infinity", M{"height": math.Inf(1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := operationCarouselAppearance(tc.value); err == nil {
				t.Fatal("invalid appearance accepted")
			}
		})
	}
	draft := pageDraft()
	expected := M{"style": "split", "fit": "contain", "widthPercent": 40.0, "height": 600.0, "margin": 48.0, "radius": 300.0}
	obj(arr(draft["contentPages"])[0])["carouselAppearance"] = clone(expected)
	obj(obj(arr(draft["contentPages"])[0])["carouselAppearance"])["image"] = "https://example.test/not-a-page-asset"
	obj(arr(draft["contentPages"])[1])["carouselAppearance"] = M{"script": "discard"}
	clean, err := operationConfig(draft)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(obj(arr(clean["contentPages"])[0])["carouselAppearance"], expected) {
		t.Fatal("appearance missing or unrecognized fields retained")
	}
	for _, raw := range arr(clean["contentPages"])[1:] {
		if _, ok := obj(raw)["carouselAppearance"]; ok {
			t.Fatal("empty or legacy appearance did not inherit defaults")
		}
	}
	// Every supported image layout must survive the normal operations validator.
	for _, style := range []string{"full", "inset", "peek", "poster", "split"} {
		obj(arr(draft["contentPages"])[0])["carouselAppearance"] = M{"style": style, "fit": "cover", "widthPercent": 100.0, "height": 120.0, "radius": 0.0, "margin": 0.0}
		if _, err := operationConfig(draft); err != nil {
			t.Fatal(style, err)
		}
	}
}

func TestPageCarouselAppearanceApplyRestartIsolationAndReset(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	draft := pageDraft()
	a := M{"style": "inset", "widthPercent": 80.0, "height": 280.0, "radius": 28.0, "margin": 12.0, "fit": "contain"}
	b := M{"style": "poster", "height": 480.0, "radius": 0.0}
	obj(arr(draft["contentPages"])[0])["carouselAppearance"] = a
	obj(arr(draft["contentPages"])[1])["carouselAppearance"] = b
	before := h.a.readState()
	w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 1, "theme": draft}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	applied := h.a.readState()
	mobile := applied.Operations["mobile"]
	if !reflect.DeepEqual(mobile.Draft, mobile.Published) {
		t.Fatal("autosave not applied atomically")
	}
	if !reflect.DeepEqual(before.Operations["tablet"], applied.Operations["tablet"]) {
		t.Fatal("another client changed")
	}
	stored, err := h.a.store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.Operations["mobile"], mobile) {
		t.Fatal("appearance not durable")
	}
	restarted, err := newApp(context.Background(), h.a.config, h.a.store)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapData, err := bootstrap(restarted.readState(), "mobile", "android", "", h.origin)
	if err != nil {
		t.Fatal(err)
	}
	cfg := obj(bootstrapData["config"])
	assertPageConfig(t, cfg)
	pages := arr(cfg["contentPages"])
	if !reflect.DeepEqual(obj(pages[0])["carouselAppearance"], a) || !reflect.DeepEqual(obj(pages[1])["carouselAppearance"], b) {
		t.Fatal("page appearance mixed or lost on restart")
	}
	good := h.a.readState()
	invalid := clone(cfg)
	obj(arr(invalid["contentPages"])[0])["carouselAppearance"] = M{"height": -1.0}
	if w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 2, "theme": invalid}, ""); w.Code != 400 {
		t.Fatal(w.Code, w.Body)
	}
	if !reflect.DeepEqual(good, h.a.readState()) {
		t.Fatal("invalid appearance replaced last good state")
	}
	reset := clone(cfg)
	delete(obj(arr(reset["contentPages"])[0]), "carouselAppearance")
	w = h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 2, "theme": reset}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	expected := clone(cfg)
	delete(obj(arr(expected["contentPages"])[0]), "carouselAppearance")
	if !reflect.DeepEqual(h.a.readState().Operations["mobile"].Published, expected) {
		t.Fatal("reset changed content, bindings, another page or theme defaults")
	}
}
