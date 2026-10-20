package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var buildVersion = "0.3.5"
var errNoState = sql.ErrNoRows

func loadConfig(name string) (config, error) {
	c := config{Listen: "127.0.0.1:8787", Storage: "mysql"}
	b, e := os.ReadFile(name)
	if e != nil {
		return c, fmt.Errorf("配置文件不可读: %w", e)
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	return validateConfig(c)
}
func validateConfig(c config) (config, error) {
	if c.Database != nil {
		if e := c.Database.validate(); e != nil {
			return c, e
		}
	}
	if c.HTTP != nil {
		if e := c.HTTP.validate(); e != nil {
			return c, e
		}
	}
	if _, _, e := net.SplitHostPort(c.Listen); e != nil {
		return c, fmt.Errorf("listen 必须是 host:port")
	}
	u, e := url.Parse(c.PublicOrigin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return c, fmt.Errorf("publicOrigin 必须是纯域名来源，不含路径或尾斜线")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return c, fmt.Errorf("访问地址必须使用 HTTP 或 HTTPS")
	}
	if c.AdminPath != "" && !regexp.MustCompile("^/([a-f0-9]{6}|[a-f0-9]{16})$").MatchString(c.AdminPath) {
		return c, fmt.Errorf("后台入口格式不正确")
	}
	if !filepath.IsAbs(c.DataDir) {
		return c, fmt.Errorf("dataDir 必须是可写的绝对路径")
	}
	c.DataDir = filepath.Clean(c.DataDir)
	if c.Storage != "mysql" && c.Storage != "file" {
		return c, fmt.Errorf("storage 仅支持 mysql 或 file")
	}
	if c.Storage == "mysql" && c.MySQLDSN == "" {
		return c, fmt.Errorf("mysql 模式需配置 mysqlDSN")
	}
	if c.Storage == "file" && u.Hostname() != "127.0.0.1" {
		return c, fmt.Errorf("file 模式仅用于本机测试，正式环境请选择 MySQL")
	}
	for _, origin := range c.ClientOrigins {
		v, e := url.Parse(origin)
		if e != nil || v.Scheme != "https" || v.Host == "" || v.User != nil || v.Path != "" || v.RawQuery != "" || v.Fragment != "" {
			return c, fmt.Errorf("clientOrigins 必须是 HTTPS 来源")
		}
	}
	if c.Redis != nil {
		if e := c.Redis.normalize(); e != nil {
			return c, e
		}
	}
	return c, nil
}
func run() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	configPath := flag.String("config", filepath.Join(filepath.Dir(exe), "config.json"), "配置文件路径（默认程序同目录）")
	listen := flag.String("listen", "0.0.0.0:8787", "首次安装监听地址")
	version := flag.Bool("version", false, "打印版本")
	check := flag.Bool("check-config", false, "检查配置")
	backup := flag.String("backup", "", "将业务数据导出到指定绝对文件")
	importPath := flag.String("import-state", "", "首次启动前从备份导入业务数据")
	flag.Parse()
	if *version {
		fmt.Println("hetun-operations " + buildVersion)
		return nil
	}
	c, e := loadConfig(*configPath)
	if e != nil {
		if errors.Is(e, os.ErrNotExist) && !*check && *backup == "" && *importPath == "" {
			installer, err := newInstaller(*configPath, *listen)
			if err != nil {
				return err
			}
			defer installer.Close()
			logStartup(*listen, "", installer.adminPath(), true)
			return serveWithUpdates(*listen, installer, installer.updater.restart)
		}
		return e
	}
	if *check {
		fmt.Println("配置格式正确")
		return nil
	}
	if e = os.MkdirAll(c.DataDir, 0700); e != nil {
		return e
	}
	var s store
	if c.Storage == "mysql" {
		s, e = openMySQL(c.MySQLDSN, c.Database)
	} else {
		s = &fileStore{filepath.Join(c.DataDir, "state.json")}
	}
	if e != nil {
		return e
	}
	defer s.Close()
	ctx := context.Background()
	if *importPath != "" {
		if _, e = s.Load(ctx); !errors.Is(e, errNoState) {
			return fmt.Errorf("目标必须是尚未初始化的空库；不会覆盖已有数据")
		}
		b, e := os.ReadFile(*importPath)
		if e != nil {
			return e
		}
		var v state
		if e = json.Unmarshal(b, &v); e != nil {
			return e
		}
		if e = validateState(v); e != nil {
			return e
		}
		if v.Credential.Algorithm != "scrypt" || !idPattern.MatchString(v.Credential.Username) || len(v.Credential.Hash) != 64 || len(v.Credential.Salt) < 16 {
			return fmt.Errorf("备份缺少有效管理员密码摘要")
		}
		for _, p := range platforms {
			r := v.Operations[p]
			r.Draft, _ = operationConfig(r.Draft)
			r.Published, _ = operationConfig(r.Published)
			for i := range r.History {
				clean, e := operationConfig(r.History[i].Config)
				if e != nil {
					return e
				}
				r.History[i].Config = clean
			}
		}
		v.Revision = 1
		if e = s.Save(ctx, v, 0); e != nil {
			return e
		}
		fmt.Println("业务备份已导入；素材目录请单独恢复")
		return nil
	}
	a, e := newApp(ctx, c, s)
	if e != nil {
		return e
	}
	defer a.cache.close()
	if *backup != "" {
		if !filepath.IsAbs(*backup) {
			return fmt.Errorf("备份文件必须为绝对路径")
		}
		b, e := json.MarshalIndent(a.readState(), "", "  ")
		if e != nil {
			return e
		}
		if e = atomicFile(*backup, b); e != nil {
			return e
		}
		fmt.Println("业务备份完成；素材目录请单独备份")
		return nil
	}
	if c.AdminPath == "" {
		c.AdminPath = "/" + randomToken()[:6]
		b, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return err
		}
		if err = atomicFile(*configPath, b); err != nil {
			return fmt.Errorf("保存后台入口失败: %w", err)
		}
		a.config = c
	}
	a.configPath, e = filepath.Abs(*configPath)
	if e != nil {
		return e
	}
	executable, e := filepath.EvalSymlinks(exe)
	if e != nil {
		return e
	}
	a.updater = newSystemUpdater(executable, a.configPath)
	logStartup(c.Listen, c.PublicOrigin, c.AdminPath, false)
	go a.autoConnectCMS()
	return serveWithUpdates(c.Listen, a, a.updater.restart)
}

