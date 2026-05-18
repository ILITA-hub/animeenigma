package service

// Phase 06 (workstream raw-jp / v0.2) tests for the catalog
// invalidator. Verifies HTTP method (POST), URL composition (PathEscape
// on shikimori_id), 200 → "ok" metric, non-2xx → "fail" metric,
// timeout bounded, and that an empty CatalogInternalAPIURL yields a
// no-op invalidator.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stubInvalidationMetrics records every IncCacheInvalidation call.
type stubInvalidationMetrics struct {
	mu      sync.Mutex
	results []string
}

func (s *stubInvalidationMetrics) IncCacheInvalidation(result string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, result)
}

func (s *stubInvalidationMetrics) count(result string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.results {
		if r == result {
			n++
		}
	}
	return n
}

func TestCatalogInvalidator_Happy200(t *testing.T) {
	var receivedMethod string
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := &stubInvalidationMetrics{}
	inv := NewCatalogInvalidator(InvalidatorConfig{
		CatalogInternalAPIURL: srv.URL,
		Timeout:               2 * time.Second,
	}, m, nil)

	inv.Invalidate(context.Background(), "57466")

	if receivedMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", receivedMethod)
	}
	if receivedPath != "/internal/cache/invalidate/raw/57466" {
		t.Errorf("path = %q, want /internal/cache/invalidate/raw/57466", receivedPath)
	}
	if m.count("ok") != 1 {
		t.Errorf("ok count = %d, want 1; results = %v", m.count("ok"), m.results)
	}
	if m.count("fail") != 0 {
		t.Errorf("fail count = %d, want 0", m.count("fail"))
	}
}

func TestCatalogInvalidator_Non2xx_Fail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := &stubInvalidationMetrics{}
	inv := NewCatalogInvalidator(InvalidatorConfig{
		CatalogInternalAPIURL: srv.URL,
		Timeout:               2 * time.Second,
	}, m, nil)

	inv.Invalidate(context.Background(), "57466")

	if m.count("fail") != 1 {
		t.Errorf("fail count = %d, want 1; results = %v", m.count("fail"), m.results)
	}
	if m.count("ok") != 0 {
		t.Errorf("ok count = %d, want 0", m.count("ok"))
	}
}

func TestCatalogInvalidator_Timeout_Bounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := &stubInvalidationMetrics{}
	inv := NewCatalogInvalidator(InvalidatorConfig{
		CatalogInternalAPIURL: srv.URL,
		Timeout:               50 * time.Millisecond,
	}, m, nil)

	start := time.Now()
	inv.Invalidate(context.Background(), "57466")
	elapsed := time.Since(start)

	if m.count("fail") != 1 {
		t.Errorf("fail count = %d, want 1; results = %v", m.count("fail"), m.results)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("invalidate took %s; want bounded by ~2× timeout (100ms)", elapsed)
	}
}

func TestCatalogInvalidator_EmptyURL_NoopNoMetric(t *testing.T) {
	m := &stubInvalidationMetrics{}
	inv := NewCatalogInvalidator(InvalidatorConfig{
		CatalogInternalAPIURL: "",
		Timeout:               2 * time.Second,
	}, m, nil)

	// noopInvalidator has no http calls — must not record any metric.
	inv.Invalidate(context.Background(), "57466")

	if m.count("ok") != 0 || m.count("fail") != 0 {
		t.Errorf("no-op invalidator must not record metrics; got %v", m.results)
	}
}

func TestCatalogInvalidator_PathEscape(t *testing.T) {
	var seenPath atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.URL.Path is already decoded; r.URL.RawPath preserves the
		// percent-encoded form when the path contained reserved chars.
		raw := r.URL.RawPath
		if raw == "" {
			raw = r.URL.Path
		}
		seenPath.Store(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := &stubInvalidationMetrics{}
	inv := NewCatalogInvalidator(InvalidatorConfig{
		CatalogInternalAPIURL: srv.URL,
		Timeout:               2 * time.Second,
	}, m, nil)

	inv.Invalidate(context.Background(), "57466/x")

	got, _ := seenPath.Load().(string)
	// PathEscape encodes "/" as "%2F" — assert via substring.
	if !strings.Contains(got, "57466%2Fx") {
		t.Errorf("server saw path %q; want substring 57466%%2Fx (PathEscape applied)", got)
	}
	if m.count("ok") != 1 {
		t.Errorf("ok count = %d, want 1", m.count("ok"))
	}
}

func TestCatalogInvalidator_EmptyShikimoriID_NoRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := &stubInvalidationMetrics{}
	inv := NewCatalogInvalidator(InvalidatorConfig{
		CatalogInternalAPIURL: srv.URL,
		Timeout:               2 * time.Second,
	}, m, nil)

	inv.Invalidate(context.Background(), "")

	if hits.Load() != 0 {
		t.Errorf("server hit %d times for empty shikimoriID; want 0", hits.Load())
	}
	if len(m.results) != 0 {
		t.Errorf("metrics recorded for empty shikimoriID: %v; want none", m.results)
	}
}
