package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Resolver order is explicit. Credentials stay in the admin-only endpoint;
// public responses expose neither endpoint URLs nor upstream response bodies.
type playbackSource struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Endpoint  string   `json:"endpoint"`
	URLField  string   `json:"urlField"`
	Enabled   bool     `json:"enabled"`
	Platforms []string `json:"platforms"`
}
type playbackConfig struct {
	Enabled        bool             `json:"enabled"`
	DirectFirst    bool             `json:"directFirst"`
	TimeoutSeconds int              `json:"timeoutSeconds"`
	TotalSeconds   int              `json:"totalSeconds"`
	MaxAttempts    int              `json:"maxAttempts"`
	Sources        []playbackSource `json:"sources"`
}
type playbackSettings struct {
	Revision         int            `json:"revision"`
	PublishedVersion int            `json:"publishedVersion"`
	PublishedAt      string         `json:"publishedAt"`
	Draft            playbackConfig `json:"draft"`
	Published        playbackConfig `json:"published"`
}
type resolverAttempt struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	ElapsedMS int64  `json:"elapsedMs"`
	Message   string `json:"message"`
}
type playbackResolution struct {
	OK       bool              `json:"ok"`
	URL      string            `json:"url,omitempty"`
	SourceID string            `json:"sourceId,omitempty"`
	Kind     string            `json:"kind,omitempty"`
	Attempts []resolverAttempt `json:"attempts"`
	Message  string            `json:"message"`
}

var resolverID = regexp.MustCompile("^[a-zA-Z0-9_-]{1,64}$")
var resolverField = regexp.MustCompile(`^[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)*$`)

