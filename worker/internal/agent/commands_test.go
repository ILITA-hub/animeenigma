package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestCommandHandler_Cancel verifies that Handle("cancel") cancels the context.
func TestCommandHandler_Cancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	h := NewCommandHandler(cancel)

	if err := h.Handle("cancel", nil); err != nil {
		t.Fatalf("Handle cancel: %v", err)
	}

	select {
	case <-ctx.Done():
		// Expected: context was cancelled.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context was not cancelled after Handle(\"cancel\")")
	}
}

// TestCommandHandler_Drain verifies that Handle("drain") closes DrainCh (idempotent).
func TestCommandHandler_Drain(t *testing.T) {
	t.Parallel()

	h := NewCommandHandler(nil)

	// Call drain twice — must not panic (idempotent close).
	if err := h.Handle("drain", nil); err != nil {
		t.Fatalf("Handle drain (1st): %v", err)
	}
	if err := h.Handle("drain", nil); err != nil {
		t.Fatalf("Handle drain (2nd): %v", err)
	}

	// DrainCh must be closed.
	select {
	case <-h.DrainCh:
		// OK
	default:
		t.Fatal("DrainCh is not closed after Handle(\"drain\")")
	}
}

// TestCommandHandler_Shutdown verifies that Handle("shutdown") closes both
// DrainCh and ShutdownCh.
func TestCommandHandler_Shutdown(t *testing.T) {
	t.Parallel()

	h := NewCommandHandler(nil)

	if err := h.Handle("shutdown", nil); err != nil {
		t.Fatalf("Handle shutdown: %v", err)
	}

	select {
	case <-h.DrainCh:
		// OK
	default:
		t.Fatal("DrainCh is not closed after Handle(\"shutdown\")")
	}

	select {
	case <-h.ShutdownCh:
		// OK
	default:
		t.Fatal("ShutdownCh is not closed after Handle(\"shutdown\")")
	}
}

// TestCommandHandler_Reconfigure verifies that Handle("reconfigure") updates
// the knobs correctly.
func TestCommandHandler_Reconfigure(t *testing.T) {
	t.Parallel()

	h := NewCommandHandler(nil)

	args, err := json.Marshal(map[string]any{
		"log_verbosity":        "debug",
		"heartbeat_interval_ms": 250,
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	if err := h.Handle("reconfigure", json.RawMessage(args)); err != nil {
		t.Fatalf("Handle reconfigure: %v", err)
	}

	h.mu.Lock()
	logV := h.LogVerbosity
	hbInt := h.HeartbeatInterval
	h.mu.Unlock()

	if logV != "debug" {
		t.Errorf("LogVerbosity: got %q, want %q", logV, "debug")
	}
	if hbInt != 250*time.Millisecond {
		t.Errorf("HeartbeatInterval: got %v, want 250ms", hbInt)
	}
}

// TestCommandHandler_Update verifies that Handle("update") closes ShutdownCh.
func TestCommandHandler_Update(t *testing.T) {
	t.Parallel()

	h := NewCommandHandler(nil)

	if err := h.Handle("update", nil); err != nil {
		t.Fatalf("Handle update: %v", err)
	}

	select {
	case <-h.ShutdownCh:
		// OK
	default:
		t.Fatal("ShutdownCh is not closed after Handle(\"update\")")
	}
}

// TestCommandHandler_UnknownCmd verifies that Handle returns a non-nil error for
// unknown commands and the error message contains the command name.
func TestCommandHandler_UnknownCmd(t *testing.T) {
	t.Parallel()

	h := NewCommandHandler(nil)

	err := h.Handle("frobulate", nil)
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
	if !strings.Contains(err.Error(), "frobulate") {
		t.Errorf("error message %q does not contain command name %q", err.Error(), "frobulate")
	}
}
