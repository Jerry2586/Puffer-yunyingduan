package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const engineContract = "hetun-playback-engine-v1"
const engineBuildContract = "hetun-playback-build-v1"

type engineDefinition struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Systems    []string `json:"systems"`
	Commercial bool     `json:"commercial"`
}

var engineCatalog = []engineDefinition{
	{"android-system", "Android 系统播放器", []string{"android"}, false},
	{"media3", "Media3 ExoPlayer", []string{"android"}, false},
	{"avplayer", "Apple AVPlayer", []string{"ios"}, false},
	{"libvlc", "libVLC", []string{"android", "ios", "windows"}, false},
	{"html5", "浏览器原生 Video", []string{"web"}, false},
	{"hlsjs", "hls.js", []string{"web"}, false},
	{"aliyun", "阿里云播放器 SDK", []string{"android", "ios"}, true},
	{"tencent", "腾讯云播放器 SDK", []string{"android", "ios"}, true},
	{"volcengine", "火山引擎播放器 SDK", []string{"android", "ios"}, true},
}
var engineTargets = []struct {
	Platform string `json:"platform"`
	OS       string `json:"os"`
	Label    string `json:"label"`
}{
	{"mobile", "android", "Android 手机"}, {"mobile", "ios", "iPhone / iOS"},
	{"tablet", "android", "Android 平板"}, {"tablet", "ios", "iPad / iPadOS"},
	{"tv", "android", "Android 电视"}, {"web", "windows", "Windows 客户端"}, {"web", "web", "电脑浏览器"},
}
var engineFeatures = []string{"engine-policy", "engine-fallback", "manual-switch", "resume-position", "playback-report"}
var engineFailureKinds = []string{"initialization", "decode", "stall"}
var engineAppID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,199}$`)
var engineSDKVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]{0,63}$`)

type installedEngine struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	License string `json:"license"`
}
type engineBuild struct {
	Contract   string            `json:"contract"`
	Platform   string            `json:"platform"`
	OS         string            `json:"os"`
	AppID      string            `json:"appId"`
	AppVersion string            `json:"appVersion"`
	Engines    []installedEngine `json:"engines"`
	Features   []string          `json:"features"`
}
type engineProfile struct {
	Platform       string       `json:"platform"`
	OS             string       `json:"os"`
	Enabled        bool         `json:"enabled"`
	Build          *engineBuild `json:"build"`
	Primary        string       `json:"primary"`
	Backup         string       `json:"backup"`
	ManualSwitch   bool         `json:"manualSwitch"`
	AutoSwitch     bool         `json:"autoSwitch"`
	SwitchOn       []string     `json:"switchOn"`
	ResumePosition bool         `json:"resumePosition"`
	MaxSwitches    int          `json:"maxSwitches"`
	MaxAttempts    int          `json:"maxAttempts"`
	SessionSeconds int          `json:"sessionSeconds"`
}
type engineConfig struct {
	Profiles []engineProfile `json:"profiles"`
}
type engineObservation struct {
	Platform      string `json:"platform"`
	OS            string `json:"os"`
	AppID         string `json:"appId"`
	AppVersion    string `json:"appVersion"`
	Engine        string `json:"engine"`
	Outcome       string `json:"outcome"`
	Reason        string `json:"reason"`
	License       string `json:"license"`
	At            string `json:"at"`
	PolicyVersion int    `json:"policyVersion"`
	TicketHash    string `json:"ticketHash"`
}
type engineSettings struct {
	Revision         int                 `json:"revision"`
	PublishedVersion int                 `json:"publishedVersion"`
	PublishedAt      string              `json:"publishedAt"`
	Draft            engineConfig        `json:"draft"`
	Published        engineConfig        `json:"published"`
	Observations     []engineObservation `json:"observations"`
}

