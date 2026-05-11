package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds the scraper service configuration.
//
// Phase 15 plan 01: only the server bind address and the megacloud-extractor
// sidecar URL are needed. Per-provider config (cookies, rate limits, base
// URLs) will be added by later plans when each provider lands.
type Config struct {
	Server                ServerConfig
	MegacloudExtractorURL string
}

// ServerConfig controls the HTTP listener.
type ServerConfig struct {
	Host string
	Port int
}

// Address returns the host:port the HTTP server binds to.
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// Load reads configuration from environment variables, falling back to
// sensible defaults that work inside the docker-compose network.
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8088),
		},
		MegacloudExtractorURL: getEnv("MEGACLOUD_EXTRACTOR_URL", "http://megacloud-extractor:3200"),
	}
	return cfg, nil
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
