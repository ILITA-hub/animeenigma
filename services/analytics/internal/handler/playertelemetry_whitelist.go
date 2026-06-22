package handler

import "strings"

// knownProviders is the canonical stream-source roster (verified against the
// live analytics.events target values + the stream_providers seed). Keep in
// sync when a provider is added/removed.
var knownProviders = map[string]struct{}{
	"gogoanime": {}, "animepahe": {}, "allanime": {}, "animefever": {},
	"miruro": {}, "nineanime": {}, "animekai": {}, "kodik": {}, "animelib": {},
	"hanime": {}, "raw": {}, "ae": {}, "18anime": {},
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
