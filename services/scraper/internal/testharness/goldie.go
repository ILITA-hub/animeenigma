// Package testharness provides shared test utilities for the scraper service.
//
// The primary helper is New(t) which returns a *goldie.Goldie rooted at the
// scraper service's top-level testdata/ directory (not the per-package
// testdata/ subfolder goldie defaults to). All provider tests in plans 16+
// share this fixture root so refactors that move a provider package do not
// invalidate committed golden files.
//
// # Capturing or refreshing fixtures
//
// Run from the scraper service root:
//
//	cd services/scraper && go test -update ./...
//
// The repo Makefile wraps this as:
//
//	make capture-goldens
//
// Committed fixtures are the source of truth. CI runs without -update and
// must never regenerate them; a diff against testdata/ on CI is a test
// failure that the developer must investigate locally.
package testharness

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sebdah/goldie/v2"
)

// New returns a *goldie.Goldie configured to read fixtures from
// services/scraper/testdata/.
//
// The fixture directory is resolved at runtime via runtime.Caller so the
// helper continues to work from any sub-package: tests in
// internal/provider/animepahe and internal/provider/9anime will both find
// the same testdata/ root.
func New(t *testing.T) *goldie.Goldie {
	t.Helper()
	return goldie.New(t, goldie.WithFixtureDir(fixtureDir()))
}

// fixtureDir returns the absolute path of services/scraper/testdata/.
//
// runtime.Caller(0) gives the path of this file
// (services/scraper/internal/testharness/goldie.go); the testdata directory
// sits two levels up.
func fixtureDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		// Fallback to goldie's default — tests will still run, just from
		// the per-package testdata/ subfolder.
		return "testdata"
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}
