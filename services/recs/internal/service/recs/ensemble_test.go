package recs

import (
	"context"
	"errors"
	"math"
	"testing"
)

// fakeSignal is a deterministic SignalModule for unit tests.
type fakeSignal struct {
	id     SignalID
	scores map[AnimeID]RawScore
	err    error
}

func (f *fakeSignal) ID() SignalID                                 { return f.id }
func (f *fakeSignal) Precompute(_ context.Context, _ UserID) error { return nil }
func (f *fakeSignal) Score(_ context.Context, _ UserID, candidates []AnimeID) (map[AnimeID]RawScore, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[AnimeID]RawScore, len(candidates))
	for _, id := range candidates {
		if v, ok := f.scores[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func TestEnsemble_RankSingleSignal(t *testing.T) {
	s1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{"a": 0, "b": 5, "c": 10}}
	e := NewEnsemble([]WeightedSignal{{Module: s1, Weight: 1.0}})

	got, err := e.Rank(context.Background(), "user-1", []AnimeID{"a", "b", "c"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got)=%d want 3", len(got))
	}
	// After normalization: a=0, b=0.5, c=1. Weight 1.0 → final equals normalized.
	if got[0].AnimeID != "c" || got[1].AnimeID != "b" || got[2].AnimeID != "a" {
		t.Errorf("unexpected sort order: %v", got)
	}
	if math.Abs(got[0].Final-1.0) > 1e-6 {
		t.Errorf("got[0].Final=%v want 1.0", got[0].Final)
	}
	if math.Abs(got[1].Final-0.5) > 1e-6 {
		t.Errorf("got[1].Final=%v want 0.5", got[1].Final)
	}
}

func TestEnsemble_RankWeightedSum(t *testing.T) {
	s1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{"a": 0, "b": 10}}
	s2 := &fakeSignal{id: "s2", scores: map[AnimeID]RawScore{"a": 10, "b": 0}}
	e := NewEnsemble([]WeightedSignal{
		{Module: s1, Weight: 0.7},
		{Module: s2, Weight: 0.3},
	})

	got, err := e.Rank(context.Background(), "user-1", []AnimeID{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// After normalization s1: a=0, b=1. s2: a=1, b=0.
	// Weighted: a = 0.7*0 + 0.3*1 = 0.3. b = 0.7*1 + 0.3*0 = 0.7.
	// Expect b first, a second.
	if got[0].AnimeID != "b" || got[1].AnimeID != "a" {
		t.Fatalf("unexpected order: %v", got)
	}
	if math.Abs(got[0].Final-0.7) > 1e-6 {
		t.Errorf("got[0].Final=%v want 0.7", got[0].Final)
	}
	if math.Abs(got[1].Final-0.3) > 1e-6 {
		t.Errorf("got[1].Final=%v want 0.3", got[1].Final)
	}
	// Breakdown must include both signal contributions per anime.
	if _, ok := got[0].Breakdown["s1"]; !ok {
		t.Errorf("got[0].Breakdown missing s1")
	}
	if _, ok := got[0].Breakdown["s2"]; !ok {
		t.Errorf("got[0].Breakdown missing s2")
	}
}

func TestEnsemble_AllSignalsZero(t *testing.T) {
	// Cold-start: signal returns no entries for any candidate.
	cold := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{}}
	e := NewEnsemble([]WeightedSignal{{Module: cold, Weight: 1.0}})
	got, err := e.Rank(context.Background(), "user-1", []AnimeID{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, r := range got {
		if r.Final != 0 {
			t.Errorf("got %v: cold-start must produce zero, got %v", r.AnimeID, r.Final)
		}
	}
}

func TestEnsemble_RankEmptyCandidates(t *testing.T) {
	s1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{"a": 1}}
	e := NewEnsemble([]WeightedSignal{{Module: s1, Weight: 1.0}})

	got, err := e.Rank(context.Background(), "user-1", []AnimeID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got)=%d want 0", len(got))
	}
}

func TestEnsemble_PropagatesSignalError(t *testing.T) {
	want := errors.New("boom")
	bad := &fakeSignal{id: "s1", err: want}
	e := NewEnsemble([]WeightedSignal{{Module: bad, Weight: 1.0}})

	_, err := e.Rank(context.Background(), "user-1", []AnimeID{"a"})
	if !errors.Is(err, want) {
		t.Errorf("err=%v want %v", err, want)
	}
}

// ----------------------------------------------------------------------------
// Phase 14 (REC-ADMIN-01) — RankWithBreakdown tests.
//
// RankWithBreakdown is the admin-debug parallel API to Rank. For every
// candidate it surfaces:
//   - Raw       — pre-normalization output of each signal
//   - Breakdown — per-pool min-max normalized [0,1] per signal
//   - Weighted  — weight × normalized per signal
//   - Final     — sum of Weighted across all signals
//   - TopContributor — the signal_id with the largest Weighted contribution
//
// The existing Rank API is untouched; admin debug uses RankWithBreakdown.
// ----------------------------------------------------------------------------

func TestEnsemble_RankWithBreakdown_PerSignalContributions(t *testing.T) {
	s1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{"a": 0, "b": 10}}
	s2 := &fakeSignal{id: "s2", scores: map[AnimeID]RawScore{"a": 10, "b": 0}}
	e := NewEnsemble([]WeightedSignal{
		{Module: s1, Weight: 0.7},
		{Module: s2, Weight: 0.3},
	})

	got, err := e.RankWithBreakdown(context.Background(), "user-1", []AnimeID{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d want 2", len(got))
	}
	// b: s1 norm=1.0, s2 norm=0.0; weighted: 0.7, 0; final=0.7; top=s1
	// a: s1 norm=0.0, s2 norm=1.0; weighted: 0, 0.3; final=0.3; top=s2
	if got[0].AnimeID != "b" || got[1].AnimeID != "a" {
		t.Fatalf("unexpected sort order: %v", got)
	}
	if math.Abs(got[0].Final-0.7) > 1e-6 {
		t.Errorf("got[0].Final=%v want 0.7", got[0].Final)
	}
	if math.Abs(got[1].Final-0.3) > 1e-6 {
		t.Errorf("got[1].Final=%v want 0.3", got[1].Final)
	}
	// Top contributor: b's biggest weighted entry is s1 (0.7), a's is s2 (0.3).
	if got[0].TopContributor != "s1" {
		t.Errorf("got[0].TopContributor=%q want s1", got[0].TopContributor)
	}
	if got[1].TopContributor != "s2" {
		t.Errorf("got[1].TopContributor=%q want s2", got[1].TopContributor)
	}
	// Raw / Breakdown / Weighted maps populated for every signal.
	for _, key := range []SignalID{"s1", "s2"} {
		if _, ok := got[0].Raw[key]; !ok {
			t.Errorf("got[0].Raw missing %q", key)
		}
		if _, ok := got[0].Breakdown[key]; !ok {
			t.Errorf("got[0].Breakdown missing %q", key)
		}
		if _, ok := got[0].Weighted[key]; !ok {
			t.Errorf("got[0].Weighted missing %q", key)
		}
	}
	// Final equals sum of Weighted entries (within float epsilon).
	for _, row := range got {
		var sum float64
		for _, w := range row.Weighted {
			sum += w
		}
		if math.Abs(sum-row.Final) > 1e-9 {
			t.Errorf("row %s: sum(Weighted)=%v != Final=%v", row.AnimeID, sum, row.Final)
		}
	}
}

func TestEnsemble_RankWithBreakdown_SingleSignal(t *testing.T) {
	s1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{"a": 0, "b": 5, "c": 10}}
	e := NewEnsemble([]WeightedSignal{{Module: s1, Weight: 1.0}})

	got, err := e.RankWithBreakdown(context.Background(), "user-1", []AnimeID{"a", "b", "c"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got)=%d want 3", len(got))
	}
	for _, row := range got {
		if row.TopContributor != "s1" {
			t.Errorf("row %s: TopContributor=%q want s1 (only signal)", row.AnimeID, row.TopContributor)
		}
	}
	// Sort: c=1.0, b=0.5, a=0
	if got[0].AnimeID != "c" || got[1].AnimeID != "b" || got[2].AnimeID != "a" {
		t.Errorf("unexpected sort order: %v", got)
	}
}

