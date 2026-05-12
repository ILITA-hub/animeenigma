---
phase: 16-animepahe-and-new-englishplayer
plan: 04
subsystem: ui
tags: [vue, typescript, i18n, frontend, scraper-client, diagnostics, watch-preferences]

# Dependency graph
requires:
  - phase: 15-foundation
    provides: "/api/anime/{id}/scraper/* HTTP contract on catalog (Phase 15 catalog→scraper wiring)"
provides:
  - "scraperApi client (getEpisodes / getServers / getStream / getHealth) targeting /api/anime/{id}/scraper/*"
  - "12 new player.* locale keys in en/ru/ja for the upcoming EnglishPlayer source dropdown + report fields"
  - "PlayerContext + DiagnosticReport extended with scraperProvider/triedChain (camelCase context, snake_case report) so ReportButton can attach orchestrator state"
  - "ReportButton.vue optional props scraperProvider + triedChain with v-if guards (legacy players unaffected)"
  - "useWatchPreferences exposes preferredScraperProvider + setPreferredScraperProvider with `pref:scraper:${animeId}` localStorage persistence (24h TTL)"
affects: [16-06-EnglishPlayer, 17-Observability, 20-Cutover]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Separate localStorage cache key per concern: `pref:` (combo) + `pref:scraper:` (provider) so they can be invalidated independently"
    - "Optional Vue props with withDefaults for non-breaking extension of shared components consumed by 4 existing players"
    - "snake_case JSON keys in DiagnosticReport interface to match the Go report.go struct tags (consistent with existing report contract)"

key-files:
  created: []
  modified:
    - "frontend/web/src/api/client.ts (added scraperApi block; hiAnimeApi + consumetApi untouched)"
    - "frontend/web/src/locales/en.json (10 new keys, englishNotAvailable nested)"
    - "frontend/web/src/locales/ru.json (10 new keys, englishNotAvailable nested)"
    - "frontend/web/src/locales/ja.json (10 new keys, englishNotAvailable nested)"
    - "frontend/web/src/utils/diagnostics.ts (PlayerContext + DiagnosticReport extended)"
    - "frontend/web/src/components/player/ReportButton.vue (new optional props + 2 new conditional info rows)"
    - "frontend/web/src/composables/useWatchPreferences.ts (preferredScraperProvider ref + setter + 24h localStorage cache)"

key-decisions:
  - "Phase 16 keeps preferredScraperProvider local-only (matches existing preferred_en_provider localStorage pattern in Anime.vue); server-side promotion deferred to Phase 18+"
  - "englishNotAvailable rendered as nested object {heading,body} per locale convention rather than two flat dotted keys"
  - "scraperApi.getHealth uses placeholder animeId `_` because catalog routes health under /anime/{id}/scraper/health for routing reasons but the handler ignores animeId"

patterns-established:
  - "Pattern: extend shared player components via optional props + withDefaults so legacy consumers continue to type-check and v-if guards keep their UI unchanged"
  - "Pattern: per-anime preference caches keyed by `pref:<scope>:${animeId}` with a uniform 24h TTL across the composable"

requirements-completed:
  - SCRAPER-UI-03
  - SCRAPER-NF-05

# Metrics
duration: ~18 min net authoring (spread across 2026-05-11 + 2026-05-12 sessions; original session paused on usage exhaustion)
completed: 2026-05-12
---

# Phase 16 Plan 04: Frontend infrastructure for new EnglishPlayer Summary

**scraperApi client + 12 new locale keys + ReportButton/diagnostics scraperProvider+triedChain wiring + useWatchPreferences per-anime scraper preference — green floor for Plan 16-06 (EnglishPlayer.vue) with hiAnimeApi/consumetApi untouched.**

## Performance

- **Duration:** ~18 min net authoring spread across two sessions (paused 2026-05-11 ~11:35 +0200 on Anthropic usage exhaustion; resumed 2026-05-12)
- **Started:** 2026-05-11T09:30:00Z (Task 1 commit time)
- **Completed:** 2026-05-12T04:14:01Z (Task 3 commit time)
- **Tasks:** 3 / 3
- **Files modified:** 7

