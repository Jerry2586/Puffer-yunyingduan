package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisConfig struct {
	Address    string `json:"address"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	DB         int    `json:"db"`
	PoolSize   int    `json:"poolSize"`
	MinIdle    int    `json:"minIdle"`
	MaxIdle    int    `json:"maxIdle"`
	TTLSeconds int    `json:"ttlSeconds"`
}

func (c *redisConfig) normalize() error {
	c.Address = strings.TrimSpace(c.Address)
	host, port, err := net.SplitHostPort(c.Address)
	n, pe := strconv.Atoi(port)
	if err != nil || pe != nil || host == "" || n < 1 || n > 65535 || strings.ContainsAny(host, "/\\ \t\r\n@") {
		return fmt.Errorf("Redis 地址需为主机:端口，例如 127.0.0.1:6379")
	}
	if c.DB < 0 || c.DB > 15 {
		return fmt.Errorf("Redis 数据库编号需为0至15")
	}
	if c.PoolSize == 0 {
		c.PoolSize = 100
	}
	if c.MaxIdle == 0 {
		c.MaxIdle = min(50, c.PoolSize)
	}
	if c.TTLSeconds == 0 {
		c.TTLSeconds = 60
	}
	if c.PoolSize < 1 || c.PoolSize > 500 || c.MinIdle < 0 || c.MinIdle > c.PoolSize || c.MaxIdle < c.MinIdle || c.MaxIdle > c.PoolSize || c.TTLSeconds < 5 || c.TTLSeconds > 3600 {
		return fmt.Errorf("Redis 连接池或缓存时长不合法")
	}
	return nil
}

type contentCache struct {
	client *redis.Client
	cfg    redisConfig
	prefix string
	hits   atomic.Uint64
	misses atomic.Uint64
	errors atomic.Uint64
}

func newContentCache(c *redisConfig, namespace string) *contentCache {
	if c == nil {
		return nil
	}
	digest := sha256.Sum256([]byte(namespace))
	return &contentCache{cfg: *c, prefix: "hetun:" + hex.EncodeToString(digest[:12]) + ":", client: redis.NewClient(&redis.Options{
		Addr: c.Address, Username: c.Username, Password: c.Password, DB: c.DB, Protocol: 2,
		PoolSize: c.PoolSize, MinIdleConns: c.MinIdle, MaxIdleConns: c.MaxIdle,
		DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, PoolTimeout: time.Second,
		MaxRetries: -1, ContextTimeoutEnabled: true, DisableIdentity: true,
	})}
}
func (c *contentCache) close() {
	if c != nil {
		_ = c.client.Close()
	}
}

// A unique, expiring test key verifies read/write permissions without modifying other applications' data.
func (c *contentCache) probe(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("请配置 Redis")
	}
	if err := c.client.Ping(ctx).Err(); err != nil {
		return err
	}
	key, value := c.prefix+"install:"+randomToken(), randomToken()
	if err := c.client.Set(ctx, key, value, 30*time.Second).Err(); err != nil {
		return err
	}
	got, err := c.client.Get(ctx, key).Result()
	cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	deleteErr := c.client.Del(cleanup, key).Err()
	if err != nil {
		return err
	}
	if got != value {
		return fmt.Errorf("Redis 读写校验失败")
	}
	return deleteErr
}
func (a *app) cachedCatalog(ctx context.Context, base string, params url.Values) (M, error) {
	c := a.cache
	if c == nil {
		return a.catalogRaw(ctx, base, params)
	}
	sum := sha256.Sum256([]byte(base + "\n" + params.Encode()))
	key := c.prefix + "catalog:v1:" + hex.EncodeToString(sum[:])
	cached, err := c.client.Get(ctx, key).Bytes()
	if err == nil {
		var result M
		if json.Unmarshal(cached, &result) == nil && result != nil {
			c.hits.Add(1)
			return result, nil
		}
	}
	if err != nil && err != redis.Nil {
		c.errors.Add(1)
	}
	c.misses.Add(1)
	result, err := a.catalogRaw(ctx, base, params)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(result)
	if err == nil && len(data) <= 2*1024*1024 {
		if e := c.client.Set(ctx, key, data, time.Duration(c.cfg.TTLSeconds)*time.Second).Err(); e != nil {
			c.errors.Add(1)
		}
	}
	return result, nil
}
func (c *contentCache) overview(ctx context.Context) M {
	if c == nil {
		return M{"configured": false, "ok": false, "message": "旧安装尚未配置 Redis"}
	}
	started := time.Now()
	result := M{"configured": true, "ok": false, "address": c.cfg.Address, "username": c.cfg.Username, "db": c.cfg.DB, "ttlSeconds": c.cfg.TTLSeconds, "passwordConfigured": c.cfg.Password != "", "cacheHits": c.hits.Load(), "cacheMisses": c.misses.Load(), "cacheErrors": c.errors.Load()}
	err := c.client.Ping(ctx).Err()
	result["ok"] = err == nil
	result["latencyMs"] = time.Since(started).Milliseconds()
	result["message"] = "Redis 连接正常"
	if err != nil {
		result["message"] = "Redis 连接失败，请检查服务、地址及认证配置"
	} else {
		info, e := c.client.Info(ctx, "server", "memory", "clients").Result()
		result["infoAvailable"] = e == nil
		if e == nil {
			for _, line := range strings.Split(info, "\n") {
				k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
				if !ok {
					continue
				}
				switch k {
				case "redis_version":
					result["version"] = v
				case "used_memory", "used_memory_peak", "connected_clients":
					n, e := strconv.ParseInt(v, 10, 64)
					if e == nil {
						result[k] = n
					}
				}
			}
		}
	}
	stats := c.client.PoolStats()
	result["pool"] = M{"total": stats.TotalConns, "idle": stats.IdleConns, "max": c.cfg.PoolSize, "minIdle": c.cfg.MinIdle, "maxIdle": c.cfg.MaxIdle, "timeouts": stats.Timeouts, "hits": stats.Hits, "misses": stats.Misses}
	return result
}
