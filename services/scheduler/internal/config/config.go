package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

type Config struct {
	Server   ServerConfig
	Database database.Config
	Redis    cache.Config
	Jobs     JobsConfig
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type JobsConfig struct {
	ShikimoriSyncCron   string
	CleanupCron         string
	TopAnimeSyncCron    string
	CalendarSyncCron    string
	ShikimoriAPIURL     string
	ShikimoriAppName    string
	CatalogServiceURL   string
	DataRetentionDays   int
	OngoingStaleHours   int
	AnnouncedStaleHours int
	ReleasedStaleHours  int

	// Phase 23 — scraper playability canary (SCRAPER-HEAL-12 / -13).
	// ScraperPlayabilityCanaryCron: cron expression for the daily canary
	//   run. Default `0 3 * * *` (03:00 local time, off-peak). The canary
	//   itself applies ±5min jitter on top of the cron tick to avoid
	//   03:00:00 fingerprinting upstream.
	// ScraperBaseURL: base URL of the in-cluster scraper service the canary
	//   calls (/scraper/servers, /scraper/stream). Default matches the
	//   docker-compose service name.
	// CanaryReportDir: directory the canary writes per-run JSON logs into.
	//   Must live under a mounted volume (player_reports) for persistence
	//   across container restarts. See CONTEXT.md D5.
	ScraperPlayabilityCanaryCron string
	ScraperBaseURL               string
	CanaryReportDir              string

	// Phase 03 (v4.0) — daily db_read P95 read-threshold recompute trigger
	// (D-03 / AR-EFFECT-01). ReadThresholdCron: cron for the daily trigger
	//   (default `0 5 * * *`, 05:00 — after the existing 01:00-04:00 jobs and
	//   well clear of the analytics purge at 03:17, so the events table has a
	//   full day of db_read rows to percentile over).
	// AnalyticsInternalURL: base URL of the in-cluster analytics service whose
	//   /internal/read-thresholds/recompute endpoint this job POSTs. Matches
	//   the docker-compose service name + the shared ANALYTICS_INTERNAL_URL env.
	ReadThresholdCron    string
	AnalyticsInternalURL string

	// Stage 2b (Smart Source Selection) — daily provider-reliability ranking
	// recompute trigger. ProviderRankingCron: cron for the daily trigger
	//   (default `30 5 * * *`, 05:30 — after the read-threshold recompute at
	//   05:00 so the two analytics aggregates don't contend). Like the
	//   read-threshold job, the scheduler only POSTs analytics' /internal
	//   recompute endpoint; analytics owns the compute + Redis publish.
	ProviderRankingCron string
}

func Load() (*Config, error) {
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8085),
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
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 2),
		},
		Jobs: JobsConfig{
			ShikimoriSyncCron:  getEnv("SHIKIMORI_SYNC_CRON", "0 2 * * *"),     // Daily at 2 AM
			CleanupCron:        getEnv("CLEANUP_CRON", "0 3 * * 0"),            // Weekly on Sunday at 3 AM
			TopAnimeSyncCron:   getEnv("TOP_ANIME_SYNC_CRON", "0 1 * * *"),     // Daily at 1 AM
			CalendarSyncCron:   getEnv("CALENDAR_SYNC_CRON", "0 4 * * 1"),      // Weekly on Monday at 4 AM
			ShikimoriAPIURL:    getEnv("SHIKIMORI_API_URL", "https://shikimori.one/api"),
			ShikimoriAppName:   getEnv("SHIKIMORI_APP_NAME", "AnimeEnigma"),
			CatalogServiceURL:   getEnv("CATALOG_SERVICE_URL", "http://catalog:8081"),
			DataRetentionDays:   getEnvInt("DATA_RETENTION_DAYS", 90),
			OngoingStaleHours:   getEnvInt("ONGOING_STALE_HOURS", 12),
			AnnouncedStaleHours: getEnvInt("ANNOUNCED_STALE_HOURS", 72),
			ReleasedStaleHours:  getEnvInt("RELEASED_STALE_HOURS", 168),
			// Phase 23 — canary.
			ScraperPlayabilityCanaryCron: getEnv("SCRAPER_PLAYABILITY_CANARY_CRON", "0 3 * * *"),
			ScraperBaseURL:               getEnv("SCRAPER_BASE_URL", "http://scraper:8088"),
			CanaryReportDir:              getEnv("CANARY_REPORT_DIR", "/data/reports/canary-runs"),
			// Phase 03 (v4.0) — daily read-threshold recompute trigger.
			ReadThresholdCron:    getEnv("READ_THRESHOLD_CRON", "0 5 * * *"),
			AnalyticsInternalURL: getEnv("ANALYTICS_INTERNAL_URL", "http://analytics:8092"),
			// Stage 2b — daily provider-ranking recompute trigger.
			ProviderRankingCron: getEnv("PROVIDER_RANKING_CRON", "30 5 * * *"), // Daily 05:30 (after read-threshold 05:00)
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
