package main

import (
	"context"
	"net/http"
	"strings"
)

// Client paths are adapters to the existing handlers, never a second business layer.
type clientScopeKey struct{}

func clientScope(r *http.Request) string {
	p, _ := r.Context().Value(clientScopeKey{}).(string)
	return p
}
func clientName(p string) string {
	if p == "web" {
		return "desktop"
	}
	return p
}

var publicClientRoutes = map[string]string{
	"playback/config": "/api/playback/config", "playback/negotiate": "/api/playback/negotiate", "playback/report": "/api/playback/report",
	"bootstrap": "/api/client/bootstrap", "catalog": "/api/catalog", "catalog/categories": "/api/catalog/categories", "playback": "/api/playback",
	"member/config": "/api/member/config", "member/plans": "/api/member/plans", "member/profile": "/api/member/profile", "member/orders": "/api/member/orders", "member/order": "/api/member/order",
	"member/login": "/api/member/login", "member/register": "/api/member/register", "member/logout": "/api/member/logout", "member/purchase": "/api/member/purchase", "member/upgrade": "/api/member/upgrade", "member/recharge": "/api/member/recharge", "member/checkout": "/api/member/checkout", "member/card": "/api/member/card",
	"ads/config": "/api/ads/config", "ads/decision": "/api/ads/decision", "ads/events": "/api/ads/events", "ads/balance": "/api/ads/balance",
}
var adminClientRoutes = map[string]string{
	"statistics": "/api/admin/statistics", "state": "/api/admin/state", "draft": "/api/admin/draft", "publish": "/api/admin/publish", "rollback": "/api/admin/rollback",
	"cache": "/api/admin/client-cache", "cache/clear": "/api/admin/client-cache/clear", "cache/search/rebuild": "/api/admin/client-cache/search/rebuild",
	"ads": "/api/admin/ads", "ads/publish": "/api/admin/ads/publish", "ads/diagnose": "/api/admin/ads/diagnose",
	"releases": "/api/admin/releases", "releases/delete": "/api/admin/releases/delete",
}

func normalizeClientRoute(r *http.Request) (*http.Request, error) {
	prefix, routes := "/api/v1/", publicClientRoutes
	if strings.HasPrefix(r.URL.Path, "/api/admin/clients/") {
		prefix, routes = "/api/admin/clients/", adminClientRoutes
	}
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return r, nil
	}
	name, endpoint, found := strings.Cut(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if !found || !has([]string{"mobile", "tablet", "tv", "desktop"}, name) {
		return nil, problem(404, "客户端接口不存在")
	}
	target, ok := routes[endpoint]
	if !ok {
		return nil, problem(404, "客户端接口不存在")
	}
	p, _ := platform(name)
	scoped := r.Clone(context.WithValue(r.Context(), clientScopeKey{}, p))
	if _, err := requestPlatform(scoped); err != nil {
		return nil, err
	}
	query := r.URL.Query()
	for _, value := range query["template"] {
		candidate, err := platform(value)
		if value == "" || err != nil || candidate != p {
			return nil, problem(400, "模板标识与客户端路径不一致")
		}
	}
	if len(query["template"]) > 1 {
		return nil, problem(400, "模板标识不能重复")
	}
	query.Set("platform", p)
	if target == "/api/admin/state" {
		query.Set("template", p)
	}
	u := *r.URL
	u.Path, u.RawPath, u.RawQuery = target, "", query.Encode()
	scoped.URL = &u
	return scoped, nil
}

// Only operations writes use template as a client selector. Payment platform fields
// belong to the CMS contract and must not be reinterpreted as a device selector.
func scopeOperationInput(r *http.Request, input M) error {
	p := clientScope(r)
	if p == "" {
		return nil
	}
	if raw, exists := input["template"]; exists {
		value, ok := raw.(string)
		candidate, err := platform(value)
		if !ok || value == "" || err != nil || candidate != p {
			return problem(400, "模板标识与客户端路径不一致")
		}
	}
	input["template"] = p
	return nil
}
func publicAdURL(r *http.Request, p, endpoint string) string {
	if clientScope(r) != "" {
		return "/api/v1/" + clientName(p) + "/ads/" + endpoint
	}
	return "/api/ads/" + endpoint + "?platform=" + p
}
