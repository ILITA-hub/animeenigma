package transport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/config"
)

// sharedCollector is instantiated once per test binary because
// libs/metrics.NewCollector registers to the global Prometheus registry via
// promauto — a second NewCollector call inside the same test process panics
// with "duplicate metrics collector registration". sync.Once gives both tests
// a clean, shared instance that NewRouter is happy with.
var (
	sharedCollectorOnce sync.Once
	sharedCollector     *metrics.Collector
)

func getSharedCollector() *metrics.Collector {
	sharedCollectorOnce.Do(func() {
		sharedCollector = metrics.NewCollector("watch-together-test")
	})
	return sharedCollector
}

// newTestRouter builds a router with the bare-minimum dependencies for
// /health + /metrics integration tests. No env vars, no Redis, no
// JWT validation — the test exercises the surface area Plan 01.1 ships.
func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	cfg := &config.Config{}
	log := logger.Default()
	return NewRouter(cfg, log, getSharedCollector())
}

func TestRouter_Health_ReturnsOK(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Errorf("expected body to contain `\"status\":\"ok\"`, got %q", string(body))
	}
}

func TestRouter_Metrics_ReturnsPrometheusFormat(t *testing.T) {
	r := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected Content-Type to contain text/plain, got %q", ct)
	}
}
