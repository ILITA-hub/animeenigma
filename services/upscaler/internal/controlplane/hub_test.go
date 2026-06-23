package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func (f *fakeLeaser) OnLeaseReq(_ context.Context, _ string) (*domain.UpscaleSegment, LeaseHandles, error) {
	handles := LeaseHandles{
		GetHandle: f.jobID + ":segment-get:0",
		GetExp:    "9999999999",
		GetSig:    "fakesig0000000000000000000000001",
		PutHandle: f.jobID + ":segment-put:0",
		PutExp:    "9999999999",
		PutSig:    "fakesig0000000000000000000000002",
	}
	return f.seg, handles, nil
}

// fakeWorkerRepo records Heartbeat calls.
type fakeWorkerRepo struct{}

func (f *fakeWorkerRepo) Heartbeat(_ context.Context, _, _ string, _ int, _ time.Time) error {
	return nil
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
