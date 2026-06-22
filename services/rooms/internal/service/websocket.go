package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/domain"
	"github.com/gorilla/websocket"
)

// ----------------------------------------------------------------------------
// Keepalive timing defaults for the gorilla/websocket read/ping pattern.
// Mirrors services/watch-together/internal/hub/connection.go:
//
//   - pongWait must comfortably exceed pingPeriod so a single dropped pong
//     does not tear down a healthy connection (rule of thumb: pong = 2*ping).
//   - writeWait is the per-write deadline for the ping control frame.
//   - maxMessageSize caps inbound JSON envelopes well above any legitimate
//     payload but below the level where a malicious client could exhaust
//     memory by streaming a giant frame into ReadJSON.
//
// Without a read deadline + pong handler, a half-open connection parks the
// read loop on ReadJSON forever — the deferred conn.Close and the shared
// WebSocketConnectionsActive gauge Dec never run (the goroutine leaks).
// ----------------------------------------------------------------------------
const (
	defaultPingPeriod     = 30 * time.Second
	defaultPongWait       = 60 * time.Second
	defaultWriteWait      = 10 * time.Second
	defaultMaxMessageSize = int64(8 * 1024)
)

type WebSocketService struct {
	log *logger.Logger

	// Per-service keepalive timings. Set once in the constructor and never
	// mutated after, so production reads are race-free. The connection-
	// lifecycle test overrides them on its own instance (via setTimingsForTest)
	// to keep the half-open-teardown assertion fast instead of waiting out a
	// real 60s pongWait — no shared global state, no concurrent write.
	pingPeriod     time.Duration
	pongWait       time.Duration
	writeWait      time.Duration
	maxMessageSize int64
}

func NewWebSocketService(log *logger.Logger) *WebSocketService {
	return &WebSocketService{
		log:            log,
		pingPeriod:     defaultPingPeriod,
		pongWait:       defaultPongWait,
		writeWait:      defaultWriteWait,
		maxMessageSize: defaultMaxMessageSize,
	}
}

// HandleConnection handles a WebSocket connection for real-time game updates.
//
// userID/username are the authenticated identity threaded from the WS-upgrade
// handler (validated from the ?token= query param). Inbound messages are
// bound to this identity so handlers can attribute actions to a real user
// rather than echoing a static success.
//
// The read loop is guarded by a read deadline + pong handler and a periodic
// ping goroutine so a half-open peer is detected within pongWait and torn
// down — releasing the goroutine and the shared connection gauge instead of
// leaking them.
func (s *WebSocketService) HandleConnection(conn *websocket.Conn, ctx context.Context, userID, username string) {
	defer conn.Close()

	s.log.Infow("WebSocket connection established", "user_id", userID, "username", username)

	// Send welcome message.
	welcomeMsg := domain.WSMessage{
		Type:    "connected",
		Payload: map[string]string{"message": "Connected to game server"},
	}

	if err := conn.WriteJSON(welcomeMsg); err != nil {
		s.log.Errorw("failed to send welcome message", "error", err, "user_id", userID)
		return
	}

	// ctx is cancelled when the read loop exits (any read error / deadline
	// breach), which stops the ping goroutine below; the ping goroutine in
	// turn closes the conn on a write error so a stuck read also unwinds.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Inbound size cap + read deadline + pong handler. Each pong extends the
	// deadline; the ping goroutine keeps a well-behaved peer ponging.
	conn.SetReadLimit(s.maxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(s.pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(s.pongWait))
	})

	// Ping goroutine: emits a ping every pingPeriod and exits on ctx.Done().
	// On a write failure (dead peer) it closes the conn so the read loop's
	// blocked ReadJSON returns immediately rather than waiting out pongWait.
	go func() {
		ticker := time.NewTicker(s.pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(s.writeWait))
				if err := conn.WriteControl(
					websocket.PingMessage,
					nil,
					time.Now().Add(s.writeWait),
				); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()

	// Listen for messages.
	for {
		var msg domain.WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.log.Errorw("WebSocket error", "error", err, "user_id", userID)
			}
			break
		}

		// Refresh the read deadline after every successful frame so an active
		// (but pong-quiet) client isn't disconnected mid-conversation.
		_ = conn.SetReadDeadline(time.Now().Add(s.pongWait))

		s.handleMessage(conn, &msg, userID, username)
	}

	s.log.Infow("WebSocket connection closed", "user_id", userID)
}

func (s *WebSocketService) handleMessage(conn *websocket.Conn, msg *domain.WSMessage, userID, username string) {
	switch msg.Type {
	case "join_room":
		s.handleJoinRoom(conn, msg, userID, username)
	case "submit_answer":
		s.handleSubmitAnswer(conn, msg, userID)
	case "ready":
		s.handleReady(conn, msg, userID)
	default:
		s.log.Warnw("unknown message type", "type", msg.Type, "user_id", userID)
	}
}

func (s *WebSocketService) handleJoinRoom(conn *websocket.Conn, msg *domain.WSMessage, userID, username string) {
	response := domain.WSMessage{
		Type: "room_joined",
		Payload: map[string]string{
			"status":   "success",
			"user_id":  userID,
			"username": username,
		},
	}
	_ = conn.WriteJSON(response)
}

func (s *WebSocketService) handleSubmitAnswer(conn *websocket.Conn, msg *domain.WSMessage, userID string) {
	response := domain.WSMessage{
		Type: "answer_submitted",
		Payload: map[string]string{
			"status":  "success",
			"user_id": userID,
		},
	}
	_ = conn.WriteJSON(response)
}

func (s *WebSocketService) handleReady(conn *websocket.Conn, msg *domain.WSMessage, userID string) {
	response := domain.WSMessage{
		Type: "ready_confirmed",
		Payload: map[string]string{
			"status":  "ready",
			"user_id": userID,
		},
	}
	_ = conn.WriteJSON(response)
}

// BroadcastToRoom sends a message to all players in a room
func (s *WebSocketService) BroadcastToRoom(roomID string, msg *domain.WSMessage) {
	// In a real implementation, this would broadcast to all connected clients in the room
	data, _ := json.Marshal(msg)
	s.log.Infow("broadcasting to room", "room_id", roomID, "message", string(data))
}
