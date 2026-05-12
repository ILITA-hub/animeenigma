package config

import (
	"testing"
)

// TestConfig_LoadScraperServiceFromEnv asserts that when SCRAPER_SERVICE_URL
// is set, Load() honours the override. Plan 17-03 adds the ScraperService
// field on the ServiceURLs struct so the gateway can forward
// /api/admin/scraper/* to the scraper service.
func TestConfig_LoadScraperServiceFromEnv(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-do-not-use-in-prod")
	t.Setenv("SCRAPER_SERVICE_URL", "http://test:9999")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.Services.ScraperService, "http://test:9999"; got != want {
		t.Errorf("cfg.Services.ScraperService = %q; want %q", got, want)
	}
}

// TestConfig_LoadScraperServiceDefault asserts the docker-compose default
// resolves when no env override is present. The default MUST match the
// internal port the scraper service binds (scraper:8088 per Phase 15/16/17).
func TestConfig_LoadScraperServiceDefault(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-do-not-use-in-prod")
	// Explicitly unset SCRAPER_SERVICE_URL so a polluted ambient env from
	// another test pass cannot mask a regression here.
	t.Setenv("SCRAPER_SERVICE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.Services.ScraperService, "http://scraper:8088"; got != want {
		t.Errorf("cfg.Services.ScraperService = %q; want %q", got, want)
	}
}
