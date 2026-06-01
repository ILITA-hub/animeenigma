# Known Issues & Incidents Log

Track issues discovered during development. Each entry should include root cause analysis and resolution status.

## Active Issues

### ISS-018: Watch Together WebSocket 400 at public edge — host nginx `/api/` stripped the Upgrade header
- **Date:** 2026-06-01
- **Severity:** High (Watch Together sync, presence, chat fully broken in production)
- **Affected:** All Watch Together rooms via `https://animeenigma.ru` (every user); not reproducible against local container ports.
- **Symptom:** Room loads but shows "Reconnecting…" + "MEMBERS (0)"; no playback sync, no pause/seek propagation, no chat. Browser console: `WebSocket connection to 'wss://animeenigma.ru/api/watch-together/ws?...' failed: Error during WebSocket handshake: Unexpected response code: 400`, looping via the composable's reconnect backoff.
- **Root cause:** The host edge nginx (`/etc/nginx/sites-available/animeenigma.ru`) `location /api/` includes `snippets/proxy-params.conf`, which sets `Connection ""` (kills keepalive AND strips the WS `Upgrade` header). Watch Together's WS lives under `/api/watch-together/ws`, so it fell into the generic `/api/` block and reached the gateway WITHOUT the upgrade headers → gateway 400. The dedicated `/ws/` + `/socket.io/` blocks already used `snippets/websocket.conf`, but watch-together's path is under `/api/`, so it never matched them. The in-container nginx (`frontend/web/nginx.conf`) sets the upgrade headers on `/api/`, which is why `:3003`/`:8000` worked and masked the bug in earlier testing.
- **Diagnosis method:** Raw authenticated WS handshakes returned `101` against `:8000` and `:3003` but `400` against `https://animeenigma.ru`; Playwright browser-level `page.on('websocket')` instrumentation surfaced the 400 + reconnect loop and confirmed the token was present (ruling out the earlier token-hydration hypothesis).
- **Fix applied:** Added a more-specific `location /api/watch-together/ws` block (declared BEFORE `/api/`) that `include`s `snippets/websocket.conf`; `nginx -t` + `systemctl reload nginx`. Public-edge handshake now returns `101 Switching Protocols`. Backup saved at `/etc/nginx/sites-available/animeenigma.ru.bak.<ts>`.
- **Not in repo:** the host edge config is server-local (not version-controlled). If the host is ever re-provisioned, this block must be re-added.
- **Regression guard:** new self-seeding e2e `frontend/web/e2e/watch-together-frieren-selfseed.spec.ts` (mocks the OurEnglish provider with a tiny MP4 so a real `<video>` mounts) drives a real 2-browser play/pause/seek + episode-change round-trip through the live WS.

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

### ISS-011: VibePlayer Ad-Decoy Poisoning
- **Date:** 2026-05-13 (PoC) — production impact 2026-05-11 → 2026-05-13 (Phase 21 ship)
- **Severity:** Critical (EnglishPlayer played NO real video for ~2 days post-v3.0 ship)
- **Affected:** EnglishPlayer (services/scraper + gogoanime provider), all anime where VibePlayer was the first server returned by gogoanime ListServers (was the default first per source-HTML order before Phase 21)
- **Symptom:** EnglishPlayer loaded the master m3u8 successfully and reported duration, but no video frame ever rendered (`readyState=0` forever). HLS.js issued no error events. The manifest parsed cleanly because it WAS a valid m3u8 — just one whose every variant playlist pointed exclusively at TikTok's ad CDN.
- **Root cause (PoC 2026-05-13):** IP-level poisoning. VibePlayer's upstream backend at `vibeplayer.site` serves master m3u8 manifests where the entire variant playlist is composed of segments at `p16-ad-sg.ibyteimg.com` (TikTok ad CDN). Real headless Chromium gets the same poison — confirmed not a fingerprint / TLS / User-Agent artifact. The poison is keyed off the request source IP (the production server's egress IP); Cloudflare WARP or other egress rotation would defeat it. See `docs/plans/2026-05-13-scraper-self-healing-spec.md §2` for the PoC findings table.
- **Why Grafana didn't catch it:** Phase 17's `provider_health_up` gauge only checked that ListServers + GetStream returned 200. Both endpoints returned valid 200s — VibePlayer's manifest IS technically valid HLS, just video-less. The probe stage's gate did not parse segments; it only checked HTTP status + content type. Pattern mirror of ISS-009 (HiAnime health check tested wrong path).
- **Bonus discovery:** PoC unpack of StreamHG/Earnvids packed-JS revealed BOTH providers expose a secondary `hls3` URL family at rotated CDNs (`managementadvisory.sbs`, `exoplanethunting.space`) for use when the `hls2` signed-URL TTL expires. Currently the extractor only captures `hls2`. Plan 22-01 ships multi-URL extraction; Plan 22-02 allowlists the hls3 hosts.
- **Fix applied (mitigation, NOT resolution):**
  1. Phase 21 Plan 03 — `SCRAPER_SERVER_PRIORITY` config (default `streamhg,earnvids,vibeplayer`) demotes VibePlayer to LAST in the server priority list. Production cold-path now hits StreamHG / Earnvids first and never reaches VibePlayer for healthy anime.
  2. Phase 21 Plan 01 — `libs/streamprobe` playability gate with hardcoded ad-CDN blocklist (`ibyteimg.com`, `p16-ad-sg`, `ad-site-i18n`, `tiktokcdn.com`) catches any VibePlayer manifest that still leaks through (e.g. future server-list rotations) and fails the gate with `Reason=ad_decoy` BEFORE the URL reaches the user. `parser_ad_decoy_total{provider, server}` metric emits per drop.
  3. Production smoke 2026-05-13 (Phase 21 Plan 03 SUMMARY): Frieren ep1 cold-path now returns a real `*.cdn-centaurus.com/hls2/.../master.m3u8` — NOT `p16-ad-sg.ibyteimg.com` — with `meta.gated=true`. Counter `parser_unplayable_total{provider="gogoanime",reason="cdn_unreachable",server="streamhg"} = 1` evidences the gate caught one failed StreamHG candidate and the orchestrator successfully iterated to a second StreamHG URL.
  4. Phase 22 (this milestone) — multi-URL extraction so when StreamHG's hls2 signed URL expires, the hls3 secondary URL kicks in before the orchestrator gives up on the server. Plan 22-01 adds the multi-source Stream; Plan 22-02 allowlists the hls3 hosts.
