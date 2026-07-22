package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCurve_ParityFixture asserts the Go curve algorithm against the shared
// cross-language fixture (test/fixtures/degradation_curve_parity.json). The same
// vectors are asserted by the Python side (services/stealth-scraper/tests/
// test_scaling.py::test_curve_parity_fixture), so a divergent edit to either
// piecewise-linear-floor(+epsilon) implementation is caught in both suites. The
// fixture cases sit within [1, max_pool] where the two agree (the service-local
// clamps differ and are tested separately).
func TestCurve_ParityFixture(t *testing.T) {
	path := parityFixturePath(t)
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("shared parity fixture not present (%v) — enforced in full-checkout CI", err)
	}
	var fx struct {
		Curve [][2]float64 `json:"curve"`
		Cases []struct {
			Score float64 `json:"score"`
			Cap   int     `json:"cap"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(blob, &fx))

	curve := make(Curve, 0, len(fx.Curve))
	for _, p := range fx.Curve {
		curve = append(curve, CurvePoint{Score: p[0], Cap: int(p[1])})
	}
	require.NotEmpty(t, fx.Cases)
	for _, c := range fx.Cases {
		assert.Equalf(t, c.Cap, curve.Cap(c.Score),
			"Go Curve.Cap(%.2f) must match the shared parity fixture", c.Score)
	}
}

// parityFixturePath resolves test/fixtures/... relative to THIS source file
// (robust to the test's working directory), four levels up from
// services/content-verify/internal/service/.
func parityFixturePath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(thisFile),
		"..", "..", "..", "..", "test", "fixtures", "degradation_curve_parity.json")
}
