package main

import (
	"net/http"
	"sort"
	"sync"
	"time"
)

// Bounded process-period counters: only known public client endpoints are counted.
// No user identifiers, URLs, credentials or unbounded request labels are retained.
type clientTelemetry struct {
	mu      sync.Mutex
	clients map[string]map[string]*endpointMetric
}
type endpointMetric struct {
	Total     int64   `json:"total"`
	Failed    int64   `json:"failed"`
	ElapsedMS float64 `json:"-"`
}

func publicMetricScope(r *http.Request) (string, string) {
	if r.Method == http.MethodOptions {
		return "", ""
	}
	normalized, err := normalizeClientRoute(r)
	if err != nil {
		return "", ""
	}
	p, err := requestPlatform(normalized)
	if err != nil {
		return "", ""
	}
	if normalized.URL.Path == "/api/bootstrap" || normalized.URL.Path == "/api/runtime" {
		return p, "bootstrap"
	}
	for endpoint, path := range publicClientRoutes {
		if normalized.URL.Path == path {
			return p, endpoint
		}
	}
	return "", ""
}
func (m *clientTelemetry) record(p, endpoint string, status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.clients == nil {
		m.clients = map[string]map[string]*endpointMetric{}
	}
	if m.clients[p] == nil {
		m.clients[p] = map[string]*endpointMetric{}
	}
	metric := m.clients[p][endpoint]
	if metric == nil {
		metric = &endpointMetric{}
		m.clients[p][endpoint] = metric
	}
	metric.Total++
	if status >= 400 {
		metric.Failed++
	}
	metric.ElapsedMS += float64(duration.Microseconds()) / 1000
}
func (m *clientTelemetry) snapshot(p string) M {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total, failed int64
	var elapsed float64
	rows := []M{}
	keys := []string{}
	for key := range m.clients[p] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		metric := m.clients[p][key]
		total += metric.Total
		failed += metric.Failed
		elapsed += metric.ElapsedMS
		rows = append(rows, M{"endpoint": key, "total": metric.Total, "failed": metric.Failed, "averageMs": metric.ElapsedMS / float64(metric.Total)})
	}
	average := float64(0)
	if total > 0 {
		average = elapsed / float64(total)
	}
	return M{"total": total, "failed": failed, "successful": total - failed, "averageMs": average, "endpoints": rows}
}
func (a *app) clientStatistics(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return problem(405, "仅支持 GET")
	}
	p, err := requestPlatform(r)
	if err != nil {
		return err
	}
	v := a.readState()
	ads := readAds(v, p)
	metrics := adMetrics{}
	for _, day := range ads.Days {
		if day != nil {
			metrics.add(*day)
		}
	}
	releases := 0
	for _, release := range v.AppReleases {
		if release.Platform == p {
			releases++
		}
	}
	version := 0
	publishedAt := ""
	if record := v.Operations[p]; record != nil {
		version = record.PublishedVersion
		publishedAt = record.PublishedAt
	}
	jsonResponse(w, 200, M{"platform": p, "client": clientName(p), "checkedAt": now(), "startedAt": a.startedAt.UTC().Format(time.RFC3339), "scope": "process", "requests": a.clientMetrics.snapshot(p), "business": M{"publishedVersion": version, "publishedAt": publishedAt, "releases": releases, "advertisingVersion": ads.PublishedVersion, "advertising": metrics}})
	return nil
}
