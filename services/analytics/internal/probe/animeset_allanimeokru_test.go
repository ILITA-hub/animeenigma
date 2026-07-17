package probe

import (
	"context"
	"errors"
	"testing"
)

// fakeAnimeSet is a canned AnimeSetResolver for exercising decorators.
type fakeAnimeSet struct {
	refs []AnimeRef
	err  error
}

func (f fakeAnimeSet) Resolve(context.Context) ([]AnimeRef, error) { return f.refs, f.err }

func TestAllanimeOkruAnimeSet_OverridesAnchorSlot(t *testing.T) {
	inner := fakeAnimeSet{refs: []AnimeRef{
		{UUID: "frieren-uuid", Name: "Фрирен", Slot: SlotAnchor, Score: 9.27},
		{UUID: "feat-uuid", Name: "Featured", Slot: SlotFeatured, Score: 8.78},
		{UUID: "rand-uuid", Name: "Random", Slot: SlotRandom, Score: 8.32},
	}}
	as := NewAllanimeOkruAnimeSet(inner, "cat-dragon-uuid", "Кот и дракон")

	refs, err := as.Resolve(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("want 3 refs (override anchor + 2 passthrough), got %d: %+v", len(refs), refs)
	}
	if refs[0].UUID != "cat-dragon-uuid" || refs[0].Slot != SlotAnchor || refs[0].Name != "Кот и дракон" {
		t.Fatalf("refs[0] must be the override anchor, got %+v", refs[0])
	}
	// The shared anchor (Frieren) must not leak through under any slot.
	for _, r := range refs {
		if r.UUID == "frieren-uuid" {
			t.Fatalf("shared anchor leaked into output: %+v", refs)
		}
	}
}

// TestAllanimeOkruAnimeSet_OverrideWinsEvenAgainstHigherScoredSlots is the crux
// of the fix: engine.go's topPlayed/pass logic keys off refs[0] positionally,
// not AnimeRef.Slot, and the inner spotlight set sorts by Score descending. A
// naive UUID swap on the SlotAnchor ref would NOT reliably land at index 0
// once other slots (featured/random) outscore the override title — this test
// pins that the override always wins position 0 regardless of score.
func TestAllanimeOkruAnimeSet_OverrideWinsEvenAgainstHigherScoredSlots(t *testing.T) {
	inner := fakeAnimeSet{refs: []AnimeRef{
		{UUID: "frieren-uuid", Name: "Фрирен", Slot: SlotAnchor, Score: 9.27},
		{UUID: "feat-uuid", Name: "Featured", Slot: SlotFeatured, Score: 9.9}, // outscores the override
	}}
	as := NewAllanimeOkruAnimeSet(inner, "cat-dragon-uuid", "Кот и дракон")

	refs, err := as.Resolve(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refs[0].UUID != "cat-dragon-uuid" {
		t.Fatalf("override must occupy index 0 regardless of score, got %+v", refs)
	}
}

func TestAllanimeOkruAnimeSet_InnerErrorStillReturnsOverrideAnchor(t *testing.T) {
	inner := fakeAnimeSet{err: errors.New("spotlight unreachable")}
	as := NewAllanimeOkruAnimeSet(inner, "cat-dragon-uuid", "Кот и дракон")

	refs, err := as.Resolve(context.Background())
	if err != nil {
		t.Fatalf("wrapper must swallow inner error (anchor-only probing must not depend on spotlight), got %v", err)
	}
	if len(refs) != 1 || refs[0].UUID != "cat-dragon-uuid" || refs[0].Slot != SlotAnchor {
		t.Fatalf("want override-anchor-only refs, got %+v", refs)
	}
}
