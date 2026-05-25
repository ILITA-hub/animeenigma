package cards

import "testing"

func TestPlatformStatsPools_LoadOK(t *testing.T) {
	if poolErr != nil {
		t.Fatalf("embedded pools failed to parse: %v", poolErr)
	}
	if len(parsedJokes.Taglines) == 0 {
		t.Fatal("no taglines")
	}
	if len(parsedJokes.UptimeQuips) == 0 {
		t.Fatal("no uptime quips")
	}
	if len(parsedJokes.Vibes) == 0 {
		t.Fatal("no vibes")
	}
	if len(parsedTiles) == 0 {
		t.Fatal("no tile entries")
	}
	for i, tl := range parsedTiles {
		if tl.Metric == "" || tl.Label == "" || len(tl.Windows) == 0 {
			t.Fatalf("tile %d incomplete: %+v", i, tl)
		}
	}
}
