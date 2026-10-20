package main

import (
	"net/http"
	"sync"
	"time"
)

// Aggregates API requests since process start. System polling and preflight are
// excluded so opening this dashboard does not inflate its own business counters.
type requestMonitor struct {
	mu                          sync.Mutex
	total, failed, active, peak int64
	elapsed                     time.Duration
}
type monitoredResponse struct {
	http.ResponseWriter
	status int
}

func (w *monitoredResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *monitoredResponse) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *monitoredResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (m *requestMonitor) begin(w http.ResponseWriter) http.ResponseWriter {
	m.mu.Lock()
	m.active++
	if m.active > m.peak {
		m.peak = m.active
	}
	m.mu.Unlock()
	return &monitoredResponse{ResponseWriter: w}
}
func (m *requestMonitor) finish(w *monitoredResponse, started time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active--
	m.total++
	m.elapsed += time.Since(started)
	if w.status >= 400 {
		m.failed++
	}
}
func (m *requestMonitor) snapshot() M {
	m.mu.Lock()
	defer m.mu.Unlock()
	average := float64(0)
	if m.total > 0 {
		average = float64(m.elapsed.Microseconds()) / 1000 / float64(m.total)
	}
	return M{"total": m.total, "failed": m.failed, "active": m.active, "peak": m.peak, "averageMs": average}
}
