package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/creack/pty"

	"github.com/ILITA-hub/animeenigma/worker/internal/wire"
)

// execAllowlist is the set of commands allowed in non-PTY (allowlist) mode.
var execAllowlist = map[string]bool{
	"ls":         true,
	"df":         true,
	"free":       true,
	"ps":         true,
	"top":        true,
	"cat":        true,
	"echo":       true,
	"uptime":     true,
	"nvidia-smi": true,
	"rocm-smi":   true,
	"env":        true,
	"date":       true,
}

// ExecHandler handles exec_open frames from the server. It either runs a
// PTY shell session (Pty=true) or executes an allowlisted command (Pty=false).
// All operations are async — Handle returns immediately and the session runs
// in a goroutine.
//
// Each running session is tracked by SessionID so an admin-sent exec_close
// frame can tear it down (kill process + close PTY). Sessions are also torn
// down when the connection context passed to Handle is cancelled (WS drop /
// worker shutdown), so no orphaned process survives a reconnect.
type ExecHandler struct {
	// send is the outbound frame channel shared with the WS write pump.
	send chan<- []byte

	// mu guards sessions.
	mu sync.Mutex
	// sessions maps SessionID → cancel func for that session's context.
	// Calling the cancel func triggers teardown (CommandContext kills the
	// process; the PTY reader unblocks when ptmx is closed on ctx-cancel).
	sessions map[string]context.CancelFunc
}

// NewExecHandler constructs an ExecHandler.
func NewExecHandler(send chan<- []byte) *ExecHandler {
	return &ExecHandler{
		send:     send,
		sessions: make(map[string]context.CancelFunc),
	}
}

// Handle dispatches an exec_open payload. It runs asynchronously so as not to
// block the WS dispatch goroutine. The parent ctx is the connection context:
// when it is cancelled (WS drop / worker shutdown) the running process/PTY is
// killed and resources are released.
func (h *ExecHandler) Handle(ctx context.Context, payload wire.ExecPayload) {
	// Derive a per-session cancellable context from the connection context so
	// either a connection drop OR an admin exec_close tears the session down.
	sessionCtx, cancel := context.WithCancel(ctx)

	h.mu.Lock()
	h.sessions[payload.SessionID] = cancel
	h.mu.Unlock()

	go h.run(sessionCtx, cancel, payload)
}

