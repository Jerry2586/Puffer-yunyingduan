package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const updateRepo = "Jerry2586/Puffer-yunyingduan"
const releaseAPI = "https://api.github.com/repos/" + updateRepo + "/releases/latest"
const maxUpdateBytes int64 = 128 << 20

type releaseAsset struct {
	Name   string
	URL    string `json:"browser_download_url"`
	Digest string
	Size   int64
}
type githubRelease struct {
	Tag        string `json:"tag_name"`
	URL        string `json:"html_url"`
	Notes      string `json:"body"`
	Published  string `json:"published_at"`
	Draft      bool
	Prerelease bool
	Assets     []releaseAsset
}
type updateRelease struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	Notes     string `json:"notes"`
	Published string `json:"publishedAt"`
	Asset     string `json:"asset"`
	Size      int64  `json:"size"`
}
type updateView struct {
	Current    string         `json:"current"`
	Platform   string         `json:"platform"`
	Repository string         `json:"repository"`
	Phase      string         `json:"phase"`
	Message    string         `json:"message"`
	CheckedAt  string         `json:"checkedAt"`
	Latest     *updateRelease `json:"latest"`
	Available  bool           `json:"available"`
	CanInstall bool           `json:"canInstall"`
	Reason     string         `json:"reason"`
}
type preparedUpdate struct{ target, candidate, backup string }

func (p *preparedUpdate) Error() string { return "系统更新已准备" }

type systemUpdater struct {
	mu                                 sync.Mutex
	view                               updateView
	client                             *http.Client
	executable, configPath, goos, arch string
	restart                            chan *preparedUpdate
	preflight                          func(context.Context, string, string, string) error
}

