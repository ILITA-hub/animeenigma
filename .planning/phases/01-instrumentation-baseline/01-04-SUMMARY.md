---
phase: 01-instrumentation-baseline
plan: 04
subsystem: instrumentation/frontend
tags: [vue, composable, axios, anon-id, instrumentation, frontend]
requirements: [M-01]
threats: [T-01-04]
dependency_graph:
  requires:
    - frontend/web/src/types/preference.ts (WatchCombo, ResolvedCombo, ResolveResponse)
    - frontend/web/src/api/client.ts (apiClient axios instance, userApi block)
    - POST /api/preferences/resolve (anon-friendly, plan 01-03)
    - POST /api/preferences/override (anon-friendly, plan 01-03)
  provides:
    - frontend/web/src/utils/anonId.ts (getOrCreateAnonId)
    - frontend/web/src/composables/useOverrideTracker.ts (useOverrideTracker, OverrideDimension, OverrideTrackerOptions, PlayerName)
    - userApi.recordOverride
    - X-Anon-ID always-set on every axios request
    - userApi.resolvePreference path migrated to /preferences/resolve
    - useWatchPreferences accepts anon callers (no auth short-circuit)
  affects:
    - plan 01-05 (frontend integration + e2e) — wires composable into KodikPlayer / AnimeLibPlayer / HiAnimePlayer / ConsumetPlayer + Anime.vue player-switch site
tech-stack:
  added: []
  patterns:
    - "Module-scoped cache + try/catch localStorage idempotent get-or-create (anonId.ts) — analog to useImageProxy.ts session helpers"
    - "Composable factory takes options object with Refs (not a string ID) — multi-ref reactivity required for D-10 window-open detection"
    - "Lock-before-await emit pattern (emittedDimensions.add BEFORE awaited POST) — prevents double-fire under network latency"
    - "Always-set request header in axios interceptor (X-Anon-ID), not gated on JWT absence — backend OptionalAuthMiddleware reads JWT first"
    - "Best-effort instrumentation: empty catch in emit() — never throw to caller, never block UX"
key-files:
  created:
    - frontend/web/src/utils/anonId.ts
    - frontend/web/src/composables/useOverrideTracker.ts
  modified:
    - frontend/web/src/api/client.ts
    - frontend/web/src/composables/useWatchPreferences.ts
key-decisions:
  - "Removed unused useAuthStore/authStore binding from useWatchPreferences after dropping the short-circuit (TS6133 unused-variable error blocked the build); the variable had no other consumer in the file. Rule 3 fix."
  - "ResolvedCombo type imported into client.ts (was not previously imported there) so recordOverride's typed signature can reference it without re-defining a structural duplicate."
  - "anonId.ts caches in module scope BEFORE attempting localStorage.setItem — guarantees a single value within a page lifecycle even if storage is fully unavailable (Pitfall 5: composable + interceptor minting different anon ids in private browsing)."
patterns-established:
  - "Frontend module-scoped UUID cache for anon identity (foundation for Phase 7 D-01)"
  - "Composable factory takes options object with Refs — multi-ref reactivity for window-gated event tracking"
  - "Always-set request header pattern in axios interceptor (X-Anon-ID) for anon-friendly backend endpoints"
  - "Best-effort fire-and-forget POST emission with lock-before-await to prevent double-fire under latency"
requirements-completed: []
metrics:
  duration: ~4 min
  completed: 2026-04-27
  tasks_completed: 3
  tasks_total: 3
  files_created: 2
  files_modified: 2
---

# Phase 1 Plan 04: Frontend Instrumentation Building Blocks — Summary

**Built the four frontend pieces (anonId utility, axios X-Anon-ID interceptor branch + recordOverride endpoint method + path migration, the useOverrideTracker composable enforcing D-07/D-08/D-09/D-10 invariants, and the dropped auth short-circuit in useWatchPreferences) so plan 05 can wire them into KodikPlayer / AnimeLibPlayer / HiAnimePlayer / ConsumetPlayer and the Anime.vue player-switch site without inventing any of the plumbing itself.**

## Performance

- **Duration:** ~4 min
- **Tasks:** 3 / 3 complete
- **Files created:** 2 (`anonId.ts`, `useOverrideTracker.ts`)
- **Files modified:** 2 (`api/client.ts`, `useWatchPreferences.ts`)

## Accomplishments

