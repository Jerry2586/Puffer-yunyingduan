package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The catalog API stays read-only. CMS mutations use a separate signed adapter.
//
//go:embed cms/Hetunops.php
var managementAdapter []byte

type managementConnection struct {
	Base string `json:"base"`
	Key  string `json:"key"`
}

var managementKeyPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
var managementKinds = map[string]bool{"videos": true, "users": true, "comments": true, "danmu": true, "requests": true, "feedback": true, "reminders": true}

func managementBase(base string) string {
	u, e := siteURL(base)
	if e != nil {
		return ""
	}
	return strings.TrimSuffix(u.String(), "/")
}
func managementExpected(data M, base string) error {
	if base == "" {
		return problem(409, "请先在系统管理中设置 CMS 内容源")
	}
	if str(data["source"]) != base {
		return coded(409, "内容源已更改或列表已过期，请刷新后重试", "MANAGEMENT_SOURCE_CHANGED")
	}
	return nil
}
func (a *app) managementConnectionFor(v state) (*managementConnection, error) {
	c := v.ManagementConnection
	if c == nil || c.Key == "" || c.Base != managementBase(v.CMSBase) {
		return nil, coded(409, "请先连接苹果 CMS 管理适配器", "CMS_MANAGEMENT_SETUP")
	}
	return c, nil
}

// Capture the connection once: a source switch must never redirect an old write.
func (a *app) managementCall(ctx context.Context, c *managementConnection, action string, data M) (M, error) {
	u, e := siteURL(c.Base)
	if e != nil {
		return nil, e
	}
	u.Path += "index.php/hetunops/gateway"
	body, e := json.Marshal(M{"action": action, "data": data})
	if e != nil {
		return nil, problem(400, "管理参数无效")
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomToken()
	mac := hmac.New(sha256.New, []byte(c.Key))
	mac.Write([]byte(stamp + "\n" + nonce + "\n"))
	mac.Write(body)
	req, e := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(body))
	if e != nil {
		return nil, problem(400, "CMS 管理地址无效")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hetun-Time", stamp)
	req.Header.Set("X-Hetun-Nonce", nonce)
	req.Header.Set("X-Hetun-Signature", hex.EncodeToString(mac.Sum(nil)))
	uncertain := func() (M, error) {
		if action != "meta" && action != "playback.sources" && !strings.HasSuffix(action, ".list") {
			return nil, coded(502, "未收到 CMS 的操作确认，请先刷新列表核对结果，避免重复提交", "CMS_WRITE_UNCONFIRMED")
		}
		return nil, coded(502, "无法连接 CMS 管理适配器，请检查安装、密钥和服务状态", "CMS_MANAGEMENT_UNAVAILABLE")
	}
	res, e := a.cms.Do(req)
	if e != nil {
		return uncertain()
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024+1))
	if e != nil || len(raw) > 4*1024*1024 {
		return uncertain()
	}
	var out M
	if json.Unmarshal(raw, &out) != nil {
		return uncertain()
	}
	if res.StatusCode != 200 || out["code"] != float64(1) {
		switch str(out["error"]) {
		case "unauthorized":
			return nil, problem(502, "CMS 管理密钥不匹配或服务器时间偏差过大")
		case "invalid":
			return nil, problem(400, "CMS 拒绝操作，请检查填写的字段、会员有效期和所选记录")
		default:
			return uncertain()
		}
	}
	result, ok := out["data"].(map[string]any)
	if !ok || len(result) == 0 {
		return uncertain()
	}
	if !managementResultValid(action, result) {
		return uncertain()
	}
	result["source"] = c.Base
	return result, nil
}

