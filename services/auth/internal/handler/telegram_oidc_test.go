package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// Callback with a provider ?error= must bounce to /auth?error=denied without
// touching any service (nil deps prove no dereference happens on this path).
func TestTelegramOIDCCallback_ProviderError(t *testing.T) {
	h := NewTelegramOIDCHandler(nil, nil, nil, logger.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/auth/telegram/oidc/callback?error=access_denied", nil)
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/auth?error=denied", rec.Header().Get("Location"))
}

// Callback without state/code is a malformed hit — generic telegram error.
// This check runs BEFORE the bind-cookie check, so it fires even with no
// cookie present.
func TestTelegramOIDCCallback_MissingParams(t *testing.T) {
	h := NewTelegramOIDCHandler(nil, nil, nil, logger.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/auth/telegram/oidc/callback", nil)
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/auth?error=telegram", rec.Header().Get("Location"))
}

// Callback with state+code but no bind cookie (different browser, expired,
// or blocked) must look like an expired attempt — same retryable error page
// — and never reach the service layer (nil deps prove no dereference).
func TestTelegramOIDCCallback_MissingBindCookie(t *testing.T) {
	h := NewTelegramOIDCHandler(nil, nil, nil, logger.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/auth/telegram/oidc/callback?state=abc&code=xyz", nil)
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/auth?error=expired", rec.Header().Get("Location"))
}
