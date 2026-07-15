package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the scraper service configuration.
//
// Phase 15 plan 03 nests megacloud-extractor settings into their own struct so
// new providers' configs can land alongside without flattening the top level.
// Phase 16 plan 05 adds RedisConfig (cache backend). AnimePahe's per-provider
// config was removed on the 2026-06-26 Camoufox revival — its base_url comes
// from the DB roster (Providers.BaseURLOf) like the other browser providers.
type Config struct {
	Server             ServerConfig
	MegacloudExtractor MegacloudExtractorConfig
	Redis              RedisConfig
	Gogoanime          GogoanimeConfig
	AllAnime           AllAnimeConfig
	Miruro             MiruroConfig
	NineAnime          NineAnimeConfig

	// ProviderTimeout bounds how long the failover orchestrator waits on a
	// SINGLE provider before moving to the next one. Without it, one slow/hung
	// provider early in the chain (e.g. animepahe when animepahe.pw is down and
	// its resolver 502s through ~5 retries ≈ 55s) consumes the ENTIRE request
	// budget — so the catalog's SCRAPER_TIMEOUT (15s) kills the request before
	// failover ever reaches a healthy provider (allanime/miruro answer in <1s).
	// Bounding per-provider time lets the chain degrade gracefully. ISS-022.
	// Read from SCRAPER_PROVIDER_TIMEOUT; default 8s (a working provider answers
	// in well under 1s, so 8s is generous headroom while staying under the 15s
	// caller budget). Set 0 to disable the per-provider cap.
	ProviderTimeout time.Duration

	// BrowserProviderTimeout overrides ProviderTimeout for EVERY engine=browser
	// provider (miruro, animepahe, gogoanime, nineanime, + any future one — the
	// set is derived from the DB roster, not hardcoded, so a new browser provider
	// inherits it automatically). A browser provider's stealth-scraper warm
	// session must cold-solve a Cloudflare Turnstile challenge (up to
	// STEALTH_CHALLENGE_SOLVE_TIMEOUT_MS = 30s default) AND idle-expires after
	// STEALTH_SESSION_TTL_SECONDS (600s) — far shorter than the 6h health-probe
	// cadence — so every scheduled probe hits a cold solve. The shared 8s
	// ProviderTimeout always kills that cold solve before it finishes, marking
	// the provider perpetually "down" regardless of whether it actually works
	// (see docs/issues/provider-recovery-log.md 2026-07-04 for animepahe; miruro
	// hit the identical trap 2026-07-06). Fast HTTP providers KEEP the short 8s
	// chain budget (they answer in <1s, and a long budget would let one hung
	// HTTP provider starve the failover chain past the caller's 15s SCRAPER_
	// TIMEOUT — ISS-022). Read from SCRAPER_BROWSER_PROVIDER_TIMEOUT; default 35s
	// (30s solve budget + margin). Set 0 to fall back to the global ProviderTimeout.
	BrowserProviderTimeout time.Duration

	// Providers is the resolved provider-management config. The catalog database
	// is the runtime source of truth; this field starts with an offline fallback.
	Providers ProvidersConfig

	// CatalogURL is the base URL of the catalog service, used to fetch provider
	// config from /internal/scraper/providers. Empty disables remote config.
	CatalogURL string

	// StealthScraperURL is the base URL of the Camoufox stealth-scraper sidecar
	// (services/stealth-scraper), used to resolve providers whose DB `engine`
	// column is "browser". Non-secret service-discovery URL (Docker network).
	StealthScraperURL string

	// ProvidersRefresh is how often to re-fetch remote provider config. 0 = no
	// periodic refresh (boot-only).
	ProvidersRefresh time.Duration
}

// ServerConfig controls the HTTP listener.
type ServerConfig struct {
	Host string
	Port int
}

// Address returns the host:port the HTTP server binds to.
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// MegacloudExtractorConfig configures the HTTP client that talks to the
// docker/megacloud-extractor sidecar. URL defaults to the docker-compose
// service name; Timeout defaults to 15s to match the sidecar's internal
// req.setTimeout(15000) (see docker/megacloud-extractor/server.js).
type MegacloudExtractorConfig struct {
	URL     string
	Timeout time.Duration
}

