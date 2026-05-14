package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminTestJWTConfig builds a deterministic JWT config for the middleware tests.
// Phase 14 (REC-ADMIN-01).
func adminTestJWTConfig() authz.JWTConfig {
	return authz.JWTConfig{
		Secret:          "phase14-admin-test-secret",
		Issuer:          "animeenigma",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
}

// chainAdminRouter mounts AuthMiddleware -> AdminRoleMiddleware -> stub handler
// so the tests exercise the same chain the production /api/admin/recs group
// does.
func chainAdminRouter(jwtCfg authz.JWTConfig) http.Handler {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(jwtCfg))
		r.Use(AdminRoleMiddleware)
		r.Get("/admin/probe", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
	})
	return r
}

func TestAdminRoleMiddleware_MissingTokenReturns401(t *testing.T) {
	cfg := adminTestJWTConfig()
	r := chainAdminRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"missing Authorization header must yield 401 from AuthMiddleware before AdminRoleMiddleware runs")
}

func TestAdminRoleMiddleware_InvalidTokenReturns401(t *testing.T) {
	cfg := adminTestJWTConfig()
	r := chainAdminRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"invalid JWT must yield 401 from AuthMiddleware before AdminRoleMiddleware runs")
}

func TestAdminRoleMiddleware_UserRoleReturns403(t *testing.T) {
	cfg := adminTestJWTConfig()
	jwtMgr := authz.NewJWTManager(cfg)
	pair, err := jwtMgr.GenerateTokenPair("user-uuid", "user", authz.RoleUser, "")
	require.NoError(t, err)

	r := chainAdminRouter(cfg)
	req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"valid JWT with role=user must yield 403 from AdminRoleMiddleware")
}

func TestAdminRoleMiddleware_AdminRoleReturns200(t *testing.T) {
	cfg := adminTestJWTConfig()
	jwtMgr := authz.NewJWTManager(cfg)
	pair, err := jwtMgr.GenerateTokenPair("admin-uuid", "admin", authz.RoleAdmin, "")
	require.NoError(t, err)

	r := chainAdminRouter(cfg)
	req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"valid JWT with role=admin must reach the next handler (200)")
	assert.Equal(t, "ok", w.Body.String())
}
