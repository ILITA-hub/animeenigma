# Codebase Concerns

**Analysis Date:** 2026-04-27

## Documented Known Issues

### Active Issues from issues.json

**ISS-001: Consumet/HiAnime HLS streams blocked by Cloudflare on owocdn.top/uwucdn.top**
- **Files:** `libs/videoutils/proxy.go`, `services/streaming/internal/service/streaming.go`
- **Status:** Partially fixed. Proxy detects upstream 4xx/5xx errors and returns clean 502 instead of forwarding HTML. Remaining work: frontend HLS.js error handler needs user-friendly message on 502; consider auto-switching to alternative server when vidcloud fails.
- **Impact:** Video playback fails with infinite reload loop when upstream CDN returns Cloudflare challenge HTML parsed as video data.

**ISS-005: Gateway P95 latency stuck at ~10s**
- **Files:** `libs/metrics/metrics.go`, `services/catalog/internal/service/catalog.go`, `services/catalog/internal/parser/{hianime,consumet,animelib,jikan}/client.go`
- **Status:** Fixed (Phase 1 & 2 deployed). Root causes: histogram bucket cap at 10s, sequential external API searches, N+1 enrichAnime queries, uncached Jikan lookups, no search result caching, incompatible chi `middleware.Timeout`, overly long client timeouts.
- **Impact:** High user-visible latency (10s+) on search and episode lookups.

**ISS-006: HLS bufferAppendError on mobile Safari (iOS)**
- **Files:** `frontend/web/src/components/player/ConsumetPlayer.vue`, `frontend/web/src/components/player/HiAnimePlayer.vue`
- **Status:** Not yet investigated. Likely codec mismatch or corrupted segments on upstream CDN.
- **Impact:** Video fails on iOS Safari with "Media error: bufferAppendError".

**ISS-008: AnimeLib player broken — Kodik iframe fallback removed**
- **Files:** `frontend/web/src/components/player/AnimeLibPlayer.vue`
- **Status:** Fixed (2026-03-23). Restored iframeUrl ref and iframe_url handling in fetchStream().
- **Impact:** AnimeLib player unusable for all Kodik-only translations (most translations use Kodik).

**ISS-009: HiAnime Go client used dead hianime.to for Search/GetEpisodes/GetServers**
- **Files:** `services/catalog/internal/parser/hianime/client.go`, `services/catalog/internal/service/catalog.go`
- **Status:** Fixed (2026-03-23). Rewrote methods to use aniwatch API instead of HTML scraping. Added JName field to SearchResult struct for better matching.
- **Impact:** HiAnime player showed "no episodes" for ALL anime.

**ISS-007: HiAnime player DOWN due to upstream domain migration**
- **Files:** `services/catalog/internal/service/health_checker.go`, `docker/docker-compose.yml`
- **Status:** Fixed (2026-03-23). Upstream domain migration from hianime.to to new domain, resolved by pulling latest aniwatch-api container.
- **Impact:** HiAnime player completely unusable for ~9 days. Recurring risk: site changes domains periodically due to anti-piracy takedowns.

---

### Recent UI Audit Findings (2026-04-20)

From `docs/issues/ui-audit-2026-04-20.md` — Mobile-specific findings:

**Major (2):**
- **UA-042** — Home /schedule shortcut link has no accessible name when icon-only
- **UA-044** — SearchAutocomplete ARIA attrs land on wrapper `<div>` instead of `<input>`
- **UA-046** — Browse Genre filter placeholder `text-white/30` fails 4.5:1 contrast
- **UA-048** — Browse h1 → h3 heading-order jump
- **UA-050** — /anime/:id error state "Failed to fetch anime" English string on RU locale
- **UA-053** — Hamburger button has no `aria-expanded`
- **UA-055** — Schedule destination missing from mobile menu
- **UA-056** — Mobile menu panel renders below Home search wrapper (z-index stack)

