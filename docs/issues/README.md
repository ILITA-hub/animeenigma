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

### ISS-008: AnimeLib player broken — Kodik iframe fallback removed
- **Date:** 2026-03-23
- **Severity:** High (AnimeLib player unusable for all Kodik-only translations)
- **Affected:** AnimeLib player, all anime where translations only have Kodik embeds (no direct MP4)
- **Symptom:** User selects any translation, player shows error "failed to get video URL". Grafana shows AnimeLib as UP (health check passes).
- **Root cause:** `AnimeLibPlayer.vue` line 364 had `// iframeUrl removed — Kodik fallback disabled to expose MP4 errors`. The `fetchStream()` method only handled `data.sources` (direct MP4) and showed an error for everything else. The backend correctly returned `iframe_url` for Kodik-based translations, but the frontend discarded it.
- **Why Grafana didn't catch it:** The health check tests `Search("naruto")` against the AnimeLib hapi API, which succeeds. The API is genuinely working — the bug was in frontend rendering, not backend availability.
- **Context:** Many translations on AnimeLib use Kodik as their player (e.g. AniLot, OnWave, CapySound, AnimeVost). Only translations with `player: "Animelib"` have direct MP4 sources. For some anime, ALL translations are Kodik-only.
- **Fix applied:**
  - Restored `iframeUrl` ref in component state
  - Added `iframe_url` handling in `fetchStream()`: when `sources` is empty but `iframe_url` exists, render Kodik iframe
  - Added `<iframe>` element in template between direct video and placeholder
  - Reset `iframeUrl` on episode change, stream fetch, and anime change
- **Key files:**
  - `frontend/web/src/components/player/AnimeLibPlayer.vue` — the fix
  - `services/catalog/internal/service/catalog.go:2146-2155` — backend Kodik fallback (was already correct)
  - `services/catalog/internal/domain/anime.go:376` — `AnimeLibStream.IframeURL` field
- **Lesson learned:** Don't disable fallback paths without providing an alternative. The "expose MP4 errors" comment suggests this was intentional debugging, but it was left in production. Kodik iframe is the primary player for most AnimeLib translations.
- **Status:** Fixed (2026-03-23)

### ISS-009: HiAnime Go client used dead hianime.to for Search/GetEpisodes/GetServers
- **Date:** 2026-03-23
- **Severity:** Critical (HiAnime player showed "no episodes" for ALL anime)
- **Affected:** HiAnime player, all anime — not just specific titles
- **Symptom:** HiAnime player showed "player.noEpisodes" for every anime, including well-known titles. Grafana showed HiAnime as UP.
- **Root cause:** The HiAnime Go client (`parser/hianime/client.go`) had `Search()`, `GetEpisodes()`, and `GetServers()` methods that scraped HTML from `hianime.to` via goquery. After hianime.to died (ISS-007), these methods all failed with connection timeouts. Meanwhile, the health checker tested the aniwatch API sidecar directly (different code path), so it reported UP. The `GetStream()` method already used the aniwatch API and worked fine — but users never reached it because Search/GetEpisodes failed first.
- **Secondary issue:** The `SearchResult` struct lacked a `JName` (Japanese name) field. HiAnime returns both English and Japanese names, but matching only used the English name. For anime like "Sousou no Frieren 2nd Season", the English name "Frieren: Beyond Journey's End Season 2" didn't match the DB name, but the Japanese name was an exact match.
- **Fix applied:**
  - Rewrote `Search()`, `GetEpisodes()`, and `GetServers()` to use the aniwatch API instead of HTML scraping
  - Added `JName` field to `SearchResult` struct
  - Updated `doHiAnimeSearch()` in catalog.go to match against both `r.Name` and `r.JName`
  - Removed dead HTML scraping code (goquery usage, `fetchDocument`, `setHeaders`, `GetAnimeInfo`, etc.)
  - Upgraded all 4 health checks to test full playback chain (search → episodes → streams), not just search
- **Key files:**
  - `services/catalog/internal/parser/hianime/client.go` — full rewrite from HTML scraping to aniwatch API
  - `services/catalog/internal/service/catalog.go` — JName matching in `doHiAnimeSearch()`
  - `services/catalog/internal/service/health_checker.go` — full-chain health checks
- **Lesson learned:** When a service has multiple access paths to an external API (direct scraping vs API sidecar), the health check must test the SAME path that user-facing code uses. Testing a separate path creates a blind spot.
- **Status:** Fixed (2026-03-23)

### ISS-007: HiAnime player DOWN due to upstream domain migration
- **Date:** 2026-03-22 (detected) / 2026-03-13 (domain shutdown)
- **Severity:** Critical (HiAnime player completely unusable for ~9 days)
- **Affected:** HiAnime player, all users relying on EN HLS streams
- **Symptom:** Grafana `Player Unavailable` alert firing for `hianime` since 2026-03-22 12:05 UTC. Aniwatch scraper returned `500 getAnimeSearchResults: fetchError: Something went wrong` on all search/episode requests. Requests timed out after ~8-10s.
- **Root cause:** HiAnime.to shut down on 2026-03-13 and migrated to a new domain. The aniwatch scraper (`rz6e/aniwatch-api`) image from 2026-03-17 still targeted the old domain. The scraper's own `/health` endpoint passed (it only checks if the Node.js server is alive), but all actual scrape requests to hianime.to failed because the domain was dead.
- **Key indicators:**
  - `player_health_up{player="hianime"} = 0` for 20+ hours
  - Aniwatch logs: `getAnimeSearchResults: fetchError: Something went wrong` (500)
  - `curl https://hianime.to` → connection timeout (domain dead)
  - Catalog logs: `failed to find anime on hianime` with 10s+ request durations
- **Fix applied:**
  - Pulled latest `rz6e/aniwatch-api:latest` image (updated to target new HiAnime domain)
  - Recreated aniwatch container: `docker compose up -d aniwatch`
  - Search latency dropped from 8s+ timeout to ~200ms
  - All 4 player health metrics returned to UP
  - Grafana alert auto-resolved
- **HiAnime domain history (for future reference):**
  - `zoro.to` → `aniwatch.to` → `hianime.to` (died 2026-03-13) → new domain
  - This site changes domains periodically due to anti-piracy takedowns
  - The USTR 2025 report explicitly traced this lineage
- **How to detect next time:**
  - Grafana alert `Player Unavailable` will fire within 5 minutes of failure
  - Aniwatch logs will show `fetchError: Something went wrong` on all scrape endpoints
  - The aniwatch `/health` endpoint will still return 200 (misleading — it only checks Node.js liveness)
- **How to fix next time:**
  1. Check if `rz6e/aniwatch-api:latest` has been updated: `docker pull rz6e/aniwatch-api:latest`
  2. If new layers pulled → recreate: `docker compose -f docker/docker-compose.yml up -d aniwatch`
  3. If no update available → check HiAnime community channels for new domain, wait for scraper update
  4. Verify fix: `curl http://localhost:3100/api/v2/hianime/search?q=naruto&page=1` should return 200 with results
- **Prevention ideas:**
  - Consider a cron job or script that checks for aniwatch image updates weekly
  - The health check could be enhanced to test actual scrape functionality, not just liveness
- **Key files:**
  - `docker/docker-compose.yml` — aniwatch service definition
  - `services/catalog/internal/service/health_checker.go` — health check logic
  - `services/catalog/internal/parser/hianime/client.go` — HiAnime client
  - `docker/grafana/provisioning/alerting/rules.yml` — `player-unavailable` alert rule
- **Status:** Fixed (2026-03-23)

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
