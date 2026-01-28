package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	catalogHandler *handler.CatalogHandler,
	adminHandler *handler.AdminHandler,
	cfg *config.Config,
	log *logger.Logger,
) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public catalog routes
		r.Route("/anime", func(r chi.Router) {
			r.Get("/search", catalogHandler.SearchAnime)
			r.Get("/browse", catalogHandler.BrowseAnime)
			r.Get("/seasonal/{year}/{season}", catalogHandler.GetSeasonalAnime)
			r.Get("/{animeId}", catalogHandler.GetAnime)
			r.Get("/{animeId}/episodes", catalogHandler.GetAnimeEpisodes)
		})

		r.Get("/genres", catalogHandler.GetGenres)

		// Admin routes (require authentication)
		r.Route("/admin", func(r chi.Router) {
			r.Use(AuthMiddleware(cfg.JWT))
			r.Use(AdminMiddleware)

			r.Post("/anime", adminHandler.CreateAnime)
			r.Post("/anime/{animeId}/videos", adminHandler.AddVideoSource)
			r.Delete("/videos/{videoId}", adminHandler.DeleteVideo)
			r.Post("/sync/shikimori/{shikimoriId}", adminHandler.SyncFromShikimori)
		})
	})

	return r
}

// AuthMiddleware validates JWT tokens
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

// AdminMiddleware ensures the user has admin role
func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authz.IsAdmin(r.Context()) {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
