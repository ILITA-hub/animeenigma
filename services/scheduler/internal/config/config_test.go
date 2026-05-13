package config

import (
	"testing"
)

// TestLoad_CanaryDefaults asserts the three new canary-related JobsConfig
// fields default to the values the Phase 23 plan locked in: cron `0 3 * * *`,
// scraper base URL pointing at the docker-compose service name, and the
// canary-run log dir under the existing player_reports volume mount.
func TestLoad_CanaryDefaults(t *testing.T) {
	// Don't pre-set the env vars — let Load() pick up defaults.
	t.Setenv("SCRAPER_PLAYABILITY_CANARY_CRON", "")
	t.Setenv("SCRAPER_BASE_URL", "")
	t.Setenv("CANARY_REPORT_DIR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err = %v; want nil", err)
	}
	if got, want := cfg.Jobs.ScraperPlayabilityCanaryCron, "0 3 * * *"; got != want {
		t.Errorf("ScraperPlayabilityCanaryCron = %q; want %q", got, want)
	}
	if got, want := cfg.Jobs.ScraperBaseURL, "http://scraper:8088"; got != want {
		t.Errorf("ScraperBaseURL = %q; want %q", got, want)
	}
	if got, want := cfg.Jobs.CanaryReportDir, "/data/reports/canary-runs"; got != want {
		t.Errorf("CanaryReportDir = %q; want %q", got, want)
	}
}

// TestLoad_CanaryOverride asserts env-var overrides are honored. Mirrors the
// `getEnv` pattern used elsewhere in this package.
func TestLoad_CanaryOverride(t *testing.T) {
	t.Setenv("SCRAPER_PLAYABILITY_CANARY_CRON", "*/5 * * * *")
	t.Setenv("SCRAPER_BASE_URL", "http://scraper-test:9999")
	t.Setenv("CANARY_REPORT_DIR", "/tmp/canary-test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() err = %v; want nil", err)
	}
	if got, want := cfg.Jobs.ScraperPlayabilityCanaryCron, "*/5 * * * *"; got != want {
		t.Errorf("ScraperPlayabilityCanaryCron = %q; want %q", got, want)
	}
	if got, want := cfg.Jobs.ScraperBaseURL, "http://scraper-test:9999"; got != want {
		t.Errorf("ScraperBaseURL = %q; want %q", got, want)
	}
	if got, want := cfg.Jobs.CanaryReportDir, "/tmp/canary-test"; got != want {
		t.Errorf("CanaryReportDir = %q; want %q", got, want)
	}
}
