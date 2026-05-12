---
phase: 18-9anime
fixed_at: 2026-05-12T00:00:00Z
review_path: .planning/phases/18-9anime/18-REVIEW.md
iteration: 1
findings_in_scope: 11
fixed: 11
skipped: 0
status: all_fixed
---

# Phase 18: Code Review Fix Report

**Fixed at:** 2026-05-12T00:00:00Z
**Source review:** `.planning/phases/18-9anime/18-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 11 (3 BLOCKER + 8 WARNING; 6 INFO skipped per fix_scope)
- Fixed: 11
- Skipped: 0

**Out of scope (deliberately skipped):**
- WR-03 (runGoja error-shape ambiguity) — not listed in the explicit warning
  set provided in the fix request; left for a future cycle.
- IN-01..IN-06 — informational-only, per fix_scope = critical_warning.

**Verification (run after final commit, all green):**
- `go build ./services/scraper/...` — clean.
- `go test ./services/scraper/...` — all packages pass (embeds, gogoanime,
  animepahe, service, handler, transport, domain, fuzzy, golint, health,
  testharness, config).
- `bunx vue-tsc --noEmit` — exit 0.
- `bunx eslint src/components/player/EnglishPlayer.vue` — exit 0.
- Phase 16 Kwik regression check: `go test -run "Kwik" ./services/scraper/internal/embeds/...` — all green.

## Fixed Issues

### CR-01: VibePlayer subtitle URL is not validated against any allowlist

**Files modified:** `services/scraper/internal/embeds/vibeplayer.go`, `services/scraper/internal/embeds/vibeplayer_test.go`
**Commit:** d3b3214
**Applied fix:** Added (1) a URL-shape regex (`^https?://[^"\s]+\.(?:vtt|srt|ass)(?:\?[^"\s]*)?$`) and (2) a host allowlist (`vibeplayer.site` + strict subdomains + `cdn.cimovix.store`) gating the captured `const subtitle = "..."` string. Failed payloads (javascript:, file:, data:, ftp:, off-host https:, suffix attacks, wrong/missing extensions) drop the Track but keep the m3u8 stream playable, and emit `parser_zero_match_total{selector="vibeplayer_sub_url_shape"}` so hostile-upstream regressions stay visible. Added `TestVibePlayer_Extract_SubtitleURLValidation` covering 13 cases (10 attacker payloads + 3 known-good).

### CR-02: Saved provider preference (`gogoanime`) is dropped on cold load

**Files modified:** `frontend/web/src/components/player/EnglishPlayer.vue`
**Commit:** d1f8cb0
**Applied fix:** Added a second restore pass inside `onMounted`, AFTER the live providers array lands from `scraperApi.getHealth()`. The new block only assigns when `selectedProvider.value === null` so it doesn't clobber an already-restored synchronous match. The 24h preference TTL is now usable for non-default providers (gogoanime, future providers).

### CR-03: `computeStreamTTL` caches malformed-`e=` URLs for the full 5min cap

**Files modified:** `services/scraper/internal/providers/gogoanime/cache.go`, `services/scraper/internal/providers/gogoanime/cache_test.go`
**Commit:** 1a7b300
**Applied fix:** Split the parse-error path so `e=` empty (truly static URL) returns `streamTTLFallback`, but `e=` present-but-unparseable / non-positive returns `0` so the orchestrator re-extracts on the next request. Renamed `non_integer_e_returns_fallback` → `non_integer_e_returns_zero` to lock the new behaviour, and added `zero_e_returns_zero` / `negative_e_returns_zero` for the bordering cases.

### WR-01 (user's W-01): packed_common reused HTTP timeout as goja budget

