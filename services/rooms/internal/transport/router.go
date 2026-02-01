package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	roomHandler *handler.RoomHandler,
	wsHandler *handler.WebSocketHandler,
	leaderboardHandler *handler.LeaderboardHandler,
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

	// Metrics endpoint
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))

			// Room routes
			r.Route("/rooms", func(r chi.Router) {
				r.Get("/", roomHandler.ListRooms)
				r.Post("/", roomHandler.CreateRoom)
				r.Get("/{roomId}", roomHandler.GetRoom)
				r.Post("/{roomId}/join", roomHandler.JoinRoom)
				r.Post("/{roomId}/leave", roomHandler.LeaveRoom)
			})

			// WebSocket endpoint
			r.Get("/ws", wsHandler.HandleWebSocket)

			// Leaderboard routes
			r.Get("/leaderboard", leaderboardHandler.GetLeaderboard)
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
