package main

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRegistrationChoosesEndpointBeforeSingleWrite(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		legacy                   bool
		probeStatus, writeStatus int
		writeBody                string
		wantWrites               int
		wantError                bool
	}{
		{"modern", false, 200, 200, "{\"code\":1}", 1, false},
		{"legacy", true, 200, 200, "{\"code\":1}", 1, false},
		{"probe failed", true, 503, 200, "{\"code\":1}", 0, true},
		{"registration not retried", false, 200, 404, "", 1, true},
		{"legacy not retried", true, 200, 500, "", 1, true},
		{"native checks preserved", true, 200, 200, "{\"code\":1002,\"msg\":\"验证码错误\"}", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if tc.legacy && r.URL.Path == "/sub/api.php/user/ajax_upgrade_data" {
						http.NotFound(w, r)
						return
					}
					if r.URL.Path != "/sub/api.php/user/ajax_upgrade_data" && r.URL.Path != "/sub/index.php/hetun/member_plans" {
						t.Errorf("unexpected probe %s", r.URL.Path)
					}
					w.WriteHeader(tc.probeStatus)
					_, _ = w.Write([]byte("{\"code\":1,\"data\":{}}"))
					return
				}
				writes++
				want := "/sub/api.php/user/register"
				if tc.legacy {
					want = "/sub/index.php/user/reg"
				}
				if r.URL.Path != want {
					t.Errorf("mutation sent to %s instead of %s", r.URL.Path, want)
				}
				_ = r.ParseForm()
				if r.Form.Get("user_name") != "alice" || r.Form.Get("user_pwd") != "correct" || r.Form.Get("user_pwd2") != "correct" || r.Form.Get("verify") != "native-captcha" {
					t.Error("native registration parameters lost")
				}
				if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
					t.Error("AJAX contract lost")
				}
				w.WriteHeader(tc.writeStatus)
				_, _ = w.Write([]byte(tc.writeBody))
			}))
			defer cms.Close()
			h := newHarness(t, "")
			jar, _ := cookiejar.New(nil)
			loggedIn, err := h.a.registerCMSMember(context.Background(), &memberSession{Base: cms.URL + "/sub/", Jar: jar}, url.Values{"user_name": {"alice"}, "user_pwd": {"correct"}, "user_pwd2": {"correct"}, "verify": {"native-captcha"}})
			if (err != nil) != tc.wantError || writes != tc.wantWrites {
				t.Fatalf("writes=%d err=%v", writes, err)
			}
			if err == nil && loggedIn != tc.legacy {
				t.Fatal("legacy login-session contract lost")
			}
			if tc.name == "native checks preserved" && !strings.Contains(err.Error(), "验证码错误") {
				t.Fatal(err)
			}
		})
	}
}

func TestLegacyRegistrationSessionAndOrderReads(t *testing.T) {
	writes, logins := 0, 0
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "api.php/") {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/index.php/hetun/member_plans":
			jsonResponse(w, 200, M{"code": 1, "data": M{"groups": []any{}, "member_name": "普通会员"}})
		case "/index.php/user/reg":
			writes++
			http.SetCookie(w, &http.Cookie{Name: "member", Value: "alice", Path: "/"})
			jsonResponse(w, 200, M{"code": 1})
		case "/index.php/user/login":
			logins++
			jsonResponse(w, 200, M{"code": 1002, "msg": "login captcha is required"})
		case "/index.php/hetun/member_info", "/index.php/hetun/member_orders", "/index.php/hetun/member_order", "/index.php/user/upgrade":
			if _, err := r.Cookie("member"); err != nil {
				jsonResponse(w, 200, M{"code": 1401, "msg": "请先登录"})
				return
			}
			if strings.HasSuffix(r.URL.Path, "member_info") {
				jsonResponse(w, 200, M{"code": 1, "info": M{"user_id": 7, "user_name": "alice"}})
				return
			}
			if strings.HasSuffix(r.URL.Path, "/upgrade") {
				if r.Method != "POST" || r.Form.Get("group_id") != "4" || r.Form.Get("long") != "month" {
					t.Error("native points-upgrade parameters lost")
				}
				jsonResponse(w, 200, M{"code": 1002, "msg": "积分不足"})
				return
			}
			order := M{"order_id": 3, "order_code": "A123", "order_status": 0, "user_id": 7, "user_pwd": "never-return"}
			if strings.HasSuffix(r.URL.Path, "member_orders") {
				if r.URL.Query().Get("limit") != "20" {
					t.Error("order limit lost")
				}
				jsonResponse(w, 200, M{"code": 1, "info": M{"list": []any{order}}})
				return
			}
			if r.URL.Query().Get("order_code") != "A123" {
				t.Error("order identity lost")
			}
			jsonResponse(w, 200, M{"code": 1, "info": order})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer cms.Close()
	initial := fixtureCMS(t)
	defer initial.Close()
	h := newHarness(t, initial.URL)
	if err := h.a.change(context.Background(), "fixture.legacy", func(s *state) error {
		s.CMSBase = cms.URL
		obj(s.Operations["mobile"].Published["features"])["member"] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	setApplicationFixture(t, h, func(c *applicationConfig) {
		c.Registration.Enabled = true
		c.Registration.Managed = false
		c.Captcha.Register = false
		c.Captcha.Login = false
	})
	w := h.req("POST", "/api/member/register", M{"username": "alice", "password": "correct", "password2": "correct"}, "")
	if w.Code != 200 || writes != 1 || logins != 0 {
		t.Fatalf("status=%d writes=%d repeatedLogins=%d body=%s", w.Code, writes, logins, w.Body)
	}
	token := str(payload(t, w)["token"])
	if token == "" {
		t.Fatal("no member session")
	}
	for _, route := range []string{"/api/member/orders?page=2", "/api/member/order?code=A123"} {
		w = h.req("GET", route, nil, token)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "A123") || strings.Contains(w.Body.String(), "user_pwd") || strings.Contains(w.Body.String(), "user_id") {
			t.Fatalf("order response %d %s", w.Code, w.Body)
		}
	}
	w = h.req("POST", "/api/member/upgrade", M{"groupId": 4, "period": "month"}, token)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "积分不足") {
		t.Fatalf("native authorization not preserved: %d %s", w.Code, w.Body)
	}
}
