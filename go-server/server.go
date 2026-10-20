package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/scrypt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type apiError struct {
	Status        int
	Message, Code string
}

func (e *apiError) Error() string                  { return e.Message }
func problem(status int, message string) error     { return &apiError{Status: status, Message: message} }
func coded(status int, message, code string) error { return &apiError{status, message, code} }

type config struct {
	Database        *databaseSettings `json:"database,omitempty"`
	HTTP            *httpSettings     `json:"http,omitempty"`
	Redis           *redisConfig      `json:"redis,omitempty"`
	AdminPath       string            `json:"adminPath,omitempty"`
	Listen          string            `json:"listen"`
	PublicOrigin    string            `json:"publicOrigin"`
	DataDir         string            `json:"dataDir"`
	Storage         string            `json:"storage"`
	MySQLDSN        string            `json:"mysqlDSN"`
	AllowPrivateCMS bool              `json:"allowPrivateCMS"`
	ClientOrigins   []string          `json:"clientOrigins"`
	CMSBase         string            `json:"cmsBase"`
	CMSRoots        []string          `json:"cmsRoots,omitempty"`
}
type adminSession struct {
	Until time.Time
	Epoch int
}
type attempt struct {
	Count int
	Start time.Time
}
type memberSession struct {
	ProfileVersion atomic.Uint64
	Jar            http.CookieJar
	Until          time.Time
	Base           string
	Mu             sync.Mutex
}
type app struct {
	searchMu      sync.Mutex
	searchJobs    map[string]*searchJob
	searchIndexes map[string]*searchIndex
	updater       *systemUpdater
	mu            sync.RWMutex
	state         state
	store         store
	config        config
	cache         *contentCache
	cms           *http.Client
	sessions      map[string]adminSession
	attempts      map[string]attempt
	members       map[string]*memberSession
	checkouts     map[string]checkout
	epoch         int
	assets        string
	web           http.Handler
	serviceKey    *rsa.PrivateKey
	discoveryMu   sync.Mutex
	systemMu      sync.Mutex
	configPath    string
	startedAt     time.Time
	requests      requestMonitor
	clientMetrics clientTelemetry
	outbound      outboundMonitor
}

