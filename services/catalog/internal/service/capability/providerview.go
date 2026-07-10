package capability

import "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"

// deriveProviderView computes the FE-facing presentation of a provider from its
// DB row plus whether this title has content on it. policy=disabled rows are
// filtered out BEFORE this is called (they are never emitted). Phase 1:
// hasContent is true for every family member that survived family-level presence
// gating, except first-party `ae` whose hasContent is a live library lookup.
// Phase 2 feeds EN providers' hasContent from the reactive no_content cache.
// Phase B adds per-title promotion (see thisAnimeWatch/promoteFloor below).
//
//	content + thisAnimeWatch>=floor → active (selectable, non-hacker) — PROMOTION,
//	                                   checked FIRST, overrides a manual policy for
//	                                   THIS title only (see below)
//	manual policy         → degraded (hacker-only; selectable only in hacker mode)
//	auto + !content       → no_content (tinted, not selectable)
//	auto + recovering     → recovering (selectable)
//	auto + (up|degraded|down) → active (selectable; degraded/down stay selectable —
//	                             live playback in the user's browser is the real test)
//
// IMPORTANT — read the LIVE (policy, health) authority, NOT the stored `status`
// column. The probe state machine mutates health (never policy — that is
// admin-only since the 2026-07-08 hysteresis redesign) but never re-writes
// the `status` column (it's only re-synced at migration), so `status` lags. The
// scraper failover gate consumes the live authority via WireStatus(); deriving
// the player feed from the SAME authority is what keeps the player, the failover
// chain, and the Grafana dashboard in lock-step (the whole point of this feature).
// `policy=manual` is exactly "pinned out of the auto chain" (admin soft-degrade,
// set by SQL); `policy=auto + health=down` is the deliberate grace where a
// canary-down provider STAYS selectable (live playback in the user's browser is
// the real test, not the coarse daily canary).
//
// thisAnimeWatch is the decayed recent-watch-success weight for THIS title on
// THIS provider (0 when analytics is unavailable or the provider has no
// recorded watches); promoteFloor is the single tunable threshold (playability.go).
// Promotion is guarded by hasContent — a no_content provider can't have real
// watches for this title anyway, so it never conflicts with the Phase-A gate.
func deriveProviderView(row domain.ScraperProvider, hasContent bool, thisAnimeWatch, promoteFloor float64) (state string, selectable, hackerOnly bool) {
	// Per-title promotion (B3/B4): a has-content provider with enough recent
	// this-title watch success flips to active/selectable regardless of a
	// manual policy. Guarded by hasContent so it never overrides Phase-A
	// no_content (a no_content provider can't have real watches anyway).
	if hasContent && thisAnimeWatch >= promoteFloor {
		return "active", true, false
	}
	if row.Policy == domain.PolicyManual {
		return "degraded", true, true
	}
	if !hasContent {
		return "no_content", false, false
	}
	if row.Health == domain.HealthRecovering {
		return "recovering", true, false
	}
	return "active", true, false
}

// audiosFromTraits lists the audio kinds a provider serves, sub before dub
// (stable for the FE filter). The player's combo audio model is binary
// (sub/dub) — original audio surfaces under sub — so SupportsRaw is a recorded
// trait that does NOT add a third selectable audio kind to the feed.
func audiosFromTraits(row domain.ScraperProvider) []string {
	out := make([]string, 0, 2)
	if row.SupportsSub {
		out = append(out, "sub")
	}
	if row.SupportsDub {
		out = append(out, "dub")
	}
	return out
}

// wireGroup normalizes the DB group for the wire. Empty defaults to "en".
func wireGroup(g string) string {
	if g == "" {
		return "en"
	}
	return g
}
