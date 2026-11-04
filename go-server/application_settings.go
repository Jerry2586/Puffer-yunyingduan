package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type applicationConfig struct {
	Application struct {
		SearchDelayMS int    `json:"searchDelayMs"`
		ListLimit     int    `json:"listLimit"`
		TVDownloadURL string `json:"tvDownloadUrl"`
	} `json:"application"`
	Registration struct {
		Enabled      bool `json:"enabled"`
		DailyIPLimit int  `json:"dailyIpLimit"`
		Managed      bool `json:"managed"`
		Active       bool `json:"active"`
		Points       int  `json:"points"`
		InvitePoints int  `json:"invitePoints"`
	} `json:"registration"`
	Captcha struct {
		BindIP   bool `json:"bindIp"`
		Login    bool `json:"login"`
		Register bool `json:"register"`
		Comments bool `json:"comments"`
		Danmu    bool `json:"danmu"`
	} `json:"captcha"`
	Search struct {
		IntervalMinutes int `json:"intervalMinutes"`
	} `json:"search"`
	History struct {
		Enabled         bool `json:"enabled"`
		IntervalSeconds int  `json:"intervalSeconds"`
		Limit           int  `json:"limit"`
	} `json:"history"`
	Avatar struct {
		Upload  bool     `json:"upload"`
		Library []string `json:"library"`
	} `json:"avatar"`
	Share struct {
		Text        string `json:"text"`
		Invite      bool   `json:"invite"`
		ContactText string `json:"contactText"`
		ContactURL  string `json:"contactUrl"`
		ContactMode string `json:"contactMode"`
	} `json:"share"`
	Membership struct {
		Managed bool `json:"managed"`
		GroupID int  `json:"groupId"`
		Day     int  `json:"day"`
		Week    int  `json:"week"`
		Month   int  `json:"month"`
		Year    int  `json:"year"`
	} `json:"membership"`
	Comments struct {
		Enabled        bool   `json:"enabled"`
		PinnedEnabled  bool   `json:"pinnedEnabled"`
		PinnedNickname string `json:"pinnedNickname"`
		PinnedAvatar   string `json:"pinnedAvatar"`
		PinnedContent  string `json:"pinnedContent"`
	} `json:"comments"`
}
type applicationSettings struct {
	Version   int               `json:"version"`
	UpdatedAt string            `json:"updatedAt"`
	Config    applicationConfig `json:"config"`
}
type registrationReservation struct {
	Base  string `json:"base"`
	IP    string `json:"ip"`
	Day   string `json:"day"`
	Token string `json:"token"`
}

