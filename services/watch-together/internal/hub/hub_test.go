package hub

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/prometheus/client_model/go"
	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
)

// ----------------------------------------------------------------------------
// Fake websocket connection — satisfies wsConn so the hub can manage it
// without a real network upgrade. Records every WriteMessage payload onto an
// internal slice for assertions; the read side parks until Close is called
// so the hub's readPump doesn't tear down the connection during a test.
// ----------------------------------------------------------------------------

type fakeConn struct {
	mu      sync.Mutex
	writes  [][]byte
	pingCt  int
	closed  chan struct{}
	closeMu sync.Once
}

func newFakeConn() *fakeConn {
	return &fakeConn{closed: make(chan struct{})}
}

func (f *fakeConn) WriteMessage(messageType int, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Copy to defeat caller reuse of the buffer.
	cp := make([]byte, len(data))
	copy(cp, data)
	f.writes = append(f.writes, cp)
	return nil
}

func (f *fakeConn) ReadMessage() (int, []byte, error) {
	// Block until Close. Real connections sit in a read syscall most of
	// their life; we emulate that without consuming CPU.
	<-f.closed
	// Returning a generic close-ish error keeps the readPump's
	// IsUnexpectedCloseError check quiet (it intentionally tolerates
	// the close family).
	return 0, nil, &fakeNetError{}
}

func (f *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(time.Time) error { return nil }
func (f *fakeConn) SetReadLimit(int64)               {}
func (f *fakeConn) SetPongHandler(func(string) error) {}

func (f *fakeConn) WriteControl(messageType int, _ []byte, _ time.Time) error {
	// Ping/Close control frames go through here in production. Count them
	// so the keepalive test can observe at least one ping.
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pingCt++
	return nil
}

func (f *fakeConn) Close() error {
	f.closeMu.Do(func() { close(f.closed) })
	return nil
}

// payloads returns a copy of every WriteMessage payload so assertions don't
// race with the writePump still pushing onto the slice.
func (f *fakeConn) payloads() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.writes))
	for i, p := range f.writes {
		cp := make([]byte, len(p))
		copy(cp, p)
		out[i] = cp
	}
	return out
}

// fakeNetError implements net.Error so the gorilla websocket error checks
// classify it as a non-unexpected close (no log noise during tests).
type fakeNetError struct{}

func (e *fakeNetError) Error() string   { return "fake conn closed" }
func (e *fakeNetError) Timeout() bool   { return false }
func (e *fakeNetError) Temporary() bool { return false }

// ----------------------------------------------------------------------------
// Test helpers — boot a Hub backed by a fresh miniredis. The repo is a real
// RoomRepo so pubsub round-trips exercise the production code path.
// ----------------------------------------------------------------------------

func newTestHub(t *testing.T) (*Hub, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	log := logger.Default()
	r := repo.NewRoomRepo(client, 900*time.Second, log)
	h := NewHub(r, log, "test-instance-aaa")
	t.Cleanup(h.Close)
	return h, mr, client
}

// registerFake attaches a fakeConn to the hub via the package-private
// registerInternal entry point. The exported Register requires a real
// *websocket.Conn; tests use this lower-level seam.
func registerFake(t *testing.T, h *Hub, roomID, userID, username string) (*Connection, *fakeConn) {
	t.Helper()
	fc := newFakeConn()
	c := h.registerInternal(roomID, userID, username, fc)
	return c, fc
}

