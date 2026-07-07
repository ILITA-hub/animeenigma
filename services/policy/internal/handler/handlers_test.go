package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/transport"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// sharedCollector is instantiated once per test binary because
// libs/metrics.NewCollector registers to the global Prometheus registry via
// promauto — a second NewCollector call inside the same test process panics
// with "duplicate metrics collector registration". sync.Once gives every test
// in this file a clean, shared instance that NewRouter is happy with.
var (
	sharedCollectorOnce sync.Once
	sharedCollector     *metrics.Collector
)

func getSharedCollector() *metrics.Collector {
	sharedCollectorOnce.Do(func() {
		sharedCollector = metrics.NewCollector("policy_test")
	})
	return sharedCollector
}

// envelope mirrors libs/httputil.Response's {success,data} wrapper shape so
// tests can decode the payload out of it without importing libs/httputil's
// unexported internals.
type envelope[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}

func newTestServer(t *testing.T) (http.Handler, authz.JWTConfig) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.FeatureFlag{}))
	svc := service.NewPolicyService(repo.NewFeatureFlagRepository(db), logger.Default())
	require.NoError(t, svc.SeedDefaults(context.Background()))
	// AccessTokenTTL MUST be set — the zero value makes GenerateTokenPair mint a
	// token with ExpiresAt==IssuedAt, which authz.ValidateAccessToken treats as
	// already-expired (no leeway), turning every admin-token test into a 401.
	jwtCfg := authz.JWTConfig{Secret: "test-secret", Issuer: "animeenigma", AccessTokenTTL: 15 * time.Minute}
	router := transport.NewRouter(
		handler.NewAdminFlagsHandler(svc, logger.Default()),
		handler.NewPublicFlagsHandler(svc, logger.Default()),
		handler.NewInternalRulesetHandler(svc, logger.Default()),
		jwtCfg, logger.Default(), getSharedCollector(),
	)
	return router, jwtCfg
}

func adminToken(t *testing.T, cfg authz.JWTConfig) string {
	t.Helper()
	m := authz.NewJWTManager(cfg)
	pair, err := m.GenerateTokenPair("a1", "admin", authz.RoleAdmin, "sess1")
	require.NoError(t, err)
	return pair.AccessToken
}

func TestInternalRuleset_ok(t *testing.T) {
	router, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/policy/ruleset", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "fanfic")
}

func TestMine_anonymousSeesEveryoneOnly(t *testing.T) {
	router, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/policy/features/mine", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "anidle")
	require.NotContains(t, body, "fanfic")          // admin-only
	require.NotContains(t, body, "showcase-editor") // admin-only
	require.NotContains(t, body, "my-feedback")     // any-authenticated, not anon
}

func TestAdminFlags_requiresAdmin(t *testing.T) {
	router, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/policy/flags", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code) // no token
}

func TestAdminSetFlag_thenMineReflects(t *testing.T) {
	router, cfg := newTestServer(t)
	tok := adminToken(t, cfg)

	body := `{"roles":["admin"],"allowUsers":["oronemu"],"denyUsers":[],"roulette":false,"failSafe":"admin","label":"Fanfic"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/policy/flags/fanfic", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// oronemu (a normal user) now resolves fanfic as visible via allowUsers.
	m := authz.NewJWTManager(cfg)
	pair, err := m.GenerateTokenPair("oronemu", "oronemu", authz.RoleUser, "s2")
	require.NoError(t, err)
	req2 := httptest.NewRequest(http.MethodGet, "/api/policy/features/mine", nil)
	req2.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)

	// httputil.OK wraps payloads in the repo-wide {success,data} envelope, so
	// decode into that shape rather than domain.MineResponse directly.
	var resp envelope[domain.MineResponse]
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
	require.Contains(t, resp.Data.Visible, "fanfic")
}
