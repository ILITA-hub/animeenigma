package transport

// Task 2 (RBAC and roulette) — gateway proxy contract test for the new
// admin user-resolve route. Task 1 added GET /api/admin/users/resolve to the
// auth service (turns a UUID/username/public_id/telegram_id into the
// canonical user record). This route MUST reach auth, admin-gated, and MUST
// NOT fall through to the generic /api/admin/* -> catalog catch-all (see
// router.go's "Admin routes (protected, proxied to catalog)" group) — that
// catch-all would silently 404 the resolve request against the wrong
// service. This mirrors buildPolicyGatewayRouter in router_policy_test.go,
// which pins the same invariant for /api/admin/policy/*.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

// resolveTestGateway is a self-contained test harness (mirrors
// buildPolicyGatewayRouter) with its own auth-service backend stub, so we
// can spy on exactly which backend a request to /api/admin/users/resolve
// reaches without touching the shared buildTestGatewayRouter helper (whose
// AuthService is deliberately "auth-unused:8080" for other tests, since
// standard JWT validation there never makes an HTTP call).
type resolveTestGateway struct {
	router        http.Handler
	authGotURL    chan string
	catalogGotURL chan string
	teardown      func()
}

func buildResolveGatewayRouter(t *testing.T) *resolveTestGateway {
	t.Helper()

	authGot := make(chan string, 8)
	authBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"u-1","username":"tester","public_id":"pub-1"}}`))
	}))

	// Unused catalog backend — proves /api/admin/users/resolve never spills
	// over into the generic catalog /admin/* group.
	catalogGot := make(chan string, 8)
	catalogBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		catalogGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 0},
		JWT:    gatewayTestJWTConfig(),
		Services: config.ServiceURLs{
			AuthService:    authBackend.URL,
			CatalogService: catalogBackend.URL,
			// Other services set to unreachable stubs — these tests don't hit them.
			PolicyService: "http://policy-unused:8098",
			WebService:    "http://web-unused:80",
		},
		RateLimit: config.RateLimitConfig{
			RequestsPerSecond: 1000,
			BurstSize:         1000,
		},
		CORSOrigins: []string{},
	}

	log := logger.Default()
	proxySvc := service.NewProxyService(cfg.Services, log)
	proxyHandler := handler.NewProxyHandler(proxySvc, log)

	// router_test passes nil for the Redis client — nil yields a pass-through
	// user-rate-limit middleware, same as every other router test file.
	router, cleanup := NewRouterWithCleanup(proxyHandler, cfg, log, sharedGatewayCollector(), nil)
	t.Cleanup(cleanup)

	return &resolveTestGateway{
		router:        router,
		authGotURL:    authGot,
		catalogGotURL: catalogGot,
		teardown: func() {
			authBackend.Close()
			catalogBackend.Close()
		},
	}
}

// TestRouter_AdminUsersResolve_NoToken_Returns401 — missing Authorization on
// /api/admin/users/resolve must 401 at the gateway before ever reaching auth.
func TestRouter_AdminUsersResolve_NoToken_Returns401(t *testing.T) {
	gw := buildResolveGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/resolve?q=tester", nil)
	req.RemoteAddr = "10.0.0.30:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rec.Code)
	}

	select {
	case got := <-gw.authGotURL:
		t.Errorf("auth backend received unexpected request %q — must not forward without a token", got)
	default:
	}
}

// TestRouter_AdminUsersResolve_NonAdminJWT_Returns403 — a valid but
// non-admin JWT must be rejected with 403 at the gateway (AdminRoleMiddleware).
func TestRouter_AdminUsersResolve_NonAdminJWT_Returns403(t *testing.T) {
	gw := buildResolveGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/resolve?q=tester", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.31:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", rec.Code)
	}
}

// TestRouter_AdminUsersResolve_AdminJWT_ProxiesToAuth — valid admin JWT
// reaches the AUTH backend (not catalog) at the unrewritten path, proving
// this route is registered ahead of (or otherwise not shadowed by) the
// generic /api/admin/* -> catalog catch-all.
func TestRouter_AdminUsersResolve_AdminJWT_ProxiesToAuth(t *testing.T) {
	gw := buildResolveGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/resolve?q=tester", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.32:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}

	select {
	case got := <-gw.authGotURL:
		if got != "/api/admin/users/resolve" {
			t.Errorf("auth backend received path = %q; want /api/admin/users/resolve", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("auth backend never received the request — is /admin/users/resolve mounted and routed to auth?")
	}

	// Catalog's generic /admin/* group must NOT have been hit.
	select {
	case got := <-gw.catalogGotURL:
		t.Errorf("catalog backend received unexpected request %q — /admin/users/resolve must not fall through to catalog", got)
	default:
	}
}

func TestRouter_AdminUsersList_AdminJWT_ProxiesToAuth(t *testing.T) {
	gw := buildResolveGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?q=test&page=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.32:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	select {
	case got := <-gw.authGotURL:
		if got != "/api/admin/users" {
			t.Errorf("auth backend received path = %q; want /api/admin/users", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("auth backend never received /api/admin/users")
	}
}

func TestRouter_AdminUsersRole_AdminJWT_ProxiesToAuth(t *testing.T) {
	gw := buildResolveGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/abc-123/role", strings.NewReader(`{"role":"admin"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.32:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	select {
	case got := <-gw.authGotURL:
		if got != "/api/admin/users/abc-123/role" {
			t.Errorf("auth backend received path = %q; want /api/admin/users/abc-123/role", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("auth backend never received the role PATCH")
	}
}
