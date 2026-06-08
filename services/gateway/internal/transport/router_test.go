package transport

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

// metrics.NewCollector registers Prometheus collectors against the global
// promauto registry, so calling it twice in the same process panics with
// "duplicate metrics collector registration". Share one collector across
// every router-construction in this test file.
var (
	gatewayTestCollector     *metrics.Collector
	gatewayTestCollectorOnce sync.Once
)

func sharedGatewayCollector() *metrics.Collector {
	gatewayTestCollectorOnce.Do(func() {
		gatewayTestCollector = metrics.NewCollector("gateway-router-test")
	})
	return gatewayTestCollector
}

// gatewayTestSecret is the symmetric secret shared by the test JWT signer
// and the gateway router's JWTValidationMiddleware.
const gatewayTestSecret = "router-test-secret-do-not-use-in-prod"

func gatewayTestJWTConfig() authz.JWTConfig {
	return authz.JWTConfig{
		Secret:          gatewayTestSecret,
		Issuer:          "animeenigma-test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: time.Hour,
	}
}

// signTestJWT returns an HS256-signed access token with the given role.
// The token's claims feed directly into JWTValidationMiddleware →
// AdminRoleMiddleware on the gateway side.
func signTestJWT(t *testing.T, role authz.Role) string {
	t.Helper()
	mgr := authz.NewJWTManager(gatewayTestJWTConfig())
	pair, err := mgr.GenerateTokenPair("user-1", "tester", role, "")
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}
	return pair.AccessToken
}

// buildTestGatewayRouter wires the gateway router on top of two httptest
// backends (one for the scraper service, one for the catalog service) so
// the router test can spy on which backend received the request — proving
// the chi route-registration order is correct.
//
// Returns (router, scraperURL, catalogURL); callers close the two
// backends individually via the returned http.Server cleanup channel.
type testGateway struct {
	router        http.Handler
	scraperGotURL chan string
	catalogGotURL chan string
	playerGotURL  chan string
	webGotURL     chan string
	teardown      func()
}

func buildTestGatewayRouter(t *testing.T) *testGateway {
	t.Helper()

	scraperGot := make(chan string, 4)
	scraperBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scraperGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"providers":{},"admin":{}}}`))
	}))
	catalogGot := make(chan string, 4)
	catalogBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		catalogGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	playerGot := make(chan string, 4)
	playerBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		playerGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	webGot := make(chan string, 4)
	webBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html></html>`))
	}))

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 0},
		JWT:    gatewayTestJWTConfig(),
		Services: config.ServiceURLs{
			AuthService:    "http://auth-unused:8080", // JWT validation is in-process; no auth call needed for non-ak_ tokens
			CatalogService: catalogBackend.URL,
			ScraperService: scraperBackend.URL,
			PlayerService:  playerBackend.URL,
			WebService:     webBackend.URL,
		},
		RateLimit: config.RateLimitConfig{
			RequestsPerSecond: 1000, // High so test traffic never trips it
			BurstSize:         1000,
		},
		CORSOrigins: []string{},
	}

	log := logger.Default()
	proxySvc := service.NewProxyService(cfg.Services, log)
	proxyHandler := handler.NewProxyHandler(proxySvc, log)
	// router_test passes nil for the Redis client — these tests cover the
	// chi route-resolution layer, not the per-user rate limiter (WV3-T3).
	// With nil, newUserRateLimitChainFn yields a pass-through middleware
	// so the protected route groups behave exactly as they did pre-T3.
	router, rateLimiterStop := NewRouterWithCleanup(proxyHandler, cfg, log, sharedGatewayCollector(), nil)
	// REVIEW.md WR-04: stop the per-IP rate-limiter's eviction goroutine
	// when the test ends, otherwise each NewRouter invocation in the test
	// suite leaks one goroutine for the lifetime of the binary.
	t.Cleanup(rateLimiterStop)

	return &testGateway{
		router:        router,
		scraperGotURL: scraperGot,
		catalogGotURL: catalogGot,
		playerGotURL:  playerGot,
		webGotURL:     webGot,
		teardown: func() {
			scraperBackend.Close()
			catalogBackend.Close()
			playerBackend.Close()
			webBackend.Close()
		},
	}
}

func TestRateLimitMiddleware_AllowsNormalTraffic(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         20,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitMiddleware(cfg)(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("single request should pass, got status %d", w.Code)
	}
}

func TestRateLimitMiddleware_BlocksExcessiveRequests(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 1,
		BurstSize:         3,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitMiddleware(cfg)(inner)

	blocked := false
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}

	if !blocked {
		t.Error("rate limiter should block excessive requests with 429")
	}
}

func TestRateLimitMiddleware_DifferentIPsIndependent(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 1,
		BurstSize:         2,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RateLimitMiddleware(cfg)(inner)

	// Exhaust IP1's burst
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// IP2 should still pass
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("different IP should not be rate limited, got status %d", w.Code)
	}
}

