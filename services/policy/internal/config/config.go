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
	Server   ServerConfig
	Database database.Config
	Redis    cache.Config
	JWT      authz.JWTConfig
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	return &Config{
		Server: ServerConfig{Host: getEnv("SERVER_HOST", "0.0.0.0"), Port: getEnvInt("SERVER_PORT", 8098)},
		Database: database.Config{
			Host: getEnv("DB_HOST", "localhost"), Port: getEnvInt("DB_PORT", 5432),
			User: getEnv("DB_USER", "postgres"), Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"), SSLMode: getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host: getEnv("REDIS_HOST", "redis"), Port: getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""), DB: getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret: getEnv("JWT_SECRET", ""), Issuer: getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
	}, nil
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func getEnvInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return d
}
func getEnvDuration(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if x, err := time.ParseDuration(v); err == nil {
			return x
		}
	}
	return d
}
