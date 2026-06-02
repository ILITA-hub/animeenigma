package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
)

type Config struct {
	Server        ServerConfig
	Database      database.Config
	IPSalt        string
	RetentionDays int
	PurgeCron     string
	MaxBatch      int
	FlushInterval time.Duration
	BufferSize    int
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

func Load() (*Config, error) {
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8092),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "postgres"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		IPSalt:        getEnv("ANALYTICS_IP_SALT", "change-me-in-production"),
		RetentionDays: getEnvInt("ANALYTICS_RETENTION_DAYS", 90),
		PurgeCron:     getEnv("ANALYTICS_PURGE_CRON", "17 3 * * *"),
		MaxBatch:      getEnvInt("ANALYTICS_MAX_BATCH", 500),
		FlushInterval: getEnvDuration("ANALYTICS_FLUSH_INTERVAL", time.Second),
		BufferSize:    getEnvInt("ANALYTICS_BUFFER_SIZE", 10000),
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
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}
func getEnvDuration(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if p, err := time.ParseDuration(v); err == nil {
			return p
		}
	}
	return d
}