// RedisConfig is the connection info for the libs/cache.RedisCache the scraper
// uses for malsync / episode / stream caches. Defaults mirror other services
// (catalog/player) so the docker-compose `redis:6379` block needs zero
// per-service overrides.
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// GogoanimeConfig is the per-provider override surface for the gogoanime.Provider
// (Phase 18 — pivots to Anitaku at anitaku.to). BaseURL defaults to
// https://anitaku.to; override via SCRAPER_GOGOANIME_BASE_URL when the mirror
// rotates. Invalid URL fails service boot.
//
// Phase 21 (SCRAPER-HEAL-03): ServerPriority is the CSV-driven priority list
// for gogoanime.ListServers — entries with extractor names appearing here
// are sorted to the front. Default: streamhg, earnvids, vibeplayer. Override
// via SCRAPER_SERVER_PRIORITY. Validation against the embeds registry
// happens at boot (main.go), not in config.Load — config stays
// registry-agnostic.
type GogoanimeConfig struct {
	BaseURL        string
	ServerPriority []string
}

// AllAnimeConfig is the per-provider override surface for the merged
// allanimeokru.Provider (Phase 26 — SCRAPER-HEAL-25; folded 2026-07-06 from
// the former standalone allanime + okru providers). Operator can
// disable/degrade it via the catalog `scraper_providers` DB table if the
// upstream goes hard down. BaseURL defaults to https://api.allanime.day
// (AllAnime's GraphQL discovery, still reused for ok.ru stream resolution);
// override via SCRAPER_ALLANIME_BASE_URL when the upstream rotates hostnames.
type AllAnimeConfig struct {
	BaseURL string
}

// NineAnimeConfig is the per-provider override surface for nineanime.Provider
// (Phase 28 — SCRAPER-HEAL-39). 9anime.me.uk is a brand-jack WordPress
// instance accepted as a low-quality, last-resort source per CONTEXT.md D2.
// BaseURL defaults to https://9anime.me.uk; override via
// SCRAPER_NINEANIME_BASE_URL when the upstream rotates hostnames. Operator
// kill/degrade via the catalog `scraper_providers` DB table when upstream
// breaks (~6-month half-life expected).
type NineAnimeConfig struct {
	BaseURL string
}

// MiruroConfig is the per-provider override surface for miruro.Provider
// (Phase 28 — SCRAPER-HEAL-37). BaseURL is the host the React SPA calls
// directly for the secure-pipe endpoint (`/api/secure/pipe`); ProxyURL +
// ProxyURLAlt are the env2.js VITE_PROXY_A / VITE_PROXY_B fallbacks
// (`pro.ultracloud.cc` / `pru.ultracloud.cc`). The fallbacks are
// preserved here for D7 allowlist parity and future failover; the SPA
// does NOT use them on the GET path (see SPIKE-MIRURO.md §Gate 2). Invalid
// URLs fail Load() at boot.
type MiruroConfig struct {
	BaseURL     string
	ProxyURL    string
	ProxyURLAlt string
}

