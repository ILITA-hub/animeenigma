package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

type Config struct {
	Server        ServerConfig
	Database      database.Config
	Redis         cache.Config
	JWT           authz.JWTConfig
	Telegram      TelegramConfig
	Reports       ReportsConfig
	Maintenance   MaintenanceConfig
	Tier2         Tier2Config
	Gacha         GachaConfig
	Recs          RecsConfig
	Notify        NotifyConfig
	Autocache     AutocacheConfig
	ContentVerify ContentVerifyConfig
}

// AutocacheConfig controls the fire-and-forget player→library autocache demand
// producer (Phase 9 / TRIG-02 — Logic B). When an active JP-audio watcher sends
// the first progress heartbeat for episode N of a watching anime, the player
// fires a next_ep demand for N+1 to the library so the autocache Planner
// pre-downloads it. Same drop-on-full / nil-safe contract as RecsConfig — a
// slow/down library never affects the heartbeat.
type AutocacheConfig struct {
	// LibraryURL is the base URL of the library service inside the Docker
	// network. Only the path /internal/library/autocache/demand is called.
	// Reuses the existing compose var LIBRARY_SERVICE_URL. Default:
	// http://library:8089
	LibraryURL string
	// DemandEnabled toggles the demand producer; when false the producer is
	// constructed disabled and all demands are silently dropped (library down /
	// dark-ship). Default: true
	DemandEnabled bool
}

// RecsConfig controls the fire-and-forget recs recompute-hint producer
// (recs engine extraction, 2026-06-11). The recs engine moved out of player
// to services/recs; player now POSTs a hint per watched episode instead of
// running the recs crons in-process.
type RecsConfig struct {
	// InternalURL is the base URL of the recs service reachable inside the
	// Docker network. Only the path /internal/recs/recompute-hint is called.
	// Default: http://recs:8094
	InternalURL string
	// HintEnabled controls whether the hint producer is active. When false the
	// producer is constructed in disabled mode and all hints are silently
	// dropped (recs outage / dark-ship scenario). Default: true
	HintEnabled bool
}

// ContentVerifyConfig controls the fire-and-forget content-verify watching
// hint producer. content-verify (:8101) accepts POST /internal/verify/hint
// {"anime_id","visitor","source"} → 204; catalog already fires visit-hints,
// this is the player-side counterpart fired once per watched episode.
type ContentVerifyConfig struct {
	// InternalURL is the base URL of the content-verify service reachable
	// inside the Docker network. Only the path /internal/verify/hint is
	// called. Default: http://content-verify:8101
	InternalURL string
	// HintEnabled controls whether the hint producer is active. When false
	// the producer is constructed in disabled mode and all hints are
	// silently dropped (content-verify outage / dark-ship scenario).
	// Default: true
	HintEnabled bool
}

// NotifyConfig controls the fire-and-forget feedback-status notification
// producer (AUTO-417). Fire-and-forget producer pattern: a notifications
// outage must never affect report submission or triage.
type NotifyConfig struct {
	// InternalURL is the base URL of the notifications service inside the
	// Docker network. Paths called: /internal/notifications and
	// /internal/notifications/invalidate. Default: http://notifications:8090
	InternalURL string
	// Enabled toggles the producer; when false all dispatches are dropped.
	Enabled bool
}

// GachaConfig controls the fire-and-forget gacha credit producer (Phase 4).
type GachaConfig struct {
	// InternalURL is the base URL of the gacha service reachable inside the
	// Docker network. Only the path /internal/gacha/credit is called.
	// Default: http://gacha:8093
	InternalURL string
	// CreditEpisode is the Энигмы amount credited per watched episode.
	// Default: 22
	CreditEpisode int64
	// CreditTitle is the Энигмы amount credited when a title is completed.
	// Default: 80
	CreditTitle int64
	// Enabled controls whether the credit producer is active. When false the
	// producer is constructed in disabled mode and all events are silently
	// dropped (gacha outage / dark-ship scenario). Default: true
	Enabled bool
}