**Minor (1):**
- **UA-043** — Hamburger `aria-label="Open menu"` — English string on Russian locale
- **UA-045** — Hamburger button 40×40 — below WCAG 2.5.5 target size
- **UA-047** — Browse Genre filter trigger missing `aria-haspopup` + `aria-expanded`
- **UA-051** — /anime/:id `<title>` is generic "Детали аниме - AnimeEnigma"
- **UA-052** — /anime/:id 9–10 `text-white/40.text-sm` nodes fail contrast

**Status:** Recommended fixes planned in Batches G–I (20 line changes across 5 files).

---

## Tech Debt

### Range Request Handling Incomplete

**Issue:** HTTP Range requests (byte-level seeking) not fully implemented for MinIO streams.

**Files:** `services/streaming/internal/service/streaming.go:210` (TODO comment), `libs/videoutils/proxy.go` (line 107 handles ranges but not fully tested)

**Impact:** Users cannot seek precisely in large video files; seeking may restart from beginning instead of jumping to requested byte offset. Worst case: 100MB+ uploads become difficult to navigate.

**Fix approach:** Implement full HTTP/1.1 206 Partial Content semantics — parse Range header, validate offsets, return Content-Range header with correct byte range.

---

### Presigned URL Generation Missing

**Issue:** MinIO upload presigned URL generation is not properly implemented.

**Files:** `services/streaming/internal/handler/upload.go:125` (TODO comment)

**Impact:** Admin uploads require direct proxy handler; cannot use presigned URLs for direct-to-MinIO browser uploads, limiting scalability and increasing server load.

**Fix approach:** Use MinIO's `PresignedPutObject` API to generate time-limited PUT URLs. Frontend can then upload directly to MinIO with SigV4 signature.

---

### Consumet Episode Cache TTL Reduced to 5 Minutes

**Issue:** Consumet episode lists are cached for only 5 minutes (line 1979 in catalog.go). This is a Band-Aid fix for ISS-027 (stale animekai tokens expire ~1 hour, breaking cached episode IDs).

**Files:** `services/catalog/internal/service/catalog.go:1978` (cache TTL = 5*time.Minute)

**Impact:** Cache misses constantly for popular anime; backend makes 12 redundant API calls per hour per anime. Higher latency, more upstream API load.

**Real fix approach:** Invalidate episode cache when stream extraction fails (detect token expiry), rather than blanket short TTL. Or cache episodes per-provider-per-token-validity window.

---

### Consumet Provider Caching Uses 24h TTL

**Issue:** Which Consumet provider works for an anime is cached for 24 hours (line 1978). If provider availability changes within that window, users get stale results.

**Files:** `services/catalog/internal/service/catalog.go:1978` (cache TTL = 24*time.Hour for `consumet:provider:{id}`)

**Impact:** If provider rotates or goes down mid-day, users continue getting directed to dead provider for hours.

**Fix approach:** Reduce to 6 hours or implement a "provider failed" signal that invalidates the cache entry immediately when a stream request fails.

---

### Large File (2856 lines) in Catalog Service

**Issue:** `services/catalog/internal/service/catalog.go` is 2856 lines — a monolith containing search, enrichment, health checks, all 4 parser integrations, sync logic.

**Files:** `services/catalog/internal/service/catalog.go`

**Impact:** Hard to navigate, test, and reason about. Changes to one parser affect the entire service. No isolation of concerns.

**Fix approach:** Split into smaller services or use dependency injection to isolate parsers. Consider extracting health checker to separate package.

---

## Architectural Fragility

### Multiple Separate Video Players with Duplication Risk

**Issue:** 4 separate Vue components (KodikPlayer, AnimeLibPlayer, HiAnimePlayer, ConsumetPlayer) each implement their own playback logic. Subtitle handling, error recovery, and player lifecycle are not DRY.

**Files:** `frontend/web/src/components/player/{Kodik,AnimeLib,HiAnime,Consumet}Player.vue`, `frontend/web/src/components/SubtitleOverlay.vue`

**Impact:** Bug fix in one player (e.g. subtitle timing) must be replicated to others. New players require copy-pasting significant code.

