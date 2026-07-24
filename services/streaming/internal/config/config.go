package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
)

type Config struct {
	Server    ServerConfig
	Redis     cache.Config
	Storage   videoutils.StorageConfig
	// S3Storage is the OPTIONAL external S3-compatible backend (e.g.
	// s3.firstvds.ru) library episodes may also live on, alongside local
	// MinIO. Nil when S3_ENDPOINT is unset — the HLS proxy then presigns
	// only against Storage, same as before this field existed.
	S3Storage *videoutils.StorageConfig
	Proxy     videoutils.ProxyConfig
	JWT       authz.JWTConfig
	Stream    StreamConfig
	Providers ProvidersConfig
	// GachaInternalURL is the Docker-network base for the gacha service
	// (services/gacha, port 8093), used by the image proxy to resolve
	// relative /api/gacha/images/{cards,banners}/<key> URLs the frontend
	// sends for card/banner art resizing.
	GachaInternalURL string
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
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	if getEnv("STREAM_TOKEN_SECRET", "") == "" {
		return nil, fmt.Errorf("STREAM_TOKEN_SECRET environment variable is required")
	}

	allowedDomains := httputil.ParseCommaList(getEnv("PROXY_ALLOWED_DOMAINS", ""))

	// Second (optional) storage backend: external S3-compatible host for
	// library episodes, alongside local MinIO. S3_ENDPOINT/S3_ACCESS_KEY/
	// S3_SECRET_KEY are shared with the `backup` service's env vars, but
	// S3_BUCKET there is the DB-backups bucket ("animeenigma") — library
	// episodes live in a separate bucket, S3_LIBRARY_BUCKET, default
	// "raw-library" to match local MinIO's bucket name.
	var s3Storage *videoutils.StorageConfig
	if endpoint := getEnv("S3_ENDPOINT", ""); endpoint != "" {
		s3Storage = &videoutils.StorageConfig{
			Endpoint:        endpoint,
			AccessKeyID:     getEnv("S3_ACCESS_KEY", ""),
			SecretAccessKey: getEnv("S3_SECRET_KEY", ""),
			UseSSL:          getEnvBool("S3_USE_SSL", true),
			BucketName:      getEnv("S3_LIBRARY_BUCKET", "raw-library"),
		}
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
			// The HLS proxy presigns upstream reads on this host through this
			// Storage, and the only objects it legitimately reads are the
			// self-hosted library (`ae` provider) HLS the storage service
			// writes to MINIO_LIBRARY_BUCKET (its STORAGE_MINIO_BUCKET,
			// default "raw-library") — NOT MINIO_BUCKET, which holds admin
			// uploads + the image cache and is served through stream tokens
			// instead. Bounding the presign scope to that one bucket keeps the
			// MinIO root credential from signing a read of any other bucket on
			// the same server (gacha-cards, upscaler-output, backups, ...).
			PresignBuckets: []string{getEnv("MINIO_LIBRARY_BUCKET", "raw-library")},
		},
		S3Storage: s3Storage,
		Proxy: videoutils.ProxyConfig{
			// Full Firefox UA (see libs/videoutils DefaultProxyConfig): a real
			// browser engine token is required by UA-locked CDNs like okcdn.
			UserAgent:      getEnv("PROXY_USER_AGENT", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0"),
			Timeout:        getEnvDuration("PROXY_TIMEOUT", 30*time.Second),
			MaxBufferSize:  getEnvInt64("PROXY_MAX_BUFFER", 10*1024*1024),
			AllowedDomains: allowedDomains,
		},
		JWT: authz.JWTConfig{
			Secret: getEnv("JWT_SECRET", ""),
			Issuer: getEnv("JWT_ISSUER", "animeenigma"),
		},
		Stream: StreamConfig{
			TokenSecret:      getEnv("STREAM_TOKEN_SECRET", ""),
			TokenTTL:         getEnvDuration("STREAM_TOKEN_TTL", 4*time.Hour),
			MaxUploadSize:    getEnvInt64("MAX_UPLOAD_SIZE", 2*1024*1024*1024), // 2GB
			AllowedQualities: []string{"360p", "480p", "720p", "1080p"},
		},
		Providers: ProvidersConfig{
			Kodik: KodikConfig{
				APIKey:  getEnv("KODIK_API_KEY", ""),
				BaseURL: getEnv("KODIK_BASE_URL", "https://kodik-api.com"),
			},
			Aniboom: AniboomConfig{
				BaseURL: getEnv("ANIBOOM_BASE_URL", ""),
			},
		},
		GachaInternalURL: getEnv("GACHA_INTERNAL_URL", "http://gacha:8093"),
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