// Load reads configuration from environment variables, falling back to
// sensible defaults that work inside the docker-compose network.
//
// REVIEW.md WR-05: MEGACLOUD_EXTRACTOR_URL is validated at boot so an invalid
// value (e.g. missing scheme) is rejected immediately rather than surfacing
// deep inside MegacloudClient.Extract on the first request. An empty URL is
// allowed (main.go warns on it).
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8088),
		},
		MegacloudExtractor: MegacloudExtractorConfig{
			URL:     getEnv("MEGACLOUD_EXTRACTOR_URL", "http://megacloud-extractor:3200"),
			Timeout: getEnvDuration("MEGACLOUD_EXTRACTOR_TIMEOUT", 15*time.Second),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "redis"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Gogoanime: GogoanimeConfig{
			BaseURL:        getEnv("SCRAPER_GOGOANIME_BASE_URL", "https://anitaku.to"),
			ServerPriority: parseServerPriority(getEnv("SCRAPER_SERVER_PRIORITY", "streamhg,earnvids,vibeplayer")),
		},
		AllAnime: AllAnimeConfig{
			BaseURL: getEnv("SCRAPER_ALLANIME_BASE_URL", "https://api.allanime.day"),
		},
		Miruro: MiruroConfig{
			BaseURL:     getEnv("SCRAPER_MIRURO_BASE_URL", "https://www.miruro.tv"),
			ProxyURL:    getEnv("SCRAPER_MIRURO_PROXY_A", "https://pro.ultracloud.cc"),
			ProxyURLAlt: getEnv("SCRAPER_MIRURO_PROXY_B", "https://pru.ultracloud.cc"),
		},
		NineAnime: NineAnimeConfig{
			BaseURL: getEnv("SCRAPER_NINEANIME_BASE_URL", "https://9anime.me.uk"),
		},
		ProviderTimeout:        getEnvDuration("SCRAPER_PROVIDER_TIMEOUT", 8*time.Second),
		BrowserProviderTimeout: getEnvDuration("SCRAPER_BROWSER_PROVIDER_TIMEOUT", 35*time.Second),
	}
	cfg.CatalogURL = getEnv("CATALOG_URL", "")
	cfg.StealthScraperURL = getEnv("STEALTH_SCRAPER_URL", "http://stealth-scraper:3000")
	if d, err := time.ParseDuration(getEnv("SCRAPER_PROVIDERS_REFRESH", "60s")); err == nil {
		cfg.ProvidersRefresh = d
	}
	// The catalog Postgres `scraper_providers` table is fetched in main.go via
	// LoadProvidersRemote and hot-reloaded every SCRAPER_PROVIDERS_REFRESH. Until
	// catalog answers, start from the compile-time all-enabled fallback.
	cfg.Providers = allProvidersEnabled("default")
	if u := cfg.MegacloudExtractor.URL; u != "" {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("invalid MEGACLOUD_EXTRACTOR_URL %q: %w", u, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid MEGACLOUD_EXTRACTOR_URL %q: missing scheme or host", u)
		}
	}
	if u := cfg.Gogoanime.BaseURL; u != "" {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("invalid SCRAPER_GOGOANIME_BASE_URL %q: %w", u, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid SCRAPER_GOGOANIME_BASE_URL %q: missing scheme or host", u)
		}
	}
	// Phase 28 — NineAnime URL validation. SCRAPER_NINEANIME_BASE_URL fails
	// Load() at boot when malformed so an operator typo doesn't surface as
	// a deferred provider-down minutes after deploy.
	if u := cfg.NineAnime.BaseURL; u != "" {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("invalid SCRAPER_NINEANIME_BASE_URL %q: %w", u, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid SCRAPER_NINEANIME_BASE_URL %q: missing scheme or host", u)
		}
	}
	// Phase 28 — Miruro URL validation. Three independent env vars all
	// validated identically; an empty value falls back to the default and
	// is therefore guaranteed non-empty by the time we get here.
	miruroURLs := []struct {
		name string
		val  string
	}{
		{"SCRAPER_MIRURO_BASE_URL", cfg.Miruro.BaseURL},
		{"SCRAPER_MIRURO_PROXY_A", cfg.Miruro.ProxyURL},
		{"SCRAPER_MIRURO_PROXY_B", cfg.Miruro.ProxyURLAlt},
	}
	for _, m := range miruroURLs {
		if m.val == "" {
			continue
		}
		parsed, err := url.Parse(m.val)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: %w", m.name, m.val, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid %s %q: missing scheme or host", m.name, m.val)
		}
	}
	return cfg, nil
}

// parseServerPriority splits a CSV priority spec into a normalized slice.
// Whitespace is trimmed, case is lowered, and empty entries (from leading
// commas / consecutive commas / trailing commas) are dropped. Empty input
// returns the canonical default ["streamhg","earnvids","vibeplayer"].
//
// Phase 21 SCRAPER-HEAL-03. Validation against the embeds registry's
// known extractor names happens in services/scraper/cmd/scraper-api/main.go
// — config.Load stays registry-agnostic so unit tests don't need to wire
// the full extractor set.
func parseServerPriority(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return []string{"streamhg", "earnvids", "vibeplayer"}
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{"streamhg", "earnvids", "vibeplayer"}
	}
	return out
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
