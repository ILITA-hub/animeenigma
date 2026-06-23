package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

// workerTestGateway extends the existing test pattern with an upscaler backend
// so we can spy on which paths the worker routes forward to.
type workerTestGateway struct {
	router         http.Handler
	upscalerGotURL chan string
	teardown       func()
}

func buildWorkerGatewayRouter(t *testing.T, externalAPIKey string) *workerTestGateway {
	t.Helper()

	upscalerGot := make(chan string, 8)
	upscalerBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upscalerGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 0},
		JWT:    gatewayTestJWTConfig(),
		Services: config.ServiceURLs{
			UpscalerService: upscalerBackend.URL,
			// Other services set to unreachable stubs — tests below don't hit them.
			AuthService:    "http://auth-unused:8080",
			CatalogService: "http://catalog-unused:8081",
			WebService:     "http://web-unused:80",
		},
		RateLimit: config.RateLimitConfig{
			RequestsPerSecond: 1000,
			BurstSize:         1000,
		},
		ExternalAPIKey: externalAPIKey,
	}

	log := logger.Default()
	proxySvc := service.NewProxyService(cfg.Services, log)
	proxyHandler := handler.NewProxyHandler(proxySvc, log)

	// Share the single metrics collector to avoid duplicate registration panics.
	collector := sharedGatewayCollector()

	router, cleanup := NewRouterWithCleanup(proxyHandler, cfg, log, collector, nil)
	t.Cleanup(cleanup)

	return &workerTestGateway{
		router:         router,
		upscalerGotURL: upscalerGot,
		teardown:       func() { upscalerBackend.Close() },
	}
}

// TestRouter_Worker_NoAPIKey_Returns401 — missing X-API-Key → 401 on all /worker/* paths.
func TestRouter_Worker_NoAPIKey_Returns401(t *testing.T) {
	gw := buildWorkerGatewayRouter(t, "test-secret")
	defer gw.teardown()

	for _, path := range []string{"/worker/enroll", "/worker/ws", "/worker/segments/abc.ts"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		gw.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("path %q without key: status = %d; want 401", path, rec.Code)
		}
		// Upscaler backend must NOT have been called.
		select {
		case got := <-gw.upscalerGotURL:
			t.Errorf("path %q: upscaler received unexpected request %q — must not forward without valid key", path, got)
		default:
		}
	}
}

// TestRouter_Worker_WrongAPIKey_Returns401 — wrong X-API-Key → 401.
func TestRouter_Worker_WrongAPIKey_Returns401(t *testing.T) {
	gw := buildWorkerGatewayRouter(t, "correct-secret")
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodPost, "/worker/enroll", nil)
	req.Header.Set("X-API-Key", "wrong-secret")
	req.RemoteAddr = "10.0.0.2:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong key: status = %d; want 401", rec.Code)
	}
}

// TestRouter_Worker_CorrectAPIKey_ForwardsToUpscaler — valid X-API-Key reaches the upscaler.
func TestRouter_Worker_CorrectAPIKey_ForwardsToUpscaler(t *testing.T) {
	const key = "correct-secret"
	gw := buildWorkerGatewayRouter(t, key)
	defer gw.teardown()

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/worker/enroll"},
		{http.MethodGet, "/worker/segments/chunk0001.ts"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("X-API-Key", key)
		req.RemoteAddr = "10.0.0.3:1234"
		rec := httptest.NewRecorder()
		gw.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s %s: status = %d; want 200", tc.method, tc.path, rec.Code)
		}
		select {
		case got := <-gw.upscalerGotURL:
			if got != tc.path {
				t.Errorf("%s: upscaler received path %q; want %q", tc.path, got, tc.path)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s %s: upscaler backend never received the request", tc.method, tc.path)
		}
	}
}

// TestRouter_Worker_EmptyConfiguredKey_RejectsAll — fail-closed: when
// EXTERNAL_API_KEY is empty every /worker/* request is rejected with 401.
func TestRouter_Worker_EmptyConfiguredKey_RejectsAll(t *testing.T) {
	gw := buildWorkerGatewayRouter(t, "") // empty = unconfigured
	defer gw.teardown()

	for _, headerVal := range []string{"", "anything", "correct-secret"} {
		req := httptest.NewRequest(http.MethodPost, "/worker/enroll", nil)
		if headerVal != "" {
			req.Header.Set("X-API-Key", headerVal)
		}
		req.RemoteAddr = "10.0.0.4:1234"
		rec := httptest.NewRecorder()
		gw.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("empty configured key, header=%q: status = %d; want 401 (fail-closed)", headerVal, rec.Code)
		}
	}
}

// TestRouter_Worker_NotUnderAPI — the /worker group must live at the ROUTER
// ROOT, not under /api. Verify that /api/worker/* is NOT recognized as a
// worker route (it should 404 or fall through to a different handler).
func TestRouter_Worker_NotUnderAPI(t *testing.T) {
	const key = "correct-secret"
	gw := buildWorkerGatewayRouter(t, key)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodPost, "/api/worker/enroll", nil)
	req.Header.Set("X-API-Key", key)
	req.RemoteAddr = "10.0.0.5:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	// /api/worker/* must NOT forward to the upscaler backend.
	select {
	case got := <-gw.upscalerGotURL:
		t.Errorf("/api/worker/enroll forwarded to upscaler (%q) — worker routes must be at root /worker/, not /api/worker/", got)
	default:
	}
	// It's fine to get any non-200 (404 from chi, 401 from some other gate, etc.)
	if rec.Code == http.StatusOK {
		t.Errorf("/api/worker/enroll returned 200 — should not be a valid route")
	}
}

// TestRouter_Worker_SegmentsNotRateLimited_EnrollRateLimited — segments route
// has NO dedicated per-path rate limiter; enroll does. We can't directly test
// "no limiter" via the router (the global limiter still fires), but we can
// confirm the route is reachable and forwarded correctly at normal load.
// This is more of a smoke test that the route is wired; the rate-limit
// distinction is verified by the workerEnrollRateLimitMW unit path.
func TestRouter_Worker_SegmentsRouteReachable(t *testing.T) {
	const key = "correct-secret"
	gw := buildWorkerGatewayRouter(t, key)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/worker/segments/video/seg-001.ts", nil)
	req.Header.Set("X-API-Key", key)
	req.RemoteAddr = "10.0.0.6:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("segments route: status = %d; want 200", rec.Code)
	}
	select {
	case got := <-gw.upscalerGotURL:
		if got != "/worker/segments/video/seg-001.ts" {
			t.Errorf("upscaler received %q; want /worker/segments/video/seg-001.ts", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upscaler backend never received segments request")
	}
}

// TestRouter_Worker_ModelsNotExposed — /worker/models/* is out of Phase 1
// and must NOT be routed to the upscaler.
func TestRouter_Worker_ModelsNotExposed(t *testing.T) {
	const key = "correct-secret"
	gw := buildWorkerGatewayRouter(t, key)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/worker/models/realesrgan-x4.onnx", nil)
	req.Header.Set("X-API-Key", key)
	req.RemoteAddr = "10.0.0.7:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	// Must NOT forward to upscaler.
	select {
	case got := <-gw.upscalerGotURL:
		t.Errorf("/worker/models/* forwarded to upscaler (%q) — out of Phase 1", got)
	default:
	}
}

