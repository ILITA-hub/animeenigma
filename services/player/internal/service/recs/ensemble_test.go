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
