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
func TestTelegramOIDCCallback_MissingParams(t *testing.T) {
	h := NewTelegramOIDCHandler(nil, nil, nil, logger.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/auth/telegram/oidc/callback", nil)
	rec := httptest.NewRecorder()

	h.Callback(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/auth?error=telegram", rec.Header().Get("Location"))
}
