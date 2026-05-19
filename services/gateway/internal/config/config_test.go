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

// TestDevMode_OnlyAllowedInDevEnvironments asserts that DevMode is only
// permitted when ENVIRONMENT is in the known dev allow-list. The previous
// deny-list-only guard (production/prod) let empty strings, misspellings,
// and staging silently slip through — DevMode bypasses admin auth, so this
// must fail closed. See audit Wave 1 (S9).
func TestDevMode_OnlyAllowedInDevEnvironments(t *testing.T) {
	cases := []struct {
		env     string
		devReq  bool
		devWant bool
	}{
		{"production", true, false},
		{"prod", true, false},
		{"staging", true, false}, // previously slipped through — now denied
		{"", true, false},        // previously slipped through — now denied
		{"PRD", true, false},     // misspelling — now denied
		{"development", true, true},
		{"dev", true, true},
		{"local", true, true},
		{"test", true, true},
		{"development", false, false}, // not requested → off
	}
	for _, c := range cases {
		t.Run(c.env+"/"+boolStr(c.devReq), func(t *testing.T) {
			t.Setenv("JWT_SECRET", "test-secret-do-not-use-in-prod")
			t.Setenv("ENVIRONMENT", c.env)
			t.Setenv("DEV_MODE", boolStr(c.devReq))
			cfg, err := Load()
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if cfg.DevMode != c.devWant {
				t.Errorf("ENVIRONMENT=%q DEV_MODE=%v → DevMode=%v, want %v",
					c.env, c.devReq, cfg.DevMode, c.devWant)
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
