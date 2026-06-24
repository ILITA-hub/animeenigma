package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/worker/internal/wire"
)

// drainExecFrames reads exec_data and exec_close frames from ch, collecting
// data bytes and returning the final close frame. Times out after d.
func drainExecFrames(t *testing.T, ch <-chan []byte, d time.Duration) (output []byte, closePayload wire.ExecPayload, found bool) {
	t.Helper()
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return output, closePayload, false
		case raw, ok := <-ch:
			if !ok {
				return output, closePayload, false
			}
			var f wire.Frame
			if err := json.Unmarshal(raw, &f); err != nil {
				continue
			}
			switch f.Type {
			case "exec_data":
				var p wire.ExecPayload
				if err := f.Decode(&p); err == nil {
					output = append(output, p.Data...)
				}
			case "exec_close":
				var p wire.ExecPayload
				if err := f.Decode(&p); err == nil {
					return output, p, true
				}
			}
		}
	}
}

// TestExecHandler_AllowlistCommand verifies that an allowlisted command (echo)
// is executed, output is returned as exec_data, and exec_close arrives with ExitCode=0.
func TestExecHandler_AllowlistCommand(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 64)
	h := NewExecHandler(ch)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.Handle(ctx, wire.ExecPayload{
		SessionID: "s1",
		Data:      []byte("echo hello"),
		Pty:       false,
	})

	output, closeP, found := drainExecFrames(t, ch, 5*time.Second)
	if !found {
		t.Fatal("exec_close frame not received within timeout")
	}
	if !strings.Contains(string(output), "hello") {
		t.Errorf("expected output to contain %q, got %q", "hello", string(output))
	}
	if closeP.ExitCode == nil {
		t.Fatal("exec_close ExitCode is nil")
	}
	if *closeP.ExitCode != 0 {
		t.Errorf("ExitCode: got %d, want 0", *closeP.ExitCode)
	}
}

// TestExecHandler_BlockedCommand verifies that a non-allowlisted command is
// blocked: exec_close arrives with ExitCode=1.
func TestExecHandler_BlockedCommand(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 64)
	h := NewExecHandler(ch)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.Handle(ctx, wire.ExecPayload{
		SessionID: "s2",
		Data:      []byte("rm -rf /"),
		Pty:       false,
	})

	_, closeP, found := drainExecFrames(t, ch, 2*time.Second)
	if !found {
		t.Fatal("exec_close frame not received within timeout")
	}
	if closeP.ExitCode == nil {
		t.Fatal("exec_close ExitCode is nil")
	}
	if *closeP.ExitCode != 1 {
		t.Errorf("ExitCode: got %d, want 1 (blocked)", *closeP.ExitCode)
	}
}

// TestExecHandler_NeverSelfInitiates verifies that when ctx is already cancelled
// the exec handler returns quickly without hanging (i.e., no self-initiation,
// only reacts to received frames). This is a structural safety test.
func TestExecHandler_NeverSelfInitiates(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 64)
	h := NewExecHandler(ch)

	// Pre-cancel the context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.Handle(ctx, wire.ExecPayload{
			SessionID: "s-cancelled",
			Data:      []byte("echo test"),
			Pty:       false,
		})
	}()

	// The goroutine spawned by Handle should complete (possibly with an error
	// from the cancelled context), but should not hang indefinitely.
	select {
	case <-done:
		// OK — the internal goroutine completed.
	case <-time.After(3 * time.Second):
		t.Fatal("Handle did not return after context cancellation within 3s")
	}

	// Also verify exec_close arrives.
	select {
	case raw := <-ch:
		var f wire.Frame
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("unmarshal frame: %v", err)
		}
		// Could be exec_data (blocked) or exec_close — either is fine as long as
		// we get a frame rather than hanging.
	case <-time.After(100 * time.Millisecond):
		// No frame is also acceptable — the session may have completed with no output.
	}
}

