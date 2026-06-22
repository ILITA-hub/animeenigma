package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/service"
)

const testJWTSecret = "rooms-ws-test-secret-do-not-use"

func testJWTConfig() authz.JWTConfig {
	return authz.JWTConfig{
		Secret:          testJWTSecret,
		Issuer:          "animeenigma-test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
}

// mintToken signs an access token against the fixture secret.
func mintToken(t *testing.T, userID, username string) string {
	t.Helper()
	now := time.Now()
	claims := authz.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "animeenigma-test",
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		UserID:   userID,
		Username: username,
		Role:     authz.RoleUser,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return signed
}

// newWSAuthServer mounts HandleWebSocket at /api/v1/ws with the production
// JWT config wiring and AllowAllOrigins-style upgrader (no Origin allowlist,
// since the gorilla client doesn't send one). Returns the server.
func newWSAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	svc := service.NewWebSocketService(logger.Default())
	// Empty allowlist would fail-closed on the (absent) Origin header for a
	// browser, but the gorilla test client sends no Origin so CheckOrigin's
	// empty-origin path returns false. We pass a permissive upgrader by
	// constructing the handler with an allowlist that the no-Origin client
	// satisfies: buildOriginCheck returns false on empty Origin, so instead
	// we rely on the gorilla client also working when CheckOrigin allows it.
	// To keep this test focused on AUTH (not Origin), wrap with a handler
	// whose upgrader allows all origins.
	h := NewWebSocketHandler(svc, logger.Default(), nil, testJWTConfig())
	// Replace the upgrader with an all-origins one so the auth assertions
	// aren't masked by Origin rejection (this test targets finding L760's
	// token path, not the Origin allowlist which TestCheckOrigin covers).
	h.upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	r := chi.NewRouter()
	r.Get("/api/v1/ws", h.HandleWebSocket)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func wsAuthURL(t *testing.T, srv *httptest.Server, token string) string {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	u.Path = "/api/v1/ws"
	q := u.Query()
	if token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// Finding L760, assertion (1): ?token=<valid> upgrades successfully.
func TestHandleWebSocket_ValidQueryTokenUpgrades(t *testing.T) {
	srv := newWSAuthServer(t)
	token := mintToken(t, "user-42", "bob")

	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, resp, err := dialer.Dial(wsAuthURL(t, srv, token), nil)
	if err != nil {
		status := -1
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial with valid token failed (status=%d): %v", status, err)
	}
	defer conn.Close()

	// The welcome frame proves the connection is live.
	var welcome struct {
		Type string `json:"type"`
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := conn.ReadJSON(&welcome); err != nil {
		t.Fatalf("read welcome: %v", err)
	}
	if welcome.Type != "connected" {
		t.Fatalf("welcome type = %q, want connected", welcome.Type)
	}
}

// Finding L760, assertion (2a): missing token → 401 BEFORE upgrade.
func TestHandleWebSocket_MissingTokenRejected(t *testing.T) {
	srv := newWSAuthServer(t)

	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, resp, err := dialer.Dial(wsAuthURL(t, srv, ""), nil)
	if err == nil {
		_ = conn.Close()
		t.Fatal("dial with no token succeeded; want 401 rejection")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		got := -1
		if resp != nil {
			got = resp.StatusCode
		}
		t.Fatalf("missing-token status = %d, want %d", got, http.StatusUnauthorized)
	}
}

// Finding L760, assertion (2b): invalid/garbage token → 401 BEFORE upgrade.
func TestHandleWebSocket_InvalidTokenRejected(t *testing.T) {
	srv := newWSAuthServer(t)

	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, resp, err := dialer.Dial(wsAuthURL(t, srv, "not-a-real-jwt"), nil)
	if err == nil {
		_ = conn.Close()
		t.Fatal("dial with invalid token succeeded; want 401 rejection")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		got := -1
		if resp != nil {
			got = resp.StatusCode
		}
		t.Fatalf("invalid-token status = %d, want %d", got, http.StatusUnauthorized)
	}
}

// Finding L760, assertion (2c): token signed with the WRONG secret → 401.
func TestHandleWebSocket_WrongSecretTokenRejected(t *testing.T) {
	srv := newWSAuthServer(t)

	// Mint against a different secret than the server validates with.
	now := time.Now()
	claims := authz.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "animeenigma-test",
			Subject:   "user-9",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
		UserID:   "user-9",
		Username: "mallory",
		Role:     authz.RoleUser,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte("a-totally-different-secret"))
	if err != nil {
		t.Fatalf("sign wrong-secret token: %v", err)
	}

	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, resp, err := dialer.Dial(wsAuthURL(t, srv, signed), nil)
	if err == nil {
		_ = conn.Close()
		t.Fatal("dial with wrong-secret token succeeded; want 401 rejection")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		got := -1
		if resp != nil {
			got = resp.StatusCode
		}
		t.Fatalf("wrong-secret status = %d, want %d", got, http.StatusUnauthorized)
	}
}
