package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

// Config is the library service top-level config. Phase 2 adds the
// Nyaa + AnimeTosho + LibrarySearch sub-configs that drive the new
// search endpoint. Phases 3-4 will extend this further with torrent
// client + ffmpeg + MinIO knobs (workstream raw-jp / v0.2).
type Config struct {
	Server        ServerConfig
	Database      database.Config
	JWT           authz.JWTConfig
	Nyaa          NyaaConfig
	AnimeTosho    AnimeToshoConfig
	LibrarySearch LibrarySearchConfig
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// NyaaConfig and AnimeToshoConfig are the per-client knobs. The default
// timeout + UA are shared (both providers tolerate the same User-Agent
// and 15s is appropriate for torrent indexers — slower and more
// variable than streaming APIs).
type NyaaConfig struct {
	BaseURL     string
	HTTPTimeout time.Duration
	UserAgent   string
}

type AnimeToshoConfig struct {
	BaseURL     string
	HTTPTimeout time.Duration
	UserAgent   string
}

// LibrarySearchConfig holds limits documented for the operator; the
// aggregator currently enforces these via package-level constants in
// internal/service/search.go. The struct is informational — Phase 3+
// may promote these to runtime config.
type LibrarySearchConfig struct {
	DefaultLimit int
	MaxLimit     int
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	// Phase 2: shared timeout + UA flow into both clients via env
	// (LIBRARY_SEARCH_TIMEOUT / LIBRARY_SEARCH_UA). Per-provider
	// overrides aren't useful in practice — both upstreams behave the
	// same on these knobs — so we keep one env var per concept.
	searchTimeout := getEnvDuration("LIBRARY_SEARCH_TIMEOUT", 15*time.Second)
	searchUA := getEnv("LIBRARY_SEARCH_UA", "AnimeEnigma/1.0 (library service)")

	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8089),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "library"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", ""),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		Nyaa: NyaaConfig{
			BaseURL:     getEnv("NYAA_BASE_URL", "https://nyaa.si"),
			HTTPTimeout: searchTimeout,
			UserAgent:   searchUA,
		},
		AnimeTosho: AnimeToshoConfig{
			BaseURL:     getEnv("ANIMETOSHO_BASE_URL", "https://feed.animetosho.org"),
			HTTPTimeout: searchTimeout,
			UserAgent:   searchUA,
		},
		LibrarySearch: LibrarySearchConfig{
			DefaultLimit: getEnvInt("LIBRARY_SEARCH_DEFAULT_LIMIT", 50),
			MaxLimit:     getEnvInt("LIBRARY_SEARCH_MAX_LIMIT", 200),
		},
	}, nil
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
