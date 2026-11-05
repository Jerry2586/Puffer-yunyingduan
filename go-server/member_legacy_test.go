package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLegacyCMSMemberReadAndLogin(t *testing.T) {
	calls := map[string]int{}
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sub/api.php/user/login", "/sub/api.php/user/get_info", "/sub/api.php/user/ajax_upgrade_data":
			http.NotFound(w, r)
		case "/sub/index.php/user/login":
			_ = r.ParseForm()
			if r.Method != "POST" || r.Form.Get("user_name") != "alice" || r.Form.Get("user_pwd") != "correct" {
				t.Error("legacy login lost method or credentials")
			}
			http.SetCookie(w, &http.Cookie{Name: "member", Value: "alice", Path: "/sub/"})
			jsonResponse(w, 200, M{"code": 1})
		case "/sub/index.php/hetun/member_info":
			if _, err := r.Cookie("member"); err != nil {
				jsonResponse(w, 200, M{"code": 1401, "msg": "请先登录"})
				return
			}
			jsonResponse(w, 200, M{"code": 1, "info": M{"user_id": 7, "user_name": "alice", "user_points": 123, "user_pwd": "must-not-leak"}})
		case "/sub/index.php/hetun/member_plans":
			_, err := r.Cookie("member")
			jsonResponse(w, 200, M{"code": 1, "data": M{"is_login": err == nil, "member_name": "普通会员", "expire_date": "", "groups": []any{M{"group_id": 4, "group_points_month": 100}}}})
		default:
			t.Errorf("unexpected legacy route: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer cms.Close()
	h := newHarness(t, "")
	newSession := func() *memberSession {
		jar, _ := cookiejar.New(nil)
		return &memberSession{Base: cms.URL + "/sub/", Jar: jar}
	}
	s := newSession()
	plans, err := h.a.memberCall(context.Background(), s, "api.php/user/ajax_upgrade_data", "GET", nil)
	if err != nil || obj(plans["data"])["is_login"] != false {
		t.Fatalf("guest plans: %v %v", plans, err)
	}
	if _, err = h.a.memberCall(context.Background(), s, "api.php/user/login", "POST", url.Values{"user_name": {"alice"}, "user_pwd": {"correct"}}); err != nil {
		t.Fatal(err)
	}
	profile, err := h.a.profile(context.Background(), s)
	if err != nil || numericOr(profile["user_id"], 0) != 7 || profile["member_name"] != "普通会员" {
		t.Fatalf("profile: %v %v", profile, err)
	}
	if _, ok := profile["user_pwd"]; ok {
		t.Fatal("sensitive profile field leaked")
	}
	if _, err = h.a.profile(context.Background(), newSession()); err == nil {
		t.Fatal("member cookie leaked into another session")
	}
	if calls["/sub/index.php/user/login"] != 1 || calls["/sub/index.php/hetun/member_info"] != 2 {
		t.Fatalf("unexpected requests: %v", calls)
	}
}

func TestLegacyMemberFallbackOnlyForMissingKnownEndpoints(t *testing.T) {
	cases := []struct {
		name, route, body string
		status, wantCalls int
		wantError         bool
	}{
		{"http forbidden", "api.php/user/get_info", "", 403, 1, true},
		{"server error", "api.php/user/get_info", "", 500, 1, true},
		{"redirect", "api.php/user/get_info", "", 302, 1, true},
		{"invalid json", "api.php/user/get_info", "<html>blocked</html>", 200, 1, true},
		{"login rejected", "api.php/user/login", "{\"code\":0,\"msg\":\"密码错误\"}", 200, 1, true},
		{"session rejected", "api.php/user/get_info", "{\"code\":1401,\"msg\":\"请先登录\"}", 200, 1, true},
		{"modern success", "api.php/user/get_info", "{\"code\":1,\"info\":{}}", 200, 1, false},
		{"payment never retried", "api.php/payment/upgrade", "", 404, 1, true},
		{"registration never retried", "api.php/user/register", "", 404, 1, true},
		{"missing adapter", "api.php/user/get_info", "", 404, 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer cms.Close()
			h := newHarness(t, "")
			jar, _ := cookiejar.New(nil)
			_, err := h.a.memberCall(context.Background(), &memberSession{Base: cms.URL, Jar: jar}, tc.route, "GET", nil)
			if (err != nil) != tc.wantError || calls != tc.wantCalls {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if tc.status == 404 {
				var api *apiError
				if !errors.As(err, &api) || api.Code != "CMS_HTTP_404" {
					t.Fatalf("HTTP error lost its identity: %v", err)
				}
			}
			if tc.name == "session rejected" && !strings.Contains(err.Error(), "请先登录") {
				t.Fatal(err)
			}
		})
	}
}