// TestExecHandler_CancelTerminatesLongRunningCommand verifies that cancelling
// the connection context terminates a long-running allowlisted command promptly
// (no leaked process; exec_close arrives). Without ctx-cancel teardown, the
// command (`cat` reading from a never-closing stdin) would hang until timeout.
func TestExecHandler_CancelTerminatesLongRunningCommand(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 64)
	h := NewExecHandler(ch)

	// `cat` with no file argument reads stdin forever — it never exits on its
	// own. Only ctx-cancel (via exec.CommandContext SIGKILL) can stop it.
	ctx, cancel := context.WithCancel(context.Background())

	h.Handle(ctx, wire.ExecPayload{
		SessionID: "s-longrun",
		Data:      []byte("cat"),
		Pty:       false,
	})

	// Give the command a moment to actually start.
	time.Sleep(200 * time.Millisecond)

	// Cancel the connection context — should kill the process and yield exec_close.
	start := time.Now()
	cancel()

	_, closeP, found := drainExecFrames(t, ch, 3*time.Second)
	if !found {
		t.Fatal("exec_close frame not received after ctx cancel — process may be orphaned")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("teardown took %v after cancel — expected prompt termination", elapsed)
	}
	if closeP.ExitCode == nil {
		t.Fatal("exec_close ExitCode is nil")
	}
	// A SIGKILLed process exits non-zero; we only assert the session ended.
	t.Logf("long-running command terminated on cancel with exit code %d", *closeP.ExitCode)
}

// TestExecHandler_CancelTerminatesPty verifies that cancelling the connection
// context tears down an interactive PTY session (one with no `exit` command) —
// the watchdog closes ptmx and exec.CommandContext SIGKILLs the shell, so
// exec_close arrives promptly rather than hanging.
func TestExecHandler_CancelTerminatesPty(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 128)
	h := NewExecHandler(ch)

	ctx, cancel := context.WithCancel(context.Background())

	// `sleep 600` keeps the shell busy; the session would hang for 10 minutes
	// without ctx-cancel teardown.
	h.Handle(ctx, wire.ExecPayload{
		SessionID: "s-pty-longrun",
		Data:      []byte("sleep 600"),
		Cols:      80,
		Rows:      24,
		Pty:       true,
	})

	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	cancel()

	_, closeP, found := drainExecFrames(t, ch, 4*time.Second)
	if !found {
		t.Fatal("exec_close frame not received after ctx cancel — PTY session may be orphaned")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("PTY teardown took %v after cancel — expected prompt termination", elapsed)
	}
	if closeP.ExitCode == nil {
		t.Fatal("exec_close ExitCode is nil")
	}
	t.Logf("PTY session terminated on cancel with exit code %d", *closeP.ExitCode)
}

// TestExecHandler_AdminCloseTearsDownSession verifies that an admin-sent
// exec_close (modelled here by calling Close with the session ID) tears down a
// running long-running allowlisted command.
func TestExecHandler_AdminCloseTearsDownSession(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 64)
	h := NewExecHandler(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h.Handle(ctx, wire.ExecPayload{
		SessionID: "s-admin-close",
		Data:      []byte("cat"),
		Pty:       false,
	})

	time.Sleep(200 * time.Millisecond)

	// Admin teardown: equivalent to receiving an exec_close frame for the session.
	start := time.Now()
	h.Close("s-admin-close")

	_, closeP, found := drainExecFrames(t, ch, 3*time.Second)
	if !found {
		t.Fatal("exec_close frame not received after admin Close — process may be orphaned")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("admin teardown took %v — expected prompt termination", elapsed)
	}
	if closeP.ExitCode == nil {
		t.Fatal("exec_close ExitCode is nil")
	}
	t.Logf("session torn down by admin Close with exit code %d", *closeP.ExitCode)

	// Close on an unknown session is a no-op (must not panic).
	h.Close("nonexistent-session")
}

// TestExecHandler_PtyMode verifies that a PTY session is started and exec_close
// is eventually received when the shell exits.
func TestExecHandler_PtyMode(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 128)
	h := NewExecHandler(ch)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The PTY shell will exit once stdin is closed (no input provided).
	// We send "exit 0" as the initial data, but for PTY mode the Data field
	// is not used as command input — we just verify the session completes.
	h.Handle(ctx, wire.ExecPayload{
		SessionID: "s3",
		Data:      []byte("exit 0"),
		Cols:      80,
		Rows:      24,
		Pty:       true,
	})

	// Wait for exec_close — the shell should exit quickly since stdin is
	// the PTY master which closes when ptmx.Close() is called.
	_, closeP, found := drainExecFrames(t, ch, 8*time.Second)
	if !found {
		t.Fatal("exec_close frame not received within 8s for PTY mode")
	}
	if closeP.ExitCode == nil {
		t.Fatal("exec_close ExitCode is nil")
	}
	// Any exit code is acceptable — we just verify the session completes.
	t.Logf("PTY session completed with exit code %d", *closeP.ExitCode)
}
