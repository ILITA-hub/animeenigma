package hub

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
)

// ----------------------------------------------------------------------------
// Timing constants for the gorilla/websocket pump pattern. Values mirror the
// canonical "chat" example from the gorilla docs and align with the design doc
// §WebSocket Protocol (30s ping / 60s pong deadline). Tuning rationale:
//
//   - pongWait must comfortably exceed pingPeriod so a single dropped pong
//     does not tear down a healthy connection (rule of thumb: pong = 2*ping).
//   - writeWait is the per-write deadline; 10s is generous given Redis and
//     the read pump being on separate goroutines.
//   - maxMessageSize caps inbound JSON envelopes at 64 KiB — well above any
//     legitimate payload (largest is chat with 500-char body ≈ 1 KiB) but
//     below the level where a malicious client could exhaust memory.
//   - sendBufferSize is the per-connection outbound buffer. 64 envelopes
//     covers a normal burst (room snapshot + member-joined fanout) without
//     ever blocking the hub. Slow consumers hitting full buffer trip the
//     non-blocking Send fast-path and increment wt_ws_messages_dropped_total.
// ----------------------------------------------------------------------------
const (
	pingPeriod     = 30 * time.Second
	pongWait       = 60 * time.Second
	writeWait      = 10 * time.Second
	maxMessageSize = 64 * 1024
	sendBufferSize = 64
)

// wsConn is the narrow subset of *gorilla/websocket.Conn the hub actually
// touches. Splitting it into an interface lets unit tests in hub_test.go
// swap in a fakeConn double that records WriteMessage calls without spinning
// up an httptest server + real upgrade — a pattern called out explicitly in
// the 01.3 plan §<action> ("fakeConn test double that records WriteMessage
// calls"). Real *websocket.Conn satisfies every method by signature.
type wsConn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	SetReadLimit(limit int64)
	SetPongHandler(h func(appData string) error)
	WriteControl(messageType int, data []byte, deadline time.Time) error
	Close() error
}

// Connection wraps a single websocket peer in a room. It owns two goroutines
// (readPump + writePump), a buffered outbound channel, and a once-only close
// path so concurrent failure modes (read error during a parallel Unregister)
// don't panic on double-close. The exported fields (UserID, Username, RoomID)
// are read-only after Register() and may be inspected by any goroutine.
//
// sendCh is intentionally unexported — outside callers must go through
// Connection.Send so the hub can enforce the non-blocking drop-on-full
// invariant and increment the dropped-message counter centrally.
type Connection struct {
	UserID   string
	Username string
	RoomID   string

	// conn is the wsConn-typed websocket transport. Stored as the interface
	// (not the concrete *websocket.Conn) so unit tests can substitute a fake.
	conn wsConn

	// sendCh is the per-connection outbound buffer. writePump drains it.
	sendCh chan []byte

	// hub is set by Hub.Register so the readPump can call hub.Unregister(c)
	// on read failure without the caller needing to manage the cleanup path.
	hub *Hub

	// OnMessage is the inbound-envelope dispatcher set by the WS-upgrade
	// wiring in 01.5/01.6. The hub itself never reads inbound envelopes —
	// the readPump unmarshals into an Envelope, bumps the receive counter,
	// and hands the result to OnMessage. May be nil in tests that only
	// exercise the outbound side; readPump tolerates that gracefully.
	OnMessage func(*Connection, domain.Envelope)

	// log is the per-connection logger reference (a thin shared pointer).
	log *logger.Logger

	// closed is signalled by the writePump exit (or by an explicit Close)
	// so other goroutines can short-circuit instead of blocking on sendCh.
	closed chan struct{}

	// closeOnce serializes the close path. Two readers (the readPump
	// failure path and an explicit Hub.Close) can race; only the first
	// reaches the cleanup body.
	closeOnce sync.Once
}

