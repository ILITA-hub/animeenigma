package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the chi router for the gacha service.
//
//	GET  /health                  (public)
//	GET  /metrics                 (public, prom)
//	POST /internal/gacha/credit   (internal — gateway never proxies)
//	GET  /internal/health         (internal)
//	GET  /api/gacha/wallet        (JWT)
func NewRouter(
	walletHandler *handler.WalletHandler,
	internalHandler *handler.InternalHandler,
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

	// Register GET + HEAD: the docker-compose healthcheck probes /health with
	// `wget --spider` (an HTTP HEAD). chi does NOT auto-route HEAD to a GET
	// handler, so a GET-only /health returns 405 to the probe and marks the
	// container permanently "unhealthy" (the latent bug behind notifications'
	// 9000+ failing-streak). Handling HEAD too makes the healthcheck pass.
	healthFn := func(w http.ResponseWriter, _ *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	}
	r.Get("/health", healthFn)
	r.Head("/health", healthFn)
	r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.Handler().ServeHTTP(w, req)
	})

	// Internal — no middleware, Docker-network-only.
	r.Post("/internal/gacha/credit", internalHandler.Credit)
	r.Get("/internal/health", internalHandler.Health)

	// Public — JWT-gated.
	r.Route("/api/gacha", func(r chi.Router) {
		r.Use(AuthMiddleware(jwtConfig))
		r.Get("/wallet", walletHandler.GetWallet)
	})

	return r
}

// AuthMiddleware validates the JWT access token and puts claims on the
// context. Copied from services/notifications/internal/transport/router.go
// (project convention — every service re-validates the gateway's JWT).
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