- **Remaining work (path to Resolved status):**
  - **Cloudflare WARP egress sidecar** — separate future phase (`.planning/ROADMAP.md` reserves Phase 24 for this work). Routing scraper egress through WARP would land the requests on Cloudflare IPs that VibePlayer's backend does not poison, restoring VibePlayer as a working server. Until this lands, VibePlayer stays deprioritized AND the streamprobe blocklist is the defense-in-depth backstop.
  - Phase 23 canary (the v3.1 self-maintenance loop) will catch any new ad-CDN family that appears in production by failing the playability gate and firing `ScraperAdDecoySurge` to the maintenance bot.
  - When WARP ships and VibePlayer's ad-decoy rate drops to zero for 30 consecutive days (verified via `parser_ad_decoy_total{server="vibeplayer"}` flat-line), move this entry to `## Resolved Issues` and flip status to `Fixed`.
- **Key files:**
  - `libs/streamprobe/probe.go` — playability gate
  - `libs/streamprobe/blocklist.go` — hardcoded ad-CDN host blocklist (hls3 of `ibyteimg.com`, `p16-ad-sg`, etc.)
  - `services/scraper/internal/providers/gogoanime/client.go` — `coldPathGated` + `SortByPriority`
  - `services/scraper/internal/config/config.go` — `SCRAPER_SERVER_PRIORITY` env var
  - `services/scraper/cmd/scraper-api/main.go` — `ValidatePriorityList` fail-fast at boot
  - `services/scraper/internal/embeds/streamhg.go` / `earnvids.go` — Phase 22 multi-URL extraction (this phase)
  - `libs/videoutils/proxy.go` — HLSProxyAllowedDomains (Phase 22 adds hls3 hosts)
- **Lesson learned:** Health checks that test only HTTP-status + content-type miss content-level poisoning. The streamprobe playability gate (Phase 21) walks the manifest to first-segment HEAD and inspects segment hostnames — this is the correct depth of validation for a streaming-aware health check. Pattern echoed in ISS-009 (HiAnime). The reusable rule: **health-check the same code path the user takes, AND test that the bytes the user receives are actually the right TYPE of bytes (not just HTTP-200).**
- **Status:** Mitigated (2026-05-13) — root cause (IP-level poisoning) persists; symptom resolved via server-priority deprioritization + ad-CDN blocklist. Will flip to `Fixed` after WARP egress ships in a future phase.
- **BLK-INT-01 closure note (2026-05-19, Phase 25 / SCRAPER-HEAL-21):** The hls3-rotation arm of this issue is now self-healed via the maintenance bot pipeline. Operator runbook published at `docs/issues/2026-05-19-hls3-rotation-self-heal-runbook.md`. Future hls3 rotations follow the runbook (canary → Grafana → maintenance webhook → Telegram → operator-approve → commit), not direct edits. The rotated hosts captured in the 2026-05-13 audit hotfix (`cdn-centaurus.com`, `meadowlarkdesignstudio.cfd`, `goldenridgeproduction.shop`) remain in `HLSProxyAllowedDomains` and are now part of the audit-trail-preserving lineage. The Telegram-driven button_fix arm of the loop remains gated on operator confirmation per Phase 25 Plan 25-04 Task 3 — the autonomous portion of Phase 25 cannot drive Telegram interactions.

