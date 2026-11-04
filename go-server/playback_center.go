package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type cmsPlaybackSource struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	TrialSeconds int    `json:"trialSeconds"`
	Mode         string `json:"mode"`
}
type danmuPreferences struct {
	Enabled         bool   `json:"enabled"`
	ThirdParty      bool   `json:"thirdParty"`
	Endpoint        string `json:"endpoint"`
	Match           string `json:"match"`
	PreferredDomain string `json:"preferredDomain"`
	TVDensity       int    `json:"tvDensity"`
}
type playbackPreferences struct {
	Version   int                 `json:"version"`
	UpdatedAt string              `json:"updatedAt"`
	Source    string              `json:"source"`
	Sources   []cmsPlaybackSource `json:"sources"`
	Danmu     danmuPreferences    `json:"danmu"`
}

func playbackPreferencesFor(v state) playbackPreferences {
	c := playbackPreferences{Source: managementBase(v.CMSBase), Sources: []cmsPlaybackSource{}, Danmu: danmuPreferences{Enabled: true, Match: "id", TVDensity: 40}}
	if v.PlaybackPreferences != nil {
		c = clone(*v.PlaybackPreferences)
		if c.Source != managementBase(v.CMSBase) {
			c.Sources = []cmsPlaybackSource{}
			c.Source = managementBase(v.CMSBase)
		}
	}
	return c
}
func validatePlaybackPreferences(c playbackPreferences) error {
	if len(c.Sources) > 100 {
		return problem(400, "播放源最多 100 条")
	}
	seen := map[string]bool{}
	for _, s := range c.Sources {
		if !resolverID.MatchString(s.Code) || seen[s.Code] || strings.TrimSpace(s.Name) == "" || utf8.RuneCountInString(s.Name) > 60 || s.TrialSeconds < 0 || s.TrialSeconds > 3600 || !has([]string{"auto", "direct"}, s.Mode) {
			return problem(400, "播放源标识、名称、模式或试看秒数无效")
		}
		seen[s.Code] = true
	}
	d := c.Danmu
	if !has([]string{"id", "title"}, d.Match) || d.TVDensity < 1 || d.TVDensity > 100 {
		return problem(400, "弹幕匹配方式或 TV 密度无效")
	}
	if d.Endpoint != "" {
		u, e := mediaHTTPURL(d.Endpoint)
		if e != nil || u.RawQuery != "" || strings.ContainsAny(d.Endpoint, "{}\r\n") {
			return problem(400, "弹幕接口须为不含查询参数的 HTTP(S) 地址")
		}
		if d.PreferredDomain != "" && !strings.EqualFold(d.PreferredDomain, u.Hostname()) {
			return problem(400, "优先域名必须与弹幕接口域名一致")
		}
	}
	if len(d.PreferredDomain) > 253 || strings.ContainsAny(d.PreferredDomain, "/ :\r\n") {
		return problem(400, "请填写纯域名")
	}
	if d.ThirdParty && d.Endpoint == "" {
		return problem(400, "请先填写第三方弹幕接口")
	}
	return nil
}
func publicDanmuPreferences(v state, p string) M {
	d := playbackPreferencesFor(v).Danmu
	density := 100
	if p == "tv" {
		density = d.TVDensity
	}
	return M{"enabled": d.Enabled, "thirdParty": d.ThirdParty, "density": density, "review": communityPolicy(v).ReviewDanmu}
}
func (a *app) adminPlaybackCenter(w http.ResponseWriter, r *http.Request) error {
	v := a.readState()
	if r.URL.Path == "/api/admin/playback-center/sources" {
		if r.Method != "GET" {
			return problem(405, "请求方法不支持")
		}
		connection, e := a.managementConnectionFor(v)
		if e != nil {
			return e
		}
		out, e := a.managementCall(r.Context(), connection, "playback.sources", M{})
		if e != nil {
			return e
		}
		out["source"] = managementBase(v.CMSBase)
		jsonResponse(w, 200, out)
		return nil
	}
	if r.URL.Path == "/api/admin/playback-center/test-danmu" {
		if r.Method != "POST" {
			return problem(405, "请求方法不支持")
		}
		data, e := readBody(w, r, 16*1024)
		if e != nil {
			return e
		}
		var c playbackPreferences
		raw, _ := json.Marshal(data["config"])
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if dec.Decode(&c) != nil {
			return problem(400, "弹幕设置字段无效")
		}
		if e = validatePlaybackPreferences(c); e != nil {
			return e
		}
		id := str(data["videoId"])
		ep, ok := managementInteger(data["episode"], 1, 10000)
		if !vodID.MatchString(id) || !ok {
			return problem(400, "请填写真实影片 ID 与集数")
		}
		film, e := a.danmuFilm(r.Context(), v, id, ep)
		if e != nil {
			return e
		}
		rows, e := a.fetchThirdPartyDanmu(r.Context(), c.Danmu, id, str(film["title"]), ep)
		if e != nil {
			return e
		}
		jsonResponse(w, 200, M{"ok": true, "count": len(rows), "items": rows[:min(5, len(rows))]})
		return nil
	}
	if r.URL.Path != "/api/admin/playback-center" {
		return problem(404, "播放中心接口不存在")
	}
	if r.Method == "GET" {
		jsonResponse(w, 200, playbackPreferencesFor(v))
		return nil
	}
	if r.Method != "POST" {
		return problem(405, "请求方法不支持")
	}
	data, e := readBody(w, r, 64*1024)
	if e != nil {
		return e
	}
	var c playbackPreferences
	raw, _ := json.Marshal(data)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&c) != nil {
		return problem(400, "播放设置字段无效")
	}
	if e = validatePlaybackPreferences(c); e != nil {
		return e
	}
	e = a.change(r.Context(), "playback.preferences", func(s *state) error {
		if e := managementExpected(M{"source": c.Source}, managementBase(s.CMSBase)); e != nil {
			return e
		}
		if playbackPreferencesFor(*s).Version != c.Version {
			return coded(409, "播放设置已变更，请读取最新版后重试", "PLAYBACK_SETTINGS_CHANGED")
		}
		c.Version++
		c.UpdatedAt = now()
		s.PlaybackPreferences = &c
		return nil
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 200, playbackPreferencesFor(a.readState()))
	return nil
}
func playbackSourceFor(v state, code string) *cmsPlaybackSource {
	for _, s := range playbackPreferencesFor(v).Sources {
		if s.Code == code {
			return &s
		}
	}
	return nil
}
func applyPlaybackSources(v state, result M) {
	prefs := playbackPreferencesFor(v)
	rank := map[string]int{}
	for i, s := range prefs.Sources {
		rank[s.Code] = i
	}
	for _, raw := range arr(result["items"]) {
		film := obj(raw)
		sources := []any{}
		for i, rawSource := range arr(film["sources"]) {
			source := obj(rawSource)
			if source["sid"] == nil {
				source["sid"] = i + 1
			}
			code := str(source["code"])
			if code == "" {
				code = str(source["name"])
			}
			if rule := playbackSourceFor(v, code); rule != nil {
				if !rule.Enabled {
					continue
				}
				source["name"] = rule.Name
				source["trialSeconds"] = rule.TrialSeconds
			}
			sources = append(sources, source)
		}
		sort.SliceStable(sources, func(i, j int) bool {
			a, ok := rank[str(obj(sources[i])["code"])]
			if !ok {
				a = len(rank)
			}
			b, ok := rank[str(obj(sources[j])["code"])]
			if !ok {
				b = len(rank)
			}
			return a < b
		})
		film["sources"] = sources
	}
}
func (a *app) danmuFilm(ctx context.Context, v state, id string, ep int) (M, error) {
	if ep < 1 || ep > 10000 || !vodID.MatchString(id) {
		return nil, problem(400, "影片或集数无效")
	}
	result, e := a.catalog(ctx, v.CMSBase, url.Values{"ids": {id}})
	if e != nil {
		return nil, e
	}
	items := arr(result["items"])
	if len(items) != 1 {
		return nil, problem(404, "影片不存在")
	}
	film := obj(items[0])
	for _, s := range arr(film["sources"]) {
		if len(arr(obj(s)["episodes"])) >= ep {
			return film, nil
		}
	}
	return nil, problem(400, "影片集数不存在")
}
func (a *app) fetchThirdPartyDanmu(ctx context.Context, c danmuPreferences, id, title string, episode int) ([]M, error) {
	if c.Endpoint == "" {
		return nil, problem(400, "请填写第三方弹幕接口")
	}
	u, e := mediaHTTPURL(c.Endpoint)
	if e != nil {
		return nil, e
	}
	q := u.Query()
	if c.Match == "title" {
		q.Set("title", title)
	} else {
		q.Set("id", id)
	}
	q.Set("episode", strconv.Itoa(episode))
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return nil, problem(400, "弹幕接口无效")
	}
	req.Header.Set("Accept", "application/json")
	res, e := a.cms.Do(req)
	if e != nil {
		return nil, problem(502, "弹幕接口无法连接或地址被拒绝")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, problem(502, "弹幕接口未返回成功状态")
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, 1024*1024+1))
	if e != nil || len(raw) > 1024*1024 {
		return nil, problem(502, "弹幕响应超过 1MB 或读取失败")
	}
	var payload struct {
		Items []struct {
			Content  string `json:"content"`
			Position int    `json:"position"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &payload) != nil || payload.Items == nil || len(payload.Items) > 1000 {
		return nil, problem(502, "弹幕接口须返回 items 数组，最多 1000 条")
	}
	rows := []M{}
	for _, item := range payload.Items {
		if strings.TrimSpace(item.Content) == "" || utf8.RuneCountInString(item.Content) > 100 || item.Position < 0 || item.Position > 86400000 {
			return nil, problem(502, "弹幕内容或毫秒进度无效")
		}
		rows = append(rows, M{"content": item.Content, "position": item.Position, "episode": episode, "videoId": id, "thirdParty": true})
	}
	return rows, nil
}
func (a *app) clientThirdPartyDanmu(w http.ResponseWriter, r *http.Request, v state) error {
	if r.Method != "GET" {
		return problem(405, "请求方法不支持")
	}
	p, e := requestPlatform(r)
	if e != nil {
		return e
	}
	if obj(v.Operations[p].Published["features"])["member"] != true {
		return problem(403, "当前客户端未开放会员互动")
	}
	d := playbackPreferencesFor(v).Danmu
	if !d.Enabled || !d.ThirdParty {
		jsonResponse(w, 200, M{"items": []any{}})
		return nil
	}
	ip := requestIP(r)
	a.mu.Lock()
	key := "danmu-read:" + ip
	x := a.attempts[key]
	if time.Since(x.Start) > time.Minute {
		x = attempt{Start: time.Now()}
	}
	if x.Count >= 30 {
		a.mu.Unlock()
		return problem(429, "弹幕请求过于频繁")
	}
	x.Count++
	a.attempts[key] = x
	a.mu.Unlock()
	id := r.URL.Query().Get("videoId")
	ep := episodeParameter(r.URL.Query().Get("episode"))
	if !vodID.MatchString(id) || ep < 1 || ep > 10000 {
		return problem(400, "影片或集数无效")
	}
	film, e := a.danmuFilm(r.Context(), v, id, ep)
	if e != nil {
		return e
	}
	items, e := a.fetchThirdPartyDanmu(r.Context(), d, id, str(film["title"]), ep)
	if e != nil {
		return e
	}
	jsonResponse(w, 200, M{"items": items})
	return nil
}

// The original CMS sid is stable even when the client source list is reordered.
func (a *app) playbackSourcePolicy(ctx context.Context, v state, id string, sid, episode int) (string, int, string, error) {
	result, e := a.catalog(ctx, v.CMSBase, url.Values{"ids": {id}})
	if e != nil {
		return "", 0, "", e
	}
	for _, item := range arr(result["items"]) {
		for _, raw := range arr(obj(item)["sources"]) {
			source := obj(raw)
			if number(source["sid"]) != sid {
				continue
			}
			if len(arr(source["episodes"])) < episode {
				return "", 0, "", problem(400, "影片集数不存在")
			}
			code := str(source["code"])
			if rule := playbackSourceFor(v, code); rule != nil {
				if !rule.Enabled {
					return "", 0, "", problem(403, "该播放线路已关闭")
				}
				return code, rule.TrialSeconds, rule.Mode, nil
			}
			return code, 0, "auto", nil
		}
	}
	return "", 0, "", problem(404, "影片或播放线路不存在")
}