// Accept only integral values; never silently truncate record IDs or versions.
func managementInteger(value any, minValue, maxValue int) (int, bool) {
	var n int64
	switch value := value.(type) {
	case int:
		n = int64(value)
	case int64:
		n = value
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) || value < float64(minValue) || value > float64(maxValue) {
			return 0, false
		}
		n = int64(value)
	case json.Number:
		var err error
		n, err = value.Int64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return int(n), n >= int64(minValue) && n <= int64(maxValue)
}
func managementResultValid(action string, data M) bool {
	integer := func(value any, min, max int) bool { _, ok := managementInteger(value, min, max); return ok }
	text := func(value any) bool { _, ok := value.(string); return ok }
	if action == "playback.sources" {
		items, ok := data["items"].([]any)
		if !ok || len(items) > 100 {
			return false
		}
		seen := map[string]bool{}
		for _, raw := range items {
			row := obj(raw)
			code := str(row["code"])
			if !resolverID.MatchString(code) || seen[code] || !text(row["name"]) {
				return false
			}
			if _, ok := row["enabled"].(bool); !ok {
				return false
			}
			seen[code] = true
		}
		return true
	}
	if strings.HasPrefix(action, "member.") && !integer(data["userId"], 1, 2147483647) {
		return false
	}
	if action == "meta" {
		version, ok := managementInteger(data["adapterVersion"], 1, 1)
		if !ok || version != 1 {
			return false
		}
		for _, key := range []string{"categories", "groups"} {
			items, ok := data[key].([]any)
			if !ok {
				return false
			}
			seen := map[int]bool{}
			for _, raw := range items {
				row, ok := raw.(map[string]any)
				if !ok {
					return false
				}
				id, ok := managementInteger(row["id"], 1, 2147483647)
				if !ok || seen[id] || !text(row["name"]) {
					return false
				}
				seen[id] = true
				if key == "categories" && !integer(row["parentId"], 0, 2147483647) {
					return false
				}
			}
		}
		return true
	}
	if strings.HasSuffix(action, ".list") {
		items, ok := data["items"].([]any)
		if !ok {
			return false
		}
		total, ok := managementInteger(data["total"], 0, 2147483647)
		if !ok || total < len(items) {
			return false
		}
		size, ok := managementInteger(data["pageSize"], 1, 100)
		if !ok || len(items) > size {
			return false
		}
		pages, ok := managementInteger(data["pages"], 1, 2147483647)
		if !ok || pages != max(1, (total+size-1)/size) {
			return false
		}
		if !integer(data["page"], 1, pages) {
			return false
		}
		seen := map[int]bool{}
		for _, raw := range items {
			row, ok := raw.(map[string]any)
			if !ok {
				return false
			}
			id, ok := managementInteger(row["id"], 1, 2147483647)
			if !ok || seen[id] || !text(row["name"]) || !integer(row["status"], 0, 1) {
				return false
			}
			seen[id] = true
			if action == "videos.list" {
				for _, key := range []string{"cover", "categoryName", "sources", "year", "area"} {
					if !text(row[key]) {
						return false
					}
				}
				if !integer(row["categoryId"], 0, 2147483647) || !integer(row["level"], 0, 9) {
					return false
				}
			} else {
				for _, key := range []string{"nickname", "groupId", "groupName", "lastLoginIP"} {
					if !text(row[key]) {
						return false
					}
				}
				for _, key := range []string{"points", "expiresAt", "lastLoginAt"} {
					if !integer(row[key], 0, 2147483647) {
						return false
					}
				}
			}
		}
		return true
	}
	return data["ok"] == true && integer(data["affected"], 1, 100)
}

