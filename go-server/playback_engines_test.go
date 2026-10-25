package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func engineFixtureBuild(p, os string) engineBuild {
	id := "android-system"
	if os == "ios" {
		id = "avplayer"
	}
	if os == "windows" {
		id = "libvlc"
	}
	if os == "web" {
		id = "html5"
	}
	return engineBuild{engineBuildContract, p, os, "test.hetun.client", "1.0.0", []installedEngine{{id, "test-1", "not_required"}}, []string{"engine-policy", "playback-report", "resume-position"}}
}
func engineFixtureConfig(p, os string) engineConfig {
	c := defaultEngineConfig()
	for i := range c.Profiles {
		profile := &c.Profiles[i]
		if profile.Platform == p && profile.OS == os {
			b := engineFixtureBuild(p, os)
			profile.Enabled = true
			profile.Build = &b
			profile.Primary = b.Engines[0].ID
		}
	}
	return c
}
func publishEngineFixture(t *testing.T, h *harness, c engineConfig) {
	t.Helper()
	s := playbackEngineState(h.a.readState())
	if w := h.req("PUT", "/api/admin/playback-engines", M{"revision": s.Revision, "config": c}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := h.req("POST", "/api/admin/playback-engines/publish", M{"revision": s.Revision + 1}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
}
func TestPlaybackEnginePersistenceAndAdminBoundary(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	if w := h.req("GET", "/api/admin/playback-engines", nil, ""); w.Code != 200 || number(obj(payload(t, w)["settings"])["revision"]) != 1 {
		t.Fatal(w.Code, w.Body)
	}
	cookie := h.cookie
	h.cookie = ""
	if w := h.req("GET", "/api/admin/playback-engines", nil, ""); w.Code != 401 {
		t.Fatal("admin auth", w.Code)
	}
	h.cookie = cookie
	c := engineFixtureConfig("mobile", "ios")
	if w := h.req("PUT", "/api/admin/playback-engines", M{"revision": 1, "config": c}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := h.req("PUT", "/api/admin/playback-engines", M{"revision": 1, "config": c}, ""); w.Code != 409 {
		t.Fatal("stale draft overwrote", w.Code)
	}
	p, _ := engineProfileFor(playbackEngineState(h.a.readState()).Published, "mobile", "ios")
	if p.Enabled {
		t.Fatal("draft went live")
	}
	if w := h.req("POST", "/api/admin/playback-engines/publish", M{"revision": 2}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	stored, e := h.a.store.Load(context.Background())
	if e != nil || stored.PlaybackEngines.PublishedVersion != 1 {
		t.Fatal("durability", e)
	}
	p, _ = engineProfileFor(stored.PlaybackEngines.Published, "mobile", "ios")
	if !p.Enabled || p.Primary != "avplayer" {
		t.Fatal(p)
	}
	req := httptest.NewRequest("PUT", h.origin+"/api/admin/playback-engines", strings.NewReader("{}"))
	req.AddCookie(&http.Cookie{Name: "hetun_session", Value: cookie})
	rr := httptest.NewRecorder()
	h.a.ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Fatal("trusted writer boundary", rr.Code)
	}
}
func TestPlaybackEngineValidation(t *testing.T) {
	base := engineFixtureConfig("mobile", "android")
	cases := map[string]func(*engineConfig){
		"missing target":   func(c *engineConfig) { c.Profiles = c.Profiles[1:] },
		"duplicate target": func(c *engineConfig) { c.Profiles[1] = c.Profiles[0] },
		"unsupported os":   func(c *engineConfig) { c.Profiles[0].OS = "ios" },
		"same engines":     func(c *engineConfig) { c.Profiles[0].Backup = c.Profiles[0].Primary },
		"budget":           func(c *engineConfig) { c.Profiles[0].MaxAttempts = 0 },
		"duration":         func(c *engineConfig) { c.Profiles[0].SessionSeconds = 301 },
		"conditions":       func(c *engineConfig) { c.Profiles[0].SwitchOn = []string{"unknown"} },
		"build scope":      func(c *engineConfig) { c.Profiles[0].Build.Platform = "tv" },
		"unknown engine":   func(c *engineConfig) { c.Profiles[0].Build.Engines[0].ID = "missing" },
		"duplicate engine": func(c *engineConfig) {
			c.Profiles[0].Build.Engines = append(c.Profiles[0].Build.Engines, c.Profiles[0].Build.Engines[0])
		},
		"fake free license": func(c *engineConfig) { c.Profiles[0].Build.Engines[0].License = "valid" },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			c := clone(base)
			edit(&c)
			if validateEngineConfig(c, false) == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
	for name, edit := range map[string]func(*engineConfig){
		"unregistered":         func(c *engineConfig) { c.Profiles[0].Build = nil },
		"uninstalled primary":  func(c *engineConfig) { c.Profiles[0].Primary = "media3" },
		"fallback unavailable": func(c *engineConfig) { c.Profiles[0].AutoSwitch = true },
		"manual unavailable":   func(c *engineConfig) { c.Profiles[0].ManualSwitch = true },
		"adapter missing":      func(c *engineConfig) { c.Profiles[0].Build.Features = []string{"engine-policy"} },
	} {
		t.Run(name, func(t *testing.T) {
			c := clone(base)
			edit(&c)
			if validateEngineConfig(c, false) != nil {
				t.Fatal("cannot save planning draft")
			}
			if validateEngineConfig(c, true) == nil {
				t.Fatal("unsafe publication accepted")
			}
		})
	}
	var c engineConfig
	if decodeEngineValue(M{"profiles": []any{}, "extra": true}, &c) == nil {
		t.Fatal("unknown fields")
	}
	if decodeEngineValue(nil, &c) == nil {
		t.Fatal("null config")
	}
}
func TestPlaybackEngineCommercialLicenseAndRuntimeIntersection(t *testing.T) {
	c := engineFixtureConfig("mobile", "android")
	p := &c.Profiles[0]
	p.Backup = "aliyun"
	p.AutoSwitch = true
	p.ManualSwitch = true
	p.Build.Engines = append(p.Build.Engines, installedEngine{"aliyun", "test-2", "unknown"})
	p.Build.Features = append(p.Build.Features, "engine-fallback", "manual-switch")
	if validateEngineConfig(c, true) == nil {
		t.Fatal("unlicensed commercial engine published")
	}
	p.Build.Engines[1].License = "valid"
	if e := validateEngineConfig(c, true); e != nil {
		t.Fatal(e)
	}
	v := initialState()
	s := playbackEngineState(v)
	s.Published = c
	s.PublishedVersion = 1
	v.PlaybackEngines = &s
	b := clone(*p.Build)
	b.Engines = b.Engines[:1]
	if _, e := negotiateEngine(v, "mobile", &b); e == nil {
		t.Fatal("runtime missing backup accepted")
	}
	s.Published.Profiles[0].AutoSwitch = false
	if _, e := negotiateEngine(v, "mobile", &b); e == nil {
		t.Fatal("manual switching silently lost its backup")
	}
	s.Published.Profiles[0].AutoSwitch = true
	b = clone(*p.Build)
	b.AppVersion = "2.0.0"
	if _, e := negotiateEngine(v, "mobile", &b); e == nil {
		t.Fatal("wrong build accepted")
	}
	b = clone(*p.Build)
	b.Features = []string{"engine-policy"}
	if _, e := negotiateEngine(v, "mobile", &b); e == nil {
		t.Fatal("absent runtime feature accepted")
	}
	b = clone(*p.Build)
	policy, e := negotiateEngine(v, "mobile", &b)
	if e != nil || policy["primary"] != "android-system" || policy["backup"] != "aliyun" {
		t.Fatal(policy, e)
	}
	if _, e := negotiateEngine(v, "tablet", &b); e == nil {
		t.Fatal("cross platform accepted")
	}
}
func TestPlaybackEngineSevenTargetsAndPublicRoutes(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	for _, target := range engineTargets {
		c := engineFixtureConfig(target.Platform, target.OS)
		publishEngineFixture(t, h, c)
		b := engineFixtureBuild(target.Platform, target.OS)
		path := "/api/v1/" + clientName(target.Platform)
		w := h.req("POST", path+"/playback/negotiate", M{"client": b}, "")
		if w.Code != 200 || payload(t, w)["status"] != "negotiated" {
			t.Fatal(target, w.Code, w.Body)
		}
		w = h.req("GET", path+"/playback/config?os="+target.OS, nil, "")
		if w.Code != 200 || payload(t, w)["enabled"] != true {
			t.Fatal(w.Code, w.Body)
		}
		w = h.req("GET", path+"/bootstrap?os="+target.OS+"&version=1.0.0", nil, "")
		if w.Code != 200 || number(obj(payload(t, w)["playback"])["version"]) != playbackEngineState(h.a.readState()).PublishedVersion {
			t.Fatal("bootstrap stale policy", w.Code, w.Body)
		}
	}
	if w := h.req("POST", "/api/v1/tablet/playback/negotiate?platform=mobile", M{"client": engineFixtureBuild("tablet", "ios")}, ""); w.Code != 400 {
		t.Fatal("scope conflict", w.Code)
	}
	if w := h.req("GET", "/api/v1/mobile/playback/config?os=windows", nil, ""); w.Code != 400 {
		t.Fatal("invalid OS", w.Code)
	}
}
func TestPlaybackEngineInspectIsReadOnlyAndTruthful(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	revision := h.a.readState().Revision
	w := h.req("POST", "/api/admin/playback-engines/inspect", M{"build": engineFixtureBuild("tablet", "ios")}, "")
	if w.Code != 200 || payload(t, w)["deviceVerified"] != false || h.a.readState().Revision != revision {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("POST", "/api/admin/playback-engines/inspect", M{"build": M{"contract": engineBuildContract, "supportsEverything": true}}, ""); w.Code != 400 {
		t.Fatal("invalid declaration", w.Code)
	}
}
func TestPlaybackEngineAuthorizedTicketsAndReports(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	publishEngineFixture(t, h, engineFixtureConfig("mobile", "ios"))
	b := engineFixtureBuild("mobile", "ios")
	input := M{"id": "1", "sid": 1, "nid": 1, "playbackClient": b}
	if w := h.req("POST", "/api/v1/mobile/playback", input, ""); w.Code == 200 || str(payload(t, w)["engineReportTicket"]) != "" {
		t.Fatal("ticket issued without CMS permission", w.Code)
	}
	login := h.req("POST", "/api/v1/mobile/member/login", M{"username": "viewer", "password": "correct"}, "")
	member := str(payload(t, login)["token"])
	playback := h.req("POST", "/api/v1/mobile/playback", input, member)
	if playback.Code != 200 {
		t.Fatal(playback.Code, playback.Body)
	}
	response := payload(t, playback)
	ticket := str(response["engineReportTicket"])
	if ticket == "" || obj(response["enginePolicy"])["primary"] != "avplayer" {
		t.Fatal(response)
	}
	report := M{"ticket": ticket, "engine": "avplayer", "outcome": "prepared", "reason": "", "license": "not_required"}
	if w := h.req("POST", "/api/v1/tv/playback/report", report, ""); w.Code != 401 {
		t.Fatal("cross platform report", w.Code)
	}
	invalid := clone(report)
	invalid["ticket"] = ticket + "bad"
	if w := h.req("POST", "/api/v1/mobile/playback/report", invalid, ""); w.Code != 401 {
		t.Fatal("forged ticket", w.Code)
	}
	invalid = clone(report)
	invalid["engine"] = "aliyun"
	if w := h.req("POST", "/api/v1/mobile/playback/report", invalid, ""); w.Code != 400 {
		t.Fatal("wrong engine", w.Code)
	}
	w := h.req("POST", "/api/v1/mobile/playback/report", report, "")
	if w.Code != 200 || payload(t, w)["deviceVerified"] != false {
		t.Fatal(w.Code, w.Body)
	}
	w = h.req("POST", "/api/v1/mobile/playback/report", report, "")
	if w.Code != 200 || len(playbackEngineState(h.a.readState()).Observations) != 1 {
		t.Fatal("retry not idempotent", w.Code)
	}
	invalid = clone(report)
	invalid["outcome"] = "failed"
	invalid["reason"] = "decode"
	if w = h.req("POST", "/api/v1/mobile/playback/report", invalid, ""); w.Code != 409 {
		t.Fatal("ticket reused", w.Code)
	}
	// Configuration CAS is independent of telemetry writes.
	s := playbackEngineState(h.a.readState())
	if w = h.req("PUT", "/api/admin/playback-engines", M{"revision": s.Revision, "config": s.Draft}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("POST", "/api/admin/playback-engines/publish", M{"revision": s.Revision + 1}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("POST", "/api/v1/mobile/playback/report", report, ""); w.Code != 409 {
		t.Fatal("stale policy feedback accepted", w.Code)
	}
	// Legacy apps continue to receive the authorized media address and an explicit legacy status.
	w = h.req("POST", "/api/v1/mobile/playback", M{"id": "1", "sid": 1, "nid": 1}, member)
	if w.Code != 200 || obj(payload(t, w)["enginePolicy"])["status"] != "legacy_client" {
		t.Fatal(w.Code, w.Body)
	}
}
func TestPlaybackEngineExpiredTicket(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	publishEngineFixture(t, h, engineFixtureConfig("mobile", "android"))
	b := engineFixtureBuild("mobile", "android")
	v := h.a.readState()
	policy, _ := negotiateEngine(v, "mobile", &b)
	valid := issueEngineReportTicket(v, "mobile", &b, policy)
	raw, _ := base64.RawURLEncoding.DecodeString(strings.Split(valid, ".")[0])
	var ticket engineReportTicket
	json.Unmarshal(raw, &ticket)
	ticket.Expires = time.Now().Add(-time.Minute).Unix()
	raw, _ = json.Marshal(ticket)
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	expired := encoded + "." + base64.RawURLEncoding.EncodeToString(engineTicketMAC(v, encoded))
	if w := h.req("POST", "/api/v1/mobile/playback/report", M{"ticket": expired, "engine": "android-system", "outcome": "prepared", "license": "not_required"}, ""); w.Code != 401 {
		t.Fatal("expired feedback accepted", w.Code)
	}
}

func TestPlaybackEngineCommercialReadyRequiresLicense(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	c := engineFixtureConfig("mobile", "android")
	c.Profiles[0].Primary = "aliyun"
	c.Profiles[0].Build.Engines = []installedEngine{{"aliyun", "fixture-1", "valid"}}
	publishEngineFixture(t, h, c)
	v := h.a.readState()
	b := c.Profiles[0].Build
	policy, e := negotiateEngine(v, "mobile", b)
	if e != nil {
		t.Fatal(e)
	}
	ticket := issueEngineReportTicket(v, "mobile", b, policy)
	for _, license := range []string{"unknown", "invalid"} {
		w := h.req("POST", "/api/v1/mobile/playback/report", M{"ticket": ticket, "engine": "aliyun", "outcome": "prepared", "license": license}, "")
		if w.Code != 400 {
			t.Fatal("unlicensed ready report accepted", license, w.Code)
		}
	}
}
