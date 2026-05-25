// Package config holds the watch-together service runtime configuration.
//
// Loaded once at boot via Load(); JWT_SECRET is the only required env var.
// All other settings have safe defaults (WT-FOUND-01 / WT-NF-03):
//
//   - Server.Port:    SERVER_PORT (default 8091)
//   - MaxMembers:     WATCH_TOGETHER_MAX_MEMBERS (default 10, per WT-NF-02)
//   - RoomTTL:        WATCH_TOGETHER_ROOM_TTL (default 900s, sliding)
//   - GracePeriod:    WATCH_TOGETHER_GRACE_PERIOD (default 5m post-last-disconnect)
//
// No Postgres / GORM — this service is Redis-only by design
// (Phase 01-CONTEXT.md / WT-FOUND-02 deferred persistence to v1.2).
package config

import (
	"fmt"
	"os"
	"strconv"
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
		MaxMembers:  getEnvInt("WATCH_TOGETHER_MAX_MEMBERS", 10),
		RoomTTL:     getEnvDuration("WATCH_TOGETHER_ROOM_TTL", 900*time.Second),
		GracePeriod: getEnvDuration("WATCH_TOGETHER_GRACE_PERIOD", 5*time.Minute),
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
