package testharness

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewReturnsGoldie proves the helper compiles, that goldie v2 links into
// the build, and that the returned tester is non-nil.
func TestNewReturnsGoldie(t *testing.T) {
	g := New(t)
	require.NotNil(t, g, "New(t) must return a non-nil *goldie.Goldie")
}

// TestGoldieFixtureDir asserts the fixture directory resolves under a
// top-level testdata/ folder of the scraper service — not the per-package
// testdata/ subfolder goldie defaults to. This catches accidental moves of
// the testdata/ root in future refactors.
func TestGoldieFixtureDir(t *testing.T) {
	dir := fixtureDir()
	require.NotEmpty(t, dir, "fixtureDir() must return a non-empty path")

	clean := filepath.Clean(dir)
	require.True(
		t,
		strings.HasSuffix(clean, "testdata"),
		"fixture dir %q must end in testdata", clean,
	)

	// And the parent of that testdata/ should be the scraper service root,
	// i.e. should end in services/scraper.
	parent := filepath.Clean(filepath.Dir(clean))
	require.True(
		t,
		strings.HasSuffix(parent, filepath.Join("services", "scraper")),
		"testdata parent %q must be services/scraper", parent,
	)
}
