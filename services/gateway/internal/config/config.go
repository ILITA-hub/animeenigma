package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
)

type Config struct {
	Server   ServerConfig
	JWT      authz.JWTConfig
	Services ServiceURLs
	RateLimit RateLimitConfig
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type ServiceURLs struct {
	AuthService     string
	CatalogService  string
	PlayerService   string
	RoomsService    string
	StreamingService string
}

type RateLimitConfig struct {
	RequestsPerSecond int
	BurstSize         int
}

func Load() (*Config, error) {
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8000),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		Services: ServiceURLs{
			AuthService:     getEnv("AUTH_SERVICE_URL", "http://auth:8080"),
			CatalogService:  getEnv("CATALOG_SERVICE_URL", "http://catalog:8081"),
			PlayerService:   getEnv("PLAYER_SERVICE_URL", "http://player:8082"),
			RoomsService:    getEnv("ROOMS_SERVICE_URL", "http://rooms:8083"),
			StreamingService: getEnv("STREAMING_SERVICE_URL", "http://streaming:8084"),
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: getEnvInt("RATE_LIMIT_RPS", 100),
			BurstSize:        getEnvInt("RATE_LIMIT_BURST", 200),
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

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
