package main

import (
	"context"
	"testing"
)

func TestNavigationTextColorPersistenceAndReset(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	for _, key := range []string{"categories", "navigation", "serviceMenuItems"} {
		t.Run(key, func(t *testing.T) {
			h := newHarness(t, cms.URL)
			draft := pageDraft()
			rows := []any{
				M{"id": "first-color", "label": "第一个入口", "page": "home", "enabled": true, "textColor": "#Ff6B35"},
				M{"id": "second-color", "label": "第二个入口", "page": "home", "enabled": true},
			}
			draft[key] = rows
			w := h.req("PUT", "/api/admin/draft", M{"version": 1, "theme": draft, "template": "runtime-mobile"}, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
			w = h.req("POST", "/api/admin/publish", M{"version": 2, "template": "runtime-mobile"}, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
			restarted, err := newApp(context.Background(), h.a.config, h.a.store)
			if err != nil {
				t.Fatal(err)
			}
			state := restarted.readState()
			for _, cfg := range []M{state.Operations["mobile"].Draft, state.Operations["mobile"].Published} {
				saved := arr(cfg[key])
				if obj(saved[0])["textColor"] != "#Ff6B35" {
					t.Fatal("text color lost after restart")
				}
				if _, exists := obj(saved[1])["textColor"]; exists {
					t.Fatal("color leaked to another entry")
				}
			}
			response, err := bootstrap(state, "mobile", "web", "", h.origin)
			if err != nil {
				t.Fatal(err)
			}
			if obj(arr(obj(response["config"])[key])[0])["textColor"] != "#Ff6B35" {
				t.Fatal("client color missing")
			}
			delete(obj(rows[0]), "textColor")
			w = h.req("PUT", "/api/admin/draft", M{"version": 3, "theme": draft, "template": "runtime-mobile"}, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
			w = h.req("POST", "/api/admin/publish", M{"version": 4, "template": "runtime-mobile"}, "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body)
			}
			restarted, err = newApp(context.Background(), h.a.config, h.a.store)
			if err != nil {
				t.Fatal(err)
			}
			state = restarted.readState()
			for _, cfg := range []M{state.Operations["mobile"].Draft, state.Operations["mobile"].Published} {
				if _, exists := obj(arr(cfg[key])[0])["textColor"]; exists {
					t.Fatal("reset color reappeared after save and restart")
				}
			}
			response, err = bootstrap(state, "mobile", "web", "", h.origin)
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := obj(arr(obj(response["config"])[key])[0])["textColor"]; exists {
				t.Fatal("reset color reappeared in client configuration")
			}
		})
	}
}

func TestNavigationTextColorValidation(t *testing.T) {
	for _, key := range []string{"categories", "navigation", "serviceMenuItems"} {
		t.Run(key, func(t *testing.T) {
			for _, value := range []any{"", "red", "#abc", "#11223344", "url(https://example.test)", "#12GG00", float64(123), nil} {
				draft := pageDraft()
				draft[key] = []any{M{"id": "color-check", "label": "入口", "page": "home", "enabled": true, "textColor": value}}
				if _, err := operationConfig(draft); err == nil {
					t.Fatalf("invalid text color accepted: %v", value)
				}
			}
			for _, value := range []string{"#000000", "#FFFFFF", "#aB1234"} {
				draft := pageDraft()
				draft[key] = []any{M{"id": "color-check", "label": "入口", "page": "home", "enabled": true, "textColor": value}}
				if _, err := operationConfig(draft); err != nil {
					t.Fatal("valid color rejected", err)
				}
			}
		})
	}
}
