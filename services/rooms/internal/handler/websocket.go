package handler

import (
	"net/http"
	"net/url"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/service"
	"github.com/gorilla/websocket"
)

func buildOriginCheck(allowed []string) func(r *http.Request) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			set[u.Scheme+"://"+u.Host] = struct{}{}
		}
	}
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		_, ok := set[u.Scheme+"://"+u.Host]
		return ok
	}
}

func newUpgrader(allowed []string) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     buildOriginCheck(allowed),
	}
}

type WebSocketHandler struct {
	wsService  *service.WebSocketService
	log        *logger.Logger
	upgrader   websocket.Upgrader
	jwtManager *authz.JWTManager
}

func NewWebSocketHandler(wsService *service.WebSocketService, log *logger.Logger, allowedOrigins []string, jwtConfig authz.JWTConfig) *WebSocketHandler {
	return &WebSocketHandler{
		wsService:  wsService,
		log:        log,
		upgrader:   newUpgrader(allowedOrigins),
		jwtManager: authz.NewJWTManager(jwtConfig),
	}
}

// HandleWebSocket handles WebSocket connections for real-time game updates.
//
// Auth lives in the ?token= query param (validated pre-upgrade) because
// browsers cannot set an Authorization header on a native WebSocket upgrade —
// this mirrors the project-wide convention used by watch-together. The /ws
// route is therefore mounted OUTSIDE the header/cookie AuthMiddleware group.
func (h *WebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Pre-upgrade JWT validation. 401 BEFORE Upgrade so the failure shows as
	// a clean HTTP status in the browser network panel rather than a
	// successful upgrade followed by an immediate close.
	token := r.URL.Query().Get("token")
	if token == "" {
		h.log.Debugw("ws upgrade rejected: missing token")
		httputil.Unauthorized(w)
		return
	}
	claims, err := h.jwtManager.ValidateAccessToken(token)
	if err != nil {
		h.log.Debugw("ws upgrade rejected: invalid token", "error", err)
		httputil.Unauthorized(w)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Errorw("failed to upgrade connection", "error", err)
		return
	}

	metrics.WebSocketConnectionsActive.Inc()
	defer metrics.WebSocketConnectionsActive.Dec()

	h.wsService.HandleConnection(conn, r.Context(), claims.UserID, claims.Username)
}
