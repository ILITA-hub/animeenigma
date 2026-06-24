package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
type ExecHandler struct {
	// send is the outbound frame channel shared with the WS write pump.
	send chan<- []byte
}

// NewExecHandler constructs an ExecHandler.
func NewExecHandler(send chan<- []byte) *ExecHandler {
	return &ExecHandler{send: send}
}

// Handle dispatches an exec_open payload. It runs asynchronously so as not to
// block the WS dispatch goroutine. ctx cancellation terminates the session.
func (h *ExecHandler) Handle(ctx context.Context, payload wire.ExecPayload) {
	go h.run(ctx, payload)
}

// run executes the session and emits exec_data + exec_close frames.
func (h *ExecHandler) run(ctx context.Context, payload wire.ExecPayload) {
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
// (e.g. "exit\n" to terminate a non-interactive session). The PTY master is
// closed after writing so the shell sees EOF on stdin and exits.
func (h *ExecHandler) runPty(ctx context.Context, payload wire.ExecPayload) {
	cols := uint16(payload.Cols)
	rows := uint16(payload.Rows)
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	cmd := exec.CommandContext(ctx, "/bin/sh")

	// Set PTY size if provided.
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
	// Then close the write side so the shell gets EOF on stdin.
	if len(payload.Data) > 0 {
		ptmx.Write(payload.Data) //nolint:errcheck
		// Ensure a newline so the shell executes the command.
		if payload.Data[len(payload.Data)-1] != '\n' {
			ptmx.Write([]byte{'\n'}) //nolint:errcheck
		}
	}

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

	// Wait for shell to exit or ctx to cancel.
	exitCode := 0
	if err := cmd.Wait(); err != nil {
		exitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}

	// Close the PTY master to unblock the reader goroutine.
	ptmx.Close()

	// Wait for reader goroutine to drain.
	<-readDone

	h.sendExecClose(payload.SessionID, exitCode)
}
