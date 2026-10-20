package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"time"
)

type adMetrics struct {
	Requests    int64 `json:"requests"`
	Fills       int64 `json:"fills"`
	Impressions int64 `json:"impressions"`
	Clicks      int64 `json:"clicks"`
	Completes   int64 `json:"completes"`
	Errors      int64 `json:"errors"`
	Rewards     int64 `json:"rewards"`
	Points      int64 `json:"points"`
}

func (m *adMetrics) add(x adMetrics) {
	m.Requests += x.Requests
	m.Fills += x.Fills
	m.Impressions += x.Impressions
	m.Clicks += x.Clicks
	m.Completes += x.Completes
	m.Errors += x.Errors
	m.Rewards += x.Rewards
	m.Points += x.Points
}

type adFrequency struct {
	Day      string    `json:"day"`
	Requests int       `json:"requests"`
	Served   int       `json:"served"`
	Last     time.Time `json:"last"`
}
type adDecision struct {
	ID             string          `json:"id"`
	Device         string          `json:"device"`
	Slot           string          `json:"slot"`
	OS             string          `json:"os"`
	Source         string          `json:"source"`
	CreativeID     string          `json:"creativeId"`
	ChannelID      string          `json:"channelId"`
	Unit           string          `json:"unit"`
	User           string          `json:"user"`
	Created        time.Time       `json:"created"`
	Expires        time.Time       `json:"expires"`
	Flags          map[string]bool `json:"flags"`
	RewardPoints   int             `json:"rewardPoints"`
	RewardCap      int             `json:"rewardCap"`
	RewardCooldown int             `json:"rewardCooldown"`
	Rewarded       bool            `json:"rewarded"`
	VIP            bool            `json:"vip"`
	FallbackUsed   bool            `json:"fallbackUsed"`
}
type adEvent struct {
	At         string `json:"at"`
	Decision   string `json:"decision"`
	Slot       string `json:"slot"`
	Source     string `json:"source"`
	CreativeID string `json:"creativeId"`
	Type       string `json:"type"`
	Code       string `json:"code"`
}
type adReward struct {
	ID          string `json:"id"`
	Transaction string `json:"transaction"`
	User        string `json:"user"`
	Decision    string `json:"decision"`
	Channel     string `json:"channel"`
	Points      int    `json:"points"`
	At          string `json:"at"`
}

func adDay(acc *adAccount, t time.Time) *adMetrics {
	key := t.UTC().Format("2006-01-02")
	if acc.Days[key] == nil {
		acc.Days[key] = &adMetrics{}
	}
	return acc.Days[key]
}
func adPrune(acc *adAccount, t time.Time) {
	for k, d := range acc.Decisions {
		if t.After(d.Expires) {
			delete(acc.Decisions, k)
		}
	}
	cutoff := t.UTC().AddDate(0, 0, -1).Format("2006-01-02")
	for k, f := range acc.Frequency {
		if f.Day < cutoff {
			delete(acc.Frequency, k)
		}
	}
	cutoff = t.UTC().AddDate(0, 0, -89).Format("2006-01-02")
	for k := range acc.Days {
		if k < cutoff {
			delete(acc.Days, k)
		}
	}
}
func pushAdEvent(acc *adAccount, d *adDecision, kind, code string) {
	acc.Events = append([]adEvent{{At: now(), Decision: d.ID[:12], Slot: d.Slot, Source: d.Source, CreativeID: d.CreativeID, Type: kind, Code: code}}, acc.Events...)
	if len(acc.Events) > 500 {
		acc.Events = acc.Events[:500]
	}
}

