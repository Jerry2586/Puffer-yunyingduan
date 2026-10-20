package main

import (
	"context"
	"encoding/json"
	"github.com/redis/go-redis/v9"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Each platform owns its generation. Replacing it invalidates even in-flight
// responses written with an older generation; old keys retain a bounded TTL.
func (c *contentCache) clientCacheStatus(ctx context.Context, p string) (M, error) {
	out := M{"platform": p, "configured": c != nil, "available": false, "checkedAt": now(), "entries": 0, "countComplete": true, "ttlSeconds": 0}
	if c == nil {
		out["message"] = "未配置 Redis，影片数据直接读取内容源"
		return out, nil
	}
	if err := c.client.Ping(ctx).Err(); err != nil {
		out["message"] = "Redis 无法连接，请检查服务配置"
		return out, nil
	}
	generation, err := c.client.Get(ctx, c.prefix+"generation:"+p).Result()
	if err == redis.Nil {
		generation = "0"
	} else if err != nil {
		return nil, problem(503, "无法读取缓存状态")
	}
	keys, complete, err := c.cacheKeys(ctx, p, generation)
	if err != nil {
		return nil, problem(503, "无法读取缓存条目，请检查 Redis 权限和连接")
	}
	kinds := M{}
	for _, kind := range []string{"config", "users", "catalog"} {
		gen, e := c.generation(ctx, p, kind)
		if e != nil {
			return nil, problem(503, "无法读取缓存状态")
		}
		rows, all, e := c.kindKeys(ctx, p, kind, gen)
		if e != nil {
			return nil, problem(503, "无法读取缓存条目")
		}
		kinds[kind] = M{"entries": len(rows), "countComplete": all, "ttlSeconds": c.kindTTL(kind)}
	}
	out["kinds"] = kinds
	out["available"] = true
	out["entries"] = len(keys)
	out["countComplete"] = complete
	out["ttlSeconds"] = c.cfg.TTLSeconds
	out["message"] = "当前客户端缓存可用"
	return out, nil
}
func (c *contentCache) cacheKeys(ctx context.Context, p, generation string) ([]string, bool, error) {
	return c.kindKeys(ctx, p, "catalog", generation)
}
func (c *contentCache) kindKeys(ctx context.Context, p, kind, generation string) ([]string, bool, error) {
	keys := []string{}
	seen := map[string]bool{}
	var cursor uint64
	for scans := 0; scans < 100; scans++ {
		batch, next, err := c.client.Scan(ctx, cursor, c.dataPrefix(p, kind, generation)+"*", 100).Result()
		if err != nil {
			return nil, false, err
		}
		for _, key := range batch {
			if !seen[key] {
				keys = append(keys, key)
				seen[key] = true
			}
		}
		if next == 0 {
			return keys, true, nil
		}
		if len(keys) >= 5000 {
			return keys, false, nil
		}
		cursor = next
	}
	return keys, false, nil
}
func (a *app) adminClientCache(w http.ResponseWriter, r *http.Request) error {
	if r.URL.Query().Get("platform") == "" {
		return problem(400, "请明确选择客户端")
	}
	p, err := requestPlatform(r)
	if err != nil {
		return err
	}
	if r.URL.Path == "/api/admin/client-cache" {
		if r.Method != "GET" {
			return problem(405, "不支持此方法")
		}
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		out, err := a.cache.clientCacheStatus(ctx, p)
		if err != nil {
			return err
		}
		out["search"] = a.searchStatus(p)
		jsonResponse(w, 200, out)
		return nil
	}
	if r.Method != "POST" {
		return problem(405, "不支持此方法")
	}
	input, err := readBody(w, r, 4096)
	if err != nil {
		return err
	}
	kind := str(input["kind"])
	if !has([]string{"config", "users", "catalog"}, kind) {
		return problem(400, "不支持的缓存类型")
	}
	if a.cache == nil {
		return coded(409, "未配置 Redis，当前没有可清理的服务端缓存", "CACHE_NOT_CONFIGURED")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	c := a.cache
	generation := randomToken()
	previous, err := c.client.GetSet(ctx, c.generationKey(p, kind), generation).Result()
	if err == redis.Nil {
		previous = "0"
		err = nil
	}
	if err != nil {
		return problem(503, "缓存清理未确认，请检查 Redis 后刷新状态并重试")
	}
	keys, complete, scanErr := c.kindKeys(ctx, p, kind, previous)
	deleted := int64(0)
	cleanupOK := scanErr == nil
	if cleanupOK {
		for start := 0; start < len(keys); start += 100 {
			n, e := c.client.Del(ctx, keys[start:min(start+100, len(keys))]...).Result()
			deleted += n
			if e != nil {
				cleanupOK = false
				break
			}
		}
	}
	event, _ := json.Marshal(M{"at": now(), "event": "client.cache.clear", "platform": p, "kind": kind, "generation": generation, "deleted": deleted})
	f, e := os.OpenFile(filepath.Join(a.config.DataDir, "audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	auditOK := e == nil
	if e == nil {
		_, e = f.Write(append(event, '\n'))
		closeErr := f.Close()
		auditOK = e == nil && closeErr == nil
	}
	jsonResponse(w, 200, M{"ok": true, "platform": p, "kind": kind, "invalidated": true, "deleted": deleted, "cleanupComplete": cleanupOK && complete, "auditRecorded": auditOK, "clearedAt": now()})
	return nil
}
