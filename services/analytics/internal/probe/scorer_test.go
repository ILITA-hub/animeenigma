package probe

import (
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
