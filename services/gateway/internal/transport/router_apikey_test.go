package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
)

// fakeAuthService stands in for the real auth service's
// POST /internal/resolve-api-key endpoint. It returns a fixed
// (user_id, username, role) triple wrapped in the {"data":{...}} envelope
// that resolveApiKey() in router.go expects.
//
// The fake records every body it received so tests can assert that the
// gateway sent the right raw token through.
type fakeAuthService struct {
	server         *httptest.Server
	receivedTokens []string
	userID         string
	username       string
	role           authz.Role
}

func newFakeAuthService(t *testing.T, userID, username string, role authz.Role) *fakeAuthService {
	t.Helper()
	fas := &fakeAuthService{
		userID:   userID,
		username: username,
		role:     role,
	}
	fas.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/resolve-api-key" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			APIKey string `json:"api_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fas.receivedTokens = append(fas.receivedTokens, body.APIKey)

		resp := map[string]any{
			"data": map[string]any{
				"user_id":  fas.userID,
				"username": fas.username,
				"role":     fas.role,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	return fas
}

func (f *fakeAuthService) URL() string { return f.server.URL }
func (f *fakeAuthService) Close()       { f.server.Close() }

// extractMintedTokenFromHeader peels "Bearer <jwt>" off the rewritten
// Authorization header that JWTValidationMiddleware sets on its way to the
// next handler. The downstream chain only sees this minted JWT — never the
// original ak_*.
func extractMintedTokenFromHeader(t *testing.T, h http.Header) string {
	t.Helper()
	auth := h.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		t.Fatalf("Authorization header missing Bearer prefix: %q", auth)
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

// decodeClaims parses the minted JWT using the same JWTManager the gateway
// uses (symmetric HS256 with the test secret) so we can read the SID claim.
func decodeClaims(t *testing.T, tokenString string) *authz.Claims {
	t.Helper()
	mgr := authz.NewJWTManager(gatewayTestJWTConfig())
	claims, err := mgr.ValidateAccessToken(tokenString)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	return claims
}

// TestJWTValidationMiddleware_APIKeyMintsJWTWithDerivedSessionID — the main
// mint site (router.go:392). An ak_* request gets resolved, a JWT is minted,
// and the JWT's "sid" claim equals deriveAPIKeySessionID(userID, rawToken, now).
//
// We can't fix the exact "now" the middleware uses, but the derivation is
// stable across the UTC day, so we compute the expected SID using both
// today's UTC date and "today-or-tomorrow" tolerance to handle the rare
// midnight-boundary flake.
func TestJWTValidationMiddleware_APIKeyMintsJWTWithDerivedSessionID(t *testing.T) {
	rawAPIKey := "ak_test_apikeyderivation_0123456789abcdef"
	userID := "user-ak-1"
	fas := newFakeAuthService(t, userID, "ak_user", authz.RoleUser)
	defer fas.Close()

	// Inner sink: captures the rewritten Authorization header so we can
	// decode the minted JWT and read the SessionID claim.
	type capture struct {
		mintedAuth string
		ctxClaims  *authz.Claims
	}
	cap := &capture{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.mintedAuth = r.Header.Get("Authorization")
		if c, ok := authz.ClaimsFromContext(r.Context()); ok {
			cap.ctxClaims = c
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := JWTValidationMiddleware(gatewayTestJWTConfig(), fas.URL())
	handler := mw(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+rawAPIKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	if cap.mintedAuth == "" {
		t.Fatal("downstream did not see a rewritten Authorization header")
	}

	minted := strings.TrimPrefix(cap.mintedAuth, "Bearer ")
	claims := decodeClaims(t, minted)

	if claims.UserID != userID {
		t.Errorf("claims.UserID = %q; want %q", claims.UserID, userID)
	}
	if claims.SessionID == "" {
		t.Fatal("claims.SessionID is empty — WV3-T1 regression: derived SID not wired")
	}

	// Compute expected SID across two adjacent UTC days to absorb any
	// midnight crossing between mint-time and assertion-time.
	now := time.Now()
	expectedToday := deriveAPIKeySessionID(userID, rawAPIKey, now)
	expectedYesterday := deriveAPIKeySessionID(userID, rawAPIKey, now.Add(-24*time.Hour))
	expectedTomorrow := deriveAPIKeySessionID(userID, rawAPIKey, now.Add(24*time.Hour))

	if claims.SessionID != expectedToday &&
		claims.SessionID != expectedYesterday &&
		claims.SessionID != expectedTomorrow {
		t.Errorf("claims.SessionID = %q; expected one of (today=%q, yesterday=%q, tomorrow=%q)",
			claims.SessionID, expectedToday, expectedYesterday, expectedTomorrow)
	}

	// Auth service must have received the raw ak_* token, not the minted JWT.
	if len(fas.receivedTokens) != 1 || fas.receivedTokens[0] != rawAPIKey {
		t.Errorf("auth service receivedTokens = %v; want [%q]", fas.receivedTokens, rawAPIKey)
	}
}

// TestJWTValidationMiddleware_APIKeyTwoCallsSameDay — two consecutive ak_*
// requests on the same UTC day produce the same SID (acceptance criterion:
// audit-log correlation across calls).
func TestJWTValidationMiddleware_APIKeyTwoCallsSameDay(t *testing.T) {
	rawAPIKey := "ak_test_samedaystable_0123456789abcdef"
	userID := "user-ak-2"
	fas := newFakeAuthService(t, userID, "ak_user", authz.RoleUser)
	defer fas.Close()

	var capturedAuths []string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuths = append(capturedAuths, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	})
	handler := JWTValidationMiddleware(gatewayTestJWTConfig(), fas.URL())(inner)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/whatever", nil)
		req.Header.Set("Authorization", "Bearer "+rawAPIKey)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("iter %d: status = %d; want 200", i, rec.Code)
		}
	}

	if len(capturedAuths) != 2 {
		t.Fatalf("expected 2 captured auths, got %d", len(capturedAuths))
	}
	claims1 := decodeClaims(t, strings.TrimPrefix(capturedAuths[0], "Bearer "))
	claims2 := decodeClaims(t, strings.TrimPrefix(capturedAuths[1], "Bearer "))

	if claims1.SessionID == "" || claims2.SessionID == "" {
		t.Fatalf("empty SID — claims1.sid=%q claims2.sid=%q", claims1.SessionID, claims2.SessionID)
	}
	if claims1.SessionID != claims2.SessionID {
		t.Errorf("same-day SIDs differ: %q vs %q", claims1.SessionID, claims2.SessionID)
	}
}

// TestOptionalJWTValidationMiddleware_APIKeyMintsJWTWithDerivedSessionID —
// the optional-auth mint site (router.go:448). Same contract: ak_* → JWT
// with non-empty derived SID.
func TestOptionalJWTValidationMiddleware_APIKeyMintsJWTWithDerivedSessionID(t *testing.T) {
	rawAPIKey := "ak_test_optionalauthderivation_0123456789ab"
	userID := "user-ak-3"
	fas := newFakeAuthService(t, userID, "ak_user", authz.RoleUser)
	defer fas.Close()

	var capturedAuth string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	handler := OptionalJWTValidationMiddleware(gatewayTestJWTConfig(), fas.URL())(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/users/recs", nil)
	req.Header.Set("Authorization", "Bearer "+rawAPIKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	minted := strings.TrimPrefix(capturedAuth, "Bearer ")
	claims := decodeClaims(t, minted)

	if claims.UserID != userID {
		t.Errorf("claims.UserID = %q; want %q", claims.UserID, userID)
	}
	if claims.SessionID == "" {
		t.Fatal("claims.SessionID is empty — WV3-T1 regression on OptionalJWTValidationMiddleware")
	}
	now := time.Now()
	expectedToday := deriveAPIKeySessionID(userID, rawAPIKey, now)
	expectedYesterday := deriveAPIKeySessionID(userID, rawAPIKey, now.Add(-24*time.Hour))
	expectedTomorrow := deriveAPIKeySessionID(userID, rawAPIKey, now.Add(24*time.Hour))
	if claims.SessionID != expectedToday &&
		claims.SessionID != expectedYesterday &&
		claims.SessionID != expectedTomorrow {
		t.Errorf("OptionalJWT minted SessionID = %q; not in adjacent-day set", claims.SessionID)
	}
}

// TestJWTValidationMiddleware_PasswordLoginJWTUnchanged — regression check:
// non-ak_ JWTs (the standard password-login path) are validated, NOT
// reminted, and their SessionID claim is whatever the signer put there
// (empty when no session). Confirms we didn't touch the JWT-path.
func TestJWTValidationMiddleware_PasswordLoginJWTUnchanged(t *testing.T) {
	// Sign a token using the test secret — same shape password-login emits.
	originalToken := signTestJWT(t, authz.RoleUser)

	var capturedAuth string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})
	// authServiceURL is unused for non-ak_ tokens, but must be non-empty.
	handler := JWTValidationMiddleware(gatewayTestJWTConfig(), "http://unused")(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.Header.Set("Authorization", "Bearer "+originalToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	// Header must be the SAME token — no remint on JWT path.
	if capturedAuth != "Bearer "+originalToken {
		t.Errorf("password-login JWT should pass through unchanged.\n got = %q\nwant = %q",
			capturedAuth, "Bearer "+originalToken)
	}
}