// Close tears down the session identified by sessionID, if it is running.
// This is the handler for an admin-sent exec_close frame: it cancels the
// session context, which kills the process and closes the PTY. It is a no-op
// for unknown sessions.
func (h *ExecHandler) Close(sessionID string) {
	h.mu.Lock()
	cancel := h.sessions[sessionID]
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// run executes the session and emits exec_data + exec_close frames.
// cancel is the session's own cancel func; it is released (and the session
// deregistered) when the session ends so the map does not leak.
func (h *ExecHandler) run(ctx context.Context, cancel context.CancelFunc, payload wire.ExecPayload) {
	defer func() {
		cancel()
		h.mu.Lock()
		delete(h.sessions, payload.SessionID)
		h.mu.Unlock()
	}()

	if payload.Pty {
		h.runPty(ctx, payload)
	} else {
		h.runAllowlist(ctx, payload)
	}
}

// sendExecData enqueues an exec_data frame.
func (h *ExecHandler) sendExecData(sessionID string, data []byte) {
	f, err := wire.NewFrame("exec_data", 0, wire.ExecPayload{
		SessionID: sessionID,
		Data:      data,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec: build exec_data: %v\n", err)
		return
	}
	raw, err := json.Marshal(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec: marshal exec_data: %v\n", err)
		return
	}
	select {
	case h.send <- raw:
	default:
		fmt.Fprintf(os.Stderr, "exec: send channel full, dropping exec_data for session %s\n", sessionID)
	}
}

// sendExecClose enqueues an exec_close frame with the given exit code.
func (h *ExecHandler) sendExecClose(sessionID string, exitCode int) {
	ec := exitCode
	f, err := wire.NewFrame("exec_close", 0, wire.ExecPayload{
		SessionID: sessionID,
		ExitCode:  &ec,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec: build exec_close: %v\n", err)
		return
	}
	raw, err := json.Marshal(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec: marshal exec_close: %v\n", err)
		return
	}
	select {
	case h.send <- raw:
	default:
		fmt.Fprintf(os.Stderr, "exec: send channel full, dropping exec_close for session %s\n", sessionID)
	}
}

// runAllowlist handles a non-PTY exec request with allowlist enforcement.
//
// The command runs under exec.CommandContext(ctx, ...), so cancelling ctx
// (connection drop or admin exec_close) sends SIGKILL to the process group
// and CombinedOutput returns promptly — no orphaned process survives.
func (h *ExecHandler) runAllowlist(ctx context.Context, payload wire.ExecPayload) {
	cmdStr := strings.TrimSpace(string(payload.Data))
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		h.sendExecClose(payload.SessionID, 1)
		return
	}

	if !execAllowlist[parts[0]] {
		h.sendExecData(payload.SessionID, []byte(fmt.Sprintf("blocked: command %q is not allowed\n", parts[0])))
		h.sendExecClose(payload.SessionID, 1)
		return
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		h.sendExecData(payload.SessionID, out)
	}

	exitCode := 0
	if err != nil {
		exitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	h.sendExecClose(payload.SessionID, exitCode)
}

// runPty handles a PTY exec request. Spawns /bin/sh with a PTY, streams
// output as exec_data frames, and sends exec_close when the shell exits.
//
// If payload.Data is non-empty it is written to the PTY as initial input
// (e.g. a command to run). The PTY write side is NOT closed after the initial
// write — this keeps the session interactive so subsequent exec_data frames
// can stream more input. Teardown is driven exclusively by ctx cancellation
// (connection drop or admin exec_close): a watchdog goroutine closes ptmx,
// which makes the shell receive EOF/SIGHUP and exit, unblocking cmd.Wait.
func (h *ExecHandler) runPty(ctx context.Context, payload wire.ExecPayload) {
	cols := uint16(payload.Cols)
	rows := uint16(payload.Rows)
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	// exec.CommandContext so ctx-cancel also SIGKILLs the shell as a backstop
	// even if closing the PTY does not make it exit.
	cmd := exec.CommandContext(ctx, "/bin/sh")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
	if err != nil {
		// Fallback: send an error and close.
		h.sendExecData(payload.SessionID, []byte(fmt.Sprintf("pty: start failed: %v\n", err)))
		h.sendExecClose(payload.SessionID, 1)
		return
	}

	// Write initial data (if any) to the PTY so the shell can act on it.
	// Do NOT close the write side: the session stays open for interactive
	// streaming until ctx-cancel (connection drop / admin exec_close).
	if len(payload.Data) > 0 {
		ptmx.Write(payload.Data) //nolint:errcheck
		// Ensure a trailing newline so the shell executes the command.
		if payload.Data[len(payload.Data)-1] != '\n' {
			ptmx.Write([]byte{'\n'}) //nolint:errcheck
		}
	}

	// Watchdog: when ctx is cancelled (connection drop / admin exec_close),
	// close the PTY master. This unblocks the reader and makes the shell exit.
	// waitDone is closed once the shell has exited so the watchdog can return
	// without leaking when the session ends normally.
	waitDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			ptmx.Close() //nolint:errcheck — idempotent close; reader unblocks
		case <-waitDone:
			// Session ended normally; nothing to tear down.
		}
	}()

	// Read PTY output in a goroutine and forward as exec_data frames.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				h.sendExecData(payload.SessionID, chunk)
			}
			if rerr != nil {
				return
			}
		}
	}()

	// Wait for shell to exit (normal exit, or SIGKILL from ctx-cancel via
	// CommandContext, or EOF after the watchdog closes ptmx).
	exitCode := 0
	if werr := cmd.Wait(); werr != nil {
		exitCode = 1
		if ee, ok := werr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}

	// Signal the watchdog the session ended so it returns without leaking.
	close(waitDone)

	// Close the PTY master to unblock the reader goroutine on the normal path.
	// Safe to call even if the watchdog already closed it (close is idempotent
	// enough: a double Close returns an error we ignore).
	ptmx.Close() //nolint:errcheck

	// Wait for reader goroutine to drain.
	<-readDone

	h.sendExecClose(payload.SessionID, exitCode)
}
