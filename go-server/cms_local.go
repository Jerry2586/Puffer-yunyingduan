package main

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-sql-driver/mysql"
)

// This parser accepts only literal PHP arrays. It never evaluates PHP or returns
// source excerpts, which can contain database credentials.
type phpLiterals struct {
	s string
	i int
}

func (p *phpLiterals) skip() {
	for p.i < len(p.s) {
		if unicode.IsSpace(rune(p.s[p.i])) {
			p.i++
			continue
		}
		if strings.HasPrefix(p.s[p.i:], "//") || p.s[p.i] == '#' {
			for p.i < len(p.s) && p.s[p.i] != '\n' {
				p.i++
			}
			continue
		}
		if strings.HasPrefix(p.s[p.i:], "/*") {
			end := strings.Index(p.s[p.i+2:], "*/")
			if end < 0 {
				p.i = len(p.s)
				return
			}
			p.i += end + 4
			continue
		}
		break
	}
}
func (p *phpLiterals) take(s string) bool {
	p.skip()
	if strings.HasPrefix(p.s[p.i:], s) {
		p.i += len(s)
		return true
	}
	return false
}
func (p *phpLiterals) value(depth int) (any, error) {
	p.skip()
	bad := errors.New("本地 CMS 数据库配置不是受支持的静态 PHP 数组；未执行任何 PHP")
	if depth > 16 || p.i >= len(p.s) {
		return nil, bad
	}
	if p.s[p.i] == '\'' || p.s[p.i] == '"' {
		quote := p.s[p.i]
		p.i++
		var b strings.Builder
		for p.i < len(p.s) {
			c := p.s[p.i]
			p.i++
			if c == quote {
				return b.String(), nil
			}
			if quote == '"' && c == '$' {
				return nil, bad
			}
			if c == '\\' {
				if p.i >= len(p.s) {
					return nil, bad
				}
				n := p.s[p.i]
				p.i++
				if n == quote || n == '\\' {
					b.WriteByte(n)
				} else if quote == '\'' {
					b.WriteByte('\\')
					b.WriteByte(n)
				} else {
					switch n {
					case 'n':
						b.WriteByte('\n')
					case 'r':
						b.WriteByte('\r')
					case 't':
						b.WriteByte('\t')
					default:
						return nil, bad
					}
				}
			} else {
				b.WriteByte(c)
			}
		}
		return nil, bad
	}
	close := ""
	if p.take("[") {
		close = "]"
	} else if p.take("array") {
		if !p.take("(") {
			return nil, bad
		}
		close = ")"
	}
	if close != "" {
		m := M{}
		index := 0
		for {
			if p.take(close) {
				return m, nil
			}
			v, e := p.value(depth + 1)
			if e != nil {
				return nil, e
			}
			key := strconv.Itoa(index)
			if p.take("=>") {
				key = str(v)
				v, e = p.value(depth + 1)
				if e != nil {
					return nil, e
				}
			}
			m[key] = v
			index++
			if p.take(close) {
				return m, nil
			}
			if !p.take(",") {
				return nil, bad
			}
		}
	}
	start := p.i
	for p.i < len(p.s) && ((p.s[p.i] >= 'a' && p.s[p.i] <= 'z') || (p.s[p.i] >= '0' && p.s[p.i] <= '9') || p.s[p.i] == '-') {
		p.i++
	}
	token := p.s[start:p.i]
	switch token {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	if n, e := strconv.Atoi(token); e == nil {
		return n, nil
	}
	return nil, bad
}
func parseCMSDatabase(data []byte) (M, error) {
	p := phpLiterals{s: strings.TrimPrefix(string(data), "\ufeff")}
	bad := errors.New("本地 CMS 数据库配置格式不支持；仅支持静态 return 数组")
	if !p.take("<?php") || !p.take("return") {
		return nil, bad
	}
	v, e := p.value(0)
	if e != nil {
		return nil, e
	}
	p.take(";")
	p.take("?>")
	p.skip()
	if p.i != len(p.s) {
		return nil, bad
	}
	m, ok := v.(M)
	if !ok {
		return nil, bad
	}
	return m, nil
}

var cmsIdentifier = regexp.MustCompile(`^[A-Za-z0-9_]{0,48}$`)

func (a *app) cmsRoots() []string {
	if len(a.config.CMSRoots) > 0 {
		return a.config.CMSRoots
	}
	return []string{"/www/wwwroot"}
}
func (a *app) localCMSConfig(root string) (*mysql.Config, string, error) {
	fail := func(msg string) (*mysql.Config, string, error) { return nil, "", errors.New(msg) }
	real, e := filepath.EvalSymlinks(root)
	if e != nil {
		return fail("本地 CMS 目录不可读")
	}
	allowed := false
	for _, boundary := range a.cmsRoots() {
		b, err := filepath.EvalSymlinks(boundary)
		if err == nil && withinDirectory(b, real) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fail("本地 CMS 目录不在允许的站点范围内")
	}
	file, e := filepath.EvalSymlinks(filepath.Join(real, "application", "database.php"))
	if e != nil || !withinDirectory(real, file) {
		return fail("本地 CMS 数据库配置不可读或指向站点之外")
	}
	info, e := os.Stat(file)
	if e != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return fail("本地 CMS 数据库配置文件无效")
	}
	data, e := os.ReadFile(file)
	if e != nil {
		return fail("运营服务没有读取 CMS 数据库配置的权限")
	}
	m, e := parseCMSDatabase(data)
	if e != nil {
		return nil, "", e
	}
	if str(m["type"]) != "mysql" || str(m["dsn"]) != "" || numericOr(m["deploy"], 0) != 0 || (m["rw_separate"] == true || numericOr(m["rw_separate"], 0) != 0) {
		return fail("本地读取仅支持单机 MySQL 静态配置")
	}
	host := str(m["hostname"])
	if !has([]string{"localhost", "127.0.0.1", "::1"}, host) {
		return fail("本地读取仅连接同服务器 MySQL，请检查 CMS 数据库主机")
	}
	port := 3306
	if str(m["hostport"]) != "" {
		port, e = strconv.Atoi(str(m["hostport"]))
		if e != nil || port < 1 || port > 65535 {
			return fail("CMS 数据库端口无效")
		}
	}
	prefix := str(m["prefix"])
	if !cmsIdentifier.MatchString(prefix) {
		return fail("CMS 数据表前缀无效")
	}
	if str(m["database"]) == "" || str(m["username"]) == "" {
		return fail("CMS 数据库配置不完整")
	}
	c := mysql.NewConfig()
	c.User = str(m["username"])
	c.Passwd = str(m["password"])
	c.DBName = str(m["database"])
	c.Net = "tcp"
	c.Addr = net.JoinHostPort(host, strconv.Itoa(port))
	c.Timeout = 3 * time.Second
	c.ReadTimeout = 5 * time.Second
	c.WriteTimeout = 3 * time.Second
	c.Params = map[string]string{"charset": "utf8mb4"}
	return c, prefix, nil
}
func (a *app) localRootFor(base string) string {
	a.mu.RLock()
	saved, root := a.state.CMSBase, a.state.CMSLocalRoot
	a.mu.RUnlock()
	if root == "" {
		return ""
	}
	u, e := cmsEndpoint(base)
	v, err := cmsEndpoint(saved)
	if e != nil || err != nil || u.String() != v.String() {
		return ""
	}
	return root
}
func localCMSError(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		switch me.Number {
		case 1044, 1045:
			return errors.New("本地 CMS 数据库拒绝连接，请检查站点数据库账号的读取权限")
		case 1049:
			return errors.New("CMS 配置的数据库不存在")
		case 1146, 1054:
			return errors.New("本地 CMS 数据表结构不兼容，请检查表前缀与 CMS 版本")
		case 1142:
			return errors.New("CMS 数据库账号没有所需表的 SELECT 权限")
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errors.New("本地 CMS 读取超时或已取消")
	}
	return errors.New("本地 CMS 数据库连接或读取失败，请检查 MySQL 服务与读取权限")
}
func selectCMSRows(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]any, error) {
	rows, e := tx.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	cols, e := rows.Columns()
	if e != nil {
		return nil, e
	}
	out := []any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if e = rows.Scan(ptrs...); e != nil {
			return nil, e
		}
		m := M{}
		for i, k := range cols {
			if b, ok := values[i].([]byte); ok {
				m[k] = string(b)
			} else {
				m[k] = values[i]
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// All catalog reads are constrained to public video/category fields in a
// read-only transaction. No CMS schema, content, settings or user tables change.
func (a *app) localCatalog(ctx context.Context, root string, p url.Values) (M, error) {
	cfg, prefix, e := a.localCMSConfig(root)
	if e != nil {
		return nil, e
	}
	db, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		return nil, localCMSError(e)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return nil, localCMSError(e)
	}
	defer tx.Rollback()
	types := "`" + prefix + "type`"
	vod := "`" + prefix + "vod`"
	if p.Get("ac") == "list" {
		xs, err := selectCMSRows(ctx, tx, "SELECT type_id,type_pid,type_name FROM "+types+" WHERE type_mid=1 AND type_status=1 ORDER BY type_sort,type_id")
		if err != nil {
			return nil, localCMSError(err)
		}
		return M{"code": 1, "class": xs}, nil
	}
	where := "v.vod_status=1 AND t.type_mid=1 AND t.type_status=1"
	args := []any{}
	if category := p.Get("t"); category != "" {
		if !digits.MatchString(category) || len(category) > 20 {
			return nil, errors.New("分类 ID 必须为数字")
		}
		where += " AND (v.type_id=? OR v.type_id_1=?)"
		args = append(args, category, category)
	}
	if keyword := p.Get("wd"); keyword != "" {
		if len(keyword) > 300 {
			return nil, errors.New("检索参数过长")
		}
		keyword = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(keyword)
		where += " AND v.vod_name LIKE ? ESCAPE '!'"
		args = append(args, "%"+keyword+"%")
	}
	if ids := p.Get("ids"); ids != "" {
		xs := strings.Split(ids, ",")
		if len(xs) > 60 {
			return nil, errors.New("影片 ID 最多60个")
		}
		marks := []string{}
		for _, id := range xs {
			if !digits.MatchString(id) || len(id) > 20 {
				return nil, errors.New("影片 ID 必须为数字")
			}
			marks = append(marks, "?")
			args = append(args, id)
		}
		where += " AND v.vod_id IN (" + strings.Join(marks, ",") + ")"
	}
	from := " FROM " + vod + " v JOIN " + types + " t ON t.type_id=v.type_id WHERE " + where
	var total int
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*)"+from, args...).Scan(&total); e != nil {
		return nil, localCMSError(e)
	}
	page, _ := strconv.Atoi(p.Get("pg"))
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		page = 10000
	}
	limit := 20
	if p.Get("ids") != "" {
		limit = 60
	}
	fields := "v.vod_id,v.vod_name,v.vod_pic,v.type_id,t.type_name,v.vod_year,v.vod_area,v.vod_score,v.vod_actor,v.vod_director,v.vod_remarks,v.vod_state,v.vod_content,v.vod_blurb,v.vod_play_from,v.vod_play_url"
	queryArgs := append(append([]any{}, args...), limit, (page-1)*limit)
	xs, e := selectCMSRows(ctx, tx, "SELECT "+fields+from+" ORDER BY v.vod_time DESC,v.vod_id DESC LIMIT ? OFFSET ?", queryArgs...)
	if e != nil {
		return nil, localCMSError(e)
	}
	return M{"code": 1, "page": page, "pagecount": (total + limit - 1) / limit, "total": total, "list": xs}, nil
}
func (a *app) selectedCMS(ctx context.Context, input M) (M, string, error) {
	base := str(input["base"])
	root := a.localRootFor(base)
	if id := str(input["siteId"]); id != "" {
		candidates, _ := scanCMSSites(a.cmsRoots())
		addCMSBindings([]string{"/www/server/panel/vhost/nginx"}, candidates)
		preferKnownCMSAddress(candidates, base)
		found := false
		for _, c := range candidates {
			if str(c["id"]) == id {
				a.probeCMSCandidate(ctx, c)
				if obj(c["report"])["ok"] != true {
					return obj(c["report"]), "", nil
				}
				base = str(c["base"])
				root = str(c["localRoot"])
				found = true
				break
			}
		}
		if !found {
			return nil, "", problem(400, "站点已变化，请重新自动寻找 CMS")
		}
	}
	return a.checkCMSWithRoot(ctx, base, root), root, nil
}
