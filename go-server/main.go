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
	"strings"
	"syscall"
	"time"
)

var buildVersion = "0.3.0"
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
	if _, _, e := net.SplitHostPort(c.Listen); e != nil {
		return c, fmt.Errorf("listen 必须是 host:port")
	}
	u, e := url.Parse(c.PublicOrigin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return c, fmt.Errorf("publicOrigin 必须是纯域名来源，不含路径或尾斜线")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && u.Hostname() == "127.0.0.1") {
		return c, fmt.Errorf("正式访问域名必须使用 HTTPS，本机测试可用 http://127.0.0.1")
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
	return c, nil
}
func run() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	configPath := flag.String("config", filepath.Join(filepath.Dir(exe), "config.json"), "配置文件路径（默认程序同目录）")
	listen := flag.String("listen", "127.0.0.1:8787", "首次安装监听地址")
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
			log.Printf("首次安装：监听 %s；请在宝塔绑定域名和HTTPS，访问 https://你的域名%s（临时安装入口，安装后关闭）", *listen, installer.route)
			return serve(*listen, installer)
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
		s, e = openMySQL(c.MySQLDSN)
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
	log.Printf("河豚运营端 %s 监听 %s", buildVersion, c.Listen)
	return serve(c.Listen, a)
}
func serve(listen string, handler http.Handler) error {
	server := &http.Server{Addr: listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 * 1024}
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopped)
	go func() {
		<-stopped
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	e := server.ListenAndServe()
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
func main() {
	if e := run(); e != nil {
		log.Fatal(e)
	}
}
