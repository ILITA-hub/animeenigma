package gogoanime

import (
	"net/url"
	"strconv"
	"time"
)

// streamTTLCap is the upper bound on cache TTL for an extracted HLS/MP4 URL.
// RESEARCH.md Pitfall 3: StreamHG / Earnvids sign m3u8 URLs with
// `&e=<seconds_to_live>` (sometimes paired with `&s=<unix_signed_at>`).
// 5min is the cap so a stale-but-not-yet-expired URL is naturally refreshed.
const streamTTLCap = 5 * time.Minute

// streamTTLGuard is the safety margin subtracted from the upstream expiry —
// invalidate BEFORE the URL actually goes 403/410.
const streamTTLGuard = 30 * time.Second

// streamTTLFallback is used when the stream URL has no `e=` query param at
// all (vibeplayer-style — static, unsigned m3u8). Best-effort fallback per
// RESEARCH.md Pitfall 3 — we still want to cache because re-running the
// extractor is expensive, but the cap is conservative so callers re-extract
// on the next miss if a stale URL surfaces.
const streamTTLFallback = streamTTLCap

// computeStreamTTL parses signed-URL expiry semantics used by StreamHG and
// Earnvids: query param `e=<seconds_to_live>` (delta, not absolute Unix ts).
// Returns:
//   - streamTTLFallback when no `e=` query param is present (vibeplayer
//     static-m3u8 case).
//   - When `s=<unix_signed_at>` is also present, treat (s+e)-now as the
//     headroom (absolute expiry semantics).
//   - When only `e=<delta>` is present, treat e as a delta-from-now.
//   - 0 when the resulting headroom (minus 30s guard) is non-positive — the
//     caller must NOT cache in that case (the cached URL would just be a
//     known-bad URL).
//
// NOTE: This differs from animepahe/cache.go::computeStreamTTL which parses
// an absolute `expires=<unix>` timestamp. The StreamHG/Earnvids signed URLs
// emit `e=<delta_seconds>` (and optionally `s=<unix_signed_at>`), NOT
// `e=<unix_ts>`.
func computeStreamTTL(streamURL string, now time.Time) time.Duration {
	u, err := url.Parse(streamURL)
	if err != nil {
		return streamTTLFallback
	}
	q := u.Query()
	eStr := q.Get("e")
	if eStr == "" {
		return streamTTLFallback
	}
	eSec, err := strconv.ParseInt(eStr, 10, 64)
	if err != nil || eSec <= 0 {
		return streamTTLFallback
	}
	// Prefer the (s + e) absolute-expiry interpretation when s= is present
	// and parses. Falls back to the delta-from-now interpretation otherwise.
	var headroom time.Duration
	usedAbsolute := false
	if sStr := q.Get("s"); sStr != "" {
		if sUnix, sErr := strconv.ParseInt(sStr, 10, 64); sErr == nil && sUnix > 0 {
			expiry := time.Unix(sUnix+eSec, 0)
			headroom = expiry.Sub(now) - streamTTLGuard
			usedAbsolute = true
		}
	}
	if !usedAbsolute {
		// s= absent (or unparseable): treat e as delta-from-now.
		headroom = time.Duration(eSec)*time.Second - streamTTLGuard
	}
	if headroom <= 0 {
		return 0
	}
	if headroom > streamTTLCap {
		return streamTTLCap
	}
	return headroom
}
