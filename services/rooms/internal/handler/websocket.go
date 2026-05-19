package handler

import (
	"net/http"
	"net/url"

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
	wsService *service.WebSocketService
	log       *logger.Logger
	upgrader  websocket.Upgrader
}

func NewWebSocketHandler(wsService *service.WebSocketService, log *logger.Logger, allowedOrigins []string) *WebSocketHandler {
	return &WebSocketHandler{
		wsService: wsService,
		log:       log,
		upgrader:  newUpgrader(allowedOrigins),
	}
}

// HandleWebSocket handles WebSocket connections for real-time game updates
func (h *WebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Errorw("failed to upgrade connection", "error", err)
		return
	}

	metrics.WebSocketConnectionsActive.Inc()
	defer metrics.WebSocketConnectionsActive.Dec()

	h.wsService.HandleConnection(conn, r.Context())
}