func TestAdminRoleMiddleware_AdminAllowed(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("admin content"))
	})

	handler := AdminRoleMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	claims := &authz.Claims{
		UserID:   "admin-1",
		Username: "admin",
		Role:     authz.RoleAdmin,
	}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("admin should be allowed, got status %d", w.Code)
	}
}

func TestAdminRoleMiddleware_UserBlocked(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := AdminRoleMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	claims := &authz.Claims{
		UserID:   "user-1",
		Username: "regular_user",
		Role:     authz.RoleUser,
	}
	ctx := authz.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("regular user should be blocked with 403, got status %d", w.Code)
	}
}

func TestAdminRoleMiddleware_NoClaims(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := AdminRoleMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	// No claims in context
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("request without claims should be blocked with 403, got status %d", w.Code)
	}
}

// TestRouter_AdminScraperProxy_AdminJWT_Returns200 — valid admin JWT routes
// /api/admin/scraper/health through to the scraper backend with the path
// rewritten to /scraper/health/admin (Plan 17-03 acceptance).
func TestRouter_AdminScraperProxy_AdminJWT_Returns200(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}

	select {
	case got := <-gw.scraperGotURL:
		if got != "/scraper/health/admin" {
			t.Errorf("scraper backend received path = %q; want /scraper/health/admin", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scraper backend never received the request")
	}

	// Catalog backend must NOT have been called.
	select {
	case got := <-gw.catalogGotURL:
		t.Errorf("catalog backend received unexpected request: %q", got)
	default:
	}
}

// TestRouter_AdminScraperRejectsNonAdminJWT — non-admin JWT → 403.
func TestRouter_AdminScraperRejectsNonAdminJWT(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.2:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", rec.Code)
	}
}

// TestRouter_AdminScraperRejectsMissingJWT — missing Authorization → 401.
func TestRouter_AdminScraperRejectsMissingJWT(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	req.RemoteAddr = "10.0.0.3:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rec.Code)
	}
}

// TestRouter_AdminScraperRoutedBeforeCatalogAdmin — chi resolves routes in
// registration order; the /admin/scraper/* group MUST precede the catalog
// /admin/* group so the scraper backend receives the request, not catalog.
//
// We assert this by spying on BOTH backends. If the route-ordering bug
// regresses (somebody flips the registration order), the catalog backend
// receives the request and the test fails loudly.
func TestRouter_AdminScraperRoutedBeforeCatalogAdmin(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/scraper/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.4:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}

	// Drain the scraper channel — must be the SOLE receiver.
	select {
	case <-gw.scraperGotURL:
	case <-time.After(2 * time.Second):
		t.Fatal("scraper backend never received the request (route-ordering regression?)")
	}
	select {
	case got := <-gw.catalogGotURL:
		t.Errorf("catalog backend received the request — /admin/scraper/* must precede /admin/* in the router: %q", got)
	default:
	}
}

// TestRouter_AdminReportsRoutedToPlayer — the admin feedback browser lives in
// the player service, so /api/admin/reports (and its sub-paths) MUST reach
// player, not the generic /admin/* → catalog group. Spies on both backends so
// a route-ordering regression fails loudly.
func TestRouter_AdminReportsRoutedToPlayer(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	for _, path := range []string{"/api/admin/reports", "/api/admin/reports/2026-06-05T12-00-00_alice_feedback"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "10.0.0.5:1234"
		rec := httptest.NewRecorder()
		gw.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("path %q: status = %d; want 200 (body=%q)", path, rec.Code, rec.Body.String())
		}
		select {
		case <-gw.playerGotURL:
		case <-time.After(2 * time.Second):
			t.Fatalf("path %q: player backend never received the request (route-ordering regression?)", path)
		}
		select {
		case got := <-gw.catalogGotURL:
			t.Errorf("path %q: catalog received it — /admin/reports* must precede /admin/* → catalog: %q", path, got)
		default:
		}
	}
}

// TestRouter_AdminFeedbackPageFallsThroughToWeb — the browser route
// /admin/feedback (NOT /api/admin/...) must fall through to the web SPA so
// AdminFeedback.vue renders. Without the fall-through chi 404s the navigation
// before the SPA loads (the bug that hid the page). Mirrors the /recs and
// /collections fall-throughs.
func TestRouter_AdminFeedbackPageFallsThroughToWeb(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	for _, path := range []string{"/admin/feedback", "/admin/raw-library"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "10.0.0.7:1234"
		rec := httptest.NewRecorder()
		gw.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("path %q: status = %d; want 200 (body=%q)", path, rec.Code, rec.Body.String())
		}
		select {
		case <-gw.webGotURL:
		case <-time.After(2 * time.Second):
			t.Fatalf("path %q: web backend never received the request (missing /admin fall-through?)", path)
		}
	}
}

// TestRouter_AdminReportsRequiresAdmin — non-admin JWT must be 403 at the gateway.
func TestRouter_AdminReportsRequiresAdmin(t *testing.T) {
	gw := buildTestGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/reports", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.6:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("non-admin status = %d; want 403", rec.Code)
	}
}
