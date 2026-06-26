package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"

	"github.com/ILITA-hub/animeenigma/worker/internal/wire"
)

// TestRun_EnrollUnauthorizedIsTerminal (B2b): when the server rejects the enroll
// token with 401 (single-use token already consumed/revoked), Run must return the
// TERMINAL ErrSessionRejected rather than retrying forever — so main() can exit
// with a distinct code instead of crash-looping. With permanent sessions a 401 is
// no longer the expected "session expired" path.
func TestRun_EnrollUnauthorizedIsTerminal(t *testing.T) {
	t.Parallel()

	var enrollHits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/worker/enroll" {
			enrollHits.Add(1)
			w.WriteHeader(http.StatusUnauthorized) // consumed/revoked token
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := Config{ServerURL: srv.URL, EnrollToken: "already-used"}
	c := NewClient(cfg)
	c.backoff = BackoffConfig{Initial: 5 * time.Millisecond, Max: 20 * time.Millisecond}

	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background()) }()

	select {
	case err := <-done:
		var rejected ErrSessionRejected
		if !errors.As(err, &rejected) {
			t.Fatalf("Run returned %v (%T); want ErrSessionRejected", err, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not exit on terminal 401 (likely crash-looping)")
	}

	// It must NOT have retried enroll many times — a terminal 401 is one-and-done.
	if n := enrollHits.Load(); n != 1 {
		t.Errorf("expected exactly 1 enroll attempt on terminal 401, got %d", n)
	}
}

// ── Test 8: live-WS lease_grant drives RunLeaseLoop → GET + PUT ─────────────

// TestLeaseGrantDrivesLeaseLoop verifies the end-to-end WS wiring:
// a lease_grant frame received over the live WebSocket connection is forwarded
// to RunLeaseLoop, which downloads the input segment (GET) and uploads the
// output segment (PUT). This discharges the Task-15 contract: "wiring the lease
// loop into the real WS dispatch".
//
// The test uses CopyProcessor (via processorFn) so it does not require ffmpeg.
func TestLeaseGrantDrivesLeaseLoop(t *testing.T) {
	t.Parallel()

	var putCount atomic.Int64

	// Combined server: enroll + WS control plane + segment data plane.
	mux := http.NewServeMux()

	// POST /worker/enroll
	mux.HandleFunc("/worker/enroll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fakeEnrollResp) //nolint:errcheck
	})

	// Segment data plane: GET returns fake content; PUT records the upload.
	mux.HandleFunc("/worker/segments/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("segment-data")) //nolint:errcheck
		case http.MethodPut:
			putCount.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// WS control plane: wait for register frame, then send a lease_grant.
	mux.HandleFunc("/worker/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Read (and discard) the register frame.
		_, _, err = conn.ReadMessage()
		if err != nil {
			return
		}

		// Send a lease_grant pointing at the segment server above.
		grant := wire.LeaseGrantPayload{
			JobID: "test-job",
			Idx:   0,
			Handles: wire.LeaseHandles{
				GetHandle: "gh", GetExp: "9999999999", GetSig: "gs",
				PutHandle: "ph", PutExp: "9999999999", PutSig: "ps",
			},
		}
		f, err := wire.NewFrame("lease_grant", 10, grant)
		if err != nil {
			return
		}
		raw, err := json.Marshal(f)
		if err != nil {
			return
		}
		conn.WriteMessage(gorillaws.TextMessage, raw) //nolint:errcheck

		// Keep the connection open long enough for the lease loop to complete.
		// The test context will cancel first; gorilla will see a close message.
		time.Sleep(5 * time.Second)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := Config{
		ServerURL:   srv.URL,
		EnrollToken: "tok",
		Mode:        "batch",
		WorkDir:     t.TempDir(),
	}
	c := NewClient(cfg)
	c.backoff = BackoffConfig{Initial: 10 * time.Millisecond, Max: 50 * time.Millisecond}

	// Inject CopyProcessor so no ffmpeg binary is needed in this integration test.
	c.processorFn = func(_ Config) (Processor, error) {
		return CopyProcessor{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	go c.Run(ctx) //nolint:errcheck

	// Wait for at least one PUT (proves lease loop downloaded, processed, and uploaded).
	if !waitFor(t, 5*time.Second, func() bool { return putCount.Load() >= 1 }) {
		t.Fatalf("expected at least 1 PUT upload (lease loop processed segment), got %d", putCount.Load())
	}

	cancel()
}

// safeBuffer is a goroutine-safe bytes.Buffer for capturing stdout in tests.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *safeBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeBuffer) Len() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Len()
}

func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// testUpgrader upgrades HTTP to WebSocket. Origin check is permissive for tests
// because httptest.Server is on 127.0.0.1 and the test dialer does not send an
// Origin header (matches production constraint).
var testUpgrader = gorillaws.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// fakeEnrollResp is the fixed EnrollResponse returned by the fake enroll endpoint.
var fakeEnrollResp = wire.EnrollResponse{
	WorkerID: "test-worker-1",
	Handle:   "h",
	Exp:      "9999999999",
	Sig:      "s",
}

// testServer bundles an httptest.Server with the hooks tests need to inspect.
type testServer struct {
	ts *httptest.Server

	enrollCalled atomic.Int64 // number of POST /worker/enroll calls

	mu       sync.Mutex
	frames   []wire.Frame   // frames received over WS in order of arrival
	wsClosed []bool         // true for each WS session that was force-closed
	wsConns  []*gorillaws.Conn // every WS connection ever accepted
}

// newTestServer builds a test HTTP server with /worker/enroll and /worker/ws
// routes. serverClose controls whether the server immediately closes each WS
// connection (for reconnect tests).
func newTestServer(t *testing.T, serverClose bool) *testServer {
	t.Helper()
	ts := &testServer{}

	mux := http.NewServeMux()

	// POST /worker/enroll — return the fixed enroll response.
	mux.HandleFunc("/worker/enroll", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ts.enrollCalled.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fakeEnrollResp) //nolint:errcheck
	})

	// GET /worker/ws — upgrade and capture frames; optionally close immediately.
	mux.HandleFunc("/worker/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		ts.mu.Lock()
		ts.wsConns = append(ts.wsConns, conn)
		ts.mu.Unlock()

		if serverClose {
			// Force-close to trigger reconnect on the client side.
			conn.Close()
			return
		}

		// Set up a pong handler so server-side pings are tracked.
		conn.SetPongHandler(func(appData string) error {
			return nil
		})

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var f wire.Frame
			if json.Unmarshal(msg, &f) == nil {
				ts.mu.Lock()
				ts.frames = append(ts.frames, f)
				ts.mu.Unlock()
			}
		}
	})

	ts.ts = httptest.NewServer(mux)
	t.Cleanup(ts.ts.Close)
	return ts
}

