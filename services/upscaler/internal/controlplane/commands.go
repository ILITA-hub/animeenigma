package controlplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// ErrWorkerNotConnected is returned by Issuer.Issue when the target worker is
// not currently connected to the Hub.
var ErrWorkerNotConnected = errors.New("controlplane: worker not connected")

// hubSender is the interface Issuer uses to send frames to workers.
// It is satisfied by *Hub (see hub.go).
type hubSender interface {
	Send(workerID string, f Frame) error
}

// allowedCommands is the whitelist of commands that can be issued to workers.
// "exec" is intentionally excluded (reserved for Task 13).
var allowedCommands = map[string]bool{
	"cancel":      true,
	"drain":       true,
	"shutdown":    true,
	"reconfigure": true,
	"update":      true,
}

// allowedCommandList returns the whitelisted commands as a sorted, pipe-joined
// string for error messages. Derived from allowedCommands so the message can
// never drift from the actual whitelist.
func allowedCommandList() string {
	cmds := make([]string, 0, len(allowedCommands))
	for c := range allowedCommands {
		cmds = append(cmds, c)
	}
	sort.Strings(cmds)
	return strings.Join(cmds, "|")
}

// Issuer sends typed command frames to workers via the Hub.
type Issuer struct {
	hub hubSender
}

// NewIssuer constructs an Issuer backed by the given hub.
func NewIssuer(hub hubSender) *Issuer {
	return &Issuer{hub: hub}
}

// Issue validates cmd against the whitelist and delivers a command frame to
// workerID. Returns an error when cmd is not whitelisted, or when hub.Send
// fails (e.g., worker not connected, send buffer full).
func (is *Issuer) Issue(workerID, cmd string, args json.RawMessage) error {
	if !allowedCommands[cmd] {
		return fmt.Errorf("controlplane: command %q not allowed (whitelist: %s)", cmd, allowedCommandList())
	}
	f, err := NewFrame("command", 0, CommandPayload{Cmd: cmd, Args: args})
	if err != nil {
		return fmt.Errorf("controlplane: marshal command frame: %w", err)
	}
	if err := is.hub.Send(workerID, f); err != nil {
		if errors.Is(err, errWorkerNotFound) {
			return ErrWorkerNotConnected
		}
		return err
	}
	metrics.UpscaleCommandTotal.WithLabelValues(cmd).Inc()
	return nil
}