## Accomplishments

- Frontend now has a typed `scraperApi` block ready to consume the Phase 15 catalog scraper routes; the four legacy `hiAnimeApi`/`consumetApi` methods are left untouched (SCRAPER-UI-03 lock holds until Phase 20 cutover).
- All 12 new player UI strings are translated into English, Russian, and Japanese with consistent nesting for `englishNotAvailable.{heading,body}`.
- `ReportButton.vue` now accepts `scraperProvider` + `triedChain` props and forwards them into the diagnostics payload; legacy callers (HiAnimePlayer.vue, ConsumetPlayer.vue, AnimeLibPlayer.vue, KodikPlayer.vue) compile and render unchanged because the props default to `null`/`[]` and the new info rows are `v-if` gated.
- `useWatchPreferences` exposes a per-anime scraper provider preference with 24h localStorage persistence, ready for the source dropdown that 16-06 will mount.

## Task Commits

Each task was committed atomically on the worktree branch (`worktree-agent-a67373dce644a2895`):

1. **Task 1: scraperApi block + new locale keys (all three locales)** — `c186cc8` (feat)
2. **Task 2: Extend diagnostics.ts + ReportButton.vue with scraperProvider + triedChain** — `afe1fe0` (feat)
3. **Task 3: Add preferredScraperProvider persistence to useWatchPreferences** — `00b55f3` (feat)

_Note: Tasks 1 + 2 were committed in the original worktree session (paused on usage exhaustion); Task 3 was completed in the resume session after rebasing the new worktree branch onto main to inherit the prior task commits. The plan metadata commit (this SUMMARY) follows._

## Files Created/Modified

- `frontend/web/src/api/client.ts` — Added `scraperApi` export with `getEpisodes`, `getServers`, `getStream`, `getHealth`. Existing `hiAnimeApi` + `consumetApi` blocks intentionally left in place (Phase 20 owns deletion).
- `frontend/web/src/locales/en.json` — 10 new keys under `player.*` (with `englishNotAvailable` nested as `{heading,body}` so the count of "tabEnglish-pattern" greps reports 10 unique keys = 12 strings).
- `frontend/web/src/locales/ru.json` — Russian translations of the same 10 keys.
- `frontend/web/src/locales/ja.json` — Japanese translations of the same 10 keys.
- `frontend/web/src/utils/diagnostics.ts` — `PlayerContext` gains `scraperProvider?: string | null` and `triedChain?: string[]`; `DiagnosticReport` gains `scraper_provider` (string|null) + `tried_chain` (string[]) snake_case fields; `collectDiagnostics` maps them with `?? null` / `?? []` defaults.
- `frontend/web/src/components/player/ReportButton.vue` — `Props` gains two optional fields, `withDefaults` provides null/[] defaults, two new `v-if` rows render only when `scraperProvider` is non-null or `triedChain` is non-empty; `submitReport()` threads both into `collectDiagnostics`.
- `frontend/web/src/composables/useWatchPreferences.ts` — Added `preferredScraperProvider: Ref<string | null>` + `setPreferredScraperProvider(value)`. Reads `pref:scraper:${animeId}` on instantiation (24h TTL via shared `CACHE_TTL` constant), writes on every setter call with quota errors swallowed.

## Locale Key Audit

| Key | en.json | ru.json | ja.json |
|---|---|---|---|
| `player.tabEnglish` | ✓ | ✓ | ✓ |
| `player.tabDebugSuffix` | ✓ | ✓ | ✓ |
| `player.source` | ✓ | ✓ | ✓ |
| `player.sourceSingleTooltip` | ✓ | ✓ | ✓ |
| `player.sourceMultiTooltip` | ✓ | ✓ | ✓ |
| `player.sourceUnhealthy` | ✓ | ✓ | ✓ |
| `player.englishNotAvailable.heading` | ✓ | ✓ | ✓ |
| `player.englishNotAvailable.body` | ✓ | ✓ | ✓ |
| `player.sourceSwitchFailed` | ✓ | ✓ | ✓ |
| `player.sourceUnavailable` | ✓ | ✓ | ✓ |
| `player.reportProvider` | ✓ | ✓ | ✓ |
| `player.reportTried` | ✓ | ✓ | ✓ |

