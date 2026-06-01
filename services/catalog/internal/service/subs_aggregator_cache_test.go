package service

import "testing"

// A transient provider failure must NOT be cached for the full TTL, otherwise
// one blip freezes a degraded "providers_down" result into the panel for hours
// (regression: That Time I Got Reincarnated as a Slime S4 showed
// "opensubtitles down" for 6h after a momentary OpenSubtitles timeout).
func TestSubsCacheTTL(t *testing.T) {
	full := &AggregateResponse{}
	if got := subsCacheTTL(full); got != fullSubsCacheTTL {
		t.Fatalf("full-success TTL = %v, want %v", got, fullSubsCacheTTL)
	}

	degraded := &AggregateResponse{ProvidersDown: []string{"opensubtitles"}}
	if got := subsCacheTTL(degraded); got != degradedSubsCacheTTL {
		t.Fatalf("degraded TTL = %v, want %v", got, degradedSubsCacheTTL)
	}

	if degradedSubsCacheTTL >= fullSubsCacheTTL {
		t.Fatalf("degraded TTL (%v) must be much shorter than full (%v) so failures self-heal",
			degradedSubsCacheTTL, fullSubsCacheTTL)
	}
}
