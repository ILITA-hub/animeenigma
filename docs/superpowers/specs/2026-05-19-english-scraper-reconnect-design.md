# English Scraper Reconnect — Design

> **SUPERSEDED 2026-05-19** — content absorbed into v3.1 Phase 24 (EN Reconnect) per `.planning/milestones/v3.1-REOPENING.md`. The authoritative planning surface for this work is now `.planning/milestones/v3.1-phases/24-en-reconnect/24-CONTEXT.md`. Project B from this design moved to v3.1 Phase 26 (Provider Expansion); browse-filter activation moved from this design's Phase A.2 into v3.1 Phase 26 Wave 1 per Phase 24 D4. This file remains in `docs/superpowers/specs/` as a historical record of the brainstorming path that led to v3.1's reopening.

**Date:** 2026-05-19
**Status:** Superseded — see header note
**Scope:** Project A of the two-part "fix older + add new providers" initiative. Project B (provider expansion) is a separate, later cycle.

## Problem

`services/scraper` is a complete microservice (port 8088) shipped through Phases 15–19 with a working failover orchestrator and three registered providers:

- **Gogoanime/Anitaku** — live, primary (Phase 18)
- **AnimePahe** — live, second-chance (Phase 16)
- **AnimeKai** — registered but escape-hatched after upstream shutdown 2026-05-10; every method returns `domain.ErrProviderDown`, so the orchestrator falls through (Phase 19)

The catalog already proxies `/api/anime/{id}/scraper/{episodes,servers,stream,health}` to it, and `scraperApi` exists in `frontend/web/src/api/client.ts`. The 2026-05-18 cleanup deleted the HiAnime and Consumet players and removed the EN tab from `Anime.vue` without ever wiring the scraper to a frontend player. End users currently have **no way to reach the English source** even though the backend works.

## Goal

Restore one user-facing English streaming tab in the player, backed by the scraper service's failover chain. Single-source, single-tab — no source-picker.

## Non-goals

- Adding new providers (= Project B).
- Resurrecting AnimeKai's MegaUp token generator (= Project B, v3.1 carry-over).
- A multi-source switcher UI ("English (Anitaku)" vs. "English (Pahe)" tabs). The scraper's internal orchestrator already picks the best provider; surfacing it would re-introduce the UI complexity that the 2026-05-18 cleanup deliberately removed.
- Bringing back legacy translation/source preference cross-matching that the watch-preference work narrowed.
- Per-anime `prefer` override UI. The scraperApi already accepts `prefer` and we'll wire it through, but the UI to set it is out of scope.

## Backend snapshot (no changes for A.1; small additions in A.2)

Verified before writing this spec:

- `services/scraper/cmd/scraper-api/main.go` registers gogoanime, animepahe, and animekai (kill-switch gates respected via `SCRAPER_DEGRADED_PROVIDERS`).
- `services/catalog/internal/transport/router.go` mounts `/anime/{animeId}/scraper/{episodes,servers,stream,health}` public.
- `services/gateway/internal/transport/router.go` exposes `/api/anime/*` via `ProxyToCatalog` with **no JWT middleware** — anonymous users can already hit the scraper through the public catalog path. Confirmed by reading the `r.HandleFunc("/anime/*", proxyHandler.ProxyToCatalog)` registration outside any `JWTValidationMiddleware` group.
- `frontend/web/src/api/client.ts` exports `scraperApi` with five methods:
  - `getEpisodes(animeId, prefer?)`
  - `getServers(animeId, episodeId, prefer?)`
  - `getStream(animeId, episodeId, serverId, category: 'sub'|'dub', prefer?)`
  - `getHealth()` (catalog-routed via `/anime/_/scraper/health`)

**Phase A.1 is frontend-only.** Phase A.2 adds a small backend change set (one GORM field + one repo method + one switch-case entry — enumerated under "Files touched"). If a backend deficit is discovered during A.1 implementation (e.g., a missing CORS header on the proxied stream URL), it gets fixed in place — but no scope expansion beyond the phase's enumerated files.

## Architecture

