package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"path/filepath"
	"strconv"
	"time"
)

type captchaChallenge struct {
	Digest     [32]byte
	Action, IP string
	Until      time.Time
}
type historyEntry struct {
	VideoID   string `json:"videoId"`
	Title     string `json:"title"`
	Poster    string `json:"poster"`
	Episode   int    `json:"episode"`
	Position  int    `json:"position"`
	UpdatedAt string `json:"updatedAt"`
}
type memberHistory struct {
	Base   string         `json:"base"`
	UserID int            `json:"userId"`
	Items  []historyEntry `json:"items"`
}

func captchaEnabled(v state, action string) bool {
	c := applicationState(v).Config.Captcha
	switch action {
	case "login":
		return c.Login
	case "register":
		return c.Register
	case "comments":
		return c.Comments
	case "danmu":
		return c.Danmu
	}
	return false
}
func (a *app) verifyApplicationCaptcha(r *http.Request, v state, action string, data M) error {
	if !captchaEnabled(v, action) {
		return nil
	}
	token, answer := str(data["captchaToken"]), str(data["captchaAnswer"])
	a.mu.Lock()
	challenge, ok := a.captchas[token]
	delete(a.captchas, token)
	a.mu.Unlock()
	digest := sha256.Sum256([]byte(answer))
	if !ok || time.Now().After(challenge.Until) || challenge.Action != action || challenge.IP != "" && challenge.IP != requestIP(r) || subtle.ConstantTimeCompare(digest[:], challenge.Digest[:]) != 1 {
		return coded(400, "验证码错误或已过期，请刷新后重试", "CAPTCHA_INVALID")
	}
	return nil
}
func (a *app) newApplicationCaptcha(w http.ResponseWriter, r *http.Request, v state) error {
	action := r.URL.Query().Get("action")
	if !has([]string{"login", "register", "comments", "danmu"}, action) {
		return problem(400, "验证码用途无效")
	}
	if !captchaEnabled(v, action) {
		jsonResponse(w, 200, M{"required": false})
		return nil
	}
	ip := requestIP(r)
	a.mu.Lock()
	if a.captchas == nil {
		a.captchas = map[string]captchaChallenge{}
	}
	for k, c := range a.captchas {
		if time.Now().After(c.Until) {
			delete(a.captchas, k)
		}
	}
	key := "captcha:" + ip
	x := a.attempts[key]
	if time.Since(x.Start) > time.Minute {
		x = attempt{Start: time.Now()}
	}
	if x.Count >= 20 || len(a.captchas) >= 5000 {
		a.mu.Unlock()
		return problem(429, "验证码请求过于频繁，请稍后重试")
	}
	x.Count++
	a.attempts[key] = x
	a.mu.Unlock()
	raw := make([]byte, 5)
	if _, e := rand.Read(raw); e != nil {
		return e
	}
	answer := make([]byte, 5)
	for i, b := range raw {
		answer[i] = '0' + b%10
	}
	// Raster PNG works in web and native clients. The answer never leaves the server.
	glyphs := []string{"111101101101111", "010110010010111", "111001111100111", "111001111001111", "101101111001001", "111100111001111", "111100111101111", "111001001001001", "111101111101111", "111101111001111"}
	im := image.NewRGBA(image.Rect(0, 0, 150, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 150; x++ {
			im.Set(x, y, color.RGBA{240, 244, 250, 255})
		}
	}
	for i, ch := range answer {
		g := glyphs[int(ch-'0')]
		dy := int(raw[i] % 5)
		for j, b := range g {
			if b == '1' {
				for y := 0; y < 6; y++ {
					for x := 0; x < 6; x++ {
						im.Set(10+i*27+(j%3)*6+x, 7+dy+(j/3)*6+y, color.RGBA{35, 60, 105, 255})
					}
				}
			}
		}
	}
	noise := make([]byte, 150)
	_, _ = rand.Read(noise)
	for i, b := range noise {
		im.Set(i, int(b)%48, color.RGBA{110, 150, 180, 255})
	}
	var buf bytes.Buffer
	if e := png.Encode(&buf, im); e != nil {
		return e
	}
	token := randomToken()
	bound := ""
	if applicationState(v).Config.Captcha.BindIP {
		bound = ip
	}
	a.mu.Lock()
	if len(a.captchas) >= 5000 {
		a.mu.Unlock()
		return problem(429, "验证码请求过于频繁")
	}
	a.captchas[token] = captchaChallenge{Digest: sha256.Sum256(answer), Action: action, IP: bound, Until: time.Now().Add(5 * time.Minute)}
	a.mu.Unlock()
	jsonResponse(w, 200, M{"required": true, "token": token, "image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), "expiresIn": 300})
	return nil
}
func (a *app) applicationMember(r *http.Request, v state, p string) (*memberSession, int, error) {
	if obj(v.Operations[p].Published["features"])["member"] != true {
		return nil, 0, problem(403, "当前客户端未开放会员功能")
	}
	s, e := a.memberSession(r, v.CMSBase, false)
	if e != nil {
		return nil, 0, e
	}
	profile, e := a.profile(r.Context(), s)
	if e != nil {
		return nil, 0, e
	}
	uid := numericOr(profile["user_id"], 0)
	if uid < 1 {
		return nil, 0, problem(401, "会员身份无效")
	}
	return s, uid, nil
}
func (a *app) applicationClientRoute(w http.ResponseWriter, r *http.Request, v state) error {
	p, e := requestPlatform(r)
	if e != nil {
		return e
	}
	route := r.URL.Path
	if route == "/api/application/config" {
		if r.Method != "GET" {
			return problem(405, "请求方法不支持")
		}
		jsonResponse(w, 200, publicApplication(v, p))
		return nil
	}
	if route == "/api/application/captcha" {
		if r.Method != "GET" {
			return problem(405, "请求方法不支持")
		}
		return a.newApplicationCaptcha(w, r, v)
	}
	if r.Method != "GET" && r.Method != "POST" {
		return problem(405, "请求方法不支持")
	}
	session, uid, e := a.applicationMember(r, v, p)
	if e != nil {
		return e
	}
	cfg := applicationState(v).Config
	base := managementBase(v.CMSBase)
	if route == "/api/member/invitation" {
		if r.Method != "GET" {
			return problem(405, "请求方法不支持")
		}
		if !cfg.Share.Invite {
			return problem(403, "邀请功能未开启")
		}
		jsonResponse(w, 200, M{"code": invitationCode(v, uid), "text": cfg.Share.Text})
		return nil
	}
	if route == "/api/member/avatar" {
		if r.Method != "POST" {
			return problem(405, "请求方法不支持")
		}
		if e := a.applicationRateLimit("avatar:"+base+":"+strconv.Itoa(uid), 5, time.Minute); e != nil {
			return e
		}
		data, e := readBody(w, r, 1500*1024)
		if e != nil {
			return e
		}
		avatar := str(data["url"])
		if encoded := str(data["data"]); encoded != "" {
			if !cfg.Avatar.Upload {
				return problem(403, "未开放上传头像")
			}
			b, e := base64.StdEncoding.DecodeString(encoded)
			if e != nil || len(b) == 0 || len(b) > 1024*1024 {
				return problem(400, "头像须为不超过 1MB 的 PNG 或 JPG 图片")
			}
			info, format, e := image.DecodeConfig(bytes.NewReader(b))
			if e != nil || !has([]string{"png", "jpeg"}, format) || info.Width > 2048 || info.Height > 2048 || info.Width < 1 || info.Height < 1 {
				return problem(400, "头像尺寸须为 1–2048 像素")
			}
			ext := ".png"
			if format == "jpeg" {
				ext = ".jpg"
			}
			hash := sha256.Sum256(b)
			name := hex.EncodeToString(hash[:]) + ext
			if e = atomicFile(filepath.Join(a.assets, name), b); e != nil {
				return e
			}
			avatar = "/api/assets/" + name
		} else if !has(cfg.Avatar.Library, avatar) {
			return problem(400, "请选择头像库中的头像")
		}
		if !validSettingURL(avatar, true) || avatar == "" {
			return problem(400, "头像地址无效")
		}
		if assetPattern.MatchString(avatar) {
			addresses := a.serviceAddresses(v)
			if len(addresses) == 0 {
				return problem(409, "请先配置客户端可访问的服务地址")
			}
			avatar = addresses[0] + avatar
		}
		if e := a.applicationSnapshotCurrent(v, p); e != nil {
			return e
		}
		connection, e := a.managementConnectionFor(v)
		if e != nil {
			return e
		}
		_, e = a.managementCall(r.Context(), connection, "member.avatar", M{"id": uid, "avatar": avatar})
		if e != nil {
			return e
		}
		session.ProfileVersion.Add(1)
		jsonResponse(w, 200, M{"ok": true, "url": avatar})
		return nil
	}
	if route != "/api/member/history" {
		return problem(404, "会员接口不存在")
	}
	if !cfg.History.Enabled {
		return problem(403, "云端历史记录未开启")
	}
	if r.Method == "GET" {
		items := []historyEntry{}
		for _, h := range v.MemberHistories {
			if h.Base == base && h.UserID == uid {
				items = h.Items[:min(len(h.Items), cfg.History.Limit)]
				break
			}
		}
		jsonResponse(w, 200, M{"items": items})
		return nil
	}
	data, e := readBody(w, r, 8*1024)
	if e != nil {
		return e
	}
	id := str(data["videoId"])
	ep, ok := managementInteger(data["episode"], 1, 10000)
	pos, valid := managementInteger(data["position"], 0, 86400000)
	if !vodID.MatchString(id) || !ok || !valid {
		return problem(400, "历史记录影片、集数或进度无效")
	}
	film, e := a.danmuFilm(r.Context(), v, id, ep)
	if e != nil {
		return e
	}
	entry := historyEntry{VideoID: id, Title: str(film["title"]), Poster: str(film["poster"]), Episode: ep, Position: pos, UpdatedAt: now()}
	e = a.change(r.Context(), "member.history", func(s *state) error {
		if managementBase(s.CMSBase) != base || !applicationState(*s).Config.History.Enabled || obj(s.Operations[p].Published["features"])["member"] != true {
			return problem(409, "内容源或历史记录设置已变更")
		}
		limit := applicationState(*s).Config.History.Limit
		idx := -1
		total := 0
		for i, h := range s.MemberHistories {
			total += len(h.Items)
			if h.Base == base && h.UserID == uid {
				idx = i
			}
		}
		if idx < 0 {
			if total >= 20000 {
				return problem(503, "历史记录容量已满")
			}
			s.MemberHistories = append(s.MemberHistories, memberHistory{Base: base, UserID: uid})
			idx = len(s.MemberHistories) - 1
		}
		previous := s.MemberHistories[idx].Items
		next := []historyEntry{entry}
		for _, x := range previous {
			if x.VideoID != id && len(next) < limit {
				next = append(next, x)
			}
		}
		if total-len(previous)+len(next) > 20000 {
			return problem(503, "历史记录容量已满")
		}
		s.MemberHistories[idx].Items = next
		return nil
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 200, M{"ok": true, "entry": entry})
	return nil
}
func registrationMemberData(c applicationConfig, user, pass, ip string, inviter int) M {
	return M{"username": user, "password": pass, "ip": ip, "active": c.Registration.Active, "points": c.Registration.Points, "invitePoints": c.Registration.InvitePoints, "inviter": inviter}
}
func membershipPrice(v state, period string) int {
	c := applicationState(v).Config.Membership
	switch period {
	case "day":
		return c.Day
	case "week":
		return c.Week
	case "month":
		return c.Month
	case "year":
		return c.Year
	}
	return 0
}
func episodeParameter(raw string) int { n, _ := strconv.Atoi(raw); return n }

// No external calls are made while holding the application mutex.
func (a *app) applicationRateLimit(key string, limit int, window time.Duration) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.attempts == nil {
		a.attempts = map[string]attempt{}
	}
	x := a.attempts[key]
	if time.Since(x.Start) > window {
		x = attempt{Start: time.Now()}
	}
	if x.Count >= limit {
		return problem(429, "操作频繁，请稍后重试")
	}
	x.Count++
	a.attempts[key] = x
	return nil
}
func (a *app) applicationSnapshotCurrent(v state, p string) error {
	live := a.readState()
	if managementBase(live.CMSBase) != managementBase(v.CMSBase) || applicationState(live).Version != applicationState(v).Version {
		return problem(409, "内容源或应用设置已更新，请刷新后重试")
	}
	if obj(live.Operations[p].Published["features"])["member"] != true {
		return problem(403, "当前客户端未开放会员功能")
	}
	return nil
}
