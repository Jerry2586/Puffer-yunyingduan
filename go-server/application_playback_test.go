package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func setApplicationFixture(t *testing.T, h *harness, update func(*applicationConfig)) {
	t.Helper()
	if err := h.a.change(context.Background(), "fixture.application", func(s *state) error {
		cfg := applicationState(*s)
		update(&cfg.Config)
		cfg.Version++
		s.ApplicationSettings = &cfg
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationSettingsCASSharedClientsAndValidation(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	cfg := defaultApplicationConfig()
	cfg.Application.SearchDelayMS = 850
	cfg.Application.ListLimit = 7
	cfg.Application.TVDownloadURL = "https://download.example/app.apk"
	cfg.History.Enabled = true
	w := h.req("POST", "/api/admin/application-settings", M{"version": 0, "config": cfg}, "")
	if w.Code != 200 || number(payload(t, w)["version"]) != 1 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	for _, client := range []string{"mobile", "tablet", "tv", "desktop"} {
		out := payload(t, h.req("GET", "/api/v1/"+client+"/application/config", nil, ""))
		if number(out["version"]) != 1 || number(out["listLimit"]) != 7 || number(out["searchDelayMs"]) != 850 || len(str(out["sourceKey"])) != 24 {
			t.Fatalf("%s shared config: %v", client, out)
		}
		if (str(out["tvDownloadUrl"]) != "") != (client == "tv") {
			t.Fatalf("TV address leaked to %s", client)
		}
	}
	w = h.req("POST", "/api/admin/application-settings", M{"version": 0, "config": cfg}, "")
	if w.Code != 409 || str(payload(t, w)["code"]) != "APPLICATION_SETTINGS_CHANGED" {
		t.Fatalf("stale write: %d %s", w.Code, w.Body)
	}
	raw, _ := json.Marshal(cfg)
	unknown := M{}
	_ = json.Unmarshal(raw, &unknown)
	unknown["unrecognized"] = true
	for _, input := range []M{{"version": 1.5, "config": cfg}, {"version": 1, "config": unknown}} {
		if w = h.req("POST", "/api/admin/application-settings", input, ""); w.Code != 400 {
			t.Fatalf("invalid config accepted: %d", w.Code)
		}
	}
	cfg.Registration.Managed = true
	if w = h.req("POST", "/api/admin/application-settings", M{"version": 1, "config": cfg}, ""); w.Code != 409 {
		t.Fatalf("managed registration without adapter: %d", w.Code)
	}
	if applicationState(h.a.readState()).Version != 1 {
		t.Fatal("rejected writes changed version")
	}
	h.cookie = ""
	for _, route := range []string{"/api/admin/application-settings", "/api/admin/playback-center"} {
		if w = h.req("GET", route, nil, ""); w.Code != 401 {
			t.Fatalf("admin access %s: %d", route, w.Code)
		}
	}
}

func TestApplicationConfigRejectsUnsafeAndOutOfRangeValues(t *testing.T) {
	cases := []struct {
		name string
		edit func(*applicationConfig)
	}{
		{"list zero", func(c *applicationConfig) { c.Application.ListLimit = 0 }},
		{"search delay", func(c *applicationConfig) { c.Application.SearchDelayMS = 3001 }},
		{"download protocol", func(c *applicationConfig) { c.Application.TVDownloadURL = "javascript:alert(1)" }},
		{"registration limit", func(c *applicationConfig) { c.Registration.DailyIPLimit = -1 }},
		{"history interval", func(c *applicationConfig) { c.History.IntervalSeconds = 9 }},
		{"history capacity", func(c *applicationConfig) { c.History.Limit = 1001 }},
		{"avatar scheme", func(c *applicationConfig) { c.Avatar.Library = []string{"data:image/svg+xml,x"} }},
		{"administrator group", func(c *applicationConfig) {
			c.Membership.Managed = true
			c.Membership.GroupID = 1
			c.Membership.Month = 10
		}},
		{"empty paid plan", func(c *applicationConfig) { c.Membership.Managed = true; c.Membership.GroupID = 3 }},
		{"empty pinned comment", func(c *applicationConfig) { c.Comments.PinnedEnabled = true }},
		{"contact credentials", func(c *applicationConfig) { c.Share.ContactURL = "https://user:secret@example.com/" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := defaultApplicationConfig()
			tc.edit(&c)
			if validateApplicationConfig(c) == nil {
				t.Fatal("unsafe config accepted")
			}
		})
	}
	c := defaultApplicationConfig()
	c.Membership.Managed = true
	c.Membership.GroupID = 3
	c.Membership.Month = 123
	if err := validateApplicationConfig(c); err != nil {
		t.Fatal(err)
	}
	v := state{ApplicationSettings: &applicationSettings{Config: c}}
	if membershipPrice(v, "month") != 123 || membershipPrice(v, "unknown") != 0 {
		t.Fatal("membership pricing not server controlled")
	}
}

func TestApplicationCaptchaSingleUsePurposeIPAndExpiry(t *testing.T) {
	h := newHarness(t, "")
	v := h.a.readState()
	c := defaultApplicationConfig()
	c.Captcha.Login = true
	c.Captcha.Comments = true
	v.ApplicationSettings = &applicationSettings{Config: c}
	r := httptest.NewRequest("POST", h.origin+"/api/member/login", nil)
	r.RemoteAddr = "192.0.2.1:12345"
	cases := []struct {
		name, action, ip, answer string
		until                    time.Time
		wantOK                   bool
	}{
		{"valid", "login", "192.0.2.1", "12345", time.Now().Add(time.Minute), true},
		{"wrong purpose", "comments", "192.0.2.1", "12345", time.Now().Add(time.Minute), false},
		{"wrong ip", "login", "192.0.2.2", "12345", time.Now().Add(time.Minute), false},
		{"wrong answer", "login", "192.0.2.1", "54321", time.Now().Add(time.Minute), false},
		{"expired", "login", "192.0.2.1", "12345", time.Now().Add(-time.Minute), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h.a.captchas = map[string]captchaChallenge{"test": {Digest: sha256.Sum256([]byte("12345")), Action: tc.action, IP: tc.ip, Until: tc.until}}
			data := M{"captchaToken": "test", "captchaAnswer": tc.answer}
			err := h.a.verifyApplicationCaptcha(r, v, "login", data)
			if (err == nil) != tc.wantOK {
				t.Fatalf("validation result: %v", err)
			}
			if h.a.verifyApplicationCaptcha(r, v, "login", data) == nil {
				t.Fatal("challenge reused")
			}
		})
	}
	w := httptest.NewRecorder()
	r = httptest.NewRequest("GET", h.origin+"/api/application/captcha?action=login", nil)
	if err := h.a.newApplicationCaptcha(w, r, v); err != nil {
		t.Fatal(err)
	}
	out := payload(t, w)
	image := strings.TrimPrefix(str(out["image"]), "data:image/png;base64,")
	b, err := base64.StdEncoding.DecodeString(image)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = png.DecodeConfig(strings.NewReader(string(b))); err != nil {
		t.Fatalf("captcha is not PNG: %v", err)
	}
	if str(out["token"]) == "" || out["required"] != true || out["answer"] != nil {
		t.Fatal("invalid public challenge contract")
	}
}

func TestRegistrationReservationIsAtomicDurableAndSiteScoped(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	setApplicationFixture(t, h, func(c *applicationConfig) { c.Registration.DailyIPLimit = 1 })
	v := h.a.readState()
	r := httptest.NewRequest("POST", h.origin+"/api/member/register", nil)
	r.RemoteAddr = "192.0.2.5:9999"
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := h.a.reserveRegistration(r, v); results <- err }()
	}
	wg.Wait()
	close(results)
	passed := 0
	for err := range results {
		if err == nil {
			passed++
		} else if e, ok := err.(*apiError); !ok || e.Status != 429 {
			t.Fatal(err)
		}
	}
	if passed != 1 || len(h.a.readState().Registrations) != 1 {
		t.Fatalf("daily cap passed %d", passed)
	}
	persisted, err := h.a.store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.Registrations) != 1 {
		t.Fatal("registration reservation not persisted")
	}
	code := invitationCode(v, 7)
	if invitationUser(v, code) != 7 || invitationUser(v, code+"x") != 0 {
		t.Fatal("invitation signature not checked")
	}
	v.CMSBase = cms.URL + "/other"
	if invitationUser(v, code) != 0 {
		t.Fatal("invitation crossed sites")
	}
	if err = h.a.change(context.Background(), "fixture.source", func(s *state) error { s.CMSBase = v.CMSBase; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = h.a.reserveRegistration(r, v); err != nil {
		t.Fatalf("different site shared cap: %v", err)
	}
}

func TestCloudHistoryUsesVerifiedMetadataAndSiteIdentity(t *testing.T) {
	h, token := communityFixture(t)
	setApplicationFixture(t, h, func(c *applicationConfig) { c.History.Enabled = true })
	if w := h.req("GET", "/api/member/history", nil, ""); w.Code != 401 {
		t.Fatalf("anonymous history: %d", w.Code)
	}
	w := h.req("POST", "/api/member/history", M{"videoId": "1", "episode": 1, "position": 12345, "title": "forged", "poster": "https://evil.test/", "userId": 999}, token)
	if w.Code != 200 {
		t.Fatalf("history: %d %s", w.Code, w.Body)
	}
	entry := obj(payload(t, w)["entry"])
	if str(entry["title"]) != "一" || strings.Contains(str(entry["poster"]), "evil.test") {
		t.Fatalf("untrusted metadata persisted: %v", entry)
	}
	for _, input := range []M{{"videoId": "1", "episode": 2, "position": 0}, {"videoId": "999", "episode": 1, "position": 0}, {"videoId": "1", "episode": 1.5, "position": 0}, {"videoId": "1", "episode": 1, "position": -1}} {
		if w = h.req("POST", "/api/member/history", input, token); w.Code < 400 || w.Code >= 500 {
			t.Fatalf("invalid history accepted: %d %s", w.Code, w.Body)
		}
	}
	w = h.req("POST", "/api/member/history", M{"videoId": "1", "episode": 1, "position": 0}, token)
	if w.Code != 200 {
		t.Fatalf("completion: %d", w.Code)
	}
	items := arr(payload(t, h.req("GET", "/api/v1/tablet/member/history", nil, token))["items"])
	if len(items) != 1 || number(obj(items[0])["position"]) != 0 {
		t.Fatal("history not deduplicated/shared")
	}
	if err := h.a.change(context.Background(), "fixture.other.history", func(s *state) error {
		s.MemberHistories = append([]memberHistory{{Base: managementBase(s.CMSBase), UserID: 999, Items: []historyEntry{{VideoID: "2", Title: "other account"}}}, {Base: managementBase(s.CMSBase) + "/other", UserID: 7, Items: []historyEntry{{VideoID: "3", Title: "other site"}}}}, s.MemberHistories...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	items = arr(payload(t, h.req("GET", "/api/member/history", nil, token))["items"])
	if len(items) != 1 || str(obj(items[0])["videoId"]) != "1" {
		t.Fatalf("history identity leak: %v", items)
	}
	setApplicationFixture(t, h, func(c *applicationConfig) { c.History.Enabled = false })
	if w = h.req("GET", "/api/member/history", nil, token); w.Code != 403 {
		t.Fatalf("disabled history: %d", w.Code)
	}
}

func TestAvatarRejectsArbitraryURLAndInvalidImages(t *testing.T) {
	h, token := communityFixture(t)
	setApplicationFixture(t, h, func(c *applicationConfig) {
		c.Avatar.Upload = true
		c.Avatar.Library = []string{"https://images.test/approved.png"}
	})
	for _, input := range []M{{"url": "https://evil.test/avatar.png"}, {"data": base64.StdEncoding.EncodeToString([]byte("<svg/>"))}, {"data": base64.StdEncoding.EncodeToString(make([]byte, 1024*1024+1))}} {
		w := h.req("POST", "/api/member/avatar", input, token)
		if w.Code != 400 {
			t.Fatalf("invalid avatar: %d %s", w.Code, w.Body)
		}
	}
}

func TestPlaybackPreferencesCASAndStableSourceIDs(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	cfg := playbackPreferencesFor(h.a.readState())
	cfg.Sources = []cmsPlaybackSource{{Code: "b", Name: "备用", Enabled: true, TrialSeconds: 90, Mode: "direct"}, {Code: "a", Name: "主线", Enabled: true, Mode: "auto"}, {Code: "c", Name: "关闭", Enabled: false, Mode: "auto"}}
	raw, _ := json.Marshal(cfg)
	input := M{}
	_ = json.Unmarshal(raw, &input)
	if w := h.req("POST", "/api/admin/playback-center", input, ""); w.Code != 200 {
		t.Fatalf("save sources: %d %s", w.Code, w.Body)
	}
	if w := h.req("POST", "/api/admin/playback-center", input, ""); w.Code != 409 {
		t.Fatalf("source stale write: %d", w.Code)
	}
	input["version"] = 1
	input["source"] = cms.URL + "/other"
	if w := h.req("POST", "/api/admin/playback-center", input, ""); w.Code != 409 {
		t.Fatalf("source mismatch: %d", w.Code)
	}
	result := M{"items": []any{M{"sources": []any{M{"code": "a", "name": "old A", "sid": 1}, M{"code": "b", "name": "old B", "sid": 2}, M{"code": "c", "name": "old C", "sid": 3}}}}}
	applyPlaybackSources(h.a.readState(), result)
	sources := arr(obj(arr(result["items"])[0])["sources"])
	if len(sources) != 2 || number(obj(sources[0])["sid"]) != 2 || str(obj(sources[0])["name"]) != "备用" || number(obj(sources[0])["trialSeconds"]) != 90 || number(obj(sources[1])["sid"]) != 1 {
		t.Fatalf("source IDs changed during reorder: %v", sources)
	}
	v := h.a.readState()
	v.CMSBase = cms.URL + "/new"
	if len(playbackPreferencesFor(v).Sources) != 0 {
		t.Fatal("source rules crossed sites")
	}
}

func TestPlaybackSourcePolicyChecksRealLineEpisodeAndTrial(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	v := h.a.readState()
	c := playbackPreferencesFor(v)
	c.Sources = []cmsPlaybackSource{{Code: "线路", Name: "试看片源", Enabled: true, TrialSeconds: 60, Mode: "direct"}}
	v.PlaybackPreferences = &c
	code, trial, mode, err := h.a.playbackSourcePolicy(context.Background(), v, "1", 1, 1)
	if err != nil || code != "线路" || trial != 60 || mode != "direct" {
		t.Fatalf("source policy %s %d %s %v", code, trial, mode, err)
	}
	for _, pair := range [][2]int{{1, 2}, {2, 1}} {
		if _, _, _, err = h.a.playbackSourcePolicy(context.Background(), v, "1", pair[0], pair[1]); err == nil {
			t.Fatal("nonexistent line/episode accepted")
		}
	}
	c.Sources[0].Enabled = false
	if _, _, _, err = h.a.playbackSourcePolicy(context.Background(), v, "1", 1, 1); err == nil {
		t.Fatal("disabled source played")
	}
}

func TestThirdPartyDanmuBoundsQueryAndPublicPrivacy(t *testing.T) {
	h := newHarness(t, "")
	body := `{"items":[{"content":"hello","position":1500}]}`
	var got url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer upstream.Close()
	c := danmuPreferences{Enabled: true, ThirdParty: true, Endpoint: upstream.URL, Match: "title", TVDensity: 35}
	rows, err := h.a.fetchThirdPartyDanmu(context.Background(), c, "1", "名侦探", 2)
	if err != nil || len(rows) != 1 || got.Get("title") != "名侦探" || got.Get("id") != "" || got.Get("episode") != "2" || number(rows[0]["position"]) != 1500 {
		t.Fatalf("danmu contract: %v %v %v", rows, got, err)
	}
	for _, invalid := range []string{`{"items":[{"content":"x","position":-1}]}`, `{"items":[{"content":"x","position":1.5}]}`, `{"items":null}`, `{"items":[` + strings.Repeat(`{"content":"x","position":0},`, 1000) + `{"content":"x","position":0}]}`, strings.Repeat("x", 1024*1024+1)} {
		body = invalid
		if _, err = h.a.fetchThirdPartyDanmu(context.Background(), c, "1", "name", 1); err == nil {
			t.Fatal("invalid upstream danmu accepted")
		}
	}
	v := h.a.readState()
	p := playbackPreferencesFor(v)
	p.Danmu = c
	v.PlaybackPreferences = &p
	for _, platform := range []string{"mobile", "tv"} {
		out := publicDanmuPreferences(v, platform)
		if out["endpoint"] != nil || out["preferredDomain"] != nil {
			t.Fatal("private upstream settings leaked")
		}
		want := 100
		if platform == "tv" {
			want = 35
		}
		if number(out["density"]) != want {
			t.Fatal("TV density leaked into another client")
		}
	}
	p.Danmu.Endpoint = upstream.URL + "?secret=value"
	if validatePlaybackPreferences(p) == nil {
		t.Fatal("query-bearing upstream URL accepted")
	}
}

func TestApplicationListLimitRemapsAcrossCMSPages(t *testing.T) {
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ac") == "list" {
			jsonResponse(w, 200, M{"code": 1, "class": []any{M{"type_id": 1, "type_name": "电影"}}, "list": []any{}})
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("pg"))
		if page < 1 {
			page = 1
		}
		list := []any{}
		for id := (page-1)*3 + 1; id <= min(page*3, 8); id++ {
			list = append(list, M{"vod_id": id, "vod_name": fmt.Sprint(id)})
		}
		jsonResponse(w, 200, M{"code": 1, "page": page, "pagecount": 3, "total": 8, "limit": 3, "list": list})
	}))
	defer cms.Close()
	h := newHarness(t, cms.URL)
	ctx := context.WithValue(context.Background(), catalogLimitKey{}, 2)
	seen := []string{}
	for page := 1; page <= 4; page++ {
		out, err := h.a.catalog(ctx, cms.URL, url.Values{"page": {strconv.Itoa(page)}})
		if err != nil {
			t.Fatal(err)
		}
		if number(out["page"]) != page || number(out["pages"]) != 4 || number(out["total"]) != 8 {
			t.Fatalf("pagination: %v", out)
		}
		for _, item := range arr(out["items"]) {
			seen = append(seen, str(obj(item)["id"]))
		}
	}
	if !reflect.DeepEqual(seen, []string{"1", "2", "3", "4", "5", "6", "7", "8"}) {
		t.Fatalf("page gaps/duplicates: %v", seen)
	}
}

