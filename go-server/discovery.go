package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Recognition reads public site metadata. The separate local adapter reads
// static database settings; no PHP is executed.
func publicCMSMetadata(data string, key string) string {
	re := regexp.MustCompile(`["']` + regexp.QuoteMeta(key) + `["']\s*=>\s*["']([^"'\r\n]{0,300})["']`)
	m := re.FindStringSubmatch(data)
	if len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
func withinDirectory(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func scanCMSSites(roots []string) ([]M, []string) {
	candidates, warnings := []M{}, []string{}
	seen := map[string]bool{}
	visited := 0
	var walk func(string, string, int)
	walk = func(dir, boundary string, depth int) {
		if visited >= 200 || len(candidates) >= 16 {
			return
		}
		visited++
		real, err := filepath.EvalSymlinks(dir)
		if err != nil || !withinDirectory(boundary, real) {
			return
		}
		if seen[real] {
			return
		}
		seen[real] = true
		file := filepath.Join(real, "application", "extra", "maccms.php")
		metadataFile, err := filepath.EvalSymlinks(file)
		if err == nil && withinDirectory(real, metadataFile) {
			info, statErr := os.Stat(metadataFile)
			api, apiErr := os.Stat(filepath.Join(real, "api.php"))
			if statErr == nil && !info.IsDir() && info.Size() <= 1024*1024 && apiErr == nil && !api.IsDir() {
				data, readErr := os.ReadFile(metadataFile)
				if readErr != nil {
					warnings = append(warnings, "部分 CMS 站点不可读，请为运营服务用户授予站点配置读取权限。")
					return
				}
				address := publicCMSMetadata(string(data), "site_url")
				base := ""
				if address == "" {
					directoryName := filepath.Base(real)
					if strings.Contains(directoryName, ".") && !strings.ContainsAny(directoryName, " :\\") {
						address = directoryName
					}
				}
				if address != "" {
					if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
						address = "https://" + address
					}
					installDir := publicCMSMetadata(string(data), "install_dir")
					if installDir != "" && installDir != "/" {
						if parsed, e := url.Parse(address); e == nil && !strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/"+strings.Trim(installDir, "/")) {
							address = strings.TrimRight(address, "/") + "/" + strings.Trim(installDir, "/")
						}
					}
					if u, e := cmsEndpoint(address); e == nil {
						base = u.String()
					}
				}
				name := publicCMSMetadata(string(data), "site_name")
				if name == "" {
					name = filepath.Base(real)
				}
				digest := sha256.Sum256([]byte(real))
				_, adapterErr := os.Stat(filepath.Join(real, "application", "index", "controller", "Hetun.php"))
				candidate := M{"id": hex.EncodeToString(digest[:10]), "root": real, "name": name, "base": base, "adapterInstalled": adapterErr == nil, "report": nil, "addresses": []M{}}
				addCMSAddress(candidate, base, "CMS 站点配置")
				directoryName := filepath.Base(real)
				if strings.Contains(directoryName, ".") && !strings.ContainsAny(directoryName, " :\\") {
					address := "https://" + directoryName
					if installDir := publicCMSMetadata(string(data), "install_dir"); installDir != "" && installDir != "/" {
						address += "/" + strings.Trim(installDir, "/")
					}
					addCMSAddress(candidate, address, "站点目录名称")
				}
				candidates = append(candidates, candidate)
				return
			}
		}
		if depth == 0 {
			return
		}
		entries, err := os.ReadDir(real)
		if err != nil {
			warnings = append(warnings, "扫描目录不可读："+real)
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if !entry.IsDir() || strings.HasPrefix(name, ".") || has([]string{"vendor", "node_modules", "runtime", "uploads", "cache", "data"}, name) {
				continue
			}
			walk(filepath.Join(real, name), boundary, depth-1)
		}
	}
	for i, root := range roots {
		if i >= 8 {
			break
		}
		real, err := filepath.EvalSymlinks(root)
		if err == nil {
			walk(real, real, 2)
		} else if !os.IsNotExist(err) {
			warnings = append(warnings, "扫描目录不可读："+root)
		}
	}
	return candidates, warnings
}