## scraperApi Method Signatures

```typescript
export const scraperApi = {
  getEpisodes: (animeId: string, prefer?: string) => Promise<AxiosResponse>
  getServers:  (animeId: string, episodeId: string, prefer?: string) => Promise<AxiosResponse>
  getStream:   (animeId: string, episodeId: string, serverId: string,
                category: 'sub' | 'dub', prefer?: string) => Promise<AxiosResponse>
  getHealth:   () => Promise<AxiosResponse>   // routes to /anime/_/scraper/health
}
```

When `prefer` is omitted, the orchestrator picks the default (currently AnimePahe — Plan 16-03 owns the provider; Phase 18 adds 9anime).

## ReportButton Prop Diff

```diff
 interface Props {
   playerType: string
   animeId: string
   animeName: string
   episodeNumber?: number | null
   serverName?: string | null
   streamUrl?: string | null
   errorMessage?: string | null
   accentColor?: string
+  scraperProvider?: string | null   // Phase 16
+  triedChain?: string[]             // Phase 16
 }

 const props = withDefaults(defineProps<Props>(), {
   accentColor: '#a855f7',
+  scraperProvider: null,
+  triedChain: () => [],
 })
```

Template gains two `v-if` rows (after the existing `serverName` row):

- `<div v-if="scraperProvider">` renders `player.reportProvider: {{ scraperProvider }}`
- `<div v-if="triedChain && triedChain.length > 0">` renders `player.reportTried: {{ triedChain.join(', ') }}`

Legacy consumers (HiAnimePlayer.vue, ConsumetPlayer.vue, AnimeLibPlayer.vue, KodikPlayer.vue) do not pass the new props — defaults keep the UI identical.

## diagnostics.ts Interface Diff

```diff
 export interface PlayerContext {
   playerType: string
   animeId: string
   animeName: string
   episodeNumber?: number | null
   serverName?: string | null
   streamUrl?: string | null
   errorMessage?: string | null
+  scraperProvider?: string | null  // Phase 16 SCRAPER-NF-05
+  triedChain?: string[]            // Phase 16 SCRAPER-NF-05
 }

 export interface DiagnosticReport {
   /* …existing fields… */
   error_message: string | null
+  scraper_provider: string | null  // snake_case for Go report.go struct tag
+  tried_chain: string[]            // snake_case for Go report.go struct tag
   /* …console_logs, network_logs, page_html, description… */
 }
```

`collectDiagnostics` returns the new fields with `ctx.scraperProvider ?? null` / `ctx.triedChain ?? []` so callers that don't pass them get safe defaults.

## useWatchPreferences Exports Diff

```diff
 return {
   resolvedCombo,
   isLoading,
   resolve,
+  preferredScraperProvider,        // Phase 16 SCRAPER-NF-02
+  setPreferredScraperProvider,     // Phase 16 SCRAPER-NF-02
 }
```

- Cache key: `pref:scraper:${animeId}` (distinct from existing `pref:${animeId}` so combo and scraper-provider caches invalidate independently).
- TTL: shared 24h `CACHE_TTL` constant (consistent with the combo cache).
- Storage shape: `{ value: string | null, ts: number }` — `ts` is the absolute write timestamp so reads can compare against `Date.now()` directly.
- Quota / parse failures are swallowed silently (matches existing composable patterns).

## Decisions Made

