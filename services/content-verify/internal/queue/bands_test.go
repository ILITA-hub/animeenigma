package queue

import (
	"testing"
	"time"
)

func TestBandOf(t *testing.T) {
	now := time.Now()
	_ = now
	cases := []struct {
		c    Candidate
		want Band
	}{
		{Candidate{Pinned: true, Ongoing: true}, BandPinned},
		{Candidate{Ongoing: true}, BandOngoing},
		{Candidate{Top: true}, BandWatchedTop},
		{Candidate{Visitors: 3}, BandWatchedTop},
		{Candidate{}, BandIdle},
		{Candidate{Idle: true, Planners: 5}, BandIdle},
	}
	for i, c := range cases {
		if got := BandOf(c.c); got != c.want {
			t.Errorf("case %d: BandOf=%d want %d", i, got, c.want)
		}
	}
}

func TestIntraLessOngoingFreshFirst(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	soon := now.Add(6 * time.Hour)
	far := now.Add(20 * 24 * time.Hour)
	fresh := Candidate{Ongoing: true, NextEpisodeAt: &soon, Visitors: 1, MalScore: 5}
	stale := Candidate{Ongoing: true, NextEpisodeAt: &far, Visitors: 9, MalScore: 9}
	// Fresh beats a higher-visitor stale one within the 48h window.
	if !IntraLess(fresh, stale, now, 48*time.Hour) {
		t.Fatal("fresh ongoing should sort before stale")
	}
}

func TestIntraLessWatchedByVisitorsThenRank(t *testing.T) {
	now := time.Now()
	a := Candidate{Visitors: 5, TopRank: 50}
	b := Candidate{Visitors: 2, TopRank: 1}
	if !IntraLess(a, b, now, time.Hour) {
		t.Fatal("more visitors should win in Band 2")
	}
	// Top:true is required alongside TopRank here — BandOf only classifies
	// BandWatchedTop via Visitors>0 or Top (production always sets both
	// together in BuildCandidates' it.Top loop; TopRank alone is not enough).
	c := Candidate{Visitors: 0, Top: true, TopRank: 10}
	d := Candidate{Visitors: 0, Top: true, TopRank: 0, MalScore: 9}
	if !IntraLess(c, d, now, time.Hour) {
		t.Fatal("ranked should sort before unranked in Band 2")
	}
}

func TestWeightedPick(t *testing.T) {
	w := [3]int{60, 30, 10}
	if weightedPick(w, 0.0) != BandOngoing {
		t.Error("0.0 → Band1")
	}
	if weightedPick(w, 0.7) != BandWatchedTop {
		t.Error("0.7 → Band2")
	}
	if weightedPick(w, 0.95) != BandIdle {
		t.Error("0.95 → Band3")
	}
}

func TestBandOrderPinnedFirstThenPrimaryThenRest(t *testing.T) {
	order := bandOrder([3]int{60, 30, 10}, 0.95) // primary = Band3
	if order[0] != BandPinned || order[1] != BandIdle {
		t.Fatalf("order=%v", order)
	}
	// remaining bands present in fixed priority
	if len(order) != 4 {
		t.Fatalf("order len=%d", len(order))
	}
}

func TestCooldownTTLByBand(t *testing.T) {
	idle := 168 * time.Hour
	if CooldownTTL(BandOngoing, idle) != 6*time.Hour {
		t.Error("ongoing 6h")
	}
	if CooldownTTL(BandWatchedTop, idle) != 24*time.Hour {
		t.Error("watched 24h")
	}
	if CooldownTTL(BandIdle, idle) != idle {
		t.Error("idle = CV_IDLE_COOLDOWN")
	}
}