func defaultApplicationConfig() applicationConfig {
	var c applicationConfig
	c.Application.SearchDelayMS = 350
	c.Application.ListLimit = 100
	c.Registration.Enabled = true
	c.Registration.Active = true
	c.Captcha.BindIP = true
	c.History.IntervalSeconds = 60
	c.History.Limit = 100
	c.Avatar.Library = []string{}
	c.Share.ContactMode = "copy"
	c.Comments.Enabled = true
	return c
}
func applicationState(v state) applicationSettings {
	if v.ApplicationSettings != nil {
		return clone(*v.ApplicationSettings)
	}
	return applicationSettings{Config: defaultApplicationConfig()}
}
func validSettingURL(raw string, asset bool) bool {
	if raw == "" {
		return true
	}
	if asset && assetPattern.MatchString(raw) {
		return true
	}
	u, e := url.Parse(raw)
	return e == nil && len(raw) <= 2048 && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && !strings.ContainsAny(raw, "\r\n")
}
func validateApplicationConfig(c applicationConfig) error {
	bounded := func(n, lo, hi int) bool { return n >= lo && n <= hi }
	if !bounded(c.Application.SearchDelayMS, 0, 3000) || !bounded(c.Application.ListLimit, 1, 100) || !validSettingURL(c.Application.TVDownloadURL, false) {
		return problem(400, "应用设置无效：搜索延迟 0–3000ms，列表上限 1–100，下载地址须为 HTTP(S)")
	}
	if !bounded(c.Registration.DailyIPLimit, 0, 1000) || !bounded(c.Registration.Points, 0, 1000000) || !bounded(c.Registration.InvitePoints, 0, 1000000) {
		return problem(400, "注册限制或积分无效")
	}
	if c.Search.IntervalMinutes != 0 && !bounded(c.Search.IntervalMinutes, 1, 1440) {
		return problem(400, "索引同步间隔为 0 或 1–1440 分钟")
	}
	if !bounded(c.History.IntervalSeconds, 10, 3600) || !bounded(c.History.Limit, 1, 1000) {
		return problem(400, "历史同步间隔为 10–3600 秒，保留条数为 1–1000")
	}
	if len(c.Avatar.Library) > 100 {
		return problem(400, "头像库最多 100 张")
	}
	for _, u := range c.Avatar.Library {
		if u == "" || !validSettingURL(u, true) {
			return problem(400, "头像链接无效")
		}
	}
	if utf8.RuneCountInString(c.Share.Text) > 1000 || utf8.RuneCountInString(c.Share.ContactText) > 1000 || !validSettingURL(c.Share.ContactURL, false) || !has([]string{"copy", "link"}, c.Share.ContactMode) {
		return problem(400, "分享与联系配置无效")
	}
	if c.Share.ContactMode == "link" && c.Share.ContactURL == "" {
		return problem(400, "请选择联系链接")
	}
	m := c.Membership
	if !bounded(m.GroupID, 0, 2147483647) || !bounded(m.Day, 0, 100000000) || !bounded(m.Week, 0, 100000000) || !bounded(m.Month, 0, 100000000) || !bounded(m.Year, 0, 100000000) || m.Managed && (m.GroupID < 3 || m.Day+m.Week+m.Month+m.Year == 0) {
		return problem(400, "请选择付费会员组并设置兑换积分；0 表示关闭该周期")
	}
	if !validSettingURL(c.Comments.PinnedAvatar, true) || utf8.RuneCountInString(c.Comments.PinnedNickname) > 50 || utf8.RuneCountInString(c.Comments.PinnedContent) > 2000 || c.Comments.PinnedEnabled && strings.TrimSpace(c.Comments.PinnedContent) == "" {
		return problem(400, "置顶评论内容或头像无效")
	}
	return nil
}
func applicationReport(v state) M {
	s := applicationState(v)
	return M{"version": s.Version, "updatedAt": s.UpdatedAt, "config": s.Config, "source": managementBase(v.CMSBase), "adapterConfigured": v.ManagementConnection != nil && v.ManagementConnection.Base == managementBase(v.CMSBase) && v.ManagementConnection.Key != "", "capabilities": M{"vpnDetection": false, "history": true, "captcha": true, "searchSync": "full-snapshot", "nativeTVDownload": false}}
}
func (a *app) adminApplication(w http.ResponseWriter, r *http.Request) error {
	if r.Method == "GET" {
		jsonResponse(w, 200, applicationReport(a.readState()))
		return nil
	}
	if r.Method != "POST" {
		return problem(405, "请求方法不支持")
	}
	data, e := readBody(w, r, 64*1024)
	if e != nil {
		return e
	}
	version, ok := managementInteger(data["version"], 0, 2147483646)
	if !ok {
		return problem(400, "设置版本无效")
	}
	var c applicationConfig
	raw, e := json.Marshal(data["config"])
	if e != nil {
		return problem(400, "设置格式无效")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&c); e != nil {
		return problem(400, "设置字段无效")
	}
	if e = validateApplicationConfig(c); e != nil {
		return e
	}
	e = a.change(r.Context(), "application.settings", func(s *state) error {
		if applicationState(*s).Version != version {
			return coded(409, "设置已被其他窗口更新，请读取最新版后重试", "APPLICATION_SETTINGS_CHANGED")
		}
		if c.Registration.Managed || c.Membership.Managed {
			if s.ManagementConnection == nil || s.ManagementConnection.Base != managementBase(s.CMSBase) || s.ManagementConnection.Key == "" {
				return problem(409, "运营中心注册与会员积分规则需要先接通 CMS 管理适配器")
			}
		}
		if c.Avatar.Library == nil {
			c.Avatar.Library = []string{}
		}
		s.ApplicationSettings = &applicationSettings{Version: version + 1, UpdatedAt: now(), Config: c}
		return nil
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 200, applicationReport(a.readState()))
	return nil
}
func publicApplication(v state, p string) M {
	s := applicationState(v)
	c := s.Config
	tv := ""
	if p == "tv" {
		tv = c.Application.TVDownloadURL
	}
	sourceHash := sha256.Sum256([]byte(managementBase(v.CMSBase)))
	return M{"sourceKey": hex.EncodeToString(sourceHash[:12]), "version": s.Version, "searchDelayMs": c.Application.SearchDelayMS, "listLimit": c.Application.ListLimit, "tvDownloadUrl": tv, "registrationEnabled": c.Registration.Enabled, "captcha": c.Captcha, "history": c.History, "avatar": c.Avatar, "share": c.Share, "comments": c.Comments, "membership": c.Membership, "capabilities": M{"vpnDetection": false, "nativeTVDownload": false}}
}
func requestIP(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	return ip.String()
}
func (a *app) reserveRegistration(r *http.Request, v state) (string, error) {
	c := applicationState(v).Config.Registration
	if !c.Enabled {
		return "", problem(403, "当前暂未开放注册")
	}
	token := randomToken()
	ip := requestIP(r)
	day := time.Now().UTC().Format("2006-01-02")
	base := managementBase(v.CMSBase)
	if ip == "" {
		return "", problem(400, "无法校验注册地址")
	}
	e := a.change(r.Context(), "member.register.reserve", func(s *state) error {
		if managementBase(s.CMSBase) != base || applicationState(*s).Version != applicationState(v).Version {
			return problem(409, "内容源已更改，请重试")
		}
		cfg := applicationState(*s).Config.Registration
		if !cfg.Enabled {
			return problem(403, "当前暂未开放注册")
		}
		kept := []registrationReservation{}
		count := 0
		for _, x := range s.Registrations {
			if x.Day == day {
				kept = append(kept, x)
				if x.Base == base && x.IP == ip {
					count++
				}
			}
		}
		if cfg.DailyIPLimit > 0 && count >= cfg.DailyIPLimit {
			return problem(429, "当前 IP 今日注册名额已用完")
		}
		if len(kept) >= 20000 {
			return problem(503, "注册计数容量已满，请稍后重试")
		}
		s.Registrations = append(kept, registrationReservation{Base: base, IP: ip, Day: day, Token: token})
		return nil
	})
	return token, e
}

// Reservations survive uncertain CMS responses, so retries cannot bypass the daily cap.
func invitationCode(v state, uid int) string {
	body := managementBase(v.CMSBase) + "\n" + strconv.Itoa(uid)
	mac := hmac.New(sha256.New, []byte(v.Credential.Hash))
	mac.Write([]byte(body))
	return strconv.Itoa(uid) + "." + hex.EncodeToString(mac.Sum(nil))
}
func invitationUser(v state, code string) int {
	id, _, ok := strings.Cut(code, ".")
	uid, e := strconv.Atoi(id)
	if !ok || e != nil || uid < 1 {
		return 0
	}
	if !hmac.Equal([]byte(code), []byte(invitationCode(v, uid))) {
		return 0
	}
	return uid
}
