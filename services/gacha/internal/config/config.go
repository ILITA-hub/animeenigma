package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
)

// Config is the gacha service configuration.
type Config struct {
	Server   ServerConfig
	Database database.Config
	Redis    cache.Config
	JWT      authz.JWTConfig
	Economy  EconomyConfig
	Storage  videoutils.StorageConfig

	// Enabled is the backend dark-ship toggle (GACHA_ENABLED). When false,
	// the internal credit endpoint no-ops with 200 (so producers don't
	// error) and the service still boots. Frontend is gated separately by
	// VITE_GACHA_ENABLED. Default true.
	Enabled bool
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

// EconomyConfig holds tunable currency knobs (spec §5). Numeric balance is
// in whole «Энигмы».
type EconomyConfig struct {
	StarterBonus int64 // one-time grant on first wallet access (default 300)

	// Phase 4 — daily claim knobs (spec §5.2).
	DailyBase       int64 // GACHA_DAILY_BASE, default 50 — base award on any daily claim
	DailyStreakStep int64 // GACHA_DAILY_STREAK_STEP, default 10 — Энигм per consecutive-day streak day
	DailyStreakCap  int64 // GACHA_DAILY_STREAK_CAP, default 100 — max streak bonus (caps at step*10 by default)

	// Phase 3 — pull-engine knobs (spec §5.1/5.3).
	PullCostX1    int64 // GACHA_PULL_COST_X1, default 100
	PullCostX10   int64 // GACHA_PULL_COST_X10, default 900 (×10 with the 10% discount)
	PityThreshold int   // GACHA_PITY_THRESHOLD, default 90 — the Nth pull without SSR is forced SSR
	WeightN       int   // GACHA_WEIGHT_N, default 69
	WeightR       int   // GACHA_WEIGHT_R, default 22
	WeightSR      int   // GACHA_WEIGHT_SR, default 8
	WeightSSR     int   // GACHA_WEIGHT_SSR, default 1
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			// Port 8093: next free after analytics:8092 (8087 maintenance,
			// 8089 library, 8090 notifications, 8091 watch-together).
			Port: getEnvInt("SERVER_PORT", 8093),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
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
		Economy: EconomyConfig{
			StarterBonus:    int64(getEnvInt("GACHA_STARTER_BONUS", 300)),
			DailyBase:       int64(getEnvInt("GACHA_DAILY_BASE", 50)),
			DailyStreakStep: int64(getEnvInt("GACHA_DAILY_STREAK_STEP", 10)),
			DailyStreakCap:  int64(getEnvInt("GACHA_DAILY_STREAK_CAP", 100)),
			PullCostX1:      int64(getEnvInt("GACHA_PULL_COST_X1", 100)),
			PullCostX10:   int64(getEnvInt("GACHA_PULL_COST_X10", 900)),
			PityThreshold: getEnvInt("GACHA_PITY_THRESHOLD", 90),
			WeightN:       getEnvInt("GACHA_WEIGHT_N", 69),
			WeightR:       getEnvInt("GACHA_WEIGHT_R", 22),
			WeightSR:      getEnvInt("GACHA_WEIGHT_SR", 8),
			WeightSSR:     getEnvInt("GACHA_WEIGHT_SSR", 1),
		},
		Storage: videoutils.StorageConfig{
			Endpoint:        getEnv("MINIO_ENDPOINT", "minio:9000"),
			AccessKeyID:     getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretAccessKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
			UseSSL:          getEnvBool("MINIO_USE_SSL", false),
			BucketName:      getEnv("GACHA_MINIO_BUCKET", "gacha-cards"),
		},
		Enabled: getEnvBool("GACHA_ENABLED", true),
	}, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	case "0", "f", "false", "n", "no", "off":
		return false
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
