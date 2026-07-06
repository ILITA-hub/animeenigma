package transport

// Task 6 (RBAC and roulette, Phase 1) — gateway proxy contract tests for the
// new policy-service routes. This is PROXY-ONLY routing: the gateway does not
// yet enforce the policy ruleset against other services (that's Phase 2's
// FeatureGate middleware). These tests pin three invariants:
//
//  1. GET /api/policy/features/mine is JWT-OPTIONAL and reaches the policy
//     backend (anonymous callers get the everyone-flags feed from
//     policy-service itself, not a gateway-level bounce).
//  2. GET /api/admin/policy/flags (and friends under /admin/policy/*) is
//     gated JWT + admin at the gateway — no token → 401, non-admin → 403,
//     admin JWT → reaches the policy backend.
//  3. GET /internal/policy/ruleset is Docker-network-only and must NOT be
//     reachable through the gateway at all (no route registered for it).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

// policyTestGateway is a self-contained test harness (mirrors
// buildWorkerGatewayRouter in router_worker_test.go) with its own
// policy-service backend stub, so we can spy on exactly which paths reach
// it without touching the shared buildTestGatewayRouter helper.
type policyTestGateway struct {
	router        http.Handler
	policyGotURL  chan string
	catalogGotURL chan string
	teardown      func()
}

func buildPolicyGatewayRouter(t *testing.T) *policyTestGateway {
	t.Helper()

	policyGot := make(chan string, 8)
	policyBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policyGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"visible":{},"anidle":true}}`))
	}))

	// Unused catalog backend — proves /admin/policy* never spills over into
	// the generic catalog /admin/* group.
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
			PolicyService:  policyBackend.URL,
			CatalogService: catalogBackend.URL,
			// Other services set to unreachable stubs — these tests don't hit them.
			AuthService: "http://auth-unused:8080",
			WebService:  "http://web-unused:80",
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

	return &policyTestGateway{
		router:        router,
		policyGotURL:  policyGot,
		catalogGotURL: catalogGot,
		teardown: func() {
			policyBackend.Close()
			catalogBackend.Close()
		},
	}
}

// TestRouter_Policy_FeaturesMine_Anonymous_ProxiesToPolicy — anonymous
// (no Authorization header) GET /api/policy/features/mine must reach the
// policy backend, NOT bounce off a 401 at the gateway. Per-user visibility
// resolution (anonymous ⇒ everyone-flags) happens on the policy-service side.
func TestRouter_Policy_FeaturesMine_Anonymous_ProxiesToPolicy(t *testing.T) {
	gw := buildPolicyGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/api/policy/features/mine", nil)
	req.RemoteAddr = "10.0.0.20:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}

	select {
	case got := <-gw.policyGotURL:
		if got != "/api/policy/features/mine" {
			t.Errorf("policy backend received path = %q; want /api/policy/features/mine", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("policy backend never received the request — is /policy/features/mine mounted?")
	}
}

// TestRouter_Policy_FeaturesMine_WithJWT_ProxiesToPolicy — a valid (non-admin)
// JWT must also reach the backend: the route is JWT-OPTIONAL, not JWT-only.
func TestRouter_Policy_FeaturesMine_WithJWT_ProxiesToPolicy(t *testing.T) {
	gw := buildPolicyGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/policy/features/mine", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.21:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}

	select {
	case <-gw.policyGotURL:
	case <-time.After(2 * time.Second):
		t.Fatal("policy backend never received the authenticated request")
	}
}

// TestRouter_Policy_AdminFlags_NoToken_Returns401 — missing Authorization on
// /api/admin/policy/flags must 401 at the gateway before ever reaching the
// policy backend.
func TestRouter_Policy_AdminFlags_NoToken_Returns401(t *testing.T) {
	gw := buildPolicyGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/policy/flags", nil)
	req.RemoteAddr = "10.0.0.22:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rec.Code)
	}

	select {
	case got := <-gw.policyGotURL:
		t.Errorf("policy backend received unexpected request %q — must not forward without a token", got)
	default:
	}
}

// TestRouter_Policy_AdminFlags_NonAdminJWT_Returns403 — a valid but
// non-admin JWT must be rejected with 403 at the gateway (AdminRoleMiddleware).
func TestRouter_Policy_AdminFlags_NonAdminJWT_Returns403(t *testing.T) {
	gw := buildPolicyGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/policy/flags", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.23:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", rec.Code)
	}
}

// TestRouter_Policy_AdminFlags_AdminJWT_ProxiesToPolicy — valid admin JWT
// reaches the policy backend at the unrewritten path.
func TestRouter_Policy_AdminFlags_AdminJWT_ProxiesToPolicy(t *testing.T) {
	gw := buildPolicyGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/policy/flags", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.24:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}

	select {
	case got := <-gw.policyGotURL:
		if got != "/api/admin/policy/flags" {
			t.Errorf("policy backend received path = %q; want /api/admin/policy/flags", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("policy backend never received the request — is /admin/policy/* mounted?")
	}

	// Catalog's generic /admin/* group must NOT have been hit.
	select {
	case got := <-gw.catalogGotURL:
		t.Errorf("catalog backend received unexpected request %q — /admin/policy/* must not fall through to catalog", got)
	default:
	}
}

// TestRouter_Policy_AdminRoulette_AdminJWT_ProxiesToPolicy — PUT
// /api/admin/policy/roulette (distinct sub-path under /admin/policy/*) also
// reaches the policy backend under the same admin gate.
func TestRouter_Policy_AdminRoulette_AdminJWT_ProxiesToPolicy(t *testing.T) {
	gw := buildPolicyGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/policy/roulette", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.25:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}

	select {
	case got := <-gw.policyGotURL:
		if got != "/api/admin/policy/roulette" {
			t.Errorf("policy backend received path = %q; want /api/admin/policy/roulette", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("policy backend never received the request")
	}
}

// TestRouter_Policy_InternalRuleset_NotRouted — GET /internal/policy/ruleset
// must NOT be reachable through the gateway at all (D-05 security model: the
// gateway never registers a route under /internal/*). We assert it is not a
// 200 forwarded to the policy backend — any other status (404 from chi, etc.)
// is acceptable proof the route simply doesn't exist at the gateway.
func TestRouter_Policy_InternalRuleset_NotRouted(t *testing.T) {
	gw := buildPolicyGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/internal/policy/ruleset", nil)
	req.RemoteAddr = "10.0.0.26:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Errorf("/internal/policy/ruleset returned 200 — must not be gateway-routed (Docker-network-only)")
	}

	select {
	case got := <-gw.policyGotURL:
		t.Errorf("policy backend received /internal/policy/ruleset via the gateway (%q) — this must be Docker-network-only, never proxied", got)
	default:
	}
}
