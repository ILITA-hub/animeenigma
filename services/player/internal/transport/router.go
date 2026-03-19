package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	progressHandler *handler.ProgressHandler,
	listHandler *handler.ListHandler,
	historyHandler *handler.HistoryHandler,
	reviewHandler *handler.ReviewHandler,
	malImportHandler *handler.MALImportHandler,
	malExportHandler *handler.MALExportHandler,
	shikimoriImportHandler *handler.ShikimoriImportHandler,
	reportHandler *handler.ReportHandler,
	syncHandler *handler.SyncHandler,
	activityHandler *handler.ActivityHandler,
	exportHandler *handler.ExportHandler,
	preferenceHandler *handler.PreferenceHandler,
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
	r.Route("/api", func(r chi.Router) {
		// Protected routes - user data
		r.Route("/users", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))

			// Watchlist routes
			r.Get("/watchlist", listHandler.GetUserList)
			r.Post("/watchlist", listHandler.AddToList)
			r.Put("/watchlist", listHandler.UpdateListEntry)
			r.Post("/watchlist/migrate", listHandler.MigrateListEntry)
			r.Get("/watchlist/statuses", listHandler.GetWatchlistStatuses)
			r.Get("/watchlist/{animeId}", listHandler.GetUserAnimeEntry)
			r.Delete("/watchlist/{animeId}", listHandler.DeleteListEntry)
			r.Post("/watchlist/{animeId}/episode", listHandler.MarkEpisodeWatched)

			// Progress routes
			r.Post("/progress", progressHandler.UpdateProgress)
			r.Get("/progress/{animeId}", progressHandler.GetProgress)

			// History routes
			r.Get("/history", historyHandler.GetWatchHistory)

			// User's reviews
			r.Get("/reviews", reviewHandler.GetUserReviews)

			// MAL Import (async - background goroutine)
			r.Post("/import/mal", malImportHandler.ImportMALList)

			// Shikimori Import (async - background goroutine)
			r.Post("/import/shikimori", shikimoriImportHandler.ImportShikimoriList)
			r.Post("/import/shikimori/migrate", shikimoriImportHandler.MigrateShikimoriEntries)

			// Unified job status polling
			r.Get("/import/{jobId}", syncHandler.GetJobStatus)

			// Sync status for page-load resume
			r.Get("/sync/status", syncHandler.GetSyncStatus)

			// MAL Export (async - queued)
			r.Post("/mal-export", malExportHandler.InitiateExport)
			r.Get("/mal-export", malExportHandler.GetUserExports)
			r.Get("/mal-export/{exportId}", malExportHandler.GetExportStatus)
			r.Delete("/mal-export/{exportId}", malExportHandler.CancelExport)

			// JSON export
			r.Get("/export/json", exportHandler.ExportJSON)

			// Error reports
			r.Post("/report", reportHandler.SubmitReport)

			// Preference routes
			r.Post("/preferences/resolve", preferenceHandler.ResolvePreference)
			r.Get("/preferences/global", preferenceHandler.GetGlobalPreferences)
			r.Get("/preferences/{animeId}", preferenceHandler.GetAnimePreference)
		})

		// Public user watchlist
		r.Get("/users/{userId}/watchlist/public", listHandler.GetPublicWatchlist)
		r.Get("/users/{userId}/watchlist/public/stats", listHandler.GetPublicWatchlistStats)

		// Public activity feed
		r.Get("/activity/feed", activityHandler.GetFeed)

		// Batch anime ratings (public)
		r.Post("/anime/ratings/batch", reviewHandler.GetBatchAnimeRatings)

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
