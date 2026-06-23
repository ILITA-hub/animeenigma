package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

// Config is the upscaler service top-level config.
type Config struct {
	Server   ServerConfig
	Database database.Config
	Redis    cache.Config
	JWT      authz.JWTConfig
	Upscaler UpscalerConfig
}

// ServerConfig holds the HTTP server listen address.
type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// MinioConfig drives the upscaler's MinIO writer (staging uploads).
// Mirrors services/library/internal/config.MinioConfig.
type MinioConfig struct {
	Endpoint          string
	AccessKey         string
	SecretKey         string
	Bucket            string
	UseSSL            bool
	UploadConcurrency int
}

// UpscalerConfig holds upscaler-specific knobs.
type UpscalerConfig struct {
	LibraryURL          string
	MinIO               MinioConfig
	JobCapabilitySecret string
	SegmentSeconds      int
	DefaultScale        int
	RemoteShellEnabled  bool
	StagingDir          string
	TorrentsDir         string
}

// Load reads environment variables and returns a validated Config.
// Returns an error if JWT_SECRET is not set (caller should log.Fatalw).
func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8096),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "upscaler"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "redis"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", ""),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		Upscaler: UpscalerConfig{
			LibraryURL:          getEnv("LIBRARY_URL", "http://library:8089"),
			JobCapabilitySecret: getEnv("JOB_CAPABILITY_SECRET", ""),
			SegmentSeconds:      getEnvInt("SEGMENT_SECONDS", 45),
			DefaultScale:        getEnvInt("DEFAULT_SCALE", 2),
			RemoteShellEnabled:  getEnvBool("REMOTE_SHELL_ENABLED", true),
			StagingDir:          getEnv("UPSCALE_STAGING_DIR", "/data/upscale-staging"),
			TorrentsDir:         getEnv("LIBRARY_TORRENTS_DIR", "/data/torrents"),
			MinIO:               loadMinIO(),
		},
	}, nil
}

// loadMinIO reads MinIO connection settings from environment variables,
// mirroring the pattern from services/library/internal/config.
func loadMinIO() MinioConfig {
	return MinioConfig{
		Endpoint:          getEnv("MINIO_ENDPOINT", "minio:9000"),
		AccessKey:         getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		SecretKey:         getEnv("MINIO_SECRET_KEY", "minioadmin"),
		Bucket:            getEnv("MINIO_BUCKET", "upscaler"),
		UseSSL:            getEnvBool("MINIO_USE_SSL", false),
		UploadConcurrency: getEnvInt("MINIO_UPLOAD_CONCURRENCY", 4),
	}
}

// getEnvBool returns the boolean value of an env var, parsing common
// truthy/falsy strings. Defaults to defaultVal on parse failure or empty.
// Recognised true: "true", "1", "yes", "on" (case-insensitive).
func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	switch strings.ToLower(val) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
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
