package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixtureCMS(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.Contains(path, "provide/vod"):
			if r.Form.Get("ac") == "list" {
				jsonResponse(w, 200, M{"code": 1, "class": []any{M{"type_id": 1, "type_name": "电影"}}, "list": []any{}})
				return
			}
			jsonResponse(w, 200, M{"code": 1, "page": 1, "pagecount": 1, "total": 2, "list": []any{M{"vod_id": 1, "vod_name": "一", "vod_pic": "/poster.jpg", "vod_score": "7", "vod_play_url": "第一集$https://secret.test/a.m3u8", "vod_play_from": "线路"}, M{"vod_id": 2, "vod_name": "二", "vod_score": "9"}}})
		case strings.HasSuffix(path, "/login"):
			if r.Form.Get("user_pwd") != "correct" {
				jsonResponse(w, 200, M{"code": 0, "msg": "密码错误"})
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "member", Value: r.Form.Get("user_name"), Path: "/"})
			jsonResponse(w, 200, M{"code": 1})
		case strings.HasSuffix(path, "/ajax_upgrade_data"):
			_, e := r.Cookie("member")
			jsonResponse(w, 200, M{"code": 1, "data": M{"is_login": e == nil, "member_name": "VIP", "expire_date": "2027-01-01", "groups": []any{M{"group_id": 3, "group_name": "会员", "price_month": 10}}}})
		case strings.HasSuffix(path, "/get_info"):
			c, e := r.Cookie("member")
			if e != nil {
				jsonResponse(w, 200, M{"code": 1401, "msg": "未登录"})
				return
			}
			jsonResponse(w, 200, M{"code": 1, "info": M{"user_name": c.Value, "user_id": 7, "user_pwd": "NEVER_EXPOSE"}})
		case strings.HasSuffix(path, "/token"):
			jsonResponse(w, 200, M{"code": 1, "info": M{"token": "fixture-csrf"}})
		case strings.HasSuffix(path, "/gopay"):
			if r.Header.Get("X-CSRF-Token") != "fixture-csrf" {
				jsonResponse(w, 200, M{"code": 0, "msg": "缺少CSRF"})
				return
			}
			jsonResponse(w, 200, M{"code": 1, "info": M{"payment_data": M{"type": "html", "html": `<script>alert('untrusted')</script><form action="https://pay.test/pay" method="post"><input name="sign" value="abc&amp;def" type="hidden"><script>fetch('https://evil.test')</script></form>`}}})
		case strings.HasSuffix(path, "/play"):
			if _, e := r.Cookie("member"); e != nil {
				jsonResponse(w, 200, M{"code": 1401, "msg": "需要会员"})
				return
			}
			jsonResponse(w, 200, M{"code": 1, "info": M{"url": "https://video.test/1.m3u8", "name": "第一集"}})
		default:
			jsonResponse(w, 200, M{"code": 1, "info": M{"order_id": 3, "order_code": "A123", "order_price": 10}, "data": M{"order_id": 3, "order_code": "A123"}})
		}
	}))
}

type harness struct {
	a              *app
	origin, cookie string
	t              *testing.T
}

