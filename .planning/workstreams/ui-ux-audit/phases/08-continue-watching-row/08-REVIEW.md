---
phase: 08-continue-watching-row
reviewed: 2026-05-13T00:00:00Z
depth: standard
files_reviewed: 11
files_reviewed_list:
  - services/player/internal/repo/progress.go
  - services/player/internal/service/progress.go
  - services/player/internal/handler/progress.go
  - services/player/internal/transport/router.go
  - services/player/internal/domain/watch.go
  - services/player/internal/repo/progress_test.go
  - frontend/web/src/api/client.ts
  - frontend/web/src/composables/useContinueWatching.ts
  - frontend/web/src/components/home/ContinueWatchingRow.vue
  - frontend/web/src/views/Home.vue
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/locales/ja.json
findings:
  critical: 1
  warning: 3
  info: 3
  total: 7
status: findings_found
---

# Phase 8: Code Review Report

**Reviewed:** 2026-05-13
**Depth:** standard
**Files Reviewed:** 11 (Go + Vue + i18n)
**Status:** findings_found

## Summary

The Continue-Watching row is mostly well-implemented. SQL is parameterized (no injection risk), JWT auth is enforced server-side via `authz.ClaimsFromContext` with no user_id leakage from body/query (no IDOR), the composable correctly gates on `auth.token` to avoid 401s for anonymous users, and the limit input is defended in depth (handler rejects non-numeric/negative, repo clamps `[1, 20]`).

One real defect blocks plan compliance: **the `?episode={N}` query param in the Continue-Watching deep link is never read by Anime.vue**. The deep link "happens to work" only because server-side resume state usually matches the requested episode — but the plan explicitly contracts that the click should resume on that exact episode, and the codebase has no code path that does so.

Secondary findings are defense-in-depth nits (handler-level limit clamp, dialect-specific SQL literal, silent INNER JOIN drop on deleted anime).

## Critical Issues

### CR-01: `?episode={N}` query param has no effect — Continue-Watching deep link is functionally broken

**File:** `frontend/web/src/views/Anime.vue:943-952` (and absence of any `route.query.episode` read across the entire `frontend/web/src/` tree)

**Issue:** The Continue-Watching component routes to `/anime/${item.anime.id}?episode=${item.episode_number}` (`ContinueWatchingRow.vue:17`). Per the phase plan: *"Clicking a card routes to /anime/{id}?episode={N} so the player resumes on the correct episode."*

But `Anime.vue` never reads `route.query.episode`. The `resumeStartEpisode` computed (lines 943-952) only considers:
1. `resumeOverrideEpisode` (set by the in-page "Rewatch from ep. 1" button)
2. The server-side resume state machine's `startEpisode`
3. localStorage `lastEpisode` fallback

A grep across the entire `frontend/web/src/` tree returns exactly one `route.query` consumer — `$route.query.legacy === '1'` at `Anime.vue:378`. Nothing reads `episode`.

In practice this often appears to work because the server-side resume state usually equals the most-recent in-progress episode (the same data the Continue-Watching SQL surfaces). But the two diverge whenever:
- The user has just completed episode N (server resume state -> N+1) but is being shown an older "in-progress" row for episode N-1.
- Multiple sessions have advanced the server state past the row's `episode_number`.
- The `resume.kind` is `next-up` and the user clicks a card whose episode is one behind.

In those cases the link silently lands the user on the wrong episode with zero feedback. The contract in the phase plan is violated.

**Fix:**

Wire `route.query.episode` into `resumeStartEpisode`. Drop it into the same precedence chain, ranked above the state machine because it is a user-driven explicit selection:

```ts
// In Anime.vue, near line 943
const queryEpisode = computed<number | undefined>(() => {
  const v = route.query.episode
  const s = Array.isArray(v) ? v[0] : v
  if (typeof s !== 'string' || s === '') return undefined
  const n = parseInt(s, 10)
  return Number.isFinite(n) && n > 0 ? n : undefined
})

const resumeStartEpisode = computed<number | undefined>(() => {
  if (resumeOverrideEpisode.value && resumeOverrideEpisode.value > 0) {
    return resumeOverrideEpisode.value
  }
  // NEW: explicit deep-link wins over state-machine resume.
  if (queryEpisode.value !== undefined) {
    return queryEpisode.value
  }
  if (resumeAuth.value && resume.loaded.value) {
    const s = resume.startEpisode.value
    return s > 0 ? s : (lastEpisode.value ?? 1)
  }
  return lastEpisode.value
})
```

