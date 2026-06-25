package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	gorillaws "github.com/gorilla/websocket"
)

// ── Minimal fakes for hub tests ───────────────────────────────────────────────

// fakeLeaser always returns a fixed segment with idx=0 for job "hub-job-001".
type fakeLeaser struct {
	jobID string
	seg   *domain.UpscaleSegment
}

func newFakeLeaser(jobID string, idx int) *fakeLeaser {
	return &fakeLeaser{
		jobID: jobID,
		seg:   &domain.UpscaleSegment{JobID: jobID, Idx: idx, Status: domain.SegLeased},
	}
}

func (f *fakeLeaser) OnLeaseReq(_ context.Context, _ string) (*domain.UpscaleSegment, LeaseHandles, string, int, error) {
	handles := LeaseHandles{
		GetHandle: f.jobID + ":segment-get:0",
		GetExp:    "9999999999",
		GetSig:    "fakesig0000000000000000000000001",
		PutHandle: f.jobID + ":segment-put:0",
		PutExp:    "9999999999",
		PutSig:    "fakesig0000000000000000000000002",
	}
	return f.seg, handles, "mock", 2, nil
}

// fakeWorkerRepo records Heartbeat calls.
type fakeWorkerRepo struct{}

func (f *fakeWorkerRepo) Heartbeat(_ context.Context, _, _ string, _ int, _ time.Time) error {
	return nil
}

// slowLeaser blocks for `delay` inside OnLeaseReq before returning a fixed
// segment, and counts how many times OnLeaseReq actually started. It lets the
// tests prove (a) the lease path runs OFF the read loop (a slow lease doesn't
// block ping/pong) and (b) the per-Conn single-flight drops duplicate
// in-flight lease_req frames.
type slowLeaser struct {
	jobID    string
	delay    time.Duration
	gate     chan struct{} // when non-nil, OnLeaseReq blocks on it instead of sleeping
	started  atomic.Int32  // incremented when OnLeaseReq begins
	finished atomic.Int32  // incremented when OnLeaseReq returns
}

func (s *slowLeaser) OnLeaseReq(ctx context.Context, _ string) (*domain.UpscaleSegment, LeaseHandles, string, int, error) {
	s.started.Add(1)
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			s.finished.Add(1)
			return nil, LeaseHandles{}, "", 0, ctx.Err()
		}
	} else if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			s.finished.Add(1)
			return nil, LeaseHandles{}, "", 0, ctx.Err()
		}
	}
	s.finished.Add(1)
	seg := &domain.UpscaleSegment{JobID: s.jobID, Idx: 0, Status: domain.SegLeased}
	handles := LeaseHandles{
		GetHandle: s.jobID + ":segment-get:0", GetExp: "9999999999", GetSig: "fakesig0000000000000000000000001",
		PutHandle: s.jobID + ":segment-put:0", PutExp: "9999999999", PutSig: "fakesig0000000000000000000000002",
	}
	return seg, handles, "mock", 2, nil
}