func randomToken() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func credentials(username, password string) (credential, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9_]{3,40}$`).MatchString(username) {
		return credential{}, problem(400, "账号需为3–40位字母、数字或下划线")
	}
	if password == "" {
		return credential{}, problem(400, "密码不能为空")
	}
	salt := randomToken()[:32]
	key, e := scrypt.Key([]byte(password), []byte(salt), 16384, 8, 1, 32)
	if e != nil {
		return credential{}, e
	}
	return credential{username, salt, hex.EncodeToString(key), "scrypt"}, nil
}
func matches(c credential, password string) bool {
	key, e := scrypt.Key([]byte(password), []byte(c.Salt), 16384, 8, 1, 32)
	if e != nil {
		return false
	}
	want, e := hex.DecodeString(c.Hash)
	return e == nil && subtle.ConstantTimeCompare(key, want) == 1
}
func newApp(ctx context.Context, c config, s store) (*app, error) {
	v, e := s.Load(ctx)
	if errors.Is(e, errNoState) {
		v = initialState()
		password := randomToken()[:24]
		v.Credential, e = credentials("admin", password)
		if e != nil {
			return nil, e
		}
		if c.CMSBase != "" {
			v.CMSBase = c.CMSBase
		}
		b, _ := json.MarshalIndent(M{"username": "admin", "password": password}, "", "  ")
		if e = atomicFile(filepath.Join(c.DataDir, "initial-admin.json"), b); e != nil {
			return nil, e
		}
		if e = s.Save(ctx, v, 0); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	}
	migrateAppReleases(&v)
	if e = validateState(v); e != nil {
		return nil, e
	}
	a := &app{startedAt: time.Now(), state: v, store: s, config: c, cms: configuredCMSClient(c), sessions: map[string]adminSession{}, attempts: map[string]attempt{}, members: map[string]*memberSession{}, checkouts: map[string]checkout{}, assets: filepath.Join(c.DataDir, "assets")}
	if e = os.MkdirAll(a.assets, 0700); e != nil {
		return nil, e
	}
	web, e := fs.Sub(embeddedWeb, "web")
	if e != nil {
		return nil, e
	}
	a.web = http.FileServer(http.FS(web))
	if e = a.loadServiceKey(); e != nil {
		return nil, e
	}
	a.outbound.settings = effectiveHTTP(c.HTTP)
	a.cms.Transport = &trackedTransport{base: a.cms.Transport, metrics: &a.outbound}
	a.cache = newContentCache(c.Redis, c.DataDir)
	return a, nil
}
func (a *app) readState() state { a.mu.RLock(); defer a.mu.RUnlock(); return clone(a.state) }
func (a *app) change(ctx context.Context, event string, fn func(*state) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	next := clone(a.state)
	if e := fn(&next); e != nil {
		return e
	}
	old := a.state.Revision
	next.Revision = old + 1
	if e := a.store.Save(ctx, next, old); e != nil {
		return e
	}
	a.state = next
	b, _ := json.Marshal(M{"at": now(), "event": event, "revision": next.Revision})
	f, e := os.OpenFile(filepath.Join(a.config.DataDir, "audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e == nil {
		_, e = f.Write(append(b, '\n'))
		f.Close()
	}
	if e != nil {
		log.Print("审计记录写入失败，请检查磁盘权限")
	}
	return nil
}
func (a *app) setupStatus(v state) M {
	return M{"installed": str(v.Setup["completedAt"]) != "", "completedAt": str(v.Setup["completedAt"]), "cmsConfigured": v.CMSBase != "", "publicOrigin": a.config.PublicOrigin, "localPreview": strings.HasPrefix(a.config.PublicOrigin, "http://127.0.0.1:"), "username": v.Credential.Username}
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func readBody(w http.ResponseWriter, r *http.Request, max int64) (M, error) {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	defer r.Body.Close()
	var m M
	d := json.NewDecoder(r.Body)
	if e := d.Decode(&m); e != nil {
		return nil, problem(400, "JSON 格式错误或请求内容过大")
	}
	if m == nil {
		return nil, problem(400, "请求内容必须为 JSON 对象")
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return nil, problem(400, "请求只能包含一个 JSON 对象")
	}
	return m, nil
}
func (a *app) authenticate(r *http.Request) (string, error) {
	cookie, e := r.Cookie("hetun_session")
	if e != nil {
		return "", problem(401, "请先登录运营中心")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[cookie.Value]
	if !ok || time.Now().After(s.Until) || s.Epoch != a.epoch {
		delete(a.sessions, cookie.Value)
		return "", problem(401, "请先登录运营中心")
	}
	return cookie.Value, nil
}
func (a *app) cookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: "hetun_session", Value: token, Path: "/api", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: strings.HasPrefix(a.config.PublicOrigin, "https://"), MaxAge: maxAge})
}
func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/api/admin/system/") && r.Method != "OPTIONS" {
		w = a.requests.begin(w)
		defer a.requests.finish(w.(*monitoredResponse), time.Now())
	}
	if p, endpoint := publicMetricScope(r); p != "" {
		tracked := &monitoredResponse{ResponseWriter: w}
		w = tracked
		started := time.Now()
		defer func() { a.clientMetrics.record(p, endpoint, tracked.status, time.Since(started)) }()
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	defer func() {
		if recover() != nil {
			log.Print("内部请求错误")
			jsonResponse(w, 500, M{"error": "服务内部错误"})
		}
	}()
	if e := a.handle(w, r); e != nil {
		status, code, message := 400, "", e.Error()
		var api *apiError
		if errors.As(e, &api) {
			status, code, message = api.Status, api.Code, api.Message
		}
		jsonResponse(w, status, M{"error": message, "code": code})
	}
}
func (a *app) handle(w http.ResponseWriter, r *http.Request) error {
	route := r.URL.Path
	if route == "/install" || strings.HasPrefix(route, "/install/") {
		return problem(404, "安装入口已关闭")
	}
	origin := r.Header.Get("Origin")
	admin := strings.HasPrefix(route, "/api/admin/")
	allowed := origin == a.config.PublicOrigin || has(a.serviceAddresses(a.readState()), origin)
	clientAllowed := has(a.config.ClientOrigins, origin)
	if origin != "" && (allowed || (!admin && clientAllowed)) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Add("Vary", "Origin")
	}
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Hetun-Client, X-Hetun-Platform, Authorization")
		w.WriteHeader(204)
		return nil
	}
	var routeErr error
	r, routeErr = normalizeClientRoute(r)
	if routeErr != nil {
		return routeErr
	}
	route = r.URL.Path
	public, _ := url.Parse(a.config.PublicOrigin)
	if public != nil && strings.HasPrefix(a.config.PublicOrigin, "http://127.0.0.1:") && r.Host != public.Host {
		return problem(403, "请求主机不匹配")
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		if admin && (!allowed || r.Header.Get("X-Hetun-Client") != "studio") {
			return problem(403, "请求来源不允许")
		}
		if !admin && origin != "" && !allowed && !clientAllowed {
			return problem(403, "请求来源不允许")
		}
	}
	if handled, err := a.handleDirectory(w, r); handled {
		return err
	}
	if strings.HasPrefix(route, "/api/ads/") {
		return a.publicAds(w, r)
	}
	if route == "/api/health" && r.Method == "GET" {
		v := a.readState()
		jsonResponse(w, 200, M{"status": "ok", "service": "hetun", "product": "operations-center", "version": buildVersion, "runtime": "Go", "cmsConfigured": v.CMSBase != "", "contract": contract, "themeContract": "hetun-runtime-v1", "platforms": platforms})
		return nil
	}
	if route == "/api/admin/login" && r.Method == "POST" {
		input, e := readBody(w, r, 256*1024)
		if e != nil {
			return e
		}
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		a.mu.Lock()
		record := a.attempts["admin:"+ip]
		if time.Since(record.Start) > 15*time.Minute {
			record = attempt{Start: time.Now()}
		}
		if record.Count >= 8 {
			a.mu.Unlock()
			return problem(429, "登录尝试过多，请15分钟后重试")
		}
		record.Count++
		a.attempts["admin:"+ip] = record
		c := a.state.Credential
		epoch := a.epoch
		a.mu.Unlock()
		if !matches(c, str(input["password"])) || str(input["username"]) != c.Username {
			return problem(401, "账号或密码错误")
		}
		token := randomToken()
		a.mu.Lock()
		if epoch != a.epoch {
			a.mu.Unlock()
			return problem(401, "账号已更新，请重新登录")
		}
		delete(a.attempts, "admin:"+ip)
		for k, s := range a.sessions {
			if time.Now().After(s.Until) {
				delete(a.sessions, k)
			}
		}
		if len(a.sessions) >= 1000 {
			a.mu.Unlock()
			return problem(429, "会话容量已满")
		}
		a.sessions[token] = adminSession{time.Now().Add(8 * time.Hour), a.epoch}
		a.mu.Unlock()
		a.cookie(w, token, 28800)
		jsonResponse(w, 200, M{"ok": true})
		return nil
	}
	if admin {
		token, e := a.authenticate(r)
		if e != nil {
			return e
		}
		v := a.readState()
		if strings.HasPrefix(route, "/api/admin/ads") {
			return a.adminAds(w, r)
		}
		if route == "/api/admin/client-cache/search/rebuild" {
			return a.adminRebuildSearch(w, r)
		}
		if route == "/api/admin/client-cache" || route == "/api/admin/client-cache/clear" {
			return a.adminClientCache(w, r)
		}
		if route == "/api/admin/logout" && r.Method == "POST" {
			a.mu.Lock()
			delete(a.sessions, token)
			a.mu.Unlock()
			a.cookie(w, "", -1)
			jsonResponse(w, 200, M{"ok": true})
			return nil
		}
		if route == "/api/admin/service-addresses/check" {
			return a.checkServiceAddress(w, r)
		}
		if route == "/api/admin/service-addresses" {
			return a.adminDirectory(w, r)
		}
		if route == "/api/admin/setup" && r.Method == "GET" {
			jsonResponse(w, 200, a.setupStatus(v))
			return nil
		}
		if route == "/api/admin/setup/discover" && r.Method == "POST" {
			jsonResponse(w, 200, a.discoverAndConnect(r.Context()))
			return nil
		}
		if has([]string{"/api/admin/setup/check", "/api/admin/cms/check"}, route) && r.Method == "POST" {
			input, e := readBody(w, r, 256*1024)
			if e != nil {
				return e
			}
			jsonResponse(w, 200, a.checkCMS(r.Context(), str(input["base"])))
			return nil
		}
		if route == "/api/admin/setup/complete" && r.Method == "POST" {
			if str(v.Setup["completedAt"]) != "" {
				return problem(409, "初始化已完成")
			}
			input, e := readBody(w, r, 256*1024)
			if e != nil {
				return e
			}
			c, e := credentials(str(input["username"]), str(input["password"]))
			if e != nil {
				return e
			}
			report := a.checkCMS(r.Context(), str(input["base"]))
			if report["ok"] != true {
				return problem(400, "请先完成真实 CMS 连接检测")
			}
			e = a.change(r.Context(), "setup.complete", func(next *state) error {
				if str(next.Setup["completedAt"]) != "" {
					return problem(409, "初始化已完成")
				}
				next.Credential = c
				next.CMSBase = str(report["base"])
				next.Setup = M{"completedAt": now()}
				return nil
			})
			if e != nil {
				return e
			}
			a.rotateSessions(token)
			if e = os.Remove(filepath.Join(a.config.DataDir, "initial-admin.json")); e != nil && !errors.Is(e, os.ErrNotExist) {
				log.Print("初始账号文件清理失败，请在服务器手动移除")
			}
			jsonResponse(w, 200, a.setupStatus(a.readState()))
			return nil
		}
		if route == "/api/admin/account" && r.Method == "PUT" {
			input, e := readBody(w, r, 256*1024)
			if e != nil {
				return e
			}
			if !matches(v.Credential, str(input["currentPassword"])) {
				return problem(403, "当前密码错误")
			}
			c, e := credentials(str(input["username"]), str(input["password"]))
			if e != nil {
				return e
			}
			e = a.change(r.Context(), "account.changed", func(next *state) error {
				if next.Credential.Hash != v.Credential.Hash {
					return problem(409, "账号已改变，请重新登录")
				}
				next.Credential = c
				return nil
			})
			if e != nil {
				return e
			}
			a.rotateSessions(token)
			jsonResponse(w, 200, M{"ok": true, "username": c.Username})
			return nil
		}
		if str(v.Setup["completedAt"]) == "" {
			return coded(409, "请先完成安装向导", "SETUP_REQUIRED")
		}
		if route == "/api/admin/statistics" {
			return a.clientStatistics(w, r)
		}
		if route == "/api/admin/releases" || route == "/api/admin/releases/delete" {
			return a.adminReleases(w, r)
		}
		if strings.HasPrefix(route, "/api/admin/system/") {
			return a.adminSystem(w, r)
		}
		switch {
		case route == "/api/admin/state" && r.Method == "GET":
			p, e := platform(r.URL.Query().Get("template"))
			if e != nil {
				return e
			}
			jsonResponse(w, 200, studio(v, p))
			return nil
		case route == "/api/admin/cms/status" && r.Method == "GET":
			jsonResponse(w, 200, a.checkCMS(r.Context(), v.CMSBase))
			return nil
		case route == "/api/admin/cms" && r.Method == "PUT":
			input, e := readBody(w, r, 256*1024)
			if e != nil {
				return e
			}
			report, localRoot, e := a.selectedCMS(r.Context(), input)
			if e != nil {
				return e
			}
			if (str(input["base"]) != "" || str(input["siteId"]) != "") && report["ok"] != true {
				jsonResponse(w, 400, M{"error": report["message"], "cmsStatus": report})
				return nil
			}
			e = a.change(r.Context(), "cms.changed", func(next *state) error { next.CMSBase = str(report["base"]); next.CMSLocalRoot = localRoot; return nil })
			if e != nil {
				return e
			}
			out := studio(a.readState(), "mobile")
			out["cmsStatus"] = report
			jsonResponse(w, 200, out)
			return nil
		case (route == "/api/admin/draft" && r.Method == "PUT") || ((route == "/api/admin/publish" || route == "/api/admin/rollback") && r.Method == "POST"):
			input, e := readBody(w, r, 256*1024)
			if e != nil {
				return e
			}
			if e := scopeOperationInput(r, input); e != nil {
				return e
			}
			p, e := platform(str(input["template"]))
			if e != nil {
				return e
			}
			action := path.Base(route)
			e = a.change(r.Context(), "runtime."+action, func(next *state) error { return mutateOperations(next, input, action) })
			if e != nil {
				return e
			}
			jsonResponse(w, 200, studio(a.readState(), p))
			return nil
		case route == "/api/admin/asset" && r.Method == "POST":
			return a.upload(w, r)
		}
		return problem(404, "此接口不属于运营服务")
	}
	v := a.readState()
	if route == "/api/runtime" || route == "/api/client/bootstrap" || route == "/api/bootstrap" {
		if r.Method != "GET" {
			return problem(405, "请求方法不支持")
		}
		p, e := requestPlatform(r)
		if e != nil {
			return e
		}
		if c := r.URL.Query().Get("contract"); c != "" && c != contract {
			return coded(409, "客户端协议不兼容", "CONTRACT_MISMATCH")
		}
		osName := r.URL.Query().Get("os")
		if osName == "" {
			osName = "android"
		}
		out, e := a.cachedBootstrap(r.Context(), v, p, osName, r.URL.Query().Get("version"), a.serviceAddresses(v)[0])
		if e != nil {
			return e
		}
		jsonResponse(w, 200, out)
		return nil
	}
	if route == "/api/theme" {
		return coded(410, "主题固定在安装包中，请读取运营配置接口", "PACKAGED_THEME_REQUIRED")
	}
	if route == "/api/catalog/categories" && r.Method == "GET" {
		p, e := requestPlatform(r)
		if e != nil {
			return e
		}
		raw, e := a.cachedCatalog(context.WithValue(r.Context(), cachePlatformKey{}, p), v.CMSBase, url.Values{"ac": {"list"}})
		if e != nil {
			return e
		}
		items := []any{}
		for _, raw := range arr(raw["class"]) {
			m := obj(raw)
			items = append(items, M{"id": str(m["type_id"]), "name": clean(m["type_name"])})
		}
		jsonResponse(w, 200, M{"items": items})
		return nil
	}
	if route == "/api/catalog" && r.Method == "GET" {
		p, e := requestPlatform(r)
		if e != nil {
			return e
		}
		out, e := a.catalog(context.WithValue(r.Context(), cachePlatformKey{}, p), v.CMSBase, r.URL.Query())
		if e != nil {
			return e
		}
		jsonResponse(w, 200, out)
		return nil
	}
	if strings.HasPrefix(route, "/api/assets/") {
		if r.Method != "GET" && r.Method != "HEAD" {
			return problem(405, "请求方法不支持")
		}
		if !assetPattern.MatchString(route) {
			return problem(404, "素材不存在")
		}
		w.Header().Set("Cache-Control", "public,max-age=31536000,immutable")
		http.ServeFile(w, r, filepath.Join(a.assets, path.Base(route)))
		return nil
	}
	if strings.HasPrefix(route, "/api/member/") || strings.HasPrefix(route, "/api/checkout/") || route == "/api/playback" {
		return a.memberRoute(w, r, v)
	}
	if strings.HasPrefix(route, "/api/") {
		return problem(404, "没有找到这个接口")
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		return problem(405, "请求方法不支持")
	}
	if strings.Contains(route, "..") || strings.Contains(route, "\\") {
		return problem(400, "路径无效")
	}
	if a.config.AdminPath != "" {
		entry := shortAdminPath(a.config.AdminPath)
		if route == entry+"/" || (entry != a.config.AdminPath && (route == a.config.AdminPath || route == a.config.AdminPath+"/")) {
			http.Redirect(w, r, entry, http.StatusTemporaryRedirect)
			return nil
		}
		if route == entry {
			r2 := r.Clone(r.Context())
			u := *r.URL
			u.Path = "/"
			r2.URL = &u
			w.Header().Set("Cache-Control", "no-store")
			a.web.ServeHTTP(w, r2)
			return nil
		}
		// Public static resources must not expose another entry to the SPA.
		if path.Ext(route) == "" || strings.EqualFold(path.Ext(route), ".html") {
			return problem(404, "没有找到这个页面")
		}
	}
	if a.config.AdminPath == "" && path.Ext(route) == "" && route != "/" {
		r2 := r.Clone(r.Context())
		u := *r.URL
		u.Path = "/"
		r2.URL = &u
		r = r2
	}
	w.Header().Set("Cache-Control", "no-cache")
	a.web.ServeHTTP(w, r)
	return nil
}
func (a *app) rotateSessions(keep string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.epoch++
	s := a.sessions[keep]
	s.Epoch = a.epoch
	a.sessions = map[string]adminSession{keep: s}
}
func (a *app) upload(w http.ResponseWriter, r *http.Request) error {
	input, e := readBody(w, r, 18*1024*1024)
	if e != nil {
		return e
	}
	data, e := base64.StdEncoding.DecodeString(str(input["data"]))
	if e != nil || len(data) == 0 || len(data) > 12*1024*1024 {
		return problem(400, "素材格式无效或超过12MB")
	}
	ext := strings.ToLower(filepath.Ext(str(input["name"])))
	if ext == ".jpeg" {
		ext = ".jpg"
	}
	types := map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".webp": "image/webp", ".mp4": "video/mp4"}
	mime, ok := types[ext]
	if !ok || http.DetectContentType(data) != mime {
		return problem(400, "素材类型与扩展名不匹配")
	}
	hash := sha256.Sum256(data)
	name := hex.EncodeToString(hash[:]) + ext
	if e = atomicFile(filepath.Join(a.assets, name), data); e != nil {
		return e
	}
	jsonResponse(w, 200, M{"url": "/api/assets/" + name})
	return nil
}
