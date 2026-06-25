package capability

import "github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"

// deriveProviderView computes the FE-facing presentation of a provider from its
// DB row plus whether this title has content on it. policy=disabled rows are
// filtered out BEFORE this is called (they are never emitted). Phase 1:
// hasContent is true for every family member that survived family-level presence
// gating, except first-party `ae` whose hasContent is a live library lookup.
// Phase 2 feeds EN providers' hasContent from the reactive no_content cache.
//
//	degraded            → hacker-only (selectable only when hacker mode is on)
//	enabled + !content  → no_content (tinted, not selectable)
//	enabled + recovering→ recovering (selectable)
//	enabled + (up|down) → active     (selectable; status keeps it in the chain)
func deriveProviderView(row domain.ScraperProvider, hasContent bool) (state string, selectable, hackerOnly bool) {
	if row.IsDegraded() {
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
// before raw (stable for the FE filter).
func audiosFromTraits(row domain.ScraperProvider) []string {
	out := make([]string, 0, 3)
	if row.SupportsSub {
		out = append(out, "sub")
	}
	if row.SupportsDub {
		out = append(out, "dub")
	}
	if row.SupportsRaw {
		out = append(out, "raw")
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
