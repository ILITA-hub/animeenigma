package animepahe

import "time"

// computeStreamTTL returns the cache TTL for a Kwik HLS URL based on its
// `expires=` query param. RED stub.
func computeStreamTTL(streamURL string, now time.Time) time.Duration {
	panic("not implemented (RED)")
}

// normalizeTitle folds season-suffix variants for fuzzy matching. RED stub.
func normalizeTitle(s string) string {
	panic("not implemented (RED)")
}

// jaroWinkler returns the Jaro-Winkler similarity score for two strings. RED stub.
func jaroWinkler(a, b string) float64 {
	panic("not implemented (RED)")
}
