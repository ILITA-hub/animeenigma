package animepahe

import (
	"net/url"
	"strconv"
	"strings"
	"time"
)

// streamTTLCap is the upper bound on cache TTL for an extracted Kwik HLS URL.
// RESEARCH.md Pitfall 6: Kwik signs the m3u8 with `expires=<unix>`. We cache
// for at most 5 minutes so a stale-but-not-yet-expired URL is naturally
// refreshed; tighter than `expires-30s` if the upstream expiry is far out.
const streamTTLCap = 5 * time.Minute

// streamTTLGuard is the safety margin subtracted from the upstream `expires`
// timestamp — we want to invalidate BEFORE the URL actually goes 403/410.
const streamTTLGuard = 30 * time.Second

// streamTTLFallback is used when the stream URL has no `expires=` query
// param. Best-effort fallback per RESEARCH.md Pitfall 6 — we still want to
// cache because re-running the Kwik extractor is expensive (HTTP fetch +
// goja unpack), but the cap is conservative so callers re-extract on the
// next miss if a stale URL surfaces.
const streamTTLFallback = streamTTLCap

// computeStreamTTL parses an `expires=<unix>` query param from a Kwik signed
// HLS URL and returns the cache TTL the provider should use. Returns 0 if
// the URL is already expired — the caller must NOT cache in that case (the
// next request would just serve a known-bad URL).
//
// Math (clamping):
//
//	expires - 30s - now      → headroom before upstream goes 4xx
//	min(headroom, 5min)      → caller-side cap
//	max(headroom, 0)         → never go negative
//
// Returns streamTTLFallback (5min) if the URL is unparseable or has no
// expires= param. This matches the documented best-effort behavior.
func computeStreamTTL(streamURL string, now time.Time) time.Duration {
	u, err := url.Parse(streamURL)
	if err != nil {
		return streamTTLFallback
	}
	expStr := u.Query().Get("expires")
	if expStr == "" {
		return streamTTLFallback
	}
	expSec, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return streamTTLFallback
	}
	exp := time.Unix(expSec, 0)
	headroom := exp.Sub(now) - streamTTLGuard
	if headroom <= 0 {
		return 0
	}
	if headroom > streamTTLCap {
		return streamTTLCap
	}
	return headroom
}

// normalizeTitle lower-cases, collapses whitespace, and folds the
// "Season N" / "Nth Season" / "Part N" variants into a single canonical
// form so the Jaro-Winkler scorer in the fuzzy fallback can compare
// season-suffixed titles meaningfully.
//
// Per RESEARCH.md Pitfall 5: real titles arrive as "Naruto" /
// "Naruto: Shippuuden" / "Vinland Saga Season 2" / "Vinland Saga: 2nd
// Season" — without normalization, the scorer drops below 0.85 for what
// a human would call a perfect match.
func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	// Common season suffix variants — fold to "season N".
	for _, pat := range []struct{ in, out string }{
		{" 2nd season", " season 2"},
		{" 3rd season", " season 3"},
		{" 4th season", " season 4"},
		{" 5th season", " season 5"},
		{" part 2", " season 2"},
		{" part 3", " season 3"},
		{" part 4", " season 4"},
		{" part 5", " season 5"},
	} {
		s = strings.ReplaceAll(s, pat.in, pat.out)
	}
	// Collapse separator punctuation into single spaces.
	s = strings.NewReplacer(":", " ", "·", " ", "—", " ", "–", " ", "-", " ").Replace(s)
	// Collapse runs of whitespace.
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// jaroWinkler returns a similarity score in [0,1] between two strings,
// using the standard Jaro-Winkler algorithm with prefix scale p=0.1 and
// prefix length capped at 4.
//
// Per RESEARCH.md Pitfall 5 + Assumption A6, the AnimePahe fuzzy-fallback
// threshold is 0.85. Stdlib only — no new deps.
func jaroWinkler(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	ar := []rune(a)
	br := []rune(b)

	// Match window: floor(max(la,lb)/2) - 1, but at least 0.
	window := len(ar)
	if len(br) > window {
		window = len(br)
	}
	window = window/2 - 1
	if window < 0 {
		window = 0
	}

	matchesA := make([]bool, len(ar))
	matchesB := make([]bool, len(br))
	matches := 0
	for i := 0; i < len(ar); i++ {
		lo := i - window
		if lo < 0 {
			lo = 0
		}
		hi := i + window + 1
		if hi > len(br) {
			hi = len(br)
		}
		for j := lo; j < hi; j++ {
			if matchesB[j] {
				continue
			}
			if ar[i] != br[j] {
				continue
			}
			matchesA[i] = true
			matchesB[j] = true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0.0
	}
	// Transpositions.
	k := 0
	transpositions := 0
	for i := 0; i < len(ar); i++ {
		if !matchesA[i] {
			continue
		}
		for !matchesB[k] {
			k++
		}
		if ar[i] != br[k] {
			transpositions++
		}
		k++
	}
	t := float64(transpositions) / 2.0
	m := float64(matches)
	jaro := (m/float64(len(ar)) + m/float64(len(br)) + (m-t)/m) / 3.0
	// Winkler boost: count common prefix up to 4 chars.
	prefix := 0
	limit := 4
	if limit > len(ar) {
		limit = len(ar)
	}
	if limit > len(br) {
		limit = len(br)
	}
	for i := 0; i < limit && ar[i] == br[i]; i++ {
		prefix++
	}
	return jaro + float64(prefix)*0.1*(1.0-jaro)
}
