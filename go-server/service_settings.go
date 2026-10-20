package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	mysql "github.com/go-sql-driver/mysql"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type databaseSettings struct {
	ConnectSeconds  int `json:"connectSeconds"`
	ReadSeconds     int `json:"readSeconds"`
	WriteSeconds    int `json:"writeSeconds"`
	MaxOpen         int `json:"maxOpen"`
	MaxIdle         int `json:"maxIdle"`
	LifetimeSeconds int `json:"lifetimeSeconds"`
	IdleSeconds     int `json:"idleSeconds"`
}
type httpSettings struct {
	TimeoutSeconds int `json:"timeoutSeconds"`
	MaxIdle        int `json:"maxIdle"`
	MaxIdlePerHost int `json:"maxIdlePerHost"`
	MaxPerHost     int `json:"maxPerHost"`
	IdleSeconds    int `json:"idleSeconds"`
}

func effectiveDatabase(s *databaseSettings) databaseSettings {
	if s != nil {
		return *s
	}
	return databaseSettings{10, 30, 30, 5, 2, 180, 60}
}
func effectiveHTTP(s *httpSettings) httpSettings {
	if s != nil {
		return *s
	}
	return httpSettings{12, 100, 32, 64, 60}
}
func (s databaseSettings) validate() error {
	if s.ConnectSeconds < 1 || s.ConnectSeconds > 60 || s.ReadSeconds < 1 || s.ReadSeconds > 120 || s.WriteSeconds < 1 || s.WriteSeconds > 120 || s.MaxOpen < 1 || s.MaxOpen > 500 || s.MaxIdle < 0 || s.MaxIdle > s.MaxOpen || s.LifetimeSeconds < 0 || s.LifetimeSeconds > 86400 || s.IdleSeconds < 0 || s.IdleSeconds > 86400 {
		return fmt.Errorf("MySQL 参数无效：超时需为正数，空闲连接不能超过最大连接，连接存活时间为0至86400秒")
	}
	return nil
}
func (s httpSettings) validate() error {
	if s.TimeoutSeconds < 1 || s.TimeoutSeconds > 120 || s.MaxIdle < 1 || s.MaxIdle > 1000 || s.MaxIdlePerHost < 1 || s.MaxIdlePerHost > s.MaxIdle || s.MaxPerHost < s.MaxIdlePerHost || s.MaxPerHost > 1000 || s.IdleSeconds < 1 || s.IdleSeconds > 3600 {
		return fmt.Errorf("HTTP 参数无效：请求超时1至120秒，单域名空闲连接不得超过全局空闲及单域名最大连接")
	}
	return nil
}
func databaseDSN(dsn string, s databaseSettings) (string, error) {
	c, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	c.Timeout = time.Duration(s.ConnectSeconds) * time.Second
	c.ReadTimeout = time.Duration(s.ReadSeconds) * time.Second
	c.WriteTimeout = time.Duration(s.WriteSeconds) * time.Second
	return c.FormatDSN(), nil
}
func configureDatabase(db *sql.DB, s databaseSettings) {
	db.SetMaxOpenConns(s.MaxOpen)
	db.SetMaxIdleConns(s.MaxIdle)
	db.SetConnMaxLifetime(time.Duration(s.LifetimeSeconds) * time.Second)
	db.SetConnMaxIdleTime(time.Duration(s.IdleSeconds) * time.Second)
}
func configuredCMSClient(c config) *http.Client {
	s := effectiveHTTP(c.HTTP)
	client := cmsClient(c.AllowPrivateCMS)
	t := client.Transport.(*http.Transport)
	client.Timeout = time.Duration(s.TimeoutSeconds) * time.Second
	t.MaxIdleConns = s.MaxIdle
	t.MaxIdleConnsPerHost = s.MaxIdlePerHost
	t.MaxConnsPerHost = s.MaxPerHost
	t.IdleConnTimeout = time.Duration(s.IdleSeconds) * time.Second
	return client
}
func decodeSettings(v any, out any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return problem(400, "配置格式错误")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return problem(400, "配置字段或数字格式错误")
	}
	return nil
}
func connectionView(c config) M {
	db := M{"mode": c.Storage, "settings": effectiveDatabase(c.Database)}
	if c.Storage == "mysql" {
		if d, err := mysql.ParseDSN(c.MySQLDSN); err == nil {
			db["address"] = d.Addr
			db["name"] = d.DBName
			db["username"] = d.User
		}
	}
	var redis any
	if c.Redis != nil {
		v := *c.Redis
		v.Password = ""
		redis = M{"values": v, "passwordSet": c.Redis.Password != ""}
	}
	return M{"database": db, "redis": redis, "http": effectiveHTTP(c.HTTP)}
}
func configRevision(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func connectionPending(a, b config) bool {
	x, _ := json.Marshal(M{"database": effectiveDatabase(a.Database), "redis": a.Redis, "http": effectiveHTTP(a.HTTP)})
	y, _ := json.Marshal(M{"database": effectiveDatabase(b.Database), "redis": b.Redis, "http": effectiveHTTP(b.HTTP)})
	return !bytes.Equal(x, y)
}
func (a *app) adminConnections(w http.ResponseWriter, r *http.Request) error {
	a.systemMu.Lock()
	defer a.systemMu.Unlock()
	if r.Method != "GET" && r.Method != "PUT" && !(r.Method == "POST" && r.URL.Path == "/api/admin/system/connections/test") {
		return problem(405, "不支持此方法")
	}
	if r.URL.Path == "/api/admin/system/connections/test" && r.Method != "POST" {
		return problem(405, "测试请使用POST")
	}
	disk := a.config
	var raw []byte
	var err error
	if a.configPath != "" {
		raw, err = os.ReadFile(a.configPath)
		if err != nil {
			return problem(500, "无法读取服务配置")
		}
		disk, err = loadConfig(a.configPath)
		if err != nil {
			return problem(500, "服务配置格式错误")
		}
	}
	if r.Method != "GET" {
		input, e := readBody(w, r, 16*1024)
		if e != nil {
			return e
		}
		for k := range input {
			if !has([]string{"section", "values", "revision", "passwordAction"}, k) {
				return problem(400, "包含未知配置项")
			}
		}
		section := str(input["section"])
		switch section {
		case "database":
			if disk.Storage != "mysql" {
				return problem(409, "当前为本地文件预览模式，未配置MySQL")
			}
			var s databaseSettings
			if e = decodeSettings(input["values"], &s); e != nil {
				return e
			}
			if e = s.validate(); e != nil {
				return problem(400, e.Error())
			}
			disk.Database = &s
		case "redis":
			var s redisConfig
			if e = decodeSettings(input["values"], &s); e != nil {
				return e
			}
			action := str(input["passwordAction"])
			switch action {
			case "keep":
				if disk.Redis != nil {
					s.Password = disk.Redis.Password
				} else {
					s.Password = ""
				}
			case "replace":
				if s.Password == "" {
					return problem(400, "新密码不能为空；无密码请明确选择清除")
				}
			case "clear":
				s.Password = ""
			default:
				return problem(400, "请明确Redis密码处理方式")
			}
			if e = s.normalize(); e != nil {
				return problem(400, e.Error())
			}
			disk.Redis = &s
		case "http":
			var s httpSettings
			if e = decodeSettings(input["values"], &s); e != nil {
				return e
			}
			if e = s.validate(); e != nil {
				return problem(400, e.Error())
			}
			disk.HTTP = &s
		default:
			return problem(400, "请选择数据库、Redis或HTTP配置")
		}
		if r.Method == "POST" {
			return a.testConnection(w, r, disk, section)
		}
		if a.configPath == "" {
			return problem(409, "未连接可写配置文件")
		}
		if str(input["revision"]) != configRevision(raw) {
			return problem(409, "服务配置已变化，请重新读取后保存")
		}
		if disk, err = validateConfig(disk); err != nil {
			return problem(400, "配置校验未通过")
		}
		raw, err = json.MarshalIndent(disk, "", "  ")
		if err != nil {
			return err
		}
		if err = atomicFile(a.configPath, raw); err != nil {
			return problem(500, "配置保存失败，请检查目录写入权限")
		}
		a.mu.Lock()
		f, e := os.OpenFile(filepath.Join(a.config.DataDir, "audit.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if e == nil {
			_ = json.NewEncoder(f).Encode(M{"at": now(), "event": "system.connections.saved", "revision": a.state.Revision})
			_ = f.Close()
		}
		a.mu.Unlock()
	}
	jsonResponse(w, 200, M{"active": connectionView(a.config), "saved": connectionView(disk), "revision": configRevision(raw), "restartRequired": connectionPending(a.config, disk), "writable": a.configPath != ""})
	return nil
}
func (a *app) testConnection(w http.ResponseWriter, r *http.Request, c config, section string) error {
	started := time.Now()
	var err error
	message := ""
	var checks any
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	switch section {
	case "redis":
		cache := newContentCache(c.Redis, c.DataDir)
		defer cache.close()
		err = cache.probe(ctx)
		if err == nil {
			message = "PING、临时键写入、读取与删除均通过"
		} else {
			message = "Redis连接或读写失败，请检查地址、认证及ACL权限"
		}
	case "database":
		dsn, e := databaseDSN(c.MySQLDSN, effectiveDatabase(c.Database))
		if e != nil {
			return problem(400, "数据库连接配置无效")
		}
		db, e := sql.Open("mysql", dsn)
		if e != nil {
			return problem(400, "数据库连接配置无效")
		}
		defer db.Close()
		configureDatabase(db, effectiveDatabase(c.Database))
		err = db.PingContext(ctx)
		if err == nil {
			var one int
			err = db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
		}
		if err == nil {
			message = "MySQL连接与SELECT 1查询通过；未执行建表或修改数据"
		} else {
			message = "MySQL连接或查询失败，请检查服务、账户权限及网络"
		}
	case "http":
		v := a.readState()
		if v.CMSBase == "" {
			return problem(409, "尚未连接苹果CMS，请先在苹果CMS中配置真实地址")
		}
		client := configuredCMSClient(c)
		defer client.CloseIdleConnections()
		probe := &app{cms: client}
		report := probe.checkCMS(ctx, v.CMSBase)
		checks = report
		ok, _ := report["ok"].(bool)
		if !ok {
			err = fmt.Errorf("CMS探测失败")
			message = "HTTP请求未通过，请查看逐项检测结果"
		} else {
			message = "使用当前表单参数完成CMS分类与影片请求"
		}
	}
	if ctx.Err() != nil && err != nil {
		message += "（探测达到15秒总时限）"
	}
	jsonResponse(w, 200, M{"ok": err == nil, "message": message, "latencyMs": time.Since(started).Milliseconds(), "checkedAt": now(), "report": checks})
	return nil
}
