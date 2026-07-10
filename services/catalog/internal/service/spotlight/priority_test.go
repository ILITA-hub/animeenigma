package spotlight

import (
	"context"
	"testing"
)

// A resolver that returns a card with priority 0 must be normalized to 1.0;
// a resolver that sets its own priority (e.g. curated=1.5) is left untouched.
func TestAggregator_NormalizesCardPriority(t *testing.T) {
	resolvers := []Resolver{
		&fakeResolver{typ: "normal", card: &Card{Type: "normal"}},          // Priority 0 → 1.0
		&fakeResolver{typ: "weighted", card: &Card{Type: "weighted", Priority: 1.5}},
	}
	agg := NewAggregator(newFakeCache(), testLogger(t), resolvers)

	resp, err := agg.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	got := map[string]float64{}
	for _, c := range resp.Cards {
		got[c.Type] = c.Priority
	}
	if got["normal"] != 1.0 {
		t.Errorf("normal card priority = %v, want 1.0", got["normal"])
	}
	if got["weighted"] != 1.5 {
		t.Errorf("weighted card priority = %v, want 1.5", got["weighted"])
	}
}
