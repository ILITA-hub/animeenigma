package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

func NewRouter(
	proxyHandler *handler.ProxyHandler,
	cfg *config.Config,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	h, _ := NewRouterWithCleanup(proxyHandler, cfg, log, metricsCollector, nil)
	return h
}

// NewRouterWithCleanup is the test-friendly variant of NewRouter. Returns
// the handler AND a Cleanup function that stops the per-IP rate-limiter's
// background eviction goroutine. Production callers can keep using
// NewRouter (the goroutine lives as long as the process). Test callers
// MUST register cleanup via `t.Cleanup(cleanup)` so each test does not
// leak one goroutine per `NewRouter` invocation — see REVIEW.md WR-04.
//
// The redisClient param (WV3-T3) is optional. When non-nil it enables the
// per-authenticated-user GCRA rate limiter (UserRateLimitMiddleware) layered
// on top of every protected route group — IPRateLimiter → JWT → user_rate
// → handler. Passing nil leaves the protected groups exactly as they were
// before WV3-T3 so tests that don't care about the user limiter (and code
// paths that intentionally run without Redis) keep working.
func NewRouterWithCleanup(
	proxyHandler *handler.ProxyHandler,
	cfg *config.Config,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
	redisClient *redis.Client,
) (http.Handler, func()) {
	if cfg.DevMode {
		log.Warnw("⚠️  DEV MODE ENABLED — admin auth is BYPASSED. Do NOT use in production!")
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS(cfg.CORSOrigins))
	r.Use(httputil.SecurityHeaders)
	r.Use(middleware.RealIP)
	r.Use(MaxBodySizeMiddleware(10 * 1024 * 1024)) // 10MB
	rateLimitMW, rateLimiter := RateLimitMiddlewareWithStop(cfg.RateLimit)
	r.Use(rateLimitMW)

	// WV3-T3: per-authenticated-user rate limit (GCRA / redis_rate). Built
	// ONCE here and applied inside every protected route group AFTER its
	// JWT middleware so the bucket key is user_id (not IP). When the
	// caller passes a nil Redis client (older NewRouter signature, or
	// tests that don't care), the user limiter degrades to a pass-through
	// — same effect as having no extra middleware at all.
	userRateLimit := newUserRateLimitChainFn(redisClient, cfg.RateLimit.UserPerMinute, cfg.RateLimit.UserBurst, log)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// Open Graph meta tag routes (for social media crawlers)
	// HEAD is needed because Telegram sends HEAD before GET
	ogHandler := handler.NewOpenGraphHandler(cfg.Services.CatalogService, cfg.Services.AuthService, cfg.Services.PlayerService, cfg.SiteURL, log)
	r.Get("/og/anime/{animeId}", ogHandler.ServeAnime)
	r.Head("/og/anime/{animeId}", ogHandler.ServeAnime)
	r.Get("/og/home", ogHandler.ServeHome)
	r.Head("/og/home", ogHandler.ServeHome)
	r.Get("/og/user/{publicId}", ogHandler.ServeUser)
	r.Head("/og/user/{publicId}", ogHandler.ServeUser)

	// Public status endpoints (aggregated health of all services)
	statusHandler := handler.NewStatusHandler(cfg.Services, log)
	r.Get("/api/status", statusHandler.GetStatus)
	r.Get("/status/health", statusHandler.GetHealthCheck)

	// Metrics endpoint
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// OpenAPI spec
	r.Get("/openapi.json", proxyHandler.GetOpenAPISpec)

	// Admin panel routes (protected by admin role, unless DevMode is enabled)
	r.Route("/admin", func(r chi.Router) {
		if !cfg.DevMode {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(AdminRoleMiddleware)
		}

		// Admin dashboard landing page
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
    <title>AnimeEnigma Admin</title>
    <style>
        body { font-family: -apple-system, sans-serif; max-width: 600px; margin: 50px auto; padding: 20px; }
        h1 { color: #333; }
        a { display: block; padding: 15px; margin: 10px 0; background: #f5f5f5; text-decoration: none; color: #333; border-radius: 8px; }
        a:hover { background: #e0e0e0; }
    </style>
</head>
<body>
    <h1>AnimeEnigma Admin</h1>
    <a href="/admin/grafana/">Grafana - Metrics & Dashboards</a>
    <a href="/admin/prometheus/">Prometheus - Metrics Database</a>
    <a href="/admin/grafana/explore?orgId=1&left=%7B%22datasource%22:%22Loki%22%7D">Loki - Log Explorer (via Grafana)</a>
    <a href="/admin/recs/">Rec Engine Debug — per-user signal breakdown, force-recompute, S11 audit</a>
</body>
</html>`))
		})

		r.HandleFunc("/grafana/*", proxyHandler.ProxyToGrafana)
		r.HandleFunc("/prometheus/*", proxyHandler.ProxyToPrometheus)
		r.HandleFunc("/loki/*", proxyHandler.ProxyToLoki)

		// Phase 14: /admin/recs/* falls through to the web service so the Vue
		// SPA's AdminRecs.vue route can render. Without this, chi would 404 any
		// /admin path it doesn't explicitly know — including the new SPA
		// admin debug page. Both /admin/recs (no trailing slash) and
		// /admin/recs/{user_id} are covered. Auth is already enforced by the
		// /admin route group's JWT + AdminRoleMiddleware above.
		r.HandleFunc("/recs", proxyHandler.ProxyToWeb)
		r.HandleFunc("/recs/*", proxyHandler.ProxyToWeb)

		// Phase 17 (UX-33): /admin/collections/* falls through to the web
		// SPA so AdminCollections.vue / AdminCollectionEdit.vue can render.
		// Auth is already enforced by the surrounding /admin group's JWT
		// + AdminRoleMiddleware above.
		r.HandleFunc("/collections", proxyHandler.ProxyToWeb)
		r.HandleFunc("/collections/*", proxyHandler.ProxyToWeb)
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Auth service routes (public)
		r.HandleFunc("/auth/*", proxyHandler.ProxyToAuth)

		// Phase 11 / UX-24 — System status banner (public, no JWT).
		// Sourced from gateway env (SYSTEM_BANNER_ACTIVE +
		// SYSTEM_BANNER_MESSAGE). The existing CORS / rate-limit /
		// security-headers stack at the top of NewRouter already applies.
		sysStatusHandler := handler.NewSystemStatusHandler(cfg)
		r.Get("/system/status", sysStatusHandler.GetStatus)

		// Player service routes - reviews (must be before /anime/* catch-all)
		r.Post("/anime/ratings/batch", proxyHandler.ProxyToPlayer)
		r.Get("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
		r.Get("/anime/{animeId}/reviews/me", proxyHandler.ProxyToPlayer)
		r.Post("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
		r.Delete("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
		r.Get("/anime/{animeId}/rating", proxyHandler.ProxyToPlayer)

		// Phase 14 (ui-ux-audit / UX-28) — soft social-proof follower count
		// proxied to player. Public, no JWT required. Must be registered BEFORE
		// the generic /anime/* → catalog catch-all below; otherwise chi would
		// route this path to the catalog service.
		r.Get("/anime/{animeId}/watchers-count", proxyHandler.ProxyToPlayer)

		// Player service routes - comments (must be before /anime/* catch-all)
		// GET is public; mutations (POST/PATCH/DELETE) gate at the gateway
		// for defense-in-depth — REVIEW.md CR-04. The player still runs
		// AuthMiddleware downstream, but enforcing JWT at the gateway
		// keeps unauthenticated traffic from reaching the player at all
		// and preserves the rate-limit-before-auth ordering.
		r.Get("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Post("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
			r.Patch("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
			r.Delete("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
		})

		// Catalog service routes (public)
		r.HandleFunc("/anime", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/anime/*", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/genres", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/kodik/*", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/animelib/*", proxyHandler.ProxyToCatalog)
		// Phase 18 (UX-34) — Skip-Intro / Skip-Outro CTA timestamps.
		// Public, no auth. Proxied to catalog which fronts api.aniskip.com
		// with a 7d cache. Registered alongside the other public catalog
		// passthrough routes; ordering matters less here because the URL
		// prefix /skip-times/* doesn't collide with any /admin/* path —
		// but we keep it BEFORE the /admin/* admin-gated group below for
		// the same "specific-before-general" convention used throughout
		// this file (admin proxies catch /api/admin/* unconditionally).
		r.HandleFunc("/skip-times/*", proxyHandler.ProxyToCatalog)
		// Phase 17 (UX-33) — public editorial collections. /api/admin/collections/*
		// is covered by the existing /admin/* admin-gated group below.
		r.HandleFunc("/collections", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/collections/*", proxyHandler.ProxyToCatalog)

		// Workstream hero-spotlight, v1.0 Phase 1 (HSB-BE-06) — hero spotlight
		// aggregator. Public surface (anonymous allowed). Phase 3 adds 3
		// login-only cards (personal_pick, not_time_yet, continue_watching_new)
		// so the gateway MUST resolve ak_-API-key Bearer tokens into freshly
		// minted JWTs here — otherwise the catalog OptionalAuthMiddleware sees
		// an unrecognized opaque Bearer string and falls back to "anon", and
		// the 3 personalized resolvers stay invisible to api-key callers.
		// (Bug found during Plan 03-07 execution — Rule 2 auto-fix.)
		// Mounts at /api/home/spotlight; the catalog proxy path-rewrite
		// is a no-op so the catalog router sees the same path. Registered
		// alongside the other public catalog passthroughs above; /home/* does
		// not collide with /anime/* but the "specific-before-general" placement
		// convention is project-wide.
		r.Group(func(r chi.Router) {
			r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.HandleFunc("/home/spotlight", proxyHandler.ProxyToCatalog)
		})

		// Phase 17 Plan 03: admin scraper routes (protected, proxied to scraper).
		// CRITICAL ORDER — this group MUST be registered BEFORE the generic
		// /admin/* → catalog group below. chi resolves routes in registration
		// order; the /api/admin/recs/* group at the bottom of this file is the
		// existing precedent for the same "specific-before-general" gotcha.
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(AdminRoleMiddleware)
			r.HandleFunc("/admin/scraper/*", proxyHandler.ProxyToScraper)
		})

		// Admin routes (protected, proxied to catalog) — MUST stay AFTER the
		// more-specific /admin/scraper/* group above.
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(AdminRoleMiddleware)
			r.HandleFunc("/admin/*", proxyHandler.ProxyToCatalog)
		})

		// Player service routes - public watchlist
		r.Get("/users/{userId}/watchlist/public", proxyHandler.ProxyToPlayer)
		r.Get("/users/{userId}/watchlist/public/stats", proxyHandler.ProxyToPlayer)

		// Public activity feed
		r.Get("/activity/feed", proxyHandler.ProxyToPlayer)

		// Player service routes - preferences (public, OptionalAuth on player side)
		// Per CONTEXT Critical Finding 1: must NOT be inside the JWT-protected /users/* group,
		// because anonymous users (no Authorization header) need to POST overrides + resolve.
		// T-01-01: anon-friendly endpoint is a DDoS amplification target — the player handler
		// rejects requests with neither claims nor X-Anon-ID, but the gateway also adds rate
		// limiting at the path level (existing rate limiter applies to all /api/* paths).
		r.HandleFunc("/preferences/*", proxyHandler.ProxyToPlayer)

		// Phase 10/11 (recs): anonymous trending row + logged-in "Up Next for you" row.
		// MUST be defined BEFORE the protected /users/* group so chi's longest-prefix
		// match catches /users/recs first. The OptionalJWTValidationMiddleware
		// (a) lets anonymous traffic through untouched, (b) resolves "ak_…" API keys
		// to a freshly-minted JWT that downstream OptionalAuthMiddleware can validate,
		// (c) validates real JWTs in place. Without this carve-out, ak_-key callers
		// would silently fall through to the anonymous trending row (Phase 11 bug
		// caught during Task 9 verification).
		r.Group(func(r chi.Router) {
			r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.HandleFunc("/users/recs", proxyHandler.ProxyToPlayer)
			r.HandleFunc("/users/recs/", proxyHandler.ProxyToPlayer)
		})

		// Phase 14 (REC-ADMIN-01 / REC-ADMIN-02): admin debug + force-recompute
		// routes proxied to the player service. JWT validation + admin role
		// gate at the gateway layer (defense-in-depth — player applies the same
		// gates again in services/player/internal/transport/router.go).
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(AdminRoleMiddleware)
			r.HandleFunc("/admin/recs/*", proxyHandler.ProxyToPlayer)
		})

		// Phase 14 (REC-EVAL-01): public telemetry endpoint. JWT-OPTIONAL so
		// anonymous CTR data flows from the trending row. Player applies its
		// own OptionalAuthMiddleware on the inner /events/rec route.
		r.Group(func(r chi.Router) {
			r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.HandleFunc("/events/rec", proxyHandler.ProxyToPlayer)
		})

		// Player service routes (protected)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.HandleFunc("/users/*", proxyHandler.ProxyToPlayer)
		})

		// Rooms service routes (protected)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.HandleFunc("/rooms/*", proxyHandler.ProxyToRooms)
			r.HandleFunc("/game/*", proxyHandler.ProxyToRooms)
		})

		// Themes service routes
		r.Route("/themes", func(r chi.Router) {
			// Public routes (with optional auth handled by themes service)
			r.Get("/", proxyHandler.ProxyToThemes)
			r.Get("/{id}", proxyHandler.ProxyToThemes)
			// Video/audio proxy (public)
			r.Get("/video/{basename}", proxyHandler.ProxyToThemes)
			r.Get("/audio/{basename}", proxyHandler.ProxyToThemes)

			// Protected routes (rate themes)
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Post("/{id}/rate", proxyHandler.ProxyToThemes)
				r.Delete("/{id}/rate", proxyHandler.ProxyToThemes)
				r.Get("/my-ratings", proxyHandler.ProxyToThemes)
			})

			// Admin routes (sync)
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Use(AdminRoleMiddleware)
				r.Post("/admin/sync", proxyHandler.ProxyToThemes)
				r.Get("/admin/sync/status", proxyHandler.ProxyToThemes)
			})
		})

		// Notifications service routes (workstream notifications, v1.0 Phase 1).
		// All routes JWT-required (user-scoped CRUD). The internal producer
		// endpoint (/internal/notifications) is intentionally NOT registered
		// here — it is reachable only from inside the Docker network
		// (notifications:8090), enforced by gateway-non-routing (D-05).
		// Literal sub-paths (mark-all-read, unread-count) registered BEFORE
		// param routes ({id}/...) to avoid chi precedence shadowing.
		r.Route("/notifications", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Get("/", proxyHandler.ProxyToNotifications)
				r.Get("/unread-count", proxyHandler.ProxyToNotifications)
				r.Post("/mark-all-read", proxyHandler.ProxyToNotifications)
				r.Post("/{id}/read", proxyHandler.ProxyToNotifications)
				r.Post("/{id}/dismiss", proxyHandler.ProxyToNotifications)
				r.Post("/{id}/click", proxyHandler.ProxyToNotifications)
			})
		})

		// Library service routes (workstream raw-jp / v0.2). Phase 2 adds
		// /search behind admin auth; /health remains public so the docker
		// healthcheck + ops probes still work without credentials. All other
		// /api/library/* paths are admin-gated as forward-compat for Phases
		// 3-5 (jobs, episodes, ingest endpoints).
		r.Route("/library", func(r chi.Router) {
			r.Get("/health", proxyHandler.ProxyToLibrary) // public
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Use(AdminRoleMiddleware)
				r.HandleFunc("/*", proxyHandler.ProxyToLibrary)
			})
		})

		// Streaming service routes - most are public, only admin needs auth
		r.Route("/streaming", func(r chi.Router) {
			// Public routes (no auth required)
			r.Get("/hls-proxy", proxyHandler.ProxyToStreaming)
			r.Options("/hls-proxy", proxyHandler.ProxyToStreaming) // CORS preflight
			r.Get("/proxy-status", proxyHandler.ProxyToStreaming)
			r.Get("/image-proxy", proxyHandler.ProxyToStreaming)
			r.HandleFunc("/stream/*", proxyHandler.ProxyToStreaming)
			r.HandleFunc("/internal/*", proxyHandler.ProxyToStreaming)

			// Admin routes (protected)
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.HandleFunc("/admin/*", proxyHandler.ProxyToStreaming)
			})
		})
	})

	return r, rateLimiter.Stop
}

// apiKeyHTTPClient is a shared HTTP client for API key resolution calls.
var (
	apiKeyHTTPClient     *http.Client
	apiKeyHTTPClientOnce sync.Once
)

func getApiKeyHTTPClient() *http.Client {
	apiKeyHTTPClientOnce.Do(func() {
		apiKeyHTTPClient = &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   2 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        5,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	})
	return apiKeyHTTPClient
}

// JWTValidationMiddleware validates JWT tokens and resolves API keys (ak_ prefix).
func JWTValidationMiddleware(jwtConfig authz.JWTConfig, authServiceURL string) func(http.Handler) http.Handler {
	jwtManager := authz.NewJWTManager(jwtConfig)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token == "" {
				httputil.Unauthorized(w)
				return
			}

			var claims *authz.Claims
			var err error

			if strings.HasPrefix(token, "ak_") {
				// Resolve API key via auth service internal endpoint
				resolved, resolveErr := resolveApiKey(authServiceURL, token)
				if resolveErr != nil {
					httputil.Unauthorized(w)
					return
				}
				// WV3-T1: derive a deterministic SessionID from
				// (userID, raw ak_*, UTC-day) so audit logs can correlate
				// ak_* calls and a future per-session revocation middleware
				// has something to act on. See apikey_session.go for the
				// derivation contract.
				sid := deriveAPIKeySessionID(resolved.UserID, token, time.Now())
				// Mint a short-lived JWT for downstream services
				tokenPair, mintErr := jwtManager.GenerateTokenPair(resolved.UserID, resolved.Username, resolved.Role, sid)
				if mintErr != nil {
					httputil.Unauthorized(w)
					return
				}
				// Replace header so downstream services see a valid JWT
				r.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
				resolved.SessionID = sid
				claims = resolved
			} else {
				// Standard JWT validation
				claims, err = jwtManager.ValidateAccessToken(token)
				if err != nil {
					httputil.Unauthorized(w)
					return
				}
			}

			ctx := authz.ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalJWTValidationMiddleware validates JWT or resolves API keys (ak_ prefix) when
// an Authorization header is present, but allows anonymous traffic (no header) to pass
// through unchanged. Used for endpoints that have public + personalized branches based
// on auth presence (e.g. /api/users/recs — anonymous gets the trending row, logged-in
// gets the personalized "Up Next for you" row).
//
// Behaviour:
//   - No Authorization header → pass through unchanged (downstream sees no claims)
//   - "Bearer ak_…" → resolve via auth service, mint short-lived JWT, replace header
//   - "Bearer <jwt>" → validate; on failure pass through unchanged so downstream's
//     OptionalAuth middleware also sees no claims (defense in depth — never serve
//     personalized data on a forged token)
func OptionalJWTValidationMiddleware(jwtConfig authz.JWTConfig, authServiceURL string) func(http.Handler) http.Handler {
	jwtManager := authz.NewJWTManager(jwtConfig)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token == "" {
				// No token → anonymous flow, pass through unchanged.
				next.ServeHTTP(w, r)
				return
			}

			if strings.HasPrefix(token, "ak_") {
				resolved, resolveErr := resolveApiKey(authServiceURL, token)
				if resolveErr != nil {
					// Bad/expired API key on an optional-auth route → degrade to
					// anonymous rather than 401, matching the route's contract.
					r.Header.Del("Authorization")
					next.ServeHTTP(w, r)
					return
				}
				// WV3-T1: derive deterministic SessionID — same contract as
				// JWTValidationMiddleware above; see apikey_session.go.
				sid := deriveAPIKeySessionID(resolved.UserID, token, time.Now())
				tokenPair, mintErr := jwtManager.GenerateTokenPair(resolved.UserID, resolved.Username, resolved.Role, sid)
				if mintErr != nil {
					r.Header.Del("Authorization")
					next.ServeHTTP(w, r)
					return
				}
				r.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
				resolved.SessionID = sid
				ctx := authz.ContextWithClaims(r.Context(), resolved)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Standard JWT validation — on failure, strip header and degrade to anonymous.
			claims, err := jwtManager.ValidateAccessToken(token)
			if err != nil {
				r.Header.Del("Authorization")
				next.ServeHTTP(w, r)
				return
			}
			ctx := authz.ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// resolveApiKey calls the auth service's internal endpoint to validate an API key.
func resolveApiKey(authServiceURL, apiKey string) (*authz.Claims, error) {
	body, err := json.Marshal(map[string]string{"api_key": apiKey})
	if err != nil {
		return nil, err
	}

	resp, err := getApiKeyHTTPClient().Post(
		authServiceURL+"/internal/resolve-api-key",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("auth service returned %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			UserID   string    `json:"user_id"`
			Username string    `json:"username"`
			Role     authz.Role `json:"role"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &authz.Claims{
		UserID:   result.Data.UserID,
		Username: result.Data.Username,
		Role:     result.Data.Role,
	}, nil
}

// ipLimiter wraps a rate limiter with a last-seen timestamp for cleanup.
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter manages per-IP rate limiters.
type IPRateLimiter struct {
	limiters sync.Map
	rps      rate.Limit
	burst    int
	stopCh   chan struct{}
}

// NewIPRateLimiter creates a per-IP rate limiter and starts a background
// cleanup goroutine that evicts entries not seen for 5 minutes.
func NewIPRateLimiter(rps rate.Limit, burst int) *IPRateLimiter {
	rl := &IPRateLimiter{
		rps:    rps,
		burst:  burst,
		stopCh: make(chan struct{}),
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rl.limiters.Range(func(key, value any) bool {
					entry := value.(*ipLimiter)
					if time.Since(entry.lastSeen) > 5*time.Minute {
						rl.limiters.Delete(key)
					}
					return true
				})
			case <-rl.stopCh:
				return
			}
		}
	}()

	return rl
}

// Stop terminates the background cleanup goroutine.
func (rl *IPRateLimiter) Stop() {
	close(rl.stopCh)
}

func (rl *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	now := time.Now()
	if v, ok := rl.limiters.Load(ip); ok {
		entry := v.(*ipLimiter)
		entry.lastSeen = now
		return entry.limiter
	}
	limiter := rate.NewLimiter(rl.rps, rl.burst)
	rl.limiters.Store(ip, &ipLimiter{limiter: limiter, lastSeen: now})
	return limiter
}

// RateLimitMiddleware implements per-IP rate limiting.
//
// Returns the middleware factory. In tests that want to clean up the
// background eviction goroutine, use RateLimitMiddlewareWithStop which
// also returns the *IPRateLimiter so callers can register `Stop` on
// t.Cleanup. REVIEW.md WR-04 — without that, every NewRouter() in a
// gateway test leaks one goroutine for the duration of the test binary.
func RateLimitMiddleware(cfg config.RateLimitConfig) func(http.Handler) http.Handler {
	mw, _ := RateLimitMiddlewareWithStop(cfg)
	return mw
}

// RateLimitMiddlewareWithStop is the test-friendly variant of
// RateLimitMiddleware. Returns the middleware factory PLUS the underlying
// IPRateLimiter so callers can `t.Cleanup(rl.Stop)` and not leak the
// background eviction goroutine. Production callers can ignore the
// second return value (the goroutine lives as long as the process).
func RateLimitMiddlewareWithStop(cfg config.RateLimitConfig) (func(http.Handler) http.Handler, *IPRateLimiter) {
	rl := NewIPRateLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.BurstSize)

	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			if !rl.getLimiter(ip).Allow() {
				httputil.TooManyRequests(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	return mw, rl
}

// MaxBodySizeMiddleware limits the size of incoming request bodies.
func MaxBodySizeMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// AdminRoleMiddleware ensures the user has admin role
func AdminRoleMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authz.IsAdmin(r.Context()) {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
