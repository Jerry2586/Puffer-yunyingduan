package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

func (c *contentCache) generationKey(p, kind string) string {
	if kind == "catalog" {
		return c.prefix + "generation:" + p
	}
	return c.prefix + "generation:" + kind + ":" + p
}
func (c *contentCache) generation(ctx context.Context, p, kind string) (string, error) {
	v, e := c.client.Get(ctx, c.generationKey(p, kind)).Result()
	if e == redis.Nil {
		return "0", nil
	}
	return v, e
}
func (c *contentCache) dataPrefix(p, kind, gen string) string {
	return c.prefix + kind + ":v2:" + p + ":" + gen + ":"
}
func (c *contentCache) kindTTL(kind string) int {
	if kind == "users" {
		return min(c.cfg.TTLSeconds, 30)
	}
	return c.cfg.TTLSeconds
}

// Callers must authenticate before using private cached data. Source hashes and
// independently rotated generations prevent old in-flight responses resurfacing.
// Redis failure falls back to the real source, never to a fabricated result.
func (a *app) cacheObject(ctx context.Context, p, kind, source string, load func() (M, error)) (M, error) {
	c := a.cache
	if c == nil {
		return load()
	}
	gen, e := c.generation(ctx, p, kind)
	if e != nil {
		c.errors.Add(1)
		return load()
	}
	hash := sha256.Sum256([]byte(source))
	key := c.dataPrefix(p, kind, gen) + hex.EncodeToString(hash[:])
	if data, err := c.client.Get(ctx, key).Bytes(); err == nil {
		var result M
		if json.Unmarshal(data, &result) == nil && result != nil {
			c.hits.Add(1)
			return result, nil
		}
	} else if err != redis.Nil {
		c.errors.Add(1)
	}
	c.misses.Add(1)
	result, err := load()
	if err != nil {
		return nil, err
	}
	if data, err := json.Marshal(result); err == nil && len(data) <= 2*1024*1024 {
		if c.client.Set(ctx, key, data, time.Duration(c.kindTTL(kind))*time.Second).Err() != nil {
			c.errors.Add(1)
		}
	}
	return result, nil
}
func (a *app) cachedBootstrap(ctx context.Context, v state, p, os, version, origin string) (M, error) {
	// Include the published document and releases, so a publish/rollback/URL change
	// is immediately visible without clearing Redis. Drafts are never served.
	r := v.Operations[p]
	source, err := json.Marshal(M{"published": r.Published, "version": r.PublishedVersion, "at": r.PublishedAt, "release": latestAppRelease(v, p, os), "managed": v.ManagedReleaseTargets, "cms": v.CMSBase, "os": os, "clientVersion": version, "origin": origin})
	if err != nil {
		return nil, err
	}
	return a.cacheObject(ctx, p, "config", string(source), func() (M, error) { return bootstrap(v, p, os, version, origin) })
}
func (a *app) cachedProfile(ctx context.Context, s *memberSession, p, token string) (M, error) {
	// A successful business write advances the session revision across all four
	// clients. Clearing this display cache never revokes a member login session.
	source := fmt.Sprintf("%s\n%s\n%d", s.Base, token, s.ProfileVersion.Load())
	return a.cacheObject(ctx, p, "users", source, func() (M, error) { return a.profile(ctx, s) })
}
