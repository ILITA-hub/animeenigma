package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/themes/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	themeHandler *handler.ThemeHandler,
	ratingHandler *handler.RatingHandler,
	adminHandler *handler.AdminHandler,
	videoProxyHandler *handler.VideoProxyHandler,
	jwtConfig authz.JWTConfig,
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

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// Metrics endpoint
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// API routes
	r.Route("/api/themes", func(r chi.Router) {
		// Public routes with optional auth (for user scores)
		r.Group(func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Get("/", themeHandler.ListThemes)
			r.Get("/{id}", themeHandler.GetTheme)
		})

		// Video/audio proxy (public, no auth needed)
		r.Get("/video/{basename}", videoProxyHandler.ProxyVideo)
		r.Get("/audio/{basename}", videoProxyHandler.ProxyAudio)

		// Protected routes (JWT required)
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Post("/{id}/rate", ratingHandler.RateTheme)
			r.Delete("/{id}/rate", ratingHandler.UnrateTheme)
			r.Get("/my-ratings", ratingHandler.GetMyRatings)
		})

		// Admin routes (JWT + admin role)
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Use(AdminRoleMiddleware)
			r.Post("/admin/sync", adminHandler.TriggerSync)
			r.Get("/admin/sync/status", adminHandler.GetSyncStatus)
		})
	})

	return r
}

// AuthMiddleware validates JWT tokens (required).
func AuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
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

// OptionalAuthMiddleware extracts JWT claims if present, but doesn't reject anonymous requests.
func OptionalAuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	jwtManager := authz.NewJWTManager(jwtConfig)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token != "" {
				claims, err := jwtManager.ValidateAccessToken(token)
				if err == nil {
					ctx := authz.ContextWithClaims(r.Context(), claims)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AdminRoleMiddleware ensures the user has admin role.
func AdminRoleMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authz.IsAdmin(r.Context()) {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
