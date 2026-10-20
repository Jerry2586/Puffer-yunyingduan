package main

import (
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

type outboundMonitor struct {
	settings                                httpSettings
	mu                                      sync.Mutex
	active, total, failed, reused, acquired int64
	hosts                                   map[string]int
	elapsed                                 time.Duration
}
type trackedTransport struct {
	base    http.RoundTripper
	metrics *outboundMonitor
}
type trackedBody struct {
	io.ReadCloser
	once sync.Once
	done func()
}

func (b *trackedBody) Close() error { err := b.ReadCloser.Close(); b.once.Do(b.done); return err }
func (t *trackedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m := t.metrics
	host := req.URL.Host
	start := time.Now()
	m.mu.Lock()
	if m.hosts == nil {
		m.hosts = map[string]int{}
	}
	m.active++
	m.hosts[host]++
	m.mu.Unlock()
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		m.mu.Lock()
		m.acquired++
		if info.Reused {
			m.reused++
		}
		m.mu.Unlock()
	}}
	res, err := t.base.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	done := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.active--
		m.hosts[host]--
		if m.hosts[host] == 0 {
			delete(m.hosts, host)
		}
		m.total++
		m.elapsed += time.Since(start)
		if err != nil || res.StatusCode >= 400 {
			m.failed++
		}
	}
	if err != nil {
		done()
		return res, err
	}
	res.Body = &trackedBody{ReadCloser: res.Body, done: done}
	return res, nil
}
func (m *outboundMonitor) snapshot() M {
	m.mu.Lock()
	defer m.mu.Unlock()
	busiest := "—"
	current := 0
	for host, n := range m.hosts {
		if n > current {
			current = n
			busiest = host
		}
	}
	average, reuse := float64(0), float64(0)
	if m.total > 0 {
		average = float64(m.elapsed.Microseconds()) / 1000 / float64(m.total)
	}
	if m.acquired > 0 {
		reuse = float64(m.reused) * 100 / float64(m.acquired)
	}
	s := m.settings
	if s.TimeoutSeconds == 0 {
		s = effectiveHTTP(nil)
	}
	return M{"active": m.active, "total": m.total, "failed": m.failed, "averageMs": average, "reusePercent": reuse, "busiestHost": busiest, "busiestActive": current, "maxPerHost": s.MaxPerHost, "timeoutSeconds": s.TimeoutSeconds, "idleSeconds": s.IdleSeconds, "maxIdle": s.MaxIdle, "maxIdlePerHost": s.MaxIdlePerHost, "sampling": "全量"}
}