### ISS-012: AllAnime persisted-query SHA hashes stale at v0.1 ship
- **Date:** 2026-05-18 (workstream raw-jp Phase 01 deploy)
- **Severity:** Medium (degrades to `available: false` everywhere — no crash; the chip simply shows the empty-state copy until SHAs refresh)
- **Affected:** `/api/anime/{id}/raw/episodes` and `/raw/stream` endpoints in the catalog service.
- **Symptom:** Every raw lookup returns HTTP 200 with `{"episodes":[],"available":false,"source":"allanime"}`. Catalog logs show `allanime: query rejected (likely stale SHA): {"message":"PersistedQueryNotFound","extensions":{"code":"PERSISTED_QUERY_NOT_FOUND"}}` for every anime tried.
- **Root cause:** The SHA256 hashes shipped as `SHASearchFallback`/`SHAEpisodesFallback`/`SHASourcesFallback` in `services/catalog/internal/parser/allanime/queries.go` were sourced from upstream reference projects (pystardust/ani-cli, justfoolingaround/animdl) but appear to have rotated upstream sometime between the design doc capture and the v0.1 ship date. AllAnime's Apollo persisted-query manifest only honors the active SHA list and returns `PERSISTED_QUERY_NOT_FOUND` for any hash it no longer publishes.
- **Why this is graceful (not a crash):** The error wrapping in `services/catalog/internal/service/raw_resolver.go` distinguishes upstream-transport failures from per-request rejects. A `PERSISTED_QUERY_NOT_FOUND` is a 4xx (the API IS reachable, it just declined the query) so the resolver does NOT return `errors.Unavailable` (which would 503) — it logs and returns `available: false`. The frontend renders the empty-state copy from the RAW JP locale namespace; no user-visible error.
- **Fix (operator action):**
  1. Open the AllAnime web client at `https://allmanga.to/` in a browser with devtools network panel open.
  2. Reproduce the search / episodes / sources queries; capture the `extensions.persistedQuery.sha256Hash` parameter from each GET to `api.allanime.day/api`.
  3. Set the three values in `docker/.env`:
     ```
     ALLANIME_QUERY_SEARCH_SHA=<hash>
     ALLANIME_QUERY_EPISODES_SHA=<hash>
     ALLANIME_QUERY_SOURCES_SHA=<hash>
     ```
  4. `make redeploy-catalog`.
  5. Verify `curl http://localhost:8081/api/anime/{uuid}/raw/episodes` returns `available: true` for a known Bocchi-class title.
- **Long-term mitigation:** Two options for a future phase: (a) scrape SHAs from the AllAnime web bundle at startup (fragile, banks on bundle layout); (b) maintain a small SHA-refresh cron poller against the upstream reference projects' code. Out of scope for v0.1.
- **Key files:**
  - `services/catalog/internal/parser/allanime/queries.go` — SHA constants + env-override resolution.
  - `services/catalog/internal/config/config.go` — env-var loading.
  - `docker/.env.example` — operator documentation block.
- **Status:** Open — awaiting first operator capture of live SHAs. Architecture is correct; only data refresh required.

### ISS-015: Miruro `stream` stage broken for popular anime — decoded-response cap + fractional episode number
- **Date:** 2026-05-24
- **Severity:** Medium (entire miruro provider degraded for any anime whose episodes JSON exceeded 4 MiB OR contained fractional episode numbers — i.e. all long runners)
- **Affected:** `miruro` provider's `episodes` stage and (transitively) `servers`/`stream`/`stream_segment` for One Piece (1100+ episodes) and any other show whose upstream JSON either exceeded the 4 MiB decoded cap or contained a fractional episode number (e.g. recap specials at `1004.5`).
- **Symptom (compound):**
  - **Sub-issue A — cap:** Scraper logs `miruro: decode response: scraper: extract failed (cause: miruro: decoded response exceeded size cap)`. Orchestrator fails over from miruro to the next provider.
  - **Sub-issue B — fractional:** Scraper logs `miruro: parse episodes response: scraper: extract failed (cause: json: cannot unmarshal number 1004.5 into Go struct field rawEpisode.providers.episodes.number of type int)`. Surfaces only after sub-issue A is fixed and the cap stops rejecting the body before unmarshal.
- **Root cause:**
  1. **Decoded-cap too small.** `MaxDecodedResponseBytes = 4 << 20` (4 MiB) in `services/scraper/internal/providers/miruro/obfuscation.go` was set based on a 2024-era estimate of the `info/<id>` endpoint payload (~1.3 MiB). The `episodes` endpoint at long-runner scale was never benchmarked: One Piece measured **6.04 MiB** decoded on 2026-05-22 (probe: 1162 episodes × ~5 inner-providers × sub+dub × ~80-byte JSON per entry → roughly 6-7 MiB inflated). Naruto: 2.68 MiB. AoT: 0.14 MiB. The 4 MiB cap fell exactly in the gap between Naruto and One Piece — the latter rejected silently as `ErrDecodedTooLarge`.
  2. **DTO numeric type too strict.** `rawEpisode.Number` was typed `int`; upstream returns floats (e.g. `1004.5`) for recap specials interspersed in long-running series. Go's `encoding/json` rejects float input to `int` field with the `cannot unmarshal number 1004.5` error, killing the entire parse mid-stream.