```
User opens anime page
  → Anime.vue mounts
  → languageSelector shows tabs: RU | EN | 18+ | RAW
  → user clicks EN
  → Anime.vue mounts <EnglishPlayer animeId="..." malId="..." />
    → onMounted: scraperApi.getEpisodes(animeId)
       (catalog → scraper /scraper/episodes → orchestrator → first healthy provider)
    → user picks episode → scraperApi.getServers(animeId, episodeId)
    → user picks server + category → scraperApi.getStream(...)
    → HLS URL returned → hls.js attaches to <video>
    → SubtitleOverlay teleports onto the fullscreen element when user enables JP subs
    → ReportButton, skip-intro CTA, watch-history tracking work the same as AnimeLibPlayer
```

## Components

### 1. `EnglishPlayer.vue` (new)

**Path:** `frontend/web/src/components/player/EnglishPlayer.vue`

**Responsibility:** Render the English-source player. Talks to `scraperApi`. One file, one purpose.

**Template structure** (mirrors `AnimeLibPlayer.vue` proportions; ~600–800 lines is acceptable for a player component on this codebase):

- Loading state for episode list.
- Empty state: "No English episodes available — try Kodik or AnimeLib."
- Two-column layout: video + sidebar.
- `<video ref="videoRef" controls playsinline>` with `hls.js` attached (HLS) or direct `src` (MP4 fallback if a provider returns one).
- Inline error panel for stream failures.
- Server picker dropdown above the video (compact — server name + sub/dub badge).
- Episode list sidebar on desktop, drawer/scrollable on mobile.
- `SubtitleOverlay` teleported to fullscreen element when user enables JP subs.
- `OtherSubsPanel` for the existing Jimaku selector.
- `ReportButton` per the standard player ReportButton pattern.

**Props:**
- `animeId: string` (UUID)
- `malId: string` (passthrough for any UI that needs it; the scraper resolves it server-side via FindID)
- `initialEpisode?: number` (for resume-from-history)

**Emits:**
- `progress` (current time, duration) — for watch-history tracking
- `episode-change` (episode number)
- `ended`

**State:**
- `loadingEpisodes`, `loadingServers`, `loadingStream`
- `episodes: ScraperEpisode[]`, `selectedEpisode`
- `servers: ScraperServer[]`, `selectedServer`, `category: 'sub' | 'dub'`
- `streamUrl`, `subtitleTracks`, `intro: {start, end}`, `outro: {start, end}`
- `error: string | null`

