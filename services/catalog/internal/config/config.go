package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

type Config struct {
	Server    ServerConfig
	Database  database.Config
	Redis     cache.Config
	JWT       authz.JWTConfig
	Shikimori ShikimoriConfig
	HiAnime   HiAnimeConfig
	Consumet  ConsumetConfig
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type ShikimoriConfig struct {
	BaseURL     string
	GraphQLURL  string
	UserAgent   string
	RateLimit   int
	Timeout     time.Duration
}

type HiAnimeConfig struct {
	AniwatchAPIURL string
}

type ConsumetConfig struct {
	APIURL string
}

func Load() (*Config, error) {
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8081),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma_catalog"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret: getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
			Issuer: getEnv("JWT_ISSUER", "animeenigma"),
		},
		Shikimori: ShikimoriConfig{
			BaseURL:    getEnv("SHIKIMORI_BASE_URL", "https://shikimori.one"),
			GraphQLURL: getEnv("SHIKIMORI_GRAPHQL_URL", "https://shikimori.one/api/graphql"),
			UserAgent:  getEnv("SHIKIMORI_USER_AGENT", "AnimeEnigma/1.0"),
			RateLimit:  getEnvInt("SHIKIMORI_RATE_LIMIT", 5), // requests per second
			Timeout:    getEnvDuration("SHIKIMORI_TIMEOUT", 30*time.Second),
		},
		HiAnime: HiAnimeConfig{
			AniwatchAPIURL: getEnv("ANIWATCH_API_URL", "http://aniwatch:4000"),
		},
		Consumet: ConsumetConfig{
			APIURL: getEnv("CONSUMET_API_URL", "http://consumet:3000"),
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

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