**Fragile areas:**
- Fullscreen subtitle rendering (SubtitleOverlay uses Vue teleport + requestAnimationFrame — brittle timing)
- Error recovery (each player has different timeout/retry logic)
- Playback state tracking (resume, progress sync)

**Fix approach:** Extract common player interface and base class; factor out subtitle, error handling, and state management.

---

### HLS Proxy Allowed-Domains Maintenance Burden

**Issue:** HLS proxy has a hardcoded allow-list of 15+ domains in `libs/videoutils/proxy.go:230-255`. When new CDNs emerge or existing ones change domains (like HiAnime), the list must be manually updated.

**Files:** `libs/videoutils/proxy.go:230-255` (HLSProxyAllowedDomains)

**Impact:** New CDNs are silently blocked until code is updated. If upstream CDN rotates to a new domain (common for piracy sites), users get 403 errors for hours until deploy.

**Fragile aspects:**
- Domain names like `hydaelyn-25x-*.top` use prefix wildcards; new number schemes will be rejected
- No dynamic domain discovery (e.g. reading from config or runtime database)
- Hard to test (requires mocking upstream CDN responses)

**Fix approach:** Move allowed domains to environment config (env var or Redis set). Implement DNS-based or certificate-based validation as fallback.

---

### AnimeLib Subtitles Still Broken for Direct MP4 Player

**Issue:** AnimeLib parser returns `subtitles` field for some translations (Crunchyroll teams have external ASS/VTT files). AnimeLib HTML5 `<video>` player (native HTML5) cannot render external SRT/ASS/VTT files — it only supports in-stream subtitles.

**Files:** `frontend/web/src/components/player/AnimeLibPlayer.vue`, `services/catalog/internal/service/catalog.go:2146-2155`

**Impact:** AnimeLib translations with external subtitles show no subs in the native MP4 player. Kodik iframe fallback works but is not always available.

**Root cause:** HTML5 `<video>` + `<track>` elements do not support ASS format, and the player doesn't use SubtitleOverlay for AnimeLib streams.

**Fix approach:** When AnimeLib returns MP4 + external SRT/ASS, either:
  1. Use SubtitleOverlay (like HiAnime/Consumet) to render subtitles on top of native video, or
  2. Detect ASS subtitles and auto-fallback to Kodik iframe

---

### On-Demand Catalog + External API Failures Cascade

**Issue:** Anime catalog is not pre-populated. When user searches, the flow is: Shikimori search → local DB → parser (Kodik/HiAnime/Consumet/AnimeLib). If any external API times out or is down, user sees error.

**Files:** `services/catalog/internal/service/catalog.go:1300-1500` (search flow), all 4 parsers in `services/catalog/internal/parser/{*/client.go`

**Impact:** Upstream outages (HiAnime domain migration, Cloudflare blocks, Consumet AnimeKai 500s) cascade to user errors. No offline fallback.

**Documented incidents:**
- ISS-001, ISS-007, ISS-009, ISS-021, AUTO-010, AUTO-012, AUTO-027 all caused complete player unavailability

**Fix approach:**
  1. Pre-populate top 100–1000 anime at startup and refresh periodically
  2. Cache search results aggressively (currently 15 min, could be longer for popular queries)
  3. Implement fallback parsers (if HiAnime down, try Consumet; if both down, return cached data)
  4. Add circuit breaker pattern for parser timeouts

---

### Cache Expiry Mid-Stream Not Handled

**Issue:** Video stream URLs are cached for 1 hour in Consumet (`time.Hour`), but external CDN URLs expire faster (often 30 min). If user pauses a video and resumes after URL expiry, playback fails.

**Files:** `services/catalog/internal/service/catalog.go:1307, 1352, 1408, 1469, 1636` (cache.Set with time.Hour), `libs/videoutils/proxy.go` (no cache-bust on 403/410)

**Impact:** Users cannot reliably pause and resume long videos (documentaries, movies). Worst case: mid-episode pause = playback broken.

