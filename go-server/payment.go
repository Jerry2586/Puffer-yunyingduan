package main

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type checkout struct {
	Data  M
	Until time.Time
}

var formPattern = regexp.MustCompile(`(?is)<form\b([^>]*)>(.*?)</form>`)
var inputPattern = regexp.MustCompile(`(?is)<input\b([^>]*)>`)
var attrPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9_-]*)\s*=\s*(?:"([^"<>]*)"|'([^'<>]*)'|([^\s<>"']+))`)
var redirectPattern = regexp.MustCompile(`(?i)(?:window\.)?location(?:\.href)?\s*=\s*["']([^"'<>\r\n]+)["']`)

func attrs(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, m := range attrPattern.FindAllStringSubmatch(s, -1) {
		key := strings.ToLower(m[1])
		if _, ok := out[key]; ok {
			return nil, problem(400, "收银台字段重复")
		}
		value := m[2]
		if value == "" {
			value = m[3]
		}
		if value == "" {
			value = m[4]
		}
		out[key] = html.UnescapeString(value)
	}
	return out, nil
}
func paymentURL(value, base string) (string, error) {
	b, e := siteURL(base)
	if e != nil {
		return "", e
	}
	u, e := url.Parse(value)
	if e != nil {
		return "", problem(400, "收银台地址无效")
	}
	u = b.ResolveReference(u)
	if u.Host == "" || u.User != nil || !has([]string{"http", "https"}, u.Scheme) || len(u.String()) > 8000 {
		return "", problem(400, "收银台地址无效")
	}
	return u.String(), nil
}
func normalizePayment(data M, base string) (M, error) {
	switch str(data["type"]) {
	case "qrcode":
		code := str(data["code_url"])
		u, e := url.Parse(code)
		if e != nil || !has([]string{"http", "https", "weixin", "alipays"}, u.Scheme) || u.Host == "" || len(code) > 2000 || strings.IndexFunc(code, func(r rune) bool { return r <= 32 }) >= 0 {
			return nil, problem(400, "支付二维码无效")
		}
		return M{"kind": "qrcode", "code": code}, nil
	case "data":
		d := obj(data["data"])
		s := str(d["pay_url"])
		if s == "" {
			s = str(d["url"])
		}
		if s == "" {
			break
		}
		u, e := paymentURL(s, base)
		if e != nil {
			return nil, e
		}
		return M{"kind": "redirect", "url": u}, nil
	case "html":
		markup := str(data["html"])
		if len(markup) > 120000 {
			return nil, problem(400, "收银台响应过大")
		}
		form := formPattern.FindStringSubmatch(markup)
		if len(form) > 0 {
			a, e := attrs(form[1])
			if e != nil {
				return nil, e
			}
			action, e := paymentURL(a["action"], base)
			if e != nil {
				return nil, e
			}
			method := strings.ToLower(a["method"])
			if method == "" {
				method = "get"
			}
			if !has([]string{"get", "post"}, method) {
				return nil, problem(400, "收银台方法不支持")
			}
			fields := []any{}
			for _, input := range inputPattern.FindAllStringSubmatch(form[2], -1) {
				v, e := attrs(input[1])
				if e != nil {
					return nil, e
				}
				if v["type"] != "" && strings.ToLower(v["type"]) != "hidden" {
					continue
				}
				if v["name"] == "" || len(v["name"]) > 150 || len(v["value"]) > 16000 {
					return nil, problem(400, "收银台字段无效")
				}
				fields = append(fields, M{"name": v["name"], "value": v["value"]})
				if len(fields) > 100 {
					return nil, problem(400, "收银台字段过多")
				}
			}
			if len(fields) == 0 {
				return nil, problem(400, "收银台签名字段缺失")
			}
			return M{"kind": "form", "action": action, "method": method, "fields": fields}, nil
		}
		if redirect := redirectPattern.FindStringSubmatch(markup); len(redirect) > 0 {
			u, e := paymentURL(html.UnescapeString(redirect[1]), base)
			if e != nil {
				return nil, e
			}
			return M{"kind": "redirect", "url": u}, nil
		}
	}
	return nil, problem(400, "当前支付网关格式未支持，请在 CMS 收银台完成付款")
}
func (a *app) checkoutPage(w http.ResponseWriter, r *http.Request, id string) error {
	if r.Method != "GET" {
		return problem(405, "请求方法不支持")
	}
	a.mu.Lock()
	c, ok := a.checkouts[id]
	if !ok || time.Now().After(c.Until) {
		delete(a.checkouts, id)
		a.mu.Unlock()
		return problem(410, "收银台已过期，请重新选择支付方式")
	}
	a.mu.Unlock()
	p := c.Data
	u, _ := url.Parse(str(p["action"]))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action "+u.Scheme+"://"+u.Host+"; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	escape := html.EscapeString
	page := `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>河豚 · CMS 收银台</title><style>body{background:#10151e;color:#f5f5f5;font:16px system-ui;padding:40px;max-width:600px;margin:auto}button{background:#dec799;border:0;border-radius:8px;padding:16px 24px;font-size:16px}p{line-height:1.8;color:#bbc3d0}</style><h1>继续到支付平台</h1><p>支付金额和签名由苹果 CMS 生成。完成付款后请返回河豚会员中心查询订单。</p><form action="` + escape(str(p["action"])) + `" method="` + escape(str(p["method"])) + `">`
	for _, raw := range arr(p["fields"]) {
		v := obj(raw)
		page += `<input type="hidden" name="` + escape(str(v["name"])) + `" value="` + escape(str(v["value"])) + `">`
	}
	page += `<button type="submit">前往付款</button></form></html>`
	_, e := w.Write([]byte(page))
	return e
}
