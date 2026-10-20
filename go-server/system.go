package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"fmt"
	mysql "github.com/go-sql-driver/mysql"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// System endpoints share the existing authenticated admin and origin checks.
func (a *app) adminSystem(w http.ResponseWriter, r *http.Request) error {
	switch r.URL.Path {
	case "/api/admin/system/update", "/api/admin/system/update/check", "/api/admin/system/update/install":
		return a.adminUpdate(w, r)
	case "/api/admin/system/connections", "/api/admin/system/connections/test":
		return a.adminConnections(w, r)
	case "/api/admin/system/overview":
		if r.Method != "GET" {
			return problem(405, "不支持此方法")
		}
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		dbStarted := time.Now()
		database := M{"mode": a.config.Storage, "ok": false}
		if s, ok := a.store.(*mysqlStore); ok {
			database["ok"] = s.db.PingContext(ctx) == nil
			if d, err := mysql.ParseDSN(a.config.MySQLDSN); err == nil {
				database["address"] = d.Addr
				database["name"] = d.DBName
				database["username"] = d.User
			}
			var version string
			if s.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version) == nil {
				database["version"] = version
			}
			settings := effectiveDatabase(a.config.Database)
			database["maxIdle"] = settings.MaxIdle
			database["lifetimeSeconds"] = settings.LifetimeSeconds
			stats := s.db.Stats()
			database["pool"] = M{"open": stats.OpenConnections, "inUse": stats.InUse, "idle": stats.Idle, "maxOpen": stats.MaxOpenConnections, "waitCount": stats.WaitCount, "waitDurationMs": stats.WaitDuration.Milliseconds()}
		} else {
			_, err := a.store.Load(ctx)
			database["ok"] = err == nil
		}
		database["latencyMs"] = time.Since(dbStarted).Milliseconds()
		v := a.readState()
		probeCtx, stopProbe := context.WithTimeout(r.Context(), 8*time.Second)
		defer stopProbe()
		report := a.checkCMS(probeCtx, v.CMSBase)
		redisCtx, cancelRedis := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancelRedis()
		redisReport := a.cache.overview(redisCtx)
		hostname, _ := os.Hostname()
		jsonResponse(w, 200, M{"host": hostname, "pid": os.Getpid(), "memory": systemMemory(), "requests": a.requests.snapshot(), "cms": report, "version": buildVersion, "goVersion": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "startedAt": a.startedAt.UTC().Format(time.RFC3339), "uptimeSeconds": int64(time.Since(a.startedAt).Seconds()), "cpuCores": runtime.NumCPU(), "goroutines": runtime.NumGoroutine(), "heapBytes": mem.HeapAlloc, "runtimeBytes": mem.Sys, "database": database, "cmsBase": v.CMSBase, "revision": v.Revision, "installedAt": v.Setup["completedAt"], "redis": redisReport, "http": a.outbound.snapshot(), "cpuUsage": "暂不支持", "checkedAt": now()})
		return nil
	case "/api/admin/system/config":
		a.systemMu.Lock()
		defer a.systemMu.Unlock()
		if r.Method != "GET" && r.Method != "PUT" {
			return problem(405, "不支持此方法")
		}
		disk := a.config
		if a.configPath != "" {
			var err error
			disk, err = loadConfig(a.configPath)
			if err != nil {
				return problem(500, "无法读取配置文件，请检查文件权限和配置格式")
			}
		}
		if r.Method == "PUT" {
			if a.configPath == "" {
				return problem(409, "当前实例未连接可写配置文件")
			}
			input, err := readBody(w, r, 16*1024)
			if err != nil {
				return err
			}
			if len(input) == 0 {
				return problem(400, "未提交配置项")
			}
			for k, value := range input {
				switch k {
				case "listen":
					v, ok := value.(string)
					if !ok {
						return problem(400, "监听地址需为文本")
					}
					disk.Listen = strings.TrimSpace(v)
					_, port, e := net.SplitHostPort(disk.Listen)
					n, ne := strconv.Atoi(port)
					if e != nil || ne != nil || n < 1 || n > 65535 {
						return problem(400, "监听端口需为1至65535的数字")
					}
				case "adminPath":
					v, ok := value.(string)
					if !ok || !regexp.MustCompile(`^/[a-f0-9]{6}$`).MatchString(strings.TrimSpace(v)) {
						return problem(400, "后台入口需为 / 加6位小写数字或 a-f 字母")
					}
					disk.AdminPath = strings.TrimSpace(v)
				case "publicOrigin":
					v, ok := value.(string)
					if !ok {
						return problem(400, "公开服务地址需为文本")
					}
					disk.PublicOrigin = strings.TrimSpace(v)
				case "clientOrigins":
					raw, ok := value.([]any)
					if !ok || len(raw) > 32 {
						return problem(400, "客户端来源需为列表，最多32项")
					}
					disk.ClientOrigins = []string{}
					for _, item := range raw {
						v, ok := item.(string)
						if !ok {
							return problem(400, "客户端来源必须为地址")
						}
						disk.ClientOrigins = append(disk.ClientOrigins, strings.TrimSpace(v))
					}
				default:
					return problem(400, "包含不支持的配置项")
				}
			}
			if disk, err = validateConfig(disk); err != nil {
				return problem(400, err.Error())
			}
			b, err := json.MarshalIndent(disk, "", "  ")
			if err != nil {
				return err
			}
			if err = atomicFile(a.configPath, b); err != nil {
				return problem(500, "配置保存失败，请检查目录写入权限")
			}
			// Audit contains only action metadata, never configuration secrets.
			a.mu.Lock()
			f, e := os.OpenFile(filepath.Join(a.config.DataDir, "audit.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if e == nil {
				_ = json.NewEncoder(f).Encode(M{"at": now(), "event": "system.config.saved", "revision": a.state.Revision})
				_ = f.Close()
			}
			a.mu.Unlock()
		}
		public := func(c config) M {
			return M{"listen": c.Listen, "publicOrigin": c.PublicOrigin, "clientOrigins": c.ClientOrigins, "adminPath": shortAdminPath(c.AdminPath)}
		}
		active, saved := public(a.config), public(disk)
		ab, _ := json.Marshal(active)
		sb, _ := json.Marshal(saved)
		pub, err := x509.MarshalPKIXPublicKey(&a.serviceKey.PublicKey)
		if err != nil {
			return err
		}
		fingerprint := fmt.Sprintf("%x", sha256.Sum256(pub))
		jsonResponse(w, 200, M{"identity": M{"algorithm": "RSA-2048 / SHA-256", "fingerprint": fingerprint, "directoryPath": "/api/service-directory"}, "auditPath": filepath.Join(a.config.DataDir, "audit.jsonl"), "active": active, "saved": saved, "restartRequired": string(ab) != string(sb), "writable": a.configPath != "", "storage": a.config.Storage, "adminPath": shortAdminPath(a.config.AdminPath), "dataDir": a.config.DataDir})
		return nil
	case "/api/admin/system/logs":
		if r.Method != "GET" {
			return problem(405, "不支持此方法")
		}
		filter := r.URL.Query().Get("filter")
		if !has([]string{"", "all", "system", "account", "cms"}, filter) {
			return problem(400, "不支持的日志分类")
		}
		f, err := os.Open(filepath.Join(a.config.DataDir, "audit.jsonl"))
		rows := []M{}
		truncated := false
		if err != nil && !os.IsNotExist(err) {
			return problem(500, "审计日志读取失败")
		}
		if err == nil {
			defer f.Close()
			info, e := f.Stat()
			if e != nil {
				return problem(500, "审计日志读取失败")
			}
			const limit int64 = 1024 * 1024
			if info.Size() > limit {
				_, e = f.Seek(-limit, io.SeekEnd)
				if e != nil {
					return e
				}
				truncated = true
			}
			b, e := io.ReadAll(io.LimitReader(f, limit))
			if e != nil {
				return e
			}
			lines := strings.Split(string(b), "\n")
			if truncated && len(lines) > 0 {
				lines = lines[1:]
			}
			for _, line := range lines {
				var v M
				if json.Unmarshal([]byte(line), &v) != nil {
					continue
				}
				event := str(v["event"])
				if filter != "" && filter != "all" && !strings.Contains(event, filter) {
					continue
				}
				rows = append(rows, M{"at": str(v["at"]), "event": event, "revision": v["revision"]})
			}
			if len(rows) > 500 {
				rows = rows[len(rows)-500:]
				truncated = true
			}
		}
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
		jsonResponse(w, 200, M{"items": rows, "truncated": truncated, "limit": 500, "source": "audit.jsonl", "runtimeLogs": "请在宝塔进程管理器查看服务标准输出", "checkedAt": now()})
		return nil
	}
	return problem(404, "系统接口不存在")
}
