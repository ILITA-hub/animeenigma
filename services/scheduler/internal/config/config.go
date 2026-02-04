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
	ShikimoriAPIURL     string
	ShikimoriAppName    string
	CatalogServiceURL   string
	DataRetentionDays   int
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
			Database: getEnv("DB_NAME", "animeenigma_catalog"), // Uses catalog database
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 2),
		},
		Jobs: JobsConfig{
			ShikimoriSyncCron:  getEnv("SHIKIMORI_SYNC_CRON", "0 2 * * *"), // Daily at 2 AM
			CleanupCron:        getEnv("CLEANUP_CRON", "0 3 * * 0"),        // Weekly on Sunday at 3 AM
			ShikimoriAPIURL:    getEnv("SHIKIMORI_API_URL", "https://shikimori.one/api"),
			ShikimoriAppName:   getEnv("SHIKIMORI_APP_NAME", "AnimeEnigma"),
			CatalogServiceURL:  getEnv("CATALOG_SERVICE_URL", "http://catalog:8081"),
			DataRetentionDays:  getEnvInt("DATA_RETENTION_DAYS", 90),
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
