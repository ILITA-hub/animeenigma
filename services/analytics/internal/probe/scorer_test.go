package probe

import (
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

// v builds a non-playable Verdict in the given slot with the given reason and a
// Server URL that exercises serverShortLabel.
func v(slot AnimeSlot, reason streamprobe.Reason) Verdict {
	return Verdict{Slot: slot, Stage: StagePlayback, Reason: reason, Server: "https://h/x?type=hd-1&y"}
}

// vPlay builds a playable Verdict in the given slot.
func vPlay(slot AnimeSlot) Verdict {
	return Verdict{Slot: slot, Stage: StagePlayback, Reason: streamprobe.ReasonPlayable, Server: "https://h/x?type=hd-1&y"}
}

// --- slot-based pass-percentage cases ---

func TestRollup_SlotMajority_Up(t *testing.T) {
	// 3 of 4 slots pass → ratio 0.75 > 0.5 → Up
	verdicts := []Verdict{
		vPlay(SlotAnchor),
		vPlay(SlotFeatured),
		vPlay(SlotSpotlightRandom),
		v(SlotRandom, streamprobe.ReasonStatus403),
	}
	pv := Rollup("p", verdicts)
	if pv.Status != StatusUp || pv.Reason != "" {
		t.Fatalf("got %+v, want StatusUp with no reason", pv)
	}
}

func TestRollup_ExactlyHalf_Degraded(t *testing.T) {
	// 2 of 4 slots pass → ratio 0.5 which is NOT > 0.5 → Degraded
	verdicts := []Verdict{
		vPlay(SlotAnchor),
		vPlay(SlotFeatured),
		v(SlotSpotlightRandom, streamprobe.ReasonStatus403),
		v(SlotRandom, streamprobe.ReasonStatus403),
	}
	pv := Rollup("p", verdicts)
	if pv.Status != StatusDegraded {
		t.Fatalf("2/4 slots should be Degraded, got %v", pv.Status)
	}
}

func TestRollup_OneOf4_Degraded(t *testing.T) {
	// 1 of 4 slots pass → ratio 0.25 → Degraded
	verdicts := []Verdict{
		vPlay(SlotAnchor),
		v(SlotFeatured, streamprobe.ReasonStatus403),
		v(SlotSpotlightRandom, streamprobe.ReasonStatus403),
		v(SlotRandom, streamprobe.ReasonStatus403),
	}
	pv := Rollup("p", verdicts)
	if pv.Status != StatusDegraded {
		t.Fatalf("1/4 slots should be Degraded, got %v", pv.Status)
	}
}

func TestRollup_ZeroOf4_Down(t *testing.T) {
	// 0 of 4 slots pass → ratio 0.0 → Down
	verdicts := []Verdict{
		v(SlotAnchor, streamprobe.ReasonStatus403),
		v(SlotFeatured, streamprobe.ReasonStatus403),
		v(SlotSpotlightRandom, streamprobe.ReasonCDNUnreachable),
		v(SlotRandom, streamprobe.ReasonCDNUnreachable),
	}
	pv := Rollup("p", verdicts)
	if pv.Status != StatusDown {
		t.Fatalf("0/4 slots should be Down, got %v", pv.Status)
	}
}

func TestRollup_AnyServerInSlotPlays_SlotPasses(t *testing.T) {
	// One non-playable + one playable in the same slot → slot passes → Up
	verdicts := []Verdict{
		v(SlotAnchor, streamprobe.ReasonStatus403),
		vPlay(SlotAnchor),
	}
	pv := Rollup("p", verdicts)
	if pv.Status != StatusUp {
		t.Fatalf("slot with any playable verdict should pass; got %v", pv.Status)
	}
}

func TestRollup_Empty(t *testing.T) {
	if Rollup("p", nil).Status != StatusDown {
		t.Fatal("empty must be Down")
	}
}

func TestRollup_TieBreakDeterministic(t *testing.T) {
	// Two non-playable reasons at equal counts; "cdn_unreachable" < "status_403"
	// lexicographically so cdn_unreachable should win the tie.
	verdicts := []Verdict{
		{Slot: SlotAnchor, Stage: StagePlayback, Reason: streamprobe.ReasonStatus403, Server: "https://example.com/x"},
		{Slot: SlotFeatured, Stage: StagePlayback, Reason: streamprobe.ReasonCDNUnreachable, Server: "https://cdn.example.com/y"},
	}
	pv := Rollup("test-provider", verdicts)
	if pv.Status != StatusDown {
		t.Fatalf("expected StatusDown, got %v", pv.Status)
	}
	expected := "cdn_unreachable on "
	if !strings.Contains(pv.Reason, expected) {
		t.Fatalf("expected reason to contain %q, got %q", expected, pv.Reason)
	}
}
