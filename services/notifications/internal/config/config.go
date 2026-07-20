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
)

type Config struct {
	Server   ServerConfig
	Database database.Config
	Redis    cache.Config
	JWT      authz.JWTConfig

	// Detector is the Phase 2 v1.0 Notifications Engine cron + cleanup
	// configuration. NOTIFICATIONS_DETECTOR_ENABLED=false makes the
	// service skip scheduler.Start at boot — the producer endpoint and
	// CRUD API continue to work (D-RB-01 rollback toggle).
	Detector DetectorConfig
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// DetectorConfig groups the Phase 2 cron + cleanup + catalog-client
// settings. All values have safe defaults so an unset env still produces a
// running detector at `0 * * * *` against `http://catalog:8081`.
type DetectorConfig struct {
	// Enabled gates Scheduler.Start at boot (D-RB-01 rollback toggle).
	Enabled bool
	// Cron is the detector schedule (default "0 * * * *"). ±5min boot-time
	// jitter is applied by Scheduler.Start so simultaneous boots across
	// replicas don't synchronise parser load.
	Cron string
	// CleanupCron is the retention cleanup schedule (default "30 3 * * *").
	CleanupCron string
	// RetentionDays drives the DELETE in DismissedRetentionCleanupJob
	// (default 30, per NOTIF-DET-09).
	RetentionDays int
	// WorkerLimit caps concurrent per-combo parser calls inside the
	// detector errgroup (default 5).
	WorkerLimit int
	// ParserTimeout is the per-combo HTTP-call deadline for the catalog
	// client (default 10s).
	ParserTimeout time.Duration
	// UnreadGaugeEvery sets the polling interval for the active-unread
	// metric (default 5m). Backed by the idx_user_unread partial index so
	// the COUNT(*) is cheap.
	UnreadGaugeEvery time.Duration
	// CatalogURL is the base URL of the catalog service (default
	// "http://catalog:8081"). The detector hits
	// /internal/anime/{shikimori_id}/episodes here.
	CatalogURL string
	// Cadence tiering (spec §4). HotWindow is the ± window on next_episode_at
	// that keeps an ongoing on the hourly (every-run) tier; WarmEvery is the
	// spacing for non-hot titles; TierFloor is the hard delivery floor —
	// no combo is checked less often than this.
	HotWindow time.Duration
	WarmEvery time.Duration
	TierFloor time.Duration
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			// v1.0 Notifications Engine — Phase 1 (workstream: notifications).
			// Port 8090: 8087 was unavailable (host-native maintenance bot
			// already bound to it; same blocker that pushed library to 8089).
			// See .planning/workstreams/notifications/phases/01-notifications-foundation/SUMMARY.md
			Port: getEnvInt("SERVER_PORT", 8090),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			// D-01: single shared DB (`animeenigma`), not a dedicated
			// `notifications` DB. Cross-service read-only views in
			// internal/repo/views.go depend on this — they share the
			// same *gorm.DB handle with watch_history / anime_list / animes.
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
		Detector: DetectorConfig{
			Enabled:          getEnvBool("NOTIFICATIONS_DETECTOR_ENABLED", true),
			Cron:             getEnv("NOTIFICATIONS_DETECTOR_CRON", "0 * * * *"),
			CleanupCron:      getEnv("NOTIFICATIONS_CLEANUP_CRON", "30 3 * * *"),
			RetentionDays:    getEnvInt("NOTIFICATIONS_RETENTION_DAYS", 30),
			WorkerLimit:      getEnvInt("NOTIFICATIONS_DETECTOR_WORKER_LIMIT", 5),
			ParserTimeout:    getEnvDuration("NOTIFICATIONS_PARSER_TIMEOUT", 10*time.Second),
			UnreadGaugeEvery: getEnvDuration("NOTIFICATIONS_UNREAD_GAUGE_INTERVAL", 5*time.Minute),
			CatalogURL:       getEnv("CATALOG_URL", "http://catalog:8081"),
			HotWindow:        getEnvDuration("NOTIF_HOT_WINDOW", 36*time.Hour),
			WarmEvery:        getEnvDuration("NOTIF_WARM_EVERY", 3*time.Hour),
			TierFloor:        getEnvDuration("NOTIF_TIER_FLOOR", 6*time.Hour),
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

func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	case "0", "f", "false", "n", "no", "off":
		return false
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
