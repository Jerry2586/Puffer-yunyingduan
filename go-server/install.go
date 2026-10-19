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
	"time"
)

//go:embed install.html
var installHTML string

type installInput struct {
	PublicOrigin  string `json:"publicOrigin"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Database      string `json:"database"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Admin         string `json:"admin"`
	AdminPassword string `json:"adminPassword"`
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
func probeMySQL(ctx context.Context, dsn string) error {
	db, e := sql.Open("mysql", dsn)
	if e != nil {
		return e
	}
	defer db.Close()
	return db.PingContext(ctx)
}
func (i *installer) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.active != nil {
		return i.active.store.Close()
	}
	return nil
}
func (i *installer) configuration(v installInput) (config, error) {
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
	return validateConfig(config{Listen: i.listen, PublicOrigin: strings.TrimSpace(v.PublicOrigin), DataDir: i.dataDir, Storage: "mysql", MySQLDSN: db.FormatDSN(), ClientOrigins: []string{}})
}
func (i *installer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	i.mu.RLock()
	active := i.active
	i.mu.RUnlock()
	if active != nil {
		if strings.HasPrefix(r.URL.Path, "/install/") {
			http.NotFound(w, r)
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
	if r.URL.Path != i.route && r.URL.Path != i.route+"/test" && r.URL.Path != i.route+"/complete" {
		http.NotFound(w, r)
		return
	}
	if r.Method == "GET" && r.URL.Path == i.route {
		nonce := randomToken()
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"'; style-src 'unsafe-inline'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, strings.ReplaceAll(installHTML, "NONCE_PLACEHOLDER", nonce))
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
	if strings.HasSuffix(r.URL.Path, "/test") {
		if e = i.probe(ctx, c.MySQLDSN); e != nil {
			jsonResponse(w, 400, M{"error": "数据库连接失败，请检查地址、账号、密码、库名及访问权限"})
			return
		}
		jsonResponse(w, 200, M{"ok": true, "message": "数据库连接成功；完成安装时将检查建表和写入权限"})
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
		jsonResponse(w, 400, M{"error": "数据库初始化失败，请检查连接及建表权限"})
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
			jsonResponse(w, 500, M{"error": "数据库写入失败，请检查写入权限后重试"})
			return
		}
	} else if e != nil {
		jsonResponse(w, 500, M{"error": "无法读取数据库状态"})
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
		jsonResponse(w, 500, M{"error": "配置文件保存失败，请检查程序目录写入权限，然后使用相同账号重试"})
		return
	}
	keep = true
	i.active = a
	_ = os.Remove(filepath.Join(i.dataDir, "install-token.txt"))
	jsonResponse(w, 200, M{"ok": true, "redirect": "/#/pages/studio/index"})
}