// Prefer a saved address only when exactly one discovered site's known aliases match.
// Stale CMS metadata must not turn one local site into two selectable sources.
func preferKnownCMSAddress(candidates []M, address string) {
	u, err := cmsEndpoint(address)
	if address == "" || err != nil {
		return
	}
	base := u.String()
	var matched M
	for _, c := range candidates {
		addresses, _ := c["addresses"].([]M)
		for _, alias := range addresses {
			if str(alias["base"]) == base {
				if matched != nil {
					return
				}
				matched = c
				break
			}
		}
	}
	if matched != nil {
		matched["base"] = base
	}
}

func (a *app) discoverAndConnect(parent context.Context) M {
	a.discoveryMu.Lock()
	defer a.discoveryMu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 24*time.Second)
	defer cancel()
	roots := a.config.CMSRoots
	if len(roots) == 0 {
		roots = []string{"/www/wwwroot"}
	}
	candidates, warnings := scanCMSSites(roots)
	warnings = append(warnings, addCMSBindings([]string{"/www/server/panel/vhost/nginx"}, candidates)...)
	v := a.readState()
	explicit := v.CMSBase
	if explicit == "" {
		explicit = a.config.CMSBase
	}
	preferKnownCMSAddress(candidates, explicit)
	seen := map[string]bool{}
	for _, c := range candidates {
		if str(c["base"]) != "" {
			seen[str(c["base"])] = true
		}
	}
	if explicit != "" {
		if u, e := cmsEndpoint(explicit); e == nil && !seen[u.String()] {
			candidate := M{"id": "configured-address", "name": "已配置的 CMS", "root": "", "base": u.String(), "adapterInstalled": false, "report": nil, "addresses": []M{}}
			addCMSAddress(candidate, u.String(), "当前配置")
			candidates = append(candidates, candidate)
		}
	}
	slots := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, c := range candidates {
		if str(c["base"]) == "" {
			continue
		}
		wg.Add(1)
		go func(c M) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return
			}
			a.probeCMSCandidate(ctx, c)
		}(c)
	}
	wg.Wait()
	usable := map[string]M{}
	for _, c := range candidates {
		if obj(c["report"])["ok"] == true {
			key := str(c["base"])
			if previous := usable[key]; previous == nil || str(c["localRoot"]) != "" {
				usable[key] = c
			}
		}
	}
	recommended := ""
	currentKey := v.CMSBase
	if u, err := cmsEndpoint(currentKey); currentKey != "" && err == nil {
		currentKey = u.String()
	}
	currentHealthy := usable[currentKey] != nil
	if ctx.Err() != nil {
		warnings = append(warnings, "本次扫描未全部完成，未自动切换内容源；请重新检测或选择已通过的站点。")
	}
	if len(usable) == 1 && ctx.Err() == nil {
		for base, c := range usable {
			recommended = str(c["id"])
			if !currentHealthy || (currentKey == base && str(c["localRoot"]) != "" && v.CMSLocalRoot != str(c["localRoot"])) {
				err := a.change(parent, "cms.auto-connected", func(next *state) error {
					if next.CMSBase != v.CMSBase || next.CMSLocalRoot != v.CMSLocalRoot {
						return problem(409, "连接已被其他操作更新，请重新检测")
					}
					next.CMSBase = base
					next.CMSLocalRoot = str(c["localRoot"])
					next.UpdatedAt = now()
					return nil
				})
				if err != nil {
					warnings = append(warnings, "探测通过，但保存连接失败："+err.Error())
				}
			}
		}
	}
	if c := usable[currentKey]; c != nil && str(c["localRoot"]) != "" && str(c["localRoot"]) != a.readState().CMSLocalRoot && ctx.Err() == nil {
		err := a.change(parent, "cms.local-connected", func(next *state) error {
			if next.CMSBase != v.CMSBase || next.CMSLocalRoot != v.CMSLocalRoot {
				return problem(409, "连接已被其他操作更新，请重新检测")
			}
			next.CMSLocalRoot = str(c["localRoot"])
			next.UpdatedAt = now()
			return nil
		})
		if err != nil {
			warnings = append(warnings, "保存本地连接失败："+err.Error())
		}
	}
	saved := a.readState().CMSBase
	savedKey := saved
	if u, err := cmsEndpoint(savedKey); savedKey != "" && err == nil {
		savedKey = u.String()
	}
	connected := usable[savedKey] != nil
	var currentReport any
	if selected := usable[savedKey]; selected != nil {
		currentReport = selected["report"]
	} else {
		for _, c := range candidates {
			if str(c["base"]) == savedKey {
				currentReport = c["report"]
				break
			}
		}
	}

	if len(candidates) == 0 {
		warnings = append(warnings, "未在同服务器 /www/wwwroot 下找到苹果 CMS；请先安装 CMS，或将站点目录加入 config.json 的 cmsRoots。")
	}
	return M{"candidates": candidates, "warnings": warnings, "recommendedId": recommended, "multiple": len(usable) > 1, "checkedAt": now(), "connected": connected, "connectedBase": saved, "currentReport": currentReport}
}
func (a *app) autoConnectCMS() {
	if a.readState().CMSLocalRoot != "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a.discoverAndConnect(ctx)
}

