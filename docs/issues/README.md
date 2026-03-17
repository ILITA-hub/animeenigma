# Known Issues & Incidents Log

Track issues discovered during development. Each entry should include root cause analysis and resolution status.

## Active Issues

### ISS-001: Consumet/HiAnime HLS streams blocked by Cloudflare on owocdn.top/uwucdn.top
- **Date:** 2026-02-27
- **Severity:** High (player unusable for affected streams)
- **Affected:** Consumet player (vidcloud server), all browsers
- **Symptom:** Video plays ~0.5s then enters infinite reload loop. Console floods with `bufferAppendError` / `bufferAddCodecError` at ~200ms intervals.
- **Root cause:** Upstream CDN (`vault-*.owocdn.top`) returns Cloudflare 403 HTML challenge page instead of video segments. The HLS proxy was forwarding this HTML with `Content-Type: application/vnd.apple.mpegurl`, causing HLS.js to try parsing HTML as video data, triggering infinite error recovery loop.
- **Contributing factors:**
  - Stream URLs from Consumet API are short-lived and expire quickly
  - Cloudflare may block server IP or require browser challenges the proxy can't solve
  - `uwucdn.top` domain was missing from HLS proxy allowed domains list
- **Fix applied (partial):**
  - Proxy now detects upstream 4xx/5xx errors and returns clean 502 instead of forwarding garbage HTML (commit pending)
  - Added `proxy_upstream_errors_total{status, domain}` Prometheus metric to track CDN failures
  - Added `uwucdn.top` to allowed domains
  - Streaming service logs `upstream CDN error` with domain, status, and whether HTML was returned
- **Remaining work:**
  - Frontend HLS.js error handler should show user-friendly message on 502 instead of generic error
  - Consider auto-switching to alternative server (e.g. vidstreaming) when vidcloud fails
  - Investigate if Consumet API returns stale/expired stream URLs from cache
  - Monitor `proxy_upstream_errors_total` metric in Grafana to track frequency

### ISS-002: uwucdn.top not in HLS proxy allowed domains
- **Date:** 2026-02-27
- **Severity:** Medium (streams from this CDN silently fail)
- **Symptom:** Streaming logs show `domain not allowed for HLS proxy: vault-08.uwucdn.top`
- **Root cause:** Only `owocdn.top` was in the allowed list, but Consumet/Kwik also uses `uwucdn.top` as a mirror domain
- **Fix:** Added `uwucdn.top` to `HLSProxyAllowedDomains` in `libs/videoutils/proxy.go`
- **Status:** Fixed

### ISS-005: Gateway P95 latency stuck at ~10s in Grafana
- **Date:** 2026-03-04
- **Severity:** High (user-visible latency on search and episode lookups)
- **Affected:** All requests proxied through gateway, worst on HiAnime/Consumet/AnimeLib episode routes
- **Symptom:** Grafana "P95 Latency" panel showed gateway P95 at ~10s. After extending histogram buckets (Phase 1), the value was confirmed as real latency, not a bucket cap artifact.
- **Root causes (multiple):**
  1. **Histogram bucket cap at 10s** — `libs/metrics/metrics.go` had max bucket of 10. Grafana `histogram_quantile(0.95, ...)` couldn't compute above 10s. Fixed by adding 15 and 30 buckets.
  2. **Sequential external API searches** — `doHiAnimeSearch`, `findConsumetID`, `findAnimeLibID` in `services/catalog/internal/service/catalog.go` tried name variants sequentially. Worst case: Jikan (2s) + 3 HiAnime searches (9s) = 11s+.
  3. **N+1 enrichAnime queries** — Search results called `enrichAnime()` per-anime (2 DB queries each). Fixed with `enrichAnimesBatch()` using batch repo methods.
  4. **Uncached Jikan lookups** — Jikan English title fetched on every HiAnime search. Fixed with 7-day cache.
  5. **No search result caching** — Same query repeated within minutes hit Shikimori API again. Fixed with 15-min cache.
  6. **chi `middleware.Timeout(30s)` incompatible with proxy** — Uses `http.TimeoutHandler` which buffers entire responses in memory. Incompatible with `io.Copy(w, resp.Body)` in gateway proxy handler. Removed.
  7. **External API client timeouts too long (30s)** — HiAnime/Consumet/AnimeLib HTTP clients waited 30s per request. Reduced to 10s.
  8. **No overall deadline on parallel search** — Parallel goroutines had no collective timeout. Added `context.WithTimeout(ctx, 10s)` to all three search functions.