- **Local-only scraper preference for Phase 16.** SCRAPER-NF-02 / CONTEXT.md does not require server-side persistence for the per-anime provider preference, and there's already a precedent of a flat `preferred_en_provider` localStorage key in `Anime.vue`. Server-side promotion is deferred to Phase 18+ if cross-device sync becomes a need.
- **`englishNotAvailable` as a nested locale object** rather than two flat dotted keys. The existing schema uses nested objects for player.error.* and similar sub-namespaces; following that convention keeps `$t('player.englishNotAvailable.heading')` natural.
- **`getHealth()` uses a placeholder `_` animeId** because the catalog templates the health route under `/anime/{id}/scraper/health` for routing uniformity, but the handler ignores animeId on that path. The plan explicitly authorised this shape.

## Deviations from Plan

None — plan executed exactly as written. The only operational variance is the multi-session resume:

- Original session paused mid-flight on Anthropic usage exhaustion after committing Task 1 (`c186cc8`) and Task 2 (`afe1fe0`); the previous executor's worktree was left dirty on Task 3.
- Resume session rebased a fresh worktree branch (`worktree-agent-a67373dce644a2895`) onto main to inherit the previously-committed tasks, then implemented and committed Task 3 (`00b55f3`).

No deviation rules fired. No auto-fixes were necessary. No authentication gates encountered.

## Issues Encountered

- **Pre-existing tsconfig deprecation warning** (`baseUrl` deprecation) and missing `@types/node` / `vite/client` after a clean `node_modules` in the new worktree. Resolved by running `bun install` once — these are upstream config concerns unrelated to this plan. After install, `bunx tsc --noEmit` exits clean.
- **Worktree branch rebase needed at resume.** The new worktree was created from an older main, so the first action of the resume session was `git rebase main` to pick up `c186cc8` + `afe1fe0` + the salvage / TDD-RED commits for siblings (16-01, 16-02). The rebase was clean — no conflicts.

## User Setup Required

None — no external service configuration, no environment variables, no deployment. This is pure frontend infrastructure that becomes user-visible only when Plan 16-06 mounts `EnglishPlayer.vue`.

## Verification Performed

- `cd frontend/web && bunx tsc --noEmit` → clean (after `bun install` to fetch missing dep types).
- `cd frontend/web && bunx eslint src/api/client.ts src/composables/useWatchPreferences.ts src/utils/diagnostics.ts src/components/player/ReportButton.vue` → clean.
- Locale key audit: every one of the 12 new keys is present in en/ru/ja (`grep -c` per key, all = 1 across all three files).
- `grep -c "export const hiAnimeApi\|export const consumetApi" frontend/web/src/api/client.ts` → 2 (lock holds).
- `grep -q "preferredScraperProvider" / "setPreferredScraperProvider" / "pref:scraper:"` → all present in useWatchPreferences.ts.

## Next Phase Readiness

- Plan 16-06 (EnglishPlayer.vue + Anime.vue tab integration) has its frontend floor: `scraperApi` client, full locale coverage, `ReportButton` props, and the per-anime provider preference composable. 16-06 can mount the new player without needing further infrastructure work.
- Plan 16-03 (AnimePahe Provider backend) is unblocked and runs in parallel — no shared files with this plan.
- Plan 16-05 (boot wiring) depends on 16-03 only; not affected by this plan.
- No blockers introduced. `hiAnimeApi` + `consumetApi` continue to function until Phase 20 cutover.

## Self-Check: PASSED

Verified at SUMMARY-write time:

- `frontend/web/src/api/client.ts` — FOUND (contains `export const scraperApi`)
- `frontend/web/src/locales/en.json` — FOUND (contains `tabEnglish`)
- `frontend/web/src/locales/ru.json` — FOUND (contains `tabEnglish`)
- `frontend/web/src/locales/ja.json` — FOUND (contains `tabEnglish`)
- `frontend/web/src/utils/diagnostics.ts` — FOUND (contains `scraper_provider`)
- `frontend/web/src/components/player/ReportButton.vue` — FOUND (contains `scraperProvider`)
- `frontend/web/src/composables/useWatchPreferences.ts` — FOUND (contains `preferredScraperProvider`)
- Commits: `c186cc8`, `afe1fe0`, `00b55f3` — all present in `git log`.

---
*Phase: 16-animepahe-and-new-englishplayer*
*Completed: 2026-05-12*
