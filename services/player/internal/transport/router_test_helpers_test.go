package transport

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/stretchr/testify/require"
)

// metrics.NewCollector registers its histograms with the global
// promauto.DefaultRegisterer — calling it twice in the same test binary
// panics with "duplicate metrics collector registration attempted". Tests
// share a single Collector via this sync.Once-guarded singleton.
var (
	sharedMetricsCollector     *metrics.Collector
	sharedMetricsCollectorOnce sync.Once
)

func zeroMetricsCollector(t *testing.T) *metrics.Collector {
	t.Helper()
	sharedMetricsCollectorOnce.Do(func() {
		sharedMetricsCollector = metrics.NewCollector("player-router-test")
	})
	require.NotNil(t, sharedMetricsCollector)
	return sharedMetricsCollector
}

// zeroJWTConfig is a deterministic JWT config for tests that never exercise
// the AuthMiddleware code path. The /internal route under test does NOT
// validate JWTs — these values are placeholders to satisfy the function
// signature.
func zeroJWTConfig() authz.JWTConfig {
	return authz.JWTConfig{
		Secret:          "router-internal-list-test-secret",
		Issuer:          "animeenigma-test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
}

// readRouterSource reads router.go's text so tests can assert structural
// properties (e.g. the /internal route is registered before the r.Route("/api"
// block) that chi exposes no introspection API for.
func readRouterSource(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) failed — cannot resolve test file path")
	dir := filepath.Dir(file)
	src, err := os.ReadFile(filepath.Join(dir, "router.go"))
	require.NoError(t, err, "failed to read router.go")
	return string(src)
}