// buildTestHubWithLeaser starts a Hub backed by a caller-supplied leaser.
func buildTestHubWithLeaser(t *testing.T, leaser Leaser) (*httptest.Server, *Hub, func(workerID string) *gorillaws.Conn) {
	t.Helper()

	workers := &fakeWorkerRepo{}
	log := logger.Default()
	hub := NewHubWithConfig(leaser, workers, log, hubTestConfig)

	mux := http.NewServeMux()
	mux.HandleFunc("/worker/ws", UpgradeHandler(hub))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dial := func(workerID string) *gorillaws.Conn {
		t.Helper()
		_, exp, sig := MintSession(workerID, SessionTTL)
		u := "ws" + strings.TrimPrefix(srv.URL, "http") +
			"/worker/ws?worker_id=" + workerID + "&exp=" + exp + "&sig=" + sig
		conn, _, err := gorillaws.DefaultDialer.Dial(u, nil)
		if err != nil {
			t.Fatalf("dial %q: %v", u, err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}

	return srv, hub, dial
}

// ── Hub test server helper ────────────────────────────────────────────────────

// hubTestConfig overrides timing constants to make tests fast.
var hubTestConfig = HubConfig{
	PongWait:   500 * time.Millisecond,
	PingPeriod: 200 * time.Millisecond,
	WriteWait:  100 * time.Millisecond,
}

// buildTestHub starts a Hub backed by a fake leaser and returns the test server
// and a dial function.
func buildTestHub(t *testing.T) (*httptest.Server, *Hub, func(workerID string) *gorillaws.Conn) {
	t.Helper()

	leaser := newFakeLeaser("hub-job-001", 0)
	workers := &fakeWorkerRepo{}
	log := logger.Default()

	hub := NewHubWithConfig(leaser, workers, log, hubTestConfig)

	// Build a mint-session helper.
	mintSessionToken := func(workerID string) (exp, sig string) {
		_, e, s := MintSession(workerID, SessionTTL)
		return e, s
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/worker/ws", UpgradeHandler(hub))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dial := func(workerID string) *gorillaws.Conn {
		t.Helper()
		exp, sig := mintSessionToken(workerID)
		u := "ws" + strings.TrimPrefix(srv.URL, "http") +
			"/worker/ws?worker_id=" + workerID + "&exp=" + exp + "&sig=" + sig
		conn, _, err := gorillaws.DefaultDialer.Dial(u, nil)
		if err != nil {
			t.Fatalf("dial %q: %v", u, err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}

	return srv, hub, dial
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestHub_LeaseReqReturnsLeaseGrant verifies the full lease_req → lease_grant flow.
func TestHub_LeaseReqReturnsLeaseGrant(t *testing.T) {
	_, hub, dial := buildTestHub(t)

	wID := "hub-worker-1"
	conn := dial(wID)

	// Wait briefly for registration to complete.
	waitFor(t, func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		_, ok := hub.conns[wID]
		return ok
	}, 500*time.Millisecond, "worker to register")

	// Send a lease_req frame.
	req, _ := NewFrame("lease_req", 1, LeaseReqPayload{})
	raw, _ := json.Marshal(req)
	if err := conn.WriteMessage(gorillaws.TextMessage, raw); err != nil {
		t.Fatalf("write lease_req: %v", err)
	}

	// Read the lease_grant response.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read lease_grant: %v", err)
	}
	var f Frame
	if err := json.Unmarshal(msg, &f); err != nil {
		t.Fatalf("unmarshal frame: %v", err)
	}
	if f.Type != "lease_grant" {
		t.Errorf("frame type = %q, want %q", f.Type, "lease_grant")
	}
	var grant LeaseGrantPayload
	if err := f.Decode(&grant); err != nil {
		t.Fatalf("decode lease_grant: %v", err)
	}
	if grant.JobID != "hub-job-001" {
		t.Errorf("grant.JobID = %q, want %q", grant.JobID, "hub-job-001")
	}
	if grant.Idx != 0 {
		t.Errorf("grant.Idx = %d, want 0", grant.Idx)
	}
	if grant.Handles.GetHandle == "" || grant.Handles.PutHandle == "" {
		t.Errorf("grant.Handles empty: %+v", grant.Handles)
	}
}

// TestHub_DisconnectUnregisters verifies that when a client closes the WebSocket
// connection, the hub removes it from the connection map.
func TestHub_DisconnectUnregisters(t *testing.T) {
	_, hub, dial := buildTestHub(t)

	wID := "hub-worker-2"
	conn := dial(wID)

	// Wait for registration.
	waitFor(t, func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		_, ok := hub.conns[wID]
		return ok
	}, 500*time.Millisecond, "worker to register")

	// Close the connection.
	conn.WriteControl(
		gorillaws.CloseMessage,
		gorillaws.FormatCloseMessage(gorillaws.CloseNormalClosure, "bye"),
		time.Now().Add(100*time.Millisecond),
	)
	conn.Close()

	// Wait for unregistration.
	waitFor(t, func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		_, ok := hub.conns[wID]
		return !ok
	}, 1*time.Second, "worker to unregister")
}

// TestHub_PingKeepsConnectionAlive verifies that a well-behaved client that
// automatically pongs stays connected past pingPeriod.
func TestHub_PingKeepsConnectionAlive(t *testing.T) {
	_, hub, dial := buildTestHub(t)

	wID := "hub-worker-3"
	conn := dial(wID)
	conn.SetPingHandler(func(appData string) error {
		// Respond to pings so the server doesn't close us for not ponging.
		return conn.WriteControl(gorillaws.PongMessage, []byte(appData),
			time.Now().Add(100*time.Millisecond))
	})

	// Wait for registration.
	waitFor(t, func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		_, ok := hub.conns[wID]
		return ok
	}, 500*time.Millisecond, "worker to register")

	// Wait past pingPeriod + a bit.
	time.Sleep(hubTestConfig.PingPeriod + 100*time.Millisecond)

	// Start a goroutine to drain server messages (pings, etc.) so our read
	// doesn't block the ping handler from running on the same connection.
	go func() {
		for {
			conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// After the ping interval, the connection should still be registered.
	hub.mu.RLock()
	_, ok := hub.conns[wID]
	hub.mu.RUnlock()
	if !ok {
		t.Error("connection was removed from hub after pingPeriod despite good ping/pong")
	}
}

// TestHub_BrowserOriginRejected verifies that a connection with an Origin
// header is rejected (browser-origin check).
func TestHub_BrowserOriginRejected(t *testing.T) {
	_, _, _ = buildTestHub(t) // starts the server but we use a fresh one below

	leaser := newFakeLeaser("hub-job-001", 0)
	workers := &fakeWorkerRepo{}
	log := logger.Default()
	hub := NewHubWithConfig(leaser, workers, log, hubTestConfig)

	mux := http.NewServeMux()
	mux.HandleFunc("/worker/ws", UpgradeHandler(hub))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wID := "hub-worker-browser"
	_, e, s := MintSession(wID, SessionTTL)
	u := "ws" + strings.TrimPrefix(srv.URL, "http") +
		"/worker/ws?worker_id=" + wID + "&exp=" + e + "&sig=" + s

	hdr := http.Header{}
	hdr.Set("Origin", "https://evil.example.com")
	_, resp, err := gorillaws.DefaultDialer.Dial(u, hdr)
	if err == nil {
		t.Fatal("expected dial to fail for browser origin, but it succeeded")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

// TestHub_SlowLeaseDoesNotBlockReadPump proves the lease path runs OFF the read
// loop (I-2). A leaser that blocks far longer than pongWait must not prevent the
// read pump from processing the server's pings (auto-ponged by the client),
// so the connection stays alive — and the lease_grant still arrives once the
// slow leaser finally returns. If the lease ran inline in readPump, the server
// would stop reading, miss the client's pongs, and tear the conn down at
// pongWait (500ms here).
func TestHub_SlowLeaseDoesNotBlockReadPump(t *testing.T) {
	// gate-controlled leaser: OnLeaseReq blocks until we release the gate, which
	// we do AFTER waiting well past pongWait.
	leaser := &slowLeaser{jobID: "slow-job-001", gate: make(chan struct{})}
	_, hub, dial := buildTestHubWithLeaser(t, leaser)

	wID := "hub-worker-slow"
	conn := dial(wID)

	// Client auto-pongs (gorilla default handler) AND we keep a reader running so
	// control frames are processed. We capture the first non-control frame.
	gotFrame := make(chan []byte, 1)
	go func() {
		for {
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			select {
			case gotFrame <- msg:
			default:
			}
		}
	}()

	waitFor(t, func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		_, ok := hub.conns[wID]
		return ok
	}, 500*time.Millisecond, "worker to register")

	// Fire a lease_req. OnLeaseReq will block on the gate.
	req, _ := NewFrame("lease_req", 1, LeaseReqPayload{})
	raw, _ := json.Marshal(req)
	if err := conn.WriteMessage(gorillaws.TextMessage, raw); err != nil {
		t.Fatalf("write lease_req: %v", err)
	}

	// Wait until OnLeaseReq has actually begun (off the read loop, in a goroutine).
	waitFor(t, func() bool { return leaser.started.Load() == 1 }, 1*time.Second, "lease to start")

	// Sleep well past pongWait (500ms) with the lease still blocked. If the read
	// loop were blocked, the server would miss our pongs and close the conn here.
	time.Sleep(hubTestConfig.PongWait + 300*time.Millisecond)

	// The connection must still be registered — read loop kept ponging.
	hub.mu.RLock()
	_, stillUp := hub.conns[wID]
	hub.mu.RUnlock()
	if !stillUp {
		t.Fatal("connection was torn down while a slow lease was resolving — lease path is blocking the read pump (I-2)")
	}

	// Release the gate; the lease_grant should now arrive.
	close(leaser.gate)

	select {
	case msg := <-gotFrame:
		var f Frame
		if err := json.Unmarshal(msg, &f); err != nil {
			t.Fatalf("unmarshal grant frame: %v", err)
		}
		if f.Type != "lease_grant" {
			t.Errorf("frame type = %q, want lease_grant", f.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lease_grant never arrived after releasing the slow leaser")
	}
}

// TestHub_DuplicateLeaseReqIgnored proves the per-Conn single-flight guard: a
// second lease_req arriving while the first is still in flight is dropped, so
// OnLeaseReq starts exactly once. A worker only ever holds one lease at a time.
func TestHub_DuplicateLeaseReqIgnored(t *testing.T) {
	leaser := &slowLeaser{jobID: "dup-job-001", gate: make(chan struct{})}
	_, hub, dial := buildTestHubWithLeaser(t, leaser)

	wID := "hub-worker-dup"
	conn := dial(wID)

	// Keep a reader running so the conn stays healthy (auto-pong) and we can
	// observe exactly one grant.
	grants := make(chan []byte, 4)
	go func() {
		for {
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var f Frame
			if json.Unmarshal(msg, &f) == nil && f.Type == "lease_grant" {
				select {
				case grants <- msg:
				default:
				}
			}
		}
	}()

	waitFor(t, func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		_, ok := hub.conns[wID]
		return ok
	}, 500*time.Millisecond, "worker to register")

	// Fire the first lease_req; wait until it has begun (and is blocked on gate).
	req1, _ := NewFrame("lease_req", 1, LeaseReqPayload{})
	raw1, _ := json.Marshal(req1)
	if err := conn.WriteMessage(gorillaws.TextMessage, raw1); err != nil {
		t.Fatalf("write lease_req #1: %v", err)
	}
	waitFor(t, func() bool { return leaser.started.Load() == 1 }, 1*time.Second, "first lease to start")

	// Fire several MORE lease_req frames while the first is in flight. These must
	// all be dropped by the single-flight guard.
	for i := 2; i <= 5; i++ {
		req, _ := NewFrame("lease_req", i, LeaseReqPayload{})
		raw, _ := json.Marshal(req)
		if err := conn.WriteMessage(gorillaws.TextMessage, raw); err != nil {
			t.Fatalf("write lease_req #%d: %v", i, err)
		}
	}

	// Give the read loop time to process (and drop) the duplicates.
	time.Sleep(200 * time.Millisecond)

	if got := leaser.started.Load(); got != 1 {
		t.Fatalf("OnLeaseReq started %d times while one was in flight, want 1 (single-flight guard)", got)
	}

	// Release the gate so the first lease completes and the flag clears.
	close(leaser.gate)

	// Exactly one grant should arrive.
	select {
	case <-grants:
	case <-time.After(2 * time.Second):
		t.Fatal("no lease_grant arrived after releasing the gate")
	}

	// No second grant should follow within a short window.
	select {
	case <-grants:
		t.Fatal("a second lease_grant arrived — duplicate lease_req was not dropped")
	case <-time.After(300 * time.Millisecond):
		// good — only one grant
	}
}

// ── Helper ────────────────────────────────────────────────────────────────────

// waitFor polls cond until it returns true or timeout elapses, then fails if
// still false.
func waitFor(t *testing.T, cond func() bool, timeout time.Duration, desc string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", desc)
}
