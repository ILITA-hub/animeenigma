package animepahe

// NOTE (Phase 27 / A5): the cache key episodes:animepahe:<providerID> is
// already animeSession-keyed because FindID returns the UUID session (not
// the MAL ID). No rekeying needed on the sidecar migration.

import (
	"net/url"
	"strconv"
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

// NOTE: normalizeTitle + jaroWinkler used to live here. Phase 18 introduces
// a second consumer (Gogoanime/Anitaku) so both helpers were promoted to the
// shared package services/scraper/internal/fuzzy. The call sites in client.go
// now import fuzzy and call fuzzy.NormalizeTitle / fuzzy.JaroWinkler.