- `getOrCreateAnonId()` exists in `frontend/web/src/utils/anonId.ts`. Mints a UUIDv4 once, persists to localStorage under key `aenig_anon_id`, returns same value on subsequent calls; falls back to in-memory ephemeral UUID when storage is unavailable.
- `frontend/web/src/api/client.ts` axios request interceptor now always-attaches `X-Anon-ID` (per PATTERNS.md always-set rule, NOT in an else-if branch). `userApi.resolvePreference` migrated to `/preferences/resolve`. New `userApi.recordOverride` method added.
- `frontend/web/src/composables/useOverrideTracker.ts` exports `useOverrideTracker`, `OverrideDimension`, `OverrideTrackerOptions`, `PlayerName`. Enforces 30s window starting on resolvedCombo apply (D-10), 250ms debounce, per-(load_session_id, dimension) lock (D-07), best-effort try/catch emit, onUnmounted cleanup of timers + watcher.
- `frontend/web/src/composables/useWatchPreferences.ts` no longer rejects anon callers (`!authStore.isAuthenticated` clause removed); the empty-`available` guard remains.

## Task Commits

1. **Task 1: anonId utility + axios interceptor + path migration + recordOverride** — `a2cdc84` (feat)
2. **Task 2: useOverrideTracker composable** — `6561d6e` (feat)
3. **Task 3: Drop !authStore.isAuthenticated short-circuit** — `f6b21e8` (feat)

## Verification Status — All GREEN

| Check | Result |
|-------|--------|
| `bunx tsc --noEmit` | exit 0 |
| `bunx eslint src/composables/useOverrideTracker.ts src/composables/useWatchPreferences.ts src/utils/anonId.ts src/api/client.ts` | exit 0 |
| `grep -c "/users/preferences/resolve" frontend/web/src/api/client.ts` | 0 (path migration complete) |
| `grep -c "^export function getOrCreateAnonId" frontend/web/src/utils/anonId.ts` | 1 |
| `grep -c "^export function useOverrideTracker" frontend/web/src/composables/useOverrideTracker.ts` | 1 |
| `grep -c "X-Anon-ID" frontend/web/src/api/client.ts` | 4 |
| `grep -B 2 "X-Anon-ID" frontend/web/src/api/client.ts \| grep -c "else if"` | 0 (always-set, not else-branched) |
| `grep -c "post.*'/preferences/resolve'" frontend/web/src/api/client.ts` | 1 |
| `grep -c "post.*'/preferences/override'" frontend/web/src/api/client.ts` | 1 |
| `grep -c "recordOverride:" frontend/web/src/api/client.ts` | 1 |
| `grep -c "WINDOW_MS = 30_000" frontend/web/src/composables/useOverrideTracker.ts` | 1 |
| `grep -c "DEBOUNCE_MS = 250" frontend/web/src/composables/useOverrideTracker.ts` | 1 |
| `grep -cE "watch\(.*currentEpisode\|watch\(props" frontend/web/src/composables/useOverrideTracker.ts` | 0 (anti-pattern absent) |
| `grep -c "!authStore.isAuthenticated" frontend/web/src/composables/useWatchPreferences.ts` | 0 |

## Files Created/Modified

### Created

- `frontend/web/src/utils/anonId.ts` (33 lines) — single exported `getOrCreateAnonId()` with module-scoped cache + try/catch localStorage idempotent get-or-create. Falls back to ephemeral UUID on storage failure. Storage key is `aenig_anon_id` per CONTEXT D-11.
- `frontend/web/src/composables/useOverrideTracker.ts` (117 lines) — exports `useOverrideTracker`, `OverrideDimension`, `OverrideTrackerOptions`, `PlayerName`. Implements:
  - D-07 lock: `emittedDimensions.add(dimension)` BEFORE the awaited POST, so a click landing during the round-trip is also dropped.
  - D-08 separation: only fires on explicit `recordPickerEvent(...)` calls, never on prop watches — auto-advance call sites will bypass `selectEpisode`-style click handlers (plan 05 audit per RESEARCH §Pattern 2).
  - D-09 fresh session: `loadSessionId = crypto.randomUUID()` at composable factory call (plan 05 keys composable per anime mount).
  - D-10 window-open: `mountedAt` stays `null` until `resolvedCombo.value` first transitions truthy via watcher.
  - 30s WINDOW_MS guard, 250ms DEBOUNCE_MS coalesce, onUnmounted timer-cleanup.
  - Best-effort try/catch in `emit()` with empty handler.

