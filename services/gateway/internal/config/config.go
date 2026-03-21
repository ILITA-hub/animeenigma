package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
)

type Config struct {
	Server      ServerConfig
	JWT         authz.JWTConfig
	Services    ServiceURLs
	RateLimit   RateLimitConfig
	CORSOrigins []string
	Environment string // "production", "staging", "development", etc.
	DevMode     bool   // Skip admin auth when true (for local development)
	SiteURL     string // Public-facing base URL for OG meta tags (e.g. "https://animeenigma.ru")
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type ServiceURLs struct {
	AuthService      string
	CatalogService   string
	PlayerService    string
	RoomsService     string
	StreamingService string
	ThemesService    string
	// Admin panel services
	GrafanaService    string
	PrometheusService string
	LokiService       string
	// Infrastructure services (for status page)
	SchedulerService string
	RedisAddr        string
	PostgresAddr     string
	NatsAddr         string
}

type RateLimitConfig struct {
	RequestsPerSecond int
	BurstSize         int
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	cfg := &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8000),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", ""),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		Services: ServiceURLs{
			AuthService:      getEnv("AUTH_SERVICE_URL", "http://auth:8080"),
			CatalogService:   getEnv("CATALOG_SERVICE_URL", "http://catalog:8081"),
			PlayerService:    getEnv("PLAYER_SERVICE_URL", "http://player:8083"),
			RoomsService:     getEnv("ROOMS_SERVICE_URL", "http://rooms:8084"),
			StreamingService: getEnv("STREAMING_SERVICE_URL", "http://streaming:8082"),
			ThemesService:    getEnv("THEMES_SERVICE_URL", "http://themes:8086"),
			// Admin panel services
			GrafanaService:    getEnv("GRAFANA_SERVICE_URL", "http://grafana:3000"),
			PrometheusService: getEnv("PROMETHEUS_SERVICE_URL", "http://prometheus:9090"),
			LokiService:       getEnv("LOKI_SERVICE_URL", "http://loki:3100"),
			// Infrastructure services (for status page)
			SchedulerService: getEnv("SCHEDULER_SERVICE_URL", "http://scheduler:8085"),
			RedisAddr:        getEnv("REDIS_ADDR", "redis:6379"),
			PostgresAddr:     getEnv("POSTGRES_ADDR", "postgres:5432"),
			NatsAddr:         getEnv("NATS_ADDR", "nats:4222"),
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: getEnvInt("RATE_LIMIT_RPS", 100),
			BurstSize:         getEnvInt("RATE_LIMIT_BURST", 200),
		},
		CORSOrigins: strings.Split(getEnv("CORS_ORIGINS", ""), ","),
		Environment: strings.ToLower(getEnv("ENVIRONMENT", "")),
		DevMode:     getEnvBool("DEV_MODE", false),
		SiteURL:     strings.TrimRight(getEnv("SITE_URL", ""), "/"),
	}

	// Production safeguard: refuse to enable DevMode in production
	if cfg.DevMode && (cfg.Environment == "production" || cfg.Environment == "prod") {
		fmt.Fprintf(os.Stderr, "FATAL: DEV_MODE=true is forbidden when ENVIRONMENT=%s — forcing DevMode=false\n", cfg.Environment)
		cfg.DevMode = false
	}

	return cfg, nil
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
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
