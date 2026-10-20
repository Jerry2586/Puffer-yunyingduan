package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func adFixture(slot, os string) adConfig {
	c := emptyAds()
	for i := range c.Slots {
		if c.Slots[i].ID == slot {
			c.Slots[i].Enabled = true
			c.Slots[i].CooldownSeconds = 0
			c.Slots[i].DailyCap = 2
		}
	}
	c.Creatives = []adCreative{{ID: "creative-test", Name: "Fixture", Kind: "house", Slot: slot, Status: "active", Media: "text", Text: "Real contract test", Destination: "none", OS: []string{os}, Priority: 10, Duration: 5, SkipAfter: 3}}
	return c
}
func adSavePublish(t *testing.T, h *harness, p string, c adConfig) {
	t.Helper()
	report := payload(t, h.req("GET", "/api/admin/ads?platform="+p, nil, ""))
	w := h.req("PUT", "/api/admin/ads?platform="+p, M{"version": report["version"], "config": c}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	saved := payload(t, w)
	w = h.req("POST", "/api/admin/ads/publish?platform="+p, M{"version": saved["version"]}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
}
func TestAdsDurableServingFrequencyEventsAndIsolation(t *testing.T) {
	h := newHarness(t, "")
	c := adFixture("splash", "android")
	adSavePublish(t, h, "mobile", c)
	if w := h.req("PUT", "/api/admin/ads?platform=mobile", M{"version": 1, "config": c}, ""); w.Code != 409 {
		t.Fatal("stale draft accepted", w.Code)
	}
	if w := h.req("GET", "/api/admin/ads", nil, ""); w.Code != 400 {
		t.Fatal("missing platform", w.Code)
	}
	old := h.cookie
	h.cookie = ""
	if w := h.req("GET", "/api/admin/ads?platform=mobile", nil, ""); w.Code != 401 {
		t.Fatal("admin auth", w.Code)
	}
	h.cookie = old
	input := M{"slot": "splash", "os": "android", "deviceId": "fixture-device-123456"}
	w := h.req("POST", "/api/ads/decision?platform=tablet", input, "")
	if payload(t, w)["fill"] != false {
		t.Fatal("cross-platform fill", w.Body)
	}
	w = h.req("POST", "/api/ads/decision?platform=mobile", input, "")
	out := payload(t, w)
	if w.Code != 200 || out["fill"] != true {
		t.Fatal(w.Code, out)
	}
	id := str(out["decisionId"])
	if w = h.req("POST", "/api/ads/events?platform=mobile", M{"decisionId": id, "type": "click"}, ""); w.Code != 409 {
		t.Fatal("click without impression", w.Code)
	}
	if w = h.req("POST", "/api/ads/events?platform=tv", M{"decisionId": id, "type": "impression"}, ""); w.Code != 404 {
		t.Fatal("cross-platform receipt", w.Code)
	}
	for _, kind := range []string{"impression", "impression", "click", "complete"} {
		if w = h.req("POST", "/api/ads/events?platform=mobile", M{"decisionId": id, "type": kind}, ""); w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	w = h.req("POST", "/api/ads/decision?platform=mobile", input, "")
	if payload(t, w)["fill"] != true {
		t.Fatal(w.Body)
	}
	w = h.req("POST", "/api/ads/decision?platform=mobile", input, "")
	if payload(t, w)["fill"] != false {
		t.Fatal("daily cap ignored", w.Body)
	}
	m := obj(payload(t, h.req("GET", "/api/admin/ads?platform=mobile", nil, ""))["today"])
	if number(m["requests"]) != 3 || number(m["fills"]) != 2 || number(m["impressions"]) != 1 || number(m["clicks"]) != 1 || number(m["rewards"]) != 0 {
		t.Fatal(m)
	}
	persisted, e := h.a.store.Load(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if readAds(persisted, "mobile").Days[time.Now().UTC().Format("2006-01-02")].Impressions != 1 {
		t.Fatal("event not durable")
	}
	restarted, e := newApp(context.Background(), h.a.config, &fileStore{filepath.Join(h.a.config.DataDir, "state.json")})
	if e != nil {
		t.Fatal(e)
	}
	if readAds(restarted.readState(), "mobile").PublishedVersion != 1 {
		t.Fatal("publication lost after restart")
	}
	for _, p := range []string{"tablet", "tv", "web"} {
		if readAds(restarted.readState(), p).PublishedVersion != 0 {
			t.Fatal("published into other platform")
		}
	}
}
func TestAdsConfigScheduleSDKConsentFallback(t *testing.T) {
	c := adFixture("splash", "android")
	future := c.Creatives[0]
	future.ID = "future"
	future.Priority = 100
	future.Start = time.Now().Add(time.Hour).Format(time.RFC3339)
	c.Creatives = append(c.Creatives, future)
	ch := adChannel{ID: "admob-android", Name: "AdMob", Provider: "admob", OS: "android", Enabled: true, AppID: "ca-app-pub-1234567890123456~1234567890", Units: map[string]string{"splash": "ca-app-pub-1234567890123456/1234567890"}}
	c.Channels = []adChannel{ch}
	c.Slots[0].Order = "sdk-first"
	if e := validateAdConfig(c, "mobile"); e != nil {
		t.Fatal(e)
	}
	v, _ := selectAd(c, "splash", "android", nil, false, time.Now())
	if str(v["creativeId"]) != "creative-test" {
		t.Fatal("schedule/consent ignored", v)
	}
	v, _ = selectAd(c, "splash", "android", []string{"admob"}, false, time.Now())
	if v["source"] != "sdk" {
		t.Fatal(v)
	}
	c.Slots[0].ExcludeVIP = true
	v, _ = selectAd(c, "splash", "android", []string{"admob"}, true, time.Now())
	if v != nil {
		t.Fatal("VIP exclusion ignored")
	}
	c.Slots[0].ExcludeVIP = false
	h := newHarness(t, "")
	adSavePublish(t, h, "mobile", c)
	input := M{"slot": "splash", "os": "android", "deviceId": "fixture-device-123456", "adapters": []string{"admob"}, "consent": true}
	out := payload(t, h.req("POST", "/api/ads/decision?platform=mobile", input, ""))
	if obj(out["ad"])["source"] != "sdk" {
		t.Fatal(out)
	}
	id := str(out["decisionId"])
	out = payload(t, h.req("POST", "/api/ads/events?platform=mobile", M{"decisionId": id, "type": "no_fill"}, ""))
	if obj(obj(out["fallback"])["ad"])["source"] != "custom" {
		t.Fatal(out)
	}
	c.Channels[0].Provider = "pangle"
	if e := validateAdConfig(c, "mobile"); e == nil {
		t.Fatal("unimplemented provider enabled")
	}
	c.Channels[0].Provider = "admob"
	if e := validateAdConfig(c, "tv"); e == nil {
		t.Fatal("unsupported SDK platform")
	}
	c = adFixture("splash", "android")
	c.Creatives[0].Media = "image"
	c.Creatives[0].Asset = "/api/assets/" + strings.Repeat("a", 64) + ".png"
	if e := h.a.validateAdAssets(c); e == nil {
		t.Fatal("missing asset accepted")
	}
}
func TestAdmobSSVSignatureDurableRewardAndReplay(t *testing.T) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	adKeys.Lock()
	adKeys.keys = map[string]*ecdsa.PublicKey{"123": &key.PublicKey}
	adKeys.fetched = time.Now()
	adKeys.Unlock()
	defer func() { adKeys.Lock(); adKeys.keys = nil; adKeys.fetched = time.Time{}; adKeys.Unlock() }()
	h := newHarness(t, "")
	at := time.Now()
	id := strings.Repeat("d", 64)
	user := strings.Repeat("u", 64)
	unit := "ca-app-pub-1234567890123456/1234567890"
	e = h.a.change(context.Background(), "test.reward.prepare", func(s *state) error {
		acc := ensureAds(s, "mobile")
		acc.Decisions[id] = &adDecision{ID: id, Slot: "rewarded", Source: "sdk", User: user, ChannelID: "admob-test", Unit: unit, Created: at, Expires: at.Add(24 * time.Hour), Flags: map[string]bool{}, RewardPoints: 10, RewardCap: 5, RewardCooldown: 30}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	q := url.Values{"ad_network": {"5450213213286189855"}, "ad_unit": {"1234567890"}, "custom_data": {id}, "reward_amount": {"1"}, "reward_item": {"Reward"}, "timestamp": {fmt.Sprint(at.UnixMilli())}, "transaction_id": {"transaction-test-123"}, "user_id": {user}}
	sign := func(raw string) string {
		digest := sha256.Sum256([]byte(raw))
		sig, e := ecdsa.SignASN1(rand.Reader, key, digest[:])
		if e != nil {
			t.Fatal(e)
		}
		return raw + "&signature=" + base64.RawURLEncoding.EncodeToString(sig) + "&key_id=123"
	}
	raw := sign(q.Encode())
	if _, e = verifyAdmobQuery(raw, &key.PublicKey); e != nil {
		t.Fatal(e)
	}
	if _, e = verifyAdmobQuery(strings.Replace(raw, "Reward", "Forged", 1), &key.PublicKey); e == nil {
		t.Fatal("forged callback accepted")
	}
	if _, e = verifyAdmobQuery(sign(q.Encode()+"&user_id="+user), &key.PublicKey); e == nil {
		t.Fatal("duplicate parameters accepted")
	}
	call := func(query string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, h.origin+"/api/ads/callback/admob?"+query, nil)
		w := httptest.NewRecorder()
		h.a.ServeHTTP(w, r)
		return w
	}
	if w := call(strings.Replace(raw, "Reward", "Forged", 1)); w.Code != 403 {
		t.Fatal(w.Code, w.Body)
	}
	if w := call(raw); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w := call(raw)
	if w.Code != 200 || payload(t, w)["duplicate"] != true {
		t.Fatal("retry not idempotent", w.Code, w.Body)
	}
	state, e := h.a.store.Load(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	acc := readAds(state, "mobile")
	if len(acc.Rewards) != 1 || acc.Rewards[0].Points != 10 || !acc.Decisions[id].Rewarded {
		t.Fatal("durable reward mismatch")
	}
	if len(readAds(state, "tablet").Rewards) != 0 {
		t.Fatal("reward crossed clients")
	}
	q.Set("transaction_id", "transaction-test-456")
	if w := call(sign(q.Encode())); w.Code != 409 {
		t.Fatal("second reward on same task", w.Code, w.Body)
	}
	q.Set("transaction_id", "transaction-test-123")
	q.Set("user_id", strings.Repeat("x", 64))
	if w := call(sign(q.Encode())); w.Code != 409 {
		t.Fatal("transaction user mismatch", w.Code, w.Body)
	}
}
