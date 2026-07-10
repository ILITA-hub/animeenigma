package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/handler"
)

func NewRouter(
	adminH *handler.AdminFlagsHandler,
	publicH *handler.PublicFlagsHandler,
	internalH *handler.InternalRulesetHandler,
	adminMaintH *handler.AdminMaintenanceHandler,
	internalMaintH *handler.InternalMaintenanceHandler,
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// Docker-network-only ruleset feed (gateway does NOT proxy /internal/*).
	r.Get("/internal/policy/ruleset", internalH.GetRuleset)
	// Docker-network + host-loopback only (never gateway-proxied) — a routine
	// reads its gate then writes back status after each run.
	r.Get("/internal/maintenance/routines/{id}", internalMaintH.Gate)
	r.Post("/internal/maintenance/routines/{id}/status", internalMaintH.SetStatus)

	r.Route("/api", func(r chi.Router) {
		// Per-user visibility feed — JWT OPTIONAL (anonymous ⇒ everyone-flags).
		r.Route("/policy/features", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Get("/mine", publicH.GetMine)
		})
		// Admin CRUD — 401 (AuthMiddleware) then 403 (AdminRoleMiddleware).
		// Defense-in-depth: the gateway applies the same gates.
		r.Route("/admin/policy", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Use(AdminRoleMiddleware)
			r.Get("/flags", adminH.List)
			r.Put("/flags/{key}", adminH.SetFlag)
			r.Put("/roulette", adminH.SetRoulette)
			// Maintenance routines share the /admin/policy JWT+admin gate, so the
			// gateway needs NO new proxy route (/admin/policy/* already forwards here).
			r.Get("/maintenance/routines", adminMaintH.List)
			r.Put("/maintenance/routines/{id}", adminMaintH.SetRoutine)
		})
	})

	return r
}
