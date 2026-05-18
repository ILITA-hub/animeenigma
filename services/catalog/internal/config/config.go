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

type Config struct {
	Server    ServerConfig
	Database  database.Config
	Redis     cache.Config
	JWT       authz.JWTConfig
	Shikimori ShikimoriConfig
	Jimaku    JimakuConfig
	AnimeLib    AnimeLibConfig
	Hanime      HanimeConfig
	Telegram    TelegramConfig
	HealthCheck HealthCheckConfig
	Scraper     ScraperConfig
	// AllAnime — workstream raw-jp, Phase 01. Raw Japanese audio
	// provider backed by AllAnime's GraphQL persisted-query API.
	AllAnime AllAnimeConfig
	// OpenSubtitles — workstream raw-jp, Phase 02. Multi-language
	// subtitle source merged with Jimaku by the subs aggregator.
	OpenSubtitles OpenSubtitlesConfig
	// Library — workstream raw-jp, Phase 06 (v0.2). Hybrid resolver
	// consults this service first before falling back to AllAnime.
	Library LibraryConfig
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

type JimakuConfig struct {
	APIKey string
}

type AnimeLibConfig struct {
	Token string
}

type HanimeConfig struct {
	Email    string
	Password string
}

type HealthCheckConfig struct {
	Interval time.Duration
}

type TelegramConfig struct {
	NewsChannel string
}

// ScraperConfig configures the thin client targeting the scraper
// microservice (Phase 15+). APIURL defaults to http://scraper:8088 to
// match the docker-compose scraper service block. Timeout defaults to 15s.
type ScraperConfig struct {
	APIURL  string
	Timeout time.Duration
}

// AllAnimeConfig configures the AllAnime raw-JP parser (workstream raw-jp,
// Phase 01). Persisted-query SHA hashes rotate every few months; expose
// them as env vars so the operator can update without a code change.
type AllAnimeConfig struct {
	Domains          []string
	QuerySearchSHA   string
	QueryEpisodesSHA string
	QuerySourcesSHA  string
	HTTPTimeout      time.Duration
	Referer          string
	UserAgent        string
}

// OpenSubtitlesConfig — workstream raw-jp, Phase 02. Subtitle source.
type OpenSubtitlesConfig struct {
	APIKey    string
	UserAgent string
	Timeout   time.Duration
}

// LibraryConfig configures the catalog → library HTTP client used by
// the hybrid raw resolver (workstream raw-jp, Phase 06 / v0.2).
// APIURL defaults to http://library:8089 (Phase 1 port deviation —
// SPEC originally said 8087, actual deployment is 8089). Per-request
// Timeout defaults to 2s because the library is on the same docker
// network; any longer wait means it's actually down.
type LibraryConfig struct {
	APIURL  string
	Timeout time.Duration
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

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
			Database: getEnv("DB_NAME", "animeenigma"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret: getEnv("JWT_SECRET", ""),
			Issuer: getEnv("JWT_ISSUER", "animeenigma"),
		},
		Shikimori: ShikimoriConfig{
			BaseURL:    getEnv("SHIKIMORI_BASE_URL", "https://shikimori.io"),
			GraphQLURL: getEnv("SHIKIMORI_GRAPHQL_URL", "https://shikimori.io/api/graphql"),
			UserAgent:  getEnv("SHIKIMORI_USER_AGENT", "AnimeEnigma/1.0"),
			RateLimit:  getEnvInt("SHIKIMORI_RATE_LIMIT", 5), // requests per second
			Timeout:    getEnvDuration("SHIKIMORI_TIMEOUT", 30*time.Second),
		},
		Jimaku: JimakuConfig{
			APIKey: getEnv("JIMAKU_API_KEY", ""),
		},
		AnimeLib: AnimeLibConfig{
			Token: getEnv("ANIMELIB_TOKEN", ""),
		},
		Hanime: HanimeConfig{
			Email:    getEnv("HANIME_EMAIL", ""),
			Password: getEnv("HANIME_PASSWORD", ""),
		},
		Telegram: TelegramConfig{
			NewsChannel: getEnv("TELEGRAM_NEWS_CHANNEL", "animeenigmanews"),
		},
		HealthCheck: HealthCheckConfig{
			Interval: getEnvDuration("PLAYER_HEALTH_CHECK_INTERVAL", 5*time.Minute),
		},
		Scraper: ScraperConfig{
			APIURL:  getEnv("SCRAPER_API_URL", "http://scraper:8088"),
			Timeout: getEnvDuration("SCRAPER_TIMEOUT", 15*time.Second),
		},
		AllAnime: AllAnimeConfig{
			Domains:          splitCSV(getEnv("ALLANIME_DOMAINS", "allanime.day,allmanga.to,allanime.to")),
			QuerySearchSHA:   getEnv("ALLANIME_QUERY_SEARCH_SHA", ""),
			QueryEpisodesSHA: getEnv("ALLANIME_QUERY_EPISODES_SHA", ""),
			QuerySourcesSHA:  getEnv("ALLANIME_QUERY_SOURCES_SHA", ""),
			HTTPTimeout:      getEnvDuration("ALLANIME_HTTP_TIMEOUT", 10*time.Second),
			Referer:          getEnv("ALLANIME_REFERER", "https://allmanga.to/"),
			UserAgent:        getEnv("ALLANIME_USER_AGENT", "AnimeEnigma/1.0"),
		},
		OpenSubtitles: OpenSubtitlesConfig{
			APIKey:    getEnv("OPENSUBTITLES_API_KEY", ""),
			UserAgent: getEnv("OPENSUBTITLES_USER_AGENT", "AnimeEnigma/1.0"),
			Timeout:   getEnvDuration("OPENSUBTITLES_TIMEOUT", 10*time.Second),
		},
		Library: LibraryConfig{
			APIURL:  getEnv("LIBRARY_API_URL", "http://library:8089"),
			Timeout: getEnvDuration("LIBRARY_API_TIMEOUT", 2*time.Second),
		},
	}, nil
}

func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := []string{}
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
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
