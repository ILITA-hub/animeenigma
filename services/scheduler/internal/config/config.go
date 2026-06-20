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

	// Phase A — daily playback-health probe trigger (replaces Phase 23 canary).
	// PlaybackProbeCron: cron expression for the daily probe run.
	//   Default `0 3 * * *` (03:00 local time, off-peak). The scheduler
	//   POSTs analytics' /internal/probe/run; analytics runs the full
	//   catalog-signed resolve → HLS proxy validation chain.
	PlaybackProbeCron string

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

	// Phase 09 (v4.1 download-triggers) — Logic A ongoing-push autocache producer
	// (TRIG-01). AutocacheLogicACron: cron for the periodic sweep (default
	//   `*/20 * * * *`, every 20 min — mirrors the library autocache_config
	//   sweep_interval_min default; the library demand PK dedup + Planner backoff
	//   bound the re-asserted demand). LibraryInternalURL: base URL of the
	//   in-cluster library service whose /internal/library/autocache/demand
	//   endpoint the Logic A job POSTs (Docker-network-only); when empty the job
	//   is disabled (nil-guarded registration). AutocacheActiveWatcherDays: the D8
	//   recency window for "active watcher" (default 30). The AUTHORITATIVE value
	//   lives in library autocache_config (live-editable) — but the scheduler is on
	//   a different DB and does NOT read library's DB, so this is a scheduler env
	//   MIRROR (AUTOCACHE_ACTIVE_WATCHER_DAYS). Keep the two in sync if the library
	//   default is retuned.
	//
	//   WR-05 (Phase-09 review) — ACCEPTED as-is: this env mirror can drift from
	//   the live library autocache_config value (an admin lowering it there does
	//   not take effect here until the scheduler env is also edited + redeployed).
	//   This is a deliberate consequence of the cross-DB boundary (scheduler must
	//   not read library's DB). A future enhancement could have Logic A read the
	//   authoritative value via the library internal endpoint per-sweep; until then
	//   the redeploy requirement is the documented contract.
	AutocacheLogicACron        string
	LibraryInternalURL         string
	AutocacheActiveWatcherDays int

	// Phase 11 (v4.1 observability — OBS-05) — daily storage-need prediction job.
	// AutocachePredictionCron: cron for the daily heuristic sweep (default
	//   `0 4 * * *`, 04:00 — a single light run that COUNTS the Logic A watcher
	//   join twice and sets a {component} gauge; far cheaper than Logic A's
	//   per-row fan-out). AutocacheAvgRawEpBytes: a scheduler env MIRROR of spec
	//   §7 avg_raw_ep_size — the assumed average bytes of one raw episode, used to
	//   turn the two distinct-anime COUNTS into a predicted-bytes estimate. Default
	//   ~1.2 GiB. Like AutocacheActiveWatcherDays this is a scheduler env mirror
	//   (the authoritative Phase-10 constant lives in library); keep the two in
	//   sync if the library value is retuned.
	AutocachePredictionCron string
	AutocacheAvgRawEpBytes  int64
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
			// Phase A — daily playback-health probe trigger.
			PlaybackProbeCron: getEnv("PLAYBACK_PROBE_CRON", "0 3 * * *"),
			// Phase 03 (v4.0) — daily read-threshold recompute trigger.
			ReadThresholdCron:    getEnv("READ_THRESHOLD_CRON", "0 5 * * *"),
			AnalyticsInternalURL: getEnv("ANALYTICS_INTERNAL_URL", "http://analytics:8092"),
			// Stage 2b — daily provider-ranking recompute trigger.
			ProviderRankingCron: getEnv("PROVIDER_RANKING_CRON", "30 5 * * *"), // Daily 05:30 (after read-threshold 05:00)
			// Phase 09 — Logic A ongoing-push autocache producer (TRIG-01).
			AutocacheLogicACron:        getEnv("AUTOCACHE_LOGIC_A_CRON", "*/20 * * * *"), // Every 20 min (mirrors sweep_interval_min)
			LibraryInternalURL:         getEnv("LIBRARY_INTERNAL_URL", getEnv("LIBRARY_SERVICE_URL", "http://library:8089")),
			AutocacheActiveWatcherDays: getEnvInt("AUTOCACHE_ACTIVE_WATCHER_DAYS", 30),
			// Phase 11 (OBS-05) — daily storage-need prediction job.
			AutocachePredictionCron: getEnv("AUTOCACHE_PREDICTION_CRON", "0 4 * * *"),         // Daily 04:00
			AutocacheAvgRawEpBytes:  getEnvInt64("AUTOCACHE_AVG_RAW_EP_BYTES", 1288490188),    // ~1.2 GiB (spec §7 avg_raw_ep_size)
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

// getEnvInt64 reads a 64-bit integer env var (e.g. a byte quantity that can
// exceed 2^31 on a 32-bit build). Mirrors getEnvInt but parses with
// strconv.ParseInt(..., 10, 64) so values like AUTOCACHE_AVG_RAW_EP_BYTES
// (~1.2 GiB) are correct regardless of platform int width.
func getEnvInt64(key string, defaultVal int64) int64 {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}
