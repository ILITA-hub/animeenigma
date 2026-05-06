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
	Server      ServerConfig
	Database    database.Config
	Redis       cache.Config
	JWT         authz.JWTConfig
	Telegram    TelegramConfig
	Reports     ReportsConfig
	Maintenance MaintenanceConfig
	Tier2       Tier2Config
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
			URL: getEnv("MAINTENANCE_URL", ""),
		},
		Tier2: Tier2Config{
			HalfLifeDays:   getEnvFloat("TIER2_HALF_LIFE_DAYS", 30.0),
			MinConfidence:  getEnvFloat("TIER2_MIN_CONFIDENCE", 1800.0),
			MaxHistoryRows: getEnvInt("TIER2_MAX_HISTORY_ROWS", 5000),
			DurationFloor:  getEnvInt("TIER2_DURATION_FLOOR", 60),
		},
	}, nil
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
