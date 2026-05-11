// Package golint runs CI-time lint checks against the scraper service's own
// build manifest (go.mod). It is test-only — there is no exported runtime
// behavior. The forbidden-dependency lint exists because D-DEC §5 explicitly
// rejects anti-bot stacks (chromedp, rod, utls, tls-client, playwright,
// cloudscraper, flaresolverr) and we want CI to FAIL any PR that adds them,
// not just a code-review check that a reviewer might miss.
//
// This test runs as part of `go test ./services/scraper/...` on every CI
// build. SCRAPER-FOUND-09 / ROADMAP §15 success #5.
package golint

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

// forbiddenPrefixes triggers a violation if any go.mod require line's module
// path starts with one of these. Using prefix-match catches cousins
// (e.g. github.com/chromedp/cdproto under github.com/chromedp/).
var forbiddenPrefixes = []string{
	"github.com/chromedp/",
	"github.com/go-rod/rod",
	"github.com/refraction-networking/utls",
	"github.com/bogdanfinn/",
	"github.com/playwright-community/playwright-go",
}

// forbiddenSubstrings triggers a violation if the module path contains one of
// these substrings anywhere. Used for projects that may live under different
// orgs (cloudscraper_go, flaresolverr-go, etc.).
var forbiddenSubstrings = []string{
	"cloudscraper",
	"flaresolverr",
}

// checkForbidden returns the matched forbidden patterns (prefix or substring)
// for any require entry in the given go.mod file. Empty slice = clean.
func checkForbidden(mf *modfile.File) []string {
	var hits []string
	for _, r := range mf.Require {
		path := r.Mod.Path
		for _, p := range forbiddenPrefixes {
			if strings.HasPrefix(path, p) {
				hits = append(hits, p)
			}
		}
		for _, s := range forbiddenSubstrings {
			if strings.Contains(path, s) {
				hits = append(hits, s)
			}
		}
	}
	return hits
}

// resolveScraperGoMod walks up from this test file's location to
// services/scraper/go.mod. Anchoring via runtime.Caller(0) makes the test
// robust to being invoked from anywhere (go test ./..., a single-pkg invoke,
// or with -cwd override).
func resolveScraperGoMod(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	// thisFile = services/scraper/internal/golint/forbidden_deps_test.go
	// Walk two levels up to services/scraper/, then append go.mod.
	dir := filepath.Dir(thisFile)            // .../internal/golint
	dir = filepath.Dir(dir)                  // .../internal
	dir = filepath.Dir(dir)                  // .../scraper
	goMod := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(goMod); err != nil {
		t.Fatalf("expected go.mod at %q: %v", goMod, err)
	}
	return goMod
}

// parseFixture is a small helper for fixture-based positive-catch tests.
func parseFixture(t *testing.T, fixture string) *modfile.File {
	t.Helper()
	mf, err := modfile.Parse("fixture-go.mod", []byte(fixture), nil)
	if err != nil {
		t.Fatalf("modfile.Parse fixture: %v", err)
	}
	return mf
}

