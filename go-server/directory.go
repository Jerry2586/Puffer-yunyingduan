package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *app) loadServiceKey() error {
	file := filepath.Join(a.config.DataDir, "service-key.pem")
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		key, e := rsa.GenerateKey(rand.Reader, 2048)
		if e != nil {
			return e
		}
		data = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
		f, e := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(e) {
			data, err = os.ReadFile(file)
		} else if e != nil {
			return e
		} else {
			_, err = f.Write(data)
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		return err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("服务身份密钥格式错误")
	}
	a.serviceKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	return err
}
func (a *app) serviceAddresses(v state) []string {
	out := []string{}
	for _, raw := range arr(v.ServiceRouting["addresses"]) {
		if s := str(raw); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = append(out, a.config.PublicOrigin)
	}
	return out
}
func (a *app) directory(v state) M {
	public, _ := x509.MarshalPKIXPublicKey(&a.serviceKey.PublicKey)
	digest := sha256.Sum256(public)
	// No expiry: a pre-exported recovery file must remain usable after a long outage.
	payload, _ := json.Marshal(M{"contract": "hetun-service-directory-v1", "keyId": hex.EncodeToString(digest[:]), "revision": numericOr(v.ServiceRouting["revision"], 0), "addresses": a.serviceAddresses(v), "recoveryUrls": arr(v.ServiceRouting["recoveryUrls"]), "issuedAt": time.Now().UTC().Format(time.RFC3339)})
	hash := sha256.Sum256(payload)
	signature, _ := rsa.SignPKCS1v15(rand.Reader, a.serviceKey, crypto.SHA256, hash[:])
	return M{"payload": base64.StdEncoding.EncodeToString(payload), "signature": base64.StdEncoding.EncodeToString(signature), "publicKey": base64.StdEncoding.EncodeToString(public)}
}
func (a *app) handleDirectory(w http.ResponseWriter, r *http.Request) (bool, error) {
	if r.URL.Path != "/api/service-directory" || r.Method != "GET" {
		return false, nil
	}
	w.Header().Set("Cache-Control", "no-store")
	jsonResponse(w, 200, a.directory(a.readState()))
	return true, nil
}
func httpsAddress(raw string, file bool) (string, error) {
	u, e := url.Parse(strings.TrimSpace(raw))
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (!file && u.Path != "" && u.Path != "/") {
		return "", problem(400, "请输入完整 HTTPS 域名，不能包含账号、参数或后台入口")
	}
	if !file {
		u.Path = ""
	}
	return u.String(), nil
}
func (a *app) adminDirectory(w http.ResponseWriter, r *http.Request) error {
	if r.Method == "GET" {
		v := a.readState()
		jsonResponse(w, 200, M{"addresses": a.serviceAddresses(v), "recoveryUrls": arr(v.ServiceRouting["recoveryUrls"]), "directory": a.directory(v)})
		return nil
	}
	if r.Method != "PUT" {
		return problem(405, "不支持的操作")
	}
	input, e := readBody(w, r, 32768)
	if e != nil {
		return e
	}
	addresses := []any{}
	seen := map[string]bool{}
	rawAddresses := arr(input["addresses"])
	if len(rawAddresses) < 1 || len(rawAddresses) > 5 {
		return problem(400, "请设置一个主域名，最多四个备用域名")
	}
	for _, raw := range rawAddresses {
		s, e := httpsAddress(str(raw), false)
		if e != nil {
			return e
		}
		if !seen[s] {
			addresses = append(addresses, s)
			seen[s] = true
		}
	}
	recovery := []any{}
	if len(arr(input["recoveryUrls"])) > 3 {
		return problem(400, "独立恢复地址最多三个")
	}
	for _, raw := range arr(input["recoveryUrls"]) {
		s, e := httpsAddress(str(raw), true)
		if e != nil {
			return e
		}
		recovery = append(recovery, s)
	}
	e = a.change(r.Context(), "service.addresses.updated", func(next *state) error {
		next.ServiceRouting = M{"addresses": addresses, "recoveryUrls": recovery, "revision": numericOr(next.ServiceRouting["revision"], 0) + 1}
		return nil
	})
	if e != nil {
		return e
	}
	v := a.readState()
	jsonResponse(w, 200, M{"addresses": a.serviceAddresses(v), "recoveryUrls": recovery, "directory": a.directory(v)})
	return nil
}