Add a follow-up `watch(() => route.query.episode, ...)` so navigating between two cards on the same anime page re-mounts the player on the new episode (otherwise a same-route navigation will not re-trigger the computed if the rest of the dependency graph hasn't changed). Or call `activatePlayer()` from a watcher when `queryEpisode` flips.

Then either (a) clear the `?episode=` query param after consumption with `router.replace({ query: { ...route.query, episode: undefined } })` to avoid the page bookmark/share leaking the stale episode, or (b) accept that re-share is OK because the user is already authenticated and the link is per-user-meaningful.

## Warnings

### WR-01: Handler does not bound `limit` upper — relies on repo clamping for defense

**File:** `services/player/internal/handler/progress.go:127-153`

**Issue:** The handler accepts any positive integer for `?limit=` and only the repo clamps to `[1, 20]`:

```go
limit := 10
if v := r.URL.Query().Get("limit"); v != "" {
    if n, err := strconv.Atoi(v); err == nil && n > 0 {
        limit = n   // <- no upper bound
    }
}
```

If a future refactor inlines repo logic or skips clamping, the unbounded value flows straight to `LIMIT ?` and can be used to extract larger pages. Today it's safe because the repo clamps, but defense-in-depth at the API boundary is cheap and the convention elsewhere in this codebase (see `PaginationParams.Validate()` in `domain/watch.go:191-213`) is to clamp at the handler/DTO layer, not the repo.

**Fix:**

```go
const (
    defaultLimit = 10
    maxLimit     = 20
)
limit := defaultLimit
if v := r.URL.Query().Get("limit"); v != "" {
    if n, err := strconv.Atoi(v); err == nil && n > 0 {
        if n > maxLimit {
            n = maxLimit
        }
        limit = n
    }
}
```

Then the repo's clamp becomes a redundant safety net rather than the only line of defense.

### WR-02: INNER JOIN silently drops in-progress rows whose anime row is missing or soft-deleted

**File:** `services/player/internal/repo/progress.go:204-207`

**Issue:** The query uses `JOIN animes a ON a.id = r.anime_id ... WHERE r.rn = 1 AND a.deleted_at IS NULL`. Two consequences:

1. If `animes.id = r.anime_id` returns no row (anime hard-deleted or never inserted — possible during catalog churn or stale watch_progress rows), the entire entry is silently dropped. The user sees a shorter Continue-Watching row than expected with no indication an entry was filtered.
2. The `deleted_at IS NULL` filter inside the JOIN further silently drops soft-deleted anime. Same UX issue.

The user-facing impact is small (a missing card the user can't act on anyway), but the silent filter masks data-integrity issues — e.g. a real bug elsewhere could be losing watch_progress entries and this query would hide the symptom.

**Fix:** Either:
- Log when `ranked` row count != final output count (count the dropped rows for observability), or
- LEFT JOIN with a `WHERE a.id IS NOT NULL` filter and a log line when the COALESCE'd anime is missing.

Lowest-effort acceptable fix: leave the join shape but add a debug log in the service layer comparing the count requested vs returned, so the operator can spot drift.

### WR-03: Test fixture uses SQLite INTEGER for `completed`; production uses Postgres BOOLEAN — `WHERE completed = false` is dialect-coerced, not equivalent

**File:** `services/player/internal/repo/progress_test.go:59-77` (SQLite schema) and `services/player/internal/repo/progress.go:190` (`WHERE wp.completed = false`)

**Issue:** The test creates `completed INTEGER DEFAULT 0` (SQLite has no native boolean). The production migration creates a Postgres `BOOLEAN`. The query uses the literal `false`, which is a Postgres BOOLEAN literal; in SQLite the value `false` is treated as a column reference (or, in newer SQLite, as the literal 0) — this happens to work in tests because the integer column compares equal to 0.

A future query that uses `IS FALSE` or `NOT completed` would not exercise identically across both environments. Two-database test fidelity is a known fragility class; flagging so future SQL changes (e.g. partial-index usage, `IS NOT TRUE` semantics for NULL handling) get an integration-style Postgres test rather than relying on the SQLite scaffold.

**Fix:** Either add a `//go:build integration`-tagged Postgres testcontainer test for `ListContinueWatching` (preferred — caches and Postgres testcontainers are already used elsewhere per `CLAUDE.md`'s testing section), or document the SQLite fidelity gap as a known limitation. No code change required for v1.

## Info

### IN-01: `transition-all` on progress bar is overly broad

**File:** `frontend/web/src/components/home/ContinueWatchingRow.vue:34`

**Issue:** `class="h-full bg-cyan-400 transition-all"` — `transition-all` applies to every animatable property, which is wasteful when only `width` changes. Minor (modern engines optimize this).

**Fix:** Use `transition-[width]` (or `transition-{width,opacity}`) for explicit property selection.

### IN-02: Progress bar shows 0% when duration is unknown — visually indistinguishable from "just started"

**File:** `frontend/web/src/components/home/ContinueWatchingRow.vue:65-70`

**Issue:** `progressPct` returns `0` when `duration <= 0`. A freshly-opened episode with no `duration` heartbeat yet (which is the common case for "user opened the player but a heartbeat hasn't fired") renders the same as "user is at 0:00 of a known-duration episode." Both display a width-0 bar; the user has no way to tell which case they're in.

**Fix:** When `duration` is 0 but `progress > 0`, render a "indeterminate" treatment (e.g. a thin dashed stripe or hide the bar entirely):

```ts
function progressState(item): 'unknown' | 'value' {
  return (!item.duration || item.duration <= 0) ? 'unknown' : 'value'
}
```

And in the template render `v-if="progressState(item) === 'value'"` for the cyan bar, otherwise show nothing or a muted placeholder.

### IN-03: Composable's auth watcher comment misstates the trigger condition

**File:** `frontend/web/src/composables/useContinueWatching.ts:62-72`

**Issue:** The comment says *"Re-fetch on auth transitions (login from anonymous, logout to anonymous)."* The watch source is `auth.token`, which is initialized to `localStorage.getItem('token')` — i.e. `string | null`, never `undefined`. The `oldToken !== undefined` gate is copied from `useRecs.ts` where the same comment is also slightly misleading. The gate's actual purpose is to defend against Vue's `immediate: true` semantics (which this watcher does NOT enable, so the gate is in fact a no-op).

**Fix:** Either remove the redundant gate, or change the comment to acknowledge it as defensive code matching the existing convention rather than a real-trigger filter. Suggested:

```ts
// Defensive: mirrors useRecs.ts pattern. Watcher does not use immediate:true,
// so oldToken is always a real string|null, not undefined. The check is a no-op
// in current Vue semantics but kept for consistency with the codebase convention.
```

---

_Reviewed: 2026-05-13_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
