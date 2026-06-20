package probe

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

func TestProviderVerdict_Status(t *testing.T) {
	if StatusUp.Gauge() != 1.0 || StatusDegraded.Gauge() != 0.5 || StatusDown.Gauge() != 0.0 {
		t.Fatalf("gauge mapping wrong")
	}
}

func TestVerdict_Playable(t *testing.T) {
	v := Verdict{Reason: streamprobe.ReasonPlayable}
	if !v.Playable() {
		t.Fatalf("expected playable")
	}
	if (Verdict{Reason: streamprobe.ReasonStatus403}).Playable() {
		t.Fatalf("403 must not be playable")
	}
}
