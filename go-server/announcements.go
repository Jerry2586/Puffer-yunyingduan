package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Announcements are shared by all clients. Client operations retain their other
// independent settings, but can never override this collection.
type announcementEntry struct {
	ID        string   `json:"id"`
	Config    M        `json:"config"`
	Sources   []string `json:"sources,omitempty"`
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}
type announcementState struct {
	Version   int                 `json:"version"`
	UpdatedAt string              `json:"updatedAt"`
	Items     []announcementEntry `json:"items"`
}

func announcementConfig(input M) (M, error) {
	ann := clone(obj(defaults["announcement"]))
	for _, key := range []string{"enabled", "text", "title", "style", "image", "frequency", "actionLabel", "page", "url"} {
		if value, ok := input[key]; ok {
			ann[key] = value
		}
	}
	if _, ok := ann["enabled"].(bool); !ok {
		return nil, problem(400, "公告开关格式无效")
	}
	for _, key := range []string{"text", "title", "style", "image", "frequency", "actionLabel", "page", "url"} {
		if _, ok := ann[key].(string); !ok {
			return nil, problem(400, "公告文字格式无效")
		}
	}
	if !has([]string{"strip", "ticker", "card", "modal"}, str(ann["style"])) || !has([]string{"launch", "daily", "once"}, str(ann["frequency"])) || !has(pages, str(ann["page"])) {
		return nil, problem(400, "公告样式、频率或页面无效")
	}
	for _, key := range []string{"image", "url"} {
		if !asset(str(ann[key])) {
			return nil, problem(400, "公告地址无效")
		}
	}
	return ann, nil
}
func validateAnnouncementInput(ann M) error {
	for key, limit := range map[string]int{"title": 120, "text": 8000, "image": 2048, "url": 2048, "actionLabel": 40} {
		if utf8.RuneCountInString(str(ann[key])) > limit {
			return problem(400, "公告标题、正文或链接超出长度限制")
		}
	}
	target := str(ann["url"])
	if target != "" {
		parsed, err := url.Parse(target)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || strings.ContainsAny(target, "\\") || strings.IndexFunc(target, unicode.IsSpace) >= 0 {
			return problem(400, "跳转链接必须为完整的 HTTP 或 HTTPS 地址")
		}
	}
	if strings.TrimSpace(str(ann["title"])) == "" {
		return problem(400, "请填写公告标题")
	}
	if ann["enabled"] == true && strings.TrimSpace(str(ann["text"])) == "" {
		return problem(400, "请先填写公告内容再开启")
	}
	return nil
}

