package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPlaybackRuleBindingOrderAndDeletionGuard(t *testing.T) {
	c := defaultPlaybackConfig()
	c.Enabled = true
	c.Sources = []playbackSource{resolverFixtureSource("a", "https://a.test"), resolverFixtureSource("b", "https://b.test"), resolverFixtureSource("other", "https://other.test")}
	v := state{Playback: &playbackSettings{Published: c}}
	prefs := playbackPreferencesFor(v)
	prefs.Sources = []cmsPlaybackSource{{Code: "cms-a", Name: "主线", Enabled: true, Mode: "resolve", ResolverIDs: []string{"b", "a"}}}
	v.PlaybackPreferences = &prefs
	selected := playbackConfigForSource(v, "cms-a")
	if selected.DirectFirst || len(selected.Sources) != 2 || selected.Sources[0].ID != "b" || selected.Sources[1].ID != "a" {
		t.Fatal("binding/order ignored", selected)
	}
	selected.Sources[0].Name = "changed"
	if v.Playback.Published.Sources[1].Name == "changed" {
		t.Fatal("shared state mutated")
	}
	if e := validatePlaybackBindings(prefs, c); e != nil {
		t.Fatal(e)
	}
	c.Sources = c.Sources[:1]
	if e := validatePlaybackBindings(prefs, c); e == nil {
		t.Fatal("bound resolver deletion accepted")
	}
	prefs.Sources[0].ResolverIDs = []string{"a", "a"}
	if e := validatePlaybackPreferences(prefs); e == nil {
		t.Fatal("duplicate binding accepted")
	}
}

func TestPlaybackFailedDirectDoesNotLoop(t *testing.T) {
	a := &app{}
	c := defaultPlaybackConfig()
	result, err := a.resolvePlayback(context.Background(), c, "https://media.test/video.mp4", "mobile", []string{"direct"})
	if err != nil || result.OK || result.URL != "" {
		t.Fatal("failed direct must stop", result, err)
	}
}

func TestLocalPlaybackRulesAreStaticBoundedAndOrdered(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	root := t.TempDir()
	h.a.config.CMSRoots = []string{root}
	file := filepath.Join(root, "site", "application", "extra", "vodplayer.php")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	raw := "<?php return ['later'=>['from'=>'line-b','show'=>'备用','status'=>'0','sort'=>'9','parse'=>'SECRET'],'first'=>['from'=>'line-a','show'=>'主线','status'=>'1','sort'=>'1']];"
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	rows, err := h.a.localPlaybackSources(filepath.Join(root, "site"))
	if err != nil || len(rows) != 2 || str(obj(rows[0])["code"]) != "line-a" || obj(rows[0])["enabled"] != true || obj(rows[1])["enabled"] != false {
		t.Fatal(rows, err)
	}
	if _, ok := obj(rows[1])["parse"]; ok {
		t.Fatal("upstream parse secrets exposed")
	}
	if err := os.WriteFile(file, []byte("<?php return getenv('RULES');"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = h.a.localPlaybackSources(filepath.Join(root, "site")); err == nil {
		t.Fatal("dynamic PHP accepted")
	}
	if _, err = h.a.localPlaybackSources(t.TempDir()); err == nil {
		t.Fatal("outside root accepted")
	}
}

func TestPlaybackBindingsPersistAndRejectMissingResolvers(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	c := defaultPlaybackConfig()
	c.Enabled = true
	c.Sources = []playbackSource{resolverFixtureSource("one", "https://resolver.test")}
	if w := h.req("PUT", "/api/admin/playback-resolvers", M{"revision": 1, "config": c}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := h.req("POST", "/api/admin/playback-resolvers/publish", M{"revision": 2}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	prefs := playbackPreferencesFor(h.a.readState())
	prefs.Sources = []cmsPlaybackSource{{Code: "cms", Name: "来源", Enabled: true, Mode: "resolve", ResolverIDs: []string{"missing"}}}
	if w := h.req("POST", "/api/admin/playback-center", resolverMap(prefs), ""); w.Code != 400 {
		t.Fatal("missing accepted", w.Code, w.Body)
	}
	prefs.Sources[0].ResolverIDs = []string{"one"}
	if w := h.req("POST", "/api/admin/playback-center", resolverMap(prefs), ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	stored, e := h.a.store.Load(context.Background())
	if e != nil || stored.PlaybackPreferences.Sources[0].ResolverIDs[0] != "one" {
		t.Fatal(stored, e)
	}
	c.Sources = []playbackSource{}
	if w := h.req("PUT", "/api/admin/playback-resolvers", M{"revision": 3, "config": c}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := h.req("POST", "/api/admin/playback-resolvers/publish", M{"revision": 4}, ""); w.Code != 400 {
		t.Fatal("bound deletion published", w.Code, w.Body)
	}
}