func updateURLAllowed(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	}
	return false
}
func newSystemUpdater(executable, configPath string) *systemUpdater {
	u := &systemUpdater{executable: executable, configPath: configPath, goos: runtime.GOOS, arch: runtime.GOARCH, restart: make(chan *preparedUpdate, 1), preflight: preflightUpdate}
	u.client = &http.Client{Timeout: 8 * time.Minute, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ResponseHeaderTimeout: 20 * time.Second, TLSHandshakeTimeout: 10 * time.Second}, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || !updateURLAllowed(r.URL.String()) {
			return fmt.Errorf("更新下载重定向地址不受信任")
		}
		return nil
	}}
	u.view = updateView{Current: buildVersion, Platform: u.goos + "/" + u.arch, Repository: updateRepo, Phase: "idle", Message: "点击检查更新，从 GitHub 获取正式版本"}
	return u
}
func (u *systemUpdater) snapshot() updateView {
	u.mu.Lock()
	defer u.mu.Unlock()
	v := u.view
	if v.Latest != nil {
		copy := *v.Latest
		v.Latest = &copy
	}
	v.CanInstall = false
	switch {
	case u.goos != "linux":
		v.Reason = "当前系统仅支持检查版本；面板安装更新适用于 Linux 服务器"
	case u.arch != "amd64" && u.arch != "arm64":
		v.Reason = "尚未发布当前处理器架构的更新包"
	case u.executable == "" || u.configPath == "":
		v.Reason = "无法确定程序或配置文件位置"
	default:
		v.CanInstall = true
	}
	return v
}
func updateBusy(p string) bool { return p != "idle" && p != "checked" && p != "failed" }
func (u *systemUpdater) phase(phase, message string) {
	u.mu.Lock()
	u.view.Phase = phase
	u.view.Message = message
	u.mu.Unlock()
}
func (u *systemUpdater) get(ctx context.Context, raw string, max int64) ([]byte, error) {
	if !updateURLAllowed(raw) {
		return nil, fmt.Errorf("更新地址不受信任")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Hetun-Operations/"+buildVersion)
	resp, e := u.client.Do(req)
	if e != nil {
		return nil, fmt.Errorf("连接 GitHub 失败: %w", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub 返回 HTTP %d（请检查 Release 是否公开及访问额度）", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("更新文件超过大小限制")
	}
	return b, nil
}
func (u *systemUpdater) release(ctx context.Context) (updateRelease, releaseAsset, githubRelease, error) {
	var g githubRelease
	b, e := u.get(ctx, releaseAPI, 2<<20)
	if e != nil {
		return updateRelease{}, releaseAsset{}, g, e
	}
	if e = json.Unmarshal(b, &g); e != nil {
		return updateRelease{}, releaseAsset{}, g, fmt.Errorf("GitHub 版本响应无效")
	}
	v := strings.TrimPrefix(g.Tag, "v")
	if !versionPattern.MatchString(v) || g.Draft || g.Prerelease || g.URL != "https://github.com/"+updateRepo+"/releases/tag/"+g.Tag {
		return updateRelease{}, releaseAsset{}, g, fmt.Errorf("不是有效的正式版本")
	}
	name := "Hetun-Operations-" + v + "-" + u.goos + "-" + u.arch + ".tar.gz"
	r := updateRelease{Version: v, URL: g.URL, Notes: g.Notes, Published: g.Published}
	for _, asset := range g.Assets {
		if asset.Name == name {
			expected := "https://github.com/" + updateRepo + "/releases/download/" + g.Tag + "/" + name
			if asset.URL != expected || asset.Size <= 0 || asset.Size > maxUpdateBytes {
				return r, releaseAsset{}, g, fmt.Errorf("更新包地址或大小无效")
			}
			r.Asset = name
			r.Size = asset.Size
			return r, asset, g, nil
		}
	}
	return r, releaseAsset{}, g, nil
}
func (u *systemUpdater) check(ctx context.Context) error {
	u.mu.Lock()
	if updateBusy(u.view.Phase) {
		u.mu.Unlock()
		return problem(409, "已有检查或更新任务正在进行")
	}
	u.view.Phase = "checking"
	u.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	r, _, _, e := u.release(ctx)
	u.mu.Lock()
	defer u.mu.Unlock()
	u.view.CheckedAt = now()
	if e != nil {
		u.view.Phase = "failed"
		u.view.Latest = nil
		u.view.Available = false
		u.view.Message = e.Error()
		return nil
	}
	u.view.Latest = &r
	u.view.Available = newer(r.Version, buildVersion)
	u.view.Phase = "checked"
	u.view.Message = "当前已是最新正式版本"
	if u.view.Available {
		u.view.Message = "发现可安装的新版本"
	}
	if newer(buildVersion, r.Version) {
		u.view.Message = "当前运行版本高于最新正式版本"
	}
	if r.Asset == "" {
		u.view.Message = "正式版本尚未附带当前平台的安装包"
	}
	return nil
}
func (u *systemUpdater) start(version string) error {
	s := u.snapshot()
	if !s.CanInstall {
		return problem(409, s.Reason)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if updateBusy(u.view.Phase) {
		return problem(409, "已有更新任务正在进行")
	}
	if u.view.Latest == nil || version != u.view.Latest.Version || !u.view.Available || u.view.Latest.Asset == "" {
		return problem(409, "请先检查更新并选择可用的新版本")
	}
	u.view.Phase = "queued"
	u.view.Message = "更新已开始，请保持页面打开"
	go u.install(version)
	return nil
}
func validSHA(s string) bool { b, e := hex.DecodeString(s); return e == nil && len(b) == sha256.Size }
func (u *systemUpdater) checksum(ctx context.Context, a releaseAsset, g githubRelease) (string, error) {
	if strings.HasPrefix(a.Digest, "sha256:") {
		s := strings.TrimPrefix(a.Digest, "sha256:")
		if validSHA(s) {
			return strings.ToLower(s), nil
		}
		return "", fmt.Errorf("发布的 SHA256 格式错误")
	}
	for _, f := range g.Assets {
		if f.Name == "SHA256SUMS.txt" {
			expected := "https://github.com/" + updateRepo + "/releases/download/" + g.Tag + "/SHA256SUMS.txt"
			if f.URL != expected {
				return "", fmt.Errorf("校验文件地址无效")
			}
			b, e := u.get(ctx, f.URL, 64<<10)
			if e != nil {
				return "", e
			}
			for _, line := range strings.Split(string(b), "\n") {
				p := strings.Fields(line)
				if len(p) == 2 && strings.TrimPrefix(p[1], "*") == a.Name && validSHA(p[0]) {
					return strings.ToLower(p[0]), nil
				}
			}
		}
	}
	return "", fmt.Errorf("发布缺少此安装包的 SHA256 校验值")
}
func unpackUpdate(archive []byte, directory string) (string, error) {
	z, e := gzip.NewReader(bytes.NewReader(archive))
	if e != nil {
		return "", fmt.Errorf("更新包不是有效 gzip")
	}
	defer z.Close()
	tr := tar.NewReader(z)
	h, e := tr.Next()
	if e != nil {
		return "", fmt.Errorf("更新包为空或损坏")
	}
	if h.Name != "hetun-operations" || h.Typeflag != tar.TypeReg || h.Size <= 0 || h.Size > maxUpdateBytes {
		return "", fmt.Errorf("更新包必须只含 hetun-operations 普通可执行文件")
	}
	f, e := os.CreateTemp(directory, ".hetun-update-*")
	if e != nil {
		return "", fmt.Errorf("程序目录不可写: %w", e)
	}
	name := f.Name()
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	if _, e = io.CopyN(f, tr, h.Size); e != nil {
		return "", e
	}
	if _, e = tr.Next(); e != io.EOF {
		return "", fmt.Errorf("更新包包含额外文件或损坏")
	}
	// Consume gzip footer so its CRC is checked before the executable is accepted.
	if _, e = io.Copy(io.Discard, io.LimitReader(z, maxUpdateBytes+1)); e != nil {
		return "", e
	}
	if e = f.Chmod(0700); e != nil {
		return "", e
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	ok = true
	return name, nil
}
func preflightUpdate(ctx context.Context, candidate, configPath, version string) error {
	run := func(args ...string) (string, error) {
		c, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		b, e := exec.CommandContext(c, candidate, args...).Output()
		return strings.TrimSpace(string(b)), e
	}
	output, e := run("--version")
	if e != nil || output != "hetun-operations "+version {
		return fmt.Errorf("候选程序版本预检失败")
	}
	if _, e = run("--check-config", "--config", configPath); e != nil {
		return fmt.Errorf("新版本无法读取现有配置，已取消更新")
	}
	return nil
}
func (u *systemUpdater) install(version string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var candidate string
	fail := func(e error) {
		if candidate != "" {
			os.Remove(candidate)
		}
		u.phase("failed", e.Error())
	}
	u.phase("checking", "重新核对 GitHub 正式版本")
	r, a, g, e := u.release(ctx)
	if e != nil {
		fail(e)
		return
	}
	if r.Version != version || !newer(r.Version, buildVersion) || a.URL == "" {
		fail(fmt.Errorf("正式版本已变化，请重新检查更新"))
		return
	}
	sum, e := u.checksum(ctx, a, g)
	if e != nil {
		fail(e)
		return
	}
	u.phase("downloading", "正在下载服务器对应的安装包")
	b, e := u.get(ctx, a.URL, maxUpdateBytes)
	if e != nil {
		fail(e)
		return
	}
	if int64(len(b)) != a.Size {
		fail(fmt.Errorf("安装包大小与发布信息不符"))
		return
	}
	u.phase("verifying", "正在校验 SHA256 和更新包结构")
	hash := sha256.Sum256(b)
	if hex.EncodeToString(hash[:]) != sum {
		fail(fmt.Errorf("SHA256 校验失败，更新已取消"))
		return
	}
	candidate, e = unpackUpdate(b, filepath.Dir(u.executable))
	if e != nil {
		fail(e)
		return
	}
	u.phase("preflight", "正在检查新程序版本及现有配置兼容性")
	if e = u.preflight(ctx, candidate, u.configPath, version); e != nil {
		fail(e)
		return
	}
	u.phase("restarting", "校验通过，正在重启服务；随后需要重新登录")
	select {
	case u.restart <- &preparedUpdate{target: u.executable, candidate: candidate, backup: u.executable + ".previous"}:
	case <-ctx.Done():
		fail(ctx.Err())
	}
}
func (p *preparedUpdate) swap() error {
	// Previous backup must be a regular file; never follow a link or replace a directory.
	for _, name := range []string{p.target, p.candidate, p.backup} {
		st, e := os.Lstat(name)
		if errors.Is(e, os.ErrNotExist) && name == p.backup {
			continue
		}
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("程序路径不是普通文件")
		}
	}
	if e := os.Remove(p.backup); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	// Hard-link the current inode before atomic rename, leaving target present throughout.
	if e := os.Link(p.target, p.backup); e != nil {
		return e
	}
	if e := os.Rename(p.candidate, p.target); e != nil {
		return e
	}
	return nil
}
func (a *app) adminUpdate(w http.ResponseWriter, r *http.Request) error {
	if a.updater == nil {
		return problem(503, "系统更新组件尚未初始化")
	}
	switch r.URL.Path {
	case "/api/admin/system/update":
		if r.Method != "GET" {
			return problem(405, "不支持此方法")
		}
	case "/api/admin/system/update/check":
		if r.Method != "POST" {
			return problem(405, "不支持此方法")
		}
		if e := a.updater.check(r.Context()); e != nil {
			return e
		}
	case "/api/admin/system/update/install":
		if r.Method != "POST" {
			return problem(405, "不支持此方法")
		}
		m, e := readBody(w, r, 1024)
		if e != nil {
			return e
		}
		if len(m) != 1 {
			return problem(400, "仅允许指定已检查的 version")
		}
		if e = a.updater.start(str(m["version"])); e != nil {
			return e
		}
	default:
		return problem(404, "没有找到这个接口")
	}
	jsonResponse(w, 200, a.updater.snapshot())
	return nil
}