// Import all distinct legacy draft/published announcements exactly once. The
// first enabled published announcement remains active; unpublished drafts never
// become active during migration. The original platform documents are retained.
func migrateAnnouncements(v *state) bool {
	if v.Announcements != nil {
		return false
	}
	shared := &announcementState{Version: 1, UpdatedAt: now(), Items: []announcementEntry{}}
	v.Announcements = shared
	seen := map[string]int{}
	active := ""
	names := map[string]string{"mobile": "手机", "tablet": "平板", "tv": "电视", "web": "电脑"}
	for _, p := range platforms {
		r := v.Operations[p]
		if r == nil {
			continue
		}
		for _, source := range []struct {
			name      string
			config    M
			published bool
		}{{"已发布", r.Published, true}, {"草稿", r.Draft, false}} {
			cfg, err := announcementConfig(obj(source.config["announcement"]))
			if err != nil {
				continue
			}
			raw, _ := json.Marshal(cfg)
			preset, _ := announcementConfig(nil)
			presetRaw, _ := json.Marshal(preset)
			if string(raw) == string(presetRaw) && cfg["enabled"] != true {
				continue
			}
			enabled := cfg["enabled"] == true
			cfg["enabled"] = false
			fingerprint, _ := json.Marshal(cfg)
			sum := sha256.Sum256(fingerprint)
			id := "notice-" + hex.EncodeToString(sum[:12])
			index, ok := seen[id]
			if !ok {
				index = len(shared.Items)
				seen[id] = index
				stamp := r.PublishedAt
				if stamp == "" {
					stamp = shared.UpdatedAt
				}
				shared.Items = append(shared.Items, announcementEntry{ID: id, Config: cfg, Sources: []string{}, CreatedAt: stamp, UpdatedAt: stamp})
			}
			shared.Items[index].Sources = append(shared.Items[index].Sources, names[p]+" · "+source.name)
			if source.published && enabled && active == "" {
				active = id
			}
		}
	}
	for i := range shared.Items {
		shared.Items[i].Config["enabled"] = shared.Items[i].ID == active
	}
	// Migration also changes the public projection. Advance every client so that
	// version-gated clients and caches fetch the newly shared configuration.
	for _, p := range platforms {
		if r := v.Operations[p]; r != nil {
			r.PublishedVersion++
			r.PublishedAt = shared.UpdatedAt
		}
	}
	v.UpdatedAt = shared.UpdatedAt
	return true
}
func currentAnnouncement(v state) M {
	if v.Announcements == nil {
		v = clone(v)
		migrateAnnouncements(&v)
	}
	for _, entry := range v.Announcements.Items {
		if entry.Config["enabled"] == true {
			return clone(entry.Config)
		}
	}
	cfg, _ := announcementConfig(nil)
	cfg["enabled"] = false
	cfg["text"] = ""
	return cfg
}
func withSharedAnnouncement(v state, config M) M {
	out := clone(config)
	out["announcement"] = currentAnnouncement(v)
	return out
}
func validateAnnouncements(shared *announcementState) error {
	if shared == nil {
		return nil
	}
	if shared.Version < 1 || len(shared.Items) > 200 {
		return problem(400, "共享公告数据无效")
	}
	seen := map[string]bool{}
	active := 0
	for _, entry := range shared.Items {
		if !idPattern.MatchString(entry.ID) || seen[entry.ID] {
			return problem(400, "共享公告标识无效")
		}
		seen[entry.ID] = true
		cfg, err := announcementConfig(entry.Config)
		if err != nil {
			return err
		}
		if cfg["enabled"] == true {
			active++
		}
	}
	if active > 1 {
		return problem(400, "只能开启一条共享公告")
	}
	return nil
}
func announcementsReport(v state) *announcementState {
	if v.Announcements == nil {
		v = clone(v)
		migrateAnnouncements(&v)
	}
	return v.Announcements
}
func mutateAnnouncements(v *state, input M) error {
	migrateAnnouncements(v)
	shared := v.Announcements
	version, ok := input["version"].(float64)
	if !ok || version != float64(shared.Version) {
		return problem(409, "公告已被其他管理员修改，请重新加载后再编辑")
	}
	action := str(input["action"])
	id := str(input["id"])
	index := -1
	for i := range shared.Items {
		if shared.Items[i].ID == id {
			index = i
			break
		}
	}
	if action != "create" && index < 0 {
		return problem(404, "公告不存在")
	}
	switch action {
	case "create", "update":
		raw, ok := input["config"].(map[string]any)
		if !ok {
			return problem(400, "公告内容必须为对象")
		}
		cfg, err := announcementConfig(raw)
		if err != nil {
			return err
		}
		if err = validateAnnouncementInput(cfg); err != nil {
			return err
		}
		if action == "create" {
			if len(shared.Items) >= 200 {
				return problem(400, "公告最多保留 200 条，请先删除不再使用的公告")
			}
			index = len(shared.Items)
			shared.Items = append(shared.Items, announcementEntry{ID: "notice-" + randomToken()[:24], Sources: []string{}, CreatedAt: now()})
		}
		shared.Items[index].Config = cfg
	case "toggle":
		enabled, ok := input["enabled"].(bool)
		if !ok {
			return problem(400, "公告开关格式无效")
		}
		if enabled && strings.TrimSpace(str(shared.Items[index].Config["text"])) == "" {
			return problem(400, "请先填写公告内容再开启")
		}
		shared.Items[index].Config["enabled"] = enabled
	case "delete":
		shared.Items = append(shared.Items[:index], shared.Items[index+1:]...)
		index = -1
	default:
		return problem(400, "公告操作无效")
	}
	stamp := now()
	if index >= 0 {
		shared.Items[index].UpdatedAt = stamp
		if shared.Items[index].Config["enabled"] == true {
			for i := range shared.Items {
				if i != index && shared.Items[i].Config["enabled"] == true {
					shared.Items[i].Config["enabled"] = false
					shared.Items[i].UpdatedAt = stamp
				}
			}
		}
	}
	shared.Version++
	shared.UpdatedAt = stamp
	v.UpdatedAt = stamp
	for _, p := range platforms {
		r := v.Operations[p]
		r.PublishedVersion++
		r.PublishedAt = stamp
	}
	return nil
}
func (a *app) adminAnnouncements(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Path != "/api/admin/announcements" {
		return problem(404, "接口不存在")
	}
	if r.Method == http.MethodGet {
		jsonResponse(w, 200, announcementsReport(a.readState()))
		return nil
	}
	if r.Method != http.MethodPost {
		return problem(405, "不支持的请求方法")
	}
	input, err := readBody(w, r, 64*1024)
	if err != nil {
		return err
	}
	var result *announcementState
	err = a.change(r.Context(), "announcements."+str(input["action"]), func(v *state) error {
		if err := mutateAnnouncements(v, input); err != nil {
			return err
		}
		result = clone(v.Announcements)
		return nil
	})
	if err != nil {
		var api *apiError
		if !errors.As(err, &api) {
			log.Printf("共享公告保存失败: %v", err)
			return problem(500, "公告保存失败，请稍后重试；上次生效公告保持不变")
		}
		return err
	}
	jsonResponse(w, 200, result)
	return nil
}