### Modified

- `frontend/web/src/api/client.ts`:
  - Added imports: `getOrCreateAnonId from '@/utils/anonId'`, plus `ResolvedCombo` added to the `@/types/preference` type-import.
  - Request interceptor: replaced the `if (token && config.headers) { ... }` block with an unconditional `config.headers = config.headers || {}` followed by token attach (when present) AND always-set `X-Anon-ID` via `getOrCreateAnonId()`. Inline comment cites PATTERNS.md/RESEARCH.md for the always-set rationale.
  - `userApi.resolvePreference`: path migrated `/users/preferences/resolve` → `/preferences/resolve`.
  - `userApi.recordOverride`: new typed POST method targeting `/preferences/override` with the full payload shape (`anime_id`, `load_session_id`, `dimension`, `original_combo`, `new_combo`, `ms_since_load`, `tier`, `tier_number`, `player`).
- `frontend/web/src/composables/useWatchPreferences.ts`:
  - Dropped the `!authStore.isAuthenticated` short-circuit; only the empty-`available[]` guard remains.
  - Removed the now-unused `useAuthStore`/`authStore` import + binding (Rule 3: TS6133 unused-variable error blocked the build).

## Snippet: New Interceptor Branch (10 lines)

```ts
config.headers = config.headers || {}
if (token) {
  config.headers.Authorization = `Bearer ${token}`
}
// Always-set X-Anon-ID per PATTERNS.md. The backend OptionalAuthMiddleware reads
// JWT first regardless of X-Anon-ID presence, so always-set is harmless on JWT
// routes (handlers ignore unknown headers) and removes a class of subtle bugs
// where X-Anon-ID would be missing if the user had a JWT but the JWT was rejected
// downstream.
config.headers['X-Anon-ID'] = getOrCreateAnonId()
return config
```

## Snippet: Exported API of `useOverrideTracker`

```ts
export type OverrideDimension = 'language' | 'player' | 'team' | 'episode'
export type PlayerName = 'kodik' | 'animelib' | 'hianime' | 'consumet'

export interface OverrideTrackerOptions {
  animeId: string
  player: PlayerName
  resolvedCombo: Ref<ResolvedCombo | null>
  currentEpisode: Ref<number>
}

export function useOverrideTracker(opts: OverrideTrackerOptions): {
  recordPickerEvent: (
    dimension: OverrideDimension,
    newCombo: Partial<WatchCombo> & { episode?: number },
  ) => void
  loadSessionId: string
}
```

## Notes for Plan 05 (Frontend Integration + e2e)

- Import path: `import { useOverrideTracker } from '@/composables/useOverrideTracker'`.
- Instantiate **per player** for the `episode | team | language` dimensions (one composable instance inside each of the four player components):
  ```ts
  const tracker = useOverrideTracker({
    animeId: props.animeId,
    player: 'kodik', // or 'animelib' | 'hianime' | 'consumet'
    resolvedCombo: toRef(props, 'preferredCombo'),
    currentEpisode: selectedEpisode,
  })
  ```
- Instantiate **once at Anime.vue level** for the `player` dimension only (the player-switch site).
- Picker click handlers (`selectEpisode`, `selectTranslation`, `selectServer`, language toggles) MUST call `tracker.recordPickerEvent('episode'|'team'|'language', newCombo)` BEFORE doing the existing work. The event is fire-and-forget; the click handler returns immediately.
- **Critical anti-pattern (research §Anti-Patterns):** auto-advance call sites must NOT funnel through `selectEpisode`. Audit each player and refactor auto-advance to call a sibling `_advanceEpisode(nextEp)` that bypasses the wrapper. HiAnime: `tryNextServer()` (~line 1071) and end-of-episode handler (~line 1264) are confirmed offenders per the pattern map.
- Anon callers benefit transparently: the axios interceptor sets `X-Anon-ID` on every request; the composable's POST goes through the same axios instance, and the backend `OverrideHandler` accepts it.
- The five `loadSessionId`s on a single anime page (4 player instances + 1 Anime.vue instance) is fine — Grafana joins on `(tier, anon, player)`, not `load_session_id`. The session id only lives in the Loki line for forensic queries.

## Decisions Made

