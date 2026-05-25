// router_watch_together_test.go — workstream watch-together v1.0 Phase 1
// Plan 01.7.2. Router-level integration tests for the new watch-together
// routes:
//
//	GET  /api/watch-together/ws           — WS upgrade, NO JWT middleware
//	POST /api/watch-together/rooms        — REST, JWT-required
//	GET  /api/watch-together/rooms/{id}   — REST, JWT-required
//
// The /ws path MUST sit outside JWTValidationMiddleware (browsers can't set
// Authorization on a WS upgrade — auth lives in ?token=). The REST routes
// behave exactly like /api/notifications: 401 on missing JWT, 200 on valid.
package transport

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

// wtTestGateway is the watch-together-flavoured fixture: a gateway router
// configured with a real httptest WS backend so tests can dial through the
// gateway and assert end-to-end behaviour.
type wtTestGateway struct {
	server         *httptest.Server
	router         http.Handler
	wtRESTGotURL   chan string // REST path observed by backend
	wtWSGotQuery   chan string // WS upgrade query string observed by backend
	wtWSGotPath    chan string // WS upgrade path observed by backend
	teardown       func()
}

func buildWatchTogetherGateway(t *testing.T) *wtTestGateway {
	t.Helper()

	restGot := make(chan string, 4)
	wsQueryGot := make(chan string, 4)
	wsPathGot := make(chan string, 4)

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	// One backend serving BOTH REST (returns JSON 200) and WS (upgrades).
	// Distinguishes by URL path: /api/watch-together/ws → WS upgrade,
	// otherwise → REST 200.
	wtBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/watch-together/ws" {
			select {
			case wsQueryGot <- r.URL.RawQuery:
			default:
			}
			select {
			case wsPathGot <- r.URL.Path:
			default:
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			_ = conn.Close()
			return
		}
		select {
		case restGot <- r.URL.Path:
		default:
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 0},
		JWT:    gatewayTestJWTConfig(),
		Services: config.ServiceURLs{
			AuthService:          "http://auth-unused:8080",
			WatchTogetherService: wtBackend.URL,
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
	router, rateLimiterStop := NewRouterWithCleanup(proxyHandler, cfg, log, sharedGatewayCollector(), nil)
	t.Cleanup(rateLimiterStop)

	// Wrap router in an httptest.Server so we can dial it with a real
	// gorilla/websocket client (which needs a network listener; we can't
	// dial httptest.NewRecorder because it doesn't expose a TCP socket).
	server := httptest.NewServer(router)

	return &wtTestGateway{
		server:       server,
		router:       router,
		wtRESTGotURL: restGot,
		wtWSGotQuery: wsQueryGot,
		wtWSGotPath:  wsPathGot,
		teardown: func() {
			server.Close()
			wtBackend.Close()
		},
	}
}

// TestRouter_WatchTogether_WS_NoAuthRequired — the WS endpoint MUST work
// without an Authorization header (browsers can't set one on WS upgrades).
// We assert this by dialing without any Authorization and observing the
// backend receive the upgrade with our ?token=... query string.
func TestRouter_WatchTogether_WS_NoAuthRequired(t *testing.T) {
	gw := buildWatchTogetherGateway(t)
	defer gw.teardown()

	u, _ := url.Parse(gw.server.URL)
	u.Scheme = "ws"
	u.Path = "/api/watch-together/ws"
	u.RawQuery = "token=fake-jwt-not-validated-here&room=room-abc"

	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 5 * time.Second
	conn, resp, err := dialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v (resp=%+v)", err, resp)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d; want 101", resp.StatusCode)
	}

	select {
	case got := <-gw.wtWSGotQuery:
		if got != "token=fake-jwt-not-validated-here&room=room-abc" {
			t.Errorf("backend query = %q; want token=fake-jwt-not-validated-here&room=room-abc", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch-together backend never received the WS upgrade")
	}
	select {
	case got := <-gw.wtWSGotPath:
		if got != "/api/watch-together/ws" {
			t.Errorf("backend path = %q; want /api/watch-together/ws", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch-together backend never observed path")
	}
}

// TestRouter_WatchTogether_REST_RequiresAuth — POST /api/watch-together/rooms
// without Authorization → 401 from JWTValidationMiddleware. Confirms the
// REST group sits INSIDE the JWT-required block while WS sits OUTSIDE.
func TestRouter_WatchTogether_REST_RequiresAuth(t *testing.T) {
	gw := buildWatchTogetherGateway(t)
	defer gw.teardown()

	req := httptest.NewRequest(http.MethodPost, "/api/watch-together/rooms", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401 (body=%q)", rec.Code, rec.Body.String())
	}
	// Backend must NOT have received the request.
	select {
	case got := <-gw.wtRESTGotURL:
		t.Errorf("backend received request without auth — JWT middleware not applied: %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestRouter_WatchTogether_REST_PassesWithAuth — valid JWT on POST
// /api/watch-together/rooms forwards to the watch-together backend with
// the path preserved verbatim.
func TestRouter_WatchTogether_REST_PassesWithAuth(t *testing.T) {
	gw := buildWatchTogetherGateway(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodPost, "/api/watch-together/rooms", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.2:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	select {
	case got := <-gw.wtRESTGotURL:
		if got != "/api/watch-together/rooms" {
			t.Errorf("backend path = %q; want /api/watch-together/rooms (no rewrite expected)", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch-together backend never received the REST request")
	}
}

// TestRouter_WatchTogether_REST_GetRoomByID — GET /api/watch-together/rooms/{id}
// is JWT-required AND the {id} path segment is preserved through to the backend.
func TestRouter_WatchTogether_REST_GetRoomByID(t *testing.T) {
	gw := buildWatchTogetherGateway(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/watch-together/rooms/room-abc-123", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.3:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	select {
	case got := <-gw.wtRESTGotURL:
		if got != "/api/watch-together/rooms/room-abc-123" {
			t.Errorf("backend path = %q; want /api/watch-together/rooms/room-abc-123", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch-together backend never received the GET request")
	}
}

// TestRouter_WatchTogether_REST_DeleteRoomByID — DELETE /api/watch-together/rooms/{id}
// follows the same JWT + verbatim-path contract as GET.
func TestRouter_WatchTogether_REST_DeleteRoomByID(t *testing.T) {
	gw := buildWatchTogetherGateway(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleUser)
	req := httptest.NewRequest(http.MethodDelete, "/api/watch-together/rooms/room-xyz", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.4:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	select {
	case got := <-gw.wtRESTGotURL:
		if got != "/api/watch-together/rooms/room-xyz" {
			t.Errorf("backend path = %q; want /api/watch-together/rooms/room-xyz", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch-together backend never received the DELETE request")
	}
}