func TestScheduledSearchSkipsRunningJobsAndFreshSnapshots(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	setApplicationFixture(t, h, func(c *applicationConfig) { c.Search.IntervalMinutes = 30 })
	at := time.Now()
	source := h.a.searchSource(h.a.readState().CMSBase)
	running := &searchJob{Source: source, Phase: "running"}
	recent := &searchJob{Source: source, Phase: "succeeded", FinishedAt: at.UTC().Format(time.RFC3339)}
	h.a.searchJobs = map[string]*searchJob{"mobile": running, "tablet": recent, "tv": recent}
	current := &searchIndex{Source: source, BuiltAt: at}
	h.a.searchIndexes = map[string]*searchIndex{"web": current}
	h.a.scheduleSearch(at)
	if h.a.searchJobs["mobile"] != running || h.a.searchJobs["tablet"] != recent || h.a.searchJobs["tv"] != recent || h.a.searchJobs["web"] != nil || h.a.searchIndexes["web"] != current {
		t.Fatal("scheduler duplicated current work")
	}
}

func TestScheduledSearchReplacesJobsFromPreviousSource(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	setApplicationFixture(t, h, func(c *applicationConfig) { c.Search.IntervalMinutes = 30 })
	source := h.a.searchSource(h.a.readState().CMSBase)
	old := &searchJob{Source: "old-source", Phase: "running"}
	recent := &searchJob{Source: "old-source", Phase: "succeeded", FinishedAt: now()}
	h.a.searchJobs = map[string]*searchJob{"mobile": old, "tablet": recent, "tv": recent, "web": recent}
	if status := h.a.searchStatus("mobile"); status["phase"] != "idle" {
		t.Fatal("old source status exposed")
	}
	h.a.scheduleSearch(time.Now())
	deadline := time.Now().Add(3 * time.Second)
	for {
		complete := true
		h.a.searchMu.Lock()
		for _, p := range platforms {
			job := h.a.searchJobs[p]
			if job == nil || job == old || job == recent || job.Source != source {
				h.a.searchMu.Unlock()
				t.Fatal("old source job blocked new source")
			}
			if job.Phase == "running" {
				complete = false
			}
		}
		h.a.searchMu.Unlock()
		if complete {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scheduled jobs did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRetiredSearchJobCannotOverwriteNewSnapshot(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	gate := make(chan struct{})
	var once sync.Once
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ac") == "list" {
			jsonResponse(w, 200, M{"code": 1, "class": []any{M{"type_id": 1, "type_name": "电影"}}, "list": []any{}})
			return
		}
		select {
		case <-gate:
			once.Do(func() { close(entered) })
			<-release
		default:
		}
		jsonResponse(w, 200, M{"code": 1, "page": 1, "pagecount": 1, "total": 0, "list": []any{}})
	}))
	defer cms.Close()
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	h := newHarness(t, cms.URL)
	close(gate)
	base := h.a.readState().CMSBase
	source := h.a.searchSource(base)
	old := &searchJob{Source: source, Phase: "running"}
	h.a.searchJobs = map[string]*searchJob{"mobile": old}
	done := make(chan struct{})
	go func() { h.a.rebuildSearch("mobile", base, source, old); close(done) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("old job did not start")
	}
	replacement := &searchJob{Source: "new-source", Phase: "succeeded"}
	snapshot := &searchIndex{Source: "new-source", BuiltAt: time.Now()}
	h.a.searchMu.Lock()
	h.a.searchJobs["mobile"] = replacement
	h.a.searchIndexes = map[string]*searchIndex{"mobile": snapshot}
	h.a.searchMu.Unlock()
	unblock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("old job did not stop")
	}
	h.a.searchMu.Lock()
	defer h.a.searchMu.Unlock()
	if h.a.searchJobs["mobile"] != replacement || h.a.searchIndexes["mobile"] != snapshot || old.Phase != "running" {
		t.Fatal("retired job overwrote current state")
	}
}
