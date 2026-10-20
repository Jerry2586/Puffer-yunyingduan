package main

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type appRelease struct {
	ID          string  `json:"id"`
	Platform    string  `json:"platform"`
	OS          string  `json:"os"`
	Version     string  `json:"version"`
	SizeMB      float64 `json:"sizeMB"`
	DownloadURL string  `json:"downloadUrl"`
	BrowserURL  string  `json:"browserUrl"`
	Notes       string  `json:"notes"`
	Force       bool    `json:"force"`
	Enabled     bool    `json:"enabled"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
	Revision    int     `json:"revision"`
	Legacy      bool    `json:"legacy,omitempty"`
}

func releaseTarget(p, os string) bool {
	return (has([]string{"mobile", "tablet"}, p) && has([]string{"android", "ios"}, os)) || (p == "tv" && os == "android") || (p == "web" && has([]string{"windows", "web"}, os))
}
func releaseHTTPS(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && len(raw) <= 2048
}
func canonicalReleaseVersion(s string) string {
	if !versionPattern.MatchString(s) {
		return ""
	}
	parts := strings.Split(s, ".")
	for i, p := range parts {
		n, _ := strconv.Atoi(p)
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}
func latestAppRelease(v state, p, os string) *appRelease {
	var latest *appRelease
	for _, row := range v.AppReleases {
		if row.Platform != p || row.OS != os || !row.Enabled {
			continue
		}
		if latest == nil || newer(row.Version, latest.Version) {
			r := row
			latest = &r
		}
	}
	return latest
}

// Import actual published legacy update settings once; the next durable mutation saves them.
func migrateAppReleases(v *state) {
	if v.ManagedReleaseTargets == nil {
		v.ManagedReleaseTargets = map[string]bool{}
	}
	for _, p := range platforms {
		rec := v.Operations[p]
		if rec == nil {
			continue
		}
		settings := obj(rec.Published["clientSettings"])
		ver := canonicalReleaseVersion(str(settings["latestVersion"]))
		if ver == "" {
			continue
		}
		for _, os := range []string{"android", "ios"} {
			target := p + "/" + os
			key := "androidUrl"
			if os == "ios" {
				key = "iosUrl"
			}
			raw := str(settings[key])
			if !releaseTarget(p, os) || v.ManagedReleaseTargets[target] || !releaseHTTPS(raw) {
				continue
			}
			v.AppReleases = append(v.AppReleases, appRelease{ID: "legacy-" + p + "-" + os, Platform: p, OS: os, Version: ver, DownloadURL: raw, Notes: str(settings["updateNotes"]), Force: settings["forceUpdate"] == true, Enabled: true, CreatedAt: rec.PublishedAt, UpdatedAt: rec.PublishedAt, Revision: 1, Legacy: true})
			v.ManagedReleaseTargets[target] = true
		}
	}
}
func applyAppRelease(v state, p, os string, settings M) {
	if !v.ManagedReleaseTargets[p+"/"+os] {
		return
	}
	settings["latestVersion"] = ""
	settings["updateNotes"] = ""
	settings["forceUpdate"] = false
	settings["notifyOnLaunch"] = false
	settings["androidUrl"] = ""
	settings["iosUrl"] = ""
	settings["downloadUrl"] = ""
	settings["browserUrl"] = ""
	settings["sizeMB"] = 0
	settings["releaseId"] = ""
	if r := latestAppRelease(v, p, os); r != nil {
		settings["latestVersion"] = r.Version
		settings["updateNotes"] = r.Notes
		settings["forceUpdate"] = r.Force
		settings["notifyOnLaunch"] = true
		key := "downloadUrl"
		if os == "android" {
			key = "androidUrl"
		}
		if os == "ios" {
			key = "iosUrl"
		}
		settings[key] = r.DownloadURL
		settings["browserUrl"] = r.BrowserURL
		settings["sizeMB"] = r.SizeMB
		settings["releaseId"] = r.ID
	}
}
func releaseList(v state) M {
	rows := append([]appRelease{}, v.AppReleases...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt > rows[j].CreatedAt })
	current := []string{}
	for _, p := range platforms {
		for _, os := range []string{"android", "ios", "windows", "web"} {
			if r := latestAppRelease(v, p, os); r != nil {
				current = append(current, r.ID)
			}
		}
	}
	return M{"items": rows, "currentIds": current, "total": len(rows)}
}
func saveAppRelease(v *state, in M, create bool) error {
	r := appRelease{ID: str(in["id"]), Platform: str(in["platform"]), OS: str(in["os"]), Version: canonicalReleaseVersion(str(in["version"])), DownloadURL: strings.TrimSpace(str(in["downloadUrl"])), BrowserURL: strings.TrimSpace(str(in["browserUrl"])), Notes: strings.TrimSpace(str(in["notes"]))}
	if !releaseTarget(r.Platform, r.OS) {
		return problem(400, "请选择有效的设备平台与系统")
	}
	if r.Version == "" {
		return problem(400, "版本号必须为三段数字，每段最多9位")
	}
	size, ok := in["sizeMB"].(float64)
	if !ok || math.IsNaN(size) || math.IsInf(size, 0) || size <= 0 || size > 1048576 {
		return problem(400, "请输入有效的安装包大小（MB），必须大于0")
	}
	r.SizeMB = size
	if !releaseHTTPS(r.DownloadURL) || (r.BrowserURL != "" && !releaseHTTPS(r.BrowserURL)) {
		return problem(400, "下载地址必须是完整的 HTTPS 地址")
	}
	if utf8.RuneCountInString(r.Notes) < 1 || utf8.RuneCountInString(r.Notes) > 500 {
		return problem(400, "更新说明需填写1至500字")
	}
	enabled, ok := in["enabled"].(bool)
	if !ok {
		return problem(400, "缺少版本发布状态")
	}
	r.Enabled = enabled
	force, ok := in["force"].(bool)
	if !ok {
		return problem(400, "缺少强制更新设置")
	}
	r.Force = force
	index := -1
	for i, other := range v.AppReleases {
		if other.ID == r.ID {
			index = i
		}
		if other.Platform == r.Platform && other.OS == r.OS && other.Version == r.Version && (create || other.ID != r.ID) {
			return problem(409, "此平台的相同版本号已存在，请编辑已有版本")
		}
	}
	if create {
		if len(v.AppReleases) >= 2000 {
			return problem(400, "最多保存2000条版本，请清理历史记录")
		}
		r.ID = randomToken()[:16]
		r.CreatedAt = now()
		r.Revision = 1
	} else {
		if index < 0 {
			return problem(404, "版本记录不存在")
		}
		prev := v.AppReleases[index]
		if number(in["revision"]) != prev.Revision {
			return problem(409, "版本已被修改，请刷新列表后重试")
		}
		if r.Platform != prev.Platform || r.OS != prev.OS {
			return problem(400, "已有版本不能更换平台，请新增版本")
		}
		r.CreatedAt = prev.CreatedAt
		r.Revision = prev.Revision + 1
	}
	r.UpdatedAt = now()
	if create {
		v.AppReleases = append(v.AppReleases, r)
	} else {
		v.AppReleases[index] = r
	}
	if v.ManagedReleaseTargets == nil {
		v.ManagedReleaseTargets = map[string]bool{}
	}
	v.ManagedReleaseTargets[r.Platform+"/"+r.OS] = true
	touchReleasePlatform(v, r.Platform)
	v.UpdatedAt = now()
	return nil
}
func deleteAppRelease(v *state, in M) error {
	for i, r := range v.AppReleases {
		if r.ID == str(in["id"]) {
			if number(in["revision"]) != r.Revision {
				return problem(409, "版本已被修改，请刷新列表后重试")
			}
			v.AppReleases = append(v.AppReleases[:i], v.AppReleases[i+1:]...)
			touchReleasePlatform(v, r.Platform)
			v.UpdatedAt = now()
			return nil
		}
	}
	return problem(404, "版本记录不存在")
}
func scopedReleaseList(v state, p string) M {
	if p != "" {
		rows := []appRelease{}
		for _, row := range v.AppReleases {
			if row.Platform == p {
				rows = append(rows, row)
			}
		}
		v.AppReleases = rows
	}
	return releaseList(v)
}
func scopeReleaseInput(v state, input M, p string, create bool) error {
	if p == "" {
		return nil
	}
	if raw, exists := input["platform"]; exists {
		value, ok := raw.(string)
		candidate, err := platform(value)
		if !ok || value == "" || err != nil || candidate != p {
			return problem(400, "版本平台与客户端路径不一致")
		}
	}
	input["platform"] = p
	if !create {
		for _, row := range v.AppReleases {
			if row.ID == str(input["id"]) {
				if row.Platform != p {
					return problem(404, "此客户端中不存在该版本")
				}
				return nil
			}
		}
		return problem(404, "版本记录不存在")
	}
	return nil
}
func (a *app) adminReleases(w http.ResponseWriter, r *http.Request) error {
	p := clientScope(r)
	if r.URL.Path == "/api/admin/releases" && r.Method == "GET" {
		jsonResponse(w, 200, scopedReleaseList(a.readState(), p))
		return nil
	}
	deleting := r.URL.Path == "/api/admin/releases/delete" && r.Method == "POST"
	if !deleting && !(r.URL.Path == "/api/admin/releases" && has([]string{"POST", "PUT"}, r.Method)) {
		return problem(405, "请求方法不支持")
	}
	input, e := readBody(w, r, 16*1024)
	if e != nil {
		return e
	}
	action := "saved"
	if deleting {
		action = "deleted"
	}
	e = a.change(r.Context(), fmt.Sprintf("releases.%s", action), func(next *state) error {
		if err := scopeReleaseInput(*next, input, p, !deleting && r.Method == "POST"); err != nil {
			return err
		}
		if deleting {
			return deleteAppRelease(next, input)
		}
		return saveAppRelease(next, input, r.Method == "POST")
	})
	if e != nil {
		return e
	}
	jsonResponse(w, 200, scopedReleaseList(a.readState(), p))
	return nil
}
func touchReleasePlatform(v *state, p string) {
	if r := v.Operations[p]; r != nil {
		r.PublishedVersion++
		r.PublishedAt = now()
	}
}
