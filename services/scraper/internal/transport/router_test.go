package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/config"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/service"
)

// metrics.NewCollector registers Prometheus collectors against the global
// promauto registry, so calling it twice in the same process panics with
// "duplicate metrics collector registration". We share a single Collector
// across every router test so each test still gets an independent router
// but the metric collectors are registered exactly once.
var (
	sharedMC     *metrics.Collector
	sharedMCOnce sync.Once
)

func getSharedMC() *metrics.Collector {
	sharedMCOnce.Do(func() {
		sharedMC = metrics.NewCollector("scraper-router-test")
	})
	return sharedMC
}

// freshTestRouter builds a real router from real (zero-provider) wiring.
// Returns a new chi.Router on every call (cheap), but reuses the shared
// metrics.Collector to avoid the global-registry collision.
func freshTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := logger.Default()
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 0},
		MegacloudExtractor: config.MegacloudExtractorConfig{
			URL: "http://localhost:0",
		},
	}
	orch := service.NewOrchestrator(log, domain.NewRegistry())
	sh := handler.NewScraperHandler(orch, log)
	return NewRouter(sh, cfg, log, getSharedMC())
}

// TestRouter_AllScraperRoutesRegistered fires GETs at every /scraper/* route
// and asserts none of them 404. The handler-tier contract (503 vs 200) is
// verified by handler/scraper_test.go; this test is purely route-presence.
func TestRouter_AllScraperRoutesRegistered(t *testing.T) {
	t.Parallel()
	r := freshTestRouter(t)

	cases := []struct {
		path       string
		wantStatus int
	}{
		{"/scraper/episodes", http.StatusServiceUnavailable},
		{"/scraper/servers", http.StatusServiceUnavailable},
		{"/scraper/stream", http.StatusServiceUnavailable},
		{"/scraper/health", http.StatusOK},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			r.ServeHTTP(rec, req)
			if rec.Code == http.StatusNotFound {
				t.Fatalf("%s returned 404; route not registered", tc.path)
			}
			if rec.Code != tc.wantStatus {
				t.Errorf("%s status = %d; want %d", tc.path, rec.Code, tc.wantStatus)
			}
		})
	}
}

// TestRouter_HealthAndMetricsStillWork verifies the operational endpoints
// from plan 15-01 still respond correctly — the plan 03 router refactor
// must not regress them.
func TestRouter_HealthAndMetricsStillWork(t *testing.T) {
	t.Parallel()
	r := freshTestRouter(t)

	// /health (service liveness — separate from /scraper/health)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/health status = %d; want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ok") {
		t.Errorf("/health body = %q; want substring ok", rec.Body.String())
	}

	// /metrics (Prometheus exposition)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/metrics status = %d; want 200", rec.Code)
	}
	// Prometheus exposition format includes "# HELP" comment lines.
	if !strings.Contains(rec.Body.String(), "# HELP") {
		t.Errorf("/metrics body missing Prometheus exposition format")
	}
}

// TestRouter_ScraperHealthIsLiveSnapshot verifies the /scraper/health endpoint
// goes through the orchestrator (not the operational /health stub) by checking
// for the "providers" key which only the orchestrator path emits.
func TestRouter_ScraperHealthIsLiveSnapshot(t *testing.T) {
	t.Parallel()
	r := freshTestRouter(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/scraper/health", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"providers"`) {
		t.Errorf("/scraper/health body = %q; want substring %q", rec.Body.String(), `"providers"`)
	}
}
