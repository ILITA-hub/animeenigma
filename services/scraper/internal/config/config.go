package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
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
// BaseURL defaults to https://animepahe.ru (the canonical host); Plan 16-01's
// connectivity note documents https://animepahe.com as a Cloudflare-fronted
// alias on networks where the direct host is blocked. Setting the env var at
// deploy time keeps the rotation restart-not-rebuild.
type AnimePaheConfig struct {
	BaseURL string
}

// Load reads configuration from environment variables, falling back to
// sensible defaults that work inside the docker-compose network.
//
// REVIEW.md WR-05: MEGACLOUD_EXTRACTOR_URL and (Phase 16) ANIMEPAHE_BASE_URL
// are validated at boot so an invalid value (e.g. missing scheme) is rejected
// immediately rather than surfacing deep inside MegacloudClient.Extract or
// animepahe.Provider.FindID on the first request. An empty URL is allowed
// (main.go warns on it).
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
			BaseURL: getEnv("ANIMEPAHE_BASE_URL", "https://animepahe.ru"),
		},
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
	if u := cfg.AnimePahe.BaseURL; u != "" {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, fmt.Errorf("invalid ANIMEPAHE_BASE_URL %q: %w", u, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid ANIMEPAHE_BASE_URL %q: missing scheme or host", u)
		}
	}
	return cfg, nil
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