**Fix approach:**
  1. Return shorter cache TTL (e.g. 10 min) and refresh token on each play request, or
  2. Detect 403/410 upstream errors and invalidate cache immediately, or
  3. Implement stream URL validation before returning to frontend (HEAD request to check 403 early)

---

### Subtitle Parser Edge Cases

**Issue:** Subtitle parser uses `ass-compiler` (npm package) for ASS format, custom SRT/VTT parsing. Edge cases:
  - ASS files with non-standard timing formats
  - Malformed SRT (missing blank lines between cues)
  - VTT with embedded HTML

**Files:** `frontend/web/src/utils/subtitle-parser.ts`, depends on `ass-compiler` npm package

**Impact:** Subtitles fail silently or display incorrectly for edge-case files. Users see "no subtitles" instead of malformed-but-readable text.

**Testing:** No visible unit tests for parser edge cases.

**Fix approach:** Add test fixtures for malformed ASS/SRT/VTT files. Implement graceful fallback (log warnings, render best-effort).

---

## Security Concerns

### JWT_SECRET Environment Variable Required

**Issue:** JWT secret is stored in environment variable `JWT_SECRET`. If container env vars are leaked (Docker inspect, pod logs), all JWT tokens can be forged.

**Files:** `services/auth/internal/config/config.go:46-70`

**Current mitigation:** Secrets are managed via Kubernetes Secrets (in deploy/) and Docker .env files (not committed). But during debugging or log collection, env vars can accidentally surface.

**Recommendation:** Rotate JWT_SECRET regularly (e.g. quarterly). Consider using a key management service (AWS KMS, HashiCorp Vault) for production. Add audit logging for JWT creation/validation.

---

### HLS Proxy Allows-List Can Be Bypassed with Subdomain Tricks

**Issue:** The allow-list check in `libs/videoutils/proxy.go:184-209` uses wildcard matching, but edge cases exist:
  - `netmagcdn.com` matches `evil.netmagcdn.com` (correct), but also matches any subdomain
  - Prefix wildcard `htv-*` matches `htv-evil.com` (intended) but also `p.htv-evil.com` (unintended nesting)

**Files:** `libs/videoutils/proxy.go:184-209` (isDomainAllowed)

**Impact:** Low risk (CDNs are pre-vetted by upstream providers), but domain allow-list is a trust boundary.

**Fix approach:** Use explicit domain matching (no wildcards). If wildcards needed, validate subdomain depth to prevent nested bypasses.

---

### SSRF Risk in HLS Proxy Domain Allow-List

**Issue:** HLS proxy allows `netmagcdn.com`, `owocdn.top`, etc. These are external CDNs, but an attacker could potentially:
  1. Guess internal service hostnames (e.g. `catalog.internal`, `postgres.internal`)
  2. If those hostnames resolve from the proxy service, SSRF to internal endpoints

**Files:** `libs/videoutils/proxy.go:82-146` (ProxyStream)

**Mitigation:** Allow-list is pre-vetted CDNs only, not user-input. Domain validation at line 90 checks against allow-list before making request.

**Recommendation:** Add rate limiting per source IP to proxy handler to prevent SSRF scanning. Log all upstream errors to detect probing attempts.

---

### Telegram Bot Token Storage

**Issue:** Telegram bot webhook secret is stored in env var `X-Telegram-Bot-Api-Secret-Token` and verified at runtime in `services/auth/internal/handler/telegram_bot.go:80-82`.

**Files:** `services/auth/internal/handler/telegram_bot.go`

**Mitigation:** Webhook secret is validated on every request. If leaked, attacker can POST fake updates to the webhook.

**Recommendation:** Rotate webhook secret periodically. Use HMAC-SHA256 to sign webhooks (Telegram supports this via X-Telegram-Bot-Api-Secret-Token header).

---

### API Key Storage (SHA-256 Hash)

**Issue:** API keys are hashed with SHA-256 in `users.api_key_hash` column. SHA-256 is cryptographically secure but not designed for password hashing (no salt, no slow hash function like bcrypt).

