// Package config loads the governor's environment configuration. The governor
// is deliberately storage-free: no Postgres, no JWT — its state is the
// in-memory score smoother + level quantizer plus the Redis keys it publishes.
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
	// LevelTTL is the Redis key TTL; sized to a few ticks so a dead governor
	// fails open (missing key = LevelNormal for every consumer).
	LevelTTL time.Duration
	// PromFailTicks is how many consecutive failed Prometheus polls flip the
	// governor to fail-open (publish LevelNormal + prometheus_unreachable).
	PromFailTicks int
	// ScoreAlphaUp / ScoreAlphaDown parameterize the single smoothed pressure
	// score's asymmetric EWMA (rise fast ~60s, decay slow ~5min). The level is
	// a hysteresis quantization of this score (no separate discrete machine).
	ScoreAlphaUp   float64
	ScoreAlphaDown float64
	// Level quantizer Schmitt thresholds on the smoothed score (0.5 ≈ elevated
	// breach, 1.0 ≈ critical breach). Exit < enter per tier gives the anti-flap
	// gap; the slow alphaDown supplies the exit-slow hold time.
	EnterElevated float64
	ExitElevated  float64
	EnterCritical float64
	ExitCritical  float64
	// StalenessMax: freshest-sample age above which the governor holds the level
	// rather than trusting a lagging signal (0 disables).
	StalenessMax time.Duration

	// Uplink-egress governance (opt-in). UplinkMbps <= 0 disables it (unknown
	// uplink must never false-positive). The fractions are the share of uplink
	// at which egress pressure trips elevated / critical.
	UplinkMbps         float64
	EgressElevatedFrac float64
	EgressCriticalFrac float64
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{Host: getEnv("SERVER_HOST", "0.0.0.0"), Port: getEnvInt("SERVER_PORT", 8100)},
		Redis: cache.Config{
			Host: getEnv("REDIS_HOST", "redis"), Port: getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""), DB: getEnvInt("REDIS_DB", 0),
		},
		PrometheusURL:      getEnv("GOVERNOR_PROMETHEUS_URL", "http://prometheus:9090/prometheus"),
		AnalyticsURL:       getEnv("GOVERNOR_ANALYTICS_URL", "http://analytics:8092"),
		Tick:               getEnvDuration("GOVERNOR_TICK", 15*time.Second),
		LevelTTL:           getEnvDuration("GOVERNOR_LEVEL_TTL", 60*time.Second),
		PromFailTicks:      getEnvInt("GOVERNOR_PROM_FAIL_TICKS", 3),
		ScoreAlphaUp:       getEnvFloat("GOVERNOR_SCORE_ALPHA_UP", 0.5),
		ScoreAlphaDown:     getEnvFloat("GOVERNOR_SCORE_ALPHA_DOWN", 0.05),
		EnterElevated:      getEnvFloat("GOVERNOR_ENTER_ELEVATED", 0.45),
		ExitElevated:       getEnvFloat("GOVERNOR_EXIT_ELEVATED", 0.20),
		EnterCritical:      getEnvFloat("GOVERNOR_ENTER_CRITICAL", 0.90),
		ExitCritical:       getEnvFloat("GOVERNOR_EXIT_CRITICAL", 0.55),
		StalenessMax:       getEnvDuration("GOVERNOR_STALENESS_MAX", 45*time.Second),
		UplinkMbps:         getEnvFloat("GOVERNOR_UPLINK_MBPS", 0),
		EgressElevatedFrac: getEnvFloat("GOVERNOR_EGRESS_ELEVATED_FRAC", 0.75),
		EgressCriticalFrac: getEnvFloat("GOVERNOR_EGRESS_CRITICAL_FRAC", 0.90),
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

func getEnvFloat(k string, d float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
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
