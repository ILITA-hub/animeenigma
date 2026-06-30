package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	catalogHandler *handler.CatalogHandler,
	characterHandler *handler.CharacterHandler,
	adminHandler *handler.AdminHandler,
	newsHandler *handler.NewsHandler,
	collectionHandler *handler.CollectionHandler,
	skipTimesHandler *handler.SkipTimesHandler,
	aeHandler *handler.AeHandler,
	subtitlesHandler *handler.SubtitlesHandler,
	internalCacheHandler *handler.InternalCacheHandler,
	internalEpisodesHandler *handler.InternalEpisodesHandler,
	internalEpisodesValidateHandler *handler.InternalEpisodesValidateHandler,
	internalScraperProvidersHandler *handler.InternalScraperProvidersHandler,
	internalProbeHandler *handler.InternalProbeHandler,
	internalSubtitleProbeHandler *handler.InternalSubtitleProbeHandler,
	spotlightHandler *handler.SpotlightHandler,
	internalGuessPoolHandler *handler.InternalGuessPoolHandler,
	capabilitiesHandler *handler.CapabilitiesHandler,
	internalProviderPolicyHandler *handler.InternalProviderPolicyHandler,
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
	// AR-EGRESS-01/02: seed origin + coarse operation (chi route pattern) into
	// W3C baggage and the authenticated user_id into a private ctx value, so the
	// recording transport attributes every outbound effect to the inbound request
	// that caused it (user_id never rides the wire — T-02-PII). Mounted on the
	// chi router so the lazy operation resolver can read the route pattern.
	r.Use(tracing.SeedMiddleware("catalog"))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	// Metrics endpoint
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// Internal endpoints (workstream raw-jp / v0.2 / Phase 06).
	// Mounted OUTSIDE /api with no AuthMiddleware — nginx/gateway
	// does NOT proxy /internal/*, so the route is reachable only
	// from within the docker network. Mirrors the precedent set by
	// services/auth/internal/transport/router.go's
	// /internal/resolve-api-key.
	if internalCacheHandler != nil {
		r.Post("/internal/cache/invalidate/raw/{shikimoriId}", internalCacheHandler.InvalidateRaw)
	}

	// Phase 2 v1.0 Notifications Engine — NOTIF-DET-01 / D-DET-02.
	// Same gateway-non-routing security model as InvalidateRaw above.
	if internalEpisodesHandler != nil {
		r.Get("/internal/anime/{shikimoriId}/episodes", internalEpisodesHandler.GetLatestEpisode)
	}

	// Watch-Together workstream / Phase 04 — WT-STATE-02.
	// Sibling of /internal/anime/{shikimoriId}/episodes — validates a
	// (player, episode_id, translation_id) tuple for the watch-together
	// service's state:change_* inbound handlers. Same root-router,
	// no-middleware security model. The notifications detector's
	// /episodes contract is NOT modified.
	if internalEpisodesValidateHandler != nil {
		r.Get("/internal/anime/{shikimoriId}/episodes/validate", internalEpisodesValidateHandler.Validate)
	}

	// Scraper provider config + capability traits (spec 2026-06-15).
	// Same gateway-non-routing security model as the internal endpoints above.
	if internalScraperProvidersHandler != nil {
		r.Get("/internal/scraper/providers", internalScraperProvidersHandler.List)
	}

	// Probe-result endpoint: applies a scraper verdict (pass/fail) via the
	// Task-5 state machine (providerpolicy.ApplyVerdict) and persists the
	// resulting policy/health transition. Reachable only from within the
	// Docker network — same gateway-non-routing security model as peers above.
	if internalProviderPolicyHandler != nil {
		r.Post("/internal/providers/probe-result", internalProviderPolicyHandler.ProbeResult)
		r.Get("/internal/providers/probe-plan", internalProviderPolicyHandler.ProbePlan)
	}

	// Playback-probe ae target set (newest distinct-anime library uploads mapped
	// to catalog UUIDs). Same gateway-non-routing model as the endpoints above.
	if internalProbeHandler != nil {
		r.Get("/internal/probe/ae-targets", internalProbeHandler.AeTargets)
	}

	if internalSubtitleProbeHandler != nil {
		r.Post("/internal/subtitle-probe/run", internalSubtitleProbeHandler.Run)
	}

	// Anidle guess-game pool (spec 2026-06-15) — Docker-network only.
	if internalGuessPoolHandler != nil {
		r.Get("/internal/guessgame/pool", internalGuessPoolHandler.GetPool)
	}

	// API routes
	r.Route("/api", func(r chi.Router) {
		// News endpoint (before /anime route to avoid wildcard conflict)
		r.Get("/anime/news", newsHandler.GetNews)

		// Hero spotlight aggregator (workstream hero-spotlight, v1.0 Phase 1).
		// Phase 3 (Plan 03-04) wraps this — and ONLY this — route with
		// OptionalAuthMiddleware so login-only cards (personal_pick login
		// path, not_time_yet, continue_watching_new) become eligible when a
		// valid JWT is present. The middleware NEVER 401s — anon callers
		// proceed with userID=nil. Feature-flag gated INSIDE the handler —
		// returns bare 404 when SPOTLIGHT_ENABLED=false (see config.go).
		r.With(OptionalAuthMiddleware(cfg.JWT)).Get("/home/spotlight", spotlightHandler.Get)
		// v4 B-1 «Ещё разок» — public, no auth (the random pick is global).
		r.Get("/home/spotlight/reroll", spotlightHandler.GetReroll)

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
			// Pinned translations: reading the pins is public, but pinning is a
			// GLOBAL curation action (PinnedTranslation has no user_id — a pin
			// reorders the listing for everyone), so the writes are admin-only.
			// They previously sat here unauthenticated, letting any anonymous
			// caller pin/unpin (audit #4). URLs are unchanged; the FE admin UI
			// sends the JWT and already handles the error path.
			r.Get("/{animeId}/pinned-translations", catalogHandler.GetPinnedTranslations)
			r.Group(func(r chi.Router) {
				r.Use(AuthMiddleware(cfg.JWT))
				r.Use(AdminMiddleware)
				r.Post("/{animeId}/pin-translation", catalogHandler.PinTranslation)
				r.Delete("/{animeId}/pin-translation/{translationId}", catalogHandler.UnpinTranslation)
			})
			// Aniboom video sources
			r.Get("/{animeId}/aniboom/translations", catalogHandler.GetAniboomTranslations)
			r.Get("/{animeId}/aniboom/video", catalogHandler.GetAniboomVideo)
			// Kodik video sources
			r.Get("/{animeId}/kodik/translations", catalogHandler.GetKodikTranslations)
			r.Get("/{animeId}/kodik/video", catalogHandler.GetKodikVideo)
			r.Get("/{animeId}/kodik/stream", catalogHandler.GetKodikStream)
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
			// P2.8: wrap ONLY the stream route with OptionalAuthMiddleware so an
			// authed caller's JWT user id can seed the per-user quota key
			// (scraperUserKey) for the stealth-scraper sidecar's per-user
			// admission. The middleware NEVER 401s — anonymous playback proceeds
			// without claims and falls through to the salted-IP key.
			r.With(OptionalAuthMiddleware(cfg.JWT)).Get("/{animeId}/scraper/stream", catalogHandler.GetScraperStream)
			r.Get("/{animeId}/scraper/health", catalogHandler.GetScraperHealth)
			// Ranked capability report (spec 2026-06-15 P4 — EN family in v1).
			// Public, no auth — same as the scraper routes above.
			if capabilitiesHandler != nil {
				r.Get("/{animeId}/capabilities", capabilitiesHandler.Get)
			}
			// First-party ("AnimeEnigma") provider: self-hosted library
			// (MinIO HLS) only — episodes/stream resolve straight from what's
			// encoded on-prem, with proxy-signed URLs. Public, no auth.
			r.Get("/{animeId}/ae/episodes", aeHandler.GetAeEpisodes)
			r.Get("/{animeId}/ae/stream", aeHandler.GetAeStream)
			// Multi-provider subtitles (workstream raw-jp, Phase 02). Jimaku
			// + OpenSubtitles merged via /service/subs_aggregator.go.
			r.Get("/{animeId}/subtitles", subtitlesHandler.Get)
			r.Get("/{animeId}/subtitles/all", subtitlesHandler.GetAll)
			// Lazy OpenSubtitles file resolve — spends 1 download quota unit
			// per cache miss, then cached 24h (workstream raw-jp follow-on).
			r.Get("/{animeId}/subtitles/opensubtitles/file/{fileID}", subtitlesHandler.GetOpenSubtitlesFile)
			// Lazy anime365 file resolve — fetches RU subtitle bytes (ASS→VTT),
			// proxied + cached 24h (spec 2026-06-24).
			r.Get("/{animeId}/subtitles/anime365/file/{transId}", subtitlesHandler.GetAnime365File)
			// Hanime video sources
			r.Get("/{animeId}/hanime/episodes", catalogHandler.GetHanimeEpisodes)
			r.Get("/{animeId}/hanime/stream", catalogHandler.GetHanimeStream)
			// AnimeJoy RU-sub video sources — two independent leg providers
			// (animejoy-sibnet / animejoy-allvideo) over a shared discovery
			// base. Public, no auth, like the sibling stream routes. The
			// resolved tokened MP4 is proxy-signed; the FE forwards
			// url/referer/exp/sig to /api/streaming/hls-proxy.
			r.Get("/{animeId}/animejoy-sibnet/stream", catalogHandler.GetAnimejoySibnetStream)
			r.Get("/{animeId}/animejoy-allvideo/stream", catalogHandler.GetAnimejoyAllVideoStream)
			// Per-leg episode + team inventory (the FE adapter's listEpisodes /
			// listTeams source — the player drives a per-provider episode list,
			// one per leg). Same public/no-auth wrapping as the sibling stream
			// routes above.
			r.Get("/{animeId}/animejoy-sibnet/episodes", catalogHandler.GetAnimejoySibnetEpisodes)
			r.Get("/{animeId}/animejoy-allvideo/episodes", catalogHandler.GetAnimejoyAllVideoEpisodes)
			// 18anime (18+) video sources
			r.Get("/{animeId}/anime18/episodes", catalogHandler.GetAnime18Episodes)
			r.Get("/{animeId}/anime18/stream", catalogHandler.GetAnime18Stream)
			// Jimaku Japanese subtitles
			r.Get("/{animeId}/jimaku/subtitles", catalogHandler.GetJimakuSubtitles)
			// Character roster for an anime
			r.Get("/{animeId}/characters", characterHandler.GetAnimeCharacters)
		})

		// Kodik search (for finding anime not in our DB)
		r.Get("/kodik/search", catalogHandler.SearchKodik)

		// AnimeLib search
		r.Get("/animelib/search", catalogHandler.SearchAnimeLib)

		// Phase 18 (UX-34) — Skip-Intro / Skip-Outro CTA timestamps.
		// Public, no auth. Backend proxy of aniskip.com with 7d cache.
		// Both path segments must be positive integers — the handler
		// enforces this so chi's path-param parse is not the only gate.
		r.Get("/skip-times/{malId}/{episode}", skipTimesHandler.Get)

		r.Get("/genres", catalogHandler.GetGenres)
		r.Get("/studios", catalogHandler.GetStudios)

		// Phase 17 (UX-33) — public editorial collections. Registered
		// BEFORE the /admin group so chi's longest-prefix match resolves
		// public routes first (admin uses a literal /admin/ prefix so
		// there's no actual collision, but the convention stays consistent
		// with other public-before-admin blocks in this router).
		r.Route("/collections", func(r chi.Router) {
			r.Get("/", collectionHandler.ListPublic)
			r.Get("/{slug}", collectionHandler.GetBySlug)
		})

		// Public character routes
		r.Route("/characters", func(r chi.Router) {
			r.Get("/{characterId}", characterHandler.GetCharacter)
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
