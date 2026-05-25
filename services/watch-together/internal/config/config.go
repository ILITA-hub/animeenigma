// Package config holds the watch-together service runtime configuration.
//
// Loaded once at boot via Load(); JWT_SECRET is the only required env var.
// All other settings have safe defaults (WT-FOUND-01 / WT-NF-03):
//
//   - Server.Port:     SERVER_PORT (default 8091)
//   - MaxMembers:      WATCH_TOGETHER_MAX_MEMBERS (default 10, per WT-NF-02)
//   - RoomTTL:         WATCH_TOGETHER_ROOM_TTL (default 900s, sliding)
//   - GracePeriod:     WATCH_TOGETHER_GRACE_PERIOD (default 5m post-last-disconnect)
//   - PublicBaseURL:   WATCH_TOGETHER_PUBLIC_BASE_URL (default https://animeenigma.ru)
//
// No Postgres / GORM — this service is Redis-only by design
// (Phase 01-CONTEXT.md / WT-FOUND-02 deferred persistence to v1.2).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
)

type Config struct {
	Server ServerConfig
	Redis  cache.Config
	JWT    authz.JWTConfig

	// MaxMembers caps room size (WT-NF-02 / 01-CONTEXT.md). Default 10.
	MaxMembers int
	// RoomTTL is the sliding TTL applied to wt:room:{id}* keys; refreshed on
	// every inbound event. Default 900s.
	RoomTTL time.Duration
	// GracePeriod is the post-last-disconnect window before the room is
	// torn down. Default 5m.
	GracePeriod time.Duration

	// PublicBaseURL is the public origin used to construct invite + ws URLs
	// in the POST /rooms response. Default "https://animeenigma.ru" (prod);
	// override via WATCH_TOGETHER_PUBLIC_BASE_URL. NEVER include a trailing
	// slash — Load() trims it. The handler swaps http→ws / https→wss for
	// the ws_url field; see wsURLFromBase in internal/handler/rooms.go.
	PublicBaseURL string

	// AllowAllOrigins disables the WebSocket Origin-header allowlist on
	// the /ws upgrade handler. Production deployments leave this `false`
	// so only requests originating from PublicBaseURL can upgrade; local
	// dev (`make dev` + Vite dev server on a different port) flips it on
	// via WATCH_TOGETHER_ALLOW_ALL_ORIGINS=true. NEVER enable in prod.
	AllowAllOrigins bool
}

type ServerConfig struct {
	Host string
	Port int
}

// Address returns the host:port pair for net.Listen / http.Server.Addr.
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			// v1.0 Watch Together — Phase 1 (workstream: watch-together).
			// Port 8091: next free after notifications:8090.
			Port: getEnvInt("SERVER_PORT", 8091),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "redis"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", ""),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		MaxMembers:      getEnvInt("WATCH_TOGETHER_MAX_MEMBERS", 10),
		RoomTTL:         getEnvDuration("WATCH_TOGETHER_ROOM_TTL", 900*time.Second),
		GracePeriod:     getEnvDuration("WATCH_TOGETHER_GRACE_PERIOD", 5*time.Minute),
		PublicBaseURL:   strings.TrimRight(getEnv("WATCH_TOGETHER_PUBLIC_BASE_URL", "https://animeenigma.ru"), "/"),
		AllowAllOrigins: getEnvBool("WATCH_TOGETHER_ALLOW_ALL_ORIGINS", false),
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

// getEnvBool parses a permissive boolean: "1", "true", "yes", "on" (any case)
// → true; "0", "false", "no", "off" (any case) → false; anything else returns
// the default. Mirrors the cautious-parse style of getEnvInt/getEnvDuration so
// a malformed value never crashes boot.
func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultVal
	}
}
