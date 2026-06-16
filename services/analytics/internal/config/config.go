package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

type Config struct {
	Server        ServerConfig
	Database      database.Config
	Redis         cache.Config
	IPSalt        string
	RetentionDays int
	PurgeCron     string
	MaxBatch      int
	FlushInterval time.Duration
	BufferSize    int

	// StoreBackend selects the EventStore implementation. One of
	// "postgres" (default — keeps the system exactly as today), "clickhouse"
	// (CH-only), or "dualwrite" (PG source-of-truth + CH best-effort fan-out).
	// Default stays "postgres" for the reversibility guarantee (RESEARCH §Migration).
	StoreBackend string
	// ClickHouse holds the native-protocol connection params for the CH backend.
	// Only consulted when StoreBackend is "clickhouse" or "dualwrite".
	ClickHouse ClickHouseConfig
}

// ClickHouseConfig mirrors the env-driven Database config shape for the native
// ClickHouse connection (CLICKHOUSE_* envs). Host/Port are joined into the
// native host:port address consumed by repo.OpenClickHouse.
type ClickHouseConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

// Addr returns the host:port native-protocol address (e.g. "clickhouse:9000").
func (c ClickHouseConfig) Addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }

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
		// Redis carries the read_thresholds hash that the daily db_read P95
		// recompute publishes (D-03). DB 2 matches the scheduler/catalog shared
		// instance so the GORM services read the same hash.
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "redis"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 2),
		},
		IPSalt:        getEnv("ANALYTICS_IP_SALT", "change-me-in-production"),
		RetentionDays: getEnvInt("ANALYTICS_RETENTION_DAYS", 90),
		PurgeCron:     getEnv("ANALYTICS_PURGE_CRON", "17 3 * * *"),
		MaxBatch:      getEnvInt("ANALYTICS_MAX_BATCH", 500),
		FlushInterval: getEnvDuration("ANALYTICS_FLUSH_INTERVAL", time.Second),
		BufferSize:    getEnvInt("ANALYTICS_BUFFER_SIZE", 10000),
		StoreBackend:  getEnv("ANALYTICS_STORE_BACKEND", "postgres"),
		ClickHouse: ClickHouseConfig{
			Host:     getEnv("CLICKHOUSE_HOST", "clickhouse"),
			Port:     getEnvInt("CLICKHOUSE_PORT", 9000),
			Database: getEnv("CLICKHOUSE_DB", "analytics"),
			User:     getEnv("CLICKHOUSE_USER", "analytics"),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),
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
