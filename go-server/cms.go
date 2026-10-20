package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func cmsEndpoint(base string) (*url.URL, error) {
	u, e := url.Parse(strings.TrimSpace(base))
	if e != nil || u.Host == "" || !has([]string{"http", "https"}, u.Scheme) || u.User != nil || u.Fragment != "" {
		return nil, problem(400, "请输入完整的 HTTP(S) 苹果 CMS 地址")
	}
	if !strings.Contains(u.Path, "/api.php/provide/vod") {
		u.Path = strings.TrimSuffix(u.Path, "/") + "/api.php/provide/vod/"
	}
	u.RawQuery = ""
	return u, nil
}
func siteURL(base string) (*url.URL, error) {
	u, e := cmsEndpoint(base)
	if e != nil {
		return nil, e
	}
	u.Path = strings.Split(u.Path, "/api.php/provide/vod")[0] + "/"
	return u, nil
}
func forbiddenIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return true
	}
	if v := ip.To4(); v != nil {
		return v[0] == 0 || v[0] >= 224 || (v[0] == 100 && v[1] >= 64 && v[1] <= 127)
	}
	return len(ip) != 16 || ip[0]&0xe0 != 0x20
}
func cmsClient(allowPrivate bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 32
	transport.MaxConnsPerHost = 64
	transport.IdleConnTimeout = 60 * time.Second
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("CMS 域名无法解析")
		}
		for _, ip := range ips {
			if !allowPrivate && forbiddenIP(ip.IP) {
				return nil, problem(400, "CMS 地址指向内网；需管理员明确启用 allowPrivateCMS")
			}
		}
		var last error
		for _, ip := range ips {
			conn, e := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}
	return &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}
