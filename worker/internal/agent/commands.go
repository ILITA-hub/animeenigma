package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// reconfigureArgs is the JSON shape for "reconfigure" command args.
type reconfigureArgs struct {
	LogVerbosity        string `json:"log_verbosity"`
	HeartbeatIntervalMs int    `json:"heartbeat_interval_ms"`
}

// CommandHandler handles server-sent command frames. It holds the current segment's
// cancel func and coordinates drain/shutdown with the lease loop.
type CommandHandler struct {
	cancel func()

	// drainOnce and shutdownOnce ensure idempotent channel closes.
	drainOnce    sync.Once
	shutdownOnce sync.Once

	// DrainCh is closed when a drain command is received.
	// The lease loop should select on this to stop accepting new work.
	DrainCh chan struct{}

	// ShutdownCh is closed after drain to signal the worker should exit.
	ShutdownCh chan struct{}

	// mu guards reconfigurable knobs.
	mu sync.Mutex

	// LogVerbosity is the current log verbosity level.
	LogVerbosity string

	// HeartbeatInterval is the current heartbeat interval.
	HeartbeatInterval time.Duration
}

// NewCommandHandler constructs a CommandHandler. cancelFn is called by Cancel();
// pass context.CancelFunc from the current segment's context, or a no-op.
func NewCommandHandler(cancelFn context.CancelFunc) *CommandHandler {
	if cancelFn == nil {
		cancelFn = func() {}
	}
	return &CommandHandler{
		cancel:     cancelFn,
		DrainCh:    make(chan struct{}),
		ShutdownCh: make(chan struct{}),
	}
}

// SetCancel replaces the cancel func (called when a new segment starts).
func (h *CommandHandler) SetCancel(cancelFn context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cancelFn == nil {
		cancelFn = func() {}
	}
	h.cancel = cancelFn
}

// Handle dispatches a command by name. Unknown commands return an error.
func (h *CommandHandler) Handle(cmd string, args json.RawMessage) error {
	switch cmd {
	case "cancel":
		h.Cancel()
		return nil
	case "drain":
		h.Drain()
		return nil
	case "shutdown":
		h.Shutdown()
		return nil
	case "reconfigure":
		return h.Reconfigure(args)
	case "update":
		h.Update()
		return nil
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// Cancel cancels the current segment's context.
func (h *CommandHandler) Cancel() {
	h.mu.Lock()
	fn := h.cancel
	h.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Drain closes DrainCh once, signalling no new leases should be accepted.
func (h *CommandHandler) Drain() {
	h.drainOnce.Do(func() {
		close(h.DrainCh)
	})
}

// Shutdown drains and then closes ShutdownCh once.
func (h *CommandHandler) Shutdown() {
	h.Drain()
	h.shutdownOnce.Do(func() {
		close(h.ShutdownCh)
	})
}

// Reconfigure parses the reconfigure args JSON and updates knobs.
func (h *CommandHandler) Reconfigure(raw json.RawMessage) error {
	var a reconfigureArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return fmt.Errorf("reconfigure: parse args: %w", err)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if a.LogVerbosity != "" {
		h.LogVerbosity = a.LogVerbosity
	}
	if a.HeartbeatIntervalMs > 0 {
		h.HeartbeatInterval = time.Duration(a.HeartbeatIntervalMs) * time.Millisecond
	}
	return nil
}

// Update drains and shuts down — the orchestrator will replace the container.
func (h *CommandHandler) Update() {
	h.Shutdown()
}
