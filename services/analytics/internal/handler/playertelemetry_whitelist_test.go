package handler

import "testing"

// rosterStub is a fixed-membership stub satisfying providerRoster, used by
// tests that don't need live catalog fetches. Case-insensitive to match the
// real roster.Client contract.
type rosterStub struct{ names map[string]struct{} }

func (r rosterStub) Known(name string) bool {
	_, ok := r.names[name]
	return ok
}

// oldStaticRoster mirrors the pre-AUTO-608 compile-time knownProviders set,
// used so existing whitelist expectations keep holding under the new
// roster-backed lookup.
func oldStaticRoster() rosterStub {
	return rosterStub{names: map[string]struct{}{
		"gogoanime": {}, "allanime-okru": {}, "miruro": {}, "nineanime": {}, "animekai": {},
		"kodik-noads": {}, "animelib": {}, "hanime": {}, "ae": {}, "18anime": {},
		"animejoy-sibnet": {}, "animejoy-allvideo": {},
		"allanime": {}, "animepahe": {}, "animefever": {},
	}}
}

// Player telemetry feeds the source-ranking GROUP BY target, so an
// unwhitelisted provider can inject arbitrary rows into the ranking aggregates
// (audit medium #2). Only the known roster (or a synthetic id) is honored;
// everything else drops.
func TestWhitelistProvider(t *testing.T) {
	roster := oldStaticRoster()
	known := []string{
		"gogoanime", "animepahe", "allanime", "animefever", "miruro",
		"nineanime", "animekai", "kodik", "animelib", "hanime",
		"ae", "18anime",
	}
	for _, p := range known {
		if whitelistProvider(p, roster) != p {
			t.Fatalf("known provider %q was dropped/altered", p)
		}
	}
	// case/whitespace normalization
	if whitelistProvider("  KODIK ", roster) != "kodik" {
		t.Fatalf("provider not normalized to canonical lowercase")
	}
	for _, p := range []string{"", "evil", "'; DROP TABLE", "be", "http://x", "gogoanime\x00"} {
		if whitelistProvider(p, roster) != "" {
			t.Fatalf("unknown/forged provider %q was NOT dropped", p)
		}
	}
}

func TestWhitelistProvider_CurrentRoster(t *testing.T) {
	roster := oldStaticRoster()
	// Capability ids the FE actually sends on player-events, plus the probe roster name.
	for _, p := range []string{
		"allanime-okru", "animejoy-sibnet", "animejoy-allvideo", "kodik-noads",
		"gogoanime", "miruro", "nineanime", "animekai", "kodik", "animelib", "hanime", "ae", "18anime",
	} {
		if got := whitelistProvider(p, roster); got != p {
			t.Errorf("whitelistProvider(%q) = %q, want %q (must be in roster)", p, got, p)
		}
	}
	if got := whitelistProvider("  AllAnime-OKRU ", roster); got != "allanime-okru" {
		t.Errorf("whitelistProvider trims+lowercases: got %q", got)
	}
	if got := whitelistProvider("evil-injected", roster); got != "" {
		t.Errorf("unknown provider must be dropped, got %q", got)
	}
}

// TestWhitelistProvider_RosterDriven is the AUTO-608 regression test: a
// provider that is NOT in the (frozen) old static set but IS a live roster
// row now passes, and one that is in neither is still dropped — proving the
// roster (not a compile-time map) is the membership authority.
func TestWhitelistProvider_RosterDriven(t *testing.T) {
	roster := rosterStub{names: map[string]struct{}{"brand-new-provider": {}}}
	if got := whitelistProvider("brand-new-provider", roster); got != "brand-new-provider" {
		t.Errorf("roster-known new provider must pass, got %q", got)
	}
	if got := whitelistProvider("gogoanime", roster); got != "" {
		t.Errorf("provider absent from roster must be dropped, got %q", got)
	}
	// Synthetic ids pass regardless of roster membership.
	if got := whitelistProvider("offline", roster); got != "offline" {
		t.Errorf("synthetic provider must pass without roster membership, got %q", got)
	}
	// nil roster: only synthetics pass.
	if got := whitelistProvider("kodik", nil); got != "kodik" {
		t.Errorf("synthetic provider must pass with nil roster, got %q", got)
	}
	if got := whitelistProvider("gogoanime", nil); got != "" {
		t.Errorf("non-synthetic provider must drop with nil roster, got %q", got)
	}
}