func TestEnsemble_RankWithBreakdown_AllZeroDeterministicTopContributor(t *testing.T) {
	// All signals produce empty maps -> all candidates have Final=0,
	// Weighted=0 for every signal. TopContributor must be deterministic.
	// The implementation initializes topVal=-1 so the FIRST signal in the
	// registry always wins ties (including the all-zero case).
	cold1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{}}
	cold2 := &fakeSignal{id: "s2", scores: map[AnimeID]RawScore{}}
	e := NewEnsemble([]WeightedSignal{
		{Module: cold1, Weight: 0.5},
		{Module: cold2, Weight: 0.5},
	})

	got, err := e.RankWithBreakdown(context.Background(), "user-1", []AnimeID{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, row := range got {
		if row.Final != 0 {
			t.Errorf("row %s: Final=%v want 0 (cold-start)", row.AnimeID, row.Final)
		}
		if row.TopContributor != "s1" {
			t.Errorf("row %s: TopContributor=%q want s1 (deterministic tiebreak: first signal in registry)",
				row.AnimeID, row.TopContributor)
		}
	}
}

func TestEnsemble_RankWithBreakdown_EmptyPool(t *testing.T) {
	s1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{"a": 1}}
	e := NewEnsemble([]WeightedSignal{{Module: s1, Weight: 1.0}})

	got, err := e.RankWithBreakdown(context.Background(), "user-1", []AnimeID{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got)=%d want 0", len(got))
	}
}

func TestEnsemble_RankWithBreakdown_PropagatesSignalError(t *testing.T) {
	want := errors.New("boom")
	bad := &fakeSignal{id: "s1", err: want}
	e := NewEnsemble([]WeightedSignal{{Module: bad, Weight: 1.0}})

	_, err := e.RankWithBreakdown(context.Background(), "user-1", []AnimeID{"a"})
	if !errors.Is(err, want) {
		t.Errorf("err=%v want %v", err, want)
	}
}

func TestEnsemble_RankWithBreakdown_DescendingFinalSort(t *testing.T) {
	s1 := &fakeSignal{id: "s1", scores: map[AnimeID]RawScore{"a": 1, "b": 5, "c": 3, "d": 9}}
	e := NewEnsemble([]WeightedSignal{{Module: s1, Weight: 1.0}})

	got, err := e.RankWithBreakdown(context.Background(), "user-1", []AnimeID{"a", "b", "c", "d"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Final < got[i].Final {
			t.Errorf("not sorted desc by Final at i=%d: %v < %v", i, got[i-1].Final, got[i].Final)
		}
	}
}