**Files modified:** `services/scraper/internal/embeds/packed_common.go`, `services/scraper/internal/embeds/streamhg.go`, `services/scraper/internal/embeds/earnvids.go`
**Commit:** 0359be9
**Applied fix:** Introduced `defaultPackedGojaTimeout = 5 * time.Second` distinct from `defaultPackedHTTPTimeout = 15 * time.Second`. Both StreamHG and Earnvids constructors now wire HTTP=15s and goja=5s — matching the KwikExtractor defaults exactly. A hostile packed-JS infinite-loop payload can no longer pin a goroutine for 15s of CPU.

### WR-02 (user's W-02): wrong selector emitted on `runGoja` failure

**Files modified:** `services/scraper/internal/embeds/packed_common.go`, `services/scraper/internal/embeds/streamhg.go`, `services/scraper/internal/embeds/earnvids.go`
**Commit:** 9fed6e0
**Applied fix:** Added `selectorGojaFail` field to `packedExtractor` and emit it on the `runGoja` error path (was previously reusing `selectorPackerFail`). Concrete values: `streamhg_goja` / `earnvids_goja`. The Phase 17 dashboard can now distinguish runtime trips (JS shape change / timeout / hostile loop) from `extractPacker` balance-paren misses (HTML shape change).

### WR-04 (user's W-03): raw extractor error leaked tokens via stage health

**Files modified:** `services/scraper/internal/providers/gogoanime/client.go`
**Commit:** 1bf4014
**Applied fix:** Added `classifyStreamErr` helper that maps an extractor error to one of `extract_failed` / `provider_down` / `not_found` / `unknown`. `GetStream` now records only `"gogoanime: stream <category>"` to `stages[StageStream].LastErr` (surfaced to admins via `/api/admin/scraper/health`). The original err is still returned to the orchestrator unchanged so failover behaviour is preserved.

### WR-05 (user's W-04): hand-rolled insertion sort

**Files modified:** `services/scraper/internal/providers/gogoanime/client.go`
**Commit:** 28c7857
**Applied fix:** Replaced the manual insertion-sort loop with `sort.Ints(nums)` (stdlib introsort, O(n log n) worst case). Added `"sort"` to the import block.

### WR-06 (user's W-05): dead `_ = err` in MalSyncClient.Lookup

**Files modified:** `services/scraper/internal/providers/gogoanime/malsync.go`
**Commit:** 37a69d1
**Applied fix:** Added an optional `*logger.Logger` field to `MalSyncClient` (defaults to `logger.Default()`; overridable via new `WithMalSyncLogger` option). The unexpected-error branch now logs a warn-level breadcrumb with the key + error instead of swallowing silently. The Lookup path still falls through to the upstream so transient Redis blips don't break MAL resolution.

### WR-07 + WR-08 (user's W-06 + W-07): wire sourceSwitchFailed + null-guard setPreferredScraperProvider

**Files modified:** `frontend/web/src/components/player/EnglishPlayer.vue`
**Commit:** 219994c
**Note:** Both findings touch the same `catch` block in `switchProvider`, so they're paired in a single atomic commit per the agent's multi-file-finding rule extended to a multi-finding-single-hunk situation. Both fixes are still individually traceable via the commit message.
**Applied fix:**
- WR-07: `error.value = t('player.sourceSwitchFailed', { provider: capitalizeProvider(...) })` surfaces the failure via the same overlay used elsewhere in the player. The locale key now has a live consumer; pre-fix it was dead in all 3 bundles.
- WR-08: Guarded `setPreferredScraperProvider(prior)` with `if (prior !== null)`. The composable accepts null today as the clearing call, but the guard makes intent explicit and defends against future signature tightening.

### WR-09 (user's W-08): dead `_ = cat` in fetchEpisodes

**Files modified:** `services/scraper/internal/providers/gogoanime/client.go`
**Commit:** 7ad4207
**Applied fix:** Removed the unused `cat` (domain.CategorySub|Dub) variable. The `isDub` boolean that actually gates the href-filter loop is unchanged. ListServers re-derives the category from the episode-ID slug independently, so propagation through the Episode struct was never wired and was misleading to readers.

---

_Fixed: 2026-05-12T00:00:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
