package main

import (
	"context"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func communityFixture(t *testing.T) (*harness, string) {
	t.Helper()
	cms := fixtureCMS(t)
	t.Cleanup(cms.Close)
	h := newHarness(t, cms.URL)
	if err := h.a.change(context.Background(), "fixture.member", func(s *state) error {
		for _, p := range platforms {
			obj(s.Operations[p].Published["features"])["member"] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	w := h.req("POST", "/api/member/login", M{"username": "alice", "password": "correct"}, "")
	if w.Code != 200 {
		t.Fatalf("member login status %d", w.Code)
	}
	return h, str(payload(t, w)["token"])
}
func communitySubmit(t *testing.T, h *harness, token, kind string, data M) M {
	t.Helper()
	w := h.req("POST", "/api/community/"+kind, data, token)
	if w.Code != 201 {
		t.Fatalf("%s submit status %d", kind, w.Code)
	}
	return payload(t, w)
}
func communityAction(t *testing.T, h *harness, kind, action string, records ...M) *httptest.ResponseRecorder {
	t.Helper()
	ids := []any{}
	versions := M{}
	for _, record := range records {
		ids = append(ids, record["id"])
		versions[strconv.Itoa(number(record["id"]))] = record["version"]
	}
	return h.req("POST", "/api/admin/manage/"+kind+"/action", M{"source": managementBase(h.a.readState().CMSBase), "action": action, "ids": ids, "versions": versions}, "")
}
func TestCommunityModerationIdentityPrivacyAndPersistence(t *testing.T) {
	h, token := communityFixture(t)
	created := communitySubmit(t, h, token, "comments", M{"videoId": "1", "content": "待审核评论", "userId": 999, "nickname": "伪造身份", "videoName": "伪造标题"})
	if number(created["userId"]) != 7 || str(created["nickname"]) != "alice" || str(created["videoName"]) != "一" || created["approved"] != false {
		t.Fatal("unverified identity or default approval accepted")
	}
	for _, key := range []string{"ip", "base", "reporters"} {
		if _, exists := created[key]; exists {
			t.Fatal("private field in public response")
		}
	}
	public := h.req("GET", "/api/community/comments?videoId=1", nil, "")
	if public.Code != 200 || number(payload(t, public)["total"]) != 0 {
		t.Fatal("pending comment publicly visible")
	}
	mine := h.req("GET", "/api/community/comments?mine=1", nil, token)
	if mine.Code != 200 || number(payload(t, mine)["total"]) != 1 {
		t.Fatal("member cannot see own pending comment")
	}
	admin := h.req("GET", "/api/admin/manage/comments", nil, "")
	if admin.Code != 200 || str(obj(arr(payload(t, admin)["items"])[0])["ip"]) == "" {
		t.Fatal("moderator IP missing")
	}
	if w := communityAction(t, h, "comments", "approve", created); w.Code != 200 {
		t.Fatalf("approve status %d", w.Code)
	}
	for _, client := range []string{"mobile", "tablet", "tv", "desktop"} {
		w := h.req("GET", "/api/v1/"+client+"/community/comments?videoId=1", nil, "")
		if w.Code != 200 || number(payload(t, w)["total"]) != 1 {
			t.Fatal("approved comment missing from client alias")
		}
		item := obj(arr(payload(t, w)["items"])[0])
		for _, key := range []string{"ip", "base", "reporters"} {
			if _, exists := item[key]; exists {
				t.Fatal("private data in client alias")
			}
		}
	}
	stored, err := h.a.store.Load(context.Background())
	if err != nil || len(stored.Community.Records) != 1 || !stored.Community.Records[0].Approved || stored.Community.Records[0].Version != 2 {
		t.Fatal("moderation not durably saved")
	}
	if w := h.req("GET", "/api/community/comments?mine=1", nil, ""); w.Code != 401 {
		t.Fatal("anonymous own-list request accepted")
	}
	if w := h.req("POST", "/api/community/comments", M{"videoId": "1", "content": "匿名"}, ""); w.Code != 401 {
		t.Fatal("anonymous submission accepted")
	}
}
func TestCommunityBulkConflictsAreAtomic(t *testing.T) {
	h, token := communityFixture(t)
	first := communitySubmit(t, h, token, "comments", M{"videoId": "1", "content": "第一条"})
	second := communitySubmit(t, h, token, "comments", M{"videoId": "1", "content": "第二条"})
	if w := communityAction(t, h, "comments", "approve", second); w.Code != 200 {
		t.Fatal("initial approval failed")
	}
	before := h.a.readState().Community
	w := communityAction(t, h, "comments", "delete", first, second)
	if w.Code != 409 || str(payload(t, w)["code"]) != "MANAGEMENT_RECORD_CHANGED" || !reflect.DeepEqual(before, h.a.readState().Community) {
		t.Fatal("stale bulk request partially mutated records")
	}
	missing := M{"id": 999, "version": 1}
	if w := communityAction(t, h, "comments", "approve", first, missing); w.Code != 409 || !reflect.DeepEqual(before, h.a.readState().Community) {
		t.Fatal("missing bulk record partially approved records")
	}
	h.a.store = &applyFailStore{h.a.store}
	if w := communityAction(t, h, "comments", "approve", first); w.Code == 200 || !reflect.DeepEqual(before, h.a.readState().Community) {
		t.Fatal("failed durable save changed live records")
	}
}
func TestCommunitySourceIsolationAndFeatureGate(t *testing.T) {
	h, token := communityFixture(t)
	item := communitySubmit(t, h, token, "comments", M{"videoId": "1", "content": "旧内容源"})
	oldSource := managementBase(h.a.readState().CMSBase)
	if err := h.a.change(context.Background(), "fixture.source", func(s *state) error { s.CMSBase = "https://different.test"; return nil }); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/api/community/comments?videoId=1", "/api/admin/manage/comments"} {
		w := h.req("GET", route, nil, "")
		if w.Code != 200 || number(payload(t, w)["total"]) != 0 {
			t.Fatal("old source records exposed after source switch")
		}
	}
	w := h.req("POST", "/api/admin/manage/comments/action", M{"source": oldSource, "action": "delete", "ids": []any{item["id"]}, "versions": M{strconv.Itoa(number(item["id"])): item["version"]}}, "")
	if w.Code != 409 || len(h.a.readState().Community.Records) != 1 {
		t.Fatal("stale-source admin write accepted")
	}
	if err := h.a.change(context.Background(), "fixture.gate", func(s *state) error {
		s.CMSBase = oldSource
		obj(s.Operations["mobile"].Published["features"])["member"] = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if w := h.req("GET", "/api/v1/mobile/community/comments?videoId=1", nil, ""); w.Code != 403 {
		t.Fatal("disabled client feature accessible")
	}
	if w := h.req("POST", "/api/v1/mobile/community/feedback", M{"content": "不能提交"}, token); w.Code != 403 {
		t.Fatal("disabled feature accepted submission")
	}
	if w := h.req("GET", "/api/v1/tablet/community/comments?videoId=1", nil, ""); w.Code != 200 {
		t.Fatal("mobile switch disabled independent tablet feature")
	}
}
func TestCommunityPolicyConflictsAndBlacklists(t *testing.T) {
	h, token := communityFixture(t)
	base := managementBase(h.a.readState().CMSBase)
	data := M{"source": base, "version": 0, "reviewComments": false, "reviewDanmu": true, "keywords": []any{"blocked", "blocked"}, "ips": []any{"2001:0db8:0:0:0:0:0:1", "2001:db8::1"}}
	w := h.req("POST", "/api/admin/manage/moderation", data, "")
	out := payload(t, w)
	if w.Code != 200 || number(out["version"]) != 1 || len(arr(out["keywords"])) != 1 || len(arr(out["ips"])) != 1 || str(arr(out["ips"])[0]) != "2001:db8::1" {
		t.Fatal("policy validation or IP canonicalization failed")
	}
	if w := h.req("POST", "/api/admin/manage/moderation", data, ""); w.Code != 409 {
		t.Fatal("stale policy replaced current rules")
	}
	if w := h.req("POST", "/api/community/feedback", M{"content": "BLOCKED phrase"}, token); w.Code != 400 {
		t.Fatal("case-insensitive keyword blacklist bypassed")
	}
	comment := communitySubmit(t, h, token, "comments", M{"content": "无需审核", "videoId": "1"})
	if comment["approved"] != true {
		t.Fatal("comment review toggle ignored")
	}
	danmu := communitySubmit(t, h, token, "danmu", M{"content": "待审弹幕", "videoId": "1", "position": 1200})
	if danmu["approved"] != false || number(danmu["position"]) != 1200 {
		t.Fatal("danmu review toggle or position ignored")
	}
	data["version"] = 1
	data["ips"] = []any{"192.0.2.1"}
	if w := h.req("POST", "/api/admin/manage/moderation", data, ""); w.Code != 200 {
		t.Fatal("IP policy save failed")
	}
	if w := h.req("POST", "/api/community/feedback", M{"content": "被封禁地址"}, token); w.Code != 403 {
		t.Fatal("blocked remote address accepted")
	}
	data["version"] = 2
	data["ips"] = []any{"invalid-address"}
	if w := h.req("POST", "/api/admin/manage/moderation", data, ""); w.Code != 400 {
		t.Fatal("invalid blacklist accepted")
	}
}
func TestCommunityPrivateQueuesAndReplies(t *testing.T) {
	h, token := communityFixture(t)
	for _, kind := range []string{"requests", "feedback"} {
		created := communitySubmit(t, h, token, kind, M{"content": "请求内容"})
		if err := h.a.change(context.Background(), "fixture.other-user", func(s *state) error {
			c := communityStateFor(s)
			copy := c.Records[len(c.Records)-1]
			copy.ID = c.NextID
			c.NextID++
			copy.UserID = 8
			copy.Content = "其他会员私有内容"
			c.Records = append(c.Records, copy)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		w := h.req("GET", "/api/community/"+kind, nil, token)
		if w.Code != 200 || number(payload(t, w)["total"]) != 1 || strings.Contains(w.Body.String(), "其他会员私有内容") {
			t.Fatal("another member's private queue exposed")
		}
		w = h.req("POST", "/api/admin/manage/"+kind+"/action", M{"source": managementBase(h.a.readState().CMSBase), "action": "reply", "ids": []any{created["id"]}, "versions": M{strconv.Itoa(number(created["id"])): created["version"]}, "reply": "已处理"}, "")
		if w.Code != 200 {
			t.Fatalf("reply status %d", w.Code)
		}
		w = h.req("GET", "/api/community/"+kind, nil, token)
		if str(obj(arr(payload(t, w)["items"])[0])["reply"]) != "已处理" {
			t.Fatal("member did not receive moderator reply")
		}
	}
}
func TestCommunityReportsRemindersAndLimits(t *testing.T) {
	h, token := communityFixture(t)
	comment := communitySubmit(t, h, token, "comments", M{"videoId": "1", "content": "评论"})
	if w := communityAction(t, h, "comments", "approve", comment); w.Code != 200 {
		t.Fatal("approve failed")
	}
	for i := 0; i < 2; i++ {
		w := h.req("POST", "/api/community/comments/report", M{"id": comment["id"]}, token)
		expected := 200
		if i == 1 {
			expected = 409
		}
		if w.Code != expected {
			t.Fatalf("report attempt status %d", w.Code)
		}
	}
	if h.a.readState().Community.Records[0].Reports != 1 {
		t.Fatal("duplicate reporter counted twice")
	}
	reminder := communitySubmit(t, h, token, "reminders", M{"videoId": "1"})
	if w := h.req("POST", "/api/community/reminders", M{"videoId": "1"}, token); w.Code != 429 {
		t.Fatal("reminder cooldown bypassed")
	}
	if err := h.a.change(context.Background(), "fixture.age", func(s *state) error {
		for i := range s.Community.Records {
			if s.Community.Records[i].ID == number(reminder["id"]) {
				s.Community.Records[i].UpdatedAt = time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reminder = communitySubmit(t, h, token, "reminders", M{"videoId": "1"})
	if number(reminder["count"]) != 2 || number(reminder["version"]) != 2 {
		t.Fatal("repeat reminder did not update existing record")
	}
	if w := h.req("POST", "/api/community/reminders", M{"videoId": "1"}, token); w.Code != 429 {
		t.Fatal("repeat reminder did not renew cooldown")
	}
	for _, data := range []M{{"videoId": "999", "content": "missing"}, {"videoId": "1", "content": "bad position", "position": 1.5}, {"videoId": "1", "content": strings.Repeat("弹", 101)}} {
		w := h.req("POST", "/api/community/danmu", data, token)
		if w.Code != 400 && w.Code != 404 {
			t.Fatal("invalid video, position or content accepted")
		}
	}
}
func TestCommunityPerMemberRateLimit(t *testing.T) {
	h, token := communityFixture(t)
	for i := 0; i < 10; i++ {
		communitySubmit(t, h, token, "feedback", M{"content": "意见"})
	}
	if w := h.req("POST", "/api/community/feedback", M{"content": "太频繁"}, token); w.Code != 429 || len(h.a.readState().Community.Records) != 10 {
		t.Fatal("member submission limit bypassed")
	}
}
