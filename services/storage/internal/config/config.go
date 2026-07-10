// Package config holds the storage service runtime configuration.
//
// Loaded once at boot via Load(); every setting has a safe default so the
// service boots in dev with just MinIO (no S3 creds needed). If
// STORAGE_S3_ENDPOINT is empty the s3 backend is absent — placement
// resolving to "s3" falls back to "minio" (service.Placement) so dev
// environments keep working without the external S3.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/storage/internal/domain"
)

// Config is the fully-resolved storage service configuration.
type Config struct {
	Server ServerConfig
	Minio  BackendConfig
	S3     BackendConfig // Endpoint == "" means the s3 backend is absent

	// Defaults maps a content class (domain.Class*) to its default backend
	// id (domain.Backend*). Per-request "override" only applies to
	// domain.ClassLibraryManual; see service.Placement.
	Defaults map[string]string
}

// ServerConfig is the HTTP listener configuration.
type ServerConfig struct {
	Host string
	Port int
}

// Address returns the host:port pair for http.Server.Addr.
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// BackendConfig holds one S3-compatible backend's connection settings.
type BackendConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// Load reads STORAGE_* environment variables, falling back to the spec
// defaults for every field.
func Load() (*Config, error) {
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("STORAGE_PORT", 8099),
		},
		Minio: BackendConfig{
			Endpoint:  getEnv("STORAGE_MINIO_ENDPOINT", "minio:9000"),
			AccessKey: getEnv("STORAGE_MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey: getEnv("STORAGE_MINIO_SECRET_KEY", "minioadmin"),
			Bucket:    getEnv("STORAGE_MINIO_BUCKET", "raw-library"),
			UseSSL:    getEnvBool("STORAGE_MINIO_USE_SSL", false),
		},
		S3: BackendConfig{
			Endpoint:  getEnv("STORAGE_S3_ENDPOINT", ""),
			AccessKey: getEnv("STORAGE_S3_ACCESS_KEY", ""),
			SecretKey: getEnv("STORAGE_S3_SECRET_KEY", ""),
			Bucket:    getEnv("STORAGE_S3_BUCKET", "raw-library"),
			UseSSL:    getEnvBool("STORAGE_S3_USE_SSL", true),
		},
		Defaults: map[string]string{
			domain.ClassLibraryAuto:   getEnv("STORAGE_CLASS_LIBRARY_AUTO", domain.BackendS3),
			domain.ClassLibraryManual: getEnv("STORAGE_CLASS_LIBRARY_MANUAL", domain.BackendMinio),
			domain.ClassUpscaled:      getEnv("STORAGE_CLASS_UPSCALED", domain.BackendS3),
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

// getEnvBool parses a permissive boolean: "1", "true", "yes", "on" (any
// case) -> true; "0", "false", "no", "off" (any case) -> false; anything
// else (including unset) returns the default.
func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultVal
	}
}
