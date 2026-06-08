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
	commentHandler *handler.CommentHandler,
	malImportHandler *handler.MALImportHandler,
	malExportHandler *handler.MALExportHandler,
	shikimoriImportHandler *handler.ShikimoriImportHandler,
	reportHandler *handler.ReportHandler,
	syncHandler *handler.SyncHandler,
	activityHandler *handler.ActivityHandler,
	exportHandler *handler.ExportHandler,
	preferenceHandler *handler.PreferenceHandler,
	overrideHandler *handler.OverrideHandler,
	recsHandler *handler.RecsHandler,
	adminRecsHandler *handler.AdminRecsHandler, // Phase 14 (REC-ADMIN-01 / REC-ADMIN-02)
	adminReportsHandler *handler.AdminReportsHandler, // admin feedback browser
	recEventsHandler *handler.RecEventsHandler, // Phase 14 (REC-EVAL-01)
	internalListHandler *handler.InternalListHandler, // hero-spotlight v1.0 Phase 3
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

	// Internal endpoint (workstream hero-spotlight, v1.0 Phase 3).
	// Mounted OUTSIDE /api with no AuthMiddleware — nginx/gateway does NOT
	// proxy /internal/*, so the route is reachable only from within the
	// docker network. Defense-in-depth gateway-not-proxied assertion is
	// added in Plan 04's gateway router test. Precedent:
	// services/catalog/internal/transport/router.go's
	// /internal/cache/invalidate/raw/{shikimoriId} +
	// /internal/anime/{shikimoriId}/episodes routes.
	if internalListHandler != nil {
		r.Get("/internal/users/{user_id}/list", internalListHandler.ListByStatuses)
	}

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
			r.Post("/progress/{animeId}/dropoff", progressHandler.MarkDropOff)

			// Continue-Watching row (Phase 8 / UX-15 / UA-061). One row per anime,
			// most recent in-progress episode, ordered by last_watched_at DESC.
			// Mounted as a leaf inside /users (not under /progress) so the
			// gateway's existing /users/* JWT proxy catches it without a new
			// gateway route.
			r.Get("/continue-watching", progressHandler.ListContinueWatching)

			// Bulk per-card anime-progress (Phase 9 / UX-16). Comma-separated
			// ?ids=a,b,c (max 50). Returns a JSON object keyed by anime_id
			// with the user's furthest episode reached + completion flags.
			// The AnimeCardNew composable batches per visible grid page.
			r.Get("/anime-progress", progressHandler.GetBulkProgress)

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
			r.Get("/preferences/global", preferenceHandler.GetGlobalPreferences)
			r.Get("/preferences/tier2", preferenceHandler.GetTier2DebugView)
			r.Delete("/preferences/learned", preferenceHandler.ResetLearnedPreferences)
			r.Get("/preferences/{animeId}", preferenceHandler.GetAnimePreference)
			r.Post("/preferences/{animeId}/force", preferenceHandler.ForceCombo)
		})

		// Public preference routes — anon-friendly via OptionalAuthMiddleware.
		// Per CONTEXT Critical Finding 1: this group lives OUTSIDE /users so the gateway's
		// JWT validation does not reject anon callers before they reach the player service.
		r.Route("/preferences", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Post("/resolve", preferenceHandler.ResolvePreference)
			r.Post("/override", overrideHandler.RecordOverride)
		})

		// Phase 10: anonymous trending recs row.
		// MUST live OUTSIDE the protected r.Route("/users", ...) group above —
		// that group uses AuthMiddleware which 401s anonymous callers. We mount
		// /users/recs as a sibling and apply OptionalAuthMiddleware so JWTs
		// decode if present but anonymous callers pass through with the same
		// payload (Phase 11 will branch on claims for personalization).
		r.Route("/users/recs", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Get("/", recsHandler.GetRecs)
		})

		// Phase 14 (REC-ADMIN-01 / REC-ADMIN-02): admin debug + force-recompute.
		// AuthMiddleware first (401 on missing/invalid JWT), AdminRoleMiddleware
		// second (403 on non-admin role). Defense-in-depth: the gateway also
		// applies the same gates (services/gateway/internal/transport/router.go).
		r.Route("/admin/recs", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Use(AdminRoleMiddleware)
			r.Get("/{user_id}", adminRecsHandler.GetAdminRecs)
			r.Post("/{user_id}/recompute", adminRecsHandler.ForceRecompute)
		})

		// Admin feedback browser: read + triage the on-disk user feedback /
		// error reports that SubmitReport persists. Admin-role-gated (gateway
		// applies the same gates again — defense-in-depth). Explicit paths (not
		// a nested Route with Get("/")) so the bare `/admin/reports` list path
		// matches without a trailing-slash redirect.
		if adminReportsHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(AuthMiddleware(jwtConfig))
				r.Use(AdminRoleMiddleware)
				r.Get("/admin/reports", adminReportsHandler.List)
				r.Get("/admin/reports/{id}", adminReportsHandler.Get)
				r.Patch("/admin/reports/{id}/status", adminReportsHandler.SetStatus)
			})
		}

		// Phase 14 (REC-EVAL-01): public telemetry endpoint. JWT-OPTIONAL —
		// anonymous trending CTR data is valid per CONTEXT.md §C4. The handler
		// increments Prometheus counters AND persists a rec_events row.
		r.Route("/events", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Post("/rec", recEventsHandler.PostRecEvent)
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
			// Phase 1 (workstream: social) plan 04 — public comment listing.
			// MUST live outside the AuthMiddleware-protected group below so
			// anonymous readers can fetch the comments feed.
			r.Get("/comments", commentHandler.ListComments)
			// Phase 14 (ui-ux-audit / UX-28) — soft social-proof follower
			// count. Public, no auth: returns { count: int } of users with
			// status='watching' for this anime. Hidden in the UI when
			// count < 5 to avoid empty signals on niche titles.
			r.Get("/watchers-count", listHandler.GetWatchersCount)

			// Protected routes
			r.Group(func(r chi.Router) {
				r.Use(AuthMiddleware(jwtConfig))
				r.Post("/reviews", reviewHandler.CreateOrUpdateReview)
				r.Get("/reviews/me", reviewHandler.GetUserReview)
				r.Delete("/reviews", reviewHandler.DeleteReview)
				// Phase 1 (workstream: social) plan 04 — comment mutations.
				r.Post("/comments", commentHandler.CreateComment)
				r.Patch("/comments/{commentId}", commentHandler.UpdateComment)
				r.Delete("/comments/{commentId}", commentHandler.DeleteComment)
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
