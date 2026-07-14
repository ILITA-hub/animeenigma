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
	Groq     GroqConfig
	DailyCap int // FANFIC_DAILY_CAP — max generations per user per day (default 100)

	CatalogURL           string
	CatalogTimeout       time.Duration
	ContinueContextRunes int

	// Daily "Фанфик дня" bot generation — see internal/service/ensure_daily.go.
	AlertsBotToken string   // TELEGRAM_ALERTS_BOT_TOKEN — empty ⇒ alerter falls back to Noop (fail-open)
	AlertsChatID   string   // TELEGRAM_ADMIN_CHAT_ID — empty ⇒ alerter falls back to Noop (fail-open)
	DailyAnimePool []string // FANFIC_DAILY_ANIME_POOL — CSV of shikimori IDs the daily picker draws from
	BotLanguage    string   // FANFIC_BOT_LANGUAGE — language the bot generates the daily fanfic in
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

type GroqConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	if getEnv("FANFIC_GROQ_API_KEY", "") == "" {
		return nil, fmt.Errorf("FANFIC_GROQ_API_KEY environment variable is required")
	}
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8097),
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
		Groq: GroqConfig{
			APIKey:  getEnv("FANFIC_GROQ_API_KEY", ""),
			BaseURL: getEnv("FANFIC_GROQ_BASE_URL", "https://api.groq.com/openai/v1"),
			Model:   getEnv("FANFIC_GROQ_MODEL", "llama-3.1-8b-instant"),
			Timeout: getEnvDuration("FANFIC_GROQ_TIMEOUT", 120*time.Second),
		},
		DailyCap: getEnvInt("FANFIC_DAILY_CAP", 100),

		CatalogURL:           getEnv("CATALOG_URL", "http://catalog:8081"),
		CatalogTimeout:       getEnvDuration("FANFIC_CATALOG_TIMEOUT", 5*time.Second),
		ContinueContextRunes: getEnvInt("FANFIC_CONTINUE_CONTEXT_RUNES", 24000),

		AlertsBotToken: getEnv("TELEGRAM_ALERTS_BOT_TOKEN", ""),
		AlertsChatID:   getEnv("TELEGRAM_ADMIN_CHAT_ID", ""),
		DailyAnimePool: getEnvCSV("FANFIC_DAILY_ANIME_POOL", "20,21,1735,52991,16498,5114"),
		BotLanguage:    getEnv("FANFIC_BOT_LANGUAGE", "ru"),
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
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
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

// getEnvCSV splits a comma-separated env var into a trimmed, non-empty-entry
// slice, falling back to a (also CSV) default when the env var is unset.
func getEnvCSV(key, def string) []string {
	raw := getEnv(key, def)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
