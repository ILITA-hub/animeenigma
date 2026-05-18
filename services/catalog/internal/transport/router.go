package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	catalogHandler *handler.CatalogHandler,
	adminHandler *handler.AdminHandler,
	newsHandler *handler.NewsHandler,
	collectionHandler *handler.CollectionHandler,
	skipTimesHandler *handler.SkipTimesHandler,
	rawHandler *handler.RawHandler,
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
		// News endpoint (before /anime route to avoid wildcard conflict)
		r.Get("/anime/news", newsHandler.GetNews)

		// Public catalog routes
		r.Route("/anime", func(r chi.Router) {
			r.Get("/", catalogHandler.BrowseAnime) // GET /api/anime - default list
			r.Get("/search", catalogHandler.SearchAnime)
			r.Get("/browse", catalogHandler.BrowseAnime)
			r.Get("/trending", catalogHandler.GetTrendingAnime)
			r.Get("/popular", catalogHandler.GetPopularAnime)
			r.Get("/recent", catalogHandler.GetRecentAnime)
			r.Get("/schedule", catalogHandler.GetSchedule)
			r.Get("/ongoing", catalogHandler.GetOngoingAnime)
			r.Post("/batch-refresh", catalogHandler.BatchRefreshAnime)
			r.Post("/calendar-sync", catalogHandler.SyncCalendar)
			r.Get("/seasonal/{year}/{season}", catalogHandler.GetSeasonalAnime)
			r.Get("/mal/{malId}", catalogHandler.ResolveMALAnime)
			r.Get("/shikimori/{shikimoriId}", catalogHandler.ResolveShikimoriAnime)
			r.Get("/{animeId}", catalogHandler.GetAnime)
			r.Post("/{animeId}/refresh", catalogHandler.RefreshAnime)
			r.Get("/{animeId}/episodes", catalogHandler.GetAnimeEpisodes)
			r.Get("/{animeId}/related", catalogHandler.GetRelatedAnime)
			// Phase 13 (REC-SIG-06): Shikimori /similar endpoint feed for the
			// player service's S6 pin cascade. Public read, no auth required.
			r.Get("/{animeId}/similar", catalogHandler.GetSimilarAnime)
			// Pinned translations (public)
			r.Get("/{animeId}/pinned-translations", catalogHandler.GetPinnedTranslations)
			r.Post("/{animeId}/pin-translation", catalogHandler.PinTranslation)
			r.Delete("/{animeId}/pin-translation/{translationId}", catalogHandler.UnpinTranslation)
			// Aniboom video sources
			r.Get("/{animeId}/aniboom/translations", catalogHandler.GetAniboomTranslations)
			r.Get("/{animeId}/aniboom/video", catalogHandler.GetAniboomVideo)
			// Kodik video sources
			r.Get("/{animeId}/kodik/translations", catalogHandler.GetKodikTranslations)
			r.Get("/{animeId}/kodik/video", catalogHandler.GetKodikVideo)
			// HiAnime video sources
			r.Get("/{animeId}/hianime/episodes", catalogHandler.GetHiAnimeEpisodes)
			r.Get("/{animeId}/hianime/servers", catalogHandler.GetHiAnimeServers)
			r.Get("/{animeId}/hianime/stream", catalogHandler.GetHiAnimeStream)
			// Consumet video sources
			r.Get("/{animeId}/consumet/episodes", catalogHandler.GetConsumetEpisodes)
			r.Get("/{animeId}/consumet/servers", catalogHandler.GetConsumetServers)
			r.Get("/{animeId}/consumet/stream", catalogHandler.GetConsumetStream)
			// AnimeLib video sources
			r.Get("/{animeId}/animelib/episodes", catalogHandler.GetAnimeLibEpisodes)
			r.Get("/{animeId}/animelib/translations", catalogHandler.GetAnimeLibTranslations)
			r.Get("/{animeId}/animelib/stream", catalogHandler.GetAnimeLibStream)
			// Scraper (Phase 15+ — universal English provider orchestration via
			// the scraper microservice on :8088. Phase 15: episodes/servers/stream
			// return 503 not-yet-implemented; health returns the live snapshot.
			// Phase 16+ plugs in real provider implementations.)
			r.Get("/{animeId}/scraper/episodes", catalogHandler.GetScraperEpisodes)
			r.Get("/{animeId}/scraper/servers", catalogHandler.GetScraperServers)
			r.Get("/{animeId}/scraper/stream", catalogHandler.GetScraperStream)
			r.Get("/{animeId}/scraper/health", catalogHandler.GetScraperHealth)
			// Raw JP video source (workstream raw-jp, Phase 01). AllAnime
			// GraphQL persisted-query API resolves original Japanese audio
			// HLS streams. Public, no auth.
			r.Get("/{animeId}/raw/episodes", rawHandler.GetEpisodes)
			r.Get("/{animeId}/raw/stream", rawHandler.GetStream)
			// Hanime video sources
			r.Get("/{animeId}/hanime/episodes", catalogHandler.GetHanimeEpisodes)
			r.Get("/{animeId}/hanime/stream", catalogHandler.GetHanimeStream)
			// Jimaku Japanese subtitles
			r.Get("/{animeId}/jimaku/subtitles", catalogHandler.GetJimakuSubtitles)
		})

		// Kodik search (for finding anime not in our DB)
		r.Get("/kodik/search", catalogHandler.SearchKodik)

		// HiAnime search
		r.Get("/hianime/search", catalogHandler.SearchHiAnime)

		// Consumet search
		r.Get("/consumet/search", catalogHandler.SearchConsumet)

		// AnimeLib search
		r.Get("/animelib/search", catalogHandler.SearchAnimeLib)

		// Phase 18 (UX-34) — Skip-Intro / Skip-Outro CTA timestamps.
		// Public, no auth. Backend proxy of aniskip.com with 7d cache.
		// Both path segments must be positive integers — the handler
		// enforces this so chi's path-param parse is not the only gate.
		r.Get("/skip-times/{malId}/{episode}", skipTimesHandler.Get)

		r.Get("/genres", catalogHandler.GetGenres)

		// Phase 17 (UX-33) — public editorial collections. Registered
		// BEFORE the /admin group so chi's longest-prefix match resolves
		// public routes first (admin uses a literal /admin/ prefix so
		// there's no actual collision, but the convention stays consistent
		// with other public-before-admin blocks in this router).
		r.Route("/collections", func(r chi.Router) {
			r.Get("/", collectionHandler.ListPublic)
			r.Get("/{slug}", collectionHandler.GetBySlug)
		})

		// Admin routes (require authentication)
		r.Route("/admin", func(r chi.Router) {
			r.Use(AuthMiddleware(cfg.JWT))
			r.Use(AdminMiddleware)

			r.Post("/anime", adminHandler.CreateAnime)
			r.Post("/anime/{animeId}/videos", adminHandler.AddVideoSource)
			r.Delete("/videos/{videoId}", adminHandler.DeleteVideo)
			r.Post("/sync/shikimori/{shikimoriId}", adminHandler.SyncFromShikimori)

			// Hide/unhide anime
			r.Post("/anime/{animeId}/hide", adminHandler.HideAnime)
			r.Delete("/anime/{animeId}/hide", adminHandler.UnhideAnime)
			r.Get("/anime/hidden", adminHandler.GetHiddenAnime)

			// Update shikimori_id
			r.Patch("/anime/{animeId}/shikimori", adminHandler.UpdateShikimoriID)

			// Link MAL ID
			r.Patch("/anime/{animeId}/mal", adminHandler.LinkMALID)

			// Phase 17 (UX-33) — editorial collections admin CRUD.
			r.Get("/collections", collectionHandler.ListAdmin)
			r.Post("/collections", collectionHandler.Create)
			r.Get("/collections/{id}", collectionHandler.GetAdmin)
			r.Put("/collections/{id}", collectionHandler.Update)
			r.Delete("/collections/{id}", collectionHandler.Delete)
			r.Post("/collections/{id}/items", collectionHandler.AddItem)
			r.Delete("/collections/{id}/items/{animeId}", collectionHandler.RemoveItem)
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