// getFrames returns a snapshot of captured frames.
func (ts *testServer) getFrames() []wire.Frame {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	out := make([]wire.Frame, len(ts.frames))
	copy(out, ts.frames)
	return out
}

// wsConnCount returns how many WebSocket connections have been accepted.
func (ts *testServer) wsConnCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.wsConns)
}

// buildClient constructs a test-friendly Client wired to ts, with fast backoff
// and a capturable stdout buffer.
func buildClient(ts *testServer, buf *safeBuffer) *Client {
	cfg := Config{
		ServerURL:   ts.ts.URL,
		EnrollToken: "test-token",
		Mode:        "batch",
	}
	c := NewClient(cfg)
	// Fast backoff so reconnect tests complete quickly.
	c.backoff = BackoffConfig{
		Initial: 10 * time.Millisecond,
		Max:     50 * time.Millisecond,
	}
	if buf != nil {
		c.stdout = buf
	}
	return c
}

// waitFor polls pred until it returns true or deadline is reached.
func waitFor(t *testing.T, d time.Duration, pred func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// ── Test 1: client enrolls successfully ──────────────────────────────────────

func TestEnrollCalled(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, false)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c := buildClient(ts, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx) //nolint:errcheck
	}()

	// Wait until enroll is called.
	if !waitFor(t, 2*time.Second, func() bool { return ts.enrollCalled.Load() >= 1 }) {
		t.Fatal("expected POST /worker/enroll to be called")
	}

	cancel()
	<-done
}

