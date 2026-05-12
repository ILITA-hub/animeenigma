package main

// main_test.go — boot-time invariants for the scraper-api entrypoint.
//
// These tests pin behavior that is observable at startup before any probe
// tick lands. The bulk of cmd/scraper-api/main.go is wiring that requires
// a running Redis + HTTP server to exercise; the unit-testable surface is
// the small helpers like bootHealthSeedValue() that decide per-provider
// boot state.

import (
	"testing"
)

// TestBootHealthSeedValue_AnimeKaiSeedsZero locks in the Phase 19 CR-01
// invariant: when AnimeKai is registered (flag-on), its provider_health_up
// gauge children must be seeded with 0 — NOT the optimistic default of 1 —
// so Grafana never shows a green panel during the ~15 min before the first
// probe tick fires. The escape-hatch contract in animekai/client.go and
// .planning/phases/19-animekai-gated/19-REVIEW.md CR-01 depends on this.
func TestBootHealthSeedValue_AnimeKaiSeedsZero(t *testing.T) {
	if got := bootHealthSeedValue("animekai"); got != 0 {
		t.Fatalf("bootHealthSeedValue(\"animekai\") = %v; want 0 (escape-hatch invariant — Grafana must NOT show green at boot)", got)
	}
}

// TestBootHealthSeedValue_RealProvidersSeedOne — every non-escape-hatch
// provider keeps the optimistic Up=1 default so the "no probe yet, assume
// healthy" semantic from Plan 17-01 is preserved.
func TestBootHealthSeedValue_RealProvidersSeedOne(t *testing.T) {
	cases := []string{"animepahe", "gogoanime", "anything-future"}
	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			if got := bootHealthSeedValue(name); got != 1 {
				t.Fatalf("bootHealthSeedValue(%q) = %v; want 1 (real-provider default)", name, got)
			}
		})
	}
}
