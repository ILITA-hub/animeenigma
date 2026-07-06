package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

type Config struct {
	Server      ServerConfig
	JWT         authz.JWTConfig
	Services    ServiceURLs
	RateLimit   RateLimitConfig
	CORSOrigins []string
	Environment string // "production", "staging", "development", etc.
	DevMode     bool   // Skip admin auth when true (for local development)
	SiteURL     string // Public-facing base URL for OG meta tags (e.g. "https://animeenigma.org")
	// Phase 11 / UX-24 — env-backed system-status banner. When
	// SystemBannerActive=true AND SystemBannerMessage is non-empty,
	// GET /api/system/status returns a single Incident sourced from
	// these vars. Defaults: off + empty.
	SystemBannerActive  bool
	SystemBannerMessage string
	// GachaAdminOnly is the gacha (Лудка) dark-ship gate (spec §12). When
	// true, the /api/gacha/* route group additionally requires the admin
	// role, so the лудка is forbidden/invisible to regular users on the
	// live site until the bundled global-update release. Flip to false (env
	// GACHA_ADMIN_ONLY=false + restart-gateway) to open it to all
	// authenticated users. Default true.
	GachaAdminOnly bool
	// ProfileWallAdminOnly is the profile-showcase ("стена") dark-ship gate.
	// When true, the /api/users/{id}/showcase routes additionally require the
	// admin role, so the showcase is invisible to regular users until the
	// bundled release. Flip GACHA_ADMIN_ONLY + PROFILE_WALL_ADMIN_ONLY=false
	// together to reveal both. Default true.
	ProfileWallAdminOnly bool
	// PoisonClientIPs is the anti-scrape "tarpit" target list — exact IPs
	// and/or CIDR ranges (comma-separated env POISON_CLIENT_IPS). Requests
	// from these clients get structurally-valid but semantically-fake JSON
	// for known-scraped endpoints (see transport/poison.go), silently
	// corrupting the abuser's dataset instead of an obvious 403. Empty by
	// default (feature off). Change + `make restart-gateway` (no rebuild).
	PoisonClientIPs []string
	// ExternalAPIKey is the static shared secret for the /worker/* edge
	// (EXTERNAL_API_KEY env). Compared via subtle.ConstantTimeCompare.
	// Fail-closed: empty = reject all /worker/* requests. This is a COARSE
	// defense-in-depth filter, NOT the auth boundary — real per-worker auth
	// is the enroll→session→idx-bound-capability chain (Tasks 5/10).
	// Rotate: set a new value + `make restart-gateway` (no rebuild).
	ExternalAPIKey string
	// FanficAdminOnly is the AI fanfic-engine dark-ship gate (spec
	// 2026-07-06), mirroring GachaAdminOnly. When true, the /api/fanfic/*
	// route group additionally requires the admin role, so the feature is
	// forbidden/invisible to regular users until the bundled release. Flip
	// to false (env FANFIC_ADMIN_ONLY=false + restart-gateway) to open it to
	// all authenticated (non-guest) users. Default true.
	FanficAdminOnly bool
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

type ServiceURLs struct {
	AuthService      string
	CatalogService   string
	PlayerService    string
	RoomsService     string
	ScraperService   string // Phase 17 Plan 03: scraper service URL for /api/admin/scraper/*.
	StreamingService string
	ThemesService    string
	LibraryService   string // workstream raw-jp / v0.2 — library service on port 8087
	// NotificationsService — workstream notifications, v1.0 Phase 1. Port 8090
	// (8087 was unavailable: host-native maintenance bot already bound there;
	// same blocker that pushed library to 8089).
	NotificationsService string
	// WatchTogetherService — workstream watch-together, v1.0 Phase 1. Port 8091
	// (Redis-only co-watch service; exposes REST /api/watch-together/rooms/*
	// and WebSocket /api/watch-together/ws). The WS upgrade is proxied by a
	// dedicated WS reverse-proxy in transport/ws_proxy.go (the standard
	// ProxyService strips RFC 7230 §6.1 hop-by-hop headers, which is correct
	// for HTTP but breaks the WS handshake — see ws_proxy.go for the why).
	WatchTogetherService string
	// AnalyticsService — clickstream ingestion service. Port 8092.
	// Only POST /collect is gateway-routable (public, no JWT). The internal
	// erasure endpoint (/internal/erase) is Docker-network-only and is
	// intentionally never registered at the gateway (D-05 security model).
	AnalyticsService string
	// GachaService — workstream gacha (Лудка), Phase 1. Port 8093. Exposes
	// /api/gacha/* (JWT; admin-gated during dark-ship). The internal credit
	// endpoint (/internal/gacha/credit) is Docker-network-only and never
	// registered at the gateway (D-05 security model).
	GachaService string
	// RecsService — recommendation engine, extracted from player (spec
	// 2026-06-11). Port 8094.
	RecsService string
	// AnidleService — anime guessing game service (spec 2026-06-15). Port 8095.
	AnidleService string
	// UpscalerService — video upscaler service. Port 8096. Admin-gated REST
	// (/api/upscale/*). Internal segment-handle endpoints are Docker-network-only.
	UpscalerService string
	// FanficService — AI fanfic-generation engine (spec 2026-07-06). Port 8097.
	// Exposes /api/fanfic/* (JWT; admin-gated during dark-ship via
	// FanficAdminOnly). The SSE /generate route is proxied with per-chunk
	// flushing (proxyStreamFlush) so token deltas reach the browser live.
	FanficService string
	// PolicyService — RBAC + roulette policy service (RBAC and roulette
	// Phase 1). Port 8098. Exposes /api/policy/features/mine (JWT-optional)
	// and /api/admin/policy/* (JWT + admin). The internal ruleset endpoint
	// (/internal/policy/ruleset) is Docker-network-only and never registered
	// at the gateway (D-05 security model). This task is proxy-only wiring —
	// gateway-side enforcement of the ruleset against OTHER services (the
	// FeatureGate middleware) is Phase 2.
	PolicyService string
	WebService    string
	// Admin panel services
	GrafanaService    string
	PrometheusService string
	// Infrastructure services (for status page)
	SchedulerService string
	RedisAddr        string
	PostgresAddr     string
	NatsAddr         string
}

type RateLimitConfig struct {
	RequestsPerSecond int
	BurstSize         int

	// WV3-T3 — per-authenticated-user rate limit (GCRA / redis_rate).
	// Layered on top of the existing per-IP limiter above. Applied at
	// the gateway AFTER auth so the bucket key is user_id (not IP).
	// Anonymous traffic is unaffected by these knobs.
	UserPerMinute int // default 240 — env USER_RATE_LIMIT_PER_MINUTE
	UserBurst     int // default 40 — env USER_RATE_LIMIT_BURST
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	cfg := &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8000),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", ""),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", time.Hour),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		Services: ServiceURLs{
			AuthService:          getEnv("AUTH_SERVICE_URL", "http://auth:8080"),
			CatalogService:       getEnv("CATALOG_SERVICE_URL", "http://catalog:8081"),
			PlayerService:        getEnv("PLAYER_SERVICE_URL", "http://player:8083"),
			RoomsService:         getEnv("ROOMS_SERVICE_URL", "http://rooms:8084"),
			ScraperService:       getEnv("SCRAPER_SERVICE_URL", "http://scraper:8088"),
			StreamingService:     getEnv("STREAMING_SERVICE_URL", "http://streaming:8082"),
			ThemesService:        getEnv("THEMES_SERVICE_URL", "http://themes:8086"),
			LibraryService:       getEnv("LIBRARY_SERVICE_URL", "http://library:8089"),
			NotificationsService: getEnv("NOTIFICATIONS_SERVICE_URL", "http://notifications:8090"),
			WatchTogetherService: getEnv("WATCH_TOGETHER_SERVICE_URL", "http://watch-together:8091"),
			AnalyticsService:     getEnv("ANALYTICS_SERVICE_URL", "http://analytics:8092"),
			GachaService:         getEnv("GACHA_SERVICE_URL", "http://gacha:8093"),
			RecsService:          getEnv("RECS_SERVICE_URL", "http://recs:8094"),
			AnidleService:        getEnv("ANIDLE_SERVICE_URL", "http://anidle:8095"),
			UpscalerService:      getEnv("UPSCALER_SERVICE_URL", "http://upscaler:8096"),
			FanficService:        getEnv("FANFIC_SERVICE_URL", "http://fanfic:8097"),
			PolicyService:        getEnv("POLICY_SERVICE_URL", "http://policy:8098"),
			WebService:           getEnv("WEB_SERVICE_URL", "http://web:80"),
			// Admin panel services
			GrafanaService:    getEnv("GRAFANA_SERVICE_URL", "http://grafana:3000"),
			PrometheusService: getEnv("PROMETHEUS_SERVICE_URL", "http://prometheus:9090"),
			// Infrastructure services (for status page)
			SchedulerService: getEnv("SCHEDULER_SERVICE_URL", "http://scheduler:8085"),
			// REDIS_ADDR is an explicit escape hatch; when unset, derive the
			// addr from the REDIS_HOST(+REDIS_PORT) convention the rest of the
			// stack uses (audit finding L480). Compose sets only REDIS_HOST, so
			// without this derivation the per-user GCRA limiter would silently
			// fall back to the hardcoded "redis:6379" and break on any
			// non-default Redis topology (fail-open → limiter disabled).
			RedisAddr:    getEnv("REDIS_ADDR", getEnv("REDIS_HOST", "redis")+":"+getEnv("REDIS_PORT", "6379")),
			PostgresAddr: getEnv("POSTGRES_ADDR", "postgres:5432"),
			NatsAddr:     getEnv("NATS_ADDR", "nats:4222"),
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: getEnvInt("RATE_LIMIT_RPS", 100),
			BurstSize:         getEnvInt("RATE_LIMIT_BURST", 200),
			// WV3-T3 defaults, resized 2026-06-12. The original 60/min
			// burst-10 429'd legitimate SPA navigation: one profile load
			// fires ~10 authenticated calls (6 watchlist-tab prefetches +
			// statuses + sync + notifications), so a reload within the
			// 1-token/sec refill window tripped the bucket. 240/min with
			// burst 40 absorbs several rapid page loads while still
			// neutering scripted abuse (hundreds of req/min).
			UserPerMinute: getEnvInt("USER_RATE_LIMIT_PER_MINUTE", 240),
			UserBurst:     getEnvInt("USER_RATE_LIMIT_BURST", 40),
		},
		CORSOrigins: httputil.ParseCommaList(getEnv("CORS_ORIGINS", "")),
		Environment: strings.ToLower(getEnv("ENVIRONMENT", "")),
		DevMode:     getEnvBool("DEV_MODE", false),
		SiteURL:     strings.TrimRight(getEnv("SITE_URL", ""), "/"),
		// Phase 11 / UX-24 — system-status banner env vars.
		SystemBannerActive:  getEnvBool("SYSTEM_BANNER_ACTIVE", false),
		SystemBannerMessage: getEnv("SYSTEM_BANNER_MESSAGE", ""),
		// Gacha (Лудка) dark-ship admin-gate — default true (spec §12).
		GachaAdminOnly: getEnvBool("GACHA_ADMIN_ONLY", true),
		// Profile showcase ("стена") dark-ship gate — default true.
		ProfileWallAdminOnly: getEnvBool("PROFILE_WALL_ADMIN_ONLY", true),
		// Anti-scrape poison target list — empty = feature off.
		PoisonClientIPs: httputil.ParseCommaList(getEnv("POISON_CLIENT_IPS", "")),
		// Worker edge API key — empty = reject all /worker/* (fail-closed).
		ExternalAPIKey: getEnv("EXTERNAL_API_KEY", ""),
		// Fanfic engine dark-ship admin-gate — default true (spec 2026-07-06).
		FanficAdminOnly: getEnvBool("FANFIC_ADMIN_ONLY", true),
	}

	// DevMode is only permitted in known development environments. Any
	// other ENVIRONMENT value (including the empty string) fails closed.
	// See audit Wave 1 (S9): the previous deny-list missed misspellings,
	// staging, and the empty-string default.
	devEnvs := map[string]bool{
		"development": true,
		"dev":         true,
		"local":       true,
		"test":        true,
	}
	if cfg.DevMode && !devEnvs[cfg.Environment] {
		fmt.Fprintf(os.Stderr, "WARN: DEV_MODE=true is forbidden when ENVIRONMENT=%q — forcing DevMode=false\n", cfg.Environment)
		cfg.DevMode = false
	}

	return cfg, nil
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
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
