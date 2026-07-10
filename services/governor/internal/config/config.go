// Package config loads the governor's environment configuration. The governor
// is deliberately storage-free: no Postgres, no JWT — its state is the
// in-memory hysteresis machine plus the Redis keys it publishes.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

type Config struct {
	Server ServerConfig
	Redis  cache.Config

	// PrometheusURL is the Prometheus base incl. route-prefix
	// (http://prometheus:9090/prometheus).
	PrometheusURL string
	// AnalyticsURL is the analytics service base for the Docker-network-only
	// transition sink (http://analytics:8092).
	AnalyticsURL string

	// Tick is the evaluation period.
	Tick time.Duration
	// EnterTicks / ExitTicks parameterize the enter-fast / exit-slow
	// hysteresis (defaults: 4 ticks ≈ 60s to raise, 20 ticks ≈ 5min to lower).
	EnterTicks int
	ExitTicks  int
	// LevelTTL is the Redis key TTL; sized to a few ticks so a dead governor
	// fails open (missing key = LevelNormal for every consumer).
	LevelTTL time.Duration
	// PromFailTicks is how many consecutive failed Prometheus polls flip the
	// governor to fail-open (publish LevelNormal + prometheus_unreachable).
	PromFailTicks int
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{Host: getEnv("SERVER_HOST", "0.0.0.0"), Port: getEnvInt("SERVER_PORT", 8099)},
		Redis: cache.Config{
			Host: getEnv("REDIS_HOST", "redis"), Port: getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""), DB: getEnvInt("REDIS_DB", 0),
		},
		PrometheusURL: getEnv("GOVERNOR_PROMETHEUS_URL", "http://prometheus:9090/prometheus"),
		AnalyticsURL:  getEnv("GOVERNOR_ANALYTICS_URL", "http://analytics:8092"),
		Tick:          getEnvDuration("GOVERNOR_TICK", 15*time.Second),
		EnterTicks:    getEnvInt("GOVERNOR_ENTER_TICKS", 4),
		ExitTicks:     getEnvInt("GOVERNOR_EXIT_TICKS", 20),
		LevelTTL:      getEnvDuration("GOVERNOR_LEVEL_TTL", 60*time.Second),
		PromFailTicks: getEnvInt("GOVERNOR_PROM_FAIL_TICKS", 3),
	}
	if cfg.Tick <= 0 {
		return nil, fmt.Errorf("GOVERNOR_TICK must be positive")
	}
	return cfg, nil
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