**Stream lifecycle:**
- On episode change: fetch servers, auto-pick first server, fetch stream.
- On server change: refetch stream only.
- On `<video error>` event: attempt one retry with a fresh `/stream` call (stream URLs have ~5 min TTL per the scraper's cache). If retry fails, surface error.

**HLS player choice:** `hls.js`. The existing `videoutils/proxy.go` handles CORS for streaming CDNs already. No video.js bundle — keep it small. (HiAnimePlayer.vue used video.js; we're not preserving that decision since the component is being rewritten clean.)

### 2. `Anime.vue` rewiring

**File:** `frontend/web/src/views/Anime.vue`

Three local changes:

a. **Type widening** at the language/provider whitelist arrays (lines ~1117–1130 from the 2026-05-18 localStorage-sanitization fix):

```ts
const VALID_LANGUAGES = ['ru', 'en', '18+', 'raw'] as const
const VALID_PROVIDERS = ['kodik', 'animelib', 'english', 'hanime', 'raw'] as const
```

b. **Tab button:** add `<button>` for EN between RU and 18+. Label from `videoTab.english` i18n key. Mirrors the surviving RU tab markup exactly.

c. **Player mount branch:** add a `v-else-if` block for `videoProvider === 'english'` that renders `<EnglishPlayer :animeId="anime.id" :malId="anime.mal_id" />`.

d. **Remove** the `applyResolvedCombo` filter that strips `'en'` / `'english'` (~10 lines of dead code now becoming live).

e. **Remove** the leftover stale-localStorage comment on line 1118 that references `'english' / 'hianime' / 'consumet'` — rewrite to just note the whitelist's purpose.

### 3. Type-union widening (small, mechanical)

- `frontend/web/src/types/preference.ts:2` — `player` union already includes `'english'`. Confirm and leave.
- `frontend/web/src/composables/useOverrideTracker.ts:28` — `PlayerName` union already includes `'english'`. Confirm and leave.
- `frontend/web/src/api/client.ts:424` — preference body type already includes `'english'`. Confirm and leave.
- `frontend/web/src/composables/useBrowseFilters.ts` — `Provider` union currently `'kodik' | 'animelib'`. Add `'english'` plus a PROVIDER_VALUES entry.

The widening for `useOverrideTracker` / `client.ts` / `preference.ts` is **already** done (vestige of conservative cleanup) — we just need to start using the slot.

### 4. `BrowseSidebar.vue` provider filter

**File:** `frontend/web/src/components/browse/BrowseSidebar.vue`

Add a third row to `providerOptions` (currently lines 247–258):

```ts
{
  value: 'english',
  label: t('browse.filters.provider.english'),
  accent: 'text-purple-500 focus:ring-purple-500',
},
```

Purple chosen to avoid collision with cyan (kodik) and orange (animelib).

Backend `provider` filter at `services/catalog/internal/handler/catalog.go` currently switches on `"kodik", "animelib"` only — verified. To activate the new filter row we need to (a) add `"english"` to the switch case, (b) add a `has_english bool` GORM field to the `Anime` model, and (c) add a `SetHasEnglish` repo method. These are A.2 deliverables — see "Phased rollout" below. **For Phase A.1**, the BrowseSidebar new row is purely cosmetic / forward-compatible: it shows in the UI but matches nothing yet. Decision: defer adding the row until A.2 to avoid shipping a non-functional filter. The provider-filter changes in this design therefore land entirely with A.2, NOT A.1.

### 5. i18n keys (minimal set)

Three locale files: `en.json`, `ru.json`, `ja.json`. Add only these new keys:

- `videoTab.english` — "English" / "Английский" / "英語"
- `browse.filters.provider.english` — "English" / "Английский" / "英語"
- `player.englishEmpty` — "No English episodes available — try Kodik or AnimeLib." (and equivalents)
- `player.englishUnavailable` — "English sources temporarily unavailable. Try Kodik or AnimeLib." (shown on `503 no_providers`)
- `player.serverPicker` — "Server" / "Сервер" / "サーバー" (for the server-picker label)
- `player.categorySub` / `player.categoryDub` — "Sub" / "Dub" labels

**Do not** re-introduce the cleanup-removed keys (`tabEnglish`, `tabDebugSuffix`, `source`, `sourceSingleTooltip`, `sourceUnhealthy`, etc.). Those served a multi-source-switcher UI that we're not rebuilding.

### 6. Health-aware tab hiding

On `Anime.vue` mount, fire `scraperApi.getHealth()` once. The response shape per `services/scraper/internal/domain/provider.go`:

```ts
{
  providers: Array<{
    provider: string,
    stages: Record<string, { status: 'UP' | 'DOWN' | 'DEGRADED' | 'UNKNOWN', ... }>
  }>
}
```

If **every** provider reports `DOWN` across **all** stages, hide the EN tab. If at least one provider has a `UP` stage, show it normally. Cache the health snapshot for 60s on the client to avoid hammering the endpoint when the user opens many anime pages.

Failure of the health call itself: **show the tab anyway**. Better to risk a "click EN → see empty state" than to hide a working source because of a transient health-endpoint hiccup.

## Data flow

```
EnglishPlayer.vue
  ├─ onMounted
  │   ├─ scraperApi.getHealth() — gate-decision in Anime.vue, already done before mount
  │   └─ scraperApi.getEpisodes(animeId)
  │       → catalog /anime/{id}/scraper/episodes
  │       → scraper /scraper/episodes
  │       → orchestrator.runFailover(prefer="")
  │       → gogoanime.ListEpisodes (or fall through to animepahe)
  │       → []Episode JSON
  ├─ user picks episode
  │   └─ scraperApi.getServers(animeId, ep.id)
  │       → returns []Server
  ├─ user picks server (or auto-picks index 0)
  │   └─ scraperApi.getStream(animeId, ep.id, srv.id, category)
  │       → returns { sources: [{url, type: 'hls'|'mp4'}], tracks: [...], intro, outro }
  ├─ hls.js attaches to <video>
  └─ on <video error>: one retry with fresh getStream, then surface error
```

## Error handling

| Failure | UI Response |
|---------|-------------|
| `503 no_providers` from any scraper call | Show `player.englishUnavailable`. Keep tab visible so user can refresh later. |
| `404` from `getEpisodes` (anime not found on any provider) | Show `player.englishEmpty`. |
| Empty `episodes: []` array (anime found, no episodes aired) | Show `player.englishEmpty` with the same "try other sources" hint. |
| `getServers` returns empty | Show "No servers for this episode — try a different one." |
| `getStream` succeeds but the URL fails to play (`<video error>`) | One automatic retry with fresh `getStream`. Persisting failure shows inline error with a "Report" CTA. |
| Network error / catalog or scraper container down | Show generic "Source temporarily unavailable" + retry button. |

All errors that reach the user also get a working `ReportButton` so the diagnostics flow stays intact.

## Phased rollout (within Project A)

**Phase A.1 — Player component + tab (the user-visible win)**
- Build `EnglishPlayer.vue`.
- Wire EN tab in `Anime.vue`, widen type unions, restore the dead-code branches.
- i18n keys added (player + tab keys only — the browse-filter key is added with A.2).
- `EnglishPlayer.vue` uses `hls.js` (HLS), with MP4 direct-`src` fallback.
- No browse-filter integration and no `BrowseSidebar.vue` change in A.1 — that's A.2.
- Done = user can pick EN tab on any anime page, the scraper returns episodes for it, a stream plays.

**Phase A.2 — Browse filter + `has_english` column**
- Add `has_english bool` to `services/catalog/internal/domain/anime.go` (GORM AutoMigrate handles the column add).
- `services/catalog/internal/repo/anime.go` SetHasEnglish setter.
- Background pass on anime detail to populate the column: when a user views an anime page and the EN tab successfully lists episodes, mark `has_english=true` (idempotent, async).
- `BrowseSidebar.vue` filter activates and matches real rows.
- Done = browse filter narrows to anime with confirmed English availability.

**Phase A.3 — Health-aware tab hiding (polish)**
- Wire `scraperApi.getHealth()` into `Anime.vue` mount.
- Hide tab when all providers DOWN.
- Done = tab no longer shows when there's nothing behind it.

A.1 is the minimum lovable product. A.2 and A.3 are optional follow-on phases that can be shipped separately or deferred to Project B.

## Testing strategy

**Unit (vitest):**
- `EnglishPlayer.vue` mounts, calls `scraperApi.getEpisodes` once, renders the episode list.
- Error branches render the correct i18n key.
- `useBrowseFilters` widening doesn't break existing kodik/animelib filtering.

**E2E (Playwright via `bunx playwright test`):**
- New spec `frontend/web/tests/e2e/english-player.spec.ts`:
  1. Log in as `ui_audit_bot`.
  2. Open an anime known to have English coverage (Frieren, MAL ID 52991, or Demon Slayer, MAL ID 38000).
  3. Click EN tab → assert episode list renders.
  4. Click episode 1 → assert `<video>` element gains a `src` and `readyState >= 2` within 15s.
  5. Click `Report` button → assert modal opens with the expected fields.

**Manual smoke (this server IS production):**
- After `make redeploy-web` + `make redeploy-catalog`, hit `https://animeenigma.ru/anime/<slug>` as a logged-out user.
- Confirm EN tab visible, click → episode list loads → video plays.
- Confirm logged-in user sees the same.
- Confirm browse filter (when A.2 is in) narrows results.

**Backend verification (run before frontend wiring, as part of A.1 Phase 0):**
```bash
# Confirm provider failover is healthy
curl -s http://localhost:8000/api/anime/_/scraper/health | jq .
# Pick a known anime UUID (from DB) and confirm episodes return
curl -s "http://localhost:8000/api/anime/<uuid>/scraper/episodes" | jq '.data.episodes | length'
# Servers + stream on episode 1
curl -s "http://localhost:8000/api/anime/<uuid>/scraper/servers?episode=<ep-id>" | jq .
curl -s "http://localhost:8000/api/anime/<uuid>/scraper/stream?episode=<ep-id>&server=<srv-id>&category=sub" | jq .
```

If any of these return 503 with `code: "no_providers"`, A.1 is blocked until the scraper service is recovered — fix that first, not the frontend.

## Risk register

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Scraper providers degrade after release (Anitaku mirror rotation, AnimePahe DDoS-Guard tightening) | Medium | Health-aware tab hiding (A.3) softens this; the existing kill-switch (`SCRAPER_DEGRADED_PROVIDERS`) is the operator's lever. |
| Stream URLs expire mid-playback | Medium | One-shot retry in EnglishPlayer's `<video error>` handler. |
| Kwik / streamhg / earnvids embed extractors break upstream | Medium | Independent embed extractors in `services/scraper/internal/embeds`; each has its own goldens. Failure surfaces as `ErrExtractFailed` → orchestrator falls through. |
| CORS on the proxied stream URL fails in a browser context that didn't exist when the proxy was written | Low | `libs/videoutils/proxy.go` is provider-agnostic. Adding a new allowed host (if needed) is a one-line config change. |
| The `prefer` query param is silently ignored | Low | No UI to set it in A. Future-proofing only. |

## Open questions for plan-writing (deferred, not blockers)

- Should A.2's `has_english` backfill be opportunistic (set on first user visit) or proactive (background job in the scheduler service)? Opportunistic is YAGNI-friendly; proactive populates filters for never-visited anime. Default proposed: **opportunistic**, with a possible scheduler job as Project B.
- Should we expose a "server" picker in A.1, or auto-pick the first server and add the picker only if user complaints arrive? Default proposed: **show the picker** — it's two lines of template, matches the AnimePahe / Gogoanime reality of multiple servers per episode, and saves a Phase A.4.

## Done definition

Project A is complete when, on production:

1. A logged-out user can open an anime page, see an EN tab, click it, see an episode list, click episode 1, and see a video play.
2. The EN tab does not appear when all scraper providers are DOWN per `getHealth`.
3. The browse provider filter offers an "English" option that narrows correctly (A.2 deliverable).
4. The Playwright e2e spec passes against the production-equivalent deployment.
5. `/animeenigma-after-update` has been invoked, the changelog entry is in `frontend/web/public/changelog.json`, all services are healthy, and the commit chain is on `origin/main`.

## Files touched (estimated)

**New:**
- `frontend/web/src/components/player/EnglishPlayer.vue` (~700 lines)
- `frontend/web/tests/e2e/english-player.spec.ts` (~80 lines)

**Modified (A.1):**
- `frontend/web/src/views/Anime.vue` (~30 lines of changes: tab button, v-else-if, type-union widening, dead-comment cleanup; health-check call moves to A.3)
- `frontend/web/src/locales/{en,ru,ja}.json` (~5 keys each × 3 files; browse-filter key deferred to A.2)
- `frontend/web/public/changelog.json` (1 entry)

**Modified (A.2):**
- `services/catalog/internal/domain/anime.go` (add `HasEnglish bool` GORM field)
- `services/catalog/internal/repo/anime.go` (add `SetHasEnglish` to provider-map + method)
- `services/catalog/internal/handler/catalog.go` (add `"english"` to providers filter switch case)
- `services/catalog/internal/service/catalog.go` (opportunistic setter call on successful scraper-episode resolution)
- `frontend/web/src/composables/useBrowseFilters.ts` (~2 lines)
- `frontend/web/src/components/browse/BrowseSidebar.vue` (~6 lines: new providerOptions entry)
- `frontend/web/src/locales/{en,ru,ja}.json` (+1 key for the browse-filter row)

**Modified (A.3):**
- `frontend/web/src/views/Anime.vue` (~15 lines: `scraperApi.getHealth()` mount call, 60s cache, tab gating)

**No changes to:**
- `services/scraper/*` (entire microservice unchanged)
- `services/gateway/*` (routes already public)
- `docker/docker-compose.yml` (scraper container env unchanged)
- `CLAUDE.md` (will get a refresh in Project B once the EN section can describe live providers)
