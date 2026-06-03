package config

import (
	"errors"
	"fmt"
	"log"
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
// Phase 16 plan 05 adds RedisConfig (cache backend) and AnimePaheConfig
// (provider-specific overrides).
type Config struct {
	Server             ServerConfig
	MegacloudExtractor MegacloudExtractorConfig
	Redis              RedisConfig
	AnimePahe          AnimePaheConfig
	Gogoanime          GogoanimeConfig
	AnimeKai           AnimeKaiConfig
	AllAnime           AllAnimeConfig
	AnimeFever         AnimeFeverConfig
	Miruro             MiruroConfig
	NineAnime          NineAnimeConfig
	DegradedProviders  DegradedProvidersConfig

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

	// Providers is the resolved provider-management config (scraper-providers.yaml,
	// or env fallback). Source of truth for enable/disable + reason/description.
	Providers ProvidersConfig
}

// DegradedProvidersConfig is the global kill-switch for providers known to be
// upstream-dead. Names in the set are NOT Register()-ed with the orchestrator,
// so failover skips them with zero per-request latency cost. Read from
// SCRAPER_DEGRADED_PROVIDERS (comma-separated, lowercase names).
//
// Use case: animepahe.ru is IP-blocked and animepahe.io is FingerprintJS-gated
// (2026-05-18); anitaku.to migrated to a different platform (anineko.to) the
// existing parser can't handle. Until per-provider fixes land, mark them
// degraded so the orchestrator stops wasting cycles on guaranteed failures.
//
// Flipping a provider OFF this list is a config-only restart — no rebuild.
type DegradedProvidersConfig struct {
	Names map[string]bool
}

// IsDegraded reports whether the named provider is in the kill-switch set.
// Name comparison is case-insensitive to match how providers identify
// themselves (Provider.Name() returns lowercase by convention).
func (d DegradedProvidersConfig) IsDegraded(name string) bool {
	if d.Names == nil {
		return false
	}
	return d.Names[strings.ToLower(name)]
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

// AnimePaheConfig is the per-provider override surface for animepahe.Provider.
//
// Phase 27 SCRAPER-HEAL-30: the Go parser no longer talks to animepahe.*
// directly. ResolverURL points at the stealth-Chromium sidecar
// (`services/animepahe-resolver/`) which owns the upstream challenge stack
// (DDoS-Guard, browser-fingerprint, etc.). Defaults to the docker-compose
// service name `http://animepahe-resolver:3000`. Override via
// SCRAPER_ANIMEPAHE_RESOLVER_URL when running on a non-default port or
// rotating the sidecar in place. The pre-Phase-27 BaseURL field (paired
// with the old upstream-URL env binding) has been removed entirely — there
// is no fallback to upstream animepahe.* hosts.
type AnimePaheConfig struct {
	ResolverURL string
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

// AnimeKaiConfig is the per-provider override surface for animekai.Provider
// (Phase 19 — gated, ESCAPE-HATCH path). Enabled defaults to FALSE in
// production. Toggle via SCRAPER_ANIMEKAI_ENABLED=true. BaseURL defaults to
// https://anikai.to (animekai.to 301-redirects here as of 2026-05-12).
// Override via SCRAPER_ANIMEKAI_BASE_URL when the mirror rotates.
// SCRAPER-KAI-05: flag is read at orchestrator startup; restart-not-rebuild
// is achieved via `docker compose restart scraper`.
type AnimeKaiConfig struct {
	Enabled bool
	BaseURL string
}

// AllAnimeConfig is the per-provider override surface for allanime.Provider
// (Phase 26 — SCRAPER-HEAL-25). Unlike AnimeKai, AllAnime ships always-on
// — there is no SCRAPER_ALLANIME_ENABLED gate. Operator can disable via
// SCRAPER_DEGRADED_PROVIDERS=allanime if the upstream goes hard down.
// BaseURL defaults to https://api.allanime.day; override via
// SCRAPER_ALLANIME_BASE_URL when the upstream rotates hostnames.
type AllAnimeConfig struct {
	BaseURL string
}

// AnimeFeverConfig is the per-provider override surface for
// animefever.Provider (Phase 28 — SCRAPER-HEAL-36). Always-on; operator
// can disable via SCRAPER_DEGRADED_PROVIDERS=animefever if upstream goes
// hard down. BaseURL defaults to https://animefever.cc; override via
// SCRAPER_ANIMEFEVER_BASE_URL when the upstream rotates hostnames.
type AnimeFeverConfig struct {
	BaseURL string
}

// NineAnimeConfig is the per-provider override surface for nineanime.Provider
// (Phase 28 — SCRAPER-HEAL-39). 9anime.me.uk is a brand-jack WordPress
// instance accepted as a low-quality, last-resort source per CONTEXT.md D2.
// BaseURL defaults to https://9anime.me.uk; override via
// SCRAPER_NINEANIME_BASE_URL when the upstream rotates hostnames. Operator
// kill via SCRAPER_DEGRADED_PROVIDERS=nineanime when upstream breaks
// (~6-month half-life expected).
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
// REVIEW.md WR-05: MEGACLOUD_EXTRACTOR_URL and (Phase 27)
// SCRAPER_ANIMEPAHE_RESOLVER_URL are validated at boot so an invalid value
// (e.g. missing scheme) is rejected immediately rather than surfacing deep
// inside MegacloudClient.Extract or animepahe.Provider.FindID on the first
// request. An empty URL is allowed (main.go warns on it).
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
		AnimePahe: AnimePaheConfig{
			ResolverURL: getEnv("SCRAPER_ANIMEPAHE_RESOLVER_URL", "http://animepahe-resolver:3000"),
		},
		Gogoanime: GogoanimeConfig{
			BaseURL:        getEnv("SCRAPER_GOGOANIME_BASE_URL", "https://anitaku.to"),
			ServerPriority: parseServerPriority(getEnv("SCRAPER_SERVER_PRIORITY", "streamhg,earnvids,vibeplayer")),
		},
		AnimeKai: AnimeKaiConfig{
			Enabled: getEnvBool("SCRAPER_ANIMEKAI_ENABLED", false),
			BaseURL: getEnv("SCRAPER_ANIMEKAI_BASE_URL", "https://anikai.to"),
		},
		AllAnime: AllAnimeConfig{
			BaseURL: getEnv("SCRAPER_ALLANIME_BASE_URL", "https://api.allanime.day"),
		},
		AnimeFever: AnimeFeverConfig{
			BaseURL: getEnv("SCRAPER_ANIMEFEVER_BASE_URL", "https://animefever.cc"),
		},
		Miruro: MiruroConfig{
			BaseURL:     getEnv("SCRAPER_MIRURO_BASE_URL", "https://www.miruro.tv"),
			ProxyURL:    getEnv("SCRAPER_MIRURO_PROXY_A", "https://pro.ultracloud.cc"),
			ProxyURLAlt: getEnv("SCRAPER_MIRURO_PROXY_B", "https://pru.ultracloud.cc"),
		},
		NineAnime: NineAnimeConfig{
			BaseURL: getEnv("SCRAPER_NINEANIME_BASE_URL", "https://9anime.me.uk"),
		},
		ProviderTimeout: getEnvDuration("SCRAPER_PROVIDER_TIMEOUT", 8*time.Second),
	}
	// Provider management config: scraper-providers.yaml is the source of truth.
	// Falls back to the legacy SCRAPER_DEGRADED_PROVIDERS env when the file path
	// is unset or the file is missing (zero-break migration). ISS-023.
	if providersPath := getEnv("SCRAPER_PROVIDERS_FILE", ""); providersPath != "" {
		_, statErr := os.Stat(providersPath)
		switch {
		case statErr == nil:
			pc, err := LoadProviders(providersPath)
			if err != nil {
				return nil, fmt.Errorf("scraper providers file: %w", err)
			}
			cfg.Providers = pc
			cfg.DegradedProviders = pc.toDegradedConfig()
		case errors.Is(statErr, os.ErrNotExist):
			// Missing file → env fallback (zero-break migration).
			cfg.DegradedProviders = parseDegradedProviders(getEnv("SCRAPER_DEGRADED_PROVIDERS", ""))
			cfg.Providers = providersFromDegraded(cfg.DegradedProviders, "env-fallback")
		default:
			// Path set but unreadable (permission/IO/is-a-directory) — fail fast
			// rather than silently abandon the operator's explicit source of truth.
			return nil, fmt.Errorf("scraper providers file %q: %w", providersPath, statErr)
		}
	} else {
		cfg.DegradedProviders = parseDegradedProviders(getEnv("SCRAPER_DEGRADED_PROVIDERS", ""))
		cfg.Providers = providersFromDegraded(cfg.DegradedProviders, "env")
	}
	if u := cfg.MegacloudExtractor.URL; u != "" {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("invalid MEGACLOUD_EXTRACTOR_URL %q: %w", u, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid MEGACLOUD_EXTRACTOR_URL %q: missing scheme or host", u)
		}
	}
	if u := cfg.AnimePahe.ResolverURL; u != "" {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("invalid SCRAPER_ANIMEPAHE_RESOLVER_URL %q: %w", u, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid SCRAPER_ANIMEPAHE_RESOLVER_URL %q: missing scheme or host", u)
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
	if u := cfg.AnimeKai.BaseURL; u != "" {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("invalid SCRAPER_ANIMEKAI_BASE_URL %q: %w", u, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid SCRAPER_ANIMEKAI_BASE_URL %q: missing scheme or host", u)
		}
	}
	if u := cfg.AnimeFever.BaseURL; u != "" {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("invalid SCRAPER_ANIMEFEVER_BASE_URL %q: %w", u, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid SCRAPER_ANIMEFEVER_BASE_URL %q: missing scheme or host", u)
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

// parseDegradedProviders splits a CSV list of provider names into a set.
// Whitespace is trimmed, names are lowercased, empties dropped. Empty input
// returns an empty set (no providers degraded — the production default).
//
// Example: SCRAPER_DEGRADED_PROVIDERS="gogoanime, animepahe" disables both
// from registration in main.go.
func parseDegradedProviders(csv string) DegradedProvidersConfig {
	m := make(map[string]bool)
	for _, p := range strings.Split(csv, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		m[p] = true
	}
	return DegradedProvidersConfig{Names: m}
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

// getEnvBool reads a boolean env var using strconv.ParseBool semantics.
// Accepts "1", "t", "T", "TRUE", "true", "True" → true; "0", "f", "F",
// "FALSE", "false", "False" → false. Unparseable values fall back to the
// default (matching the lenient getEnv / getEnvInt / getEnvDuration pattern),
// but the helper ALSO logs a WARN-level line on parse failure (WR-03) so
// an operator who typo'd the value (e.g. "yes-please" or "on") sees their
// value was rejected instead of silently shipping the default. Phase 19
// introduced this helper for SCRAPER_ANIMEKAI_ENABLED.
func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err == nil {
		return b
	}
	// WR-03: unparseable value — log a warning so the operator notices
	// their intent did not take effect. We do not return an error here
	// because callers (and downstream tests like TestLoad_AnimeKaiEnabledInvalid)
	// rely on the lenient fall-back-to-default convention.
	log.Printf("WARN config: %s=%q is not a valid boolean; falling back to default=%v (accepted values: 1/0, t/f, true/false, T/F, TRUE/FALSE, True/False)", key, val, defaultVal)
	return defaultVal
}
