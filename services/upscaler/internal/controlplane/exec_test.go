package controlplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── Fake hub ─────────────────────────────────────────────────────────────────

// execFakeHub records frames sent to each workerID. Satisfies hubSender.
// Named with "exec" prefix to avoid collision with fakeHub in commands_test.go.
type execFakeHub struct {
	mu     sync.Mutex
	frames map[string][]Frame
	// errOnSend, when set, is returned for any Send call to the matching worker.
	errOnSend map[string]error
}

func newExecFakeHub() *execFakeHub {
	return &execFakeHub{
		frames:    make(map[string][]Frame),
		errOnSend: make(map[string]error),
	}
}

func (h *execFakeHub) Send(workerID string, f Frame) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err, ok := h.errOnSend[workerID]; ok {
		return err
	}
	h.frames[workerID] = append(h.frames[workerID], f)
	return nil
}

// sentFrames returns a copy of frames sent to workerID.
func (h *execFakeHub) sentFrames(workerID string) []Frame {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]Frame, len(h.frames[workerID]))
	copy(cp, h.frames[workerID])
	return cp
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// buildRelay builds an ExecRelay backed by an execFakeHub, using the given writer for audit.
func buildRelay(t *testing.T, enabled bool, audit *bytes.Buffer) (*ExecRelay, *execFakeHub) {
	t.Helper()
	hub := newExecFakeHub()
	relay := NewExecRelay(hub, ExecRelayConfig{
		Enabled:     enabled,
		IdleTimeout: 5 * time.Second,
	}, nil, audit)
	return relay, hub
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestExec_DisabledGate verifies that Open returns ErrRemoteShellDisabled when
// the feature is disabled, and sends NO frame to the hub.
func TestExec_DisabledGate(t *testing.T) {
	var audit bytes.Buffer
	relay, hub := buildRelay(t, false /* disabled */, &audit)

	sid, err := relay.Open("worker-1", "admin-1", false)
	if !errors.Is(err, ErrRemoteShellDisabled) {
		t.Fatalf("want ErrRemoteShellDisabled, got err=%v sid=%q", err, sid)
	}
	if sid != "" {
		t.Errorf("want empty sessionID on disabled, got %q", sid)
	}
	if frames := hub.sentFrames("worker-1"); len(frames) != 0 {
		t.Errorf("want 0 frames sent to worker, got %d", len(frames))
	}
	if audit.Len() != 0 {
		t.Errorf("want no audit line on disabled, got %q", audit.String())
	}
}

// TestExec_OpenSendsFrame verifies that Open (when enabled) sends an exec_open
// frame to the correct worker and writes an audit EXEC_OPEN line.
func TestExec_OpenSendsFrame(t *testing.T) {
	var audit bytes.Buffer
	relay, hub := buildRelay(t, true, &audit)

	sid, err := relay.Open("worker-A", "admin-X", true)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}
	if sid == "" {
		t.Fatal("Open returned empty session ID")
	}

	// Verify exec_open frame was sent to the worker.
	frames := hub.sentFrames("worker-A")
	if len(frames) != 1 {
		t.Fatalf("want 1 frame sent to worker-A, got %d", len(frames))
	}
	if frames[0].Type != "exec_open" {
		t.Errorf("frame type = %q, want exec_open", frames[0].Type)
	}

	// Verify frame payload contains sessionID and pty=true.
	var payload ExecPayload
	if err := frames[0].Decode(&payload); err != nil {
		t.Fatalf("decode exec_open payload: %v", err)
	}
	if payload.SessionID != sid {
		t.Errorf("payload.SessionID = %q, want %q", payload.SessionID, sid)
	}
	if !payload.Pty {
		t.Error("payload.Pty = false, want true")
	}

	// Verify EXEC_OPEN audit line.
	line := audit.String()
	if !strings.Contains(line, "EXEC_OPEN") {
		t.Errorf("audit line missing EXEC_OPEN: %q", line)
	}
	if !strings.Contains(line, "session="+sid) {
		t.Errorf("audit line missing session: %q", line)
	}
	if !strings.Contains(line, "worker=worker-A") {
		t.Errorf("audit line missing worker: %q", line)
	}
	if !strings.Contains(line, "admin=admin-X") {
		t.Errorf("audit line missing admin: %q", line)
	}
	if !strings.Contains(line, "pty=true") {
		t.Errorf("audit line missing pty=true: %q", line)
	}
}