// Tier2Config controls the Phase 6 weighted, time-decayed Tier 2 inference.
// Tunable at runtime so we can adjust thresholds in production without a code change.
type Tier2Config struct {
	HalfLifeDays   float64 // exponential decay half-life (days). Default 30.
	MinConfidence  float64 // min total weighted history to lock. Below = fall through to Tier 3. Default 1800 (≈30min effective).
	MaxHistoryRows int     // safety cap on history rows pulled per resolve. Default 5000.
	DurationFloor  int     // min duration_watched for a row's weight contribution (handles legacy duration_watched=0 rows). Default 60.
}

type TelegramConfig struct {
	BotToken    string
	AdminChatID string
}

type MaintenanceConfig struct {
	URL string // e.g. http://172.18.0.1:8087
	// Token is the shared secret sent as X-Maintenance-Token on /api/reports.
	// Must match the maintenance daemon's REPORTS_AUTH_TOKEN (finding #39).
	Token string
}

type ReportsConfig struct {
	Dir string
}

type ServerConfig struct {
	Host string
	Port int
}

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
			Port: getEnvInt("SERVER_PORT", 8082),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		// Redis is required by player as of Phase 10 — recs handler caches
		// the anonymous trending row (recs:public:trending:topN, 6h TTL) and
		// the population orchestrator writes a cache-buster timestamp on each
		// successful tick (recs:popsignal:lastcomputed). The compose file
		// resolves the host as "redis"; other services in this stack use the
		// same convention.
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
		Telegram: TelegramConfig{
			BotToken:    getEnv("TELEGRAM_BOT_TOKEN", ""),
			AdminChatID: getEnv("TELEGRAM_ADMIN_CHAT_ID", ""),
		},
		Reports: ReportsConfig{
			Dir: getEnv("REPORTS_DIR", "/data/reports"),
		},
		Maintenance: MaintenanceConfig{
			URL:   getEnv("MAINTENANCE_URL", ""),
			Token: getEnv("REPORTS_AUTH_TOKEN", ""),
		},
		Tier2: Tier2Config{
			HalfLifeDays:   getEnvFloat("TIER2_HALF_LIFE_DAYS", 30.0),
			MinConfidence:  getEnvFloat("TIER2_MIN_CONFIDENCE", 1800.0),
			MaxHistoryRows: getEnvInt("TIER2_MAX_HISTORY_ROWS", 5000),
			DurationFloor:  getEnvInt("TIER2_DURATION_FLOOR", 60),
		},
		Gacha: GachaConfig{
			InternalURL:   getEnv("GACHA_INTERNAL_URL", "http://gacha:8093"),
			CreditEpisode: int64(getEnvInt("GACHA_CREDIT_EPISODE", 22)),
			CreditTitle:   int64(getEnvInt("GACHA_CREDIT_TITLE", 80)),
			Enabled:       getEnvBool("GACHA_CREDIT_ENABLED", true),
		},
		Recs: RecsConfig{
			InternalURL: getEnv("RECS_INTERNAL_URL", "http://recs:8094"),
			HintEnabled: getEnvBool("RECS_HINT_ENABLED", true),
		},
		Notify: NotifyConfig{
			InternalURL: getEnv("NOTIFICATIONS_INTERNAL_URL", "http://notifications:8090"),
			Enabled:     getEnvBool("FEEDBACK_NOTIFY_ENABLED", true),
		},
		Autocache: AutocacheConfig{
			LibraryURL:    getEnv("LIBRARY_SERVICE_URL", "http://library:8089"),
			DemandEnabled: getEnvBool("AUTOCACHE_DEMAND_ENABLED", true),
		},
		ContentVerify: ContentVerifyConfig{
			InternalURL: getEnv("CONTENT_VERIFY_INTERNAL_URL", "http://content-verify:8101"),
			HintEnabled: getEnvBool("CONTENT_VERIFY_HINT_ENABLED", true),
		},
	}, nil
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch v {
	case "1", "t", "true", "y", "yes", "on":
		return true
	case "0", "f", "false", "n", "no", "off":
		return false
	}
	return def
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
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