func newHarness(t *testing.T, cms string) *harness {
	t.Helper()
	dir := t.TempDir()
	c := config{PublicOrigin: "http://127.0.0.1:8787", DataDir: dir, AllowPrivateCMS: true}
	a, e := newApp(context.Background(), c, &fileStore{filepath.Join(dir, "state.json")})
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "initial-admin.json"))
	if e != nil {
		t.Fatal(e)
	}
	var login M
	_ = json.Unmarshal(b, &login)
	h := &harness{a: a, origin: c.PublicOrigin, t: t}
	res := h.req("POST", "/api/admin/login", login, "")
	if res.Code != 200 {
		t.Fatalf("login %d: %s", res.Code, res.Body)
	}
	h.cookie = res.Result().Cookies()[0].Value
	if cms != "" {
		res = h.req("POST", "/api/admin/setup/complete", M{"base": cms, "username": "owner", "password": "StrongLocal12345"}, "")
		if res.Code != 200 {
			t.Fatalf("setup %d: %s", res.Code, res.Body)
		}
	}
	return h
}
func (h *harness) req(method, route string, input M, member string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(input)
	r := httptest.NewRequest(method, h.origin+route, strings.NewReader(string(b)))
	r.Header.Set("Origin", h.origin)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Hetun-Client", "studio")
	if h.cookie != "" {
		r.AddCookie(&http.Cookie{Name: "hetun_session", Value: h.cookie})
	}
	if member != "" {
		r.Header.Set("Authorization", "Bearer "+member)
	}
	w := httptest.NewRecorder()
	h.a.ServeHTTP(w, r)
	return w
}
func payload(t *testing.T, w *httptest.ResponseRecorder) M {
	t.Helper()
	var m M
	if e := json.Unmarshal(w.Body.Bytes(), &m); e != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	return m
}
func TestSetupAndAdminBoundary(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, "")
	if w := h.req("GET", "/api/admin/state", nil, ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := h.req("POST", "/api/admin/setup/complete", M{"base": "bad", "username": "owner", "password": "StrongLocal12345"}, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := h.req("POST", "/api/admin/setup/complete", M{"base": cms.URL, "username": "owner", "password": "StrongLocal12345"}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if _, e := os.Stat(filepath.Join(h.a.config.DataDir, "initial-admin.json")); !os.IsNotExist(e) {
		t.Fatal("initial password retained")
	}
	for _, route := range []string{"/api/admin/license", "/api/admin/export", "/api/admin/install"} {
		if w := h.req("GET", route, nil, ""); w.Code != 404 {
			t.Fatal(route, w.Code, w.Body)
		}
	}
	old := h.cookie
	h.cookie = ""
	if w := h.req("GET", "/api/admin/state", nil, ""); w.Code != 401 {
		t.Fatal("unprotected admin", w.Code)
	}
	h.cookie = old
	r := httptest.NewRequest("POST", h.origin+"/api/admin/publish", strings.NewReader(`{"version":1}`))
	r.AddCookie(&http.Cookie{Name: "hetun_session", Value: old})
	r.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	h.a.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF", w.Code)
	}
	w = h.req("PUT", "/api/admin/account", M{"currentPassword": "StrongLocal12345", "username": "owner", "password": "NextPassword12345"}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("POST", "/api/admin/login", M{"username": "owner", "password": "StrongLocal12345"}, ""); w.Code != 401 {
		t.Fatal("old password valid")
	}
}
func TestPublishRollbackPersistenceAndPlatforms(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	draft := clone(defaults)
	draft["colors"] = M{"accent": "BAD_VISUAL"}
	draft["brand"] = M{"name": "NO_OVERRIDE"}
	obj(arr(draft["sections"])[0])["title"] = "更新栏目"
	obj(draft["clientSettings"])["latestVersion"] = "2.0.0"
	obj(draft["clientSettings"])["androidUrl"] = "https://download.test/app.apk"
	w := h.req("PUT", "/api/admin/draft", M{"version": 1, "theme": draft, "template": "runtime-tv"}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("POST", "/api/admin/publish", M{"version": 1, "template": "runtime-tv"}, ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w = h.req("POST", "/api/admin/publish", M{"version": 2, "template": "runtime-tv"}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = h.req("GET", "/api/client/bootstrap?platform=tv&version=1.0.0&os=android", nil, "")
	m := payload(t, w)
	if w.Code != 200 || number(m["version"]) != 2 || m["contract"] != contract || obj(m["update"])["available"] != true {
		t.Fatal(w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "BAD_VISUAL") || strings.Contains(w.Body.String(), "NO_OVERRIDE") {
		t.Fatal("visual override leaked")
	}
	for _, route := range []string{"/api/bootstrap?platform=tv", "/api/runtime?platform=tv"} {
		res := h.req("GET", route, nil, "")
		if res.Code != 200 || payload(t, res)["contract"] != contract {
			t.Fatal(route, res.Code, res.Body)
		}
	}
	mobile := payload(t, h.req("GET", "/api/client/bootstrap?platform=mobile", nil, ""))
	if number(mobile["version"]) != 1 {
		t.Fatal("platform bleed")
	}
	for _, route := range []string{"/api/client/bootstrap?contract=other", "/api/client/bootstrap?os=bad", "/api/client/bootstrap?version=1.bad"} {
		if w = h.req("GET", route, nil, ""); w.Code < 400 {
			t.Fatal(route)
		}
	}
	if w = h.req("POST", "/api/admin/rollback", M{"version": 3, "targetVersion": 1, "template": "runtime-tv"}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if h.a.readState().Operations["tv"].PublishedVersion != 3 {
		t.Fatal("rollback version regressed")
	}
	a, e := newApp(context.Background(), h.a.config, h.a.store)
	if e != nil {
		t.Fatal(e)
	}
	if a.readState().Operations["tv"].PublishedVersion != 3 || !matches(a.readState().Credential, "StrongLocal12345") {
		t.Fatal("restart lost state")
	}
	// Simultaneous windows can only save one draft at a given version.
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- h.req("PUT", "/api/admin/draft", M{"version": 1, "theme": defaults}, "").Code
		}()
	}
	wg.Wait()
	close(statuses)
	ok, conflict := 0, 0
	for status := range statuses {
		if status == 200 {
			ok++
		}
		if status == 409 {
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal(ok, conflict)
	}
}
func TestCatalogAndMembership(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	w := h.req("GET", "/api/catalog?ids=2,1&orderBy=manual", nil, "")
	m := payload(t, w)
	items := arr(m["items"])
	if w.Code != 200 || len(items) != 2 || obj(items[0])["id"] != "2" || strings.Contains(w.Body.String(), "secret.test") {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("GET", "/api/catalog?orderBy=hot", nil, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = h.req("POST", "/api/playback", M{"id": "1", "sid": 1, "nid": 1}, ""); w.Code == 200 {
		t.Fatal("anonymous playback allowed")
	}
	w = h.req("POST", "/api/member/login", M{"username": "alice", "password": "correct"}, "")
	m = payload(t, w)
	token := str(m["token"])
	if w.Code != 200 || token == "" || strings.Contains(w.Body.String(), "NEVER_EXPOSE") {
		t.Fatal(w.Code, w.Body)
	}
	profile := obj(m["profile"])
	if profile["member_name"] != "VIP" || profile["expire_date"] != "2027-01-01" {
		t.Fatal("member profile contract", profile)
	}
	for _, price := range []string{"NaN", "+Inf", "-Inf"} {
		if bad := h.req("POST", "/api/member/recharge", M{"price": price}, token); bad.Code != 400 {
			t.Fatal("nonfinite amount accepted", price)
		}
	}
	if w = h.req("POST", "/api/playback", M{"id": "1", "sid": 1, "nid": 1}, token); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w = h.req("POST", "/api/member/purchase", M{"platform": "ios", "groupId": 3, "period": "month"}, token); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = h.req("POST", "/api/member/checkout", M{"orderId": 3, "code": "A123", "payment": "epay"}, token)
	m = payload(t, w)
	if w.Code != 200 || m["kind"] != "form" {
		t.Fatal(w.Code, w.Body)
	}
	w = h.req("GET", str(m["checkoutPath"]), nil, "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "<script") || !strings.Contains(w.Body.String(), "abc&amp;def") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "form-action https://pay.test") {
		t.Fatal(w.Code, w.Body)
	}
	w = h.req("POST", "/api/member/logout", M{}, token)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = h.req("GET", "/api/member/profile", nil, token); w.Code != 401 {
		t.Fatal("logout invalidation failed")
	}
}
func TestCMSNetworkAndPaymentGuards(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	u, _ := url.Parse(cms.URL)
	if _, e := cmsFetch(ctx, cmsClient(false), u, "GET", nil, nil); e == nil {
		t.Fatal("private CMS allowed by default")
	}
	for _, data := range []M{{"type": "html", "html": `<form action="javascript:evil()"><input name="sign" value="x"></form>`}, {"type": "html", "html": `<form action="https://pay.test" action="https://evil.test"><input name="x"></form>`}, {"type": "html", "html": `<script>fetch('https://evil.test')</script>`}, {"type": "qrcode", "code_url": "javascript:alert(1)"}} {
		if _, e := normalizePayment(data, "https://cms.test"); e == nil {
			t.Fatal("unsafe payment accepted", data)
		}
	}
}
func TestNoRuntimeOrSecretFilesExposed(t *testing.T) {
	h := newHarness(t, "")
	for _, route := range []string{"/state.json", "/initial-admin.json", "/config.json", "/audit.jsonl", "/go.mod", "/../state.json"} {
		w := h.req("GET", route, nil, "")
		b, _ := io.ReadAll(w.Result().Body)
		if w.Code == 200 {
			t.Fatal("private file route served", route, string(b))
		}
	}
}

func TestOperationalValidationAndConfig(t *testing.T) {
	for _, value := range []any{1.5, 1e50} {
		v := initialState()
		if err := mutateOperations(&v, M{"version": value, "template": "mobile", "theme": clone(defaults)}, "draft"); err == nil {
			t.Fatal("fractional or overflowing version accepted")
		}
	}
	input := clone(defaults)
	input["ads"] = []any{M{"id": "ad-1", "enabled": true, "start": "2026-10-20T12:00", "end": "2026-10-20T13:00"}}
	clean, err := operationConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(str(obj(arr(clean["ads"])[0])["start"]), "Z") {
		t.Fatal("date not normalized")
	}
	input["ads"] = []any{M{"id": "ad-1", "enabled": true, "start": "2026-10-20T12:00:00+08:00", "end": "2026-10-20T05:00:00Z"}}
	if _, err = operationConfig(input); err != nil {
		t.Fatal("timezone chronological compare", err)
	}
	input["ads"] = []any{M{"id": "ad-1", "enabled": "false"}}
	if _, err = operationConfig(input); err == nil {
		t.Fatal("string switch accepted")
	}
	file := filepath.Join(t.TempDir(), "config.json")
	c := config{Listen: "127.0.0.1:8787", PublicOrigin: "http://127.0.0.1:8787", DataDir: t.TempDir(), Storage: "file"}
	body, _ := json.Marshal(c)
	if err = os.WriteFile(file, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadConfig(file); err != nil {
		t.Fatal(err)
	}
	var legacy M
	_ = json.Unmarshal(body, &legacy)
	legacy["licenseKey"] = "obsolete"
	body, _ = json.Marshal(legacy)
	_ = os.WriteFile(file, body, 0600)
	if _, err = loadConfig(file); err == nil {
		t.Fatal("obsolete commercial config accepted")
	}
}

func TestAdministratorPasswordChoice(t *testing.T) {
	for _, password := range []string{"1", "abc", "中文", strings.Repeat("x", 300)} {
		c, err := credentials("owner", password)
		if err != nil {
			t.Fatal("chosen password rejected", err)
		}
		if !matches(c, password) || matches(c, password+"wrong") {
			t.Fatal("password matching failed")
		}
	}
	if _, err := credentials("owner", ""); err == nil {
		t.Fatal("empty password accepted")
	}
}

func TestLegacyAdminEntryRedirectsToShortEntry(t *testing.T) {
	c := config{Listen: "127.0.0.1:8787", PublicOrigin: "http://127.0.0.1:8787", DataDir: t.TempDir(), Storage: "file", AdminPath: "/abcdef1234567890"}
	c, err := validateConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	a, err := newApp(context.Background(), c, &fileStore{filepath.Join(t.TempDir(), "state.json")})
	if err != nil {
		t.Fatal(err)
	}
	defer a.store.Close()
	for _, entry := range []string{c.AdminPath, c.AdminPath + "/", "/abcdef/"} {
		w := httptest.NewRecorder()
		a.ServeHTTP(w, httptest.NewRequest("GET", c.PublicOrigin+entry, nil))
		if w.Code != 307 || w.Header().Get("Location") != "/abcdef" {
			t.Fatal("legacy entry did not redirect", w.Code)
		}
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, httptest.NewRequest("GET", c.PublicOrigin+"/abcdef", nil))
	if w.Code != 200 {
		t.Fatal("short entry unavailable", w.Code)
	}
	for _, entry := range []string{"/123456", "/abcdef1234567890"} {
		c.AdminPath = entry
		if _, err := validateConfig(c); err != nil {
			t.Fatal(err)
		}
	}
	c.AdminPath = "/abc"
	if _, err := validateConfig(c); err == nil {
		t.Fatal("invalid entry accepted")
	}
}
