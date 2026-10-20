package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mysql "github.com/go-sql-driver/mysql"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed install.html
var installHTML string

type installInput struct {
	Redis         *redisConfig `json:"redis"`
	PublicOrigin  string       `json:"publicOrigin"`
	Host          string       `json:"host"`
	Port          int          `json:"port"`
	Database      string       `json:"database"`
	Username      string       `json:"username"`
	Password      string       `json:"password"`
	Admin         string       `json:"admin"`
	AdminPassword string       `json:"adminPassword"`
}
type installer struct {
	mu                                                 sync.RWMutex
	configPath, dataDir, listen, route, installationID string
	active                                             *app
	connect                                            func(string) (store, error)
	probe                                              func(context.Context, string) error
}

func newInstaller(configPath, listen string) (*installer, error) {
	if _, _, e := net.SplitHostPort(listen); e != nil {
		return nil, fmt.Errorf("安装监听地址需为 host:port")
	}
	absolute, e := filepath.Abs(configPath)
	if e != nil {
		return nil, e
	}
	dir := filepath.Join(filepath.Dir(absolute), "data")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	tokenFile := filepath.Join(dir, "install-token.txt")
	b, e := os.ReadFile(tokenFile)
	if errors.Is(e, os.ErrNotExist) {
		b = []byte(randomToken())
		e = atomicFile(tokenFile, b)
	}
	if e != nil {
		return nil, e
	}
	token := strings.TrimSpace(string(b))
	if len(token) != 64 {
		return nil, fmt.Errorf("安装入口文件损坏，请核对 data/install-token.txt")
	}
	if _, e = hex.DecodeString(token); e != nil {
		return nil, fmt.Errorf("安装入口文件格式错误")
	}
	digest := sha256.Sum256([]byte(token))
	return &installer{configPath: absolute, dataDir: dir, listen: listen, route: "/install/" + token, installationID: hex.EncodeToString(digest[:]), connect: func(dsn string) (store, error) { return openMySQL(dsn) }, probe: probeMySQL}, nil
}
func (i *installer) adminPath() string { return "/" + strings.TrimPrefix(i.route, "/install/")[:6] }
func probeMySQL(ctx context.Context, dsn string) error {
	db, e := sql.Open("mysql", dsn)
	if e != nil {
		return e
	}
	defer db.Close()
	return db.PingContext(ctx)
}

