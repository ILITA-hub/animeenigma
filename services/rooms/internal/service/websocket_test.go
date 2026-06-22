package service

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/gorilla/websocket"
)

// wsTestServer mounts HandleConnection behind a real websocket upgrade so the
// connection-lifecycle tests exercise the actual gorilla read loop + ping
// goroutine. done is closed when HandleConnection returns (i.e. the server
// side fully tore the connection down), letting tests assert teardown timing.
//
// ping/pong/write tighten the per-service keepalive timings on a fresh service
// instance (no shared global) so the half-open-teardown assertion completes in
// well under a second instead of the production 60s pongWait. Pass 0 for any
// of them to keep the production default.
func wsTestServer(t *testing.T, ping, pong, write time.Duration) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	svc := NewWebSocketService(logger.Default())
	if ping > 0 {
		svc.pingPeriod = ping
	}
	if pong > 0 {
		svc.pongWait = pong
	}
	if write > 0 {
		svc.writeWait = write
	}
	done := make(chan struct{})
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		svc.HandleConnection(conn, r.Context(), "user-1", "alice")
		close(done)
	}))
	t.Cleanup(srv.Close)
	return srv, done
}

func dialWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// Finding L746, assertion (1): a client that connects and then goes silent
// (never answering pings, the gorilla default control handler still auto-pongs
// — so to truly simulate half-open we install a no-op ping handler client-side
// AND stop reading). The server's read deadline must fire and HandleConnection
// must return within ~pongWait. Without SetReadDeadline the ReadJSON parks
// forever and this times out.
func TestHandleConnection_HalfOpenTeardownByReadDeadline(t *testing.T) {
	srv, done := wsTestServer(t, 200*time.Millisecond, 500*time.Millisecond, 200*time.Millisecond)
	conn := dialWS(t, srv)

	// Drain the welcome frame, then suppress automatic pong replies so the
	// server's read deadline is never refreshed — a true half-open peer.
	conn.SetPingHandler(func(string) error { return nil })
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read welcome: %v", err)
	}
	// Stop touching the connection entirely.

	select {
	case <-done:
		// Server tore the connection down via the read deadline. Good.
	case <-time.After(3 * time.Second):
		t.Fatal("HandleConnection did not return after read deadline expired — half-open connection leaked")
	}
}

// Finding L746, assertion (2): an oversized inbound frame must break the read
// loop (SetReadLimit), causing HandleConnection to return rather than allocate
// the giant payload.
func TestHandleConnection_OversizedFrameRejected(t *testing.T) {
	srv, done := wsTestServer(t, 200*time.Millisecond, 2*time.Second, 200*time.Millisecond)
	conn := dialWS(t, srv)

	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read welcome: %v", err)
	}

	// Write a text frame larger than maxMessageSize (8 KiB).
	huge := strings.Repeat("x", int(defaultMaxMessageSize)+1024)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"join_room","payload":"`+huge+`"}`)); err != nil {
		t.Fatalf("write huge frame: %v", err)
	}

	select {
	case <-done:
		// Read loop exited on the read-limit breach. Good.
	case <-time.After(3 * time.Second):
		t.Fatal("HandleConnection did not return after oversized frame — SetReadLimit not enforced")
	}
}

// Finding L746, assertion (3): a well-behaved client that answers pings (the
// gorilla default ping handler auto-pongs) stays connected past pongWait. We
// keep the connection in a ReadMessage loop (so control frames are processed)
// and confirm HandleConnection has NOT returned after > pongWait.
func TestHandleConnection_HealthyClientSurvivesPastPongWait(t *testing.T) {
	srv, done := wsTestServer(t, 100*time.Millisecond, 300*time.Millisecond, 200*time.Millisecond)
	conn := dialWS(t, srv)

	// Pump reads in the background so the client's default ping handler runs
	// and auto-pongs, refreshing the server's read deadline.
	readErr := make(chan error, 1)
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				readErr <- err
				return
			}
		}
	}()

	// Wait well past pongWait (300ms): a healthy connection must still be up.
	select {
	case <-done:
		t.Fatal("HandleConnection returned early — healthy ponging client was disconnected")
	case err := <-readErr:
		t.Fatalf("client read error on a healthy connection: %v", err)
	case <-time.After(1 * time.Second):
		// Still connected after ~3x pongWait. Good. Close to clean up.
		_ = conn.Close()
	}
}

// Finding L760: handleJoinRoom binds the response to the authenticated user
// rather than emitting a static success. End-to-end through the read loop.
func TestHandleConnection_JoinRoomBindsAuthenticatedUser(t *testing.T) {
	srv, _ := wsTestServer(t, 200*time.Millisecond, 2*time.Second, 200*time.Millisecond)
	conn := dialWS(t, srv)

	// welcome
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read welcome: %v", err)
	}

	if err := conn.WriteJSON(map[string]any{"type": "join_room"}); err != nil {
		t.Fatalf("write join_room: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp struct {
		Type    string            `json:"type"`
		Payload map[string]string `json:"payload"`
	}
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("read join response: %v", err)
	}
	if resp.Type != "room_joined" {
		t.Fatalf("type = %q, want room_joined", resp.Type)
	}
	if resp.Payload["user_id"] != "user-1" {
		t.Errorf("response not identity-bound: user_id = %q, want user-1", resp.Payload["user_id"])
	}
	if resp.Payload["username"] != "alice" {
		t.Errorf("response not identity-bound: username = %q, want alice", resp.Payload["username"])
	}
}
