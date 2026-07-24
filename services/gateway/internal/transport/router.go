package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
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

	// Policy ruleset cache — polls policy-service's Docker-network-only feed and
	// backs the FeatureGate middleware. Fail-static; cold start uses per-flag
	// failSafe. The returned cleanup cancels the refresher.
	//
	// rulesetRefreshInterval guards against the zero-value cfg.RulesetRefresh
	// that older test callers' bare config.Config{} literals produce (they
	// pre-date this field) — time.NewTicker panics on a non-positive
	// duration, so config.Load()'s 15s default is re-applied here as a
	// belt-and-suspenders fallback.
	rulesetRefreshInterval := cfg.RulesetRefresh
	if rulesetRefreshInterval <= 0 {
		rulesetRefreshInterval = 15 * time.Second
	}
	rulesetCtx, rulesetCancel := context.WithCancel(context.Background())
	featureRuleset := newRulesetCache(
		httpRulesetFetch(cfg.Services.PolicyService, &http.Client{Timeout: 5 * time.Second}),
		log,
	)
	featureRuleset.Start(rulesetCtx, rulesetRefreshInterval)

	// Zundamon voice synthesis is a low-priority workload. Reuse the gateway's
	// Redis client to watch the governor signal, then shed the public facade at
	// Elevated and Critical before any CPU work reaches VOICEVOX.
	degradationWatcher := cache.NewDegradationWatcherFromClient(redisClient, 2*time.Second)
	degradationWatcher.Start(rulesetCtx)
	var zundamonHandler *handler.ZundamonHandler
	if cfg.Services.VoicevoxService != "" {
		var err error
		zundamonHandler, err = handler.NewZundamonHandler(cfg.Services.VoicevoxService, degradationWatcher, log)
		if err != nil {
			log.Fatalw("failed to build Zundamon VOICEVOX facade", "error", err, "target", cfg.Services.VoicevoxService)
		}
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS(cfg.CORSOrigins))
	r.Use(httputil.SecurityHeaders)
	// Trust ONLY the edge-set X-Real-IP for the client address, not chi's
	// middleware.RealIP (which also honors the client-spoofable True-Client-IP
	// and first X-Forwarded-For entry). See RealClientIP.
	r.Use(RealClientIP)
	// Anti-scrape tarpit: feed fake-but-valid JSON to configured scraper IPs
	// (POISON_CLIENT_IPS). MUST run AFTER RealIP so r.RemoteAddr is the true
	// client IP. No-op when the list is empty.
	r.Use(PoisonMiddleware(cfg.PoisonClientIPs, log))
	// 10MB global cap; the worker segment data-plane is exempt (multi-MB video
	// segment uploads, capability-signed + nginx-capped at 512m + streamed to MinIO).
	r.Use(MaxBodySizeMiddleware(10*1024*1024, "/worker/segments/"))
	rateLimitMW, rateLimiter := RateLimitMiddlewareWithStop(cfg.RateLimit)
	r.Use(rateLimitMW)

	// WV3-T3: per-authenticated-user rate limit (GCRA / redis_rate). Built
	// ONCE here and applied inside every protected route group AFTER its
	// JWT middleware so the bucket key is user_id (not IP). When the
	// caller passes a nil Redis client (older NewRouter signature, or
	// tests that don't care), the user limiter degrades to a pass-through
	// — same effect as having no extra middleware at all.
	userRateLimit := newUserRateLimitChainFn(redisClient, cfg.RateLimit.UserPerMinute, cfg.RateLimit.UserBurst, log)

	// Workstream watch-together v1.0 Phase 1 (Plan 01.7) — dedicated WS
	// reverse proxy for /api/watch-together/ws. The standard ProxyService
	// path strips RFC 7230 §6.1 hop-by-hop headers (Upgrade, Connection),
	// which is correct for HTTP but breaks the WS handshake — see
	// ws_proxy.go for the rationale. Built at router-construction time
	// so a misconfigured target URL fails fast at startup, not on first
	// upgrade attempt.
	//
	// When WatchTogetherService is unset (legacy tests built before
	// workstream watch-together shipped — they construct a minimal
	// ServiceURLs with only the fields they care about), we install a
	// 502 stub instead of fataling. Production startup ALWAYS has a
	// value because config.Load() defaults it to "http://watch-together:8091".
	var wtWSProxy http.HandlerFunc
	if cfg.Services.WatchTogetherService == "" {
		wtWSProxy = func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "watch-together service not configured", http.StatusBadGateway)
		}
	} else {
		built, err := newWSProxy(cfg.Services.WatchTogetherService, log)
		if err != nil {
			log.Fatalw("failed to build watch-together ws proxy", "error", err, "target", cfg.Services.WatchTogetherService)
		}
		wtWSProxy = built
	}

	// Worker edge (ext.animeenigma.org) — /worker/* proxied to upscaler:8096.
	// No JWT; gated by ExternalAPIKeyMiddleware (static shared secret). Real
	// per-worker auth is the enroll→session→capability chain (Tasks 5/10).
	//
	// When UpscalerService is unset (tests that pre-date the upscaler), we
	// install a 502 stub instead of fataling. Production startup ALWAYS has a
	// value because config.Load() defaults it to "http://upscaler:8096".
	var (
		workerProxyHandler *handler.ExternalAPIHandler
		workerWSProxy      http.HandlerFunc
	)
	if cfg.Services.UpscalerService == "" {
		stub502 := func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"error":"upscaler not configured"}`, http.StatusBadGateway)
		}
		workerWSProxy = stub502
		// workerProxyHandler stays nil — handled below in route registration.
	} else {
		builtHandler, err := handler.NewExternalAPIHandler(cfg.Services.UpscalerService, log)
		if err != nil {
			log.Fatalw("failed to build worker proxy handler", "error", err, "target", cfg.Services.UpscalerService)
		}
		workerProxyHandler = builtHandler

		builtWS, err := handler.NewWorkerWSProxy(cfg.Services.UpscalerService, log)
		if err != nil {
			log.Fatalw("failed to build worker ws proxy", "error", err, "target", cfg.Services.UpscalerService)
		}
		workerWSProxy = builtWS
	}

	// workerEnrollRateLimit is a dedicated per-IP limiter for /worker/enroll
	// and /worker/ws. /worker/segments/* is intentionally EXCLUDED from this
	// limiter — segment uploads are large (hundreds of MB) and take long enough
	// that the token bucket would false-trip and 429 legitimate transfers (CD-12).
	// The global per-IP limiter (rateLimitMW, applied above) still applies to ALL
	// paths including segments as a coarse backstop.
	workerEnrollRL := NewIPRateLimiter(rate.Limit(cfg.RateLimit.RequestsPerSecond), cfg.RateLimit.BurstSize)
	workerEnrollRateLimitMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			if !workerEnrollRL.getLimiter(ip).Allow() {
				httputil.TooManyRequests(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	// Stop the worker enroll rate limiter's background goroutine when the
	// router is torn down (mirrors the global rateLimiter.Stop pattern).
	// Also cancels the policy ruleset cache's background refresher.
	origCleanup := rateLimiter.Stop
	combinedCleanup := func() {
		origCleanup()
		workerEnrollRL.Stop()
		rulesetCancel()
	}

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

	// Worker edge (ext.animeenigma.org → gateway → upscaler:8096).
	// Mounted at /worker (NOT under /api, NOT under any JWT/admin group).
	// Authentication: static X-API-Key (ExternalAPIKeyMiddleware, fail-closed).
	// Per-IP rate limit: applied to /worker/enroll + /worker/ws ONLY.
	//   /worker/segments/* and /worker/models/* are EXCLUDED from the per-path
	//   rate limiter — large binary transfers would false-trip the token bucket
	//   (CD-12; see handler/external_api.go). The global per-IP limiter
	//   (rateLimitMW at the top of this router) still covers all paths.
	// /worker/models/* (T27): streams model .tar artifacts to workers that hold
	//   a valid name-bound HMAC capability handle (T25 contract). No extra
	//   gateway-side auth beyond the API-key gate; the upscaler enforces the
	//   capability handle. Uses the same streaming proxy (FlushInterval=-1).
	r.Route("/worker", func(r chi.Router) {
		// API-key gate applies to the whole /worker group.
		r.Use(ExternalAPIKeyMiddleware(cfg.ExternalAPIKey))

		// /worker/enroll — small JSON registration. Rate-limited per-IP.
		r.With(workerEnrollRateLimitMW).HandleFunc("/enroll", func(w http.ResponseWriter, r *http.Request) {
			if workerProxyHandler != nil {
				workerProxyHandler.ProxyWorker(w, r)
			} else {
				http.Error(w, `{"error":"bad_gateway"}`, http.StatusBadGateway)
			}
		})

		// /worker/ws — WebSocket upgrade. Rate-limited per-IP. Dedicated WS
		// reverse proxy preserves Upgrade/Connection hop-by-hop headers that
		// ProxyService.Forward would strip (same rationale as watch-together ws).
		r.With(workerEnrollRateLimitMW).Get("/ws", workerWSProxy)

		// /worker/segments/* — large binary segment bytes. NO per-path rate
		// limit (see comment above). Streams without full-body buffering
		// (FlushInterval=-1 in the ExternalAPIHandler director).
		r.With(largeTransferDeadlineMiddleware).HandleFunc("/segments/*", func(w http.ResponseWriter, r *http.Request) {
			if workerProxyHandler != nil {
				workerProxyHandler.ProxyWorker(w, r)
			} else {
				http.Error(w, `{"error":"bad_gateway"}`, http.StatusBadGateway)
			}
		})

		// /worker/models/* (T27) — model artifact download for workers.
		// NO per-path rate limit (same reasoning as /worker/segments/* — large
		// binary bodies). The upscaler enforces the name-bound HMAC capability
		// handle; the gateway adds only the API-key gate. Streaming proxy:
		// FlushInterval=-1 ensures model .tar bytes flow without gateway OOM.
		// X-Gateway-Internal is stripped by the ExternalAPIHandler director
		// (defence-in-depth; same as /worker/segments/*).
		r.With(largeTransferDeadlineMiddleware).HandleFunc("/models/*", func(w http.ResponseWriter, r *http.Request) {
			if workerProxyHandler != nil {
				workerProxyHandler.ProxyWorker(w, r)
			} else {
				http.Error(w, `{"error":"bad_gateway"}`, http.StatusBadGateway)
			}
		})
	})

	// Admin panel routes (protected by admin role, unless DevMode is enabled)
	r.Route("/admin", func(r chi.Router) {
		if !cfg.DevMode {
			// AdminSessionRefreshMiddleware runs FIRST: browser-driven admin
			// tools (Grafana etc.) run outside the Vue SPA, so nothing renews
			// the ~1h access_token cookie. This transparently refreshes it
			// from the refresh_token cookie so admin sessions last as long as
			// the login, instead of 401ing once the access token expires.
			r.Use(AdminSessionRefreshMiddleware(cfg.JWT, cfg.Services.AuthService, log))
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			// NOTE: the per-user GCRA limiter (userRateLimit) is deliberately
			// NOT applied here. A single Grafana/Prometheus page fires dozens
			// of sub-requests, which tripped the 60/min budget and produced
			// spurious 429s. /admin is already admin-gated (JWT + AdminRole,
			// single trusted user) and the global per-IP limiter still applies.
			r.Use(AdminRoleMiddleware)
		}

		// Admin dashboard landing page — now rendered by the Vue SPA
		// (AdminDashboard.vue) instead of hardcoded HTML, so it matches the
		// site design. Falls through to the web service exactly like /recs,
		// /feedback, /collections, etc. below. Auth (JWT + AdminRole +
		// session-refresh) is already applied by the surrounding /admin group.
		r.HandleFunc("/", proxyHandler.ProxyToWeb)

		r.HandleFunc("/grafana/*", proxyHandler.ProxyToGrafana)
		r.HandleFunc("/prometheus/*", proxyHandler.ProxyToPrometheus)

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

		// Admin feedback browser SPA route (/admin/feedback) — same
		// fall-through as /recs and /collections so AdminFeedback.vue renders.
		// Without it chi 404s the browser navigation before the SPA loads.
		r.HandleFunc("/feedback", proxyHandler.ProxyToWeb)
		r.HandleFunc("/feedback/*", proxyHandler.ProxyToWeb)

		// Raw-library admin SPA route (/admin/raw-library) — same fall-through
		// (was missing, so the page 404'd at the gateway).
		r.HandleFunc("/raw-library", proxyHandler.ProxyToWeb)
		r.HandleFunc("/raw-library/*", proxyHandler.ProxyToWeb)

		// Gacha (Лудка) admin SPA routes (/admin/gacha — cards/groups/banners
		// management pages, Phase 5 UI) — same fall-through as /recs,
		// /collections, /feedback above. Auth is already enforced by the
		// surrounding /admin group's JWT + AdminRoleMiddleware.
		r.HandleFunc("/gacha", proxyHandler.ProxyToWeb)
		r.HandleFunc("/gacha/*", proxyHandler.ProxyToWeb)

		// Policy admin SPA route (/admin/policy — RBAC and roulette, Phase 3's
		// AdminPolicy.vue) — same fall-through as /recs, /collections,
		// /feedback, /gacha above. Auth is already enforced by the
		// surrounding /admin group's JWT + AdminRoleMiddleware.
		r.HandleFunc("/policy", proxyHandler.ProxyToWeb)
		r.HandleFunc("/policy/*", proxyHandler.ProxyToWeb)
	})

	// Fanfic engine SPA route (/fanfics — admin-gated in the router meta;
	// dark-shipped via VITE_FANFIC_ADMIN_ONLY on the web). Unlike the
	// /admin/gacha etc. pages above, this is a top-level (non-/admin) page
	// so it is NOT wrapped in JWT/AdminRole middleware here — the actual
	// /api/fanfic/* calls the page makes are gated server-side by the
	// route group below (JWT + BlockGuestRole + conditional AdminRole).
	r.HandleFunc("/fanfics", proxyHandler.ProxyToWeb)

	// Magic-link SSO bridge routes (public, no JWT). These endpoints return
	// 302 redirects for cross-domain (.ru → .org) login handoff; the gateway
	// must relay the redirect verbatim to the browser (not chase it) so the
	// Location header and any Set-Cookie reach the client unchanged.
	// Registered at the gateway root (not under /api) so the redirect target
	// URL — which lives on the .org domain — can differ from the gateway's
	// own /api prefix.
	r.Get("/magic-link-generate", proxyHandler.ProxyToAuthNoRedirect)
	r.Get("/magic-link-login", proxyHandler.ProxyToAuthNoRedirect)

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Track B5: advertise the rotating masked analytics base on every
		// /api response (see handler/masked_analytics.go).
		r.Use(handler.MaskedPathHintMiddleware([]byte(cfg.JWT.Secret)))

		// Public, narrowly bounded Zundamon facade. The raw VOICEVOX API stays
		// Docker-network-only; this surface filters to the exact ずんだもん
		// speaker, serializes synthesis, and observes governor load shedding.
		if zundamonHandler == nil {
			unavailable := func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"error":{"code":"unavailable","message":"VOICEVOX is not configured"}}`, http.StatusServiceUnavailable)
			}
			r.Get("/zundamon/status", unavailable)
			r.Post("/zundamon/synthesis", unavailable)
		} else {
			r.Get("/zundamon/status", zundamonHandler.Status)
			r.Post("/zundamon/synthesis", zundamonHandler.Synthesize)
		}

		// Auth service routes (public)
		r.HandleFunc("/auth/*", proxyHandler.ProxyToAuth)

		// Phase 11 / UX-24 — System status banner (public, no JWT).
		// Sourced from gateway env (SYSTEM_BANNER_ACTIVE +
		// SYSTEM_BANNER_MESSAGE). The existing CORS / rate-limit /
		// security-headers stack at the top of NewRouter already applies.
		sysStatusHandler := handler.NewSystemStatusHandler(cfg)
		r.Get("/system/status", sysStatusHandler.GetStatus)

		// Clickstream ingestion (Plan 1). PUBLIC — anonymous visitors tracked.
		// Per-IP rate limiting already applies to all /api/* paths. Only
		// /collect is exposed; /internal/erase is Docker-network-only.
		r.Post("/analytics/collect", proxyHandler.ProxyToAnalytics)
		// FE error log sink (log-only, no DB). PUBLIC — same trust model as
		// /collect; per-IP rate limiting already applies to all /api/* paths.
		r.Post("/analytics/client-errors", proxyHandler.ProxyToAnalytics)
		// Player telemetry beacon (resolve/stall outcomes). PUBLIC — anonymous,
		// same trust model as /collect; per-IP rate limiting already applies.
		r.Post("/analytics/player-events", proxyHandler.ProxyToAnalytics)

		// Track B5: rotating masked ingestion alias. Param route — chi
		// prefers static siblings, so every existing /api/<service> route
		// wins; only otherwise-unmatched two-segment POSTs land here, and the
		// handler 404s anything without a valid HMAC bucket segment.
		maskedAnalytics := handler.NewMaskedAnalyticsHandler(proxyHandler, []byte(cfg.JWT.Secret))
		r.Post("/{maskedSeg}/{maskedEp}", maskedAnalytics.Handle)

		// Player service routes - reviews (must be before /anime/* catch-all)
		r.Post("/anime/ratings/batch", proxyHandler.ProxyToPlayer)
		r.Get("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
		r.Get("/anime/{animeId}/reviews/me", proxyHandler.ProxyToPlayer)
		r.Post("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
		r.Delete("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
		r.Get("/anime/{animeId}/rating", proxyHandler.ProxyToPlayer)
		// AUTO-408 — toggle an emoji reaction on a review. Auth-gated like
		// comment mutations (JWT validation + per-user rate limit + guest
		// block); the player applies AuthMiddleware again downstream. Must be
		// registered BEFORE the /anime/* → catalog catch-all below.
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(BlockGuestRoleMiddleware)
			r.Post("/anime/{animeId}/reviews/{reviewId}/reactions/{emoji}", proxyHandler.ProxyToPlayer)
			// AUTO-408 — admin moderation: remove a specific user's reaction.
			// The player enforces the admin role downstream (AdminRoleMiddleware
			// + handler re-check); the gateway gate here is JWT-validity only,
			// same as the toggle route above.
			r.Delete("/anime/{animeId}/reviews/{reviewId}/reactions/{emoji}/users/{userId}", proxyHandler.ProxyToPlayer)
		})

		// Phase 14 (ui-ux-audit / UX-28) — soft social-proof follower count
		// proxied to player. Public, no JWT required. Must be registered BEFORE
		// the generic /anime/* → catalog catch-all below; otherwise chi would
		// route this path to the catalog service.
		r.Get("/anime/{animeId}/watchers-count", proxyHandler.ProxyToPlayer)

		// Aggregate anime-page context (page-fetch optimization 2026-06-11).
		// Optional-auth: the proxy forwards Authorization as-is and the player
		// decodes it downstream (OptionalAuthMiddleware), so anonymous and
		// authenticated callers both pass through without a gateway JWT gate.
		// Must be registered BEFORE the /anime/* → catalog catch-all below.
		r.Get("/anime/{animeId}/viewer-context", proxyHandler.ProxyToPlayer)

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
			r.Use(BlockGuestRoleMiddleware)
			r.Post("/anime/{animeId}/comments", proxyHandler.ProxyToPlayer)
			r.Patch("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
			r.Delete("/anime/{animeId}/comments/{commentId}", proxyHandler.ProxyToPlayer)
		})

		// Catalog service routes (public)
		r.HandleFunc("/anime", proxyHandler.ProxyToCatalog)
		// Scraper JSON routes (episodes/servers/stream/health) need the longer
		// scraperJSONClient timeout (cold engine=browser provider discovery can
		// exceed the plain client's 15s even on a healthy provider) — MUST be
		// registered BEFORE the generic /anime/* below (same "specific-before-
		// general" gotcha as /admin/scraper/* vs /admin/* further down).
		r.HandleFunc("/anime/{id}/scraper/*", proxyHandler.ProxyToCatalogScraperJSON)
		r.HandleFunc("/anime/_/scraper/health", proxyHandler.ProxyToCatalogScraperJSON)
		r.HandleFunc("/anime/*", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/genres", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/studios", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/kodik/*", proxyHandler.ProxyToCatalog)
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
		r.HandleFunc("/characters", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/characters/*", proxyHandler.ProxyToCatalog)

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
			// v4 B-1 «Ещё разок» — fresh random_tail pick, public.
			r.HandleFunc("/home/spotlight/reroll", proxyHandler.ProxyToCatalog)
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

		// Admin feedback browser routes — proxied to the PLAYER service (the
		// report archive lives there). MUST be registered BEFORE the generic
		// /admin/* → catalog group below (same "specific-before-general" gotcha
		// as /admin/scraper/*). Both the bare list path and the wildcard are
		// registered so `/api/admin/reports` and `/api/admin/reports/{id}...`
		// both reach player. Player applies the same JWT + admin gates again.
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(AdminRoleMiddleware)
			r.HandleFunc("/admin/reports", proxyHandler.ProxyToPlayer)
			r.HandleFunc("/admin/reports/*", proxyHandler.ProxyToPlayer)
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
			r.HandleFunc("/users/recs", proxyHandler.ProxyToRecs)
			r.HandleFunc("/users/recs/", proxyHandler.ProxyToRecs)
			r.HandleFunc("/users/recs/*", proxyHandler.ProxyToRecs)
		})

		// Phase 14 (REC-ADMIN-01 / REC-ADMIN-02): admin debug + force-recompute
		// routes proxied to the recs service. JWT validation + admin role
		// gate at the gateway layer (defense-in-depth — recs applies the same
		// gates again in services/recs/internal/transport/router.go).
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(AdminRoleMiddleware)
			r.HandleFunc("/admin/recs/*", proxyHandler.ProxyToRecs)
		})

		// Phase 14 (REC-EVAL-01): public telemetry endpoint. JWT-OPTIONAL so
		// anonymous CTR data flows from the trending row. Recs applies its
		// own OptionalAuthMiddleware on the inner /events/rec route.
		r.Group(func(r chi.Router) {
			r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.HandleFunc("/events/rec", proxyHandler.ProxyToRecs)
		})

		// Anidle guessing game (spec 2026-06-15) — guest-friendly, JWT optional.
		r.Group(func(r chi.Router) {
			r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.HandleFunc("/anidle/*", proxyHandler.ProxyToAnidle)
		})

		// policy-service (RBAC and roulette, Phase 1 Task 6). Per-user
		// visibility feed is JWT-OPTIONAL; admin CRUD is JWT + admin
		// (defense-in-depth — policy re-applies both gates server-side).
		// This is PROXY-ONLY routing: gateway-side ENFORCEMENT of the
		// ruleset against other services' routes (the FeatureGate
		// middleware) lands in Phase 2. /internal/policy/ruleset is
		// Docker-network-only and is intentionally never registered here.
		r.Group(func(r chi.Router) {
			r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.HandleFunc("/policy/features/mine", proxyHandler.ProxyToPolicy)
		})
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(AdminRoleMiddleware)
			r.HandleFunc("/admin/policy/*", proxyHandler.ProxyToPolicy)
		})

		// Task 2 (RBAC and roulette) — admin-only user management subtree,
		// proxied to AUTH (not catalog): canonical user-resolve, the user
		// list, and per-user role management. Turns a UUID, username,
		// public_id, or telegram_id into the canonical user record; consumed
		// by other admin surfaces (recs picker, policy admin UI). Mirrors the
		// /admin/policy/* group immediately above (same JWT + userRateLimit +
		// AdminRoleMiddleware chain). CRITICAL ORDER: this is a static
		// "/admin/users..." path, so chi's route tree resolves it against the
		// generic "/api/admin/*" -> catalog catch-all group above the SAME way
		// "/admin/policy/*" already does (see TestRouter_Policy_AdminFlags_AdminJWT_ProxiesToPolicy
		// for the sibling proof, and TestRouter_AdminUsersResolve_AdminJWT_ProxiesToAuth,
		// TestRouter_AdminUsersList_AdminJWT_ProxiesToAuth, and
		// TestRouter_AdminUsersRole_AdminJWT_ProxiesToAuth in
		// router_resolve_test.go for this route) — a more specific static
		// segment always wins over a shallower wildcard regardless of
		// registration order, but this group is still placed next to its
		// closest analog for readability.
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(AdminRoleMiddleware)
			// List + role management + resolve all live in auth. The bare path
			// and the wildcard are both registered (chi's /* does not match the
			// slash-less bare path). A more-specific static/param subtree still
			// wins over the generic /admin/* -> catalog group.
			r.HandleFunc("/admin/users", proxyHandler.ProxyToAuth)
			r.HandleFunc("/admin/users/*", proxyHandler.ProxyToAuth)
		})

		// Profile showcase ("стена") — runtime-gated by the policy ruleset
		// (flag "profile-wall"). OptionalJWT so the flag's eventual "everyone"
		// audience is public (anonymous); the seed roles:[admin] reproduces the
		// prior admin-only dark-ship. The player enforces owner-only writes
		// from JWT claims downstream (defense-in-depth). Registered BEFORE the
		// protected /users/* group so chi matches these specific routes first.
		r.Group(func(r chi.Router) {
			r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(FeatureGate("profile-wall", featureRuleset))
			r.Get("/users/{userId}/showcase", proxyHandler.ProxyToPlayer)
			r.Put("/users/me/showcase", proxyHandler.ProxyToPlayer)
			r.Get("/users/{userId}/compatibility", proxyHandler.ProxyToPlayer)
		})

		// Player service routes (protected)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(BlockGuestRoleMiddleware)
			r.HandleFunc("/users/*", proxyHandler.ProxyToPlayer)
		})

		// Rooms service routes (protected)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(userRateLimit)
			r.Use(BlockGuestRoleMiddleware)
			r.HandleFunc("/rooms/*", proxyHandler.ProxyToRooms)
			// /game/* is the path family the SPA actually calls (gameApi in
			// frontend/web/src/api/client.ts → /api/game/rooms/*). Both /rooms
			// and /game route to the rooms service; the path-rewrite in
			// proxy.go (case "rooms") maps both onto the service's /api/v1/rooms
			// mount (audit finding L753).
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
				r.Use(BlockGuestRoleMiddleware)
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
				r.Use(BlockGuestRoleMiddleware)
				r.Get("/", proxyHandler.ProxyToNotifications)
				r.Get("/unread-count", proxyHandler.ProxyToNotifications)
				r.Post("/mark-all-read", proxyHandler.ProxyToNotifications)
				r.Post("/{id}/read", proxyHandler.ProxyToNotifications)
				r.Post("/{id}/dismiss", proxyHandler.ProxyToNotifications)
				r.Post("/{id}/delete", proxyHandler.ProxyToNotifications)
				r.Post("/{id}/click", proxyHandler.ProxyToNotifications)
			})
		})

		// Watch-together service routes (workstream watch-together, v1.0
		// Phase 1 — see .planning/workstreams/watch-together/).
		//
		// SPLIT auth handling (mirrors the watch-together service's own
		// router in services/watch-together/internal/transport/router.go):
		//
		//   - /ws    → registered OUTSIDE JWTValidationMiddleware. Browsers
		//              CAN'T set Authorization: Bearer on a WebSocket
		//              upgrade handshake (the Sec-WebSocket-* handshake is
		//              strict), so the watch-together service validates
		//              the JWT itself from a ?token= query param. The
		//              gateway forwards the upgrade transparently via the
		//              dedicated WS reverse proxy (see ws_proxy.go for
		//              why we can't reuse ProxyService.Forward).
		//   - /rooms → JWT-required REST CRUD (standard Bearer header).
		//
		// Internal forward-compat route /internal/watch-together/* is NOT
		// registered (WT-FOUND-08 — Docker-network-only, same D-05 model
		// as notifications).
		r.Route("/watch-together", func(r chi.Router) {
			// WS upgrade — no JWT middleware here (auth via ?token=).
			r.Get("/ws", wtWSProxy)
			// REST CRUD — JWT + per-user rate limit.
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Route("/rooms", func(r chi.Router) {
					r.Post("/", proxyHandler.ProxyToWatchTogether)
					r.Get("/{id}", proxyHandler.ProxyToWatchTogether)
					r.Delete("/{id}", proxyHandler.ProxyToWatchTogether)
				})
			})
		})

		// Gacha (Лудка) service routes — workstream gacha, Phase 1.
		// JWT-required (logged-in-only; guests blocked via BlockGuestRole).
		// FeatureGate("gacha", ...) resolves the effective audience from the
		// policy-service ruleset (RBAC and roulette Phase 2 Task 3) — this
		// superseded the static admin-only dark-ship config bool (spec §12,
		// removed in Task 4); flip it via the /admin/policy flags UI, not an
		// env var + restart.
		// The internal credit endpoint (/internal/gacha/credit) is NOT
		// registered here — Docker-network-only (D-05).
		r.Route("/gacha", func(r chi.Router) {
			// Public card/banner art (Phase 2). Browsers load these via <img>
			// (no JWT header possible) — unauthenticated by design: keys are
			// unguessable UUIDs and the content is anime character art. The
			// gacha service validates the key shape and serves with nosniff.
			r.Get("/images/*", proxyHandler.ProxyToGacha)

			// Admin content API (Phase 2) — ALWAYS admin-gated, independent
			// of the "gacha" feature flag's audience: these are admin tools,
			// full stop. The gacha service re-validates JWT+admin downstream.
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Use(AdminRoleMiddleware)
				r.HandleFunc("/admin/*", proxyHandler.ProxyToGacha)
			})

			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Use(BlockGuestRoleMiddleware)
				r.Use(FeatureGate("gacha", featureRuleset))
				r.Get("/wallet", proxyHandler.ProxyToGacha)

				// Player pull engine (Phase 3): active banners (+my pity),
				// the pull itself, and the collection album.
				r.Get("/banners", proxyHandler.ProxyToGacha)
				r.Post("/banners/{id}/pull", proxyHandler.ProxyToGacha)
				r.Get("/collection", proxyHandler.ProxyToGacha)

				// Daily streak claim (Phase 4).
				r.Post("/daily", proxyHandler.ProxyToGacha)
			})
		})

		// Fanfic engine (spec 2026-07-06). JWT-required, guest-blocked, and
		// gated by FeatureGate("fanfic", ...) — the policy-service ruleset
		// (RBAC and roulette Phase 2 Task 3), superseding the static
		// admin-only dark-ship config bool (removed in Task 4). Flip it via
		// the /admin/policy flags UI, not an env var + restart. The SSE
		// /generate route uses
		// the flushing stream proxy (proxyStreamFlush) so token deltas reach
		// the browser live.
		r.Route("/fanfic", func(r chi.Router) {
			// Public daily reader («Фанфик дня» spotlight click-through). No
			// JWT / feature gate: the spotlight card is served to everyone,
			// and the fanfic service applies its own OptionalAuth +
			// explicit-content gating on this route. The global per-IP
			// limiter covers anon traffic. Mirrors the /themes
			// public-routes-then-Group layout; the static /daily segment
			// wins over the gated /{id} below in chi's routing.
			r.Get("/daily", proxyHandler.ProxyToFanfic)

			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Use(BlockGuestRoleMiddleware)
				r.Use(FeatureGate("fanfic", featureRuleset))
				r.Post("/generate", proxyHandler.ProxyToFanficStream)
				r.Post("/{id}/continue", proxyHandler.ProxyToFanficStream)
				r.Get("/", proxyHandler.ProxyToFanfic)
				r.Get("/tags", proxyHandler.ProxyToFanfic)
				r.Get("/{id}", proxyHandler.ProxyToFanfic)
				r.Delete("/{id}", proxyHandler.ProxyToFanfic)
			})
		})

		// Upscaler service routes (admin-gated, port 8096). All /api/upscale/*
		// paths require JWT + admin role. Internal segment-handle endpoints are
		// Docker-network-only (D-05 security model).
		r.Route("/upscale", func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(AdminRoleMiddleware)
			r.HandleFunc("/*", proxyHandler.ProxyToUpscaler)
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
				r.Use(LibraryRoleMiddleware) // admin OR librarian
				r.HandleFunc("/*", proxyHandler.ProxyToLibrary)
			})
		})

		// Streaming service routes - most are public, only admin needs auth
		r.Route("/streaming", func(r chi.Router) {
			// Public routes (no auth required). The body-streaming endpoints
			// (hls-proxy GET, image-proxy, stream/*) go through the
			// no-total-timeout stream client so a long HLS/MP4 body isn't
			// truncated at 15s (audit finding L466). The OPTIONS preflight and
			// proxy-status return small JSON and stay on the API client.
			r.Get("/hls-proxy", proxyHandler.ProxyToStreamingBody)
			r.Options("/hls-proxy", proxyHandler.ProxyToStreaming) // CORS preflight
			// Track A opaque path tokens: /api/streaming/m/<token>/<leaf> →
			// streaming's /api/v1/m/... masked proxy (no url= query shape for
			// filter lists to match; spec 2026-07-10 §3). Body-streaming
			// client, same as hls-proxy.
			r.Get("/m/*", proxyHandler.ProxyToStreamingBody)
			r.Options("/m/*", proxyHandler.ProxyToStreaming)
			r.Get("/proxy-status", proxyHandler.ProxyToStreaming)
			r.Get("/image-proxy", proxyHandler.ProxyToStreamingBody)
			r.HandleFunc("/stream/*", proxyHandler.ProxyToStreamingBody)
			// NOTE: /internal/* is intentionally NOT proxied. Streaming's
			// /api/v1/internal/token is a service-to-service stream-token minter;
			// exposing it through the public gateway made it an unauthenticated
			// internet-reachable minter. Per the platform convention every
			// /internal/* endpoint is Docker-network-only — callers reach
			// streaming directly at http://streaming:8082/api/v1/internal/*.
			// Guarded by TestRouter_StreamingInternalNotProxied.

			// Admin routes (protected)
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.HandleFunc("/admin/*", proxyHandler.ProxyToStreaming)
			})
		})
	})

	return r, combinedCleanup
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

// sharedAPIKeyCache is the process-wide TTL cache for resolved ak_* API-key
// claims (audit finding L473). A single instance is shared across every
// JWT/Optional middleware so a hot key resolved by one protected group is
// reused by all of them. Backed by the real resolveApiKey + time.Now.
var sharedAPIKeyCache = newAPIKeyCache(resolveApiKey, nil)

// resolveApiKeyCached resolves an API key through the shared TTL cache, falling
// back to the auth-service POST on a miss/expiry. Both JWT middlewares use this
// instead of calling resolveApiKey directly, so hot ak_* keys skip the blocking
// per-request auth round trip.
func resolveApiKeyCached(authServiceURL, apiKey string) (*authz.Claims, error) {
	return sharedAPIKeyCache.resolveCached(authServiceURL, apiKey)
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
				// Resolve API key via auth service internal endpoint (cached:
				// hot keys skip the blocking per-request POST — audit L473).
				resolved, resolveErr := resolveApiKeyCached(authServiceURL, token)
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
				resolved, resolveErr := resolveApiKeyCached(authServiceURL, token)
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
			UserID   string     `json:"user_id"`
			Username string     `json:"username"`
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

// largeTransferDeadlineMiddleware lifts the gateway's global Read/Write timeouts
// for the worker binary data-plane (segment up/download, model fetch). Those
// stream multi-MB bodies over variable-speed worker links, so the whole-request
// 30s cap would 502 a slow upload (observed: a multi-MB segment PUT over a home
// connection). It replaces the cap with a generous per-request deadline (bounded,
// so it is not a slowloris hole), scoped to these routes ONLY — every other route
// keeps the protective global timeout. nginx also bounds the transfer (300s).
func largeTransferDeadlineMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		deadline := time.Now().Add(10 * time.Minute)
		_ = rc.SetReadDeadline(deadline)
		_ = rc.SetWriteDeadline(deadline)
		next.ServeHTTP(w, r)
	})
}

// MaxBodySizeMiddleware limits the size of incoming request bodies. Any request
// whose path has one of exemptPrefixes is passed through UNCAPPED — used for the
// worker segment data-plane (/worker/segments/*), where uploads are multi-MB
// video segments. Those are capability-signed (HMAC handle per segment), capped
// by nginx (client_max_body_size 512m), and streamed straight to MinIO, so the
// gateway's small global cap must not truncate them (a truncated body 502s).
func MaxBodySizeMiddleware(maxBytes int64, exemptPrefixes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range exemptPrefixes {
				if strings.HasPrefix(r.URL.Path, p) {
					next.ServeHTTP(w, r)
					return
				}
			}
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

// LibraryRoleMiddleware gates the raw-library surface (/api/library/*): admins
// plus the dedicated librarian role. Librarian grants ONLY this group — every
// other admin-gated group stays on AdminRoleMiddleware.
func LibraryRoleMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := authz.RoleFromContext(r.Context())
		if role != authz.RoleAdmin && role != authz.RoleLibrarian {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// BlockGuestRoleMiddleware rejects requests carrying a Watch Together guest
// JWT (role=guest) with 403. A guest token is a syntactically valid bearer
// token, so this is the gateway-side containment that keeps the ephemeral
// guest identity scoped to the Watch Together routes ONLY (guest join via
// invite link). It MUST run AFTER JWTValidationMiddleware (claims are read
// from the request context) and is applied inside every non-admin protected
// group EXCEPT the /api/watch-together group, where guests legitimately call
// GET /rooms/{id} + the WS. Admin-gated groups don't need it — AdminRoleMiddleware
// already 403s any non-admin (guests included). The watch-together SERVICE
// separately rejects guest POST /rooms (room creation) — see
// services/watch-together RoomHandler.Create.
func BlockGuestRoleMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authz.RoleFromContext(r.Context()) == authz.RoleGuest {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