// Return actionable categories without exposing driver messages, DSNs or credentials.
func installationDatabaseError(stage string, err error) M {
	code, message := "database_error", "数据库操作失败，请检查数据库服务与当前账号权限"
	var my *mysql.MySQLError
	var dns *net.DNSError
	var network net.Error
	switch {
	case errors.As(err, &my):
		switch my.Number {
		case 1045, 1698:
			code, message = "authentication_failed", "数据库认证失败：请核对数据库用户名和密码（不是宝塔登录密码），以及该账号是否允许本机 TCP 登录"
		case 1049:
			code, message = "database_not_found", "数据库不存在：请在宝塔确认完整数据库名，复制实际名称后重试"
		case 1044, 1142, 1143, 1227:
			code, message = "permission_denied", "数据库权限不足：请检查当前账号对指定库的访问、建表、读取和写入权限"
		case 1129, 1130:
			code, message = "host_denied", "数据库拒绝此来源连接：请检查当前账号的本机访问权限及 MySQL 主机限制"
		case 1040, 1203, 1226:
			code, message = "connection_limit", "数据库连接或资源额度已满，请检查当前数据库账号的连接占用与限额"
		}
	case errors.Is(err, context.DeadlineExceeded):
		code, message = "connection_timeout", "数据库连接或操作超时：请检查地址、端口、服务监听及网络连通性"
	case errors.As(err, &dns):
		code, message = "host_not_found", "数据库地址无法解析：请检查数据库主机名"
	case errors.Is(err, syscall.ECONNREFUSED):
		code, message = "connection_refused", "数据库端口拒绝连接：请检查数据库服务是否运行，以及地址、端口和监听设置"
	case errors.As(err, &network) && network.Timeout():
		code, message = "connection_timeout", "数据库连接或操作超时：请检查地址、端口、服务监听及网络连通性"
	}
	result := M{"error": message, "code": code, "stage": stage}
	if my != nil {
		result["mysqlCode"] = my.Number
	}
	return result
}
func (i *installer) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.active != nil {
		i.active.cache.close()
		return i.active.store.Close()
	}
	return nil
}
func (i *installer) configuration(v installInput) (config, error) {
	if v.Redis == nil {
		return config{}, fmt.Errorf("请在第一步配置 Redis 地址和认证信息")
	}
	if v.Host == "" || len(v.Host) > 253 || strings.ContainsAny(v.Host, "/\\ \t\r\n") || v.Port < 1 || v.Port > 65535 || v.Database == "" || v.Username == "" {
		return config{}, fmt.Errorf("请填写有效数据库地址、端口、库名与用户名")
	}
	db := mysql.NewConfig()
	db.User = v.Username
	db.Passwd = v.Password
	db.Net = "tcp"
	db.Addr = net.JoinHostPort(v.Host, strconv.Itoa(v.Port))
	db.DBName = v.Database
	db.Params = map[string]string{"charset": "utf8mb4"}
	db.Timeout = 5 * time.Second
	db.ReadTimeout = 10 * time.Second
	db.WriteTimeout = 10 * time.Second
	return validateConfig(config{AdminPath: i.adminPath(), Listen: i.listen, PublicOrigin: strings.TrimSpace(v.PublicOrigin), DataDir: i.dataDir, Storage: "mysql", Redis: v.Redis, MySQLDSN: db.FormatDSN(), ClientOrigins: []string{}})
}
func (i *installer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	i.mu.RLock()
	active := i.active
	i.mu.RUnlock()
	if active != nil {
		if strings.HasPrefix(r.URL.Path, "/install/") {
			jsonResponse(w, 404, M{"error": "安装入口已关闭", "code": "installation_closed"})
			return
		}
		active.ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	if r.Method == "GET" && r.URL.Path == "/api/health" {
		jsonResponse(w, 200, M{"status": "installation-required", "version": buildVersion, "runtime": "Go"})
		return
	}
	if r.Method == "GET" && r.URL.Path == "/" {
		http.Redirect(w, r, i.adminPath(), http.StatusTemporaryRedirect)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/"+strings.TrimPrefix(i.route, "/install/")[:16] {
		http.Redirect(w, r, i.adminPath(), http.StatusTemporaryRedirect)
		return
	}
	if r.URL.Path != i.adminPath() && r.URL.Path != i.route && r.URL.Path != i.route+"/test" && r.URL.Path != i.route+"/complete" {
		http.NotFound(w, r)
		return
	}
	if r.Method == "GET" && (r.URL.Path == i.route || r.URL.Path == i.adminPath()) {
		nonce := randomToken()
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"'; style-src 'unsafe-inline'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(installHTML, "NONCE_PLACEHOLDER", nonce), "INSTALL_ENDPOINT_PLACEHOLDER", i.route), "ADMIN_ENTRY_PLACEHOLDER", i.adminPath()))
		return
	}
	origin, e := url.Parse(r.Header.Get("Origin"))
	if r.Method != "POST" || e != nil || origin.Host != r.Host || (origin.Scheme != "https" && origin.Scheme != "http") || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || r.Header.Get("X-Hetun-Install") != "1" {
		jsonResponse(w, 403, M{"error": "安装请求来源不允许"})
		return
	}
	var v installInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&v); e != nil {
		jsonResponse(w, 400, M{"error": "安装参数格式错误"})
		return
	}
	if e = dec.Decode(&struct{}{}); e != io.EOF {
		jsonResponse(w, 400, M{"error": "安装参数格式错误"})
		return
	}
	c, e := i.configuration(v)
	if e != nil {
		jsonResponse(w, 400, M{"error": e.Error()})
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.active != nil {
		jsonResponse(w, 409, M{"error": "安装已完成，请进入后台"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cache := newContentCache(c.Redis, i.dataDir)
	defer cache.close()
	if e = cache.probe(ctx); e != nil {
		jsonResponse(w, 400, M{"error": "Redis 连接或读写校验失败，请检查服务是否启动、地址端口、密码及读写权限", "code": "redis_check_failed", "stage": "redis"})
		return
	}
	if strings.HasSuffix(r.URL.Path, "/test") {
		if e = i.probe(ctx, c.MySQLDSN); e != nil {
			jsonResponse(w, 400, installationDatabaseError("connect", e))
			return
		}
		jsonResponse(w, 200, M{"ok": true, "message": "MySQL 连接、Redis 连接及读写校验通过；完成安装时将检查建表和写入权限"})
		return
	}
	cred, e := credentials(v.Admin, v.AdminPassword)
	if e != nil {
		jsonResponse(w, 400, M{"error": e.Error()})
		return
	}
	if _, e = os.Stat(i.configPath); !errors.Is(e, os.ErrNotExist) {
		jsonResponse(w, 409, M{"error": "配置文件已存在或不可检查；请核对文件权限并重启程序"})
		return
	}
	s, e := i.connect(c.MySQLDSN)
	if e != nil {
		jsonResponse(w, 400, installationDatabaseError("initialize", e))
		return
	}
	keep := false
	defer func() {
		if !keep {
			s.Close()
		}
	}()
	existing, e := s.Load(ctx)
	if errors.Is(e, errNoState) {
		existing = initialState()
		existing.Credential = cred
		existing.Setup = M{"completedAt": now(), "installationID": i.installationID}
		if e = s.Save(ctx, existing, 0); e != nil {
			jsonResponse(w, 500, installationDatabaseError("write", e))
			return
		}
	} else if e != nil {
		jsonResponse(w, 500, installationDatabaseError("read", e))
		return
	} else if str(existing.Setup["installationID"]) != i.installationID || existing.Credential.Username != v.Admin || !matches(existing.Credential, v.AdminPassword) {
		jsonResponse(w, 409, M{"error": "此数据库已有后台数据，请使用独立空数据库；原数据未覆盖"})
		return
	}
	a, e := newApp(ctx, c, s)
	if e != nil {
		jsonResponse(w, 500, M{"error": "数据目录不可写或状态异常；请检查目录权限后使用相同账号重试"})
		return
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	if e = atomicFile(i.configPath, b); e != nil {
		a.cache.close()
		jsonResponse(w, 500, M{"error": "配置文件保存失败，请检查程序目录写入权限，然后使用相同账号重试"})
		return
	}
	a.configPath = i.configPath
	keep = true
	logStartup(c.Listen, c.PublicOrigin, c.AdminPath, false)
	i.active = a
	go a.autoConnectCMS()
	_ = os.Remove(filepath.Join(i.dataDir, "install-token.txt"))
	jsonResponse(w, 200, M{"ok": true, "redirect": i.adminPath() + "#/pages/studio/index"})
}
