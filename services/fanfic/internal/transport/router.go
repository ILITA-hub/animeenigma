package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/fanfic/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the chi router for the fanfic service.
//
//	GET    /health                     (public, GET+HEAD)
//	GET    /metrics                    (public, prom)
//	GET    /api/fanfic/daily           (optional-JWT) — "Фанфик дня" public reader
//	POST   /api/fanfic/generate        (JWT) — SSE
//	POST   /api/fanfic/{id}/continue   (JWT) — SSE, appends next part
//	GET    /api/fanfic                 (JWT) — list
//	GET    /api/fanfic/tags            (JWT) — curated tags (registered before /{id})
//	GET    /api/fanfic/{id}            (JWT)
//	DELETE /api/fanfic/{id}            (JWT)
//	GET    /internal/fanfic/daily          (docker-network only, no JWT) — compact spotlight DTO
//	POST   /internal/fanfic/ensure-daily   (docker-network only, no JWT) — scheduler cron hook
//
// dh may be nil (e.g. a caller that hasn't wired DailyService yet); the three
// daily/internal routes are only registered when it's non-nil, so a nil dh
// degrades to those routes 404ing instead of panicking.
func NewRouter(h *handler.Handler, dh *handler.DailyHandler, jwtConfig authz.JWTConfig, log *logger.Logger, mc *metrics.Collector) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(mc.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Register GET + HEAD: the docker-compose healthcheck probes /health with
	// `wget --spider` (an HTTP HEAD). chi does NOT auto-route HEAD to a GET
	// handler, so a GET-only /health would 405 the probe (project convention,
	// see gacha's router).
	r.Get("/health", handler.Health)
	r.Head("/health", handler.Health)
	r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.Handler().ServeHTTP(w, req)
	})

	if dh != nil {
		// Public daily reader — optional-JWT (NOT the mandatory AuthMiddleware
		// below, which 401s anon). Registered OUTSIDE the "/api/fanfic" JWT
		// group so anon readers get a 200, while OptionalAuth still attaches
		// claims to the context when a bearer IS present (Public needs to
		// distinguish anon vs logged-in for explicit-content gating).
		r.With(OptionalAuth(jwtConfig)).Get("/api/fanfic/daily", dh.Public)

		// Internal — docker-network only, gateway does not proxy /internal/*,
		// so these carry no auth middleware at all.
		r.Get("/internal/fanfic/daily", dh.Internal)
		r.Post("/internal/fanfic/ensure-daily", dh.Ensure)
	}

	r.Route("/api/fanfic", func(r chi.Router) {
		r.Use(AuthMiddleware(jwtConfig))
		r.Post("/generate", h.Generate)
		r.Post("/{id}/continue", h.Continue)
		r.Get("/", h.List)
		r.Get("/tags", h.Tags) // must be registered before /{id} or chi captures "tags" as the id param
		r.Get("/{id}", h.Get)
		r.Delete("/{id}", h.Delete)
	})

	return r
}

// AuthMiddleware validates the JWT access token and puts claims on the
// context. Copied from services/gacha/internal/transport/router.go (project
// convention — every service re-validates the gateway's JWT).
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

// OptionalAuth attaches JWT claims to the context when a bearer token is
// present and valid, but — unlike AuthMiddleware — never 401s: an absent or
// invalid token just falls through anonymously. Used ONLY for the public
// daily-fanfic reader, which must serve anon readers but still needs to tell
// anon apart from logged-in for explicit-content gating.
func OptionalAuth(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	jwtManager := authz.NewJWTManager(jwtConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token := httputil.BearerToken(r); token != "" {
				if claims, err := jwtManager.ValidateAccessToken(token); err == nil {
					r = r.WithContext(authz.ContextWithClaims(r.Context(), claims))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
