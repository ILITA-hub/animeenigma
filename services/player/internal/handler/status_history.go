// Package handler — status_history.go: append-only triage-transition log.
//
// Every status change (admin SetStatus + the bot's SetStatusInternal) appends
// {from, to, at, by} to a per-report list in a sidecar
// `_status_history.json` next to `_status.json`. Best-effort: a history
// write failure is logged but never fails the status change itself —
// `_status.json` stays the source of truth for the CURRENT status. History
// starts at deploy time; pre-existing reports show transitions from their
// next change onward.
//
// Exposure: full entries (incl. `by`) on the admin detail; a `by`-stripped
// shape on the user's own rows (ListMine) — triage actor names are internal.
package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	statusHistoryFileName = "_status_history.json"
	// maxHistoryPerReport bounds a pathological flip-flop loop (e.g. a
	// misbehaving bot); oldest entries are dropped first.
	maxHistoryPerReport = 100
)

// statusTransition is one triage hop. At is RFC3339 UTC.
type statusTransition struct {
	From string `json:"from"`
	To   string `json:"to"`
	At   string `json:"at"`
	By   string `json:"by,omitempty"`
}

// userStatusTransition is the submitter-visible shape — no actor name.
type userStatusTransition struct {
	From string `json:"from"`
	To   string `json:"to"`
	At   string `json:"at"`
}

func (h *AdminReportsHandler) historyPath() string {
	return filepath.Join(h.reportsDir, statusHistoryFileName)
}

// loadHistory reads the sidecar. Missing/corrupt → empty map. Caller holds h.mu.
func (h *AdminReportsHandler) loadHistory() map[string][]statusTransition {
	out := map[string][]statusTransition{}
	data, err := os.ReadFile(h.historyPath())
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

// appendHistory records one transition. Caller holds h.mu. Best-effort:
// returns nothing; failures are logged by the caller's logger via the error.
func (h *AdminReportsHandler) appendHistory(id string, tr statusTransition) {
	hist := h.loadHistory()
	entries := append(hist[id], tr)
	if len(entries) > maxHistoryPerReport {
		entries = entries[len(entries)-maxHistoryPerReport:]
	}
	hist[id] = entries
	data, err := json.MarshalIndent(hist, "", "  ")
	if err == nil {
		err = os.WriteFile(h.historyPath(), data, 0600)
	}
	if err != nil {
		h.log.Warnw("failed to persist feedback status history", "id", id, "error", err)
	}
}

// historyFor returns a report's transitions (oldest first). Caller holds h.mu.
func (h *AdminReportsHandler) historyFor(id string) []statusTransition {
	return h.loadHistory()[id]
}

func toUserTransitions(in []statusTransition) []userStatusTransition {
	if len(in) == 0 {
		return nil
	}
	out := make([]userStatusTransition, len(in))
	for i, tr := range in {
		out[i] = userStatusTransition{From: tr.From, To: tr.To, At: tr.At}
	}
	return out
}