func defaultPlaybackConfig() playbackConfig {
	return playbackConfig{DirectFirst: true, TimeoutSeconds: 5, TotalSeconds: 15, MaxAttempts: 3, Sources: []playbackSource{}}
}
func playbackState(v state) playbackSettings {
	if v.Playback != nil {
		return *v.Playback
	}
	c := defaultPlaybackConfig()
	return playbackSettings{Revision: 1, Draft: c, Published: c}
}
func mediaHTTPURL(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || !has([]string{"http", "https"}, u.Scheme) || len(raw) > 8192 {
		return nil, problem(400, "请输入有效的 HTTP(S) 资源地址")
	}
	return u, nil
}
func validatePlayback(c playbackConfig) error {
	if c.TimeoutSeconds < 1 || c.TimeoutSeconds > 10 || c.TotalSeconds < 1 || c.TotalSeconds > 20 || c.TimeoutSeconds > c.TotalSeconds || c.MaxAttempts < 1 || c.MaxAttempts > 10 || len(c.Sources) > 32 {
		return problem(400, "单次超时须为1–10秒，总时限1–20秒且不小于单次超时，尝试次数1–10，来源最多32个")
	}
	ids := map[string]bool{}
	for _, s := range c.Sources {
		if !resolverID.MatchString(s.ID) || s.ID == "direct" || ids[s.ID] || strings.TrimSpace(s.Name) == "" || len([]rune(s.Name)) > 60 {
			return problem(400, "解析来源标识重复或名称无效")
		}
		ids[s.ID] = true
		if !has([]string{"json", "redirect"}, s.Kind) {
			return problem(400, "解析类型只支持 JSON 或 HTTP 跳转")
		}
		if strings.Count(s.Endpoint, "{url}") != 1 || strings.Count(s.Endpoint, "{") != 1 || strings.Count(s.Endpoint, "}") != 1 {
			return problem(400, "解析地址必须且只能包含一个 {url} 占位符")
		}
		// Placeholder belongs in a query value, never the origin or parameter name.
		u, e := mediaHTTPURL(strings.ReplaceAll(s.Endpoint, "{url}", "HETUN_SOURCE"))
		if e != nil || strings.Contains(u.Host+u.Path, "HETUN_SOURCE") {
			return problem(400, "解析地址的 {url} 必须放在查询参数值中")
		}
		found := false
		for key, values := range u.Query() {
			if strings.Contains(key, "HETUN_SOURCE") {
				return problem(400, "占位符不能用于参数名称")
			}
			for _, value := range values {
				if strings.Contains(value, "HETUN_SOURCE") {
					found = true
				}
			}
		}
		if !found {
			return problem(400, "解析地址缺少有效的资源查询参数")
		}
		if s.Kind == "json" && (!resolverField.MatchString(s.URLField) || len(s.URLField) > 128) {
			return problem(400, "JSON 地址字段应为 url 或 data.url 等点分路径")
		}
		if len(s.Platforms) == 0 || len(s.Platforms) > 4 {
			return problem(400, "请选择解析来源适用的客户端")
		}
		platformsSeen := map[string]bool{}
		for _, p := range s.Platforms {
			if !has(platforms, p) || platformsSeen[p] {
				return problem(400, "解析来源客户端无效或重复")
			}
			platformsSeen[p] = true
		}
	}
	return nil
}
func decodePlayback(raw any) (playbackConfig, error) {
	var c playbackConfig
	b, e := json.Marshal(raw)
	if e != nil {
		return c, problem(400, "解析配置格式错误")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, problem(400, "解析配置字段或类型错误")
	}
	if c.Sources == nil {
		c.Sources = []playbackSource{}
	}
	return c, validatePlayback(c)
}
func (a *app) adminPlayback(w http.ResponseWriter, r *http.Request) error {
	saved := playbackState(a.readState())
	if r.URL.Path == "/api/admin/playback-resolvers" && r.Method == "GET" {
		jsonResponse(w, 200, saved)
		return nil
	}
	in, e := readBody(w, r, 128*1024)
	if e != nil {
		return e
	}
	if r.URL.Path == "/api/admin/playback-resolvers/test" && r.Method == "POST" {
		c, e := decodePlayback(in["config"])
		if e != nil {
			return e
		}
		if !c.Enabled {
			return problem(400, "请先在测试配置中开启轮询解析")
		}
		p := str(in["platform"])
		if !has(platforms, p) {
			return problem(400, "客户端无效")
		}
		result, e := a.resolvePlayback(r.Context(), c, str(in["url"]), p, nil)
		if e != nil {
			return e
		}
		jsonResponse(w, 200, result)
		return nil
	}
	if !(r.URL.Path == "/api/admin/playback-resolvers" && r.Method == "PUT") && !(r.URL.Path == "/api/admin/playback-resolvers/publish" && r.Method == "POST") {
		return problem(405, "请求方法不支持")
	}
	var draft playbackConfig
	if r.Method == "PUT" {
		draft, e = decodePlayback(in["config"])
		if e != nil {
			return e
		}
	}
	e = a.change(r.Context(), "playback.resolvers."+strings.ToLower(r.Method), func(v *state) error {
		current := playbackState(*v)
		if number(in["revision"]) != current.Revision {
			return problem(409, "解析配置已被修改，请重新加载后再试")
		}
		if r.Method == "PUT" {
			current.Draft = draft
		} else {
			if e := validatePlayback(current.Draft); e != nil {
				return e
			}
			current.Published = clone(current.Draft)
			current.PublishedVersion++
			current.PublishedAt = now()
		}
		current.Revision++
		v.Playback = &current
		v.UpdatedAt = now()
		return nil
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 200, playbackState(a.readState()))
	return nil
}
func resolverExcludes(raw any, c playbackConfig) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok || len(values) > 32 {
		return nil, problem(400, "已尝试来源列表无效")
	}
	ids := map[string]bool{"direct": true}
	for _, s := range c.Sources {
		ids[s.ID] = true
	}
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		id, ok := value.(string)
		if !ok || !ids[id] || seen[id] {
			return nil, problem(400, "已尝试来源无效或配置已更新，请重新发起播放")
		}
		seen[id] = true
		result = append(result, id)
	}
	return result, nil
}
func isDirectMedia(u *url.URL) bool {
	p := strings.ToLower(u.Path)
	for _, ext := range []string{".m3u8", ".mp4", ".mpd", ".webm", ".mov", ".m4v"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}
func (a *app) resolvePlayback(ctx context.Context, c playbackConfig, raw, p string, excluded []string) (playbackResolution, error) {
	result := playbackResolution{Attempts: []resolverAttempt{}}
	source, e := mediaHTTPURL(raw)
	if e != nil {
		return result, e
	}
	if !c.Enabled {
		result.OK = true
		result.URL = source.String()
		result.SourceID = "direct"
		result.Kind = "direct"
		result.Message = "轮询未启用，沿用 CMS 授权地址"
		return result, nil
	}
	if c.DirectFirst && isDirectMedia(source) && !has(excluded, "direct") {
		result.OK = true
		result.URL = raw
		result.SourceID = "direct"
		result.Kind = "direct"
		result.Message = "视频直链优先，实际可播放性由客户端确认"
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.TotalSeconds)*time.Second)
	defer cancel()
	consumed := 0
	for _, id := range excluded {
		if id != "direct" {
			consumed++
		}
	}
	for _, s := range c.Sources {
		if !s.Enabled || !has(s.Platforms, p) || has(excluded, s.ID) {
			continue
		}
		if consumed+len(result.Attempts) >= c.MaxAttempts || ctx.Err() != nil {
			break
		}
		started := time.Now()
		attemptCtx, stop := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
		resolved, message := a.callPlaybackResolver(attemptCtx, s, raw)
		stop()
		attempt := resolverAttempt{ID: s.ID, Name: s.Name, OK: resolved != "", ElapsedMS: time.Since(started).Milliseconds(), Message: message}
		result.Attempts = append(result.Attempts, attempt)
		if resolved != "" {
			result.OK = true
			result.URL = resolved
			result.SourceID = s.ID
			result.Kind = s.Kind
			result.Message = "已取得资源地址，等待客户端加载确认"
			return result, nil
		}
	}
	result.Message = "没有可用解析结果：请检查来源、客户端范围或尝试上限"
	if ctx.Err() != nil {
		result.Message = "解析已达到总时限或请求已取消"
	}
	return result, nil
}
func (a *app) callPlaybackResolver(ctx context.Context, s playbackSource, raw string) (string, string) {
	target := strings.Replace(s.Endpoint, "{url}", url.QueryEscape(raw), 1)
	req, e := http.NewRequestWithContext(ctx, "GET", target, nil)
	if e != nil {
		return "", "解析请求配置无效"
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Hetun-Playback/1")
	// Reuse outbound limits, DNS/IP checks and no-redirect policy; no member/admin cookies.
	response, e := a.cms.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return "", "解析超时或请求已取消"
		}
		return "", "解析连接失败，请检查地址和网络策略"
	}
	defer response.Body.Close()
	if s.Kind == "redirect" {
		if !has([]string{"301", "302", "303", "307", "308"}, fmt.Sprint(response.StatusCode)) {
			return "", fmt.Sprintf("需要 HTTP 跳转，实际返回 %d", response.StatusCode)
		}
		location, e := response.Location()
		if e != nil {
			return "", "跳转响应缺少有效 Location"
		}
		if _, e = mediaHTTPURL(location.String()); e != nil {
			return "", "跳转资源地址无效"
		}
		return location.String(), "HTTP 跳转解析成功"
	}
	if response.StatusCode != 200 {
		return "", fmt.Sprintf("解析服务返回 HTTP %d", response.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if e != nil {
		return "", "解析响应读取失败"
	}
	if len(data) > 2*1024*1024 {
		return "", "解析响应超过2MB限制"
	}
	var value any
	if e = json.Unmarshal(data, &value); e != nil {
		return "", "返回内容不是有效 JSON"
	}
	for _, field := range strings.Split(s.URLField, ".") {
		value = obj(value)[field]
	}
	address, ok := value.(string)
	if !ok {
		return "", "响应中没有配置的视频地址字段"
	}
	if _, e = mediaHTTPURL(address); e != nil {
		return "", "解析返回的资源地址无效"
	}
	return address, "JSON 解析成功"
}
