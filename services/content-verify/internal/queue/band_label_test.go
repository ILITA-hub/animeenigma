package queue

import (
	"context"
	"testing"
)

func TestBandLabel(t *testing.T) {
	cases := map[Band]string{
		BandPinned:     "pinned",
		BandOngoing:    "ongoing",
		BandWatchedTop: "watched_top",
		BandIdle:       "idle",
	}
	for b, want := range cases {
		if got := b.Label(); got != want {
			t.Errorf("Band(%d).Label() = %q, want %q", b, got, want)
		}
	}
	// Out-of-range is defensive, must not panic and must not be empty.
	if Band(99).Label() == "" {
		t.Error("unknown band label must not be empty")
	}
}

// TestClaimSetsUnitBand covers the band plumb (spec task 1): the *Unit Claim
// returns must carry the same band bandedCandidates classified its
// candidate into — a later task's metrics read unit.Band.Label() downstream.
// Reuses newEngineFixture (single-candidate "o1" world) from engine_test.go
// rather than inventing a new fixture: pinning the SAME already-ongoing
// candidate mid-test (the same trick TestClaimPinBypassesCooldown uses) is
// enough to exercise both BandOngoing and BandPinned without a second
// fixture, since BandOf checks Pinned before Ongoing.
func TestClaimSetsUnitBand(t *testing.T) {
	f := newEngineFixture(t, false)
	ctx := context.Background()

	u, _, release, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		t.Fatal("expected a claimed unit")
	}
	if u.Band != BandOngoing {
		t.Fatalf("unpinned ongoing candidate: Band = %v, want BandOngoing", u.Band)
	}
	release()

	// Pin the same candidate — BandOf checks Pinned first, so the claimed
	// unit's band must flip to BandPinned even though "o1" is still ongoing.
	f.engine.pins = map[string]string{"o1": ""}
	u2, _, release2, err := f.engine.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u2 == nil {
		t.Fatal("expected a claimed unit for the pinned candidate")
	}
	if u2.Band != BandPinned {
		t.Fatalf("pinned candidate: Band = %v, want BandPinned", u2.Band)
	}
	release2()
}
