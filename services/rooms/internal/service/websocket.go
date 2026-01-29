package service

import (
	"context"
	"encoding/json"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/domain"
	"github.com/gorilla/websocket"
)

type WebSocketService struct {
	log *logger.Logger
}

func NewWebSocketService(log *logger.Logger) *WebSocketService {
	return &WebSocketService{
		log: log,
	}
}

// HandleConnection handles a WebSocket connection for real-time game updates
func (s *WebSocketService) HandleConnection(conn *websocket.Conn, ctx context.Context) {
	defer conn.Close()

	s.log.Info("WebSocket connection established")

	// Send welcome message
	welcomeMsg := domain.WSMessage{
		Type:    "connected",
		Payload: map[string]string{"message": "Connected to game server"},
	}

	if err := conn.WriteJSON(welcomeMsg); err != nil {
		s.log.Errorw("failed to send welcome message", "error", err)
		return
	}

	// Listen for messages
	for {
		var msg domain.WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.log.Errorw("WebSocket error", "error", err)
			}
			break
		}

		s.handleMessage(conn, &msg)
	}

	s.log.Info("WebSocket connection closed")
}

func (s *WebSocketService) handleMessage(conn *websocket.Conn, msg *domain.WSMessage) {
	switch msg.Type {
	case "join_room":
		s.handleJoinRoom(conn, msg)
	case "submit_answer":
		s.handleSubmitAnswer(conn, msg)
	case "ready":
		s.handleReady(conn, msg)
	default:
		s.log.Warnw("unknown message type", "type", msg.Type)
	}
}

func (s *WebSocketService) handleJoinRoom(conn *websocket.Conn, msg *domain.WSMessage) {
	response := domain.WSMessage{
		Type:    "room_joined",
		Payload: map[string]string{"status": "success"},
	}
	_ = conn.WriteJSON(response)
}

func (s *WebSocketService) handleSubmitAnswer(conn *websocket.Conn, msg *domain.WSMessage) {
	response := domain.WSMessage{
		Type:    "answer_submitted",
		Payload: map[string]string{"status": "success"},
	}
	_ = conn.WriteJSON(response)
}

func (s *WebSocketService) handleReady(conn *websocket.Conn, msg *domain.WSMessage) {
	response := domain.WSMessage{
		Type:    "ready_confirmed",
		Payload: map[string]string{"status": "ready"},
	}
	_ = conn.WriteJSON(response)
}

// BroadcastToRoom sends a message to all players in a room
func (s *WebSocketService) BroadcastToRoom(roomID string, msg *domain.WSMessage) {
	// In a real implementation, this would broadcast to all connected clients in the room
	data, _ := json.Marshal(msg)
	s.log.Infow("broadcasting to room", "room_id", roomID, "message", string(data))
}