- **Fix applied (2026-05-22, redeployed 2026-05-24):**
  - `obfuscation.go`: Bumped `MaxDecodedResponseBytes` 4 MiB → 16 MiB. Added `SoftCapWarnBytes = 12 MiB` (75% of the hard cap). Client code logs a Warn when a decoded response exceeds the soft cap — gives ops ~15 years of upstream-JSON-growth heads-up before another bump is needed.
  - `dto.go`: New `episodeNumber` type (`type episodeNumber float64`) with custom `UnmarshalJSON` accepting both int and float JSON input. `Int()` method truncates toward zero (1004.5 → 1004) for the int-typed downstream `cachedEpisode.Number`. Display titles still convey the fractional nature ("Recap Special") so users disambiguate visually.
  - `client.go`: Calls `e.Number.Int()` in `normalizeEpisodes` and logs the soft-cap warn from `fetchPipe`.
  - 5 new regression tests in `obfuscation_test.go` and `client_test.go`:
    - `TestDecodeObfuscatedResponse_OnePieceClass` — asserts the cap can hold an 8 MiB payload (One Piece + 33% headroom). Pins the One Piece floor so a future regression that lowers the cap fails immediately.
    - `TestSoftCapWarnBytes_Invariants` — asserts the soft cap is strictly less than hard cap AND at least 50% of hard cap (loses signal if tuned too low).
    - `TestDecodeObfuscatedResponse_SoftCapAccepts` — payloads above the soft cap but below the hard cap must decode cleanly.
    - `TestListEpisodes_FractionalEpisodeNumber` — synthetic kiwi-block payload with `{1004, 1004.5, 1005}` must parse, with the recap special truncated to 1004 (collision with the real ep 1004 is acceptable — both surface in the list and the user disambiguates by title).
    - `TestEpisodeNumber_UnmarshalAcceptsBothShapes` — direct unit test of the JSON-flexible numeric field across `0`, `1`, `1.0`, `1004.5`, `0.7`, `-1.2`, plus a non-number rejection case.
- **Verification (live, post-redeploy 2026-05-24 09:47Z):**
  - `wget "http://127.0.0.1:8088/scraper/episodes?mal_id=21&title=One+Piece&prefer=miruro"` → 1162 episodes returned via miruro (was 0/failed pre-fix). No failover warn in scraper logs.
  - `wget ".../scraper/servers?...episode=<ep1_id>"` → returns `[{id:"kiwi"}]`.
  - `wget ".../scraper/stream?...server=kiwi"` → returns `{"sources":[{"url":"https://vault-05.uwucdn.top/.../uwu.m3u8","type":"hls","quality":"1080p"}],"headers":{"Referer":"https://kwik.cx/"}}`.
  - Fetched the first 4 KiB of the returned HLS manifest through `streaming:8082/api/v1/hls-proxy` — got a real `#EXTM3U` with AES-128-keyed segments. **The miruro provider is now genuinely playable end-to-end.**
  - Smaller-anime regression: Naruto (220 eps) and AoT (25 eps) still resolve through miruro.
  - Health endpoint: all 5 miruro stages now `up=true` with fresh `last_ok` timestamps.
- **Status:** Fixed.