// ── Test 2: client opens WS after enroll ────────────────────────────────────

func TestWSOpenedAfterEnroll(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, false)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c := buildClient(ts, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx) //nolint:errcheck
	}()

	if !waitFor(t, 2*time.Second, func() bool { return ts.wsConnCount() >= 1 }) {
		t.Fatal("expected WebSocket connection to be opened")
	}

	cancel()
	<-done
}

// ── Test 3: register frame is the first WS message ──────────────────────────

func TestRegisterFrameSentFirst(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, false)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c := buildClient(ts, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx) //nolint:errcheck
	}()

	if !waitFor(t, 2*time.Second, func() bool { return len(ts.getFrames()) >= 1 }) {
		t.Fatal("expected at least one frame to be received by server")
	}

	frames := ts.getFrames()
	if frames[0].Type != "register" {
		t.Fatalf("expected first frame type=register, got %q", frames[0].Type)
	}

	var reg wire.RegisterPayload
	if err := frames[0].Decode(&reg); err != nil {
		t.Fatalf("decode RegisterPayload: %v", err)
	}
	if reg.WorkerID != fakeEnrollResp.WorkerID {
		t.Errorf("register.worker_id = %q, want %q", reg.WorkerID, fakeEnrollResp.WorkerID)
	}

	cancel()
	<-done
}

// ── Test 4: client responds to pings (connection stays alive) ────────────────

