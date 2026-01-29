package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/config"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	streamHandler *handler.StreamHandler,
	uploadHandler *handler.UploadHandler,
	cfg *config.Config,
	log *logger.Logger,
) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		// Public streaming routes (token validated in handler)
		r.Route("/stream", func(r chi.Router) {
			r.Get("/proxy", streamHandler.ProxyStream)
			r.Get("/direct", streamHandler.DirectStream)
		})

		// Internal API for token generation (service-to-service)
		r.Route("/internal", func(r chi.Router) {
			// In production, add service-to-service auth here
			r.Post("/token", streamHandler.GenerateToken)
		})

		// Admin routes for upload
		r.Route("/admin", func(r chi.Router) {
			r.Use(AuthMiddleware(cfg.JWT))
			r.Use(AdminMiddleware)

			r.Post("/upload", uploadHandler.UploadVideo)
			r.Post("/upload/url", uploadHandler.GetUploadURL)
			r.Delete("/video", uploadHandler.DeleteVideo)
		})
	})

	return r
}

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

func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authz.IsAdmin(r.Context()) {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
