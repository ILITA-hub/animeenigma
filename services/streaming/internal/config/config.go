package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
)

type Config struct {
	Server    ServerConfig
	Redis     cache.Config
	Storage   videoutils.StorageConfig
	Proxy     videoutils.ProxyConfig
	JWT       authz.JWTConfig
	Stream    StreamConfig
	Providers ProvidersConfig
}

// ProvidersConfig holds configuration for external video providers
type ProvidersConfig struct {
	Kodik   KodikConfig
	Aniboom AniboomConfig
}

type KodikConfig struct {
	APIKey  string
	BaseURL string
}

type AniboomConfig struct {
	BaseURL string
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type StreamConfig struct {
	TokenSecret    string
	TokenTTL       time.Duration
	MaxUploadSize  int64
	AllowedQualities []string
}

func Load() (*Config, error) {
	allowedDomains := strings.Split(getEnv("PROXY_ALLOWED_DOMAINS", ""), ",")
	if len(allowedDomains) == 1 && allowedDomains[0] == "" {
		allowedDomains = []string{} // Empty means allow all
	}

	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8082),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Storage: videoutils.StorageConfig{
			Endpoint:        getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKeyID:     getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretAccessKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
			UseSSL:          getEnvBool("MINIO_USE_SSL", false),
			BucketName:      getEnv("MINIO_BUCKET", "animeenigma"),
			Region:          getEnv("MINIO_REGION", "us-east-1"),
		},
		Proxy: videoutils.ProxyConfig{
			UserAgent:      getEnv("PROXY_USER_AGENT", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
			Timeout:        getEnvDuration("PROXY_TIMEOUT", 30*time.Second),
			MaxBufferSize:  getEnvInt64("PROXY_MAX_BUFFER", 10*1024*1024),
			AllowedDomains: allowedDomains,
		},
		JWT: authz.JWTConfig{
			Secret: getEnv("JWT_SECRET", "your-secret-key"),
			Issuer: getEnv("JWT_ISSUER", "animeenigma"),
		},
		Stream: StreamConfig{
			TokenSecret:      getEnv("STREAM_TOKEN_SECRET", "stream-secret-key"),
			TokenTTL:         getEnvDuration("STREAM_TOKEN_TTL", 4*time.Hour),
			MaxUploadSize:    getEnvInt64("MAX_UPLOAD_SIZE", 2*1024*1024*1024), // 2GB
			AllowedQualities: []string{"360p", "480p", "720p", "1080p"},
		},
		Providers: ProvidersConfig{
			Kodik: KodikConfig{
				APIKey:  getEnv("KODIK_API_KEY", ""),
				BaseURL: getEnv("KODIK_BASE_URL", "https://kodikapi.com"),
			},
			Aniboom: AniboomConfig{
				BaseURL: getEnv("ANIBOOM_BASE_URL", ""),
			},
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

func getEnvInt64(key string, defaultVal int64) int64 {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
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