// waitForPayload polls fc.payloads until at least minCount entries appear
// or the timeout elapses. Returns the captured payload slice.
func waitForPayload(t *testing.T, fc *fakeConn, minCount int, timeout time.Duration) [][]byte {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got := fc.payloads()
		if len(got) >= minCount {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d payloads; got %d", minCount, len(got))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func sampleEnvelope(t *testing.T, msgType string, body any) domain.Envelope {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return domain.Envelope{Type: msgType, Data: raw}
}

// ----------------------------------------------------------------------------
// Behavior 1 — NewHub returns a non-nil hub with an empty room map.
// ----------------------------------------------------------------------------

func TestNewHub_EmptyRoomMap(t *testing.T) {
	h, _, _ := newTestHub(t)
	if h == nil {
		t.Fatal("NewHub returned nil")
	}
	if got := h.MemberCount("any-room"); got != 0 {
		t.Fatalf("MemberCount on empty hub = %d, want 0", got)
	}
	if got := h.MemberUserIDs("any-room"); len(got) != 0 {
		t.Fatalf("MemberUserIDs on empty hub = %v, want empty", got)
	}
	if h.InstanceID() == "" {
		t.Fatal("InstanceID is empty")
	}
}

// ----------------------------------------------------------------------------
// Behavior 2 — Register adds a connection; MemberCount increments to 1.
// ----------------------------------------------------------------------------

func TestRegister_IncrementsMemberCount(t *testing.T) {
	h, _, _ := newTestHub(t)
	_, _ = registerFake(t, h, "room-A", "alice", "Alice")

	if got := h.MemberCount("room-A"); got != 1 {
		t.Fatalf("MemberCount after Register = %d, want 1", got)
	}
}

// ----------------------------------------------------------------------------
// Behavior 3 — two Registers in the same room → MemberCount=2,
// MemberUserIDs deduplicated.
// ----------------------------------------------------------------------------

func TestRegister_TwoUsersSameRoom(t *testing.T) {
	h, _, _ := newTestHub(t)
	_, _ = registerFake(t, h, "room-A", "alice", "Alice")
	_, _ = registerFake(t, h, "room-A", "bob", "Bob")

	if got := h.MemberCount("room-A"); got != 2 {
		t.Fatalf("MemberCount = %d, want 2", got)
	}
	ids := h.MemberUserIDs("room-A")
	if len(ids) != 2 {
		t.Fatalf("MemberUserIDs len = %d, want 2", len(ids))
	}
	idSet := map[string]bool{ids[0]: true, ids[1]: true}
	if !idSet["alice"] || !idSet["bob"] {
		t.Fatalf("MemberUserIDs = %v, want [alice bob]", ids)
	}
}

// ----------------------------------------------------------------------------
// Behavior 4 — same user with 2 connections (multi-tab): MemberCount=2,
// MemberUserIDs has 1 entry.
// ----------------------------------------------------------------------------

func TestRegister_MultiTabSameUser(t *testing.T) {
	h, _, _ := newTestHub(t)
	_, _ = registerFake(t, h, "room-A", "alice", "Alice")
	_, _ = registerFake(t, h, "room-A", "alice", "Alice")

	if got := h.MemberCount("room-A"); got != 2 {
		t.Fatalf("MemberCount = %d, want 2 (multi-tab counted by connection)", got)
	}
	ids := h.MemberUserIDs("room-A")
	if len(ids) != 1 || ids[0] != "alice" {
		t.Fatalf("MemberUserIDs = %v, want [alice]", ids)
	}
}

// ----------------------------------------------------------------------------
// Behavior 5 — Unregister removes a connection; MemberCount decrements.
// ----------------------------------------------------------------------------

func TestUnregister_DecrementsMemberCount(t *testing.T) {
	h, _, _ := newTestHub(t)
	c, _ := registerFake(t, h, "room-A", "alice", "Alice")
	_, _ = registerFake(t, h, "room-A", "bob", "Bob")

	h.Unregister(c)
	if got := h.MemberCount("room-A"); got != 1 {
		t.Fatalf("MemberCount after Unregister = %d, want 1", got)
	}
	ids := h.MemberUserIDs("room-A")
	if len(ids) != 1 || ids[0] != "bob" {
		t.Fatalf("MemberUserIDs after Unregister(alice) = %v, want [bob]", ids)
	}
}

// ----------------------------------------------------------------------------
// Behavior 6 — Broadcast with empty excludeUserID delivers to all members.
// ----------------------------------------------------------------------------

func TestBroadcast_AllRecipients(t *testing.T) {
	h, _, _ := newTestHub(t)
	_, fcA := registerFake(t, h, "room-A", "alice", "Alice")
	_, fcB := registerFake(t, h, "room-A", "bob", "Bob")

	env := sampleEnvelope(t, domain.MsgRoomSnapshot, map[string]any{"foo": "bar"})
	delivered, err := h.Broadcast(context.Background(), "room-A", env, "")
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if delivered != 2 {
		t.Fatalf("delivered = %d, want 2", delivered)
	}

	got := waitForPayload(t, fcA, 1, time.Second)
	if len(got) != 1 {
		t.Fatalf("alice got %d messages, want 1", len(got))
	}
	got = waitForPayload(t, fcB, 1, time.Second)
	if len(got) != 1 {
		t.Fatalf("bob got %d messages, want 1", len(got))
	}

	// Decode + assert the payload roundtrips correctly.
	var decoded domain.Envelope
	if err := json.Unmarshal(got[0], &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Type != domain.MsgRoomSnapshot {
		t.Fatalf("decoded.Type = %q, want %q", decoded.Type, domain.MsgRoomSnapshot)
	}
}

// ----------------------------------------------------------------------------
// Behavior 7 — Broadcast with excludeUserID="alice" skips alice but delivers
// to bob.
// ----------------------------------------------------------------------------

func TestBroadcast_ExcludeSender(t *testing.T) {
	h, _, _ := newTestHub(t)
	_, fcA := registerFake(t, h, "room-A", "alice", "Alice")
	_, fcB := registerFake(t, h, "room-A", "bob", "Bob")

	env := sampleEnvelope(t, domain.MsgPlaybackEvent, map[string]any{"kind": "play", "time": 1.0})
	delivered, err := h.Broadcast(context.Background(), "room-A", env, "alice")
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if delivered != 1 {
		t.Fatalf("delivered = %d, want 1 (alice excluded)", delivered)
	}

	got := waitForPayload(t, fcB, 1, time.Second)
	if len(got) != 1 {
		t.Fatalf("bob got %d messages, want 1", len(got))
	}

	// Alice MUST NOT have received anything. Give the writePump a moment
	// to drain so a delayed delivery would show up if the exclude was
	// broken.
	time.Sleep(50 * time.Millisecond)
	if got := fcA.payloads(); len(got) != 0 {
		t.Fatalf("alice received %d payloads despite being excluded: %v", len(got), got)
	}
}

// ----------------------------------------------------------------------------
// Behavior 8 — SendTo("alice") delivers only to alice.
// ----------------------------------------------------------------------------

func TestSendTo_TargetsOneUser(t *testing.T) {
	h, _, _ := newTestHub(t)
	_, fcA := registerFake(t, h, "room-A", "alice", "Alice")
	_, fcB := registerFake(t, h, "room-A", "bob", "Bob")

	env := sampleEnvelope(t, domain.MsgPlaybackCorrection, map[string]any{"time": 1.5})
	delivered, err := h.SendTo(context.Background(), "room-A", "alice", env)
	if err != nil {
		t.Fatalf("SendTo: %v", err)
	}
	if delivered != 1 {
		t.Fatalf("delivered = %d, want 1", delivered)
	}

	got := waitForPayload(t, fcA, 1, time.Second)
	if len(got) != 1 {
		t.Fatalf("alice got %d messages, want 1", len(got))
	}

	time.Sleep(50 * time.Millisecond)
	if got := fcB.payloads(); len(got) != 0 {
		t.Fatalf("bob received %d payloads despite being non-target: %v", len(got), got)
	}
}

// ----------------------------------------------------------------------------
// Behavior 8b — SendTo to a user with 2 connections delivers to both.
// ----------------------------------------------------------------------------

func TestSendTo_MultiTabUser(t *testing.T) {
	h, _, _ := newTestHub(t)
	_, fc1 := registerFake(t, h, "room-A", "alice", "Alice")
	_, fc2 := registerFake(t, h, "room-A", "alice", "Alice")

	env := sampleEnvelope(t, domain.MsgError, map[string]any{"code": "X"})
	delivered, err := h.SendTo(context.Background(), "room-A", "alice", env)
	if err != nil {
		t.Fatalf("SendTo: %v", err)
	}
	if delivered != 2 {
		t.Fatalf("delivered = %d, want 2", delivered)
	}
	waitForPayload(t, fc1, 1, time.Second)
	waitForPayload(t, fc2, 1, time.Second)
}

// ----------------------------------------------------------------------------
// Behavior 9 — Connection.Send returns false when sendCh is full;
// dropped-message metric increments.
// ----------------------------------------------------------------------------

func TestConnection_Send_DropsOnFullBuffer(t *testing.T) {
	// Build a Connection bypassing the writePump so the buffer stays full.
	fc := newFakeConn()
	t.Cleanup(func() { _ = fc.Close() })

	c := newConnection("room-A", "alice", "Alice", fc, logger.Default())
	// DO NOT start the pumps — that way sendCh accumulates.

	// Fill to capacity.
	for i := 0; i < sendBufferSize; i++ {
		if !c.Send([]byte("x")) {
			t.Fatalf("Send returned false before buffer was full at i=%d", i)
		}
	}

	before := readDroppedTotal(t)
	if c.Send([]byte("overflow")) {
		t.Fatal("Send returned true on overflow; expected false")
	}
	after := readDroppedTotal(t)
	if after-before < 1 {
		t.Fatalf("MessagesDroppedTotal increment = %f, want >= 1", after-before)
	}
}

// readDroppedTotal pulls the current MessagesDroppedTotal counter value.
// Counter has no exported Get; gather via the prometheus DTO.
func readDroppedTotal(t *testing.T) float64 {
	t.Helper()
	var m dto.Metric
	if err := MessagesDroppedTotal.Write(&m); err != nil {
		t.Fatalf("write counter: %v", err)
	}
	if m.Counter == nil || m.Counter.Value == nil {
		return 0
	}
	return *m.Counter.Value
}

// ----------------------------------------------------------------------------
// Behavior 10 — Broadcast publishes to wt:room:{id}:events Redis channel.
// We attach a sniffer subscriber on a parallel client (bypassing the hub's
// own subscriber loop) and assert it observes the published frame.
// ----------------------------------------------------------------------------

func TestBroadcast_PublishesToPubsub(t *testing.T) {
	h, _, client := newTestHub(t)
	_, _ = registerFake(t, h, "room-A", "alice", "Alice")

	// Side subscriber on the same miniredis. miniredis needs a tick to
	// actually wire the SUBSCRIBE before we publish; the bytes-received
	// channel guarantees we only publish once subscribed.
	sniffer := client.Subscribe(context.Background(), "wt:room:room-A:events")
	t.Cleanup(func() { _ = sniffer.Close() })

	// Block until SUBSCRIBE is acknowledged.
	if _, err := sniffer.Receive(context.Background()); err != nil {
		t.Fatalf("sniffer Receive (subscribe ack): %v", err)
	}

	env := sampleEnvelope(t, domain.MsgRoomSnapshot, map[string]any{"v": 1})
	if _, err := h.Broadcast(context.Background(), "room-A", env, ""); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msg, err := sniffer.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("sniffer.ReceiveMessage: %v", err)
	}
	if msg.Channel != "wt:room:room-A:events" {
		t.Fatalf("channel = %q, want wt:room:room-A:events", msg.Channel)
	}

	var frame pubsubFrame
	if err := json.Unmarshal([]byte(msg.Payload), &frame); err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	if frame.InstanceID != h.InstanceID() {
		t.Fatalf("frame.InstanceID = %q, want %q", frame.InstanceID, h.InstanceID())
	}
	if frame.Env.Type != domain.MsgRoomSnapshot {
		t.Fatalf("frame.Env.Type = %q, want %q", frame.Env.Type, domain.MsgRoomSnapshot)
	}
}

// ----------------------------------------------------------------------------
// Behavior 11 — Pubsub subscriber drops messages with our own instanceID
// (single-instance own-echo defense).
// ----------------------------------------------------------------------------

func TestPubsubSubscriber_DropsOwnEcho(t *testing.T) {
	h, _, _ := newTestHub(t)
	_, fcA := registerFake(t, h, "room-A", "alice", "Alice")

	// Inject a synthetic pubsub message tagged with our own instanceID
	// directly into handlePubsubMessage — bypasses Redis (we already test
	// the Redis publish path in Behavior 10) and isolates the dedup logic.
	frame := pubsubFrame{
		InstanceID: h.InstanceID(),
		Env:        sampleEnvelope(t, domain.MsgRoomStateChanged, map[string]any{"by": "test"}),
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	h.handlePubsubMessage("room-A", &redis.Message{Channel: "wt:room:room-A:events", Payload: string(payload)})

	// Alice MUST NOT receive the message — own echo is dropped.
	time.Sleep(50 * time.Millisecond)
	if got := fcA.payloads(); len(got) != 0 {
		t.Fatalf("alice received own echo (%d payloads): %v", len(got), got)
	}
}

// ----------------------------------------------------------------------------
// Behavior 12 — Pubsub subscriber applies messages with a foreign instanceID
// (v2 horizontal scale path).
// ----------------------------------------------------------------------------

func TestPubsubSubscriber_AppliesForeignFrame(t *testing.T) {
	h, _, _ := newTestHub(t)
	_, fcA := registerFake(t, h, "room-A", "alice", "Alice")

	frame := pubsubFrame{
		InstanceID: "other-instance-uuid",
		Env:        sampleEnvelope(t, domain.MsgChatMessageOut, map[string]any{"x": 1}),
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	h.handlePubsubMessage("room-A", &redis.Message{Channel: "wt:room:room-A:events", Payload: string(payload)})

	got := waitForPayload(t, fcA, 1, time.Second)
	if len(got) != 1 {
		t.Fatalf("alice got %d messages, want 1", len(got))
	}
	var decoded domain.Envelope
	if err := json.Unmarshal(got[0], &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Type != domain.MsgChatMessageOut {
		t.Fatalf("decoded.Type = %q, want %q", decoded.Type, domain.MsgChatMessageOut)
	}
}

// ----------------------------------------------------------------------------
// Behavior 13 — concurrent Register/Unregister/Broadcast are race-free.
// Run with `go test -race`. The asserts are coarse (no panic, hub ends
// empty) — the race detector does the heavy lifting.
// ----------------------------------------------------------------------------

func TestHub_ConcurrentSafety(t *testing.T) {
	h, _, _ := newTestHub(t)
	const goroutines = 16
	const opsPerGo = 50

	var wg sync.WaitGroup
	var registered atomic.Int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			conns := make([]*Connection, 0, opsPerGo)
			for j := 0; j < opsPerGo; j++ {
				c, _ := registerFake(t, h, "room-A", "user", "u")
				conns = append(conns, c)
				registered.Add(1)
			}
			// Broadcast a few times to exercise local fanout under contention.
			for j := 0; j < 5; j++ {
				env := sampleEnvelope(t, domain.MsgPresenceHeartbeat, map[string]any{"g": gid})
				_, _ = h.Broadcast(context.Background(), "room-A", env, "")
			}
			for _, c := range conns {
				h.Unregister(c)
			}
		}(i)
	}
	wg.Wait()

	if got := h.MemberCount("room-A"); got != 0 {
		t.Fatalf("MemberCount after concurrent ops = %d, want 0", got)
	}
}

// ----------------------------------------------------------------------------
// Behavior 14 — Hub.Close shuts down every connection.
// ----------------------------------------------------------------------------

func TestHub_Close_DrainsAllConnections(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	log := logger.Default()
	r := repo.NewRoomRepo(client, 900*time.Second, log)

	h := NewHub(r, log, "test-instance-close")
	_, _ = registerFake(t, h, "room-A", "alice", "Alice")
	_, _ = registerFake(t, h, "room-B", "bob", "Bob")

	h.Close()
	if got := h.MemberCount("room-A"); got != 0 {
		t.Fatalf("room-A MemberCount after Close = %d, want 0", got)
	}
	if got := h.MemberCount("room-B"); got != 0 {
		t.Fatalf("room-B MemberCount after Close = %d, want 0", got)
	}
}