### ISS-014: ARM (`arm.haglund.dev`) origin hangs — AniList GraphQL fallback added to `libs/idmapping`
- **Date:** 2026-05-22
- **Severity:** Medium (every miruro search blocked for 10s before fallback; catalog Jimaku-subs aggregation and `backfill-attributes` cron also affected)
- **Affected:** Every caller of `libs/idmapping` — miruro provider (`services/scraper/internal/providers/miruro/`), catalog subtitle aggregator (`services/catalog/internal/service/subs_aggregator.go`), `backfill-attributes` cron, catalog MAL import path (`services/catalog/internal/service/catalog.go`).
- **Symptom:** Scraper health endpoint reports `miruro` `search` stage DOWN with `last_err: "miruro: ARM lookup: scraper: provider down (cause: ARM request failed: Get \"https://arm.haglund.dev/...\": context deadline exceeded)"`. Catalog logs show identical timeouts during Jimaku subtitle lookups and MAL→AniList backfill cron runs.
- **Root cause:** ARM's Cloudflare-fronted origin has been silently dropping our requests at the application layer since the second week of 2026-05. From inside the scraper container (and reproducibly from the host's network too): TLS handshake completes, HTTP/2 stream opens, GET request is sent over the wire — and then the origin server NEVER responds. Curl times out cleanly at whatever budget the caller set. AUTO-139's IPv4-dialer fix (commit `68e96fc`, 2026-05-22 ~01:54Z) was a misdiagnosis: the underlying transport layer was healthy, the application origin is sick. Confirmed via `curl -v -m 8 "https://arm.haglund.dev/api/v2/ids?source=myanimelist&id=21"` from both inside the scraper container and from the host — both reach TLS handshake, send the GET, then wait until timeout.
- **Fix applied (2026-05-22):**
  - Added an AniList GraphQL fallback in `libs/idmapping/client.go`. Strategy: try ARM first (3s timeout — tightened from 10s); if ARM errors or returns no AniList ID, POST `query($mal:Int){Media(idMal:$mal,type:ANIME){id idMal}}` to `https://graphql.anilist.co`. On success, merge `AniList` + `MAL` into the result. On both-failed, return the wrapped ARM error so the maintenance bot's dispatch key (`ARM lookup`) still matches the right pattern.
  - The `Resolve*` callers see the same `*MappingResult` shape; `AniDB`/`Kitsu`/`LiveChart`/`IMDB` stay nil when the fallback fires (graceful degradation — those fields are only used by the catalog's Kitsu mappings step which already handles nil).
  - 10 unit tests in `libs/idmapping/client_test.go`: ARM happy path (AniList must NOT be hit), ARM-fails-AniList-fallback, ARM-partial-AniList-fills-gap, both-failed-error-wraps, ARM 404 + AniList success, AniList-knows-nothing returns ARM partial, empty ID, Shikimori delegates to MAL, AniList GraphQL error, non-numeric ID rejected.
  - Maintenance prompt updated with this signature → tier=`info_only` since the fallback handles the case. Escalate only if BOTH ARM and AniList appear in the same `last_err`.
- **Verification (live, post-redeploy 2026-05-22 04:06Z):**
  - `miruro` health stages: `search up=true last_ok=2026-05-22T04:06:49Z` (was up=false / never-ok before the fix).
  - `episodes up=true` (was DOWN due to short-circuit cascade from search).
  - Real query: `wget "http://127.0.0.1:8088/scraper/episodes?mal_id=16498&title=Attack+on+Titan&prefer=miruro"` returns episodes.
- **Known residual issue (separate fix needed next):** miruro's `stream` stage was reporting `http 444: ...502 upstream` pre-fix — that record is stale (probe never got past search). The new probe ticks will reveal the actual stream state. Additionally: One Piece (1100+ episodes) now reaches miruro and triggers `decoded response exceeded size cap (4 MiB)` — separate bug in `services/scraper/internal/providers/miruro/obfuscation.go` `MaxDecodedResponseBytes`. Will be addressed as ISS-015 next turn.
- **Status:** Fixed (ARM-down failure mode papered over by AniList fallback). Stream stage observability deferred to ISS-015.

### ISS-013: Nineanime upstream popular-catalog migrated off `my.1anime.site` to `megaplay.buzz` (Phase 28 provider degradation)
- **Date:** 2026-05-22
- **Severity:** Medium (last-resort EN provider degraded; failover chain still has working providers above it; popular anime now unplayable via nineanime, new uploads still work)
- **Affected:** `nineanime` provider's `stream` stage for any series whose 9anime.me.uk catalog entry has been migrated to the new player (~all popular series tested: One Piece, Attack on Titan, Demon Slayer, Jujutsu Kaisen). Stub entries with YouTube-trailer-only placeholders (e.g. "Naruto (Shinsaku Anime) 2026") were also wrongly producing this stream failure pre-fix.
- **Symptom:** Scraper health endpoint reported the nineanime `stream` stage DOWN with `last_err: "nineanime: video regex: scraper: extract failed (cause: no video source)"`. `last_ok = 0001-01-01T00:00:00Z` indicated the stage had never succeeded since the upstream migrated.
- **Root cause:** Two issues compounded.
  1. **Upstream catalog migration.** 9anime.me.uk's popular catalog moved from the legacy `my.1anime.site/index.php?action=play&file=*.mp4` direct-MP4 wrapper to a redirect chain: `1anime.site/megaplay/stream/s-2/<id>/sub` → `megaplay.buzz/stream/s-2/<id>/sub`. The new target is a dynamic JS player (obfuscated) that fetches the actual stream URL via XHR — no inline `<source src="videos/...mp4">` exists for the regex to match. The provider's `doc.go` explicitly anticipates this ("~6-month half-life expected; operator kill-switch SCRAPER_DEGRADED_PROVIDERS=nineanime").
  2. **Iframe regex too permissive.** The original `iframeSrcRegex` matched any iframe on the episode page without checking the host. When a stub series embedded a YouTube trailer first (and no MP4 wrapper anywhere), the extractor grabbed the YouTube iframe and produced a misleading "no video source" downstream — misattributing upstream catalog migration to a packed-JS drift the maintenance bot's Pattern 7 auto-edit workflow would have tried (and failed) to fix.
- **Fix applied (2026-05-22):**
  - Added explicit host allowlist (`embedAllowedHosts = ["my.1anime.site"]`) checked via `isAllowedIframeHost` in `services/scraper/internal/providers/nineanime/client.go`. The httptest-isolation case is preserved via a same-origin baseURL fallback (production never hits that fallback because 9anime.me.uk is not in the allowlist of legitimate embed hosts).
  - Two new selector identifiers wired into `parser_zero_match_total`: `my_1anime_iframe` (iframe extraction / host gate misses) and `video_mp4_source` (downstream `<source>` regex misses). The maintenance bot's Pattern 7 dispatch now sees a stable, parseable signature distinguishing upstream-shape regression from packed-JS rotation.
  - Updated `.claude/maintenance-prompt.md` Pattern 7 fix-paths list with a sub-pattern for this signature, explicitly tier=`escalate` (recommend `SCRAPER_DEGRADED_PROVIDERS=nineanime`) — auto-edit selectors does NOT apply since the breakage is upstream player technology change, not CSS-selector drift.
  - Regression tests in `services/scraper/internal/providers/nineanime/client_test.go`: `TestGetStream_YouTubeIframeRejected`, `TestGetStream_MegaplayRedirectRejected`, `TestIsAllowedIframeHost` (10 host-allowlist cases including suffix/prefix injection attempts).
- **Verification:**
  - Live drive via `wget http://127.0.0.1:8088/scraper/stream?mal_id=21&prefer=nineanime` for One Piece (popular-migrated): returns HTTP 502 with `parser_zero_match_total{provider="nineanime",selector="my_1anime_iframe"} 1`. `last_err` is now `nineanime: iframe host: scraper: extract failed (cause: iframe host "1anime.site" not in allowlist {my.1anime.site}; ...)`.
  - Live drive for Marriagetoxin (legacy direct-MP4 still active upstream): returns stream URL `https://my.1anime.site/videos/marriagetoxin-episode-1.mp4` with `type=mp4` and Referer header. Legacy path intact.
- **Operator next step (optional):** When the rest of the popular catalog migrates and new uploads also stop using `my.1anime.site`, add `nineanime` to `SCRAPER_DEGRADED_PROVIDERS` in `docker/.env`. The doc.go-documented kill-switch removes the provider from the EN failover chain without any code change.
- **Status:** Fixed (extractor + bot dispatch). Open (kill-switch decision deferred to operator — depends on rate of new-upload migration).

### ISS-016: AnimePahe sidecar `/play`+`/release` 400/404s were failover noise — not a sidecar bug (probe masking + romaji-title gap)
- **Date:** 2026-05-25
- **Severity:** Low (no user-facing breakage — investigation reclassified a suspected outage as expected behavior). animepahe resolves correctly for all title-resolvable anime; the residual is log noise + an observability gap.
- **Affected:** `animepahe` provider's `servers`/`stream` stages (the spurious 400/404 sidecar calls); the liveness probe's accuracy for animepahe.
- **Symptom (reported):** `animepahe-resolver: /play non-200: status 400` and `/release not found upstream: status 404` in scraper failover logs, while the liveness probe reported animepahe all-stages-UP.
- **Root cause (single cascade, measured live):**
  1. **malsync.moe returns numeric animepahe IDs** (e.g. AoT=`49`); `SCRAPER-HEAL-32` rejects non-UUID IDs via `isSessionShape`, so `FindID` always uses the fuzzy `/search` path (Jaro-Winkler ≥ 0.85, `animepahe/client.go`).
  2. The fuzzy match needs a title matching animepahe's **English** listing. The catalog sends `anime.NameEN` when set, else Shikimori **romaji** `anime.Name` (`catalog .../scraper.go:81-84`). When romaji ≠ English ("Shingeki no Kyojin" vs "Attack on Titan" = 0.50; "Sousou no Frieren" vs "Frieren: Beyond Journey's End" = 0.63), `FindID` fails and the orchestrator **fails over to allanime — the user still gets a stream.**
  3. The servers/stream stages then re-run the chain from animepahe carrying allanime's `<showID>:<ep>` **episode** ID (contains `:`) → sidecar `/play` **400**; the episodes stage re-runs animepahe's `ListEpisodes` with allanime's bare `<showID>` **anime** ID → sidecar `/release` **404**. Both are pure failover artifacts, not bugs. Note the two are shape-different: the episode ID has a `:` (catchable), but the bare show ID (`PcZRitDAgNmrwdY2p`, 17 alnum chars) is **indistinguishable** from a real animepahe session by `SESSION_PATTERN` alone.
  4. The probe's golden pool uses **English** titles ("Attack on Titan") → animepahe always green → the whole thing was invisible.
- **`name_en` AniList backfill considered and REJECTED by measurement.** Sampled 18 empty-`name_en` anime against AniList + live animepahe:

  | Bucket | Count/18 | Backfill helps? |
  |---|---|---|
  | AniList English title == romaji `name` (Vinland Saga, SPY×FAMILY, Cyberpunk: Edgerunners, Idol, Dorohedoro, Chihayafuru, Medalist) | ~9 | No — already resolve on animepahe via romaji (verified live: 24/12/10 eps) |
  | AniList `english: null` | 7 | No — no English title exists anywhere |
  | romaji ≠ English AND AniList has English AND `name_en` empty | ~0 | (the only bucket backfill could fix — nearly empty) |

  Anime where romaji genuinely differs from English (AoT, Demon Slayer, Frieren) **already have `name_en` populated** by Shikimori, so they're not in the empty set at all. ⇒ no Go-only resolution fix has meaningful benefit; allanime failover is the correct safety net for the unfixable tail.
- **Fix applied (2026-05-25) — lean: silence the noise, surface the signal, document it:**
  - **Foreign-episode-ID guard** in `ListServers`/`GetStream` (`services/scraper/internal/providers/animepahe/client.go`): reuse `isSessionShape`; a `:`-containing / non-session episode ID returns `domain.ErrNotFound` (failover-retryable) **before** the sidecar call. Emits `parser_zero_match_total{provider="animepahe",selector="foreign_episode_id"}`. This eliminates the `/play 400`. The `/release 404` from the episodes stage is **intentionally NOT guarded** (review m1): allanime's bare show ID is session-shaped, so a shape guard can't distinguish it — fully eliminating it would require cross-stage provider pinning in the orchestrator (deliberately out of scope; the failed `/release` triggers correct, harmless failover).
  - **Fuzzy-miss metric** `parser_zero_match_total{provider="animepahe",selector="fuzzy_no_match"}` at the sub-threshold branch — the signal the English-titled golden-pool probe masks. (Probe deliberately NOT changed to romaji: it tests providers in isolation, so a romaji entry would be a perpetual false-red.)
  - Regression tests in `client_test.go`: `TestProvider_ListServers/GetStream_ForeignEpisodeIDShortCircuits`, `TestProvider_ListServers_ValidSessionPassesGuard`, `TestProvider_FindID_FuzzyNoMatchEmitsCounter`. (Pre-existing tests used unrealistic short episode IDs the real sidecar would 400 — switched to a contract-valid 64-hex `testEpSession`.)
  - `.claude/maintenance-prompt.md`: two `info_only` Pattern-7 sub-signals (both expected behavior, not regressions; escalate only if allanime is ALSO down).
- **Verification:** `go test ./internal/providers/animepahe/... -race` green; live E2E — a romaji-title `prefer=animepahe` request fails over to allanime: the `/play 400` is gone (replaced by a clean `animepahe: foreign episode id` short-circuit), the `/release 404` remains (episodes stage, by design — see above), and both counters increment (`foreign_episode_id=1`, `fuzzy_no_match=2`). A normal romaji==English title (Vinland Saga, empty `name_en`) still resolves on animepahe (24 eps, kwik servers).
- **Status:** Fixed (noise + observability + docs). The romaji↔English tail intentionally relies on allanime failover — no further action unless allanime degrades for the same titles.

### ISS-017: AnimeFever `stream` stage DOWN — English-only matching hit no-embed compilations + status:false misread as "stale ctk"
- **Date:** 2026-05-25
- **Severity:** Medium (animefever, slot 4, was effectively dead for any anime AnimeFever lists under a romaji title — a large set — though allanime in slot 3 masked most user impact).
- **Affected:** `animefever` provider's `search` + `stream` stages; the multi-title plumbing now also benefits all other fuzzy-matching providers (gogoanime/nineanime/miruro/animepahe).
- **Symptom:** Scraper health reported animefever `stream` DOWN with `last_err: "animefever: ajax status=false: scraper: extract failed (cause: animefever: stale ctk token)"`.
- **Root cause (two compounding issues, measured live):**
  1. **English-only matching → no-embed compilations.** AnimeFever's search is English-indexed, but it lists the **main series under its ROMAJI title** (`shingeki-no-kyojin-sub.50313`) while no-embed **recap/compilation** entries use the English title (`attack-on-titan-chronicle.60449`). The catalog sent only its primary title (`NameEN` = "Attack on Titan"), which Jaro-Winkler matched to the chronicle (0.95) over the romaji main series (0.50). The chronicle has no player embed.
  2. **`status:false` misclassified as "stale ctk".** AnimeFever's `/ajax/anime/load_episodes_v2` returns `status:false / embed:false` for a no-embed entry. The code treated EVERY `status:false` as a stale ctk → wasteful evict-and-retry-once → then surfaced the misleading "stale ctk token" error. (The ctk was fine — verified: a correctly-matched series like Frieren returns `status:true` with the same ctk extraction.)
- **Fix applied (2026-05-25) — cross-service multi-title matching + honest classification:**
  - **Multi-title scoring (ISS-017).** Added `domain.AnimeRef.AltTitles []string`. The catalog (`resolveAnime`) now forwards the OTHER title forms (romaji `Name`, `NameJP`) as a comma-joined `title_alt` query param (`parser/scraper/client.go`); the scraper handler parses it (`parseAltTitles`, capped at 4, deduped, primary excluded); `animefever.FindID` scores each candidate against the primary Title AND every AltTitle, taking the max. "Attack on Titan" + romaji alt "Shingeki no Kyojin" now resolves the romaji-listed main series.
  - **status:false classification.** `fetchCtk` now reports whether the ctk came from cache. `status:false`/`embed:false` with a FRESH ctk → honest `errNoEmbed` (ErrExtractFailed, NO retry) + `parser_zero_match_total{provider="animefever",selector="no_embed"}`; only a CACHED ctk still triggers the evict-and-retry-once (genuine staleness).
  - **Golden pool consistency.** The liveness probe's golden entries carry romaji `AltTitles` (AoT→"Shingeki no Kyojin", Demon Slayer→"Kimetsu no Yaiba"). Without this the probe (a) failed for romaji-listed series AND (b) poisoned the shared per-MAL-ID slug cache (`scraper:animefever:show:<id>`) with the English-only resolution, defeating the catalog's alt-title for the 5 golden anime.
  - Tests: `animefever` `TestFindID_MultiTitleResolvesRomajiMainSeries`, `TestGetStream_FreshCtkStatusFalse_NoEmbedNoRetry`, `TestGetStream_CachedCtkStatusFalse_RetriesOnce`; handler `TestParseAltTitles`; catalog client `title_alt` assertion; `golden_test` DeepEqual fix (AnimeRef now has a slice).
  - `.claude/maintenance-prompt.md`: animefever `no_embed` (info_only) + multi-title regression (button_fix) sub-patterns.
- **Verification:** all scraper + catalog suites green (`-race`). Live E2E through the **real catalog path**: AoT (`prefer=animefever`) → `shingeki-no-kyojin-sub.50313:27242` (animefever main series via romaji alt) → playable `#EXTM3U` manifest fetched through the streaming proxy (`static-cdn-ca1.mofl.pro`). Frieren (already English-matchable) still works.
- **Status:** Fixed.

## Resolved Issues

### ISS-019: Subtitle panel showed "opensubtitles down" for hours after one transient failure (degraded result cached 6h)
- **Date:** 2026-06-01
- **Severity:** Medium (subtitle panel degraded to JP-only / "providers_down" for up to 6h per affected title)
- **Reported by:** User — `/anime/dbc95dd5-...` (That Time I Got Reincarnated as a Slime S4) showed "Некоторые источники не ответили: opensubtitles".
- **Symptom:** The "other subs" panel reported `providers_down: ["opensubtitles"]` and showed no non-JP subtitles, persistently. Flushing the title's `subs:*` Redis keys made it immediately return 18 languages (en, ru, …) with `providers_down: null`.
- **Root cause:** `SubsAggregator.FetchAll` (`services/catalog/internal/service/subs_aggregator.go`) cached every result for 6h **unconditionally** — including results where a provider transiently failed (momentary OpenSubtitles timeout/rate-limit, or a slow 301-normalized title-query round-trip exceeding the 10s client timeout). One blip froze a degraded `providers_down` result into the panel for 6 hours. (OpenSubtitles 301-redirects title queries to a lowercased form; Go follows it fine — that path works, it was just occasionally slow.)
- **Fix applied:** `subs_aggregator.go` now picks the cache TTL by result quality via `subsCacheTTL(resp)`: full results (no `ProvidersDown`) cache `6h` (`fullSubsCacheTTL`); degraded results cache `60s` (`degradedSubsCacheTTL`) so a failed provider is retried within a minute and self-heals. Pure helper covered by `subs_aggregator_cache_test.go`.
- **Verification:** Flushed + re-fetched the reported title → 18 languages, `providers_down: null`, full-success key TTL = 21600s (6h). Unit test asserts degraded < full.
- **Status:** Fixed (2026-06-01)

### ISS-010: Search returns empty / single result for any anime not in local DB (Shikimori .one → .io migration)
- **Date:** 2026-05-06
- **Severity:** High (catalog effectively read-only — no new anime can be discovered)
- **Reported by:** User trying to find "Maboroshi" (Alice to Therese no Maboroshi Koujou). Logs showed several minutes of failed searches across many query variants.
- **Symptom:** Searching any anime not already cached in local DB returned empty results. Searching a query that partially matched a stale local entry returned only that stale entry. Forced `?source=shikimori` returned `data: null`.
- **Root cause:** Shikimori migrated their domain from `shikimori.one` to `shikimori.io` and now serves a 301 redirect with an HTML body on the old host. The catalog's `SHIKIMORI_GRAPHQL_URL` default still pointed to `https://shikimori.one/api/graphql`. Go's standard `http.Client` follows 301 on a POST as a GET (per RFC) and replays no body, so the request became `GET https://shikimori.io/api/graphql` with no GraphQL query — the response was an HTML page. The JSON decoder choked on `<` and the parser returned `EXTERNAL_API_ERROR: invalid character '<' looking for beginning of value`. The error was logged at WARN and the upstream service swallowed it (returned empty results, status 200), so the failure was silent end-to-end.
- **Fix applied:**
  - `services/catalog/internal/config/config.go` — default `SHIKIMORI_BASE_URL` and `SHIKIMORI_GRAPHQL_URL` updated to `shikimori.io`
  - `services/catalog/internal/parser/shikimori/client.go:623` — poster URL prefix updated to `https://shikimori.io`
  - Busted stale `search:*` Redis keys that had cached the empty/partial results during the broken window
- **Verification:** `GET /api/anime/search?q=Maboroshi` now returns 13 results including the user's target (shikimori_id 49303 — Alice to Therese no Maboroshi Koujou).
- **Status:** Fixed (2026-05-06)

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
