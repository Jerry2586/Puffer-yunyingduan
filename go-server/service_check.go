package main

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type serviceProbe struct {
	id, name, path string
	content        bool
	valid          func(M) bool
}

// Only the explicitly configured loopback preview may bypass public-network protection.
func (a *app) probeAddress(raw string) (string, bool, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	own, _ := url.Parse(a.config.PublicOrigin)
	if err == nil && own != nil && raw == strings.TrimRight(a.config.PublicOrigin, "/") && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == "" && (u.Scheme == "http" || u.Scheme == "https") {
		if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
			return raw, true, nil
		}
	}
	value, err := httpsAddress(raw, false)
	return value, false, err
}
func (a *app) validProbeDirectory(m M, base string) bool {
	public, _ := x509.MarshalPKIXPublicKey(&a.serviceKey.PublicKey)
	if str(m["publicKey"]) != base64.StdEncoding.EncodeToString(public) {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(str(m["payload"]))
	if err != nil {
		return false
	}
	signature, err := base64.StdEncoding.DecodeString(str(m["signature"]))
	if err != nil {
		return false
	}
	digest := sha256.Sum256(data)
	if rsa.VerifyPKCS1v15(&a.serviceKey.PublicKey, crypto.SHA256, digest[:], signature) != nil {
		return false
	}
	var payload M
	if json.Unmarshal(data, &payload) != nil || payload["contract"] != "hetun-service-directory-v1" {
		return false
	}
	for _, address := range arr(payload["addresses"]) {
		if str(address) == base {
			return true
		}
	}
	return false
}
func probeJSON(ctx context.Context, client *http.Client, target string) (M, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("连接失败，请检查域名、HTTPS证书及服务监听")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, fmt.Errorf("接口返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		return nil, resp.StatusCode, fmt.Errorf("响应读取失败或超过2MB")
	}
	var value M
	if json.Unmarshal(data, &value) != nil || value == nil {
		return nil, resp.StatusCode, fmt.Errorf("接口未返回有效 JSON 对象")
	}
	return value, resp.StatusCode, nil
}
func (a *app) checkServiceAddress(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "POST" {
		return problem(405, "不支持的操作")
	}
	input, err := readBody(w, r, 4096)
	if err != nil {
		return err
	}
	base, local, err := a.probeAddress(str(input["address"]))
	if err != nil {
		return err
	}
	client := cmsClient(local)
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(r.Context(), 16*time.Second)
	defer cancel()
	probes := []serviceProbe{
		{"health", "服务身份", "/api/health", false, func(m M) bool {
			return m["status"] == "ok" && m["service"] == "hetun" && m["product"] == "operations-center" && m["contract"] == contract
		}},
		{"directory", "签名接入目录", "/api/service-directory", false, func(m M) bool { return a.validProbeDirectory(m, base) }},
	}
	for _, pair := range [][2]string{{"mobile", "android"}, {"mobile", "ios"}, {"tablet", "android"}, {"tablet", "ios"}, {"tv", "android"}, {"web", "web"}} {
		p, osName := pair[0], pair[1]
		probes = append(probes, serviceProbe{"bootstrap-" + p + "-" + osName, p + " / " + osName + " 配置", "/api/client/bootstrap?platform=" + p + "&os=" + osName + "&contract=" + contract, false, func(m M) bool {
			_, configOK := m["config"].(map[string]any)
			return m["contract"] == contract && m["themeContract"] == "hetun-runtime-v1" && m["platform"] == p && m["os"] == osName && configOK
		}})
	}
	for _, pair := range [][2]string{{"categories", "分类接口"}, {"catalog", "影片列表接口"}} {
		path := "/api/catalog?page=1"
		if pair[0] == "categories" {
			path = "/api/catalog/categories"
		}
		probes = append(probes, serviceProbe{pair[0], pair[1], path, true, func(m M) bool { _, ok := m["items"].([]any); return ok }})
	}
	checks := make([]M, len(probes))
	var wg sync.WaitGroup
	for i, probe := range probes {
		wg.Add(1)
		go func(i int, p serviceProbe) {
			defer wg.Done()
			start := time.Now()
			value, status, e := probeJSON(ctx, client, base+p.path)
			if e == nil && !p.valid(value) {
				e = fmt.Errorf("协议或服务身份不匹配；签名目录还需包含本次检测域名")
			}
			detail := "接口响应与协议校验通过"
			if e != nil {
				detail = e.Error()
			}
			if e == nil && p.content && len(arr(value["items"])) == 0 {
				detail = "接口可用，当前返回0条内容"
			}
			checks[i] = M{"id": p.id, "name": p.name, "path": p.path, "ok": e == nil, "status": status, "detail": detail, "durationMs": time.Since(start).Milliseconds(), "content": p.content}
		}(i, probe)
	}
	wg.Wait()
	connectionOK, contentOK := true, true
	for i, c := range checks {
		if c["ok"] != true {
			if probes[i].content {
				contentOK = false
			} else {
				connectionOK = false
			}
		}
	}
	jsonResponse(w, 200, M{"address": base, "connectionLink": base + "/api/service-directory", "localPreview": local, "connectionOK": connectionOK, "contentOK": contentOK, "ok": connectionOK && contentOK, "checkedAt": time.Now().UTC().Format(time.RFC3339), "checks": checks})
	return nil
}
