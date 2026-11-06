package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// A binding is checked against the published resolver list before either side
// is changed. Deleting a bound resolver requires removing its bindings first.
func validatePlaybackBindings(p playbackPreferences, c playbackConfig) error {
	known := map[string]bool{}
	for _, s := range c.Sources {
		known[s.ID] = true
	}
	for _, rule := range p.Sources {
		for _, id := range rule.ResolverIDs {
			if !known[id] {
				return problem(400, "播放源 "+rule.Name+" 绑定的解析器不存在；请先发布解析器，删除时先解除绑定")
			}
		}
	}
	return nil
}

func playbackConfigForSource(v state, code string) playbackConfig {
	c := clone(playbackState(v).Published)
	rule := playbackSourceFor(v, code)
	if rule == nil {
		return c
	}
	if rule.Mode == "resolve" {
		c.DirectFirst = false
	}
	if len(rule.ResolverIDs) > 0 {
		available := c.Sources
		c.Sources = []playbackSource{}
		for _, id := range rule.ResolverIDs {
			for _, resolver := range available {
				if resolver.ID == id {
					c.Sources = append(c.Sources, resolver)
					break
				}
			}
		}
	}
	if rule.Mode == "resolve" && !c.Enabled {
		c.Enabled = true
		c.Sources = []playbackSource{}
	}
	return c
}

func (a *app) discoverPlaybackSources(ctx context.Context, v state) (M, error) {
	if v.CMSLocalRoot != "" {
		if items, err := a.localPlaybackSources(v.CMSLocalRoot); err == nil {
			return M{"items": items, "discovery": "local"}, nil
		}
	}
	connection, err := a.managementConnectionFor(v)
	if err != nil {
		return nil, coded(409, "未能读取 CMS 播放器规则。请确认同机站点配置可读，或连接 CMS 管理适配器后重试", "CMS_PLAYBACK_DISCOVERY_UNAVAILABLE")
	}
	return a.managementCall(ctx, connection, "playback.sources", M{})
}

// Read the static player array only; never execute PHP or expose parse URLs.
func (a *app) localPlaybackSources(root string) ([]any, error) {
	bad := func() ([]any, error) {
		return nil, problem(400, "CMS 播放器配置不可读或不是受支持的静态数组")
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return bad()
	}
	allowed := false
	for _, boundary := range a.cmsRoots() {
		b, e := filepath.EvalSymlinks(boundary)
		if e == nil && withinDirectory(b, real) {
			allowed = true
			break
		}
	}
	if !allowed {
		return bad()
	}
	file, err := filepath.EvalSymlinks(filepath.Join(real, "application", "extra", "vodplayer.php"))
	if err != nil || !withinDirectory(real, file) {
		return bad()
	}
	f, err := os.Open(file)
	if err != nil {
		return bad()
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return bad()
	}
	data, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return bad()
	}
	players, err := parseCMSDatabase(data)
	if err != nil {
		return bad()
	}
	keys := make([]string, 0, len(players))
	for key := range players {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		ai, bj := numericOr(obj(players[keys[i]])["sort"], 0), numericOr(obj(players[keys[j]])["sort"], 0)
		if ai == bj {
			return keys[i] < keys[j]
		}
		return ai < bj
	})
	items := []any{}
	seen := map[string]bool{}
	for _, key := range keys {
		p := obj(players[key])
		code := str(p["from"])
		if code == "" {
			code = key
		}
		if !resolverID.MatchString(code) || seen[code] {
			continue
		}
		seen[code] = true
		name := strings.TrimSpace(str(p["show"]))
		if name == "" {
			name = code
		}
		if utf8.RuneCountInString(name) > 60 {
			name = string([]rune(name)[:60])
		}
		items = append(items, M{"code": code, "name": name, "enabled": numericOr(p["status"], 0) == 1})
		if len(items) == 100 {
			break
		}
	}
	return items, nil
}
