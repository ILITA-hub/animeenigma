package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	authHandler *handler.AuthHandler,
	telegramBotHandler *handler.TelegramBotHandler,
	userHandler *handler.UserHandler,
	sessionsHandler *handler.SessionsHandler,
	magicLinkHandler *handler.MagicLinkHandler,
	userResolveHandler *handler.UserResolveHandler,
	adminUsersHandler *handler.AdminUsersHandler,
	passkeyHandler *handler.PasskeyHandler,
	certHandler *handler.CertHandler,
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

	// Cross-domain SSO bridge (registered at root, not under /api, so the 302
	// reaches the browser rather than being consumed by the gateway's API prefix).
	r.Get("/magic-link-generate", magicLinkHandler.Generate)
	r.Get("/magic-link-login", magicLinkHandler.Login)

	// mTLS-vhost cert-login endpoints (root mux — NOT under /api, so the
	// gateway's API prefix never proxies them; only cert.animeenigma.org's
	// nginx location reaches HandshakeLogin from outside the Docker network).
	r.Get("/cert/handshake-login", certHandler.HandshakeLogin)
	r.Get("/cert/ca.pem", certHandler.CAPem)

	// Metrics endpoint
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// Admin check endpoint for nginx auth_request (outside /api for direct access)
	r.Get("/auth/admin-check", AdminCheckHandler(jwtConfig))

	// Internal endpoints (only reachable within Docker network)
	r.Post("/internal/resolve-api-key", authHandler.ResolveApiKey)

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Auth routes (public)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)
			r.Post("/guest", authHandler.GuestSession)
			r.Post("/telegram/deeplink", authHandler.DeepLink)
			r.Get("/telegram/check", authHandler.CheckDeepLink)
			r.Post("/telegram/webhook", telegramBotHandler.HandleWebhook)
			r.Post("/refresh", authHandler.RefreshToken)
			r.Post("/logout", authHandler.Logout)
			r.Post("/passkey/login/begin", passkeyHandler.LoginBegin)
			r.Post("/passkey/login/finish", passkeyHandler.LoginFinish)
			r.Post("/cert/consume", certHandler.Consume)
		})

		// Protected auth routes
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Get("/auth/me", userHandler.GetCurrentUser)
			r.Patch("/auth/profile", userHandler.UpdateCurrentUser)
			r.Put("/auth/profile/public-id", userHandler.UpdatePublicID)
			r.Put("/auth/profile/privacy", userHandler.UpdatePrivacy)
			r.Put("/auth/profile/activity-visibility", userHandler.UpdateActivityVisibility)
			r.Put("/auth/profile/avatar", userHandler.UpdateAvatar)
			r.Put("/auth/profile/timezone", userHandler.UpdateTimezone)
			r.Post("/auth/api-key", authHandler.GenerateApiKey)
			r.Delete("/auth/api-key", authHandler.RevokeApiKey)
			r.Get("/auth/api-key", authHandler.HasApiKey)
			r.Get("/auth/sessions", sessionsHandler.List)
			r.Delete("/auth/sessions/{id}", sessionsHandler.Revoke)
			r.Post("/auth/sessions/revoke-others", sessionsHandler.RevokeOthers)
			r.Post("/auth/passkey/register/begin", passkeyHandler.RegisterBegin)
			r.Post("/auth/passkey/register/finish", passkeyHandler.RegisterFinish)
			r.Get("/auth/passkeys", passkeyHandler.List)
			r.Delete("/auth/passkeys/{id}", passkeyHandler.Delete)
			r.Post("/auth/cert/issue", certHandler.Issue)
			r.Get("/auth/certs", certHandler.List)
			r.Get("/auth/certs/ca", certHandler.CAInfo)
			r.Delete("/auth/certs/{id}", certHandler.Revoke)
			r.Put("/auth/profile/cert-auto-login", certHandler.UpdateAutoLogin)
		})

		// Public profile by public_id
		r.Get("/auth/users/{publicId}", userHandler.GetUserByPublicID)

		// User routes
		r.Route("/users", func(r chi.Router) {
			// Protected routes
			r.Group(func(r chi.Router) {
				r.Use(AuthMiddleware(jwtConfig))
				r.Get("/me", userHandler.GetCurrentUser)
				r.Patch("/me", userHandler.UpdateCurrentUser)
			})

			// Public routes
			r.Get("/{userId}", userHandler.GetUser)
		})

		// Admin-only user management + canonical resolve endpoint. List/search
		// all users, change a user's role, and resolve a single identifier
		// (UUID/username/public_id/telegram_id) to the canonical record.
		r.Route("/admin/users", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Use(AdminMiddleware)
			r.Get("/", adminUsersHandler.List)
			r.Patch("/{id}/role", adminUsersHandler.UpdateRole)
			r.Get("/resolve", userResolveHandler.Resolve)
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

// AdminCheckHandler validates JWT and checks admin role for nginx auth_request
// Returns 200 if user is admin, 401 if no token, 403 if not admin
func AdminCheckHandler(jwtConfig authz.JWTConfig) http.HandlerFunc {
	jwtManager := authz.NewJWTManager(jwtConfig)

	return func(w http.ResponseWriter, r *http.Request) {
		token := httputil.BearerToken(r)
		if token == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		claims, err := jwtManager.ValidateAccessToken(token)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if claims.Role != authz.RoleAdmin {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
