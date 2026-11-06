package main

import (
	"context"
	"reflect"
	"testing"
)

func TestScheduleApplyEditRestartAndDelete(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	before := h.a.readState()
	draft := clone(defaults)
	draft["schedule"] = []any{M{"id": "schedule-check", "title": "初始排期", "date": "2026-11-07", "vodId": "1", "categoryId": "1"}}
	apply := func(version int) {
		t.Helper()
		w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": version, "theme": draft}, "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	apply(1)
	row := obj(arr(draft["schedule"])[0])
	row["title"], row["date"] = "已编辑排期", "2026-11-08"
	apply(2)
	restarted, err := newApp(context.Background(), h.a.config, h.a.store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := bootstrap(restarted.readState(), "mobile", "android", "", h.origin)
	if err != nil {
		t.Fatal(err)
	}
	actual := arr(obj(result["config"])["schedule"])
	if len(actual) != 1 || !reflect.DeepEqual(obj(actual[0]), row) {
		t.Fatal("saved schedule not delivered after restart", actual)
	}
	if !reflect.DeepEqual(before.Operations["tablet"], h.a.readState().Operations["tablet"]) {
		t.Fatal("changed another client")
	}
	saved := h.a.readState()
	row["date"] = "2026-02-30"
	w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 3, "theme": draft}, "")
	if w.Code != 400 || !reflect.DeepEqual(saved, h.a.readState()) {
		t.Fatal("invalid date changed persisted state", w.Code, w.Body)
	}
	draft["schedule"] = []any{}
	apply(3)
	result, err = bootstrap(h.a.readState(), "mobile", "android", "", h.origin)
	if err != nil || len(arr(obj(result["config"])["schedule"])) != 0 {
		t.Fatal("deleted schedule still delivered", result, err)
	}
}
