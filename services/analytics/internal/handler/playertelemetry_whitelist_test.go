package handler

import "testing"

// Player telemetry feeds the source-ranking GROUP BY target, so an
// unwhitelisted provider can inject arbitrary rows into the ranking aggregates
// (audit medium #2). Only the known roster is honored; everything else drops.
func TestWhitelistProvider(t *testing.T) {
	known := []string{
		"gogoanime", "animepahe", "allanime", "animefever", "miruro",
		"nineanime", "animekai", "kodik", "animelib", "hanime",
		"ae", "18anime",
	}
	for _, p := range known {
		if whitelistProvider(p) != p {
			t.Fatalf("known provider %q was dropped/altered", p)
		}
	}
	// case/whitespace normalization
	if whitelistProvider("  KODIK ") != "kodik" {
		t.Fatalf("provider not normalized to canonical lowercase")
	}
	for _, p := range []string{"", "evil", "'; DROP TABLE", "be", "http://x", "gogoanime\x00"} {
		if whitelistProvider(p) != "" {
			t.Fatalf("unknown/forged provider %q was NOT dropped", p)
		}
	}
}

func TestWhitelistProvider_CurrentRoster(t *testing.T) {
	// Capability ids the FE actually sends on player-events, plus the probe roster name.
	for _, p := range []string{
		"allanime-okru", "animejoy-sibnet", "animejoy-allvideo", "kodik-noads",
		"gogoanime", "miruro", "nineanime", "animekai", "kodik", "animelib", "hanime", "ae", "18anime",
	} {
		if got := whitelistProvider(p); got != p {
			t.Errorf("whitelistProvider(%q) = %q, want %q (must be in roster)", p, got, p)
		}
	}
	if got := whitelistProvider("  AllAnime-OKRU "); got != "allanime-okru" {
		t.Errorf("whitelistProvider trims+lowercases: got %q", got)
	}
	if got := whitelistProvider("evil-injected"); got != "" {
		t.Errorf("unknown provider must be dropped, got %q", got)
	}
}