- **Removed `useAuthStore` import from `useWatchPreferences.ts`.** With the short-circuit dropped, `authStore` had no consumer, producing TS6133 (unused variable). The plan said "leave it IF used elsewhere"; it was not. Removed to keep the build clean (Rule 3).
- **`ResolvedCombo` added to client.ts type import.** `recordOverride` parameters reference `ResolvedCombo` for the typed `original_combo` field; the type was already exported from `@/types/preference` but not previously imported into `client.ts`.
- **Module-scoped `cached` variable in `anonId.ts` set BEFORE the localStorage write succeeds.** Guarantees a single value within the page lifecycle even when storage is fully unavailable (private browsing). Prevents the composable + interceptor from minting different anon ids in the same session (Pitfall 5).

## Deviations from Plan

**1. [Rule 3 - Blocking issue] Removed `useAuthStore` import + `authStore` binding from `useWatchPreferences.ts`**

- **Found during:** Task 3
- **Issue:** After dropping the `!authStore.isAuthenticated` short-circuit, `authStore` had zero consumers in the file. `bunx tsc --noEmit` reported `error TS6133: 'authStore' is declared but its value is never read.` blocking the build.
- **Fix:** Removed the now-unused `import { useAuthStore } from '@/stores/auth'` and the `const authStore = useAuthStore()` binding.
- **Files modified:** `frontend/web/src/composables/useWatchPreferences.ts`
- **Commit:** `f6b21e8`
- **Plan note:** Plan instruction read "leave the `useAuthStore()` reference and `authStore` variable IF they're used elsewhere in the file (e.g., for auth-state-change cache invalidation in Phase 7)." They were not used; removed accordingly. Phase 7 can re-introduce the import when it adds auth-state-change cache invalidation.

Otherwise: plan executed exactly as written. No checkpoints, no architectural deviations, no auth gates.

## Threat Model Coverage

| Threat ID | Disposition | Mitigation Site | Status |
|-----------|-------------|-----------------|--------|
| T-01-04 (Tampering — client-controlled X-Anon-ID value) | accept | Out of scope per CONTEXT D-14 — no auth-grade trust on anon_id; UUIDv4 collision is negligible; forgery only degrades the `anon=true` denominator slightly (not a security issue). Documented in PROJECT.md alongside the baseline snapshot. | Accepted |

No new STRIDE threats introduced beyond T-01-04, which we accept.

## Issues Encountered

- **First `bunx tsc --noEmit` invocation pulled missing types** (`@types/node`, `vite/client`). Resolved by running `bun install` to materialize the dev dependencies in the worktree. Subsequent invocations clean. No actual code-side problem.

## Next Plan Readiness (Plan 05 — Frontend Integration + e2e)

Building blocks are in place:
- `getOrCreateAnonId()` ready for any future caller (no consumers required for this plan; the axios interceptor is the first).
- `useOverrideTracker` ready to be imported into all 4 players + Anime.vue.
- `userApi.recordOverride` ready to receive the composable's POSTs.
- `userApi.resolvePreference` migrated to `/preferences/resolve`; the gateway proxy + player-service `OptionalAuthMiddleware` accept the call.
- `useWatchPreferences` accepts anon callers, populating the `combo_resolve_total` denominator across the full user base.

Plan 05 can compose these against the four player components without inventing any of the plumbing.

## Self-Check: PASSED

All 2 created files exist on disk:
- FOUND: `frontend/web/src/utils/anonId.ts`
- FOUND: `frontend/web/src/composables/useOverrideTracker.ts`

All 2 modified files reflect the changes:
- FOUND: `frontend/web/src/api/client.ts` (X-Anon-ID always-set, /preferences/resolve, recordOverride)
- FOUND: `frontend/web/src/composables/useWatchPreferences.ts` (no short-circuit, no useAuthStore import)

All 3 task commits exist in git:
- FOUND: `a2cdc84` — `feat(01-04): add anonId utility and wire X-Anon-ID interceptor + recordOverride`
- FOUND: `6561d6e` — `feat(01-04): add useOverrideTracker composable for combo-override detection`
- FOUND: `f6b21e8` — `feat(01-04): drop auth short-circuit in useWatchPreferences for anon support`

Verification:
- `bunx tsc --noEmit` — exit 0
- `bunx eslint src/composables/useOverrideTracker.ts src/composables/useWatchPreferences.ts src/utils/anonId.ts src/api/client.ts` — exit 0

---
*Phase: 01-instrumentation-baseline*
*Completed: 2026-04-27*
