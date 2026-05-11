package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the scraper service configuration.
//
// Phase 15 plan 03 nests megacloud-extractor settings into their own struct so
// new providers' configs can land alongside without flattening the top level.
type Config struct {
	Server             ServerConfig
	MegacloudExtractor MegacloudExtractorConfig
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

// MegacloudExtractorConfig configures the HTTP client that talks to the
// docker/megacloud-extractor sidecar. URL defaults to the docker-compose
// service name; Timeout defaults to 15s to match the sidecar's internal
// req.setTimeout(15000) (see docker/megacloud-extractor/server.js).
type MegacloudExtractorConfig struct {
	URL     string
	Timeout time.Duration
}

// Load reads configuration from environment variables, falling back to
// sensible defaults that work inside the docker-compose network.
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8088),
		},
		MegacloudExtractor: MegacloudExtractorConfig{
			URL:     getEnv("MEGACLOUD_EXTRACTOR_URL", "http://megacloud-extractor:3200"),
			Timeout: getEnvDuration("MEGACLOUD_EXTRACTOR_TIMEOUT", 15*time.Second),
		},
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

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
