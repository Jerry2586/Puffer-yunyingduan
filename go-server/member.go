package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var memberPaths = regexp.MustCompile(`^(api|index)\.php/(user|order|payment|hetun)/[a-z_]+$`)
var orderCode = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
var vodID = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
var methods = []string{"alipay", "weixin", "epay", "codepay", "zhapay"}

func (a *app) memberSession(r *http.Request, base string, optional bool) (*memberSession, error) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	a.mu.Lock()
	defer a.mu.Unlock()
	if s, ok := a.members[token]; ok && s.Base == base && time.Now().Before(s.Until) {
		return s, nil
	}
	delete(a.members, token)
	if !optional {
		return nil, problem(401, "请先登录影视会员")
	}
	jar, _ := cookiejar.New(nil)
	return &memberSession{Jar: jar, Base: base, Until: time.Now().Add(8 * time.Hour)}, nil
}
func (a *app) memberCall(ctx context.Context, s *memberSession, route, method string, params url.Values) (M, error) {
	if !memberPaths.MatchString(route) {
		return nil, problem(400, "CMS 接口不允许")
	}
	base, e := siteURL(s.Base)
	if e != nil {
		return nil, e
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	// Select the legacy write endpoint before sending a mutation. Never retry a
	// registration or points upgrade after an upstream write has been attempted.
	if s.LegacyAPI {
		switch route {
		case "api.php/user/register":
			route = "index.php/user/reg"
		case "api.php/payment/upgrade":
			route = "index.php/user/upgrade"
		}
	}
	base.Path += route
	v, e := cmsFetch(ctx, a.cms, base, method, params, s.Jar)
	// Fall back only when a known newer route is absent. Authentication failures,
	// malformed responses and server failures must not trigger a second attempt.
	var upstream *apiError
	if errors.As(e, &upstream) && upstream.Code == "CMS_HTTP_404" {
		legacy := map[string]string{
			"api.php/user/get_info":          "index.php/hetun/member_info",
			"api.php/user/ajax_upgrade_data": "index.php/hetun/member_plans",
			"api.php/user/login":             "index.php/user/login",
			"api.php/order/get_list":         "index.php/hetun/member_orders",
			"api.php/order/check_status":     "index.php/hetun/member_order",
		}
		if fallback := legacy[route]; fallback != "" {
			base.Path = strings.TrimSuffix(base.Path, route) + fallback
			v, e = cmsFetch(ctx, a.cms, base, method, params, s.Jar)
			if e == nil && strings.HasPrefix(route, "api.php/user/") {
				s.LegacyAPI = true
			}
		}
	}
	if e != nil {
		return nil, e
	}
	if numericOr(v["code"], 0) != 1 {
		status := 400
		if numericOr(v["code"], 0) == 1401 {
			status = 401
		}
		message := clean(v["msg"])
		if message == "" {
			message = "CMS 拒绝请求"
		}
		if len(message) > 600 {
			message = message[:600]
		}
		return nil, problem(status, message)
	}
	return v, nil
}
func (a *app) profile(ctx context.Context, s *memberSession) (M, error) {
	v, e := a.memberCall(ctx, s, "api.php/user/get_info", "GET", nil)
	if e != nil {
		return nil, e
	}
	out := pick(obj(v["info"]), "user_id", "user_name", "user_nick_name", "group_id", "user_points", "user_portrait", "user_end_time")
	plans, e := a.memberCall(ctx, s, "api.php/user/ajax_upgrade_data", "GET", nil)
	if e != nil {
		return nil, e
	}
	d := obj(plans["data"])
	out["member_name"] = str(d["member_name"])
	out["expire_date"] = str(d["expire_date"])
	return out, nil
}
func (a *app) csrf(ctx context.Context, s *memberSession) error {
	base, e := siteURL(s.Base)
	if e != nil {
		return e
	}
	s.Mu.Lock()
	hasToken := false
	for _, c := range s.Jar.Cookies(base) {
		if c.Name == "csrf_token" && c.Value != "" {
			hasToken = true
		}
	}
	s.Mu.Unlock()
	if hasToken {
		return nil
	}
	v, e := a.memberCall(ctx, s, "index.php/hetun/token", "GET", nil)
	if e != nil {
		return e
	}
	token := str(obj(v["info"])["token"])
	if token == "" {
		return problem(400, "CMS 未返回 CSRF 令牌，请安装河豚适配文件")
	}
	s.Mu.Lock()
	s.Jar.SetCookies(base, []*http.Cookie{{Name: "csrf_token", Value: token, Path: base.Path}})
	s.Mu.Unlock()
	return nil
}
func safeOrder(v any) M {
	return pick(obj(v), "order_id", "order_code", "order_price", "order_status", "order_status_text", "order_time", "order_pay_time", "order_points", "order_type", "order_info")
}
func (a *app) memberRoute(w http.ResponseWriter, r *http.Request, v state) error {
	route := r.URL.Path
	if strings.HasPrefix(route, "/api/checkout/") {
		return a.checkoutPage(w, r, strings.TrimPrefix(route, "/api/checkout/"))
	}
	input := M{}
	if r.Method == "POST" {
		var e error
		input, e = readBody(w, r, 256*1024)
		if e != nil {
			return e
		}
	} else {
		for k, x := range r.URL.Query() {
			if len(x) > 0 {
				input[k] = x[0]
			}
		}
	}
	p, e := requestPlatform(r)
	if e != nil {
		return e
	}
	if route != "/api/playback" && route != "/api/member/logout" && obj(v.Operations[p].Published["features"])["member"] != true {
		return problem(403, "当前客户端未开放会员功能")
	}
	if route == "/api/playback" {
		if r.Method != "POST" {
			return problem(405, "请求方法不支持")
		}
		id := str(input["id"])
		sid, sidOK := managementInteger(input["sid"], 1, 1000)
		nid, nidOK := managementInteger(input["nid"], 1, 10000)
		if !vodID.MatchString(id) || !sidOK || !nidOK {
			return problem(400, "影片和集数参数无效")
		}
		s, e := a.memberSession(r, v.CMSBase, true)
		if e != nil {
			return e
		}
		result, e := a.memberCall(r.Context(), s, "index.php/hetun/play", "POST", url.Values{"id": {id}, "sid": {strconv.Itoa(sid)}, "nid": {strconv.Itoa(nid)}})
		if e != nil {
			return fmt.Errorf("播放权限校验失败（需安装河豚 CMS 适配文件）: %w", e)
		}
		// Permission is always checked by CMS before applying local source rules.
		code, trial, mode, e := a.playbackSourcePolicy(r.Context(), v, id, sid, nid)
		if e != nil {
			return e
		}
		info := obj(result["info"])
		u, e := url.Parse(str(info["url"]))
		if e != nil || u.Host == "" || !has([]string{"http", "https"}, u.Scheme) || u.User != nil {
			return problem(400, "CMS 未返回支持的视频地址")
		}
		var engineClient *engineBuild
		if raw, exists := input["playbackClient"]; exists {
			engineClient = &engineBuild{}
			if e := decodeEngineValue(raw, engineClient); e != nil {
				return e
			}
		}
		enginePolicy, e := negotiateEngine(v, p, engineClient)
		if e != nil {
			return e
		}
		settings := playbackState(v)
		excluded, e := resolverExcludes(input["excludeSourceIds"], settings.Published)
		if e != nil {
			return e
		}
		resolved := playbackResolution{OK: true, URL: u.String(), SourceID: "direct", Kind: "direct", Attempts: []resolverAttempt{}}
		if mode == "direct" && has(excluded, "direct") {
			return problem(502, "直连播放失败，请切换其他线路")
		}
		if mode != "direct" {
			resolved, e = a.resolvePlayback(r.Context(), playbackConfigForSource(v, code), u.String(), p, excluded)
			if e != nil {
				return e
			}
		}
		if !resolved.OK {
			jsonResponse(w, 502, M{"error": resolved.Message, "code": "PLAYBACK_RESOLUTION_FAILED", "attempts": resolved.Attempts})
			return nil
		}
		out := M{"url": resolved.URL, "name": str(info["name"]), "sourceId": resolved.SourceID, "kind": resolved.Kind, "attempts": resolved.Attempts, "resolverVersion": settings.PublishedVersion, "enginePolicy": enginePolicy, "sourceCode": code, "trialSeconds": trial}
		if engineClient != nil && enginePolicy["status"] == "negotiated" {
			out["engineReportTicket"] = issueEngineReportTicket(v, p, engineClient, enginePolicy)
		}
		jsonResponse(w, 200, out)
		return nil
	}
	name := strings.TrimPrefix(route, "/api/member/")
	writes := []string{"login", "register", "logout", "purchase", "upgrade", "recharge", "checkout", "card"}
	if (has(writes, name) && r.Method != "POST") || (!has(writes, name) && r.Method != "GET") {
		return problem(405, "请求方法不支持")
	}
	if name == "config" {
		jsonResponse(w, 200, M{"configured": v.CMSBase != "", "provider": "MacCMS V10", "iapReady": false, "cardReady": false, "paymentMethods": methods})
		return nil
	}
	if name == "login" || name == "register" {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		key := "member:" + ip
		a.mu.Lock()
		attempt := a.attempts[key]
		if time.Since(attempt.Start) > time.Minute {
			attempt.Start = time.Now()
			attempt.Count = 0
		}
		if attempt.Count >= 10 {
			a.mu.Unlock()
			return problem(429, "操作频繁，请稍后重试")
		}
		attempt.Count++
		a.attempts[key] = attempt
		a.mu.Unlock()
		user, pass := str(input["username"]), str(input["password"])
		if len(user) < 1 || len(user) > 80 || len(pass) < 1 || len(pass) > 128 {
			return problem(400, "会员账号或密码无效")
		}
		if e := a.verifyApplicationCaptcha(r, v, name, input); e != nil {
			return e
		}
		if e := a.applicationSnapshotCurrent(v, p); e != nil {
			return e
		}
		jar, _ := cookiejar.New(nil)
		s := &memberSession{Jar: jar, Base: v.CMSBase, Until: time.Now().Add(8 * time.Hour)}
		registeredSession := false
		if name == "register" {
			if pass != str(input["password2"]) {
				return problem(400, "两次输入的密码不一致")
			}
			cfg := applicationState(v).Config
			var connection *managementConnection
			if cfg.Registration.Managed {
				connection, e = a.managementConnectionFor(v)
				if e != nil {
					return e
				}
			}
			inviter := 0
			if invite := str(input["inviteCode"]); invite != "" {
				inviter = invitationUser(v, invite)
				if !cfg.Share.Invite || inviter == 0 {
					return problem(400, "邀请码无效或邀请功能已关闭")
				}
			}
			if _, e := a.reserveRegistration(r, v); e != nil {
				return e
			}
			if cfg.Registration.Managed {
				if e := a.applicationSnapshotCurrent(v, p); e != nil {
					return e
				}
				_, e = a.managementCall(r.Context(), connection, "member.register", registrationMemberData(cfg, user, pass, requestIP(r), inviter))
			} else {
				registeredSession, e = a.registerCMSMember(r.Context(), s, url.Values{"user_name": {user}, "user_pwd": {pass}, "user_pwd2": {str(input["password2"])}, "verify": {str(input["verify"])}})
			}
			if e != nil {
				return e
			}
			if cfg.Registration.Managed && !cfg.Registration.Active {
				jsonResponse(w, 200, M{"registered": true, "pending": true})
				return nil
			}
		}
		if !registeredSession {
			if _, e = a.memberCall(r.Context(), s, "api.php/user/login", "POST", url.Values{"user_name": {user}, "user_pwd": {pass}}); e != nil {
				return e
			}
		}
		profile, e := a.profile(r.Context(), s)
		if e != nil {
			return e
		}
		s.UserID = numericOr(profile["user_id"], 0)
		token := randomToken()
		a.mu.Lock()
		for k, s := range a.members {
			if time.Now().After(s.Until) {
				delete(a.members, k)
			}
		}
		if len(a.members) >= 10000 {
			a.mu.Unlock()
			return problem(503, "会员会话容量已满")
		}
		a.members[token] = s
		a.mu.Unlock()
		jsonResponse(w, 200, M{"token": token, "profile": profile})
		return nil
	}
	s, e := a.memberSession(r, v.CMSBase, name == "plans")
	if e != nil {
		return e
	}
	out := M{}
	switch name {
	case "profile":
		profile, e := a.cachedProfile(r.Context(), s, p, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if e != nil {
			return e
		}
		out["profile"] = profile
	case "plans":
		result, e := a.memberCall(r.Context(), s, "api.php/user/ajax_upgrade_data", "GET", nil)
		if e != nil {
			return e
		}
		d := obj(result["data"])
		groups := []any{}
		keys := []string{"group_id", "group_name"}
		for _, period := range []string{"day", "week", "month", "year"} {
			keys = append(keys, "price_"+period, "group_points_"+period)
		}
		for i, g := range arr(d["groups"]) {
			if i >= 50 {
				break
			}
			groups = append(groups, pick(obj(g), keys...))
		}
		out = M{"isLogin": d["is_login"] == true || numericOr(d["is_login"], 0) == 1, "memberName": str(d["member_name"]), "expireDate": str(d["expire_date"]), "points": numericOr(d["user_points"], 0), "groups": groups}
		membership := applicationState(v).Config.Membership
		out["managed"] = membership.Managed
		if membership.Managed {
			out["pointsPlan"] = M{"groupId": membership.GroupID, "day": membership.Day, "week": membership.Week, "month": membership.Month, "year": membership.Year}
		}
	case "logout":
		a.mu.Lock()
		for k, session := range a.members {
			if session == s {
				delete(a.members, k)
			}
		}
		a.mu.Unlock()
		out["ok"] = true
	case "orders":
		page := numericOr(input["page"], 1)
		if page > 1000 {
			page = 1000
		}
		result, e := a.memberCall(r.Context(), s, "api.php/order/get_list", "GET", url.Values{"page": {strconv.Itoa(page)}, "limit": {"20"}})
		if e != nil {
			return e
		}
		orders := []any{}
		for i, item := range arr(obj(result["info"])["list"]) {
			if i >= 100 {
				break
			}
			orders = append(orders, safeOrder(item))
		}
		out["orders"] = orders
	case "order":
		code := str(input["code"])
		if !orderCode.MatchString(code) {
			return problem(400, "订单号无效")
		}
		result, e := a.memberCall(r.Context(), s, "api.php/order/check_status", "GET", url.Values{"order_code": {code}})
		if e != nil {
			return e
		}
		out["order"] = safeOrder(result["info"])
	case "purchase", "upgrade":
		if name == "upgrade" && applicationState(v).Config.Membership.Managed {
			period := str(input["period"])
			price := membershipPrice(v, period)
			if price <= 0 {
				return problem(400, "该积分套餐未开放")
			}
			connection, e := a.managementConnectionFor(v)
			if e != nil {
				return e
			}
			_, uid, e := a.applicationMember(r, v, p)
			if e != nil {
				return e
			}
			if e := a.applicationRateLimit("upgrade:"+managementBase(v.CMSBase)+":"+strconv.Itoa(uid), 5, time.Minute); e != nil {
				return e
			}
			if e := a.applicationSnapshotCurrent(v, p); e != nil {
				return e
			}
			_, e = a.managementCall(r.Context(), connection, "member.upgrade", M{"id": uid, "groupId": applicationState(v).Config.Membership.GroupID, "period": period, "points": price})
			if e != nil {
				return e
			}
			s.Mu.Lock()
			s.ProfileVersion.Add(1)
			s.Mu.Unlock()
			out["ok"] = true
			break
		}
		if name == "purchase" {
			if str(input["platform"]) == "ios" {
				return problem(400, "iOS 内购尚未配置")
			}
			if e = a.csrf(r.Context(), s); e != nil {
				return e
			}
		}
		group := number(input["groupId"])
		period := str(input["period"])
		if group < 3 || group > 1000000 || !has([]string{"day", "week", "month", "year"}, period) {
			return problem(400, "请选择 CMS 会员套餐")
		}
		endpoint := "api.php/user/upgrade_order_create"
		if name == "upgrade" {
			endpoint = "api.php/payment/upgrade"
		}
		result, e := a.memberCall(r.Context(), s, endpoint, "POST", url.Values{"group_id": {strconv.Itoa(group)}, "long": {period}})
		if e != nil {
			return e
		}
		if name == "upgrade" {
			out["ok"] = true
		} else {
			d := obj(result["data"])
			if len(d) == 0 {
				d = obj(result["info"])
			}
			out["order"] = safeOrder(d)
			out["payUrl"] = ""
			if link := str(d["pay_url"]); link != "" {
				base, _ := siteURL(v.CMSBase)
				u, e := url.Parse(link)
				if e == nil {
					u = base.ResolveReference(u)
					if u.Scheme == base.Scheme && u.Host == base.Host && u.User == nil {
						out["payUrl"] = u.String()
					}
				}
			}
		}
	case "recharge":
		if str(input["platform"]) == "ios" {
			return problem(400, "iOS 内购尚未配置")
		}
		price, e := strconv.ParseFloat(str(input["price"]), 64)
		if e != nil || math.IsNaN(price) || math.IsInf(price, 0) || price <= 0 || price > 100000 {
			return problem(400, "充值金额无效")
		}
		if e = a.csrf(r.Context(), s); e != nil {
			return e
		}
		result, e := a.memberCall(r.Context(), s, "api.php/order/create", "POST", url.Values{"price": {strconv.FormatFloat(price, 'f', 2, 64)}})
		if e != nil {
			return e
		}
		out["order"] = safeOrder(result["info"])
	case "checkout":
		if str(input["platform"]) == "ios" {
			return problem(400, "iOS 内购尚未配置")
		}
		id := number(input["orderId"])
		code := str(input["code"])
		payment := str(input["payment"])
		if id < 1 || !orderCode.MatchString(code) || !has(methods, payment) {
			return problem(400, "请选择有效订单和支付方式")
		}
		if e = a.csrf(r.Context(), s); e != nil {
			return e
		}
		result, e := a.memberCall(r.Context(), s, "api.php/payment/gopay", "POST", url.Values{"order_id": {strconv.Itoa(id)}, "order_code": {code}, "payment": {payment}})
		if e != nil {
			return e
		}
		out, e = normalizePayment(obj(obj(result["info"])["payment_data"]), v.CMSBase)
		if e != nil {
			return e
		}
		if out["kind"] == "form" {
			token := randomToken()
			a.mu.Lock()
			for k, c := range a.checkouts {
				if time.Now().After(c.Until) {
					delete(a.checkouts, k)
				}
			}
			if len(a.checkouts) >= 1000 {
				a.mu.Unlock()
				return problem(503, "收银台繁忙")
			}
			a.checkouts[token] = checkout{Data: out, Until: time.Now().Add(10 * time.Minute)}
			a.mu.Unlock()
			out = M{"kind": "form", "checkoutPath": "/api/checkout/" + token}
		}
	case "card":
		return problem(400, "卡密接口尚未验证，暂不开放")
	default:
		return problem(404, "会员接口不支持")
	}
	if r.Method == "POST" {
		s.ProfileVersion.Add(1)
	}
	jsonResponse(w, 200, out)
	return nil
}
