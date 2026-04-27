// Wave 0 RED test — references types/functions created by Wave 1 plan 01-03.
// This file SHOULD fail to compile until Wave 1 lands. Going green is the Wave 1
// acceptance gate (per phase 01 VALIDATION.md).
//
// Symbols referenced that DO NOT yet exist:
//   - transport.OptionalAuthMiddleware
//     (services/player/internal/transport/optional_auth.go — created in plan 01-03)
//
// Behavioral contract — the assertions below FREEZE the Wave 1 contract:
//   - missing Authorization header → next handler called WITHOUT claims attached
//   - valid Bearer JWT → next handler called WITH claims attached
//   - malformed/expired Bearer JWT → next handler called WITHOUT claims attached
//     (i.e. the middleware NEVER rejects with 401 — that's the inversion vs.
//      AuthMiddleware in router.go:138-160)

package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testJWTConfig returns a minimal JWTConfig suitable for token mint/validate
// in unit tests. Secret is fixed so we can sign and verify in the same process.
func testJWTConfig() authz.JWTConfig {
	return authz.JWTConfig{
		Secret:          "test-secret-key",
		Issuer:          "animeenigma-test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
}

// captureNextHandler returns an http.Handler that flips a boolean and snapshots
// claims-from-context, suitable for asserting middleware passthrough behavior.
func captureNextHandler() (http.Handler, *bool, **authz.Claims, *bool) {
	called := false
	var capturedClaims *authz.Claims
	hadClaims := false

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		c, ok := authz.ClaimsFromContext(r.Context())
		hadClaims = ok && c != nil
		capturedClaims = c
		w.WriteHeader(http.StatusOK)
	})
	return h, &called, &capturedClaims, &hadClaims
}

func TestOptionalAuth_NoAuthHeader_PassesThroughWithoutClaims(t *testing.T) {
	cfg := testJWTConfig()
	mw := OptionalAuthMiddleware(cfg)

	next, called, _, hadClaims := captureNextHandler()
	wrapped := mw(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	assert.True(t, *called, "next handler must be invoked even without Authorization header")
	assert.False(t, *hadClaims, "no claims should be attached when Authorization header is absent")
	assert.Equal(t, http.StatusOK, w.Code, "middleware must NOT reject the request — that's the inversion vs AuthMiddleware")
}

func TestOptionalAuth_ValidJWT_AttachesClaims(t *testing.T) {
	cfg := testJWTConfig()
	mw := OptionalAuthMiddleware(cfg)

	// Mint a valid token using the same JWTConfig the middleware will validate against.
	jm := authz.NewJWTManager(cfg)
	pair, err := jm.GenerateTokenPair("test-user", "tester", authz.RoleUser)
	require.NoError(t, err)

	next, called, capturedClaims, hadClaims := captureNextHandler()
	wrapped := mw(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	assert.True(t, *called, "next handler must be invoked")
	require.True(t, *hadClaims, "valid JWT should attach claims to context")
	require.NotNil(t, *capturedClaims)
	assert.Equal(t, "test-user", (*capturedClaims).UserID, "claims.UserID must match the minted token")
}

func TestOptionalAuth_MalformedJWT_PassesThroughWithoutClaims(t *testing.T) {
	cfg := testJWTConfig()
	mw := OptionalAuthMiddleware(cfg)

	next, called, _, hadClaims := captureNextHandler()
	wrapped := mw(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-token")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	assert.True(t, *called, "malformed JWT must NOT cause rejection — handler downstream falls back to X-Anon-ID")
	assert.False(t, *hadClaims, "malformed JWT must not produce claims")
	assert.Equal(t, http.StatusOK, w.Code, "middleware must not return 401 on bad token")
}