func TestPingPong(t *testing.T) {
	t.Parallel()

	pongReceived := make(chan struct{}, 1)

	// Custom server that sends a ping after receiving the register frame.
	mux := http.NewServeMux()
	mux.HandleFunc("/worker/enroll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fakeEnrollResp) //nolint:errcheck
	})
	mux.HandleFunc("/worker/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Receive the register frame.
		_, _, err = conn.ReadMessage()
		if err != nil {
			return
		}

		// Track pongs from client via our pong handler.
		conn.SetPongHandler(func(string) error {
			select {
			case pongReceived <- struct{}{}:
			default:
			}
			return nil
		})

		// Send a ping.
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
		conn.WriteMessage(gorillaws.PingMessage, nil)           //nolint:errcheck

		// Keep reading (gorilla handles pong on ReadMessage).
		conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
		conn.ReadMessage()                                      //nolint:errcheck
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := Config{ServerURL: srv.URL, EnrollToken: "tok", Mode: "batch"}
	c := NewClient(cfg)
	c.backoff = BackoffConfig{Initial: 10 * time.Millisecond, Max: 50 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	go c.Run(ctx) //nolint:errcheck

	select {
	case <-pongReceived:
		// Client responded to ping — test passes.
	case <-time.After(3 * time.Second):
		t.Fatal("client did not respond to ping within 3s")
	}
}

// ── Test 5: client reconnects after forced server close ──────────────────────

func TestReconnectAfterServerClose(t *testing.T) {
	t.Parallel()

	// First connection: force-close. Subsequent connections: stay open.
	var connCount atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/worker/enroll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fakeEnrollResp) //nolint:errcheck
	})
	mux.HandleFunc("/worker/ws", func(w http.ResponseWriter, r *http.Request) {
		n := connCount.Add(1)
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		if n == 1 {
			// Force-close the first connection to trigger a reconnect.
			conn.Close()
			return
		}

		// Keep subsequent connections open to let the client stabilise.
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := Config{ServerURL: srv.URL, EnrollToken: "tok", Mode: "batch"}
	c := NewClient(cfg)
	c.backoff = BackoffConfig{Initial: 10 * time.Millisecond, Max: 50 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go c.Run(ctx) //nolint:errcheck

	// Wait until the client has established a second connection.
	if !waitFor(t, 3*time.Second, func() bool { return connCount.Load() >= 2 }) {
		t.Fatalf("expected client to reconnect (got %d WS connections)", connCount.Load())
	}

	// Verify no busy-loop: with 10ms initial backoff, 3 reconnects in <1s is
	// expected but 10+ in <100ms would indicate a backoff bug.
	time.Sleep(100 * time.Millisecond)
	finalCount := connCount.Load()
	if finalCount > 10 {
		t.Errorf("too many reconnects in <200ms: %d — possible busy-loop", finalCount)
	}

	cancel()
}

// ── Test 7: re-enroll on 401 WS upgrade ─────────────────────────────────────

// TestReEnrollOn401 verifies that when the WS upgrade returns 401 (session
// expired), the client re-enrolls and then successfully reconnects.
func TestReEnrollOn401(t *testing.T) {
	t.Parallel()

	var (
		enrollCount atomic.Int64
		wsCount     atomic.Int64
	)

	mux := http.NewServeMux()

	// Enroll endpoint: always succeeds.
	mux.HandleFunc("/worker/enroll", func(w http.ResponseWriter, r *http.Request) {
		enrollCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fakeEnrollResp) //nolint:errcheck
	})

	// WS endpoint: first upgrade returns 401; subsequent upgrades succeed.
	mux.HandleFunc("/worker/ws", func(w http.ResponseWriter, r *http.Request) {
		n := wsCount.Add(1)
		if n == 1 {
			// Reject first upgrade with 401 to trigger re-enroll.
			http.Error(w, "session expired", http.StatusUnauthorized)
			return
		}
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := Config{ServerURL: srv.URL, EnrollToken: "tok", Mode: "batch"}
	c := NewClient(cfg)
	c.backoff = BackoffConfig{Initial: 10 * time.Millisecond, Max: 50 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go c.Run(ctx) //nolint:errcheck

	// Wait for the client to have re-enrolled (enrollCount >= 2) and then
	// successfully opened a second WS connection.
	if !waitFor(t, 4*time.Second, func() bool {
		return enrollCount.Load() >= 2 && wsCount.Load() >= 2
	}) {
		t.Fatalf("client did not re-enroll on 401 (enrolls=%d ws=%d)",
			enrollCount.Load(), wsCount.Load())
	}

	cancel()
}

// ── Test 6: stdout contains ONLY neutral tokens ──────────────────────────────

func TestStdoutNeutralOnly(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, false)

	var buf safeBuffer

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c := buildClient(ts, &buf)

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx) //nolint:errcheck
	}()

	// Wait until at least one line is written.
	if !waitFor(t, 1500*time.Millisecond, func() bool { return buf.Len() > 0 }) {
		t.Fatal("expected stdout output")
	}

	cancel()
	<-done

	output := buf.String()

	// The test server URL (e.g. "127.0.0.1:PORT") must not appear in stdout.
	serverHost := ts.ts.URL
	if strings.Contains(output, serverHost) {
		t.Errorf("stdout contains server URL %q — must only contain neutral tokens\nstdout:\n%s", serverHost, output)
	}

	// Extract host:port from the server URL (strip http://).
	hostPort := strings.TrimPrefix(serverHost, "http://")
	if strings.Contains(output, hostPort) {
		t.Errorf("stdout contains host:port %q — must only contain neutral tokens\nstdout:\n%s", hostPort, output)
	}

	// Every non-empty line must be a known neutral token.
	allowed := map[string]bool{
		"starting":     true,
		"connected":    true,
		"idle":         true,
		"leased":       true,
		"processing":   true,
		"error":        true,
		"reconnecting": true,
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		if !allowed[line] {
			t.Errorf("unexpected stdout token %q — must be one of %v", line, allowed)
		}
	}
}
