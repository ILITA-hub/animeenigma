package transport

// Tasks 3-4 (RBAC and roulette, Phase 2) — gateway route-cutover tests. These
// pin the FeatureGate("fanfic"|"gacha"|"profile-wall") wiring that replaced
// the old static admin-only dark-ship config bools: the ruleset cache is
// built + started (synchronously refreshed once) inside NewRouterWithCleanup
// and consulted per-request instead of a config bool baked in at process
// start. Matrix per route: fanfic/gacha (JWT-required groups) — no token →
// 401 (JWT before FeatureGate); valid non-admin JWT → 403 (FeatureGate
// denies); admin JWT → 200 (reaches the stub backend). profile-wall
// (OptionalJWT group) — anonymous → 403 (FeatureGate denies, not 401);
// valid non-admin JWT → 403; admin JWT → 200 (reaches the player stub).

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

// featureGateTestGateway mirrors policyTestGateway (router_policy_test.go)
// with its own fanfic + gacha + player + policy backend stubs.
type featureGateTestGateway struct {
	router       http.Handler
	fanficGotURL chan string
	gachaGotURL  chan string
	playerGotURL chan string
	teardown     func()
}

func buildFeatureGateGatewayRouter(t *testing.T) *featureGateTestGateway {
	t.Helper()

	fanficGot := make(chan string, 8)
	fanficBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fanficGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))

	gachaGot := make(chan string, 8)
	gachaBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gachaGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))

	playerGot := make(chan string, 8)
	playerBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		playerGot <- r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))

	// Stub policy-service ruleset feed. NewRouterWithCleanup's rulesetCache
	// does a SYNCHRONOUS refresh before returning (Start → refresh), so this
	// snapshot is loaded by the time the router is built — no polling needed.
	policyBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"flags":{"fanfic":{"roles":["admin"]},"gacha":{"roles":["admin"]},"profile-wall":{"roles":["admin"]}},"failSafe":{"fanfic":"admin","gacha":"admin","profile-wall":"admin"}}}`))
	}))

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 0},
		JWT:    gatewayTestJWTConfig(),
		Services: config.ServiceURLs{
			FanficService: fanficBackend.URL,
			GachaService:  gachaBackend.URL,
			PlayerService: playerBackend.URL,
			PolicyService: policyBackend.URL,
			// Other services set to unreachable stubs — these tests don't hit them.
			AuthService: "http://auth-unused:8080",
			WebService:  "http://web-unused:80",
		},
		RateLimit: config.RateLimitConfig{
			RequestsPerSecond: 1000,
			BurstSize:         1000,
		},
		CORSOrigins:    []string{},
		RulesetRefresh: 15 * time.Second,
	}

	log := logger.Default()
	proxySvc := service.NewProxyService(cfg.Services, log)
	proxyHandler := handler.NewProxyHandler(proxySvc, log)

	// router_test passes nil for the Redis client — nil yields a pass-through
	// user-rate-limit middleware, same as every other router test file.
	router, cleanup := NewRouterWithCleanup(proxyHandler, cfg, log, sharedGatewayCollector(), nil)
	t.Cleanup(cleanup)

	return &featureGateTestGateway{
		router:       router,
		fanficGotURL: fanficGot,
		gachaGotURL:  gachaGot,
		playerGotURL: playerGot,
		teardown: func() {
			fanficBackend.Close()
			gachaBackend.Close()
			playerBackend.Close()
			policyBackend.Close()
		},
	}
}

// --- /api/fanfic/ -----------------------------------------------------

func TestRouter_FeatureGate_Fanfic_NoToken_Returns401(t *testing.T) {
	gw := buildFeatureGateGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/api/fanfic/", nil)
	req.RemoteAddr = "10.0.1.10:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rec.Code)
	}
	select {
	case got := <-gw.fanficGotURL:
		t.Errorf("fanfic backend received unexpected request %q — must not forward without a token", got)
	default:
	}
}

func TestRouter_FeatureGate_Fanfic_NonAdminJWT_Returns403(t *testing.T) {
	gw := buildFeatureGateGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/fanfic/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.1.11:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", rec.Code)
	}
}

func TestRouter_FeatureGate_Fanfic_AdminJWT_ProxiesToFanfic(t *testing.T) {
	gw := buildFeatureGateGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/fanfic/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.1.12:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	select {
	case got := <-gw.fanficGotURL:
		if got != "/api/fanfic/" {
			t.Errorf("fanfic backend received path = %q; want /api/fanfic/", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fanfic backend never received the request")
	}
}

// --- /api/gacha/wallet -------------------------------------------------

func TestRouter_FeatureGate_GachaWallet_NoToken_Returns401(t *testing.T) {
	gw := buildFeatureGateGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/api/gacha/wallet", nil)
	req.RemoteAddr = "10.0.1.20:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", rec.Code)
	}
	select {
	case got := <-gw.gachaGotURL:
		t.Errorf("gacha backend received unexpected request %q — must not forward without a token", got)
	default:
	}
}

func TestRouter_FeatureGate_GachaWallet_NonAdminJWT_Returns403(t *testing.T) {
	gw := buildFeatureGateGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/gacha/wallet", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.1.21:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", rec.Code)
	}
}

func TestRouter_FeatureGate_GachaWallet_AdminJWT_ProxiesToGacha(t *testing.T) {
	gw := buildFeatureGateGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/gacha/wallet", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.1.22:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	select {
	case got := <-gw.gachaGotURL:
		if got != "/api/gacha/wallet" {
			t.Errorf("gacha backend received path = %q; want /api/gacha/wallet", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gacha backend never received the request")
	}
}

// --- /api/users/{userId}/showcase (profile-wall) -----------------------
//
// Unlike fanfic/gacha (JWT-required groups), the profile-wall group uses
// OptionalJWTValidationMiddleware — so an anonymous request passes auth and
// reaches FeatureGate, which denies with 403 (not 401).

func TestRouter_FeatureGate_ProfileWall_Anonymous_Returns403(t *testing.T) {
	gw := buildFeatureGateGatewayRouter(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodGet, "/api/users/u1/showcase", nil)
	req.RemoteAddr = "10.0.1.30:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", rec.Code)
	}
	select {
	case got := <-gw.playerGotURL:
		t.Errorf("player backend received unexpected request %q — must not forward an anonymous request", got)
	default:
	}
}

func TestRouter_FeatureGate_ProfileWall_NonAdminJWT_Returns403(t *testing.T) {
	gw := buildFeatureGateGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/users/u1/showcase", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.1.31:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", rec.Code)
	}
}

func TestRouter_FeatureGate_ProfileWall_AdminJWT_ProxiesToPlayer(t *testing.T) {
	gw := buildFeatureGateGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/users/u1/showcase", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.1.32:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	select {
	case got := <-gw.playerGotURL:
		if got != "/api/users/u1/showcase" {
			t.Errorf("player backend received path = %q; want /api/users/u1/showcase", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("player backend never received the request")
	}
}