// TestForbiddenDeps_RealGoMod is the CI gate: the real services/scraper/go.mod
// MUST NOT contain any forbidden module. If a PR adds one, this fails.
func TestForbiddenDeps_RealGoMod(t *testing.T) {
	t.Parallel()
	path := resolveScraperGoMod(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	mf, err := modfile.Parse(path, data, nil)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	hits := checkForbidden(mf)
	if len(hits) > 0 {
		t.Fatalf("services/scraper/go.mod contains forbidden modules: %v\n"+
			"See D-DEC §5 Rejected — SCRAPER-FOUND-09. Anti-bot tooling "+
			"(chromedp, rod, utls, tls-client, playwright, cloudscraper, "+
			"flaresolverr) is forbidden.", hits)
	}
}

// fixtureWith builds a minimal valid go.mod string with the given require
// path. The go directive is fixed at 1.23 to match the workspace floor.
func fixtureWith(modulePath, version string) string {
	return "module example.com/x\n\ngo 1.23\n\nrequire " + modulePath + " " + version + "\n"
}

// TestForbiddenDeps_PositiveCatch_Chromedp proves the lint catches chromedp.
// This is one of the deliberate-red tests required by ROADMAP §15 success #5.
func TestForbiddenDeps_PositiveCatch_Chromedp(t *testing.T) {
	t.Parallel()
	mf := parseFixture(t, fixtureWith("github.com/chromedp/chromedp", "v0.9.5"))
	hits := checkForbidden(mf)
	if len(hits) == 0 {
		t.Fatal("expected forbidden-deps lint to catch github.com/chromedp/chromedp; got no hits")
	}
}

// TestForbiddenDeps_PositiveCatch_ChromedpCdproto proves prefix-match catches
// cousins under the chromedp org.
func TestForbiddenDeps_PositiveCatch_ChromedpCdproto(t *testing.T) {
	t.Parallel()
	mf := parseFixture(t, fixtureWith("github.com/chromedp/cdproto", "v0.0.0-20240801214329-3f85d328b335"))
	hits := checkForbidden(mf)
	if len(hits) == 0 {
		t.Fatal("expected forbidden-deps lint to catch github.com/chromedp/cdproto via prefix match")
	}
}

// TestForbiddenDeps_PositiveCatch_Rod proves the lint catches go-rod/rod.
func TestForbiddenDeps_PositiveCatch_Rod(t *testing.T) {
	t.Parallel()
	mf := parseFixture(t, fixtureWith("github.com/go-rod/rod", "v0.114.5"))
	hits := checkForbidden(mf)
	if len(hits) == 0 {
		t.Fatal("expected forbidden-deps lint to catch github.com/go-rod/rod")
	}
}

// TestForbiddenDeps_PositiveCatch_UTLS proves the lint catches utls.
func TestForbiddenDeps_PositiveCatch_UTLS(t *testing.T) {
	t.Parallel()
	mf := parseFixture(t, fixtureWith("github.com/refraction-networking/utls", "v1.6.7"))
	hits := checkForbidden(mf)
	if len(hits) == 0 {
		t.Fatal("expected forbidden-deps lint to catch github.com/refraction-networking/utls")
	}
}

// TestForbiddenDeps_PositiveCatch_TLSClient proves the lint catches
// bogdanfinn/tls-client.
func TestForbiddenDeps_PositiveCatch_TLSClient(t *testing.T) {
	t.Parallel()
	mf := parseFixture(t, fixtureWith("github.com/bogdanfinn/tls-client", "v1.7.10"))
	hits := checkForbidden(mf)
	if len(hits) == 0 {
		t.Fatal("expected forbidden-deps lint to catch github.com/bogdanfinn/tls-client")
	}
}

// TestForbiddenDeps_PositiveCatch_TLSClientFhttp proves prefix-match catches
// the fhttp cousin under bogdanfinn.
func TestForbiddenDeps_PositiveCatch_TLSClientFhttp(t *testing.T) {
	t.Parallel()
	mf := parseFixture(t, fixtureWith("github.com/bogdanfinn/fhttp", "v0.5.32"))
	hits := checkForbidden(mf)
	if len(hits) == 0 {
		t.Fatal("expected forbidden-deps lint to catch github.com/bogdanfinn/fhttp via prefix match")
	}
}

// TestForbiddenDeps_PositiveCatch_Playwright proves the lint catches the Go
// playwright bindings.
func TestForbiddenDeps_PositiveCatch_Playwright(t *testing.T) {
	t.Parallel()
	mf := parseFixture(t, fixtureWith("github.com/playwright-community/playwright-go", "v0.4501.1"))
	hits := checkForbidden(mf)
	if len(hits) == 0 {
		t.Fatal("expected forbidden-deps lint to catch github.com/playwright-community/playwright-go")
	}
}

// TestForbiddenDeps_StringMatch_Cloudscraper proves substring-match catches
// cloudscraper-style modules regardless of org.
func TestForbiddenDeps_StringMatch_Cloudscraper(t *testing.T) {
	t.Parallel()
	mf := parseFixture(t, fixtureWith("github.com/Anorov/cloudscraper_go", "v0.0.1"))
	hits := checkForbidden(mf)
	if len(hits) == 0 {
		t.Fatal("expected forbidden-deps lint to catch cloudscraper substring")
	}
}

// TestForbiddenDeps_StringMatch_Flaresolverr proves substring-match catches
// flaresolverr-related modules.
func TestForbiddenDeps_StringMatch_Flaresolverr(t *testing.T) {
	t.Parallel()
	mf := parseFixture(t, fixtureWith("github.com/FlareSolverr/flaresolverr-go", "v0.0.1"))
	hits := checkForbidden(mf)
	if len(hits) == 0 {
		t.Fatal("expected forbidden-deps lint to catch flaresolverr substring")
	}
}

// TestForbiddenDeps_AllowedDepsPass verifies the lint does NOT false-positive
// on the legitimate dependencies the scraper actually uses.
func TestForbiddenDeps_AllowedDepsPass(t *testing.T) {
	t.Parallel()
	fixture := `module example.com/x

go 1.23

require (
	github.com/PuerkitoBio/goquery v1.10.3
	github.com/hashicorp/go-retryablehttp v0.7.7
	github.com/sebdah/goldie/v2 v2.5.5
	github.com/dop251/goja v0.0.0-20240220182346-e401ed450204
	golang.org/x/time v0.5.0
	golang.org/x/net v0.39.0
)
`
	mf := parseFixture(t, fixture)
	hits := checkForbidden(mf)
	if len(hits) > 0 {
		t.Errorf("expected zero hits for allowed deps; got %v", hits)
	}
}
