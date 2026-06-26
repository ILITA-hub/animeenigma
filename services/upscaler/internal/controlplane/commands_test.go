package controlplane

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// fakeHub captures Send calls and optionally returns an error.
type fakeHub struct {
	mu    sync.Mutex
	sends []struct {
		workerID string
		frame    Frame
	}
	err error
}

func (h *fakeHub) Send(workerID string, f Frame) error {
	h.mu.Lock()
	h.sends = append(h.sends, struct {
		workerID string
		frame    Frame
	}{workerID, f})
	h.mu.Unlock()
	return h.err
}

func TestIssuer_ValidCommands(t *testing.T) {
	t.Parallel()
	cmds := []string{"cancel", "drain", "shutdown", "reconfigure", "update"}
	for _, cmd := range cmds {
		cmd := cmd
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			hub := &fakeHub{}
			issuer := NewIssuer(hub)
			args := json.RawMessage(`{}`)
			if err := issuer.Issue("worker-1", cmd, args); err != nil {
				t.Fatalf("Issue(%q): unexpected error: %v", cmd, err)
			}
			hub.mu.Lock()
			defer hub.mu.Unlock()
			if len(hub.sends) != 1 {
				t.Fatalf("Issue(%q): expected 1 send; got %d", cmd, len(hub.sends))
			}
			got := hub.sends[0]
			if got.workerID != "worker-1" {
				t.Errorf("Issue(%q): workerID = %q; want worker-1", cmd, got.workerID)
			}
			if got.frame.Type != "command" {
				t.Errorf("Issue(%q): frame.Type = %q; want command", cmd, got.frame.Type)
			}
			var p CommandPayload
			if err := json.Unmarshal(got.frame.Payload, &p); err != nil {
				t.Fatalf("Issue(%q): unmarshal payload: %v", cmd, err)
			}
			if p.Cmd != cmd {
				t.Errorf("Issue(%q): payload.Cmd = %q; want %q", cmd, p.Cmd, cmd)
			}
		})
	}
}

func TestIssuer_InvalidCommand(t *testing.T) {
	t.Parallel()
	cases := []string{"exec", "rm -rf"}
	for _, cmd := range cases {
		cmd := cmd
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			hub := &fakeHub{}
			issuer := NewIssuer(hub)
			err := issuer.Issue("worker-1", cmd, json.RawMessage(`{}`))
			if err == nil {
				t.Fatalf("Issue(%q): expected error; got nil", cmd)
			}
			if !strings.Contains(err.Error(), "not allowed") && !strings.Contains(err.Error(), "whitelist") {
				t.Errorf("Issue(%q): error %q does not mention 'not allowed' or 'whitelist'", cmd, err.Error())
			}
			hub.mu.Lock()
			defer hub.mu.Unlock()
			if len(hub.sends) != 0 {
				t.Errorf("Issue(%q): expected 0 sends for invalid cmd; got %d", cmd, len(hub.sends))
			}
		})
	}
}

func TestIssuer_WorkerNotConnected(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{err: errWorkerNotFound}
	issuer := NewIssuer(hub)
	err := issuer.Issue("worker-missing", "cancel", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for not-connected worker; got nil")
	}
}

func TestIssuer_CancelDeliversToWorker(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{}
	issuer := NewIssuer(hub)
	if err := issuer.Issue("worker-abc", "cancel", json.RawMessage(`{"reason":"test"}`)); err != nil {
		t.Fatalf("Issue cancel: %v", err)
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.sends) == 0 {
		t.Fatal("expected at least one send; got 0")
	}
	if hub.sends[0].workerID != "worker-abc" {
		t.Errorf("send to workerID = %q; want worker-abc", hub.sends[0].workerID)
	}
	var p CommandPayload
	_ = json.Unmarshal(hub.sends[0].frame.Payload, &p)
	if p.Cmd != "cancel" {
		t.Errorf("payload.Cmd = %q; want cancel", p.Cmd)
	}
}