// newConnection is the package-private constructor used by Hub.Register.
// The hub fills in `hub` after this call. Tests that exercise Connection in
// isolation (Test 9: Send drops on full buffer) construct one directly.
func newConnection(roomID, userID, username string, conn wsConn, log *logger.Logger) *Connection {
	if log == nil {
		log = logger.Default()
	}
	return &Connection{
		UserID:   userID,
		Username: username,
		RoomID:   roomID,
		conn:     conn,
		sendCh:   make(chan []byte, sendBufferSize),
		log:      log,
		closed:   make(chan struct{}),
	}
}

// Send pushes pre-serialized envelope bytes onto the connection's outbound
// channel. The send is non-blocking: if sendCh is full (slow consumer) the
// message is dropped and false is returned. The hub increments
// wt_ws_messages_dropped_total when this happens — keeping it in Send itself
// avoids spreading the metric writes across the broadcast loop.
//
// Returns false if the connection is already closed or the buffer was full
// at the moment of the call. The caller (Hub.Broadcast / Hub.SendTo) treats
// drop and closed-connection identically: don't count this connection as a
// recipient, log a warning at the hub layer, move on.
func (c *Connection) Send(payload []byte) bool {
	select {
	case <-c.closed:
		return false
	default:
	}
	select {
	case c.sendCh <- payload:
		return true
	case <-c.closed:
		return false
	default:
		// sendCh is full — slow consumer. Increment the dropped counter
		// centrally so callers can stay metric-agnostic.
		MessagesDroppedTotal.Inc()
		return false
	}
}

// Close marks the connection as closed (idempotent) and closes the
// underlying websocket. The sendCh channel is intentionally NOT closed —
// closing it would race with concurrent Connection.Send writes from a
// broadcasting goroutine (close-during-send panics + the race detector
// flags the chan header field-write). Instead, the `closed` signal channel
// is the sole termination signal: Send selects on <-c.closed and returns
// false; writePump selects on <-c.closed and exits. Buffered messages
// already queued in sendCh are simply abandoned when both pumps exit.
func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
	})
}

// readPump is the inbound-side goroutine. Canonical gorilla pattern:
//
//   1. Cap the message size to prevent abuse.
//   2. Seed the read deadline; install a pong handler that bumps it.
//   3. Loop ReadMessage → unmarshal → dispatch.
//   4. On any error: call Hub.Unregister(c) (which calls Close) and return.
//
// The readPump assumes the connection has already been added to the hub's
// room set — Hub.Register is responsible for starting both pumps after the
// map insert is visible.
func (c *Connection) readPump(ctx context.Context) {
	defer func() {
		// Unregister is idempotent w.r.t. concurrent closes via closeOnce;
		// calling it from the read goroutine on error is the canonical
		// teardown path (see gorilla "chat" example).
		if c.hub != nil {
			c.hub.Unregister(c)
		} else {
			c.Close()
		}
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closed:
			return
		default:
		}

		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
				websocket.CloseNormalClosure) {
				c.log.Warnw("watch_together ws read error",
					"room_id", c.RoomID,
					"user_id", c.UserID,
					"err", err,
				)
			}
			return
		}

		var env domain.Envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			// Malformed envelope from the client — log and keep going. The
			// connection itself is fine; only this one frame is unusable.
			c.log.Warnw("watch_together ws inbound malformed",
				"room_id", c.RoomID,
				"user_id", c.UserID,
				"err", err,
			)
			continue
		}

		MessagesReceivedTotal.WithLabelValues(env.Type).Inc()

		if c.OnMessage != nil {
			c.OnMessage(c, env)
		}
	}
}

// writePump is the outbound-side goroutine. It drains sendCh onto the wire
// and emits a periodic ping to keep the connection alive. The ticker is
// pingPeriod (30s); pongWait (60s) on the read side will fire if the peer
// stops pong-replying. Exits when sendCh is closed (set by Connection.Close)
// or any wire error occurs.
func (c *Connection) writePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		// Ensure Close() runs at least once even if we exit via the wire
		// error path before the hub noticed. closeOnce keeps it idempotent.
		c.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closed:
			return
		case payload := <-c.sendCh:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				c.log.Warnw("watch_together ws write error",
					"room_id", c.RoomID,
					"user_id", c.UserID,
					"err", err,
				)
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(writeWait),
			); err != nil {
				return
			}
		}
	}
}