// TestExec_RelayWorkerToAdmin verifies that frames from the worker are delivered
// to the admin's Subscribe channel.
func TestExec_RelayWorkerToAdmin(t *testing.T) {
	var audit bytes.Buffer
	relay, _ := buildRelay(t, true, &audit)

	sid, err := relay.Open("worker-B", "admin-Y", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Subscribe returns the admin channel.
	ch := relay.Subscribe(sid)
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}

	// Simulate the worker sending exec_data back.
	data := []byte("hello from worker")
	f, err := NewFrame("exec_data", 0, ExecPayload{
		SessionID: sid,
		Data:      data,
	})
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}
	relay.DeliverFromWorker(f)

	// The admin channel should receive the frame.
	select {
	case received, ok := <-ch:
		if !ok {
			t.Fatal("admin channel closed unexpectedly")
		}
		if received.Type != "exec_data" {
			t.Errorf("received frame type = %q, want exec_data", received.Type)
		}
		var p ExecPayload
		if err := received.Decode(&p); err != nil {
			t.Fatalf("decode received payload: %v", err)
		}
		if string(p.Data) != string(data) {
			t.Errorf("received data = %q, want %q", p.Data, data)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for exec_data on admin channel")
	}
}

// TestExec_CloseAudited verifies that closing a session (from admin side) writes
// an EXEC_CLOSE audit line and removes the session.
func TestExec_CloseAudited(t *testing.T) {
	var audit bytes.Buffer
	relay, _ := buildRelay(t, true, &audit)

	sid, err := relay.Open("worker-C", "admin-Z", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Confirm session exists.
	if ch := relay.Subscribe(sid); ch == nil {
		t.Fatal("Subscribe returned nil before close")
	}

	code := 0
	relay.CloseSession(sid, &code)

	// Session should be gone.
	if ch := relay.Subscribe(sid); ch != nil {
		t.Error("Subscribe returned non-nil channel after CloseSession")
	}

	// EXEC_CLOSE audit line should be present.
	line := audit.String()
	if !strings.Contains(line, "EXEC_CLOSE") {
		t.Errorf("audit missing EXEC_CLOSE: %q", line)
	}
	if !strings.Contains(line, "session="+sid) {
		t.Errorf("audit EXEC_CLOSE missing session: %q", line)
	}
	if !strings.Contains(line, "exit=0") {
		t.Errorf("audit EXEC_CLOSE missing exit=0: %q", line)
	}
}

// TestExec_WorkerGoneEnds verifies that WorkerGone terminates all active
// sessions for the worker and delivers exec_close to their admin channels.
func TestExec_WorkerGoneEnds(t *testing.T) {
	var audit bytes.Buffer
	relay, _ := buildRelay(t, true, &audit)

	workerID := "worker-D"
	sid, err := relay.Open(workerID, "admin-W", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	ch := relay.Subscribe(sid)
	if ch == nil {
		t.Fatal("Subscribe returned nil")
	}

	// Simulate the worker connection dropping.
	relay.WorkerGone(workerID)

	// The admin channel must receive exec_close then close.
	var gotExecClose bool
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				// Channel closed — done draining.
				goto done
			}
			if f.Type == "exec_close" {
				gotExecClose = true
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for exec_close delivery after WorkerGone")
		}
	}
done:
	if !gotExecClose {
		t.Error("admin channel did not receive exec_close before closing")
	}

	// Session must no longer exist.
	if ch := relay.Subscribe(sid); ch != nil {
		t.Error("Subscribe returned non-nil after WorkerGone")
	}

	// EXEC_CLOSE audit should be present.
	if !strings.Contains(audit.String(), "EXEC_CLOSE") {
		t.Errorf("audit missing EXEC_CLOSE after WorkerGone: %q", audit.String())
	}
}

