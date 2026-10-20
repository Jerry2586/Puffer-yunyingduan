package main

import (
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestClientStatisticsIsolationAuthAndAliases(t *testing.T) {
	h := systemHarness(t)
	for index, p := range platforms {
		for i := 0; i <= index; i++ {
			h.req("GET", "/api/v1/"+clientName(p)+"/member/config", nil, "")
		}
		w := h.req("GET", "/api/admin/clients/"+clientName(p)+"/statistics", nil, "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		data := payload(t, w)
		if data["platform"] != p || obj(data["requests"])["total"] != float64(index+1) {
			t.Fatal(data)
		}
	}
	h.req("GET", "/api/catalog?platform=mobile", nil, "") // unconfigured source is a real failure
	h.req("GET", "/api/v1/mobile/member/config?platform=tv", nil, "")
	h.req("OPTIONS", "/api/v1/mobile/member/config", nil, "")
	h.req("GET", "/api/v1/mobile/not-real", nil, "")
	w := h.req("GET", "/api/admin/clients/mobile/statistics", nil, "")
	data := payload(t, w)
	requests := obj(data["requests"])
	if requests["total"] != float64(2) || requests["failed"] != float64(1) {
		t.Fatal(data)
	}
	if w = h.req("POST", "/api/admin/clients/mobile/statistics", M{}, ""); w.Code != 405 {
		t.Fatal(w.Code, w.Body)
	}
	h.cookie = ""
	if w = h.req("GET", "/api/admin/clients/mobile/statistics", nil, ""); w.Code != 401 {
		t.Fatal(w.Code, w.Body)
	}
}
func TestClientTelemetryConcurrentBoundedAndIndependentSnapshots(t *testing.T) {
	var m clientTelemetry
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.record("tv", "catalog", 200, time.Millisecond) }()
	}
	wg.Wait()
	data := m.snapshot("tv")
	if data["total"] != int64(100) || data["averageMs"] != float64(1) {
		t.Fatal(data)
	}
	obj(data["endpoints"].([]M)[0])["total"] = int64(0)
	if m.snapshot("tv")["total"] != int64(100) || m.snapshot("tablet")["total"] != int64(0) {
		t.Fatal("snapshot mutated telemetry")
	}
	for _, path := range []string{"/api/admin/clients/tv/statistics", "/api/v1/tv/unknown", "/api/v1/tv/catalog?platform=mobile"} {
		if p, _ := publicMetricScope(httptest.NewRequest("GET", path, nil)); p != "" {
			t.Fatal(path)
		}
	}
}