func boundedInt(raw string, fallback, min, max int) int {
	n, e := strconv.Atoi(raw)
	if e != nil {
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
func managementIDs(data M) ([]int, error) {
	values := arr(data["ids"])
	if len(values) == 0 || len(values) > 100 {
		return nil, problem(400, "请选择 1–100 条记录")
	}
	ids := []int{}
	seen := map[int]bool{}
	for _, v := range values {
		n, valid := managementInteger(v, 1, 2147483647)
		if !valid {
			return nil, problem(400, "记录 ID 无效")
		}
		if !seen[n] {
			ids = append(ids, n)
			seen[n] = true
		}
	}
	return ids, nil
}
func (a *app) adminManagement(w http.ResponseWriter, r *http.Request) error {
	route := strings.TrimPrefix(r.URL.Path, "/api/admin/manage/")
	switch route {
	case "connection":
		v := a.readState()
		base := managementBase(v.CMSBase)
		if r.Method == "GET" {
			c := v.ManagementConnection
			jsonResponse(w, 200, M{"base": base, "configured": c != nil && c.Base == base && c.Key != "", "localAvailable": a.localRootFor(v.CMSBase) != ""})
			return nil
		}
		if r.Method != "POST" {
			return problem(405, "请求方法不支持")
		}
		data, e := readBody(w, r, 4096)
		if e != nil {
			return e
		}
		if e = managementExpected(data, base); e != nil {
			return e
		}
		key := str(data["key"])
		if !managementKeyPattern.MatchString(key) {
			return problem(400, "管理密钥必须是 64 位十六进制字符")
		}
		e = a.change(r.Context(), "management.connection", func(s *state) error {
			if managementBase(s.CMSBase) != base {
				return coded(409, "内容源已更改，请刷新", "MANAGEMENT_SOURCE_CHANGED")
			}
			s.ManagementConnection = &managementConnection{Base: base, Key: key}
			return nil
		})
		if e != nil {
			return e
		}
		jsonResponse(w, 200, M{"ok": true, "source": base})
		return nil
	case "connection/test", "meta":
		if route == "meta" && r.Method != "GET" || route == "connection/test" && r.Method != "POST" {
			return problem(405, "请求方法不支持")
		}
		c, e := a.managementConnectionFor(a.readState())
		if e != nil {
			return e
		}
		out, e := a.managementCall(r.Context(), c, "meta", M{})
		if e != nil {
			return e
		}
		jsonResponse(w, 200, out)
		return nil
	case "adapter":
		if r.Method != "GET" {
			return problem(405, "请求方法不支持")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=Hetunops.php")
		w.Header().Set("Cache-Control", "no-store")
		_, e := w.Write(managementAdapter)
		return e
	case "install":
		return a.installManagementAdapter(w, r)
	case "moderation":
		return a.manageModeration(w, r)
	}
	parts := strings.Split(route, "/")
	kind := parts[0]
	if !managementKinds[kind] || len(parts) > 2 || len(parts) == 2 && parts[1] != "action" {
		return problem(404, "管理模块不存在")
	}
	if kind != "videos" && kind != "users" {
		return a.manageCommunity(w, r, kind, len(parts) == 2)
	}
	v := a.readState()
	c, e := a.managementConnectionFor(v)
	if e != nil {
		return e
	}
	if len(parts) == 1 && r.Method == "GET" {
		q := r.URL.Query()
		data := M{"page": boundedInt(q.Get("page"), 1, 1, 1000000), "pageSize": boundedInt(q.Get("pageSize"), 20, 1, 100), "keyword": strings.TrimSpace(q.Get("keyword")), "status": q.Get("status"), "categoryId": q.Get("categoryId"), "groupId": q.Get("groupId"), "level": q.Get("level"), "id": q.Get("id"), "sort": q.Get("sort"), "order": q.Get("order")}
		if len(str(data["keyword"])) > 800 {
			return problem(400, "搜索内容过长")
		}
		out, e := a.managementCall(r.Context(), c, kind+".list", data)
		if e != nil {
			return e
		}
		jsonResponse(w, 200, out)
		return nil
	}
	if len(parts) != 2 || r.Method != "POST" {
		return problem(405, "请求方法不支持")
	}
	data, e := readBody(w, r, 32*1024)
	if e != nil {
		return e
	}
	if e = managementExpected(data, c.Base); e != nil {
		return e
	}
	action := str(data["action"])
	allowed := map[string]bool{"edit": true, "status": true}
	if kind == "videos" {
		allowed["category"] = true
		allowed["level"] = true
	} else {
		allowed["create"] = true
		allowed["delete"] = true
	}
	if !allowed[action] {
		return problem(400, "不支持的管理操作")
	}
	if action != "create" {
		ids, e := managementIDs(data)
		if e != nil {
			return e
		}
		if action == "edit" && len(ids) != 1 {
			return problem(400, "一次只能编辑一条记录")
		}
		data["ids"] = ids
	}
	out, e := a.managementCall(r.Context(), c, kind+"."+action, data)
	if e != nil {
		return e
	}
	// A committed CMS mutation must remain successful even if secondary cleanup fails.
	a.managementAfterWrite(c.Base, kind, action, data, out)
	jsonResponse(w, 200, out)
	return nil
}
func (a *app) managementAfterWrite(base, kind, action string, data, out M) {
	if kind == "users" {
		ids := map[int]bool{}
		for _, id := range arr(data["ids"]) {
			ids[number(id)] = true
		}
		if raw, ok := data["ids"].([]int); ok {
			for _, id := range raw {
				ids[id] = true
			}
		}
		fields := obj(data["fields"])
		status, explicitStatus := managementInteger(fields["status"], 0, 1)
		revoke := action == "delete" || action == "status" && number(data["value"]) == 0 || action == "edit" && (explicitStatus && status == 0 || str(fields["password"]) != "")
		a.mu.Lock()
		for token, s := range a.members {
			if managementBase(s.Base) == base {
				s.ProfileVersion.Add(1)
				if revoke && ids[s.UserID] {
					delete(a.members, token)
				}
			}
		}
		a.mu.Unlock()
	} else {
		a.searchMu.Lock()
		a.searchIndexes = map[string]*searchIndex{}
		for _, job := range a.searchJobs {
			if job.Phase == "running" {
				job.Phase = "failed"
				job.Message = "影片数据已更新，请重新建立索引"
				job.FinishedAt = now()
			}
		}
		a.searchMu.Unlock()
	}
	cacheOK := true
	if a.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		cacheKind := "catalog"
		if kind == "users" {
			cacheKind = "users"
		}
		for _, p := range platforms {
			_, e := a.clearClientCacheKind(ctx, p, cacheKind)
			if e != nil {
				cacheOK = false
			}
		}
	}
	event, _ := json.Marshal(M{"at": now(), "event": "management." + kind + "." + action, "affected": out["affected"]})
	f, e := os.OpenFile(filepath.Join(a.config.DataDir, "audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	auditOK := e == nil
	if e == nil {
		_, e = f.Write(append(event, '\n'))
		ce := f.Close()
		auditOK = e == nil && ce == nil
	}
	out["cacheInvalidated"] = cacheOK
	out["auditRecorded"] = auditOK
	warnings := []string{}
	if !cacheOK {
		warnings = append(warnings, "数据已保存，缓存刷新未完成，请到缓存管理中清理后核对")
	}
	if !auditOK {
		warnings = append(warnings, "数据已保存，审计日志写入失败，请检查数据目录权限")
	}
	out["warnings"] = warnings
}

// Installation is explicit and restricted to a validated local CMS root.
func (a *app) installManagementAdapter(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		return problem(405, "请求方法不支持")
	}
	data, e := readBody(w, r, 4096)
	if e != nil {
		return e
	}
	a.systemMu.Lock()
	defer a.systemMu.Unlock()
	v := a.readState()
	base := managementBase(v.CMSBase)
	if e = managementExpected(data, base); e != nil {
		return e
	}
	local := a.localRootFor(v.CMSBase)
	if local == "" {
		return problem(409, "当前内容源没有已验证的本地 CMS 目录，请手动安装适配器")
	}
	root, e := filepath.EvalSymlinks(local)
	if e != nil {
		return problem(400, "本地 CMS 目录验证失败")
	}
	root, e = filepath.Abs(root)
	if e != nil {
		return problem(400, "本地 CMS 目录验证失败")
	}
	if _, _, e := a.localCMSConfig(root); e != nil {
		return problem(400, "本地 CMS 目录验证失败")
	}
	key := randomToken()
	if c := v.ManagementConnection; c != nil && c.Base == base && c.Key != "" {
		key = c.Key
	}
	type target struct {
		rel    string
		body   []byte
		file   string
		exists bool
		info   os.FileInfo
	}
	targets := []target{{rel: "application/index/controller/Hetunops.php", body: managementAdapter}, {rel: "application/extra/hetun_operations.php", body: []byte("<?php\nreturn ['key' => '" + key + "'];\n")}}
	// Preflight every target before creating either file.
	for i := range targets {
		t := &targets[i]
		t.file = filepath.Join(root, filepath.FromSlash(t.rel))
		parent, pe := filepath.EvalSymlinks(filepath.Dir(t.file))
		rel, re := filepath.Rel(root, parent)
		if pe != nil || re != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return problem(400, "CMS 适配器目录无效")
		}
		existing, fe := os.Lstat(t.file)
		if fe == nil {
			if !existing.Mode().IsRegular() {
				return problem(409, "适配器位置不是普通文件")
			}
			b, be := os.ReadFile(t.file)
			if be != nil || !bytes.Equal(b, t.body) {
				return problem(409, "CMS 中已有不同适配器或密钥文件，请先手动检查")
			}
			t.exists = true
		} else if !os.IsNotExist(fe) {
			return problem(500, "无法检查 CMS 适配器文件")
		}
	}
	committed := false
	defer func() {
		if !committed {
			for _, t := range targets {
				if t.info != nil {
					if current, err := os.Lstat(t.file); err == nil && os.SameFile(t.info, current) {
						_ = os.Remove(t.file)
					}
				}
			}
		}
	}()
	for i := range targets {
		t := &targets[i]
		if t.exists {
			continue
		}
		f, fe := os.OpenFile(t.file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if fe != nil {
			return problem(500, "无法写入 CMS 适配器，请检查权限")
		}
		t.info, fe = f.Stat()
		if fe != nil {
			_ = f.Close()
			return problem(500, "无法验证已创建的 CMS 适配器文件，请检查目录")
		}
		_, we := f.Write(t.body)
		ce := f.Close()
		if we != nil || ce != nil {
			return problem(500, "写入 CMS 适配器失败")
		}
	}
	e = a.change(r.Context(), "management.install", func(s *state) error {
		if s.CMSBase != v.CMSBase || s.CMSLocalRoot != v.CMSLocalRoot {
			return coded(409, "内容源已更改，请刷新", "MANAGEMENT_SOURCE_CHANGED")
		}
		s.ManagementConnection = &managementConnection{Base: base, Key: key}
		return nil
	})
	if e != nil {
		return e
	}
	committed = true
	jsonResponse(w, 200, M{"ok": true, "source": base})
	return nil
}
