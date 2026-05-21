// Workstream hero-spotlight v1.0 Phase 3 — Plan 02 Task 2.
//
// Tests for the generic 1-2-3 layout helper (HSB-BE-30).

package spotlight

import (
	"math/rand"
	"strings"
	"testing"
)

func TestAdaptiveSlice_Empty_ReturnsNil(t *testing.T) {
	got := AdaptiveSlice([]int{}, nil)
	if got != nil {
		t.Fatalf("expected nil for empty input, got %v", got)
	}
}

func TestAdaptiveSlice_NilInput_ReturnsNil(t *testing.T) {
	var items []int
	got := AdaptiveSlice(items, nil)
	if got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
}

func TestAdaptiveSlice_One_ReturnsAsIs(t *testing.T) {
	in := []int{42}
	got := AdaptiveSlice(in, nil)
	if len(got) != 1 || got[0] != 42 {
		t.Fatalf("expected [42], got %v", got)
	}
}

func TestAdaptiveSlice_Two_PicksOneViaRNG_Deterministic(t *testing.T) {
	in := []int{10, 20}
	// Seed=1 is stable across Go versions for rand.New + .Intn(2).
	got := AdaptiveSlice(in, rand.New(rand.NewSource(1)))
	if len(got) != 1 {
		t.Fatalf("expected len 1, got %d (%v)", len(got), got)
	}
	if got[0] != 10 && got[0] != 20 {
		t.Fatalf("expected element from input, got %v", got[0])
	}

	// Probe both branches: across many seeds, both index 0 AND index 1 must
	// be reachable (i.e. rng IS being consulted, not hardcoded).
	saw0 := false
	saw1 := false
	for seed := int64(0); seed < 50 && !(saw0 && saw1); seed++ {
		out := AdaptiveSlice(in, rand.New(rand.NewSource(seed)))
		switch out[0] {
		case 10:
			saw0 = true
		case 20:
			saw1 = true
		}
	}
	if !saw0 || !saw1 {
		t.Fatalf("rng pick must reach both indices across seeds — saw0=%v saw1=%v", saw0, saw1)
	}
}

func TestAdaptiveSlice_Three_ReturnsTopThree(t *testing.T) {
	in := []int{1, 2, 3, 4, 5}
	got := AdaptiveSlice(in, nil)
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("expected [1,2,3], got %v", got)
	}
}

func TestAdaptiveSlice_ExactThree_ReturnsAll(t *testing.T) {
	in := []int{7, 8, 9}
	got := AdaptiveSlice(in, nil)
	if len(got) != 3 || got[0] != 7 || got[1] != 8 || got[2] != 9 {
		t.Fatalf("expected [7,8,9], got %v", got)
	}
}

func TestAdaptiveSlice_TwoWithNilRNG_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when rng=nil and len==2, got no panic")
		}
		msg, ok := r.(string)
		if !ok {
			// Some panics wrap via runtime.Error/fmt.Errorf — check stringly.
			if e, isErr := r.(error); isErr {
				msg = e.Error()
				ok = true
			}
		}
		if !ok || !strings.Contains(msg, "rng is required") {
			t.Fatalf("expected panic message to contain 'rng is required', got %v", r)
		}
	}()
	_ = AdaptiveSlice([]int{1, 2}, nil)
}

func TestAdaptiveSlice_PreservesOrderForN3Plus(t *testing.T) {
	in := []string{"a", "b", "c", "d"}
	got := AdaptiveSlice(in, nil)
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("expected [a,b,c] (positional top-K, no shuffle), got %v", got)
	}
}

func TestAdaptiveSlice_GenericString_Works(t *testing.T) {
	// Proves the generic instantiation compiles + works for non-int types.
	in := []string{"hello"}
	got := AdaptiveSlice(in, nil)
	if len(got) != 1 || got[0] != "hello" {
		t.Fatalf("expected [hello], got %v", got)
	}
}
