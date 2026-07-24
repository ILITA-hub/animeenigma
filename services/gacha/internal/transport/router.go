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

// RequireAdmin is a middleware that rejects requests from non-admin users with 403.
// It runs AFTER AuthMiddleware (which puts claims on the context).
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authz.IsAdmin(r.Context()) {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NewRouter builds the chi router for the gacha service.
//
//	GET  /health                       (public)
//	GET  /metrics                      (public, prom)
//	POST /internal/gacha/credit        (internal — gateway never proxies)
//	GET  /internal/health              (internal)
//	GET  /api/gacha/images/*           (public — browser <img> tags, no JWT)
//	GET  /api/gacha/wallet             (JWT)
//	GET  /api/gacha/banners            (JWT)
//	POST /api/gacha/banners/{id}/pull  (JWT)
//	GET  /api/gacha/collection         (JWT)
//	     /api/gacha/admin/*            (JWT + AdminRole)
func NewRouter(
	walletHandler *handler.WalletHandler,
	internalHandler *handler.InternalHandler,
	adminHandler *handler.AdminHandler,
	imagesHandler *handler.ImagesHandler,
	pullHandler *handler.PullHandler,
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

	// All /api/gacha/* routes in a single Route block to avoid chi subtree
	// conflicts. The public images endpoint and the auth-gated wallet/admin
	// endpoints are separated by r.Group (not top-level r.Use), so the images
	// route is never covered by the JWT middleware.
	r.Route("/api/gacha", func(r chi.Router) {
		// Public card/banner art. Browsers load these via <img> (no JWT header
		// possible), so the route is unauthenticated by design: keys are
		// unguessable UUIDs and the content is anime character art.
		// The gateway forwards the full original path, so the service must
		// match /api/gacha/images/* here (not a bare /images/* route).
		r.Get("/images/*", imagesHandler.Serve)

		// JWT-gated user routes.
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Get("/wallet", walletHandler.GetWallet)
			// Phase 4 — daily claim with streak bonus.
			r.Post("/daily", walletHandler.ClaimDaily)

			// Player-facing pull engine (Phase 3).
			r.Get("/banners", pullHandler.Banners)
			r.Post("/banners/{id}/pull", pullHandler.Pull)
			r.Get("/collection", pullHandler.Collection)
		})

		// Admin content API. The gateway already requires JWT+AdminRole for
		// /api/gacha/admin/*; this service re-validates both (defense in depth,
		// same pattern as every service re-checking the gateway's JWT).
		r.Route("/admin", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Use(RequireAdmin)

			// Cards
			r.Post("/cards", adminHandler.CreateCard)
			r.Get("/cards", adminHandler.ListCards)
			// Bulk ops — static segments route before /cards/{id}.
			r.Patch("/cards/bulk", adminHandler.BulkUpdateCards)
			r.Post("/cards/bulk-delete", adminHandler.BulkDeleteCards)
			r.Get("/cards/{id}", adminHandler.GetCard)
			r.Patch("/cards/{id}", adminHandler.UpdateCard)
			r.Delete("/cards/{id}", adminHandler.DeleteCard)

			// Groups
			r.Post("/groups", adminHandler.CreateGroup)
			r.Get("/groups", adminHandler.ListGroups)
			r.Patch("/groups/{id}", adminHandler.RenameGroup)
			r.Delete("/groups/{id}", adminHandler.DeleteGroup)
			r.Post("/groups/{id}/cards", adminHandler.AddCardsToGroup)
			r.Delete("/groups/{id}/cards/{cardId}", adminHandler.RemoveCardFromGroup)

			// Banners
			r.Post("/banners", adminHandler.CreateBanner)
			r.Get("/banners", adminHandler.ListBanners)
			r.Get("/banners/{id}", adminHandler.GetBanner)
			r.Patch("/banners/{id}", adminHandler.UpdateBanner)
			r.Delete("/banners/{id}", adminHandler.DeleteBanner)
			r.Put("/banners/{id}/cards", adminHandler.SetBannerCards)
			r.Post("/banners/{id}/cards", adminHandler.AddBannerCards)
			r.Post("/banners/{id}/groups/{groupId}", adminHandler.AddGroupCardsToBanner)

			// Upload (file or URL → MinIO)
			r.Post("/upload", adminHandler.Upload)
		})
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
