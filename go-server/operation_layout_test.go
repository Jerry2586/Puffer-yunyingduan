package main

import (
	"context"
	"reflect"
	"testing"
)

func layoutDraft() M {
	d := shortcutDraft()
	sections := arr(d["sections"])
	first := obj(sections[0])
	first["style"], first["columns"], first["gap"], first["radius"], first["ratio"] = "scroll", float64(4), float64(12), float64(16), "16/9"
	first["showTitle"] = false
	first["appearance"] = M{"widthPercent": float64(88), "cardWidth": float64(160), "headingColor": "#aabbcc", "paddingX": float64(0)}
	second := clone(first)
	second["id"], second["appearance"] = "independent-section", M{"headingSize": float64(24)}
	d["sections"] = append(sections, second)
	d["serviceMenuStyle"] = M{"widthPercent": float64(85), "rowHeight": float64(60), "background": "#102030"}
	groups := arr(d["categoryShortcuts"])
	obj(groups[0])["appearance"] = M{"arrangement": "row", "fontSize": float64(18), "activeBackground": "#112233"}
	obj(groups[1])["appearance"] = M{"arrangement": "column", "fontSize": float64(12)}
	return d
}
func TestOperationLayoutsApplyRestartAndReset(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	draft := layoutDraft()
	w := h.req("PUT", "/api/admin/apply", M{"version": 1, "theme": draft, "template": "runtime-mobile"}, "")
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
	config := obj(response["config"])
	expected, err := operationConfig(draft)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"sections", "serviceMenuStyle", "categoryShortcuts"} {
		if !reflect.DeepEqual(config[key], expected[key]) {
			t.Fatalf("%s lost on restart: %#v != %#v", key, config[key], expected[key])
		}
	}
	delete(obj(arr(draft["sections"])[0]), "appearance")
	delete(obj(arr(draft["categoryShortcuts"])[0]), "appearance")
	delete(draft, "serviceMenuStyle")
	w = h.req("PUT", "/api/admin/apply", M{"version": 2, "theme": draft, "template": "runtime-mobile"}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	config = h.a.readState().Operations["mobile"].Published
	if _, ok := config["serviceMenuStyle"]; ok {
		t.Fatal("menu reset lost")
	}
	if _, ok := obj(arr(config["sections"])[0])["appearance"]; ok {
		t.Fatal("section reset lost")
	}
	groups := arr(config["categoryShortcuts"])
	if _, ok := obj(groups[0])["appearance"]; ok {
		t.Fatal("group reset lost")
	}
	if obj(groups[0])["entryId"] != "movie-nav" || obj(obj(groups[1])["appearance"])["fontSize"] != float64(12) {
		t.Fatal("reset affected another owner", groups)
	}
}
func TestOperationAppearanceBoundsAndTypes(t *testing.T) {
	for kind, rules := range appearanceNumbers {
		for key, bounds := range rules {
			for _, v := range []any{bounds[0], bounds[1]} {
				if _, err := operationAppearance(M{key: v}, kind); err != nil {
					t.Fatalf("valid %s.%s=%v: %v", kind, key, v, err)
				}
			}
			for _, v := range []any{bounds[0] - 1, bounds[1] + 1, bounds[0] + 0.5, "20", nil, true} {
				if _, err := operationAppearance(M{key: v}, kind); err == nil {
					t.Fatalf("invalid %s.%s=%v accepted", kind, key, v)
				}
			}
		}
		for _, key := range appearanceColors[kind] {
			for _, v := range []any{"red", "#abc", "url(https://example.test/a)", nil, float64(1)} {
				if _, err := operationAppearance(M{key: v}, kind); err == nil {
					t.Fatal("invalid color accepted", kind, key)
				}
			}
		}
		if _, err := operationAppearance(nil, kind); err == nil {
			t.Fatal("null style accepted")
		}
		out, err := operationAppearance(M{"css": "position:fixed", "icon": "arbitrary.svg"}, kind)
		if err != nil || len(out) != 0 {
			t.Fatal("unowned fields crossed boundary", out, err)
		}
	}
	for _, v := range []any{"diagonal", nil, float64(1)} {
		if _, err := operationAppearance(M{"arrangement": v}, "middle"); err == nil {
			t.Fatal("invalid direction accepted")
		}
	}
}
func TestOperationLayoutWiringRejectsInvalidFields(t *testing.T) {
	mutations := []func(M){
		func(d M) { d["serviceMenuStyle"] = M{"widthPercent": float64(99)} },
		func(d M) { obj(arr(d["sections"])[0])["appearance"] = M{"textSize": "12"} },
		func(d M) { obj(arr(d["sections"])[0])["columns"] = float64(2.5) },
		func(d M) { obj(arr(d["sections"])[0])["style"] = "custom-css" },
		func(d M) { obj(arr(d["sections"])[0])["ratio"] = "0/1" },
		func(d M) { obj(arr(d["sections"])[0])["showTitle"] = "true" },
		func(d M) { obj(arr(d["categoryShortcuts"])[0])["appearance"] = M{"arrangement": "diagonal"} }}
	for i, mutate := range mutations {
		d := layoutDraft()
		mutate(d)
		if _, err := operationConfig(d); err == nil {
			t.Fatal("invalid mutation accepted", i)
		}
	}
	if _, err := operationConfig(clone(defaults)); err != nil {
		t.Fatal("legacy defaults rejected", err)
	}
}