**Files:** `services/auth/internal/handler/auth.go`, resolved via API key from header

**Mitigation:** API keys are long (64 hex chars = 256 bits) and random, so dictionary attacks are infeasible. Keys are rate-limited at gateway layer.

**Recommendation:** Consider bcrypt or Argon2 for future key storage. Add key rotation API.

---

### CORS Headers Set to "*" (Allow-Origin: *)

**Issue:** HLS proxy sets `Access-Control-Allow-Origin: *` at `libs/videoutils/proxy.go:126, 335`.

**Files:** `libs/videoutils/proxy.go:126, 335`

**Impact:** Any website can embed the video streams. If video URLs contain user-specific data or PII, this could leak via CORS.

**Mitigation:** Video URLs are temporary tokens (1 hour), not user-specific. Streams are from public CDNs, not user uploads.

**Recommendation:** Restrict CORS to `https://animeenigma.ru` if possible. If browser-based HLS.js client is deployed on user domains (unlikely), CORS origin should be configurable.

---

## Performance Bottlenecks

### Search Results Not Cached on Shikimori Misses

**Issue:** When searching for anime by name, if no local DB match, the code calls Shikimori API directly. Even if Shikimori returns "no results", this negative result is not cached.

**Files:** `services/catalog/internal/service/catalog.go:200-202` (cache.Set on success only)

**Impact:** Repeated searches for misspelled or non-existent anime titles hit Shikimori API repeatedly, adding latency.

**Fix approach:** Cache negative results with a shorter TTL (5 min instead of 15 min). Mark cached "no results" separately so UI can show "not found in catalog".

---

### Jikan English Title Lookups Cached for 7 Days

**Issue:** Jikan API is called for English title enrichment and cached for 7 days. If anime title is corrected on Jikan side (rare but possible), users see stale data for a week.

**Files:** `services/catalog/internal/service/catalog.go:1838` (7*24*time.Hour)

**Impact:** Low impact (Jikan titles rarely change), but title mismatches persist across a week.

**Fix approach:** Reduce to 1 day. Jikan title corrections are rare enough to not impact latency.

---

### N+1 Query Pattern in Genre/Video Enrichment (Fallback Path)

**Issue:** In `services/catalog/internal/service/catalog.go`, `enrichAnimesBatch()` does batch loading via `GetForAnimes()`, but if that fails, it falls back to `enrichAnime()` (singular) in a loop — causing N+1 queries.

**Files:** `services/catalog/internal/service/catalog.go:622-638` (fallback loop)

**Impact:** If batch load fails (rare), latency degrades from O(1) DB query to O(N) queries. User sees slow search results.

**Fix approach:** Log the batch failure and implement retry with circuit breaker before falling back to N+1.

---

### WebSocket Connection Limits in Rooms Service

**Issue:** Rooms service uses WebSocket for game rooms. No visible connection limit per room or per user.

**Files:** `services/rooms/cmd/rooms-api/main.go`, `services/rooms/internal/handler/room.go` (not fully inspected)

**Impact:** Single room could accept unlimited concurrent connections, memory leak potential.

**Fix approach:** Add per-room connection limit (e.g. 10 concurrent players) and per-user limit (1 connection per user). Disconnect on limit exceeded.

---

### Frontend Bundle Size Not Tracked

**Issue:** Frontend includes `ass-compiler` npm package (ASS subtitle parsing) and `video.js` + `hls.js` (player), but no bundle size tracking in CI.

**Files:** `frontend/web/package.json`, planned: `kuroshiro` + `kuromoji.js` (418KB dictionary for furigana generation per MEMORY.md)

**Impact:** Bundle bloat goes unnoticed. Kuroshiro+kuromoji will add 418KB if not lazy-loaded. Mobile users see slower load times.

**Fix approach:** Add `vite-plugin-compression` (already in dependencies) for gzip. Implement dynamic import for subtitle parser and furigana generation. Add bundle size threshold in CI.

---

## Testing Gaps

### No Integration Tests for Video Parsers

