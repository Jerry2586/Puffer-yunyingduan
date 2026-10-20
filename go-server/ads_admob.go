package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Official AdMob SSV: preserve signed query bytes; SHA256/ECDSA ASN.1 DER.
// https://developers.google.com/admob/android/ssv
var adKeys = struct {
	sync.Mutex
	keys      map[string]*ecdsa.PublicKey
	fetched   time.Time
	attempted time.Time
}{}

func admobKey(ctx context.Context, id string) (*ecdsa.PublicKey, error) {
	adKeys.Lock()
	defer adKeys.Unlock()
	if time.Since(adKeys.fetched) < 6*time.Hour {
		if k := adKeys.keys[id]; k != nil {
			return k, nil
		}
	}
	if time.Since(adKeys.attempted) < time.Minute {
		return nil, problem(503, "广告验签公钥暂不可用，请稍后重试")
	}
	adKeys.attempted = time.Now()
	req, e := http.NewRequestWithContext(ctx, "GET", "https://www.gstatic.com/admob/reward/verifier-keys.json", nil)
	if e != nil {
		return nil, e
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	if e != nil {
		return nil, problem(503, "无法获取AdMob验签公钥")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, problem(503, "AdMob公钥服务返回异常")
	}
	var doc struct {
		Keys []struct {
			ID     json.Number `json:"keyId"`
			PEM    string      `json:"pem"`
			Base64 string      `json:"base64"`
		} `json:"keys"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&doc); e != nil {
		return nil, problem(503, "AdMob公钥格式错误")
	}
	keys := map[string]*ecdsa.PublicKey{}
	for _, x := range doc.Keys {
		var der []byte
		if block, _ := pem.Decode([]byte(x.PEM)); block != nil {
			der = block.Bytes
		} else {
			der, _ = base64.StdEncoding.DecodeString(x.Base64)
		}
		v, e := x509.ParsePKIXPublicKey(der)
		if e == nil {
			if k, ok := v.(*ecdsa.PublicKey); ok {
				keys[string(x.ID)] = k
			}
		}
	}
	if len(keys) == 0 {
		return nil, problem(503, "AdMob未提供有效公钥")
	}
	adKeys.keys = keys
	adKeys.fetched = time.Now()
	if k := keys[id]; k != nil {
		return k, nil
	}
	return nil, problem(403, "未知AdMob签名公钥")
}
func verifyAdmobQuery(raw string, key *ecdsa.PublicKey) (url.Values, error) {
	if len(raw) > 8192 {
		return nil, problem(400, "广告回调过长")
	}
	parts := strings.Split(raw, "&")
	if len(parts) < 4 || !strings.HasPrefix(parts[len(parts)-2], "signature=") || !strings.HasPrefix(parts[len(parts)-1], "key_id=") {
		return nil, problem(400, "广告回调签名参数顺序错误")
	}
	q, e := url.ParseQuery(raw)
	if e != nil {
		return nil, problem(400, "回调参数编码错误")
	}
	for _, v := range q {
		if len(v) != 1 {
			return nil, problem(400, "回调参数重复")
		}
	}
	sig, e := base64.RawURLEncoding.DecodeString(strings.TrimRight(q.Get("signature"), "="))
	if e != nil {
		return nil, problem(403, "广告回调签名编码无效")
	}
	sum := sha256.Sum256([]byte(strings.Join(parts[:len(parts)-2], "&")))
	if key == nil || !ecdsa.VerifyASN1(key, sum[:], sig) {
		return nil, problem(403, "广告回调签名校验失败")
	}
	return q, nil
}
func (a *app) admobCallback(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "GET" {
		return problem(405, "回调仅支持GET")
	}
	if len(r.URL.RawQuery) > 8192 {
		return problem(400, "回调过长")
	}
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil || len(q["key_id"]) != 1 || len(q.Get("key_id")) > 20 {
		return problem(400, "回调公钥参数无效")
	}
	if _, e = strconv.ParseUint(q.Get("key_id"), 10, 64); e != nil {
		return problem(400, "回调公钥编号无效")
	}
	key, e := admobKey(r.Context(), q.Get("key_id"))
	if e != nil {
		return e
	}
	q, e = verifyAdmobQuery(r.URL.RawQuery, key)
	if e != nil {
		return e
	}
	duplicate, e := a.applyAdmobReward(r.Context(), q, time.Now())
	if e != nil {
		return e
	}
	jsonResponse(w, 200, M{"ok": true, "duplicate": duplicate, "ledger": "operations-points"})
	return nil
}
func (a *app) applyAdmobReward(ctx context.Context, q url.Values, t time.Time) (bool, error) {
	txn, user, id := q.Get("transaction_id"), q.Get("user_id"), q.Get("custom_data")
	if len(txn) < 8 || len(txn) > 128 || len(id) != 64 || len(user) != 64 {
		return false, problem(400, "奖励凭据缺失或无效")
	}
	duplicate := false
	e := a.change(ctx, "ads.reward.admob", func(s *state) error {
		total := 0
		for _, acc := range s.Advertising {
			total += len(acc.Rewards)
			for _, rw := range acc.Rewards {
				if rw.Transaction == txn {
					if rw.Decision != id || rw.User != user {
						return problem(409, "奖励交易与原任务不符")
					}
					duplicate = true
					return nil
				}
			}
		}
		var acc *adAccount
		var d *adDecision
		for _, x := range s.Advertising {
			if y := x.Decisions[id]; y != nil {
				acc = x
				d = y
				break
			}
		}
		if d == nil || d.Source != "sdk" || d.Slot != "rewarded" || d.User == "" || d.User != user || d.Rewarded {
			return problem(409, "没有匹配的待验证奖励任务")
		}
		unit := d.Unit
		if slash := strings.LastIndex(unit, "/"); slash >= 0 {
			unit = unit[slash+1:]
		}
		if q.Get("ad_unit") != unit && q.Get("ad_unit") != d.Unit {
			return problem(409, "奖励广告单元不匹配")
		}
		ms, e := strconv.ParseInt(q.Get("timestamp"), 10, 64)
		if e != nil {
			return problem(400, "奖励时间戳无效")
		}
		at := time.UnixMilli(ms)
		if at.Before(d.Created.Add(-time.Minute)) || at.After(d.Created.Add(30*time.Minute)) || at.After(t.Add(time.Minute)) || t.After(d.Expires) {
			return problem(410, "奖励任务已过期或回调时间无效")
		}
		if d.Flags["error"] || d.Flags["no_fill"] {
			return problem(409, "广告任务已经失败")
		}
		if d.RewardPoints < 1 || d.RewardCap < 1 {
			return problem(409, "奖励策略无效")
		}
		count, last := rewardUsage(acc, user, t)
		if count >= d.RewardCap || (!last.IsZero() && t.Sub(last) < time.Duration(d.RewardCooldown)*time.Second) {
			return problem(409, "奖励超过每日限额或处于冷却")
		}
		if total >= 20000 {
			return problem(503, "奖励账本已满，请先维护账本；本次未发放")
		}
		rw := adReward{ID: randomToken(), Transaction: txn, User: user, Decision: id, Channel: d.ChannelID, Points: d.RewardPoints, At: t.UTC().Format(time.RFC3339Nano)}
		acc.Rewards = append([]adReward{rw}, acc.Rewards...)
		d.Rewarded = true
		m := adDay(acc, t)
		m.Rewards++
		m.Points += int64(rw.Points)
		pushAdEvent(acc, d, "reward_verified", "")
		return nil
	})
	return duplicate, e
}
