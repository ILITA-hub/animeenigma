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
	"golang.org/x/time/rate"
)

func NewRouter(
	proxyHandler *handler.ProxyHandler,
	cfg *config.Config,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
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
	r.Use(RateLimitMiddleware(cfg.RateLimit))

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
</body>
</html>`))
		})

		r.HandleFunc("/grafana/*", proxyHandler.ProxyToGrafana)
		r.HandleFunc("/prometheus/*", proxyHandler.ProxyToPrometheus)
		r.HandleFunc("/loki/*", proxyHandler.ProxyToLoki)
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Auth service routes (public)
		r.HandleFunc("/auth/*", proxyHandler.ProxyToAuth)

		// Player service routes - reviews (must be before /anime/* catch-all)
		r.Post("/anime/ratings/batch", proxyHandler.ProxyToPlayer)
		r.Get("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
		r.Get("/anime/{animeId}/reviews/me", proxyHandler.ProxyToPlayer)
		r.Post("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
		r.Delete("/anime/{animeId}/reviews", proxyHandler.ProxyToPlayer)
		r.Get("/anime/{animeId}/rating", proxyHandler.ProxyToPlayer)

		// Catalog service routes (public)
		r.HandleFunc("/anime", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/anime/*", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/genres", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/kodik/*", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/hianime/*", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/consumet/*", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/animelib/*", proxyHandler.ProxyToCatalog)

		// Admin routes (protected, proxied to catalog)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
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

		// Player service routes (protected)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.HandleFunc("/users/*", proxyHandler.ProxyToPlayer)
		})

		// Rooms service routes (protected)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
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
				r.Post("/{id}/rate", proxyHandler.ProxyToThemes)
				r.Delete("/{id}/rate", proxyHandler.ProxyToThemes)
				r.Get("/my-ratings", proxyHandler.ProxyToThemes)
			})

			// Admin routes (sync)
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(AdminRoleMiddleware)
				r.Post("/admin/sync", proxyHandler.ProxyToThemes)
				r.Get("/admin/sync/status", proxyHandler.ProxyToThemes)
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
				r.HandleFunc("/admin/*", proxyHandler.ProxyToStreaming)
			})
		})
	})

	return r
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
				// Mint a short-lived JWT for downstream services
				tokenPair, mintErr := jwtManager.GenerateTokenPair(resolved.UserID, resolved.Username, resolved.Role)
				if mintErr != nil {
					httputil.Unauthorized(w)
					return
				}
				// Replace header so downstream services see a valid JWT
				r.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
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
func RateLimitMiddleware(cfg config.RateLimitConfig) func(http.Handler) http.Handler {
	rl := NewIPRateLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.BurstSize)

	return func(next http.Handler) http.Handler {
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