**Issue:** Catalog service has unit tests, but no integration tests that actually call upstream APIs (Shikimori, Kodik, HiAnime, Consumet, AnimeLib).

**Files:** `services/catalog/` — test files not found in repo scan

**Impact:** Parser regressions (e.g. API schema changes, timeout issues like ISS-007) are only caught in production.

**Fix approach:** Add integration tests that run against staging/test instances of upstream APIs. Mock failures to test fallback logic.

---

### Subtitle Parser Edge Cases Not Tested

**Issue:** `frontend/web/src/utils/subtitle-parser.ts` has no visible test fixtures for malformed ASS/SRT/VTT files.

**Files:** `frontend/web/src/utils/subtitle-parser.ts`

**Impact:** Edge cases (missing newlines, invalid timing, UTF-8 issues) cause silent failures.

**Fix approach:** Add Vitest or Jest test suite with fixtures for edge cases. Use real subtitle files from the wild.

---

### Error Report Data Not Validated

**Issue:** Error reports from frontend are captured with minimal validation. If frontend sends truncated or malformed diagnostics, reports are still saved.

**Files:** `services/player/internal/handler/report.go`, `frontend/web/src/utils/diagnostics.ts`

**Impact:** Telegram notifications and stored reports may be incomplete or useless.

**Fix approach:** Add validation schemas (e.g. Zod or Go structs with tags). Reject reports missing critical fields.

---

## Operational Concerns

### Single-Server Deployment

**Issue:** AnimeEnigma is deployed on a single machine (per CLAUDE.md "self-hosted groups"). No redundancy, no load balancing, no failover.

**Files:** `docker/docker-compose.yml` (single machine), `deploy/kustomize/` (Kubernetes manifests exist but not used)

**Impact:** Any hardware failure = complete outage. Database backups are critical.

**Recommendation:** Document recovery procedure. Implement nightly backups to external storage (S3, B2). Test backup restoration quarterly.

---

### MinIO Single Instance (No Replication)

**Issue:** MinIO is deployed as a single container with a local volume. No replication or erasure coding.

**Files:** `docker/docker-compose.yml` (MinIO service definition)

**Impact:** Hardware failure = data loss. No geo-distributed backup.

**Recommendation:** Set up MinIO in distributed mode (multiple volumes across multiple machines) or enable replication to secondary MinIO cluster.

---

### Database Auto-Migration Only (No Explicit Migrations)

**Issue:** Schema changes use GORM's `AutoMigrate()` which creates new tables/columns but doesn't drop or modify existing ones. No version control of migrations.

**Files:** `services/catalog/cmd/catalog-api/main.go`, all service `main.go` files

**Impact:** Schema drift between environments. Rollback is manual (SQL edits). Complex schema changes require downtime.

**Recommendation:** Implement explicit SQL migrations (e.g. golang-migrate) with version control. Test migrations on staging before production.

---

### No Explicit Connection Pool Limits

**Issue:** Database and Redis clients use default connection pool sizes. No visible configuration for connection limits per service.

**Files:** `libs/database/`, `libs/cache/` (config not inspected)

**Impact:** Under high load, connection pools can exhaust, causing cascading failures.

**Recommendation:** Set explicit max connection limits (e.g. 20 for catalog service, 10 for auth). Monitor pool utilization in Grafana.

---

### Scheduler Job Status Persists in Memory

**Issue:** Scheduler job last_run timestamps are stored in `scheduler_job_last_success_timestamp` Prometheus metric (as seen in ISS-013). If scheduler container restarts, metric is reset, alerting fires incorrectly.

**Files:** `services/scheduler/internal/service/`, Grafana alert rules in `docker/grafana/provisioning/alerting/rules.yml`

**Impact:** False alerts after restarts (AUTO-032, AUTO-046 show this pattern).

**Recommendation:** Store job status in database instead of Prometheus metric. Prometheus should only scrape current status.

---

## Known Bugs & Edge Cases

### Hanime Parser Not Fully Tested