func defaultEngineConfig() engineConfig {
	c := engineConfig{Profiles: []engineProfile{}}
	for _, t := range engineTargets {
		c.Profiles = append(c.Profiles, engineProfile{Platform: t.Platform, OS: t.OS, SwitchOn: []string{"initialization", "decode"}, ResumePosition: true, MaxSwitches: 1, MaxAttempts: 6, SessionSeconds: 90})
	}
	return c
}
func playbackEngineState(v state) engineSettings {
	if v.PlaybackEngines != nil {
		return *v.PlaybackEngines
	}
	c := defaultEngineConfig()
	return engineSettings{Revision: 1, Draft: c, Published: clone(c), Observations: []engineObservation{}}
}
func engineTargetExists(p, os string) bool {
	for _, t := range engineTargets {
		if t.Platform == p && t.OS == os {
			return true
		}
	}
	return false
}
func lookupEngine(id string) (engineDefinition, bool) {
	for _, d := range engineCatalog {
		if d.ID == id {
			return d, true
		}
	}
	return engineDefinition{}, false
}
func engineListValid(xs, allowed []string) bool {
	seen := map[string]bool{}
	for _, s := range xs {
		if !has(allowed, s) || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}
func decodeEngineValue(raw any, out any) error {
	b, e := json.Marshal(raw)
	if e != nil || string(b) == "null" {
		return problem(400, "播放器配置不能为空")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(out); e != nil {
		return problem(400, "播放器字段或数据类型错误")
	}
	return nil
}
func validateEngineBuild(b engineBuild) error {
	if b.Contract != engineBuildContract || !engineTargetExists(b.Platform, b.OS) || !engineAppID.MatchString(b.AppID) || !versionPattern.MatchString(b.AppVersion) || len(b.Engines) < 1 || len(b.Engines) > len(engineCatalog) || !has(b.Features, "engine-policy") || !engineListValid(b.Features, engineFeatures) {
		return problem(400, "基础包能力声明的协议、应用身份、系统或执行能力无效")
	}
	ids := map[string]bool{}
	for _, installed := range b.Engines {
		d, ok := lookupEngine(installed.ID)
		if !ok || !has(d.Systems, b.OS) || ids[installed.ID] || !engineSDKVersion.MatchString(installed.Version) || !has([]string{"not_required", "unknown", "valid", "invalid"}, installed.License) || (!d.Commercial && installed.License != "not_required") || (d.Commercial && installed.License == "not_required") {
			return problem(400, "内核重复、系统不支持、SDK版本或授权声明无效")
		}
		ids[installed.ID] = true
	}
	return nil
}
func buildHasEngine(b *engineBuild, id string, licensed bool) bool {
	if b == nil {
		return false
	}
	d, ok := lookupEngine(id)
	if !ok {
		return false
	}
	for _, e := range b.Engines {
		if e.ID == id {
			return !licensed || !d.Commercial || e.License == "valid"
		}
	}
	return false
}
func requiredEngineFeatures(p engineProfile) []string {
	out := []string{"engine-policy", "playback-report"}
	if p.AutoSwitch {
		out = append(out, "engine-fallback")
	}
	if p.ManualSwitch {
		out = append(out, "manual-switch")
	}
	if p.ResumePosition {
		out = append(out, "resume-position")
	}
	return out
}
func validateEngineConfig(c engineConfig, publishing bool) error {
	if len(c.Profiles) != len(engineTargets) {
		return problem(400, "必须保留四端全部系统配置")
	}
	seen := map[string]bool{}
	for _, p := range c.Profiles {
		key := p.Platform + "/" + p.OS
		if !engineTargetExists(p.Platform, p.OS) || seen[key] || !engineListValid(p.SwitchOn, engineFailureKinds) || p.MaxSwitches < 0 || p.MaxSwitches > 2 || p.MaxAttempts < 1 || p.MaxAttempts > 20 || p.SessionSeconds < 15 || p.SessionSeconds > 300 {
			return problem(400, "客户端、切换条件或尝试预算无效")
		}
		seen[key] = true
		for _, id := range []string{p.Primary, p.Backup} {
			if id != "" {
				d, ok := lookupEngine(id)
				if !ok || !has(d.Systems, p.OS) {
					return problem(400, "所选内核不支持当前系统")
				}
			}
		}
		if p.Primary != "" && p.Primary == p.Backup {
			return problem(400, "主备内核不能重复")
		}
		if p.Build != nil {
			if e := validateEngineBuild(*p.Build); e != nil {
				return e
			}
			if p.Build.Platform != p.Platform || p.Build.OS != p.OS {
				return problem(400, "能力声明与当前客户端不一致")
			}
		}
		if !publishing || !p.Enabled {
			continue
		}
		if p.Primary == "" || !buildHasEngine(p.Build, p.Primary, true) || (p.Backup != "" && !buildHasEngine(p.Build, p.Backup, true)) {
			return coded(409, key+"：请先登记包含所选内核及有效授权声明的基础包能力", "PLAYBACK_BUILD_REQUIRED")
		}
		if p.AutoSwitch && (p.Backup == "" || p.MaxSwitches < 1 || len(p.SwitchOn) == 0) {
			return problem(400, "自动切换需要备用内核、失败条件和切换次数")
		}
		if p.ManualSwitch && p.Backup == "" {
			return problem(400, "手动切换需要不同的备用内核")
		}
		for _, f := range requiredEngineFeatures(p) {
			if !has(p.Build.Features, f) {
				return coded(409, key+"：基础包尚未声明执行能力 "+f, "PLAYBACK_ADAPTER_REQUIRED")
			}
		}
	}
	return nil
}
func engineProfileFor(c engineConfig, p, os string) (engineProfile, bool) {
	for _, v := range c.Profiles {
		if v.Platform == p && v.OS == os {
			return v, true
		}
	}
	return engineProfile{}, false
}
func engineAdminView(v state) M {
	return M{"settings": playbackEngineState(v), "engines": engineCatalog, "targets": engineTargets, "contract": engineContract, "capabilitySource": "admin_declared", "observationSource": "playback_client_report"}
}
func (a *app) adminPlaybackEngines(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path == "/api/admin/playback-engines" && r.Method == "GET" {
		jsonResponse(w, 200, engineAdminView(a.readState()))
		return nil
	}
	if !((r.URL.Path == "/api/admin/playback-engines" && r.Method == "PUT") || (r.URL.Path == "/api/admin/playback-engines/publish" && r.Method == "POST") || (r.URL.Path == "/api/admin/playback-engines/inspect" && r.Method == "POST")) {
		return problem(405, "请求方法不支持")
	}
	in, e := readBody(w, r, 128*1024)
	if e != nil {
		return e
	}
	if r.URL.Path == "/api/admin/playback-engines/inspect" {
		var b engineBuild
		if e = decodeEngineValue(in["build"], &b); e != nil {
			return e
		}
		if e = validateEngineBuild(b); e != nil {
			return e
		}
		jsonResponse(w, 200, M{"build": b, "source": "admin_declared", "deviceVerified": false})
		return nil
	}
	var c engineConfig
	if r.Method == "PUT" {
		if e = decodeEngineValue(in["config"], &c); e != nil {
			return e
		}
		if e = validateEngineConfig(c, false); e != nil {
			return e
		}
	}
	e = a.change(r.Context(), "playback.engines."+strings.ToLower(r.Method), func(v *state) error {
		s := playbackEngineState(*v)
		if number(in["revision"]) != s.Revision {
			return problem(409, "内核配置已变化，请重新加载后再试")
		}
		if r.Method == "PUT" {
			s.Draft = c
		} else {
			if e := validateEngineConfig(s.Draft, true); e != nil {
				return e
			}
			s.Published = clone(s.Draft)
			s.PublishedVersion++
			s.PublishedAt = now()
		}
		s.Revision++
		v.PlaybackEngines = &s
		v.UpdatedAt = now()
		return nil
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 200, engineAdminView(a.readState()))
	return nil
}

// These are runtime instructions, not a server-side decoder or permission grant.
func publicEngineSummary(v state, p, os string) M {
	s := playbackEngineState(v)
	profile, exists := engineProfileFor(s.Published, p, os)
	out := M{"contract": engineContract, "platform": p, "os": os, "version": s.PublishedVersion, "enabled": exists && profile.Enabled, "status": "disabled", "negotiatePath": "/api/v1/" + clientName(p) + "/playback/negotiate"}
	if exists && profile.Enabled {
		out["status"] = "client_capabilities_required"
	}
	return out
}
func negotiateEngine(v state, p string, b *engineBuild) (M, error) {
	s := playbackEngineState(v)
	if b == nil {
		return M{"contract": engineContract, "enabled": false, "status": "legacy_client", "version": s.PublishedVersion}, nil
	}
	if e := validateEngineBuild(*b); e != nil {
		return nil, e
	}
	if b.Platform != p {
		return nil, problem(400, "内核能力与客户端路径冲突")
	}
	profile, ok := engineProfileFor(s.Published, p, b.OS)
	out := publicEngineSummary(v, p, b.OS)
	if !ok || !profile.Enabled {
		return out, nil
	}
	if profile.Build == nil || profile.Build.AppID != b.AppID || profile.Build.AppVersion != b.AppVersion {
		return nil, coded(409, "应用身份或版本与已发布内核策略不匹配", "PLAYBACK_BUILD_MISMATCH")
	}
	for _, f := range requiredEngineFeatures(profile) {
		if !has(b.Features, f) {
			return nil, coded(409, "当前安装包缺少内核策略执行能力 "+f, "PLAYBACK_ADAPTER_REQUIRED")
		}
	}
	// Intersect the published build declaration with the client's actual adapter capabilities.
	if !buildHasEngine(b, profile.Primary, true) {
		return nil, coded(409, "当前客户端没有可用的主内核或授权", "PLAYBACK_ENGINE_UNAVAILABLE")
	}
	backup := ""
	if buildHasEngine(b, profile.Backup, true) {
		backup = profile.Backup
	}
	if (profile.AutoSwitch || profile.ManualSwitch) && backup == "" {
		return nil, coded(409, "当前客户端缺少切换所需备用内核", "PLAYBACK_ENGINE_UNAVAILABLE")
	}
	out["status"] = "negotiated"
	out["primary"] = profile.Primary
	out["backup"] = backup
	out["manualSwitch"] = profile.ManualSwitch && backup != ""
	out["autoSwitch"] = profile.AutoSwitch && backup != ""
	out["switchOn"] = profile.SwitchOn
	out["resumePosition"] = profile.ResumePosition
	out["maxSwitches"] = profile.MaxSwitches
	out["maxAttempts"] = profile.MaxAttempts
	out["sessionSeconds"] = profile.SessionSeconds
	out["reportPath"] = "/api/v1/" + clientName(p) + "/playback/report"
	return out, nil
}
func (a *app) clientPlaybackEngines(w http.ResponseWriter, r *http.Request, v state) error {
	p, e := requestPlatform(r)
	if e != nil {
		return e
	}
	if r.URL.Path == "/api/playback/config" && r.Method == "GET" {
		os := r.URL.Query().Get("os")
		if !engineTargetExists(p, os) {
			return problem(400, "请选择当前客户端支持的系统")
		}
		jsonResponse(w, 200, publicEngineSummary(v, p, os))
		return nil
	}
	if r.Method != "POST" {
		return problem(405, "请求方法不支持")
	}
	in, e := readBody(w, r, 32*1024)
	if e != nil {
		return e
	}
	if r.URL.Path == "/api/playback/negotiate" {
		var b engineBuild
		if e = decodeEngineValue(in["client"], &b); e != nil {
			return e
		}
		out, e := negotiateEngine(v, p, &b)
		if e != nil {
			return e
		}
		jsonResponse(w, 200, out)
		return nil
	}
	if r.URL.Path == "/api/playback/report" {
		return a.recordEngineObservation(w, r, v, p, in)
	}
	return problem(404, "播放器接口不存在")
}

type engineReportTicket struct {
	ID            string   `json:"id"`
	Platform      string   `json:"platform"`
	OS            string   `json:"os"`
	AppID         string   `json:"appId"`
	AppVersion    string   `json:"appVersion"`
	Engines       []string `json:"engines"`
	PolicyVersion int      `json:"policyVersion"`
	Expires       int64    `json:"expires"`
}

func engineTicketMAC(v state, payload string) []byte {
	m := hmac.New(sha256.New, []byte(v.Credential.Hash+"\x00"+v.CMSBase))
	m.Write([]byte(payload))
	return m.Sum(nil)
}

// Only authorized playback issues a ticket. Negotiation and bootstrap cannot forge a device test.
func issueEngineReportTicket(v state, p string, b *engineBuild, policy M) string {
	engines := []string{str(policy["primary"])}
	if id := str(policy["backup"]); id != "" {
		engines = append(engines, id)
	}
	ticket := engineReportTicket{randomToken(), p, b.OS, b.AppID, b.AppVersion, engines, number(policy["version"]), time.Now().Add(10 * time.Minute).Unix()}
	raw, _ := json.Marshal(ticket)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + base64.RawURLEncoding.EncodeToString(engineTicketMAC(v, payload))
}
func (a *app) recordEngineObservation(w http.ResponseWriter, r *http.Request, v state, p string, in M) error {
	token := str(in["ticket"])
	if len(token) > 4096 {
		return problem(400, "播放反馈凭据无效")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return problem(401, "需要已授权播放签发的反馈凭据")
	}
	mac, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil || !hmac.Equal(mac, engineTicketMAC(v, parts[0])) {
		return problem(401, "播放反馈凭据无效")
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	var ticket engineReportTicket
	if e != nil || json.Unmarshal(raw, &ticket) != nil || ticket.Platform != p || ticket.Expires <= time.Now().Unix() {
		return problem(401, "播放反馈凭据过期或不属于当前客户端")
	}
	s := playbackEngineState(v)
	if ticket.PolicyVersion != s.PublishedVersion {
		return problem(409, "内核策略已更新，请重新发起播放")
	}
	outcome, reason, engine, license := str(in["outcome"]), str(in["reason"]), str(in["engine"]), str(in["license"])
	d, ok := lookupEngine(engine)
	if !ok || !has(ticket.Engines, engine) || !has([]string{"prepared", "failed"}, outcome) || !has([]string{"", "initialization", "decode", "stall", "network", "license", "other"}, reason) || (outcome == "prepared" && reason != "") || (outcome == "failed" && reason == "") || !has([]string{"not_required", "unknown", "valid", "invalid"}, license) || (!d.Commercial && license != "not_required") || (d.Commercial && license == "not_required") {
		return problem(400, "播放反馈内核、结果、原因或授权状态无效")
	}
	if d.Commercial && outcome == "prepared" && license != "valid" {
		return problem(400, "商业内核授权未通过，不能报告已就绪")
	}
	digest := sha256.Sum256([]byte(token))
	hash := base64.RawURLEncoding.EncodeToString(digest[:])
	observation := engineObservation{p, ticket.OS, ticket.AppID, ticket.AppVersion, engine, outcome, reason, license, now(), ticket.PolicyVersion, hash}
	e = a.change(r.Context(), "playback.engine.report", func(next *state) error {
		// Recheck after acquiring the state lock, including key/CMS changes and republish.
		if !hmac.Equal(mac, engineTicketMAC(*next, parts[0])) {
			return problem(401, "播放反馈凭据已失效")
		}
		current := playbackEngineState(*next)
		if current.PublishedVersion != ticket.PolicyVersion {
			return problem(409, "内核策略已更新")
		}
		for _, old := range current.Observations {
			if old.TicketHash == hash {
				if old.Engine == engine && old.Outcome == outcome && old.Reason == reason && old.License == license {
					return nil
				}
				return problem(409, "该播放反馈凭据已使用")
			}
		}
		current.Observations = append([]engineObservation{observation}, current.Observations...)
		if len(current.Observations) > 64 {
			current.Observations = current.Observations[:64]
		}
		next.PlaybackEngines = &current
		return nil
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 200, M{"accepted": true, "source": "playback_client_report", "deviceVerified": false})
	return nil
}
