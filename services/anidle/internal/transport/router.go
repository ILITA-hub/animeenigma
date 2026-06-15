package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/handler"
)

func NewRouter(healthHandler *handler.HealthHandler, anidleHandler *handler.AnidleHandler, jwtCfg authz.JWTConfig, log *logger.Logger, mc *metrics.Collector) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(mc.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(middleware.RealIP)

	r.Get("/health", healthHandler.Health)
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	r.Route("/api/anidle", func(r chi.Router) {
		r.Use(OptionalAuthMiddleware(jwtCfg))
		r.Get("/daily", anidleHandler.DailyMeta)
		r.Post("/daily/guess", anidleHandler.DailyGuess)
		r.Post("/daily/giveup", anidleHandler.DailyGiveUp)
		r.Get("/search", anidleHandler.Search)
		r.Post("/endless/new", anidleHandler.EndlessNew)
		r.Post("/endless/guess", anidleHandler.EndlessGuess)
		r.Get("/stats", anidleHandler.Stats)
		r.Get("/leaderboard", anidleHandler.Leaderboard)
	})

	return r
}
