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
	showcaseHandler *handler.ShowcaseHandler,
	compatibilityHandler *handler.CompatibilityHandler,
	malImportHandler *handler.MALImportHandler,
	malExportHandler *handler.MALExportHandler,
	shikimoriImportHandler *handler.ShikimoriImportHandler,
	reportHandler *handler.ReportHandler,
	syncHandler *handler.SyncHandler,
	activityHandler *handler.ActivityHandler,
	exportHandler *handler.ExportHandler,
	preferenceHandler *handler.PreferenceHandler,
	overrideHandler *handler.OverrideHandler,
	adminReportsHandler *handler.AdminReportsHandler, // admin feedback browser
	internalListHandler *handler.InternalListHandler, // hero-spotlight v1.0 Phase 3
	viewerContextHandler *handler.ViewerContextHandler, // anime-page aggregate (page-fetch optimization 2026-06-11)
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

	// Internal feedback write API — used by the host-side maintenance bot to
	// mirror Telegram conversations (and their attachments) into the same
	// on-disk store the /admin/feedback browser reads, and to drive entry
	// status as it analyzes/fixes. Same /internal/* exposure rules as above
	// (not gateway-proxied; player's port is published on 127.0.0.1 only).
	if adminReportsHandler != nil {
		r.Post("/internal/feedback", adminReportsHandler.CreateInternal)
		r.Post("/internal/feedback/{id}/attachments", adminReportsHandler.UploadAttachmentInternal)
		r.Patch("/internal/feedback/{id}/status", adminReportsHandler.SetStatusInternal)
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
			r.Get("/watchlist/facets", listHandler.GetWatchlistFacets)
			r.Post("/watchlist/bulk", listHandler.BulkUpdateList)
			r.Get("/watchlist/{animeId}", listHandler.GetUserAnimeEntry)
			r.Delete("/watchlist/{animeId}", listHandler.DeleteListEntry)
			r.Post("/watchlist/{animeId}/episode", listHandler.MarkEpisodeWatched)
			r.Post("/watchlist/{animeId}/rewatch", listHandler.Rewatch)

			// Profile showcase (Steam-style wall) — owner write. "me" resolves
			// to the JWT claims user id in the handler.
			r.Put("/me/showcase", showcaseHandler.SaveShowcase)

			// Showcase v2 — compatibility score between the viewer (JWT claims)
			// and the profile owner ({userId}). JWT is already enforced by
			// AuthMiddleware on this group; the handler re-checks for safety.
			r.Get("/{userId}/compatibility", compatibilityHandler.GetCompatibility)

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
			if adminReportsHandler != nil {
				// User-facing "my feedback" list — own reports + triage status.
				r.Get("/reports", adminReportsHandler.ListMine)
			}

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

		// The recs HTTP routes (/users/recs anonymous trending row, /admin/recs
		// admin debug + force-recompute, /events public telemetry) moved out of
		// player to services/recs — extraction 2026-06-11.

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
				r.Get("/admin/reports/{id}/attachments/{name}", adminReportsHandler.GetAttachment)
				r.Patch("/admin/reports/{id}/status", adminReportsHandler.SetStatus)
			})
		}

		// Public user watchlist
		r.Get("/users/{userId}/watchlist/public", listHandler.GetPublicWatchlist)
		r.Get("/users/{userId}/watchlist/public/stats", listHandler.GetPublicWatchlistStats)
		r.Get("/users/{userId}/watchlist/facets", listHandler.GetPublicWatchlistFacets)

		// Profile showcase public read (mirrors watchlist/public — lives
		// OUTSIDE the JWT-protected /users group so anonymous viewers can
		// read a profile's showcase once the dark-ship gate is lifted).
		r.Get("/users/{userId}/showcase", showcaseHandler.GetShowcase)

		// Public activity feed
		r.Get("/activity/feed", activityHandler.GetFeed)

		// Batch anime ratings (public)
		r.Post("/anime/ratings/batch", reviewHandler.GetBatchAnimeRatings)

		// Anime reviews routes
		r.Route("/anime/{animeId}", func(r chi.Router) {
			// Public reviews listing — wrapped in OptionalAuthMiddleware so a
			// logged-in viewer's previously-made emoji reactions come back
			// highlighted (reacted_by_me) on first load, while anonymous
			// callers still get the full list. AUTO-408.
			r.Group(func(r chi.Router) {
				r.Use(OptionalAuthMiddleware(jwtConfig))
				r.Get("/reviews", reviewHandler.GetAnimeReviews)
				// Aggregate anime-page context (page-fetch optimization
				// 2026-06-11): rating + watchers-count + the viewer's
				// progress / watchlist entry / review / saved combo in one
				// round-trip. Anonymous callers get the public subset.
				if viewerContextHandler != nil {
					r.Get("/viewer-context", viewerContextHandler.GetViewerContext)
				}
			})
			// Public routes
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
				// AUTO-408 — toggle an emoji reaction on a review.
				r.Post("/reviews/{reviewId}/reactions/{emoji}", reviewHandler.ReactToReview)
				// AUTO-408 — admin moderation: remove a specific user's
				// reaction. Admin-role-gated (handler re-checks too).
				r.Group(func(r chi.Router) {
					r.Use(AdminRoleMiddleware)
					r.Delete("/reviews/{reviewId}/reactions/{emoji}/users/{userId}", reviewHandler.AdminRemoveReaction)
				})
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