// TestExec_ConcurrentIsolated verifies that two concurrent sessions to
// different workers do not mix frames (race-detector test).
func TestExec_ConcurrentIsolated(t *testing.T) {
	var audit bytes.Buffer
	relay, _ := buildRelay(t, true, &audit)

	const workers = 4
	const messagesEach = 10

	type result struct {
		workerID  string
		sessionID string
		received  [][]byte
	}

	results := make([]result, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		i := i
		workerID := "worker-concurrent-" + string(rune('A'+i))
		adminID := "admin-" + string(rune('A'+i))

		sid, err := relay.Open(workerID, adminID, false)
		if err != nil {
			t.Fatalf("worker %s Open: %v", workerID, err)
		}
		results[i].workerID = workerID
		results[i].sessionID = sid

		ch := relay.Subscribe(sid)
		if ch == nil {
			t.Fatalf("worker %s: nil Subscribe channel", workerID)
		}

		// Reader goroutine: collects received data bytes from exec_data frames.
		wg.Add(1)
		go func(r *result, ch <-chan Frame) {
			defer wg.Done()
			for f := range ch {
				if f.Type == "exec_data" {
					var p ExecPayload
					if err := f.Decode(&p); err == nil {
						r.received = append(r.received, p.Data)
					}
				}
			}
		}(&results[i], ch)
	}

	// Send tagged messages to each worker concurrently.
	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < messagesEach; j++ {
				msg, _ := json.Marshal(map[string]interface{}{
					"worker_idx": i,
					"msg_idx":    j,
				})
				f, _ := NewFrame("exec_data", 0, ExecPayload{
					SessionID: results[i].sessionID,
					Data:      msg,
				})
				relay.DeliverFromWorker(f)
			}
		}()
	}

	// Close all sessions to unblock readers.
	for i := 0; i < workers; i++ {
		relay.CloseSession(results[i].sessionID, nil)
	}

	wg.Wait()

	// Verify each session only received its own worker's messages.
	for i := 0; i < workers; i++ {
		for _, raw := range results[i].received {
			var m map[string]interface{}
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Errorf("worker %d: unmarshal data: %v", i, err)
				continue
			}
			idx, _ := m["worker_idx"].(float64)
			if int(idx) != i {
				t.Errorf("worker %d session received frame from worker_idx=%d (cross-contamination!)", i, int(idx))
			}
		}
	}
}

// TestExec_ConcurrentCloseAndDeliverSameSession is the regression test for the
// close/deliver data race + send-on-closed-channel panic. It drives
// DeliverFromWorker and CloseSession against the SAME session concurrently in a
// tight loop. Without the fix (sess.closed guard + timer/channel ops under the
// relay lock), this reliably trips the race detector on the *time.Timer and/or
// panics with "send on closed channel". With the fix it is a clean no-op once
// the session is closed.
//
// Run under -race for full coverage.
func TestExec_ConcurrentCloseAndDeliverSameSession(t *testing.T) {
	const iterations = 200

	for it := 0; it < iterations; it++ {
		var audit bytes.Buffer
		relay, _ := buildRelay(t, true, &audit)

		workerID := "worker-race"
		sid, err := relay.Open(workerID, "admin-race", false)
		if err != nil {
			t.Fatalf("iter %d: Open: %v", it, err)
		}

		ch := relay.Subscribe(sid)
		if ch == nil {
			t.Fatalf("iter %d: nil Subscribe channel", it)
		}

		// Drain the admin channel so DeliverFromWorker sends don't all bounce off
		// a full buffer (we want real sends racing the close).
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			for range ch {
			}
		}()

		var wg sync.WaitGroup

		// Goroutine A: hammer DeliverFromWorker with exec_data on the SAME session.
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				f, _ := NewFrame("exec_data", 0, ExecPayload{
					SessionID: sid,
					Data:      []byte("x"),
				})
				// Must never panic (send-on-closed) and must be race-clean on the
				// timer even when CloseSession runs concurrently.
				relay.DeliverFromWorker(f)
			}
		}()

		// Goroutine B: hammer SendToWorker (admin→worker) which also touches the
		// timer — another concurrent timer accessor.
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = relay.SendToWorker(sid, []byte("y"))
			}
		}()

		// Goroutine C: close the session partway through the storm.
		wg.Add(1)
		go func() {
			defer wg.Done()
			relay.CloseSession(sid, nil)
		}()

		wg.Wait()
		// CloseSession closed the admin channel; the drain goroutine exits.
		<-drained

		// A redundant second close must be a clean idempotent no-op (no panic).
		relay.CloseSession(sid, nil)
	}
}

// TestExec_DefaultNoPTY verifies that pty=false in exec_open when Open is
// called with pty=false, and pty=true when called with pty=true.
func TestExec_DefaultNoPTY(t *testing.T) {
	tests := []struct {
		name    string
		pty     bool
		wantPty bool
	}{
		{"no-pty", false, false},
		{"with-pty", true, true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var audit bytes.Buffer
			relay, hub := buildRelay(t, true, &audit)

			workerID := "worker-pty-" + tc.name
			sid, err := relay.Open(workerID, "admin-1", tc.pty)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			// Clean up.
			t.Cleanup(func() { relay.CloseSession(sid, nil) })

			frames := hub.sentFrames(workerID)
			if len(frames) != 1 {
				t.Fatalf("want 1 frame sent, got %d", len(frames))
			}
			var payload ExecPayload
			if err := frames[0].Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if payload.Pty != tc.wantPty {
				t.Errorf("pty=%v, want %v", payload.Pty, tc.wantPty)
			}
		})
	}
}
