package handler

import "strings"

// knownProviders is the canonical stream-source roster. It gates player-telemetry
// ingestion (whitelistProvider) AND doubles as the playability scores-query roster
// filter (see service/playability.go). It is the UNION of:
//   - capability provider ids the FE sends as combo.provider on player-events
//     (these land in events.target), and
//   - probe_runs.provider names (PROBE_PROVIDERS), which use "kodik-noads" where
//     the capability id is "kodik".
// Keep in sync when a provider is added/removed.
var knownProviders = map[string]struct{}{
	// EN chain
	"gogoanime": {}, "allanime-okru": {}, "miruro": {}, "nineanime": {}, "animekai": {},
	// RU / adult / first-party / per-title
	"kodik": {}, "kodik-noads": {}, "animelib": {}, "hanime": {}, "ae": {}, "18anime": {},
	"animejoy-sibnet": {}, "animejoy-allvideo": {},
	// disabled tombstones (kept so any residual/legacy events still record)
	"allanime": {}, "animepahe": {}, "animefever": {},
}

// whitelistProvider returns the canonical (lowercased) provider key if it is in
// the known roster, else "". Player-telemetry provider becomes the
// source-ranking GROUP BY target, so an unwhitelisted value injects arbitrary
// rows into the ranking aggregates (audit medium #2).
func whitelistProvider(s string) string {
	k := strings.ToLower(strings.TrimSpace(s))
	if _, ok := knownProviders[k]; ok {
		return k
	}
	return ""
}
