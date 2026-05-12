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
	// Phase 17: nil cache preserves Phase 16 dispatch behaviour for router tests
	// (both the orchestrator and the admin-handler caches default to nil here —
	// these router tests assert route registration, not cache snapshot content).
	orch := service.NewOrchestrator(log, domain.NewRegistry(), nil)
	sh := handler.NewScraperHandler(orch, nil, log)
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
		// Phase 17 Plan 03: admin debug endpoint. With a nil cache the
		// handler still returns 200 (just an empty admin map).
		{"/scraper/health/admin", http.StatusOK},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			// REVIEW.md WR-10: the /scraper/health/admin route is now
			// guarded by privateOnlyMiddleware. httptest.NewRequest's
			// default RemoteAddr (192.0.2.1, TEST-NET) is public and
			// would be rejected; set a docker-bridge-style private IP.
			req.RemoteAddr = "172.18.0.5:54321"
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

// TestRouter_AdminHealthRejectsPublicRemoteAddr — WR-10 regression. The
// admin route MUST refuse requests from non-private RemoteAddr, even
// without a JWT/admin-role check on the scraper side. This is the
// defense-in-depth layer that catches a future SERVER_HOST=0.0.0.0
// accident.
func TestRouter_AdminHealthRejectsPublicRemoteAddr(t *testing.T) {
	t.Parallel()
	r := freshTestRouter(t)
	cases := []string{
		"8.8.8.8:54321",       // public Google DNS
		"203.0.113.1:54321",   // TEST-NET-3 (public, non-routable per RFC)
		"198.51.100.1:54321",  // TEST-NET-2
	}
	for _, raddr := range cases {
		raddr := raddr
		t.Run(raddr, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/scraper/health/admin", nil)
			req.RemoteAddr = raddr
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("public RemoteAddr %s got status %d; want 403", raddr, rec.Code)
			}
		})
	}
}

// TestRouter_AdminHealthAcceptsPrivateRemoteAddr — WR-10 positive case.
// Docker-bridge / loopback / link-local IPs MUST be allowed through so
// the legitimate gateway → scraper admin path keeps working.
func TestRouter_AdminHealthAcceptsPrivateRemoteAddr(t *testing.T) {
	t.Parallel()
	r := freshTestRouter(t)
	cases := []string{
		"127.0.0.1:54321",  // loopback IPv4
		"172.18.0.5:54321", // docker bridge default subnet
		"10.0.0.10:54321",  // RFC-1918
		"192.168.1.5:54321", // RFC-1918
		"[::1]:54321",      // IPv6 loopback
	}
	for _, raddr := range cases {
		raddr := raddr
		t.Run(raddr, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/scraper/health/admin", nil)
			req.RemoteAddr = raddr
			r.ServeHTTP(rec, req)
			if rec.Code == http.StatusForbidden {
				t.Errorf("private RemoteAddr %s got 403; want pass-through", raddr)
			}
		})
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