// Select only contextual placements. Geometry is supplied by the installed theme/native client.
func selectAd(c adConfig, slot, os string, adapters []string, vip bool, t time.Time) (M, string) {
	var s adSlot
	for _, x := range c.Slots {
		if x.ID == slot {
			s = x
			break
		}
	}
	if !s.Enabled {
		return nil, "广告位已关闭"
	}
	if s.ExcludeVIP && vip {
		return nil, "会员免广告"
	}
	candidates := []adCreative{}
	for _, x := range c.Creatives {
		if x.Status != "active" || x.Slot != slot || !has(x.OS, os) {
			continue
		}
		if x.Start != "" {
			st, _ := time.Parse(time.RFC3339, x.Start)
			if t.Before(st) {
				continue
			}
		}
		if x.End != "" {
			end, _ := time.Parse(time.RFC3339, x.End)
			if !t.Before(end) {
				continue
			}
		}
		candidates = append(candidates, x)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].Priority > candidates[j].Priority
	})
	var custom M
	if len(candidates) > 0 {
		x := candidates[0]
		custom = M{"source": "custom", "creativeId": x.ID, "name": x.Name, "media": x.Media, "asset": x.Asset, "text": x.Text, "destination": x.Destination, "url": x.URL, "duration": x.Duration, "skipAfter": x.SkipAfter}
	}
	var sdk M
	for _, ch := range c.Channels {
		if ch.Enabled && ch.Provider == "admob" && ch.OS == os && ch.Units[slot] != "" && has(adapters, ch.Provider) {
			sdk = M{"source": "sdk", "provider": ch.Provider, "channelId": ch.ID, "appId": ch.AppID, "unitId": ch.Units[slot]}
			break
		}
	}
	if slot == "rewarded" && !c.Reward.Enabled {
		return nil, "激励奖励已关闭"
	}
	switch s.Order {
	case "custom-only":
		sdk = nil
	case "sdk-only":
		custom = nil
	case "sdk-first":
		if sdk != nil {
			return sdk, "渠道优先"
		}
	}
	if custom != nil {
		return custom, "自定义广告匹配"
	}
	if sdk != nil {
		return sdk, "SDK渠道匹配"
	}
	return nil, "无符合条件的广告；SDK需客户端适配器与同意状态"
}
func (a *app) adHash(value string) string {
	h := sha256.New()
	h.Write(a.serviceKey.D.Bytes())
	h.Write([]byte("ads-v1:" + value))
	return hex.EncodeToString(h.Sum(nil))
}
func (a *app) adIdentity(r *http.Request, p string) (string, bool, error) {
	if r.Header.Get("Authorization") == "" {
		return "", false, nil
	}
	s := a.readState()
	if s.CMSBase == "" {
		return "", false, problem(409, "会员内容源尚未配置")
	}
	session, e := a.memberSession(r, s.CMSBase, false)
	if e != nil {
		return "", false, e
	}
	profile, e := a.profile(r.Context(), session)
	if e != nil {
		return "", false, e
	}
	id := str(profile["user_id"])
	if id == "" || id == "0" {
		return "", false, problem(401, "会员接口未返回用户ID")
	}
	end := str(profile["user_end_time"])
	isVIP := false
	if n := number(profile["user_end_time"]); n > 0 {
		isVIP = time.Unix(int64(n), 0).After(time.Now())
	} else if t, e := time.Parse("2006-01-02 15:04:05", end); e == nil {
		isVIP = t.After(time.Now())
	}
	return a.adHash(s.CMSBase + ":" + id), isVIP, nil
}
func (a *app) publicAds(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/api/ads/callback/admob" {
		return a.admobCallback(w, r)
	}
	p, e := adPlatform(r)
	if e != nil {
		return e
	}
	if r.URL.Path == "/api/ads/config" && r.Method == "GET" {
		acc := readAds(a.readState(), p)
		jsonResponse(w, 200, M{"contract": adContract, "platform": p, "version": acc.PublishedVersion, "slots": acc.Published.Slots, "decisionUrl": "/api/ads/decision?platform=" + p, "eventUrl": "/api/ads/events?platform=" + p, "rewardBalanceUrl": "/api/ads/balance?platform=" + p})
		return nil
	}
	if r.URL.Path == "/api/ads/balance" && r.Method == "GET" {
		user, _, e := a.adIdentity(r, p)
		if e != nil {
			return e
		}
		if user == "" {
			return problem(401, "请先登录影视会员")
		}
		acc := readAds(a.readState(), p)
		points := 0
		entries := []M{}
		for _, rw := range acc.Rewards {
			if rw.User == user {
				points += rw.Points
				entries = append(entries, M{"id": rw.ID, "points": rw.Points, "at": rw.At})
			}
		}
		jsonResponse(w, 200, M{"ledger": "operations-points", "platform": p, "balance": points, "entries": entries})
		return nil
	}
	if r.URL.Path == "/api/ads/decision" && r.Method == "POST" {
		return a.adServe(w, r, p)
	}
	if r.URL.Path == "/api/ads/events" && r.Method == "POST" {
		return a.adTrack(w, r, p)
	}
	return problem(405, "广告接口方法或路径不支持")
}
func (a *app) adServe(w http.ResponseWriter, r *http.Request, p string) error {
	in, e := readBody(w, r, 8192)
	if e != nil {
		return e
	}
	slot, os, device := str(in["slot"]), str(in["os"]), str(in["deviceId"])
	if !has(adSlots, slot) || !adOS(p, os) || !idPattern.MatchString(device) || len(device) < 16 {
		return problem(400, "广告位、系统或设备标识无效（设备标识16–64字符）")
	}
	adapters := []string{}
	if in["consent"] == true {
		for _, v := range arr(in["adapters"]) {
			if str(v) == "admob" {
				adapters = append(adapters, "admob")
			}
		}
	}
	user, vip, e := a.adIdentity(r, p)
	if e != nil {
		return e
	}
	if slot == "rewarded" && user == "" {
		return problem(401, "激励奖励必须登录真实会员")
	}
	t := time.Now()
	deviceHash := a.adHash(device)
	result := M{}
	e = a.change(r.Context(), "ads."+p+".decision", func(s *state) error {
		acc := ensureAds(s, p)
		adPrune(acc, t)
		if len(acc.Decisions) >= 5000 || len(acc.Frequency) >= 10000 {
			return problem(429, "当前广告账本已达容量，请稍后重试")
		}
		c := acc.Published
		day := t.UTC().Format("2006-01-02")
		fk := deviceHash + ":" + slot + ":" + day
		f := acc.Frequency[fk]
		if f == nil {
			f = &adFrequency{Day: day}
			acc.Frequency[fk] = f
		}
		if f.Requests >= 5000 {
			return problem(429, "该设备当日请求过多")
		}
		f.Requests++
		adDay(acc, t).Requests++
		var rule adSlot
		for _, x := range c.Slots {
			if x.ID == slot {
				rule = x
			}
		}
		reason := ""
		if f.Served >= rule.DailyCap {
			reason = "当日展示次数已达上限"
		} else if !f.Last.IsZero() && t.Sub(f.Last) < time.Duration(rule.CooldownSeconds)*time.Second {
			reason = "广告冷却中"
		} else if rule.Every > 1 && (f.Requests-1)%rule.Every != 0 {
			reason = "未达到投放间隔次数"
		}
		var selected M
		if reason == "" {
			selected, reason = selectAd(c, slot, os, adapters, vip, t)
		}
		if selected == nil {
			result = M{"contract": adContract, "fill": false, "reason": reason, "configVersion": acc.PublishedVersion}
			return nil
		}
		if slot == "rewarded" {
			count, last := rewardUsage(acc, user, t)
			if count >= c.Reward.DailyCap || (!last.IsZero() && t.Sub(last) < time.Duration(c.Reward.CooldownSeconds)*time.Second) {
				result = M{"fill": false, "reason": "奖励次数或冷却限制"}
				return nil
			}
			for _, d := range acc.Decisions {
				if d.User == user && d.Slot == "rewarded" && !d.Rewarded && t.Sub(d.Created) < 30*time.Minute {
					return problem(409, "已有待验证奖励任务，请先完成或等待30分钟")
				}
			}
		}
		d := &adDecision{ID: randomToken(), VIP: vip, Device: deviceHash, Slot: slot, OS: os, Source: str(selected["source"]), CreativeID: str(selected["creativeId"]), ChannelID: str(selected["channelId"]), Unit: str(selected["unitId"]), User: user, Created: t, Expires: t.Add(24 * time.Hour), Flags: map[string]bool{}, RewardPoints: c.Reward.Points, RewardCap: c.Reward.DailyCap, RewardCooldown: c.Reward.CooldownSeconds}
		acc.Decisions[d.ID] = d
		f.Served++
		f.Last = t
		adDay(acc, t).Fills++
		pushAdEvent(acc, d, "decision", "")
		result = M{"contract": adContract, "fill": true, "decisionId": d.ID, "expiresAt": d.Expires.Format(time.RFC3339), "eventExpiresAt": t.Add(30 * time.Minute).Format(time.RFC3339), "ad": selected, "reason": reason, "configVersion": acc.PublishedVersion, "reporting": "client-reported"}
		if slot == "rewarded" {
			result["ssv"] = M{"customData": d.ID, "userId": d.User, "points": d.RewardPoints, "ledger": "operations-points"}
		}
		return nil
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 200, result)
	return nil
}
func rewardUsage(acc *adAccount, user string, t time.Time) (int, time.Time) {
	count := 0
	last := time.Time{}
	for _, rw := range acc.Rewards {
		if rw.User != user {
			continue
		}
		at, e := time.Parse(time.RFC3339Nano, rw.At)
		if e != nil {
			continue
		}
		if at.UTC().Format("2006-01-02") == t.UTC().Format("2006-01-02") {
			count++
		}
		if at.After(last) {
			last = at
		}
	}
	return count, last
}
func (a *app) adTrack(w http.ResponseWriter, r *http.Request, p string) error {
	in, e := readBody(w, r, 8192)
	if e != nil {
		return e
	}
	id, kind := str(in["decisionId"]), str(in["type"])
	if !has([]string{"impression", "click", "complete", "error", "no_fill"}, kind) || len(str(in["code"])) > 120 {
		return problem(400, "广告事件类型或错误码无效")
	}
	duplicate := false
	result := M{}
	e = a.change(r.Context(), "ads."+p+".event", func(s *state) error {
		acc := ensureAds(s, p)
		d := acc.Decisions[id]
		if d == nil {
			return problem(404, "广告请求不存在或不属于此客户端")
		}
		if time.Since(d.Created) > 30*time.Minute {
			return problem(410, "事件凭据已过期")
		}
		if d.Flags[kind] {
			duplicate = true
			return nil
		}
		if (kind == "click" || kind == "complete") && !d.Flags["impression"] {
			return problem(409, "必须先上报曝光")
		}
		if d.Flags["error"] || d.Flags["no_fill"] {
			return problem(409, "该广告已终止")
		}
		if (kind == "error" || kind == "no_fill") && (d.Flags["impression"] || d.Flags["complete"]) {
			return problem(409, "已曝光的广告不能报告无填充")
		}
		if kind == "no_fill" && d.Source != "sdk" {
			return problem(400, "只有SDK渠道可报告无填充")
		}
		d.Flags[kind] = true
		m := adDay(acc, time.Now())
		switch kind {
		case "impression":
			m.Impressions++
		case "click":
			m.Clicks++
		case "complete":
			m.Completes++
		case "error", "no_fill":
			m.Errors++
		}
		pushAdEvent(acc, d, kind, str(in["code"]))
		// A failed SDK load can use one custom fallback under the same reservation/frequency limit.
		if (kind == "no_fill" || kind == "error") && d.Source == "sdk" && !d.FallbackUsed {
			c := clone(acc.Published)
			for i := range c.Slots {
				if c.Slots[i].ID == d.Slot && c.Slots[i].Order != "sdk-only" {
					c.Slots[i].Order = "custom-only"
				}
			}
			ad, _ := selectAd(c, d.Slot, d.OS, nil, d.VIP, time.Now())
			if ad != nil && len(acc.Decisions) < 5000 {
				d.FallbackUsed = true
				next := &adDecision{ID: randomToken(), Device: d.Device, Slot: d.Slot, OS: d.OS, Source: "custom", CreativeID: str(ad["creativeId"]), Created: time.Now(), Expires: d.Expires, Flags: map[string]bool{}}
				acc.Decisions[next.ID] = next
				result["fallback"] = M{"decisionId": next.ID, "ad": ad}
				pushAdEvent(acc, next, "fallback", "")
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	result["ok"] = true
	result["duplicate"] = duplicate
	result["rewardGranted"] = false
	jsonResponse(w, 200, result)
	return nil
}
