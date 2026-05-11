# Phase 16: AnimePahe + New EnglishPlayer - Context

**Gathered:** 2026-05-11
**Status:** Ready for planning
**Mode:** Smart-discuss (autonomous batch-table proposal, accepted as-is)

<domain>
## Phase Boundary

A logged-in user opens the anime detail page, clicks the new "English" tab (replacing the two old "HiAnime" + "Consumet" tabs), and watches an episode end-to-end via AnimePahe through a new unified `EnglishPlayer.vue`. The orchestrator selects AnimePahe (only live provider in this phase); the user can manually override via a "Source: AnimePahe" dropdown inside the player toolbar. Provider selection persists per-anime via `useWatchPreferences`. The old HiAnime + Consumet player tabs and their backend routes remain alive but hidden — reachable only via `?legacy=1` for debug — until Phase 20 cutover. ReportButton bug reports from the English tab include `provider:animepahe` plus the orchestrator's full `tried[]` chain.

Backend: new AnimePahe Provider implementation in `services/scraper/internal/providers/animepahe/` (registered with orchestrator at scraper boot), Kwik embed extractor in `services/scraper/internal/embeds/kwik.go` (mirrors `embeds/megacloud.go`), malsync.moe lookup with 24h cache, all four `/scraper/{episodes,servers,stream,health}` endpoints now serve real data (not Phase 15's 503 stubs). Frontend: new `EnglishPlayer.vue` component + new `scraperApi` exports in `frontend/web/src/api/client.ts`.

</domain>

<decisions>
## Implementation Decisions

### Provider override UX (inside the player)
- Dropdown lives inside the player toolbar (compact, beside the existing quality selector), not above the video
- Switching source mid-episode auto-resumes at current timestamp via Video.js `player.currentTime()` preserve/restore
- Override-fail policy: auto-fallback to orchestrator default + toast notification (no hard error blocking the user)
- Persistence scope: per-anime via existing `useWatchPreferences` composable (same pattern as RU translation persistence)

### Tab visibility & legacy access
- English tab label: plain "English" (matches Kodik/AnimeLib labeling pattern)
- `?legacy=1` URL flag adds HiAnime + Consumet tabs alongside English, each marked `(debug)` — does NOT replace English
- Default tab when all available: English (the new replacement; only one logged-in users see by default)
- Mobile: same dropdown UI on all viewports, with screen-width-aware tooltip placement (no separate mobile swipe-tab pattern)

### Implementation knobs (AnimePahe + Kwik)
- Kwik unpacker lives at `services/scraper/internal/embeds/kwik.go` (mirrors MegacloudClient pattern; reusable for any future provider that embeds Kwik)
- malsync cache key shape: `malsync:{mal_id}:{provider}` (Redis; enables multi-provider lookups via key prefix in Phases 18-19)
- DDoS-Guard cookie jar: one persistent jar per provider, shared across all AnimePahe requests (no per-request resets)
- `tried[]` chain surfaced via response body field `meta.tried` (consistent with existing JSON wrapper used by Phase 15 handlers); ReportButton diagnostics include both `provider:animepahe` (active source) and the full `tried[...]` chain

### Locked by REQUIREMENTS (no grey area)
- SCRAPER-PAHE-01: malsync.moe lookup, 24h cache, fuzzy-title fallback
- SCRAPER-PAHE-02: Episode list cached 6 hours
- SCRAPER-PAHE-03: HLS m3u8 at 480p / 720p / 1080p via kwik.cx + dop251/goja; stream URLs cached ≤ min(parsed expiry − 30s, 5min)
- SCRAPER-PAHE-04: cookiejar via golang.org/x/net/publicsuffix; no headless browser
- SCRAPER-PAHE-05: kwik.cx, owocdn.top, uwucdn.top (+ any discovered) appended to `libs/videoutils/proxy.go::HLSProxyAllowedDomains`
- SCRAPER-UI-01: new EnglishPlayer.vue uses Video.js / HLS.js + existing SubtitleOverlay.vue for Jimaku
- SCRAPER-UI-03: new `scraperApi` in `frontend/web/src/api/client.ts`; do NOT repoint hianimeApi/consumetApi (deleted in Phase 20)
- SCRAPER-NF-02: cache TTLs (24h/6h/15min/≤5min) match data freshness contract
- SCRAPER-NF-05: ReportButton emits provider:<name> + tried[] chain

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `services/scraper/internal/domain/` — Provider interface, EmbedExtractor + Registry, BaseHTTPClient (retryablehttp + per-host rate limiter + cookiejar), sentinel errors. Phase 15 lands all of these.
- `services/scraper/internal/service/orchestrator.go` — sequential failover with `prefer` override + parser_fallback_total counter + HealthSnapshot. Phase 16 just calls `Register(animepahe)` at startup.
- `services/scraper/internal/embeds/megacloud.go` — pattern for embed extractor (HTTP-wraps sidecar). Kwik will mirror this structure but use goja in-process instead of an external sidecar.
- `services/scraper/internal/handler/scraper.go` — handlers for /scraper/{episodes,servers,stream,health} currently return 503 stubs; Phase 16 swaps the orchestrator call from stub to live AnimePahe path.
- `services/catalog/internal/parser/scraper/client.go` — thin HTTP wrapper to scraper service; the catalog handlers already passthrough response body + status verbatim.
- `frontend/web/src/components/player/HiAnimePlayer.vue` (~400 lines) — reference for Video.js+HLS.js wiring + SubtitleOverlay teleport + ReportButton integration. EnglishPlayer.vue will reuse all of this, just swap the API client.
- `frontend/web/src/components/player/SubtitleOverlay.vue` — JP subtitle renderer (used by HiAnime + Consumet today). Reuse as-is.
- `frontend/web/src/composables/useWatchPreferences.ts` — per-anime persistence store; will gain a `preferredScraperProvider` field.
- `libs/videoutils/proxy.go` — HLS proxy with `HLSProxyAllowedDomains` allowlist for CORS. Phase 16 appends AnimePahe CDN hostnames.

### Established Patterns
- TDD discipline: every plan ships test commits first (RED) then implementation (GREEN). Phase 15 maintained 1:1 test/impl commits across all 4 plans.
- Atomic commits per task: each PLAN.md task is one commit; SUMMARY.md is a separate docs commit.
- Worktree-based parallel execution within a wave: each plan runs in `.claude/worktrees/agent-*`, merged back via `git merge --no-ff` with STATE.md / ROADMAP.md "main wins" snapshot/restore.
- Defensive error classification: `failoverDecision()` splits errors into terminal (ctx) vs retryable (NotFound/ProviderDown/ExtractFailed); unknown errors classified as retryable.
- All HTTP responses follow the `httputil.Response` JSON wrapper (`{success, data}` for OK, `{error, ...}` for errors), EXCEPT the 503 stubs which use raw `{error, phase}` so catalog can passthrough verbatim. Phase 16 stream responses will use the wrapped form.

### Integration Points
- Scraper boot: `services/scraper/cmd/scraper-api/main.go` — current construction order `registry → MegacloudClient → registry.Register(mc) → orchestrator → handler → router`. Phase 16 inserts `animepaheClient := animepahe.New(...) → orchestrator.Register(animepaheClient) → registry.Register(kwikExtractor)` before the orchestrator is built.
- Anime detail page: `frontend/web/src/views/Anime.vue:420-460` — current `v-else-if` chain over `videoProvider` (`kodik` / `animelib` / `hianime` / `consumet` / `hanime`). Phase 16 adds an `english` branch that mounts `EnglishPlayer`.
- Per-anime persistence: `useWatchPreferences` already keys by `animeId` and persists to backend (logged-in) + localStorage (anon). Adding `preferredScraperProvider` is one new field.

</code_context>

<specifics>
## Specific Ideas

- The `meta.tried` response body field is a new contract — scraper handlers must include it in every response (success and error). Format: `"meta": { "tried": ["animepahe"] }` (array of provider names, in order attempted). Catalog passthrough preserves the field. Frontend ReportButton reads `meta.tried` from the last scraper response cached in the player state and includes it in the report payload alongside `provider:animepahe` (the currently active source).
- The Kwik unpacker uses `dop251/goja` in-process (no external sidecar like Megacloud's Node helper) because the JS unpacking is a self-contained packer that doesn't need browser APIs. Sandboxing: goja runtime is constructed fresh per call (no global state), with a 5-second execution timeout.
- The English tab is the default for logged-in users with malsync coverage. For anime without malsync coverage, the English tab shows an empty state with "Not available in English — try Kodik or AnimeLib" rather than disappearing entirely (consistency over chrome).
- `?legacy=1` is a URL query parameter, not a feature flag or env var — it's per-request, scoped to the current Anime.vue view only, not persisted.

</specifics>

<deferred>
## Deferred Ideas

- Server-side rendering of episode list (currently client-side after API call) — deferred, not in v3.0 scope
- Adding a "report dead provider" admin button — deferred to Phase 17 (Observability)
- Multi-source picture-in-picture comparison — out of scope, not requested
- Quality auto-selection based on network speed — existing HLS.js does this; no special handling needed

</deferred>
