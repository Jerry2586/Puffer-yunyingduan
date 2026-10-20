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

// Discovery reads public MacCMS site metadata only, never database settings or PHP execution.
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
				candidates = append(candidates, M{"id": hex.EncodeToString(digest[:10]), "root": real, "name": name, "base": base, "adapterInstalled": adapterErr == nil, "report": nil})
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
	v := a.readState()
	explicit := v.CMSBase
	if explicit == "" {
		explicit = a.config.CMSBase
	}
	seen := map[string]bool{}
	for _, c := range candidates {
		if str(c["base"]) != "" {
			seen[str(c["base"])] = true
		}
	}
	if explicit != "" {
		if u, e := cmsEndpoint(explicit); e == nil && !seen[u.String()] {
			candidates = append(candidates, M{"id": "configured-address", "name": "已配置的 CMS", "root": "", "base": u.String(), "adapterInstalled": false, "report": nil})
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
			c["report"] = a.checkCMS(ctx, str(c["base"]))
		}(c)
	}
	wg.Wait()
	usable := map[string]M{}
	for _, c := range candidates {
		if obj(c["report"])["ok"] == true {
			usable[str(c["base"])] = c
		}
	}
	recommended, connected := "", false
	if len(usable) == 1 {
		for base, c := range usable {
			recommended = str(c["id"])
			if v.CMSBase == "" {
				err := a.change(parent, "cms.auto-connected", func(next *state) error {
					if next.CMSBase == "" {
						next.CMSBase = base
						next.UpdatedAt = now()
					}
					return nil
				})
				if err != nil {
					warnings = append(warnings, "探测通过，但保存连接失败："+err.Error())
				} else {
					connected = a.readState().CMSBase == base
				}
			}
		}
	}
	if len(candidates) == 0 {
		warnings = append(warnings, "未在同服务器 /www/wwwroot 下找到苹果 CMS；请先安装 CMS，或将站点目录加入 config.json 的 cmsRoots。")
	}
	return M{"candidates": candidates, "warnings": warnings, "recommendedId": recommended, "multiple": len(usable) > 1, "checkedAt": now(), "connected": connected, "connectedBase": a.readState().CMSBase}
}
func (a *app) autoConnectCMS() {
	if a.readState().CMSBase != "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a.discoverAndConnect(ctx)
}
