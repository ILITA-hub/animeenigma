package capability

import "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"

// deriveProviderView computes the FE-facing presentation of a provider from its
// DB row plus whether this title has content on it. policy=disabled rows are
// filtered out BEFORE this is called (they are never emitted). Phase 1:
// hasContent is true for every family member that survived family-level presence
// gating, except first-party `ae` whose hasContent is a live library lookup.
// Phase 2 feeds EN providers' hasContent from the reactive no_content cache.
//
//	manual policy         → degraded (hacker-only; selectable only in hacker mode)
//	auto + !content       → no_content (tinted, not selectable)
//	auto + recovering     → recovering (selectable)
//	auto + (up|down)      → active     (selectable; auto+down is the <24h grace window)
//
// IMPORTANT — read the LIVE (policy, health) authority, NOT the stored `status`
// column. The self-healing probe machine mutates policy/health on auto-demote
// (down>24h → manual) and auto-promote (recovering>24h → auto) but never re-writes
// the `status` column (it's only re-synced at migration), so `status` lags. The
// scraper failover gate consumes the live authority via WireStatus(); deriving
// the player feed from the SAME authority is what keeps the player, the failover
// chain, and the Grafana dashboard in lock-step (the whole point of this feature).
// `policy=manual` is exactly "pinned out of the auto chain" (admin soft-degrade OR
// the machine's auto-demote); `policy=auto + health=down` is the deliberate grace
// window where a transiently-canary-down provider STAYS selectable (live playback
// in the user's browser is the real test, not the coarse daily canary).
func deriveProviderView(row domain.ScraperProvider, hasContent bool) (state string, selectable, hackerOnly bool) {
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
