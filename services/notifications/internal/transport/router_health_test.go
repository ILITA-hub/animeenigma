package transport

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// libs/metrics.NewCollector registers to the global Prometheus registry via
// promauto, so a second call in the same test binary panics. sync.Once shares
// one instance across the health tests.
var (
	healthCollectorOnce sync.Once
	healthCollector     *metrics.Collector
)

func sharedHealthCollector() *metrics.Collector {
	healthCollectorOnce.Do(func() {
		healthCollector = metrics.NewCollector("notifications-health-test")
	})
	return healthCollector
}

// newHealthRouter builds the router with the bare minimum for the /health
// probe surface. Handlers are nil: /health is an inline closure and the only
// routes that reference the nil handlers (/internal/*, /api/*) are never hit
// here, so their nil-receiver method values are never dereferenced.
func newHealthRouter(t *testing.T) http.Handler {
	t.Helper()
	return NewRouter(nil, nil, nil, authz.JWTConfig{}, logger.Default(), sharedHealthCollector())
}

// The Docker healthcheck probes /health with `wget --spider`, which issues an
// HTTP HEAD. chi only answers HEAD when a HEAD route is registered; a GET-only
// /health 405s the probe and the container is reported unhealthy even though it
// is serving fine. The fleet convention (gacha, watch-together, …) registers
// both GET and HEAD on /health, so notifications must too.
func TestRouter_HealthHEAD_ReturnsOK(t *testing.T) {
	r := newHealthRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD /health: expected 200, got %d", rec.Code)
	}
}

func TestRouter_HealthGET_ReturnsOK(t *testing.T) {
	r := newHealthRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health: expected 200, got %d", rec.Code)
	}
}
