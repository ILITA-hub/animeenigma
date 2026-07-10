package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

type Config struct {
	Server      ServerConfig
	Database    database.Config
	Redis       cache.Config
	JWT         authz.JWTConfig
	Shikimori   ShikimoriConfig
	Jimaku      JimakuConfig
	AnimeLib    AnimeLibConfig
	Hanime      HanimeConfig
	Telegram    TelegramConfig
	HealthCheck HealthCheckConfig
	Scraper     ScraperConfig
	// OpenSubtitles — workstream raw-jp, Phase 02. Multi-language
	// subtitle source merged with Jimaku by the subs aggregator.
	OpenSubtitles OpenSubtitlesConfig
	// Anime365 — RU fansub aggregator (smotret-anime). Spec 2026-06-24.
	Anime365 Anime365Config
	// Library — self-hosted MinIO HLS source for the first-party ("ae")
	// provider and the raw JP provider (library-only since 2026-06-22).
	Library LibraryConfig
	// SpotlightEnabled gates GET /api/home/spotlight (workstream
	// hero-spotlight, v1.0 Phase 1). When false the handler returns
	// a bare 404 with no body so the frontend HSB-FE-02 v-if hides
	// the block. Default true. Env: SPOTLIGHT_ENABLED.
	SpotlightEnabled bool
	// Prometheus — base URL for the spotlight platform_stats card's
	// instant queries (workstream hero-spotlight). Default mirrors the
	// gateway's PROMETHEUS_SERVICE_URL incl. the /prometheus route-prefix.
	Prometheus PrometheusConfig
	// ProviderPolicy — thresholds + cadences for the probe-result endpoint's
	// ApplyVerdict state machine (Task 6 / self-healing Phase 3).
	ProviderPolicy ProviderPolicyConfig
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type ShikimoriConfig struct {
	BaseURL    string
	GraphQLURL string
	UserAgent  string
	RateLimit  int
	Timeout    time.Duration
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
// match the docker-compose scraper service block. Timeout defaults to 40s
// — must stay above SCRAPER_BROWSER_PROVIDER_TIMEOUT (35s, scraper's own
// per-provider budget for engine=browser providers' cold Cloudflare/
// Turnstile solves), or catalog cuts the request before that budget ever
// gets to run (ISS-022 follow-up, 2026-07-08 animepahe recovery).
type ScraperConfig struct {
	APIURL  string
	Timeout time.Duration
}

// OpenSubtitlesConfig — workstream raw-jp, Phase 02. Subtitle source.
type OpenSubtitlesConfig struct {
	APIKey    string
	UserAgent string
	Timeout   time.Duration
}

// Anime365Config — Russian subtitle source (smotret-anime / anime365).
// No API key; only a base URL + enable flag.
type Anime365Config struct {
	BaseURL string
	Enabled bool
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

// PrometheusConfig configures the read-only Prometheus instant-query
// client used by the spotlight platform_stats card (workstream
// hero-spotlight). URL defaults to http://prometheus:9090/prometheus
// which mirrors the gateway's PROMETHEUS_SERVICE_URL env var.
type PrometheusConfig struct {
	URL string
}

// ProviderPolicyConfig holds the thresholds and cadences used by the
// probe-result endpoint (Task 6) to apply ApplyVerdict transitions.
// PromoteAfter: how long a recovering provider must pass before up.
// Cadence: per-health-state probe intervals + sample sizes.
// (The demote-after threshold was dropped 2026-07-08 — policy is admin-only
// now; the probe machine never auto-demotes auto→manual.)
type ProviderPolicyConfig struct {
	Cadence      domain.CadenceConfig
	PromoteAfter time.Duration
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
			Timeout: getEnvDuration("SCRAPER_TIMEOUT", 40*time.Second),
		},
		OpenSubtitles: OpenSubtitlesConfig{
			APIKey:    getEnv("OPENSUBTITLES_API_KEY", ""),
			UserAgent: getEnv("OPENSUBTITLES_USER_AGENT", "AnimeEnigma/1.0"),
			Timeout:   getEnvDuration("OPENSUBTITLES_TIMEOUT", 10*time.Second),
		},
		Anime365: Anime365Config{
			BaseURL: getEnv("ANIME365_BASE_URL", "https://smotret-anime.org"),
			Enabled: getEnvBool("ANIME365_ENABLED", true),
		},
		Library: LibraryConfig{
			APIURL:  getEnv("LIBRARY_API_URL", "http://library:8089"),
			Timeout: getEnvDuration("LIBRARY_API_TIMEOUT", 2*time.Second),
		},
		SpotlightEnabled: getEnvBool("SPOTLIGHT_ENABLED", true),
		Prometheus: PrometheusConfig{
			URL: getEnv("PROMETHEUS_SERVICE_URL", "http://prometheus:9090/prometheus"),
		},
		ProviderPolicy: ProviderPolicyConfig{
			Cadence: domain.CadenceConfig{
				Up:               getEnvDuration("PROBE_CADENCE_UP", 6*time.Hour),
				Recovering:       getEnvDuration("PROBE_CADENCE_RECOVERING", 12*time.Hour),
				Manual:           getEnvDuration("PROBE_CADENCE_MANUAL", 24*time.Hour),
				RecoveringSample: getEnvInt("PROBE_RECOVERING_SAMPLE", 3),
				FullSample:       getEnvInt("PROBE_FULL_SAMPLE", 5),
			},
			PromoteAfter: getEnvDuration("PROVIDER_PROMOTE_AFTER", 24*time.Hour),
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

// getEnvBool parses a boolean env var via strconv.ParseBool. Accepts
// "true"/"false"/"1"/"0"/"t"/"f"/"True"/"False" etc. On unset OR parse
// failure (e.g. "garbage"), returns defaultVal. Used for feature flags
// like SPOTLIGHT_ENABLED (workstream hero-spotlight, v1.0 Phase 1).
func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
