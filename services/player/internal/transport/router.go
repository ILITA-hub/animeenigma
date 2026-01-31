package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	progressHandler *handler.ProgressHandler,
	listHandler *handler.ListHandler,
	historyHandler *handler.HistoryHandler,
	reviewHandler *handler.ReviewHandler,
	jwtConfig authz.JWTConfig,
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
	r.Route("/api", func(r chi.Router) {
		// Protected routes - user data
		r.Route("/users", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))

			// Watchlist routes
			r.Get("/watchlist", listHandler.GetUserList)
			r.Post("/watchlist", listHandler.AddToList)
			r.Put("/watchlist", listHandler.UpdateListEntry)
			r.Delete("/watchlist/{animeId}", listHandler.DeleteListEntry)

			// Progress routes
			r.Post("/progress", progressHandler.UpdateProgress)
			r.Get("/progress/{animeId}", progressHandler.GetProgress)

			// History routes
			r.Get("/history", historyHandler.GetWatchHistory)

			// User's reviews
			r.Get("/reviews", reviewHandler.GetUserReviews)
		})

		// Anime reviews routes
		r.Route("/anime/{animeId}", func(r chi.Router) {
			// Public routes
			r.Get("/reviews", reviewHandler.GetAnimeReviews)
			r.Get("/rating", reviewHandler.GetAnimeRating)

			// Protected routes
			r.Group(func(r chi.Router) {
				r.Use(AuthMiddleware(jwtConfig))
				r.Post("/reviews", reviewHandler.CreateOrUpdateReview)
				r.Get("/reviews/me", reviewHandler.GetUserReview)
				r.Delete("/reviews", reviewHandler.DeleteReview)
			})
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