**Issue:** Hanime parser exists (`services/catalog/internal/parser/hanime/client.go`) but has minimal usage in the issues log. Email/password authentication required but details sparse.

**Files:** `services/catalog/internal/parser/hanime/client.go`

**Impact:** Hanime player may fail in production if session tokens expire or auth fails. No health check covers full playback chain (ISS-009 shows this is a risk).

**Recommendation:** Add integration tests for Hanime parser. Verify session token refresh works.

---

### Consumet Provider Fallback Logic Not Clear

**Issue:** When Consumet search returns multiple anime with similar names, the code picks the first match. If first match is a different anime (e.g. original series vs spinoff), users get wrong streams.

**Files:** `services/catalog/internal/service/catalog.go:1950-1960` (first match wins)

**Impact:** Rare edge case, but anime like "Sword Art Online" with many sequels/spinoffs could mismatch.

**Recommendation:** Implement scoring based on name similarity (Levenshtein distance) instead of first match.

---

### Maintenance Bot Spam Cycles (ISS-044)

**Issue:** Maintenance bot spammed repeated alert fire/resolve cycles on 2026-04-07. Root cause not documented.

**Files:** `services/maintenance/cmd/maintenance/main.go` (alert dispatch logic)

**Impact:** Admin chat flooded with false notifications, signal-to-noise ratio degraded.

**Status:** Pending fix (escalated in ISS-044).

**Recommendation:** Implement alert deduplication and debouncing (e.g. alert must fire 2 times before notifying).

---

## Cross-Cutting Risks

### Hardcoded API URLs in Frontend (Partially Fixed)

**Issue:** Frontend hardcoded `localhost:8000` API URL until 2026-04-06 (AUTO-039). Build-time fix applied, but risk remains if someone rebuilds without `VITE_API_URL` arg.

**Files:** `frontend/web/Dockerfile`, `frontend/web/vite.config.ts` (build config)

**Impact:** Frontend breaks when deployed to different hostname (e.g. animeenigma.ru instead of localhost).

**Recommendation:** Use runtime API base discovery (e.g. read from window.location.origin or window.__API_URL__). Or ensure CI build always passes VITE_API_URL.

---

### Furigana Generation Planned But Not Implemented

**Issue:** Phase 4 of Japanese subtitle work (MEMORY.md) plans to add kuroshiro + kuromoji.js for furigana generation. This adds 418KB dictionary, not lazy-loaded.

**Files:** Not yet in codebase. Planned: `frontend/web/src/utils/furigana.ts`

**Impact:** Future bundle bloat if not lazy-loaded. Blocking furigana feature work.

**Recommendation:** Implement as dynamic import. Test lazy-load timing on 3G networks.

---

### Docker Image Layer Caching

**Issue:** Service Dockerfiles copy `go.mod` / `go.sum` before copying source, which is good for cache. But no `.dockerignore` prevents copying test files, vendor/ (if present), etc.

**Files:** `services/*/Dockerfile`

**Impact:** Larger build context, slower builds, larger images.

**Recommendation:** Add `.dockerignore` files to exclude test files, .git/, node_modules/, etc.

---

## Mitigation Summary

**Highest Priority (Fix immediately):**
1. **ISS-006** (HLS on iOS Safari) — investigate codec issues
2. **ISS-044** (Maintenance bot spam) — implement alert deduplication
3. **UA-053/055/056** (Mobile navbar) — add aria-expanded, role=dialog, schedule link, fix z-index

**High Priority (Fix within 1 sprint):**
1. Range request handling for seeks in video
2. Consumet episode cache invalidation logic
3. AnimeLib subtitle rendering (use SubtitleOverlay for MP4 + external subs)
4. Search result negative caching

**Medium Priority (Address in next quarter):**
1. Split large catalog.go service
2. Dynamic HLS allowed-domains config
3. Parser integration tests
4. Add database migration tool

**Low Priority (Nice to have):**
1. Bundle size tracking in CI
2. Hanime parser full test coverage
3. Consumet name matching scoring
4. MinIO replication setup

---

*Concerns audit: 2026-04-27*
