package main

// Advertising is a separate, versioned operations contract. Theme geometry is never changed here.
import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const adContract = "hetun-advertising-v1"

var adSlots = []string{"splash", "home_interstitial", "home_banner", "feed", "search_banner", "mine_banner", "playback", "rewarded"}
var adSlotNames = map[string]string{"splash": "开屏广告", "home_interstitial": "首页插屏", "home_banner": "首页横幅", "feed": "信息流", "search_banner": "搜索横幅", "mine_banner": "我的页面横幅", "playback": "播放页广告", "rewarded": "激励视频"}
var admobAppID = regexp.MustCompile(`^ca-app-pub-[0-9]{16}~[0-9]{10}$`)
var admobUnitID = regexp.MustCompile(`^ca-app-pub-[0-9]{16}/[0-9]{10}$`)

type adSlot struct {
	ID              string `json:"id"`
	Enabled         bool   `json:"enabled"`
	Order           string `json:"order"`
	DailyCap        int    `json:"dailyCap"`
	CooldownSeconds int    `json:"cooldownSeconds"`
	Every           int    `json:"every"`
	ExcludeVIP      bool   `json:"excludeVip"`
}
type adCreative struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Advertiser  string   `json:"advertiser"`
	OrderNo     string   `json:"orderNo"`
	Kind        string   `json:"kind"`
	Slot        string   `json:"slot"`
	Status      string   `json:"status"`
	Media       string   `json:"media"`
	Asset       string   `json:"asset"`
	Text        string   `json:"text"`
	Destination string   `json:"destination"`
	URL         string   `json:"url"`
	OS          []string `json:"os"`
	Priority    int      `json:"priority"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Duration    int      `json:"duration"`
	SkipAfter   int      `json:"skipAfter"`
}
type adChannel struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Provider string            `json:"provider"`
	OS       string            `json:"os"`
	Enabled  bool              `json:"enabled"`
	AppID    string            `json:"appId"`
	Units    map[string]string `json:"units"`
}
type adRewardRule struct {
	Enabled         bool `json:"enabled"`
	Points          int  `json:"points"`
	DailyCap        int  `json:"dailyCap"`
	CooldownSeconds int  `json:"cooldownSeconds"`
}
type adConfig struct {
	Slots     []adSlot     `json:"slots"`
	Creatives []adCreative `json:"creatives"`
	Channels  []adChannel  `json:"channels"`
	Reward    adRewardRule `json:"reward"`
}
type adAccount struct {
	Version          int                     `json:"version"`
	PublishedVersion int                     `json:"publishedVersion"`
	PublishedAt      string                  `json:"publishedAt"`
	Draft            adConfig                `json:"draft"`
	Published        adConfig                `json:"published"`
	Decisions        map[string]*adDecision  `json:"decisions"`
	Frequency        map[string]*adFrequency `json:"frequency"`
	Days             map[string]*adMetrics   `json:"days"`
	Events           []adEvent               `json:"events"`
	Rewards          []adReward              `json:"rewards"`
}

func emptyAds() adConfig {
	c := adConfig{Slots: []adSlot{}, Creatives: []adCreative{}, Channels: []adChannel{}, Reward: adRewardRule{Points: 10, DailyCap: 5, CooldownSeconds: 300}}
	for _, id := range adSlots {
		c.Slots = append(c.Slots, adSlot{ID: id, Order: "custom-first", DailyCap: 10, CooldownSeconds: 60, Every: 1})
	}
	return c
}
func newAdAccount() *adAccount {
	return &adAccount{Version: 1, Draft: emptyAds(), Published: emptyAds(), Decisions: map[string]*adDecision{}, Frequency: map[string]*adFrequency{}, Days: map[string]*adMetrics{}, Events: []adEvent{}, Rewards: []adReward{}}
}
func ensureAds(s *state, p string) *adAccount {
	if s.Advertising == nil {
		s.Advertising = map[string]*adAccount{}
	}
	if s.Advertising[p] == nil {
		s.Advertising[p] = newAdAccount()
	}
	return s.Advertising[p]
}
func readAds(s state, p string) *adAccount {
	if x := s.Advertising[p]; x != nil {
		return x
	}
	return newAdAccount()
}
func adPlatform(r *http.Request) (string, error) {
	if r.URL.Query().Get("platform") == "" {
		return "", problem(400, "必须指定客户端 platform")
	}
	return requestPlatform(r)
}
func adOS(p, os string) bool { return releaseTarget(p, os) }
func validateAdConfig(c adConfig, p string) error {
	if len(c.Slots) != len(adSlots) || len(c.Creatives) > 100 || len(c.Channels) > 20 {
		return problem(400, "需要8个固定广告位；每端最多100个广告、20个渠道")
	}
	seen := map[string]bool{}
	for _, s := range c.Slots {
		if !has(adSlots, s.ID) || seen[s.ID] || !has([]string{"custom-first", "sdk-first", "custom-only", "sdk-only"}, s.Order) {
			return problem(400, "广告位重复或投放顺序无效")
		}
		seen[s.ID] = true
		if s.DailyCap < 1 || s.DailyCap > 1000 || s.CooldownSeconds < 0 || s.CooldownSeconds > 86400 || s.Every < 1 || s.Every > 1000 {
			return problem(400, "广告位频控范围无效")
		}
	}
	seen = map[string]bool{}
	for _, x := range c.Creatives {
		if !idPattern.MatchString(x.ID) || seen[x.ID] || strings.TrimSpace(x.Name) == "" || len(x.Name) > 120 || len(x.Advertiser) > 120 || len(x.OrderNo) > 120 {
			return problem(400, "广告ID重复、名称为空或字段过长")
		}
		seen[x.ID] = true
		if !has(adSlots, x.Slot) || x.Slot == "rewarded" || !has([]string{"draft", "active", "paused"}, x.Status) || !has([]string{"house", "commercial"}, x.Kind) || !has([]string{"image", "video", "text"}, x.Media) {
			return problem(400, "广告位、类型或状态无效；激励视频只通过已验证渠道发奖")
		}
		if x.Priority < 0 || x.Priority > 1000 || x.Duration < 1 || x.Duration > 120 || x.SkipAfter < 0 || x.SkipAfter > x.Duration || len(x.Text) > 2000 {
			return problem(400, "优先级、展示时长或文案无效")
		}
		if x.Media == "text" && strings.TrimSpace(x.Text) == "" {
			return problem(400, "文字广告需要文案")
		}
		if x.Media != "text" {
			if !assetPattern.MatchString(x.Asset) {
				return problem(400, "广告素材必须先上传到本机素材库")
			}
			if (x.Media == "video") != strings.HasSuffix(x.Asset, ".mp4") {
				return problem(400, "广告媒体类型与素材不符")
			}
		}
		if !has([]string{"none", "url", "home", "search", "member"}, x.Destination) {
			return problem(400, "跳转类型无效")
		}
		if x.Destination == "url" {
			u, e := url.Parse(x.URL)
			if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(x.URL) > 2048 {
				return problem(400, "落地页必须为不含账号密码的HTTPS地址")
			}
		} else if x.URL != "" {
			return problem(400, "非链接广告不能填写落地地址")
		}
		if len(x.OS) == 0 || len(x.OS) > 2 {
			return problem(400, "请选择该客户端支持的系统")
		}
		ss := map[string]bool{}
		for _, os := range x.OS {
			if !adOS(p, os) || ss[os] {
				return problem(400, "目标系统不属于当前客户端或重复")
			}
			ss[os] = true
		}
		var start, end time.Time
		var e error
		if x.Start != "" {
			start, e = time.Parse(time.RFC3339, x.Start)
			if e != nil {
				return problem(400, "开始日期必须包含时区")
			}
		}
		if x.End != "" {
			end, e = time.Parse(time.RFC3339, x.End)
			if e != nil {
				return problem(400, "结束日期必须包含时区")
			}
		}
		if !start.IsZero() && !end.IsZero() && !end.After(start) {
			return problem(400, "结束时间必须晚于开始时间")
		}
	}
	seen = map[string]bool{}
	for _, c := range c.Channels {
		if !idPattern.MatchString(c.ID) || seen[c.ID] || strings.TrimSpace(c.Name) == "" || len(c.Name) > 120 || len(c.AppID) > 120 || !adOS(p, c.OS) || !has([]string{"admob", "pangle", "gromore", "tanx", "other"}, c.Provider) {
			return problem(400, "渠道名称、ID、系统或服务商无效")
		}
		seen[c.ID] = true
		if len(c.Units) > len(adSlots) {
			return problem(400, "渠道广告位过多")
		}
		for slot, u := range c.Units {
			if !has(adSlots, slot) || len(u) > 120 {
				return problem(400, "渠道广告位映射无效")
			}
		}
		if c.Enabled {
			if c.Provider != "admob" {
				return problem(409, "此服务商尚未安装真实适配器，不能启用")
			}
			if p == "tv" || p == "web" {
				return problem(409, "AdMob适配器仅支持手机和平板Android/iOS")
			}
			if !admobAppID.MatchString(c.AppID) || len(c.Units) == 0 {
				return problem(400, "AdMob App ID或广告位映射未填写正确")
			}
			for _, u := range c.Units {
				if !admobUnitID.MatchString(u) {
					return problem(400, "AdMob广告单元ID格式错误")
				}
			}
		}
	}
	if c.Reward.Points < 1 || c.Reward.Points > 10000 || c.Reward.DailyCap < 1 || c.Reward.DailyCap > 100 || c.Reward.CooldownSeconds < 30 || c.Reward.CooldownSeconds > 86400 {
		return problem(400, "奖励积分1–10000、日次数1–100、冷却30–86400秒")
	}
	if c.Reward.Enabled {
		found := false
		for _, ch := range c.Channels {
			if ch.Enabled && ch.Provider == "admob" && ch.Units["rewarded"] != "" {
				found = true
			}
		}
		if !found {
			return problem(409, "激励奖励需要启用AdMob并配置激励广告单元")
		}
	}
	return nil
}
func (a *app) validateAdAssets(c adConfig) error {
	for _, x := range c.Creatives {
		if x.Media == "text" {
			continue
		}
		f, e := os.Stat(filepath.Join(a.assets, strings.TrimPrefix(x.Asset, "/api/assets/")))
		if e != nil || !f.Mode().IsRegular() || f.Size() == 0 {
			return problem(409, "广告素材不存在，请重新上传："+x.Name)
		}
	}
	return nil
}
func decodeAds(m M) (adConfig, error) {
	var c adConfig
	b, _ := json.Marshal(m["config"])
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&c); e != nil {
		return c, problem(400, "广告配置字段或类型不正确")
	}
	return c, nil
}
func (a *app) adAdminReport(p string) M {
	acc := readAds(a.readState(), p)
	days := []M{}
	today := time.Now().UTC().Format("2006-01-02")
	totals := adMetrics{}
	for i := 29; i >= 0; i-- {
		day := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		m := adMetrics{}
		if d := acc.Days[day]; d != nil {
			m = *d
		}
		totals.add(m)
		days = append(days, M{"date": day, "metrics": m})
	}
	day := adMetrics{}
	if d := acc.Days[today]; d != nil {
		day = *d
	}
	channels := []M{}
	for _, ch := range acc.Draft.Channels {
		channels = append(channels, M{"id": ch.ID, "adapter": ch.Provider == "admob", "clientVerified": false, "status": "客户端尚未验收"})
	}
	return M{"contract": adContract, "platform": p, "version": acc.Version, "publishedVersion": acc.PublishedVersion, "publishedAt": acc.PublishedAt, "draft": acc.Draft, "published": acc.Published, "today": day, "totals": totals, "days": days, "events": acc.Events, "rewards": acc.Rewards, "channels": channels, "slotNames": adSlotNames, "callbackUrl": a.config.PublicOrigin + "/api/ads/callback/admob", "reporting": "client-reported", "ledger": "operations-points", "limits": M{"activeDecisions": 5000, "recentEvents": 500, "rewardEntries": 20000, "statisticsDays": 90}, "checkedAt": now()}
}
func (a *app) adminAds(w http.ResponseWriter, r *http.Request) error {
	p, e := adPlatform(r)
	if e != nil {
		return e
	}
	route := r.URL.Path
	if route == "/api/admin/ads" && r.Method == "GET" {
		jsonResponse(w, 200, a.adAdminReport(p))
		return nil
	}
	if route == "/api/admin/ads/diagnose" && r.Method == "POST" {
		input, e := readBody(w, r, 8192)
		if e != nil {
			return e
		}
		os := str(input["os"])
		slot := str(input["slot"])
		if !adOS(p, os) || !has(adSlots, slot) {
			return problem(400, "广告位或系统无效")
		}
		acc := readAds(a.readState(), p)
		c := acc.Published
		selected, reason := selectAd(c, slot, os, nil, false, time.Now())
		checks := []M{{"name": "广告配置已发布", "ok": acc.PublishedVersion > 0}, {"name": "当前端投放配置校验", "ok": validateAdConfig(c, p) == nil}}
		assetError := a.validateAdAssets(c)
		checks = append(checks, M{"name": "本地素材文件存在", "ok": assetError == nil})
		if assetError != nil {
			checks[len(checks)-1]["error"] = assetError.Error()
		}
		jsonResponse(w, 200, M{"mode": "read-only-diagnosis", "selected": selected, "reason": reason, "checks": checks, "checkedAt": now(), "message": "诊断不产生曝光或奖励；SDK展示需要真实客户端执行"})
		return nil
	}
	if (route == "/api/admin/ads" && r.Method == "PUT") || (route == "/api/admin/ads/publish" && r.Method == "POST") {
		input, e := readBody(w, r, 512*1024)
		if e != nil {
			return e
		}
		var c adConfig
		if r.Method == "PUT" {
			c, e = decodeAds(input)
			if e != nil {
				return e
			}
			if e = validateAdConfig(c, p); e != nil {
				return e
			}
			if e = a.validateAdAssets(c); e != nil {
				return e
			}
		}
		e = a.change(r.Context(), "ads."+p+"."+strings.ToLower(r.Method), func(s *state) error {
			acc := ensureAds(s, p)
			if number(input["version"]) != acc.Version {
				return problem(409, "广告配置已被修改，请刷新后重试")
			}
			if r.Method == "PUT" {
				acc.Draft = c
			} else {
				if e := validateAdConfig(acc.Draft, p); e != nil {
					return e
				}
				if e := a.validateAdAssets(acc.Draft); e != nil {
					return e
				}
				acc.Published = clone(acc.Draft)
				acc.PublishedVersion++
				acc.PublishedAt = now()
			}
			acc.Version++
			return nil
		})
		if e != nil {
			return e
		}
		jsonResponse(w, 200, a.adAdminReport(p))
		return nil
	}
	return problem(405, fmt.Sprintf("广告接口不支持 %s %s", r.Method, route))
}
