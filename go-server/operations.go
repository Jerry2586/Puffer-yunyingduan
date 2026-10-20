package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed default-theme.json
var defaultBytes []byte

//go:embed web
var embeddedWeb embed.FS
var platforms = []string{"mobile", "tablet", "tv", "web"}

const contract = "hetun-operations-v1"

var defaults = func() M {
	var v M
	if e := json.Unmarshal(defaultBytes, &v); e != nil {
		panic(e)
	}
	return v
}()
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var versionPattern = regexp.MustCompile(`^[0-9]{1,9}\.[0-9]{1,9}\.[0-9]{1,9}$`)
var assetPattern = regexp.MustCompile(`^/api/assets/[a-f0-9]{64}\.(png|jpg|webp|mp4)$`)
var pages = []string{"home", "category", "search", "shortdrama", "member", "mine", "ranking", "schedule", "library", "history", "settings", "disclaimer", "privacy", "about"}

func has(xs []string, s string) bool {
	for _, v := range xs {
		if v == s {
			return true
		}
	}
	return false
}
func asset(s string) bool {
	if s == "" || assetPattern.MatchString(s) {
		return true
	}
	u, e := url.Parse(s)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}
func platform(s string) (string, error) {
	s = strings.TrimPrefix(s, "runtime-")
	if s == "" {
		s = "mobile"
	}
	if s == "desktop" {
		s = "web"
	}
	if !has(platforms, s) {
		return "", problem(400, "请选择手机、平板、电视或电脑端")
	}
	return s, nil
}
func requestPlatform(r *http.Request) (string, error) {
	p := clientScope(r)
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return "", problem(400, "查询参数格式无效")
	}
	query := values["platform"]
	headers := r.Header.Values("X-Hetun-Platform")
	if len(query) > 1 || len(headers) > 1 {
		return "", problem(400, "客户端标识不能重复")
	}
	for _, source := range [][]string{query, headers} {
		for _, value := range source {
			if value == "" {
				return "", problem(400, "客户端标识不能为空")
			}
			candidate, err := platform(value)
			if err != nil {
				return "", err
			}
			if p != "" && candidate != p {
				return "", problem(400, "客户端路径、参数与平台标识不一致")
			}
			p = candidate
		}
	}
	if p == "" {
		p = "mobile"
	}
	return p, nil
}
func operationConfig(input M) (M, error) {
	out := M{}
	fields := map[string][]string{"sections": {"id", "enabled", "page", "title", "subtitle", "source", "categoryId", "limit", "orderBy", "manualIds"}, "navigation": {"id", "label", "enabled", "page", "categoryId"}, "categories": {"id", "label", "enabled", "page", "categoryId"}, "ads": {"id", "name", "slot", "enabled", "media", "asset", "page", "url", "start", "end", "duration", "skipAfter", "frequency"}, "schedule": {"id", "vodId", "title", "date"}}
	for key, keys := range fields {
		value, exists := input[key]
		if !exists {
			value = defaults[key]
		}
		xs, ok := value.([]any)
		if !ok {
			return nil, problem(400, key+" 必须为数组")
		}
		max := 20
		if key == "sections" {
			max = 40
		}
		if key == "schedule" {
			max = 100
		}
		if len(xs) > max {
			return nil, problem(400, key+" 数量超过限制")
		}
		clean := []any{}
		seen := map[string]bool{}
		for _, raw := range xs {
			m := obj(raw)
			id := str(m["id"])
			if !idPattern.MatchString(id) || seen[id] {
				return nil, problem(400, "条目编号无效或重复")
			}
			seen[id] = true
			n := pick(m, keys...)
			if _, ok := n["enabled"]; !ok && key != "schedule" {
				n["enabled"] = true
			}
			if enabled, exists := n["enabled"]; exists {
				if _, ok := enabled.(bool); !ok {
					return nil, problem(400, "启用状态必须为布尔值")
				}
			}
			for _, k := range []string{"title", "subtitle", "name", "label", "categoryId", "manualIds", "vodId"} {
				if value, exists := n[k]; exists {
					if _, ok := value.(string); !ok {
						return nil, problem(400, "运营文字字段格式无效")
					}
					if len(str(value)) > 6000 {
						return nil, problem(400, "运营文字字段过长")
					}
				}
			}
			if p := str(n["page"]); p != "" && !has(pages, p) {
				return nil, problem(400, "页面编号无效")
			}
			for _, k := range []string{"asset", "url"} {
				if !asset(str(n[k])) {
					return nil, problem(400, "素材地址无效")
				}
			}
			if key == "sections" {
				limit := number(n["limit"])
				if limit < 1 || limit > 60 {
					return nil, problem(400, "栏目数量须为1–60")
				}
				if !has([]string{"latest", "hot", "score", "manual"}, str(n["orderBy"])) {
					n["orderBy"] = "latest"
				}
				if !has([]string{"latest", "category"}, str(n["source"])) {
					n["source"] = "latest"
				}
			}
			if key == "ads" {
				for _, k := range []string{"start", "end"} {
					if value := str(n[k]); value != "" {
						parsed, err := parseAdTime(value)
						if err != nil {
							return nil, problem(400, "广告排期无效")
						}
						n[k] = parsed.UTC().Format(time.RFC3339)
					}
				}
				if str(n["start"]) != "" && str(n["end"]) != "" {
					start, _ := parseAdTime(str(n["start"]))
					end, _ := parseAdTime(str(n["end"]))
					if !end.After(start) {
						return nil, problem(400, "广告结束时间必须晚于开始时间")
					}
				}
				choices := map[string][]string{"slot": {"startup", "home", "detail", "player", "shortdrama", "mine"}, "media": {"image", "video"}, "frequency": {"session", "always"}}
				fallbacks := map[string]string{"slot": "home", "media": "image", "frequency": "session"}
				for key, values := range choices {
					if !has(values, str(n[key])) {
						n[key] = fallbacks[key]
					}
				}
				for key, bounds := range map[string][3]int{"duration": {3, 30, 5}, "skipAfter": {0, 30, 3}} {
					value := number(n[key])
					if _, exists := n[key]; !exists {
						value = bounds[2]
					}
					if value < bounds[0] {
						value = bounds[0]
					}
					if value > bounds[1] {
						value = bounds[1]
					}
					n[key] = value
				}
			}
			if key == "schedule" {
				if _, e := time.Parse("2006-01-02", str(n["date"])); e != nil {
					return nil, problem(400, "排期日期无效")
				}
			}
			clean = append(clean, n)
		}
		out[key] = clean
	}
	features := clone(obj(defaults["features"]))
	for k := range features {
		if v, ok := obj(input["features"])[k].(bool); ok {
			features[k] = v
		}
	}
	out["features"] = features
	settings := clone(obj(defaults["clientSettings"]))
	for k := range settings {
		if v, ok := obj(input["clientSettings"])[k]; ok {
			switch settings[k].(type) {
			case bool:
				if _, ok := v.(bool); !ok {
					return nil, problem(400, "客户端开关格式无效")
				}
			default:
				if _, ok := v.(string); !ok {
					return nil, problem(400, "客户端文字格式无效")
				}
			}
			settings[k] = v
		}
	}
	if v := str(settings["latestVersion"]); v != "" && !versionPattern.MatchString(v) {
		return nil, problem(400, "更新版本号格式为 x.y.z")
	}
	for _, k := range []string{"androidUrl", "iosUrl"} {
		if !asset(str(settings[k])) {
			return nil, problem(400, "更新地址无效")
		}
	}
	if settings["forceUpdate"] == true && (str(settings["latestVersion"]) == "" || (str(settings["androidUrl"]) == "" && str(settings["iosUrl"]) == "")) {
		return nil, problem(400, "强制更新需填写版本和下载地址")
	}
	out["clientSettings"] = settings
	ann := clone(obj(defaults["announcement"]))
	for _, k := range []string{"enabled", "text", "title", "image", "frequency", "actionLabel", "page", "url"} {
		if v, ok := obj(input["announcement"])[k]; ok {
			ann[k] = v
		}
	}
	if _, ok := ann["enabled"].(bool); !ok {
		return nil, problem(400, "公告开关格式无效")
	}
	for _, key := range []string{"text", "title", "image", "frequency", "actionLabel", "page", "url"} {
		if _, ok := ann[key].(string); !ok {
			return nil, problem(400, "公告文字格式无效")
		}
	}
	if !has([]string{"launch", "daily", "once"}, str(ann["frequency"])) || !has(pages, str(ann["page"])) {
		return nil, problem(400, "公告频率或页面无效")
	}
	out["announcement"] = pick(ann, "enabled", "text", "title", "image", "frequency", "actionLabel", "page", "url")
	for _, k := range []string{"image", "url"} {
		if !asset(str(ann[k])) {
			return nil, problem(400, "公告地址无效")
		}
	}
	slides := arr(obj(input["carousel"])["slides"])
	if len(slides) > 20 {
		return nil, problem(400, "轮播最多20项")
	}
	clean := []any{}
	seen := map[string]bool{}
	for _, raw := range slides {
		m := obj(raw)
		id := str(m["id"])
		if !idPattern.MatchString(id) || seen[id] {
			return nil, problem(400, "轮播编号无效或重复")
		}
		seen[id] = true
		if p := str(m["page"]); p != "" && !has(pages, p) {
			return nil, problem(400, "轮播页面无效")
		}
		for _, k := range []string{"image", "video"} {
			if !asset(str(m[k])) {
				return nil, problem(400, "轮播素材无效")
			}
		}
		clean = append(clean, pick(m, "id", "vodId", "title", "image", "video", "page"))
	}
	out["carousel"] = M{"slides": clean}
	return out, nil
}
func mergeOperations(config M) M {
	out := clone(defaults)
	for k, v := range config {
		switch k {
		case "sections", "navigation", "categories":
			list := []any{}
			base := arr(defaults[k])
			for _, raw := range arr(v) {
				m := obj(raw)
				item := M{}
				if len(base) > 0 {
					item = clone(obj(base[0]))
				}
				for _, b := range base {
					if obj(b)["id"] == m["id"] {
						item = clone(obj(b))
						break
					}
				}
				for key, value := range m {
					item[key] = value
				}
				list = append(list, item)
			}
			out[k] = list
		case "carousel", "announcement":
			item := obj(out[k])
			for key, value := range obj(v) {
				item[key] = value
			}
			out[k] = item
		default:
			out[k] = clone(v)
		}
	}
	return out
}
func initialState() state {
	v := state{UpdatedAt: now(), Operations: map[string]*record{}, Setup: M{}, Revision: 1}
	config, err := operationConfig(defaults)
	if err != nil {
		panic(err)
	}
	for _, p := range platforms {
		v.Operations[p] = &record{Version: 1, PublishedVersion: 1, Draft: clone(config), Published: clone(config), PublishedAt: now(), History: []snapshot{}}
	}
	return v
}
func studio(v state, p string) M {
	names := map[string]string{"mobile": "手机", "tablet": "平板", "tv": "电视", "web": "电脑"}
	templates := []any{}
	active := M{}
	for _, key := range platforms {
		r := v.Operations[key]
		templates = append(templates, M{"id": "runtime-" + key, "name": names[key] + "端运营配置", "platform": key, "version": r.Version, "publishedVersion": r.PublishedVersion, "draft": mergeOperations(r.Draft), "published": mergeOperations(r.Published)})
		active[key] = "runtime-" + key
	}
	r := v.Operations[p]
	out := M{"templates": templates, "activeTemplates": active, "selectedTemplate": "runtime-" + p, "cmsBase": v.CMSBase, "updatedAt": v.UpdatedAt, "publishedAt": r.PublishedAt, "contract": contract, "version": r.Version, "publishedVersion": r.PublishedVersion, "draft": mergeOperations(r.Draft), "published": mergeOperations(r.Published)}
	history := []any{}
	for _, h := range r.History {
		history = append(history, M{"version": h.Version, "publishedAt": h.PublishedAt})
	}
	out["history"] = history
	return out
}
func mutateOperations(v *state, input M, action string) error {
	p, e := platform(str(input["template"]))
	if e != nil {
		return e
	}
	r := v.Operations[p]
	if number(input["version"]) != r.Version {
		return problem(409, "此端配置已被修改，请重新加载")
	}
	switch action {
	case "draft":
		m, ok := input["theme"].(map[string]any)
		if !ok {
			return problem(400, "运营配置必须为对象")
		}
		clean, e := operationConfig(m)
		if e != nil {
			return e
		}
		for _, k := range []string{"androidUrl", "iosUrl"} {
			if u := str(obj(clean["clientSettings"])[k]); u != "" && !strings.HasPrefix(strings.ToLower(u), "https://") {
				return problem(400, "更新地址必须使用 HTTPS")
			}
		}
		r.Draft = clean
		r.Version++
	case "publish", "rollback":
		if action == "rollback" {
			var found M
			for _, h := range r.History {
				if h.Version == number(input["targetVersion"]) {
					found = h.Config
					break
				}
			}
			if found == nil {
				return problem(404, "历史版本不存在")
			}
			r.Draft = clone(found)
		}
		r.History = append([]snapshot{{r.PublishedVersion, r.PublishedAt, clone(r.Published)}}, r.History...)
		if len(r.History) > 20 {
			r.History = r.History[:20]
		}
		r.Published = clone(r.Draft)
		r.PublishedVersion++
		r.Version++
		r.PublishedAt = now()
	default:
		return problem(404, "操作不存在")
	}
	v.UpdatedAt = now()
	return nil
}
func newer(a, b string) bool {
	if !versionPattern.MatchString(a) || !versionPattern.MatchString(b) {
		return false
	}
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range aa {
		x, _ := strconv.ParseUint(aa[i], 10, 64)
		y, _ := strconv.ParseUint(bb[i], 10, 64)
		if x != y {
			return x > y
		}
	}
	return false
}
func bootstrap(v state, p, os, version, origin string) (M, error) {
	if !has([]string{"android", "ios", "windows", "web"}, os) {
		return nil, problem(400, "不支持的客户端系统")
	}
	if version != "" && !versionPattern.MatchString(version) {
		return nil, problem(400, "客户端版本必须为三段数字")
	}
	r := v.Operations[p]
	config := clone(r.Published)
	settings := obj(config["clientSettings"])
	applyAppRelease(v, p, os, settings)
	download := ""
	if os == "android" {
		download = str(settings["androidUrl"])
	}
	if os == "ios" {
		download = str(settings["iosUrl"])
	}
	if os == "windows" || os == "web" {
		download = str(settings["downloadUrl"])
	}
	if !strings.HasPrefix(strings.ToLower(download), "https://") {
		download = ""
	}
	available := download != "" && newer(str(settings["latestVersion"]), version)
	return M{"schemaVersion": 1, "contract": contract, "themeContract": "hetun-runtime-v1", "platform": p, "os": os, "version": r.PublishedVersion, "publishedAt": r.PublishedAt, "config": config, "serviceBase": origin, "pollSeconds": 15, "cmsConfigured": v.CMSBase != "", "update": M{"available": available, "required": available && settings["forceUpdate"] == true, "version": settings["latestVersion"], "notes": settings["updateNotes"], "url": download, "browserUrl": settings["browserUrl"], "sizeMB": settings["sizeMB"], "releaseId": settings["releaseId"]}}, nil
}
func validateState(v state) error {
	for _, p := range platforms {
		r := v.Operations[p]
		if r == nil || r.Version < 1 || r.PublishedVersion < 1 {
			return fmt.Errorf("运营数据缺少平台 %s", p)
		}
		for _, c := range []M{r.Draft, r.Published} {
			if _, e := operationConfig(c); e != nil {
				return e
			}
		}
	}
	return nil
}

func parseAdTime(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("广告日期格式无效")
}
