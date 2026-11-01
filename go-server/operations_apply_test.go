package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type applyFailStore struct{ store }

func (s *applyFailStore) Save(context.Context, state, int64) error {
	return errors.New("injected durable store failure")
}

func TestApplyAtomicPersistenceAndClientIsolation(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	before := h.a.readState()
	w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 1, "theme": shortcutDraft()}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	after := h.a.readState()
	r := after.Operations["mobile"]
	if r.Version != 2 || r.PublishedVersion != 2 || len(r.History) != 1 || !reflect.DeepEqual(r.Draft, r.Published) {
		t.Fatal("non-atomic apply", r)
	}
	if !reflect.DeepEqual(before.Operations["tablet"], after.Operations["tablet"]) {
		t.Fatal("changed another client")
	}
	stored, err := h.a.store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.Operations["mobile"], r) {
		t.Fatal("durable state differs")
	}
	restarted, err := newApp(context.Background(), h.a.config, h.a.store)
	if err != nil {
		t.Fatal(err)
	}
	out, err := bootstrap(restarted.readState(), "mobile", "android", "", h.origin)
	if err != nil {
		t.Fatal(err)
	}
	if out["version"] != 2 || out["pollSeconds"] != 15 || !reflect.DeepEqual(out["config"], r.Published) {
		t.Fatal("bootstrap does not expose applied data", out)
	}
	response := payload(t, w)
	if number(response["version"]) != 2 || number(response["publishedVersion"]) != 2 {
		t.Fatal(response)
	}
}

func TestApplyRejectsWithoutChangingLastGoodState(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	if w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 1, "theme": shortcutDraft()}, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	before := h.a.readState()
	invalid := shortcutDraft()
	obj(arr(invalid["categoryShortcuts"])[0])["columns"] = 7
	for _, tc := range []struct {
		name, path string
		input      M
		status     int
	}{
		{"stale", "/api/admin/clients/mobile/apply", M{"version": 1, "theme": shortcutDraft()}, 409},
		{"invalid", "/api/admin/clients/mobile/apply", M{"version": 2, "theme": invalid}, 400},
		{"wrong client", "/api/admin/clients/mobile/apply", M{"version": 2, "theme": shortcutDraft(), "template": "tv"}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := h.req("PUT", tc.path, tc.input, "")
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body)
			}
			if !reflect.DeepEqual(before, h.a.readState()) {
				t.Fatal("rejected request changed state")
			}
		})
	}
	original := h.a.store
	h.a.store = &applyFailStore{original}
	w := h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 2, "theme": shortcutDraft()}, "")
	h.a.store = original
	if w.Code < 500 || !reflect.DeepEqual(before, h.a.readState()) {
		t.Fatal("failed store changed active state", w.Code, w.Body)
	}
	stored, err := original.Load(context.Background())
	if err != nil || !reflect.DeepEqual(stored, before) {
		t.Fatal("last durable configuration changed", err)
	}
	cookie := h.cookie
	h.cookie = ""
	w = h.req("PUT", "/api/admin/clients/mobile/apply", M{"version": 2, "theme": shortcutDraft()}, "")
	h.cookie = cookie
	if w.Code != 401 {
		t.Fatal("unauthenticated apply accepted", w.Code)
	}
}

func TestApplyHistoryCapAndRollback(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, cms.URL)
	for i := 1; i <= 22; i++ {
		w := h.req("PUT", "/api/admin/clients/desktop/apply", M{"version": i, "theme": shortcutDraft()}, "")
		if w.Code != 200 {
			t.Fatal(i, w.Code, w.Body)
		}
	}
	before := h.a.readState().Operations["web"]
	if len(before.History) != 20 || before.Version != 23 || before.PublishedVersion != 23 {
		t.Fatal("history boundary", before)
	}
	target := before.History[3]
	w := h.req("POST", "/api/admin/clients/desktop/rollback", M{"version": 23, "targetVersion": target.Version}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	after := h.a.readState().Operations["web"]
	if after.Version != 24 || after.PublishedVersion != 24 || !reflect.DeepEqual(after.Published, target.Config) {
		t.Fatal("rollback did not create new applied version", after)
	}
}
