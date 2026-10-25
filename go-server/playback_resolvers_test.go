package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func resolverFixtureSource(id, endpoint string) playbackSource {
	return playbackSource{ID: id, Name: id, Kind: "json", Endpoint: endpoint + "?url={url}&key=SERVER_ONLY", URLField: "data.url", Enabled: true, Platforms: []string{"mobile", "tablet", "tv", "web"}}
}
func resolverMap(v any) M { b, _ := json.Marshal(v); var m M; _ = json.Unmarshal(b, &m); return m }
func TestPlaybackResolverConfigPersistenceAndBoundaries(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	if w := h.req("GET", "/api/admin/playback-resolvers", nil, ""); w.Code != 200 || number(payload(t, w)["revision"]) != 1 {
		t.Fatal(w.Code, w.Body)
	}
	cookie := h.cookie
	h.cookie = ""
	if w := h.req("GET", "/api/admin/playback-resolvers", nil, ""); w.Code != 401 {
		t.Fatal("admin auth", w.Code)
	}
	h.cookie = cookie
	c := defaultPlaybackConfig()
	c.Enabled = true
	c.Sources = []playbackSource{resolverFixtureSource("a", "https://resolver.test/api")}
	w := h.req("PUT", "/api/admin/playback-resolvers", M{"revision": 1, "config": c}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("PUT", "/api/admin/playback-resolvers", M{"revision": 1, "config": c}, ""); w.Code != 409 {
		t.Fatal("CAS", w.Code, w.Body)
	}
	saved := playbackState(h.a.readState())
	if saved.Published.Enabled || saved.Revision != 2 {
		t.Fatal("draft affected live", saved)
	}
	w = h.req("POST", "/api/admin/playback-resolvers/publish", M{"revision": 2}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	stored, e := h.a.store.Load(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if stored.Playback.PublishedVersion != 1 || stored.Playback.Published.Sources[0].ID != "a" {
		t.Fatal("not durable")
	}
	// A mutation without the studio source header must remain forbidden.
	req := httptest.NewRequest("PUT", h.origin+"/api/admin/playback-resolvers", strings.NewReader("{}"))
	req.AddCookie(&http.Cookie{Name: "hetun_session", Value: cookie})
	rr := httptest.NewRecorder()
	h.a.ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Fatal("origin boundary", rr.Code)
	}
	before := h.a.readState().Revision
	w = h.req("POST", "/api/admin/playback-resolvers/test", M{"config": c, "platform": "mobile", "url": "file:///bad"}, "")
	if w.Code != 400 || h.a.readState().Revision != before {
		t.Fatal("test mutates state or invalid input accepted")
	}
}
func TestPlaybackResolverValidation(t *testing.T) {
	base := defaultPlaybackConfig()
	base.Sources = []playbackSource{resolverFixtureSource("a", "https://resolver.test/api")}
	for name, edit := range map[string]func(*playbackConfig){
		"duplicate":           func(c *playbackConfig) { c.Sources = append(c.Sources, c.Sources[0]) },
		"scheme":              func(c *playbackConfig) { c.Sources[0].Endpoint = "file:///api?url={url}" },
		"host placeholder":    func(c *playbackConfig) { c.Sources[0].Endpoint = "https://{url}/api?x=1" },
		"missing placeholder": func(c *playbackConfig) { c.Sources[0].Endpoint = "https://resolver.test/api" },
		"query key":           func(c *playbackConfig) { c.Sources[0].Endpoint = "https://resolver.test/?{url}=x" },
		"bad field":           func(c *playbackConfig) { c.Sources[0].URLField = "data[0].url" },
		"platform":            func(c *playbackConfig) { c.Sources[0].Platforms = []string{"desktop"} },
		"budget":              func(c *playbackConfig) { c.TotalSeconds = 21 },
		"timeout":             func(c *playbackConfig) { c.TimeoutSeconds = 0 },
		"attempts":            func(c *playbackConfig) { c.MaxAttempts = 0 },
		"webpage":             func(c *playbackConfig) { c.Sources[0].Kind = "html" },
	} {
		t.Run(name, func(t *testing.T) {
			c := clone(base)
			edit(&c)
			if validatePlayback(c) == nil {
				t.Fatal("invalid accepted")
			}
		})
	}
	if _, e := decodePlayback(M{"enabled": "yes"}); e == nil {
		t.Fatal("coerced bool")
	}
	if _, e := decodePlayback(M{"extra": true}); e == nil {
		t.Fatal("unknown config accepted")
	}
}
func TestPlaybackResolverOrderedFailureAndEncodedSource(t *testing.T) {
	raw := "https://content.test/watch?id=1&signature=a+b#bad"
	raw = strings.TrimSuffix(raw, "#bad")
	var calls []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.URL.Query().Get("url") != raw {
			t.Errorf("source encoding: %q", r.URL.Query().Get("url"))
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("session leaked")
		}
		switch r.URL.Path {
		case "/bad":
			w.WriteHeader(503)
		case "/invalid":
			jsonResponse(w, 200, M{"data": M{"url": "javascript:alert(1)"}})
		default:
			jsonResponse(w, 200, M{"data": M{"url": "https://media.test/live.m3u8"}})
		}
	}))
	defer upstream.Close()
	h := newHarness(t, "")
	c := defaultPlaybackConfig()
	c.Enabled = true
	c.DirectFirst = false
	c.Sources = []playbackSource{resolverFixtureSource("bad", upstream.URL+"/bad"), resolverFixtureSource("invalid", upstream.URL+"/invalid"), resolverFixtureSource("ok", upstream.URL+"/ok")}
	got, e := h.a.resolvePlayback(context.Background(), c, raw, "mobile", nil)
	if e != nil || !got.OK || got.SourceID != "ok" || len(got.Attempts) != 3 || strings.Join(calls, ",") != "/bad,/invalid,/ok" {
		t.Fatal(got, e, calls)
	}
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), "SERVER_ONLY") || strings.Contains(string(b), upstream.URL) {
		t.Fatal("endpoint credentials leaked")
	}
}
func TestPlaybackResolverScopeBudgetAndContinuation(t *testing.T) {
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		jsonResponse(w, 200, M{"data": M{"url": "https://media.test/video.mp4"}})
	}))
	defer upstream.Close()
	h := newHarness(t, "")
	c := defaultPlaybackConfig()
	c.Enabled = true
	c.DirectFirst = false
	c.MaxAttempts = 1
	c.Sources = []playbackSource{resolverFixtureSource("a", upstream.URL), resolverFixtureSource("b", upstream.URL)}
	c.Sources[0].Platforms = []string{"tv"}
	got, e := h.a.resolvePlayback(context.Background(), c, "https://content.test/watch", "mobile", nil)
	if e != nil || got.SourceID != "b" || hits.Load() != 1 {
		t.Fatal(got, e)
	}
	got, _ = h.a.resolvePlayback(context.Background(), c, "https://content.test/watch", "mobile", []string{"b"})
	if got.OK || hits.Load() != 1 {
		t.Fatal("retry budget reset", got)
	}
	c.MaxAttempts = 3
	c.Sources[0].Platforms = []string{"mobile"}
	got, _ = h.a.resolvePlayback(context.Background(), c, "https://content.test/watch", "mobile", []string{"a"})
	if got.SourceID != "b" {
		t.Fatal("excluded source reused", got)
	}
	c.DirectFirst = true
	before := hits.Load()
	got, _ = h.a.resolvePlayback(context.Background(), c, "https://media.test/video.mp4?sign=x", "mobile", nil)
	if got.SourceID != "direct" || hits.Load() != before {
		t.Fatal("direct not preferred")
	}
	got, _ = h.a.resolvePlayback(context.Background(), c, "https://media.test/video.mp4", "mobile", []string{"direct"})
	if got.SourceID != "a" {
		t.Fatal("direct failure continuation", got)
	}
	if _, e = resolverExcludes([]any{"a", "a"}, c); e == nil {
		t.Fatal("duplicate excludes")
	}
	if _, e = resolverExcludes([]any{"unknown"}, c); e == nil {
		t.Fatal("unknown excludes")
	}
	c.Enabled = false
	got, _ = h.a.resolvePlayback(context.Background(), c, "https://content.test/watch", "mobile", nil)
	if !got.OK || got.SourceID != "direct" {
		t.Fatal("legacy disabled changed")
	}
}
func TestPlaybackResolverRedirectAndMalformedJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			w.Header().Set("Location", "https://media.test/a.m3u8")
			w.WriteHeader(302)
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", 2*1024*1024+1)))
		default:
			_, _ = w.Write([]byte("<html>not JSON</html>"))
		}
	}))
	defer upstream.Close()
	h := newHarness(t, "")
	c := defaultPlaybackConfig()
	c.Enabled = true
	c.DirectFirst = false
	c.Sources = []playbackSource{resolverFixtureSource("html", upstream.URL+"/html"), resolverFixtureSource("large", upstream.URL+"/large"), resolverFixtureSource("redirect", upstream.URL+"/redirect")}
	c.Sources[2].Kind = "redirect"
	got, e := h.a.resolvePlayback(context.Background(), c, "https://source.test/watch", "tv", nil)
	if e != nil || got.SourceID != "redirect" || len(got.Attempts) != 3 {
		t.Fatal(got, e)
	}
}
func TestPlaybackResolverTimeoutAndCancellation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(1500 * time.Millisecond):
		case <-r.Context().Done():
		}
		jsonResponse(w, 200, M{"data": M{"url": "https://media.test/a.mp4"}})
	}))
	defer upstream.Close()
	h := newHarness(t, "")
	c := defaultPlaybackConfig()
	c.Enabled = true
	c.DirectFirst = false
	c.TimeoutSeconds = 1
	c.TotalSeconds = 1
	c.Sources = []playbackSource{resolverFixtureSource("slow", upstream.URL), resolverFixtureSource("never", upstream.URL)}
	started := time.Now()
	got, e := h.a.resolvePlayback(context.Background(), c, "https://source.test/watch", "mobile", nil)
	if e != nil || got.OK || len(got.Attempts) != 1 || time.Since(started) > 1400*time.Millisecond {
		t.Fatal("timeout not bounded", got, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, _ = h.a.resolvePlayback(ctx, c, "https://source.test/watch", "mobile", nil)
	if len(got.Attempts) != 0 {
		t.Fatal("cancel ignored")
	}
}
func TestPlaybackResolverFourClientAuthorization(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		jsonResponse(w, 200, M{"data": M{"url": "https://media.test/parsed.mp4"}})
	}))
	defer upstream.Close()
	h := newHarness(t, cms.URL)
	c := defaultPlaybackConfig()
	c.Enabled = true
	c.DirectFirst = false
	c.Sources = []playbackSource{resolverFixtureSource("shared", upstream.URL)}
	if w := h.req("PUT", "/api/admin/playback-resolvers", M{"revision": 1, "config": c}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := h.req("POST", "/api/admin/playback-resolvers/publish", M{"revision": 2}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	for _, p := range []string{"mobile", "tablet", "tv", "desktop"} {
		w := h.req("POST", "/api/v1/"+p+"/playback", M{"id": "1", "sid": 1, "nid": 1}, "")
		if w.Code == 200 || hits.Load() != 0 {
			t.Fatal("resolver called before authorization", p, w.Code)
		}
	}
	login := h.req("POST", "/api/v1/mobile/member/login", M{"username": "viewer", "password": "correct"}, "")
	token := str(payload(t, login)["token"])
	if token == "" {
		t.Fatal(login.Body)
	}
	for _, p := range []string{"mobile", "tablet", "tv", "desktop"} {
		w := h.req("POST", "/api/v1/"+p+"/playback", M{"id": "1", "sid": 1, "nid": 1}, token)
		m := payload(t, w)
		if w.Code != 200 || m["url"] != "https://media.test/parsed.mp4" || m["sourceId"] != "shared" || strings.Contains(w.Body.String(), "SERVER_ONLY") {
			t.Fatal(p, w.Code, w.Body)
		}
	}
	w := h.req("POST", "/api/v1/tv/playback", M{"id": "1", "sid": 1, "nid": 1, "excludeSourceIds": []string{"shared"}}, token)
	if w.Code != 502 || hits.Load() != 4 {
		t.Fatal("no more sources failure", w.Code, w.Body, hits.Load())
	}
}
func TestPlaybackResolverNetworkPolicy(t *testing.T) {
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer upstream.Close()
	h := newHarness(t, "")
	h.a.cms = cmsClient(false)
	c := defaultPlaybackConfig()
	c.Enabled = true
	c.DirectFirst = false
	c.Sources = []playbackSource{resolverFixtureSource("private", upstream.URL)}
	got, _ := h.a.resolvePlayback(context.Background(), c, "https://source.test/watch", "mobile", nil)
	if got.OK || hits.Load() != 0 || len(got.Attempts) != 1 {
		t.Fatal("network policy ignored", got)
	}
}
