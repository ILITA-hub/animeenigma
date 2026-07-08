package config

import (
	"os"
	"testing"
	"time"
)

// TestLoad_ScraperTimeout_ExceedsBrowserProviderBudget guards against a
// regression class already hit once in production (2026-07-08 animepahe
// recovery): the scraper microservice grants engine=browser providers
// (animepahe, gogoanime, miruro, nineanime) a 35s per-provider failover
// budget (SCRAPER_BROWSER_PROVIDER_TIMEOUT, docker/docker-compose.yml) for
// a cold Cloudflare/Turnstile solve. If catalog's own outbound client to
// the scraper service (SCRAPER_TIMEOUT) is shorter than that, catalog kills
// the request and closes the connection before the scraper's own budget
// ever gets to finish — so the browser-provider budget is silently
// defeated by the layer above it, and any prefer=<browser-provider> call
// (including the automated health probe) fails every time, independent of
// whether the provider actually works.
func TestLoad_ScraperTimeout_ExceedsBrowserProviderBudget(t *testing.T) {
	os.Unsetenv("SCRAPER_TIMEOUT")
	t.Setenv("JWT_SECRET", "test-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	const browserProviderBudget = 35 * time.Second
	if cfg.Scraper.Timeout <= browserProviderBudget {
		t.Fatalf("Scraper.Timeout = %s, want > %s (SCRAPER_BROWSER_PROVIDER_TIMEOUT) so catalog never cuts off a browser-engine provider's cold-solve attempt before the scraper's own per-provider budget does",
			cfg.Scraper.Timeout, browserProviderBudget)
	}
}