// Keep persisted legacy entries valid while using a shorter canonical address.
func shortAdminPath(entry string) string {
	if len(entry) == 17 {
		return entry[:7]
	}
	return entry
}
func logStartup(listen, origin, adminPath string, installing bool) {
	adminPath = shortAdminPath(adminPath)
	_, port, _ := net.SplitHostPort(listen)
	log.Print("========================================")
	log.Printf("河豚运营中心 %s", buildVersion)
	log.Printf("端口：%s", port)
	log.Printf("后台访问路径：%s", adminPath)
	if origin == "" {
		log.Printf("后台地址示例：http://服务器IP:%s%s", port, adminPath)
		log.Printf("绑定域名后访问：https://你的域名%s", adminPath)
	} else {
		log.Printf("后台地址：%s%s", origin, adminPath)
	}
	if installing {
		log.Print("首次启动：打开上述后台地址，填写数据库并创建管理员")
	}
	log.Print("========================================")
}
func serve(listen string, handler http.Handler) error { return serveWithUpdates(listen, handler, nil) }
func serveWithUpdates(listen string, handler http.Handler, updates <-chan *preparedUpdate) error {
	server := &http.Server{Addr: listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 * 1024}
	listener, e := net.Listen("tcp", listen)
	if e != nil {
		return e
	}
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopped)
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	log.Print("启动成功")
	var update *preparedUpdate
	select {
	case <-stopped:
	case update = <-updates:
	case e = <-served:
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if e = server.Shutdown(ctx); e != nil {
		_ = server.Close()
		if update != nil {
			os.Remove(update.candidate)
		}
		return fmt.Errorf("关闭服务失败: %w", e)
	}
	if update != nil {
		return update
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		var update *preparedUpdate
		if errors.As(e, &update) {
			e = activateUpdate(update)
		}
		if e != nil {
			log.Fatal(e)
		}
	}
}
