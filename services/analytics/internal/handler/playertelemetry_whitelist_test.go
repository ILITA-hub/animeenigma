package handler

import "testing"

// Player telemetry feeds the source-ranking GROUP BY target, so an
// unwhitelisted provider can inject arbitrary rows into the ranking aggregates
// (audit medium #2). Only the known roster is honored; everything else drops.
func TestWhitelistProvider(t *testing.T) {
	known := []string{
		"gogoanime", "animepahe", "allanime", "animefever", "miruro",
		"nineanime", "animekai", "kodik", "animelib", "hanime", "raw",
		"ae", "18anime",
	}
	for _, p := range known {
		if whitelistProvider(p) != p {
			t.Fatalf("known provider %q was dropped/altered", p)
		}
	}
	// case/whitespace normalization
	if whitelistProvider("  RAW ") != "raw" {
		t.Fatalf("provider not normalized to canonical lowercase")
	}
	for _, p := range []string{"", "evil", "'; DROP TABLE", "be", "http://x", "gogoanime\x00"} {
		if whitelistProvider(p) != "" {
			t.Fatalf("unknown/forged provider %q was NOT dropped", p)
		}
	}
}
