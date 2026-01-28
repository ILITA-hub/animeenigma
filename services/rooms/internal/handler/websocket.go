package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/service"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

type WebSocketHandler struct {
	wsService *service.WebSocketService
	log       *logger.Logger
}

func NewWebSocketHandler(wsService *service.WebSocketService, log *logger.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		wsService: wsService,
		log:       log,
	}
}

// HandleWebSocket handles WebSocket connections for real-time game updates
func (h *WebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Errorw("failed to upgrade connection", "error", err)
		return
	}

	h.wsService.HandleConnection(conn, r.Context())
}