- **Fix applied (Phase 1):**
  - Extended histogram buckets: added 15, 30 to `libs/metrics/metrics.go`
  - Parallelized all three ID search functions (goroutines, first-match-wins)
  - Added `enrichAnimesBatch()` with `GetForAnimes()` batch repo methods
  - Cached Jikan lookups (7-day TTL)
  - Added search result caching (15-min TTL via `cache.KeySearchResults`)
  - Fixed `KeySearchResults` / `KeyAnimeList` cache key bug (`string(rune(page))` → `fmt.Sprintf`)
- **Fix applied (Phase 2):**
  - Removed `middleware.Timeout(30s)` from gateway router
  - Reduced external API client timeouts: HiAnime/Consumet/AnimeLib 30s→10s, Jikan 15s→10s
  - Added 10s `context.WithTimeout` to `doHiAnimeSearch`, `findConsumetID`, `findAnimeLibID`
  - Reduced gateway proxy client timeout 30s→15s
- **Key files:**
  - `libs/metrics/metrics.go` — histogram buckets
  - `libs/cache/ttl.go` — cache key functions
  - `services/catalog/internal/service/catalog.go` — parallel search, batch enrichment, caching
  - `services/catalog/internal/repo/genre.go` — `GetForAnimes()` batch method
  - `services/catalog/internal/repo/video.go` — `GetForAnimes()` batch method
  - `services/catalog/internal/parser/{hianime,consumet,animelib,jikan}/client.go` — client timeouts
  - `services/gateway/internal/transport/router.go` — middleware removal
  - `services/gateway/internal/service/proxy.go` — proxy client timeout
- **Grafana query:** `histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (service, le))`
- **Lesson learned:** Don't use chi `middleware.Timeout` on a reverse proxy gateway — it buffers responses via `http.TimeoutHandler`. Rely on server `WriteTimeout` and per-client HTTP timeouts instead.
- **Status:** Fix deployed, monitoring

### ISS-006: HLS bufferAppendError on mobile Safari (iOS)
- **Date:** 2026-03-17
- **Severity:** Medium (affects mobile Safari users on Consumet player)
- **Affected:** Consumet player (vidcloud server), iOS Safari 18.7 (iPhone)
- **Symptom:** Video fails to play with `Media error: bufferAppendError`. User reported on "Hell Mode: Yarikomizuki no Gamer wa Hai Settei no Isekai de Musou suru" episode 1.
- **Stream URL:** `vault-16.owocdn.top` m3u8 via vidcloud
- **Root cause (suspected):** HLS.js buffer append failure on mobile Safari — likely codec mismatch or corrupted segments from upstream CDN. Safari's MSE implementation is stricter than Chrome's and rejects segments that Chrome accepts.
- **Contributing factors:**
  - Mobile Safari has limited MSE buffer space compared to desktop
  - Upstream CDN may serve segments with codec parameters Safari doesn't support (e.g. HEVC when only H.264 expected)
  - Video.js/HLS.js error recovery may not handle Safari-specific buffer errors correctly
- **Remaining work:**
  - Investigate if specific codecs in vidcloud streams cause Safari rejection
  - Consider adding Safari-specific HLS.js config (e.g. `appendErrorMaxRetry`, `maxBufferLength`)
  - Auto-switch to alternative server (vidstreaming) when buffer errors occur
  - Test on iOS Safari with different HLS.js configurations
- **Status:** Documented, not yet investigated

## Resolved Issues

### ISS-003: Error reports received with empty fields
- **Date:** 2026-02-27
- **Severity:** Medium (reports useless without context)
- **Symptom:** Telegram notifications and server logs showed empty player_type, anime_name, etc.
- **Root cause:** Frontend `diagnostics.ts` sent camelCase JSON keys (`playerType`, `animeId`) but Go struct expected snake_case (`player_type`, `anime_id`). All fields deserialized as zero values.
- **Fix:** Updated `collectDiagnostics()` in `diagnostics.ts` to use snake_case keys matching the Go struct.
- **Status:** Fixed

### ISS-004: Error report data lost on container restart
- **Date:** 2026-02-27
- **Severity:** Medium (can't investigate user reports after deployment)
- **Symptom:** User submitted error report at 06:51 UTC, player container restarted at 08:13 UTC, all report data lost from stdout logs.
- **Root cause:** Reports were only logged to container stdout with no persistent storage.
- **Fix:** Added `player_reports` Docker volume mounted to `/data/reports/`. Each report saved as a JSON file with full diagnostics (console logs, network logs, page HTML). Files persist across container restarts.
- **Status:** Fixed