func cmsFetch(ctx context.Context, client *http.Client, u *url.URL, method string, params url.Values, jar http.CookieJar) (M, error) {
	target := *u
	var body io.Reader
	if method == "GET" {
		target.RawQuery = params.Encode()
	} else {
		body = strings.NewReader(params.Encode())
	}
	req, e := http.NewRequestWithContext(ctx, method, target.String(), body)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Hetun/Go")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", target.Scheme+"://"+target.Host)
	if method != "GET" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if jar != nil {
		for _, c := range jar.Cookies(&target) {
			req.AddCookie(c)
			if c.Name == "csrf_token" {
				req.Header.Set("X-CSRF-Token", c.Value)
			}
		}
	}
	res, e := client.Do(req)
	if e != nil {
		return nil, fmt.Errorf("CMS 连接失败: %w", e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, problem(400, fmt.Sprintf("CMS 返回 HTTP %d", res.StatusCode))
	}
	if jar != nil {
		jar.SetCookies(&target, res.Cookies())
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, 8*1024*1024+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 8*1024*1024 {
		return nil, problem(400, "CMS 响应过大")
	}
	var v M
	if e = json.Unmarshal([]byte(strings.TrimPrefix(string(b), "\ufeff")), &v); e != nil {
		text := strings.TrimSpace(strings.TrimPrefix(string(b), "\ufeff"))
		if strings.EqualFold(text, "closed") {
			return nil, problem(400, "苹果 CMS 返回 closed：请在苹果 CMS 后台开启视频采集接口，并检查接口访问限制后重试")
		}
		if strings.HasPrefix(text, "<") {
			return nil, problem(400, "CMS 返回网页而非 JSON（HTTP 200）；请检查接口地址、站点拦截或登录验证")
		}
		return nil, problem(400, fmt.Sprintf("CMS 未返回兼容 JSON（HTTP 200，响应类型 %s）；请检查采集接口设置", res.Header.Get("Content-Type")))
	}
	return v, nil
}
func (a *app) catalogRaw(ctx context.Context, base string, params url.Values) (M, error) {
	u, e := cmsEndpoint(base)
	if e != nil {
		return nil, e
	}
	v, e := cmsFetch(ctx, a.cms, u, "GET", params, nil)
	if e != nil {
		return nil, e
	}
	key := "list"
	if params.Get("ac") == "list" {
		key = "class"
	}
	if _, ok := v[key].([]any); !ok {
		return nil, problem(400, fmt.Sprintf("CMS 缺少 %s 数组", key))
	}
	if c, ok := v["code"]; ok && numericOr(c, 0) != 1 {
		return nil, problem(400, "CMS 返回失败状态："+clean(v["msg"]))
	}
	return v, nil
}
func (a *app) checkCMS(ctx context.Context, base string) M {
	begin := time.Now()
	report := M{"base": "", "configured": strings.TrimSpace(base) != "", "ok": false, "status": "unconfigured", "checkedAt": now(), "checks": []any{}, "latencyMs": int64(0), "message": "尚未配置苹果 CMS"}
	if strings.TrimSpace(base) == "" {
		return report
	}
	u, e := cmsEndpoint(base)
	if e != nil {
		report["status"] = "error"
		report["message"] = e.Error()
		return report
	}
	report["base"] = u.String()
	checks := make([]any, 2)
	var wg sync.WaitGroup
	for index, kind := range []string{"categories", "catalog"} {
		wg.Add(1)
		go func(index int, kind string) {
			defer wg.Done()
			start := time.Now()
			params := url.Values{"ac": {"list"}}
			label := "分类接口"
			if kind == "catalog" {
				params = url.Values{"ac": {"detail"}, "pg": {"1"}}
				label = "影片接口"
			}
			v, e := a.catalogRaw(ctx, u.String(), params)
			c := M{"id": kind, "label": label, "ok": e == nil, "latencyMs": time.Since(start).Milliseconds()}
			target := *u
			target.RawQuery = params.Encode()
			c["url"] = target.String()
			if e == nil {
				key := "list"
				if kind == "categories" {
					key = "class"
				}
				xs, ok := v[key].([]any)
				if !ok {
					e = fmt.Errorf("CMS 缺少 %s 数组", key)
				} else {
					for _, raw := range xs {
						m := obj(raw)
						if kind == "categories" && (m["type_id"] == nil || m["type_name"] == nil) {
							e = fmt.Errorf("分类结构无效")
						}
						if kind == "catalog" && (m["vod_id"] == nil || m["vod_name"] == nil) {
							e = fmt.Errorf("影片结构无效")
						}
					}
					c["count"] = len(xs)
					if kind == "catalog" {
						c["total"] = v["total"]
					}
				}
			}
			if e != nil {
				c["ok"] = false
				c["error"] = e.Error()
			}
			checks[index] = c
		}(index, kind)
	}
	wg.Wait()
	ready, empty := true, false
	for _, raw := range checks {
		c := obj(raw)
		if c["ok"] != true {
			ready = false
		}
		if c["ok"] == true && c["count"] == 0 {
			empty = true
		}
	}
	report["checks"] = checks
	report["ok"] = ready
	report["status"] = "error"
	failures := []string{}
	for _, raw := range checks {
		c := obj(raw)
		if c["ok"] != true {
			failures = append(failures, str(c["label"])+"："+str(c["error"]))
		}
	}
	report["message"] = strings.Join(failures, "；")
	if ready {
		report["status"] = "ready"
		report["message"] = "分类与影片接口检测通过"
		if empty {
			report["status"] = "empty"
			report["message"] = "接口已连接，但分类或影片为空"
		}
	}
	report["latencyMs"] = time.Since(begin).Milliseconds()
	return report
}

var tagPattern = regexp.MustCompile(`<[^>]*>`)

func clean(v any) string { return html.UnescapeString(tagPattern.ReplaceAllString(str(v), "")) }
func imageURL(value any, base string) string {
	if str(value) == "" {
		return ""
	}
	u, e := url.Parse(base)
	if e != nil {
		return ""
	}
	r, e := url.Parse(str(value))
	if e != nil {
		return ""
	}
	u = u.ResolveReference(r)
	if !has([]string{"http", "https"}, u.Scheme) {
		return ""
	}
	return u.String()
}
func normalizeVod(v M, base string) M {
	out := M{"id": str(v["vod_id"]), "poster": imageURL(v["vod_pic"], base), "protected": true, "categoryId": str(v["type_id"])}
	for k, source := range map[string]string{"title": "vod_name", "category": "type_name", "year": "vod_year", "area": "vod_area", "score": "vod_score", "actors": "vod_actor", "director": "vod_director"} {
		out[k] = clean(v[source])
	}
	note := v["vod_remarks"]
	if str(note) == "" {
		note = v["vod_state"]
	}
	out["note"] = clean(note)
	desc := v["vod_content"]
	if str(desc) == "" {
		desc = v["vod_blurb"]
	}
	out["description"] = clean(desc)
	groups := strings.Split(str(v["vod_play_url"]), "$$$")
	labels := strings.Split(str(v["vod_play_from"]), "$$$")
	sources := []any{}
	for i, g := range groups {
		episodes := []any{}
		for n, episode := range strings.Split(g, "#") {
			if episode == "" {
				continue
			}
			name, _, found := strings.Cut(episode, "$")
			if !found {
				name = fmt.Sprintf("第 %d 集", n+1)
			}
			episodes = append(episodes, M{"name": name, "url": ""})
		}
		if len(episodes) > 0 {
			name := fmt.Sprintf("播放线路 %d", i+1)
			if i < len(labels) && labels[i] != "" {
				name = labels[i]
			}
			sources = append(sources, M{"name": name, "episodes": episodes})
		}
	}
	out["sources"] = sources
	return out
}

var digits = regexp.MustCompile(`^\d+$`)

func (a *app) catalog(ctx context.Context, base string, q url.Values) (M, error) {
	if len(q.Get("keyword")) > 300 || len(q.Get("category")) > 60 || len(q.Get("ids")) > 1500 {
		return nil, problem(400, "检索参数过长")
	}
	order := q.Get("orderBy")
	if order == "" {
		order = "latest"
	}
	if !has([]string{"latest", "manual", "score"}, order) {
		return nil, problem(400, "不支持全库热度排序或该排序方式")
	}
	ids := []string{}
	for _, id := range strings.Split(q.Get("ids"), ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			if !digits.MatchString(id) {
				return nil, problem(400, "影片 ID 必须为数字")
			}
			ids = append(ids, id)
		}
	}
	if len(ids) > 60 {
		return nil, problem(400, "影片 ID 最多60个")
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		page = 10000
	}
	raw, e := a.cachedCatalog(ctx, base, url.Values{"ac": {"detail"}, "pg": {strconv.Itoa(page)}, "wd": {q.Get("keyword")}, "t": {q.Get("category")}, "ids": {strings.Join(ids, ",")}})
	if e != nil {
		return nil, e
	}
	items := []any{}
	for _, v := range arr(raw["list"]) {
		items = append(items, normalizeVod(obj(v), base))
	}
	if len(ids) > 0 {
		ordered := []any{}
		for _, id := range ids {
			for _, v := range items {
				if obj(v)["id"] == id {
					ordered = append(ordered, v)
					break
				}
			}
		}
		items = ordered
	}
	if order == "score" {
		sort.SliceStable(items, func(i, j int) bool {
			x, _ := strconv.ParseFloat(str(obj(items[i])["score"]), 64)
			y, _ := strconv.ParseFloat(str(obj(items[j])["score"]), 64)
			return x > y
		})
	}
	return M{"items": items, "page": numericOr(raw["page"], 1), "pages": numericOr(raw["pagecount"], 1), "total": numericOr(raw["total"], len(items))}, nil
}
func numericOr(v any, fallback int) int {
	n, _ := strconv.Atoi(str(v))
	if n < 1 {
		return fallback
	}
	return n
}