// Each attempted address is a real CMS content check; failed alternatives remain visible.
func (a *app) probeCMSCandidate(ctx context.Context, candidate M) {
	attempts := []M{}
	candidate["recognized"] = str(candidate["root"]) != ""
	if root := str(candidate["root"]); root != "" && str(candidate["base"]) != "" {
		attemptCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		report := a.checkCMSWithRoot(attemptCtx, str(candidate["base"]), root)
		cancel()
		attempts = append(attempts, M{"base": candidate["base"], "source": "同服务器本地只读", "report": report})
		candidate["report"] = report
		candidate["source"] = "同服务器本地只读"
		if report["ok"] == true {
			candidate["localRoot"] = root
			candidate["attempts"] = attempts
			return
		}
	}
	var localFailure any
	if len(attempts) > 0 {
		localFailure = attempts[0]["report"]
	}
	addresses, _ := candidate["addresses"].([]M)
	if len(addresses) == 0 {
		addresses = []M{{"base": candidate["base"], "source": "当前配置"}}
	}
	for i, address := range addresses {
		if i >= 6 || ctx.Err() != nil {
			break
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
		report := a.checkCMSWithRoot(attemptCtx, str(address["base"]), "")
		cancel()
		attempts = append(attempts, M{"base": address["base"], "source": address["source"], "report": report})
		candidate["report"] = report
		candidate["base"] = report["base"]
		candidate["source"] = address["source"]
		if report["ok"] == true {
			break
		}
	}
	if localFailure != nil && obj(candidate["report"])["ok"] != true {
		candidate["report"] = localFailure
		candidate["base"] = obj(localFailure)["base"]
		candidate["source"] = "同服务器本地只读"
	}
	candidate["attempts"] = attempts
	if candidate["report"] == nil {
		candidate["report"] = M{"base": candidate["base"], "configured": true, "ok": false, "status": "error", "checks": []any{}, "message": "本次探测时间已用完，请重新检测", "checkedAt": now()}
	}
}
func addCMSAddress(candidate M, address, source string) {
	u, err := cmsEndpoint(address)
	if err != nil {
		return
	}
	addresses, _ := candidate["addresses"].([]M)
	for _, item := range addresses {
		if item["base"] == u.String() {
			return
		}
	}
	if len(addresses) < 6 {
		candidate["addresses"] = append(addresses, M{"base": u.String(), "source": source})
	}
}
