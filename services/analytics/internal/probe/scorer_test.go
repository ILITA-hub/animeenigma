package probe

import (
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

func TestRollup_Up(t *testing.T) {
	pv := Rollup("p", []Verdict{{Reason: streamprobe.ReasonStatus403, Stage: StagePlayback}, {Reason: streamprobe.ReasonPlayable, Stage: StagePlayback}})
	if pv.Status != StatusUp || pv.Reason != "" {
		t.Fatalf("got %+v", pv)
	}
}

func TestRollup_Degraded(t *testing.T) {
	pv := Rollup("p", []Verdict{{Reason: streamprobe.ReasonStatus403, Stage: StagePlayback, Server: "x?type=hd-1&y"}})
	if pv.Status != StatusDegraded || pv.Reason != "status_403 on HD-1" {
		t.Fatalf("got %+v", pv)
	}
}

func TestRollup_Down(t *testing.T) {
	pv := Rollup("p", []Verdict{{Reason: streamprobe.ReasonCDNUnreachable, Stage: StageServers}})
	if pv.Status != StatusDown {
		t.Fatalf("got %+v", pv)
	}
}

func TestRollup_Empty(t *testing.T) {
	if Rollup("p", nil).Status != StatusDown {
		t.Fatalf("empty must be down")
	}
}

func TestRollup_TieBreakDeterministic(t *testing.T) {
	// Two non-playable reasons at equal counts; deterministic tie-break should pick the lexicographically smaller one
	verdicts := []Verdict{
		{Reason: streamprobe.ReasonStatus403, Stage: StagePlayback, Server: "https://example.com/x"},
		{Reason: streamprobe.ReasonCDNUnreachable, Stage: StagePlayback, Server: "https://cdn.example.com/y"},
	}
	pv := Rollup("test-provider", verdicts)

	// Both reasons have count 1; "cdn_unreachable" < "status_403" lexicographically
	expected := "cdn_unreachable on "
	if pv.Status != StatusDegraded {
		t.Fatalf("expected StatusDegraded, got %v", pv.Status)
	}
	if !strings.Contains(pv.Reason, expected) {
		t.Fatalf("expected reason to start with %q, got %q", expected, pv.Reason)
	}
}
