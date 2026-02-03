package transport

import (
	"net/http"

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
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)
	r.Use(RateLimitMiddleware(cfg.RateLimit))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// Metrics endpoint
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// OpenAPI spec
	r.Get("/openapi.json", proxyHandler.GetOpenAPISpec)

	// Admin panel routes (protected by admin role, unless DevMode is enabled)
	r.Route("/admin", func(r chi.Router) {
		if !cfg.DevMode {
			r.Use(JWTValidationMiddleware(cfg.JWT))
			r.Use(AdminRoleMiddleware)
		}

		// Admin dashboard landing page
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html>
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
</body>
</html>`))
		})

		r.HandleFunc("/grafana/*", proxyHandler.ProxyToGrafana)
		r.HandleFunc("/prometheus/*", proxyHandler.ProxyToPrometheus)
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Auth service routes (public)
		r.HandleFunc("/auth/*", proxyHandler.ProxyToAuth)

		// Catalog service routes (public)
		r.HandleFunc("/anime", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/anime/*", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/genres", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/kodik/*", proxyHandler.ProxyToCatalog)

		// Admin routes (protected, proxied to catalog)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT))
			r.HandleFunc("/admin/*", proxyHandler.ProxyToCatalog)
		})

		// Player service routes - public watchlist
		r.Get("/users/{userId}/watchlist/public", proxyHandler.ProxyToPlayer)

		// Player service routes (protected)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT))
			r.HandleFunc("/users/*", proxyHandler.ProxyToPlayer)
		})

		// Rooms service routes (protected)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT))
			r.HandleFunc("/rooms/*", proxyHandler.ProxyToRooms)
			r.HandleFunc("/game/*", proxyHandler.ProxyToRooms)
		})

		// Streaming service routes (protected)
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT))
			r.HandleFunc("/streaming/*", proxyHandler.ProxyToStreaming)
		})
	})

	return r
}

// JWTValidationMiddleware validates JWT tokens
func JWTValidationMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	jwtManager := authz.NewJWTManager(jwtConfig)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token == "" {
				httputil.Unauthorized(w)
				return
			}

			claims, err := jwtManager.ValidateAccessToken(token)
			if err != nil {
				httputil.Unauthorized(w)
				return
			}

			ctx := authz.ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RateLimitMiddleware implements rate limiting
func RateLimitMiddleware(cfg config.RateLimitConfig) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.BurstSize)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				httputil.TooManyRequests(w)
				return
			}
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
