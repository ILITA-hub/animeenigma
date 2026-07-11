package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/library/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter wires the chi router used by the library service. Phase
// 2 adds GET /api/library/search and Phase 3 adds the /api/library/
// jobs group (POST/GET/GET-by-id/DELETE) under the existing
// /api/library route group; auth is enforced at the gateway (see
// services/gateway/internal/transport/router.go — the /api/library/*
// prefix has JWTValidationMiddleware + AdminRoleMiddleware applied
// for all routes except /health).
//
// The jwtConfig argument is retained for forward compat — Phase 4+
// may want server-side enforcement on a subset of routes.
func NewRouter(
	healthHandler *handler.HealthHandler,
	searchHandler *handler.SearchHandler,
	jobsHandler *handler.JobsHandler,
	episodesHandler *handler.EpisodesHandler,
	autocacheConfigHandler *handler.AutocacheConfigHandler,
	autocacheInternalHandler *handler.AutocacheInternalHandler,
	filesHandler *handler.FilesHandler,
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	// Standard middleware chain (mirrors services/themes/internal/transport/router.go).
	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Health check — exposed at /health for direct probes (make health,
	// docker healthcheck) and at /api/library/health so the gateway can
	// forward `/api/library/health` verbatim without path rewriting.
	r.Get("/health", healthHandler.Health)

	// Prometheus metrics endpoint.
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// Phase 08 (SERVE-01/02/03): Docker-network-only serve-signal endpoints
	// the catalog ae-resolution producer (Plan 03) calls. Mounted at the TOP
	// LEVEL — siblings of /health + /metrics, NOT inside the /api/library group
	// — because the gateway does NOT proxy /internal/* (same rule as recs'
	// /internal/recs/recompute-hint + notifications' /internal/notifications),
	// so /internal/library/* is unreachable from the gateway/public by
	// construction. Do NOT add a gateway HandleFunc for these.
	if autocacheInternalHandler != nil {
		r.Post("/internal/library/autocache/fetch", autocacheInternalHandler.Fetch)
		r.Post("/internal/library/autocache/demand", autocacheInternalHandler.Demand)
	}

	// Docker-network-only: the newest distinct-anime library uploads, feeding
	// the analytics playback probe's ae target set. Same /internal/* non-proxied
	// rule as the autocache signals above.
	if episodesHandler != nil {
		r.Get("/internal/library/recent-episodes", episodesHandler.RecentEpisodes)
	}

	// API routes. Phase 2 adds /search; Phase 3 adds the job-control
	// group. Gateway-side admin gate covers all /api/library/*
	// non-/health routes (services/gateway/internal/transport/router.go).
	r.Route("/api/library", func(r chi.Router) {
		_ = jwtConfig // retained for forward compat (Phase 4+)
		r.Get("/health", healthHandler.Health)
		// Phase 5 (LIB-09): extended health probe for the admin UI's
		// stats strip. Returns disk + active-torrents + per-status
		// active-jobs counts. Admin-gated by the gateway prefix.
		r.Get("/health/extended", healthHandler.HealthExtended)
		r.Get("/search", searchHandler.Search)
		if jobsHandler != nil {
			r.Post("/jobs", jobsHandler.Create)
			r.Get("/jobs", jobsHandler.List)
			r.Get("/jobs/{id}", jobsHandler.Get)
			r.Delete("/jobs/{id}", jobsHandler.Delete)
			// Phase 5 (LIB-09): retroactive shikimori_id link + retry of failed jobs.
			r.Patch("/jobs/{id}", jobsHandler.Link)
			r.Post("/jobs/{id}/retry", jobsHandler.Retry)
		}
		// Phase 04: read-only episodes endpoint consumed by the Phase 5
		// admin UI and the Phase 6 hybrid resolver. Admin-gated via the
		// gateway prefix; no additional server-side enforcement needed.
		if episodesHandler != nil {
			r.Get("/episodes/{shikimori_id}/{episode}", episodesHandler.Get)
			// List every locally-encoded episode for an anime — consumed by
			// the catalog's first-party ("ae") provider resolver. Internal
			// (catalog→library over the docker network); admin-gated like the
			// rest of /api/library/* at the gateway.
			r.Get("/episodes/{shikimori_id}", episodesHandler.List)
		}
		// Phase 07 (POOL-04 + POOL-05): live-editable autocache config —
		// singleton GET/PATCH consumed by the admin UI; the master `enabled`
		// switch + freshness/budget windows are read by the future
		// downloader/evictor (Phases 8-10). Admin-gated via the gateway
		// /api/library/* prefix; no server-side enforcement needed.
		if autocacheConfigHandler != nil {
			r.Get("/autocache/config", autocacheConfigHandler.Get)
			r.Patch("/autocache/config", autocacheConfigHandler.Patch)
		}
		// Task 5/6: admin file-manager browse/download/delete over the torrent
		// working dir and the object stores. Admin-gated via the gateway
		// /api/library/* prefix; no additional server-side enforcement needed.
		if filesHandler != nil {
			r.Get("/files", filesHandler.Browse)
			r.Get("/files/download", filesHandler.Download)
			r.Delete("/files", filesHandler.Delete)
		}
	})

	return r
}
