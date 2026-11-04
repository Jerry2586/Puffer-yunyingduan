package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type communityRecord struct {
	ID        int    `json:"id"`
	Kind      string `json:"kind"`
	Base      string `json:"base"`
	UserID    int    `json:"userId"`
	Nickname  string `json:"nickname"`
	VideoID   string `json:"videoId"`
	VideoName string `json:"videoName"`
	Content   string `json:"content"`
	Reply     string `json:"reply"`
	Approved  bool   `json:"approved"`
	Reports   int    `json:"reports"`
	Reporters []int  `json:"reporters,omitempty"`
	Count     int    `json:"count"`
	Position  int    `json:"position"`
	Episode   int    `json:"episode"`
	Platform  string `json:"platform"`
	IP        string `json:"ip"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Version   int    `json:"version"`
}
type moderationPolicy struct {
	Version        int      `json:"version"`
	ReviewComments bool     `json:"reviewComments"`
	ReviewDanmu    bool     `json:"reviewDanmu"`
	Keywords       []string `json:"keywords"`
	IPs            []string `json:"ips"`
}
type communityState struct {
	NextID   int                         `json:"nextId"`
	Records  []communityRecord           `json:"records"`
	Policies map[string]moderationPolicy `json:"policies"`
}

func communityStateFor(s *state) *communityState {
	if s.Community == nil {
		s.Community = &communityState{NextID: 1, Records: []communityRecord{}, Policies: map[string]moderationPolicy{}}
	}
	if s.Community.Policies == nil {
		s.Community.Policies = map[string]moderationPolicy{}
	}
	if s.Community.NextID < 1 {
		s.Community.NextID = 1
	}
	return s.Community
}
func communityPolicy(s state) moderationPolicy {
	if s.Community != nil {
		if p, ok := s.Community.Policies[managementBase(s.CMSBase)]; ok {
			return p
		}
	}
	return moderationPolicy{ReviewComments: true, ReviewDanmu: true, Keywords: []string{}, IPs: []string{}}
}
func communityEpisode(r communityRecord) int { return max(1, r.Episode) }
func communityPublic(r communityRecord, admin bool) M {
	out := M{"id": r.ID, "kind": r.Kind, "userId": r.UserID, "nickname": r.Nickname, "videoId": r.VideoID, "videoName": r.VideoName, "content": r.Content, "reply": r.Reply, "approved": r.Approved, "reports": r.Reports, "count": r.Count, "position": r.Position, "episode": communityEpisode(r), "platform": r.Platform, "createdAt": r.CreatedAt, "updatedAt": r.UpdatedAt, "version": r.Version}
	if admin {
		out["ip"] = r.IP
	}
	return out
}
func (a *app) manageModeration(w http.ResponseWriter, r *http.Request) error {
	if r.Method == "GET" {
		v := a.readState()
		base := managementBase(v.CMSBase)
		if base == "" {
			return problem(409, "请先设置 CMS 内容源")
		}
		p := communityPolicy(v)
		jsonResponse(w, 200, M{"source": base, "version": p.Version, "reviewComments": p.ReviewComments, "reviewDanmu": p.ReviewDanmu, "keywords": p.Keywords, "ips": p.IPs})
		return nil
	}
	if r.Method != "POST" {
		return problem(405, "请求方法不支持")
	}
	data, e := readBody(w, r, 32*1024)
	if e != nil {
		return e
	}
	base := managementBase(a.readState().CMSBase)
	if e = managementExpected(data, base); e != nil {
		return e
	}
	if _, ok := data["reviewComments"].(bool); !ok {
		return problem(400, "请明确设置评论审核开关")
	}
	if _, ok := data["reviewDanmu"].(bool); !ok {
		return problem(400, "请明确设置弹幕审核开关")
	}
	version, valid := managementInteger(data["version"], 0, 2147483646)
	if !valid {
		return problem(400, "审核规则版本无效，请刷新")
	}
	p := moderationPolicy{Version: version + 1, ReviewComments: data["reviewComments"] == true, ReviewDanmu: data["reviewDanmu"] == true, Keywords: []string{}, IPs: []string{}}
	for key, target := range map[string]*[]string{"keywords": &p.Keywords, "ips": &p.IPs} {
		v, ok := data[key].([]any)
		if !ok {
			return problem(400, "黑名单应为文本列表")
		}
		seen := map[string]bool{}
		if len(v) > 200 {
			return problem(400, "每类黑名单最多 200 项")
		}
		for _, item := range v {
			raw, ok := item.(string)
			if !ok {
				return problem(400, "黑名单只能包含文本")
			}
			txt := strings.TrimSpace(raw)
			if txt == "" {
				continue
			}
			if utf8.RuneCountInString(txt) > 100 {
				return problem(400, "黑名单条目过长")
			}
			if key == "ips" && net.ParseIP(txt) == nil {
				return problem(400, "IP 黑名单包含无效 IP")
			}
			if key == "ips" {
				txt = net.ParseIP(txt).String()
			}
			if !seen[txt] {
				*target = append(*target, txt)
				seen[txt] = true
			}
		}
	}
	e = a.change(r.Context(), "management.moderation", func(s *state) error {
		if managementBase(s.CMSBase) != base {
			return coded(409, "内容源已更改，请刷新", "MANAGEMENT_SOURCE_CHANGED")
		}
		if communityPolicy(*s).Version != version {
			return coded(409, "审核规则已更新，请刷新后重试", "MANAGEMENT_RECORD_CHANGED")
		}
		communityStateFor(s).Policies[base] = p
		return nil
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 200, M{"source": base, "version": p.Version, "reviewComments": p.ReviewComments, "reviewDanmu": p.ReviewDanmu, "keywords": p.Keywords, "ips": p.IPs})
	return nil
}
func (a *app) manageCommunity(w http.ResponseWriter, r *http.Request, kind string, actionRoute bool) error {
	v := a.readState()
	base := managementBase(v.CMSBase)
	if base == "" {
		return problem(409, "请先设置 CMS 内容源")
	}
	if r.Method == "GET" && !actionRoute {
		q := r.URL.Query()
		ep := 0
		if raw := q.Get("episode"); raw != "" {
			var ok bool
			ep, ok = managementInteger(raw, 1, 10000)
			if !ok {
				return problem(400, "集数无效")
			}
		}
		items := []communityRecord{}
		if v.Community != nil {
			for _, item := range v.Community.Records {
				if item.Base != base || item.Kind != kind {
					continue
				}
				if id := q.Get("userId"); id != "" && id != strconv.Itoa(item.UserID) {
					continue
				}
				if id := q.Get("videoId"); id != "" && id != item.VideoID {
					continue
				}
				if kind == "danmu" && ep > 0 && communityEpisode(item) != ep {
					continue
				}
				if q.Get("status") == "1" && !item.Approved || q.Get("status") == "0" && item.Approved {
					continue
				}
				if q.Get("reported") == "1" && item.Reports == 0 {
					continue
				}
				if keyword := strings.ToLower(q.Get("keyword")); keyword != "" && !strings.Contains(strings.ToLower(item.Content+" "+item.VideoName+" "+item.Nickname), keyword) {
					continue
				}
				items = append(items, item)
			}
		}
		ascending := q.Get("order") == "asc"
		sort.Slice(items, func(i, j int) bool {
			x, y := items[i].ID, items[j].ID
			if q.Get("sort") == "count" {
				x, y = items[i].Count, items[j].Count
			}
			if x == y {
				x, y = items[i].ID, items[j].ID
			}
			if ascending {
				return x < y
			}
			return x > y
		})
		size := boundedInt(q.Get("pageSize"), 20, 1, 100)
		pages := max(1, (len(items)+size-1)/size)
		page := min(boundedInt(q.Get("page"), 1, 1, 1000000), pages)
		out := []any{}
		for _, item := range items[min((page-1)*size, len(items)):min(page*size, len(items))] {
			out = append(out, communityPublic(item, true))
		}
		jsonResponse(w, 200, M{"source": base, "items": out, "total": len(items), "page": page, "pages": pages, "pageSize": size})
		return nil
	}
	if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/action") {
		return problem(405, "请求方法不支持")
	}
	data, e := readBody(w, r, 32*1024)
	if e != nil {
		return e
	}
	if e = managementExpected(data, base); e != nil {
		return e
	}
	ids, e := managementIDs(data)
	if e != nil {
		return e
	}
	action := str(data["action"])
	if !has([]string{"delete", "approve", "unapprove", "edit", "reply"}, action) {
		return problem(400, "不支持的操作")
	}
	if has([]string{"approve", "unapprove"}, action) && kind != "comments" && kind != "danmu" {
		return problem(400, "此模块无需审核")
	}
	if action == "reply" && kind != "requests" && kind != "feedback" {
		return problem(400, "此模块不支持回复")
	}
	if (action == "edit" || action == "reply") && len(ids) != 1 {
		return problem(400, "一次只能编辑一条记录")
	}
	if action == "edit" && kind == "reminders" {
		return problem(400, "催更记录不支持修改正文")
	}
	content := strings.TrimSpace(str(data["content"]))
	reply := strings.TrimSpace(str(data["reply"]))
	if utf8.RuneCountInString(content) > 2000 || utf8.RuneCountInString(reply) > 2000 {
		return problem(400, "内容最多 2000 字")
	}
	if action == "edit" && kind == "danmu" && utf8.RuneCountInString(content) > 100 {
		return problem(400, "弹幕最多 100 字")
	}
	if action == "edit" && content == "" {
		return problem(400, "内容不能为空")
	}
	e = a.change(r.Context(), "management."+kind+"."+action, func(s *state) error {
		if managementBase(s.CMSBase) != base {
			return coded(409, "内容源已更改，请刷新", "MANAGEMENT_SOURCE_CHANGED")
		}
		c := communityStateFor(s)
		found := 0
		wanted := map[int]bool{}
		for _, id := range ids {
			wanted[id] = true
		}
		for _, item := range c.Records {
			if wanted[item.ID] && item.Kind == kind && item.Base == base {
				found++
				version, valid := managementInteger(obj(data["versions"])[strconv.Itoa(item.ID)], 1, 2147483647)
				if !valid || version != item.Version {
					return coded(409, "记录已更新，请刷新后重试", "MANAGEMENT_RECORD_CHANGED")
				}
			}
		}
		if found != len(ids) {
			return coded(409, "部分记录已不存在，请刷新", "MANAGEMENT_RECORD_CHANGED")
		}
		next := make([]communityRecord, 0, len(c.Records))
		for _, item := range c.Records {
			if wanted[item.ID] && item.Kind == kind && item.Base == base {
				if action == "delete" {
					continue
				}
				switch action {
				case "approve":
					item.Approved = true
				case "unapprove":
					item.Approved = false
				case "edit":
					item.Content = content
				case "reply":
					item.Reply = reply
				}
				item.Version++
				item.UpdatedAt = now()
			}
			next = append(next, item)
		}
		c.Records = next
		return nil
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 200, M{"ok": true, "source": base, "affected": len(ids)})
	return nil
}
func (a *app) communityRoute(w http.ResponseWriter, r *http.Request, v state) error {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/community/"), "/")
	if len(parts) > 2 || len(parts) == 2 && parts[1] != "report" {
		return problem(404, "互动接口不存在")
	}
	kind := parts[0]
	if !managementKinds[kind] || kind == "users" || kind == "videos" {
		return problem(404, "互动模块不存在")
	}
	p, e := requestPlatform(r)
	if e != nil {
		return e
	}
	if obj(v.Operations[p].Published["features"])["member"] != true {
		return problem(403, "当前客户端未开放会员互动")
	}
	base := managementBase(v.CMSBase)
	if base == "" {
		return problem(409, "请先配置 CMS 内容源")
	}
	mine := r.URL.Query().Get("mine") == "1"
	uid := 0
	nickname := ""
	if r.Method != "GET" || mine || kind != "comments" && kind != "danmu" {
		session, e := a.memberSession(r, v.CMSBase, false)
		if e != nil {
			return e
		}
		profile, e := a.profile(r.Context(), session)
		if e != nil {
			return e
		}
		uid = numericOr(profile["user_id"], 0)
		if uid < 1 {
			return problem(401, "会员身份无效")
		}
		nickname = str(profile["user_nick_name"])
		if nickname == "" {
			nickname = str(profile["user_name"])
		}
	}
	if r.Method == "GET" && len(parts) == 1 {
		video := r.URL.Query().Get("videoId")
		if !mine && (kind == "comments" || kind == "danmu") && !vodID.MatchString(video) {
			return problem(400, "请指定有效影片 ID")
		}
		items := []any{}
		ep := 0
		if kind == "danmu" && (!mine || r.URL.Query().Get("episode") != "") {
			ep = 1
			if raw := r.URL.Query().Get("episode"); raw != "" {
				var ok bool
				ep, ok = managementInteger(raw, 1, 10000)
				if !ok {
					return problem(400, "集数无效")
				}
			}
			if video != "" {
				if _, e := a.danmuFilm(r.Context(), v, video, ep); e != nil {
					return e
				}
			}
		}
		v = a.readState()
		if managementBase(v.CMSBase) != base {
			return coded(409, "内容源已更改，请刷新", "MANAGEMENT_SOURCE_CHANGED")
		}
		if obj(v.Operations[p].Published["features"])["member"] != true {
			return problem(403, "当前客户端未开放会员互动")
		}
		if kind == "comments" && !mine {
			c := applicationState(v).Config.Comments
			if c.PinnedEnabled {
				items = append(items, M{"id": 0, "pinned": true, "nickname": c.PinnedNickname, "avatar": c.PinnedAvatar, "content": c.PinnedContent, "videoId": video, "approved": true})
			}
		}
		if v.Community != nil && (kind != "danmu" || mine || playbackPreferencesFor(v).Danmu.Enabled) {
			for i := len(v.Community.Records) - 1; i >= 0; i-- {
				item := v.Community.Records[i]
				if item.Base != base || item.Kind != kind {
					continue
				}
				if mine || kind != "comments" && kind != "danmu" {
					if item.UserID != uid {
						continue
					}
				} else if !item.Approved {
					continue
				}
				if video != "" && item.VideoID != video {
					continue
				}
				if kind == "danmu" && ep > 0 && communityEpisode(item) != ep {
					continue
				}
				items = append(items, communityPublic(item, false))
			}
		}
		size := boundedInt(r.URL.Query().Get("pageSize"), 20, 1, 100)
		pages := max(1, (len(items)+size-1)/size)
		page := min(boundedInt(r.URL.Query().Get("page"), 1, 1, 100000), pages)
		jsonResponse(w, 200, M{"items": items[min((page-1)*size, len(items)):min(page*size, len(items))], "total": len(items), "page": page, "pages": pages})
		return nil
	}
	if r.Method != "POST" {
		return problem(405, "请求方法不支持")
	}
	data, e := readBody(w, r, 16*1024)
	if e != nil {
		return e
	}
	if len(parts) == 2 && parts[1] == "report" {
		id, valid := managementInteger(data["id"], 1, 2147483647)
		if !valid {
			return problem(400, "记录 ID 无效")
		}
		if kind != "comments" && kind != "danmu" {
			return problem(400, "此模块不支持举报")
		}
		e = a.change(r.Context(), "community.report", func(s *state) error {
			if managementBase(s.CMSBase) != base {
				return coded(409, "内容源已更改，请刷新", "MANAGEMENT_SOURCE_CHANGED")
			}
			if obj(s.Operations[p].Published["features"])["member"] != true {
				return problem(403, "当前客户端未开放会员互动")
			}
			for i := range communityStateFor(s).Records {
				item := &s.Community.Records[i]
				if item.ID == id && item.Kind == kind && item.Base == base && item.Approved {
					for _, who := range item.Reporters {
						if who == uid {
							return problem(409, "已举报此内容")
						}
					}
					if len(item.Reporters) >= 1000 {
						return problem(429, "此内容已有足够举报")
					}
					item.Reporters = append(item.Reporters, uid)
					item.Reports++
					item.Version++
					item.UpdatedAt = now()
					return nil
				}
			}
			return problem(404, "内容不存在")
		})
		if e != nil {
			return e
		}
		jsonResponse(w, 200, M{"ok": true})
		return nil
	}
	if len(parts) != 1 {
		return problem(404, "互动接口不存在")
	}
	content := strings.TrimSpace(str(data["content"]))
	video := str(data["videoId"])
	title := ""
	limit := 2000
	if kind == "danmu" {
		limit = 100
	}
	if (kind != "reminders" && content == "") || utf8.RuneCountInString(content) > limit {
		return problem(400, "内容为空或超过字数限制")
	}
	if kind == "comments" || kind == "danmu" || kind == "reminders" || video != "" {
		if !vodID.MatchString(video) {
			return problem(400, "影片 ID 无效")
		}
		cat, e := a.catalog(r.Context(), v.CMSBase, url.Values{"ids": {video}})
		if e != nil {
			return e
		}
		for _, raw := range arr(cat["items"]) {
			item := obj(raw)
			if str(item["id"]) == video {
				title = str(item["title"])
			}
		}
		if title == "" {
			return problem(404, "影片不存在")
		}
	}
	episode := 1
	if kind == "comments" && !applicationState(v).Config.Comments.Enabled {
		return problem(403, "评论发送已关闭")
	}
	if kind == "danmu" {
		if !playbackPreferencesFor(v).Danmu.Enabled {
			return problem(403, "弹幕发送已关闭")
		}
		if raw, exists := data["episode"]; exists {
			var ok bool
			episode, ok = managementInteger(raw, 1, 10000)
			if !ok {
				return problem(400, "集数无效")
			}
		}
		if _, e := a.danmuFilm(r.Context(), v, video, episode); e != nil {
			return e
		}
	}
	if kind == "comments" || kind == "danmu" {
		if e := a.verifyApplicationCaptcha(r, v, kind, data); e != nil {
			return e
		}
	}
	position := 0
	if raw, ok := data["position"]; ok {
		var valid bool
		switch n := raw.(type) {
		case float64:
			position = number(n)
			valid = n == float64(position)
		case int:
			position = n
			valid = true
		case json.Number:
			i, er := n.Int64()
			valid = er == nil && i >= 0 && i <= 86400000
			position = int(i)
		}
		if !valid {
			return problem(400, "弹幕时间无效")
		}
	}
	if position < 0 || position > 86400000 {
		return problem(400, "弹幕时间无效")
	}
	ip, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		ip = r.RemoteAddr
	}
	if parsed := net.ParseIP(ip); parsed != nil {
		ip = parsed.String()
	} else {
		ip = ""
	}
	var result M
	e = a.change(r.Context(), "community.create."+kind, func(s *state) error {
		if managementBase(s.CMSBase) != base {
			return coded(409, "内容源已更改，请刷新", "MANAGEMENT_SOURCE_CHANGED")
		}
		if obj(s.Operations[p].Published["features"])["member"] != true {
			return problem(403, "当前客户端未开放会员互动")
		}
		if applicationState(*s).Version != applicationState(v).Version || playbackPreferencesFor(*s).Version != playbackPreferencesFor(v).Version {
			return problem(409, "互动设置已更改，请刷新后重试")
		}
		if kind == "comments" && !applicationState(*s).Config.Comments.Enabled || kind == "danmu" && !playbackPreferencesFor(*s).Danmu.Enabled {
			return problem(403, "发送已关闭")
		}
		c := communityStateFor(s)
		policy := communityPolicy(*s)
		for _, b := range policy.IPs {
			if b == ip {
				return problem(403, "当前地址暂时无法提交")
			}
		}
		for _, word := range policy.Keywords {
			if strings.Contains(strings.ToLower(content), strings.ToLower(word)) {
				return problem(400, "内容包含禁止提交的关键词")
			}
		}
		recent := 0
		reminder := -1
		for i := range c.Records {
			item := &c.Records[i]
			if item.Base != base || item.UserID != uid {
				continue
			}
			at, _ := time.Parse(time.RFC3339, item.CreatedAt)
			if time.Since(at) < time.Minute {
				recent++
			}
			if item.Kind == kind && kind == "reminders" && item.VideoID == video {
				updatedAt, _ := time.Parse(time.RFC3339, item.UpdatedAt)
				if time.Since(updatedAt) < time.Hour {
					return problem(429, "同一影片每小时可催更一次")
				}
				reminder = i
			}
		}
		if recent >= 10 {
			return problem(429, "提交过于频繁，请稍后再试")
		}
		if reminder >= 0 {
			item := &c.Records[reminder]
			item.Count++
			item.Version++
			item.UpdatedAt = now()
			result = communityPublic(*item, false)
			return nil
		}
		if len(c.Records) >= 20000 {
			return problem(409, "互动记录已满，请联系管理员清理")
		}
		approved := kind != "comments" && kind != "danmu" || kind == "comments" && !policy.ReviewComments || kind == "danmu" && !policy.ReviewDanmu
		item := communityRecord{ID: c.NextID, Kind: kind, Base: base, UserID: uid, Nickname: nickname, VideoID: video, VideoName: title, Content: content, Approved: approved, Count: 1, Position: position, Episode: episode, Platform: p, IP: ip, CreatedAt: now(), UpdatedAt: now(), Version: 1}
		c.NextID++
		c.Records = append(c.Records, item)
		result = communityPublic(item, false)
		return nil
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 201, result)
	return nil
}
