# Episode-Selection Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the four overlapping "which episode opens" computations into one source of truth, kill the mount-race that opened the latest aired episode instead of ep 1, and replace the 5-state resume machine + duplicate CTA logic with pure functions.

**Architecture:** One pure module (`watchState.ts`) owns every decision (`resolveStartEpisode`, `resolveResumeState`). One composable (`useWatchState.ts`) resolves `lastWatched` for authed (server) and anon (localStorage) and exposes `startEpisode`/`banner`/`cta`. `AePlayer` consumes `initialEpisode` **reactively** (default 1, never `episodesAired`). `watchCta.ts` + `useResumeStateMachine.ts` are deleted.

**Tech Stack:** Vue 3 (`<script setup>`, Composition API), TypeScript, Vitest, vue-i18n, Tailwind v4 (Neon-Tokyo DS).

**Design spec:** `docs/superpowers/specs/2026-06-30-episode-selection-simplification-design.md`

## Global Constraints

- **Worktree only.** All edits happen in `/data/ae-episode-selection` (branch `feat/episode-selection-simplify`). NEVER edit `/data/animeenigma` (the base tree).
- **Frontend tooling:** `bun` / `bunx` ONLY — never `npm`/`npx`. Run from `frontend/web`.
- **DS-lint is a build gate:** `frontend/web/scripts/design-system-lint.sh` must report `ERRORS=0`. Bind semantic tokens; no off-palette Tailwind colors; only `font-medium`/`font-semibold`.
- **i18n parity:** every key must exist in `en.json`, `ru.json`, AND `ja.json`. The parity test fails on mismatch.
- **vue-tsc false-pass trap:** a passing `bunx tsc --noEmit` is NOT sufficient — run the REAL `bun run build` before claiming done. Named TYPE imports from `.vue`/modules use `import type { X }` (a value import of a type triggers TS2614).
- **Commits** carry all three co-authors verbatim, with author name "Claude Code" (no model/version):
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **No time-effort units** anywhere (no days/hours/sprints). Score with UXΔ/CDI/MVQ if a metric is needed.

## Decisions (locked)

- **D-1 — Anonymous `lastWatched` adapter.** Anon localStorage (`watch_progress:{id}` = `{ep: {time, maxTime, updatedAt}}`) has NO `completed` flag, so `parseLastWatchedEpisode` returns the *last-touched* (in-progress) episode. To make anon and authed produce the SAME opened episode and a consistent CTA, anon `lastWatched` (= highest completed) is derived as `max(0, parseLastWatchedEpisode(ls) - 1)`. This preserves the old anon open-episode while fixing the old anon button (which said "continue ep N+1" while the player opened ep N).
- **D-2 — Keep `loadedEpisodes`** as a pure input to `resolveResumeState` (`aired = max(episodesAired, loadedEpisodes)`), so a freshly-uploaded episode is never mislabeled "not available". We delete the state machine + 60s timer, not this signal.
- **D-3 — Banner episode number** for `next-unavailable` is `last + 1` (the episode the viewer is waiting for). For a caught-up viewer this equals `episodesAired + 1` (the old formula).

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `frontend/web/src/composables/watchState.ts` | Pure: `resolveStartEpisode`, `resolveResumeState`, types | **Create** |
| `frontend/web/src/composables/__tests__/watchState.spec.ts` | Pins the pure decisions (CTA + start + banner) | **Create** |
| `frontend/web/src/composables/aePlayer/episodeSelection.ts` | Add `shouldReselectEpisode` (race guard) | **Modify** |
| `frontend/web/src/composables/aePlayer/__tests__/episodeSelection.spec.ts` | Pins `shouldReselectEpisode` (+ existing `pickEpisodeForProvider`) | **Create** |
| `frontend/web/src/composables/useWatchState.ts` | Composable: one `lastWatched` resolver → `startEpisode`/`banner`/`cta` | **Create** |
| `frontend/web/src/composables/__tests__/useWatchState.spec.ts` | Authed (server) + anon (localStorage) paths | **Create** |
| `frontend/web/src/components/player/aePlayer/AePlayer.vue` | Reactive episode pick; drop `episodesAired` fallback; `resumeBanner` prop | **Modify** |
| `frontend/web/src/components/player/ResumePill.vue` | Single `banner: ResumeBanner` prop; 2 variants | **Modify** |
| `frontend/web/src/views/Anime.vue` | Swap glue to `useWatchState`; one start-episode chain; drop dead code | **Modify** |
| `frontend/web/src/locales/{en,ru,ja}.json` | Remove `anime.resume.episodeNotLoaded` | **Modify** |
| `frontend/web/src/composables/watchCta.ts` | Folded into `watchState.ts` | **Delete** |
| `frontend/web/src/composables/__tests__/watchCta.spec.ts` | Ported into `watchState.spec.ts` | **Delete** |
| `frontend/web/src/composables/useResumeStateMachine.ts` | Replaced by `useWatchState.ts` | **Delete** |
| `frontend/web/src/composables/__tests__/useResumeStateMachine.spec.ts` | Replaced by `useWatchState.spec.ts` | **Delete** |
| `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.resumepill.spec.ts` | Migrate fixtures to `resumeBanner` | **Modify** |

---

## Task 1: Pure decision module `watchState.ts`

**Files:**
- Create: `frontend/web/src/composables/watchState.ts`
- Test: `frontend/web/src/composables/__tests__/watchState.spec.ts`

**Interfaces:**
- Produces: `resolveStartEpisode(lastWatched: number, totalEpisodes: number): number`; `resolveResumeState(input: ResumeStateInput): ResumeState`; `clampLastWatched(lastWatched: number, totalEpisodes: number): number`; types `WatchCtaAction`, `WatchCta`, `ResumeBanner`, `ResumeStateInput`, `ResumeState`.

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/composables/__tests__/watchState.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import {
  resolveStartEpisode,
  resolveResumeState,
  type ResumeStateInput,
} from '../watchState'

// ── resolveStartEpisode — the ONLY start-episode authority ──────────────────
describe('resolveStartEpisode', () => {
  it('first-time (0 watched) → ep 1', () => {
    expect(resolveStartEpisode(0, 12)).toBe(1)
  })
  it('watching (last < total) → last + 1', () => {
    expect(resolveStartEpisode(5, 12)).toBe(6)
  })
  it('caught up (last == total) → last (re-load the final ep)', () => {
    expect(resolveStartEpisode(12, 12)).toBe(12)
  })
  it('clamps a stale last > total to total', () => {
    expect(resolveStartEpisode(14, 12)).toBe(12)
  })
  it('unknown total (0) treats any positive last as watching', () => {
    expect(resolveStartEpisode(20, 0)).toBe(21)
  })
  it('never returns < 1', () => {
    expect(resolveStartEpisode(-3, 12)).toBe(1)
  })
})

// ── resolveResumeState — CTA verb (ported from computeWatchCta, all 16) ──────
function rs(over: Partial<ResumeStateInput> = {}): ResumeStateInput {
  return {
    lastWatched: 0,
    totalEpisodes: 12,
    episodesAired: 12,
    loadedEpisodes: 0,
    status: 'released',
    nextEpisodeAt: undefined,
    listStatus: null,
    isAuthenticated: true,
    nowMs: 1_000,
    ...over,
  }
}

describe('resolveResumeState.cta — authenticated, in progress', () => {
  it('#1 not in list, nothing watched → watch', () => {
    const { cta } = resolveResumeState(rs({ listStatus: null, lastWatched: 0 }))
    expect(cta.action).toBe('watch')
    expect(cta.startEpisode).toBe(1)
    expect(cta.labelKey).toBe('anime.watchNow')
  })
  it('#2 status=watching, nothing watched → watch', () => {
    expect(resolveResumeState(rs({ listStatus: 'watching', lastWatched: 0 })).cta.action).toBe('watch')
  })
  it('#3 status=completed, nothing watched → start-from-1', () => {
    const { cta } = resolveResumeState(rs({ listStatus: 'completed', lastWatched: 0 }))
    expect(cta.action).toBe('start-from-1')
    expect(cta.labelKey).toBe('anime.startFromEp1')
  })
  it('#4 partial progress → continue from next episode', () => {
    const { cta } = resolveResumeState(rs({ listStatus: 'watching', lastWatched: 5 }))
    expect(cta.action).toBe('continue')
    expect(cta.startEpisode).toBe(6)
    expect(cta.labelKey).toBe('anime.continueEp')
    expect(cta.labelParams).toEqual({ n: 6 })
  })
  it('#5 completed list but partial progress → continue (real progress wins)', () => {
    expect(resolveResumeState(rs({ listStatus: 'completed', lastWatched: 5 })).cta.action).toBe('continue')
  })
})

describe('resolveResumeState.cta — fully watched terminal', () => {
  it('#6 watched all, status≠completed → mark-watched', () => {
    const { cta } = resolveResumeState(rs({ listStatus: 'watching', lastWatched: 12 }))
    expect(cta.action).toBe('mark-watched')
    expect(cta.labelKey).toBe('anime.markAsWatched')
  })
  it('#7 watched all, status=completed → rewatch', () => {
    const { cta } = resolveResumeState(rs({ listStatus: 'completed', lastWatched: 12 }))
    expect(cta.action).toBe('rewatch')
    expect(cta.labelKey).toBe('anime.resume.rewatch')
  })
  it('#8 watched all, not in list → mark-watched', () => {
    expect(resolveResumeState(rs({ listStatus: null, lastWatched: 12 })).cta.action).toBe('mark-watched')
  })
  it('#9 last > total (anomaly) → clamp, treat as full → rewatch', () => {
    expect(resolveResumeState(rs({ listStatus: 'completed', lastWatched: 14 })).cta.action).toBe('rewatch')
  })
  it('#12 watched all but status=dropped → mark-watched', () => {
    expect(resolveResumeState(rs({ listStatus: 'dropped', lastWatched: 12 })).cta.action).toBe('mark-watched')
  })
})

describe('resolveResumeState.cta — unknown total', () => {
  it('#10 unknown total never "full" → continue', () => {
    const { cta } = resolveResumeState(rs({ listStatus: 'watching', lastWatched: 20, totalEpisodes: 0, episodesAired: 0 }))
    expect(cta.action).toBe('continue')
    expect(cta.startEpisode).toBe(21)
  })
  it('#11 unknown total, nothing watched, completed → start-from-1', () => {
    expect(resolveResumeState(rs({ listStatus: 'completed', lastWatched: 0, totalEpisodes: 0, episodesAired: 0 })).cta.action).toBe('start-from-1')
  })
})

describe('resolveResumeState.cta — anonymous', () => {
  it('#14 anon, nothing watched → watch', () => {
    expect(resolveResumeState(rs({ isAuthenticated: false, listStatus: null, lastWatched: 0 })).cta.action).toBe('watch')
  })
  it('#15 anon, partial → continue', () => {
    expect(resolveResumeState(rs({ isAuthenticated: false, listStatus: null, lastWatched: 5 })).cta.action).toBe('continue')
  })
  it('#16 anon, watched all → watch (never mark/rewatch)', () => {
    const { cta } = resolveResumeState(rs({ isAuthenticated: false, listStatus: null, lastWatched: 12 }))
    expect(cta.action).toBe('watch')
  })
})

// ── resolveResumeState — banner (collapsed 5 kinds → 3) ──────────────────────
describe('resolveResumeState.banner', () => {
  it('first-time → none', () => {
    expect(resolveResumeState(rs({ lastWatched: 0 })).banner).toEqual({ kind: 'none' })
  })
  it('finished (last >= total) → none', () => {
    expect(resolveResumeState(rs({ lastWatched: 12 })).banner).toEqual({ kind: 'none' })
  })
  it('watching (next aired) → just-finished{episode:last}', () => {
    const { banner } = resolveResumeState(rs({ lastWatched: 5, status: 'released' }))
    expect(banner).toEqual({ kind: 'just-finished', episode: 5 })
  })
  it('loadedEpisodes overrides lagging episodesAired → just-finished', () => {
    const { banner } = resolveResumeState(rs({
      lastWatched: 5, status: 'ongoing', episodesAired: 5, loadedEpisodes: 6,
    }))
    expect(banner).toEqual({ kind: 'just-finished', episode: 5 })
  })
  it('ongoing, next not aired, future ETA → next-unavailable with etaLabel', () => {
    const { banner } = resolveResumeState(rs({
      lastWatched: 5, totalEpisodes: 12, status: 'ongoing', episodesAired: 5, loadedEpisodes: 0,
      nextEpisodeAt: new Date(10_000).toISOString(), nowMs: 1_000,
      formatEta: () => 'in 2 days',
    }))
    expect(banner).toEqual({ kind: 'next-unavailable', episode: 6, etaLabel: 'in 2 days' })
  })
  it('ongoing, next air time PAST (aired-not-loaded) → next-unavailable, no eta', () => {
    const { banner } = resolveResumeState(rs({
      lastWatched: 5, totalEpisodes: 12, status: 'ongoing', episodesAired: 5, loadedEpisodes: 0,
      nextEpisodeAt: new Date(500).toISOString(), nowMs: 1_000,
      formatEta: () => 'should-not-be-used',
    }))
    expect(banner).toEqual({ kind: 'next-unavailable', episode: 6 })
  })
})
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/watchState.spec.ts`
Expected: FAIL — `Cannot find module '../watchState'`.

- [ ] **Step 3: Write the implementation**

Create `frontend/web/src/composables/watchState.ts`:

```ts
// Pure decision functions for episode selection + resume presentation.
//
// Single source of truth for "which episode does the player open" and
// "what banner / CTA verb to show". Replaces watchCta.ts and the
// startEpisode/kind logic of useResumeStateMachine.ts.
//
// Pinned by src/composables/__tests__/watchState.spec.ts.

export type WatchCtaAction = 'watch' | 'start-from-1' | 'continue' | 'mark-watched' | 'rewatch'

export interface WatchCta {
  action: WatchCtaAction
  /** Episode the player should mount on. */
  startEpisode: number
  /** i18n key for the button label. */
  labelKey: string
  /** Optional i18n interpolation params (e.g. { n } for "continue from ep N"). */
  labelParams?: Record<string, number>
}

export type ResumeBanner =
  | { kind: 'none' }
  | { kind: 'just-finished'; episode: number }
  | { kind: 'next-unavailable'; episode: number; etaLabel?: string }

export interface ResumeStateInput {
  /** Highest COMPLETED episode for this user; 0 when none. */
  lastWatched: number
  /** Total episodes; 0 when unknown (then "full" is unreachable). */
  totalEpisodes: number
  /** Shikimori episodes_aired; 0 when unknown. */
  episodesAired: number
  /** Highest episode loaded into our sources (overrides lagging episodesAired); 0 when unknown. */
  loadedEpisodes: number
  /** anime.status — 'ongoing' | 'released' | 'completed' | … */
  status: string
  /** ISO air time of the next episode, or undefined. */
  nextEpisodeAt?: string
  /** anime_list status, or null when not in list / anonymous. */
  listStatus: string | null
  /** Whether the viewer is logged in (drives list-aware CTA actions). */
  isAuthenticated: boolean
  /** Localized formatter for a FUTURE air time → ETA label. Omitted ⇒ no ETA. */
  formatEta?: (iso: string) => string
  /** Injectable clock for the future/past air-time comparison (defaults to Date.now()). */
  nowMs?: number
}

export interface ResumeState {
  banner: ResumeBanner
  cta: WatchCta
}

/** Clamp lastWatched into [0, total] when total is known; else floor at 0. */
export function clampLastWatched(lastWatched: number, totalEpisodes: number): number {
  const total = totalEpisodes > 0 ? totalEpisodes : 0
  return total > 0 ? Math.min(Math.max(lastWatched, 0), total) : Math.max(lastWatched, 0)
}

/**
 * The episode the player should mount on. The ONLY start-episode authority.
 *   first-time (0)            → 1
 *   caught up (last >= total) → last   (re-load the final episode)
 *   otherwise                 → last + 1
 * Always returns a number >= 1.
 */
export function resolveStartEpisode(lastWatched: number, totalEpisodes: number): number {
  const last = clampLastWatched(lastWatched, totalEpisodes)
  if (last <= 0) return 1
  const total = totalEpisodes > 0 ? totalEpisodes : 0
  if (total > 0 && last >= total) return last
  return last + 1
}

function resolveCta(a: {
  isAuthenticated: boolean
  last: number
  isFull: boolean
  isCompleted: boolean
}): WatchCta {
  const { isAuthenticated, last, isFull, isCompleted } = a
  const watch: WatchCta = { action: 'watch', startEpisode: 1, labelKey: 'anime.watchNow' }
  const continueFrom = (ep: number): WatchCta => ({
    action: 'continue',
    startEpisode: ep,
    labelKey: 'anime.continueEp',
    labelParams: { n: ep },
  })

  // Anonymous: no list, so the list-aware terminals never apply.
  if (!isAuthenticated) {
    return last > 0 && !isFull ? continueFrom(last + 1) : watch
  }
  // Nothing watched yet.
  if (last === 0) {
    return isCompleted
      ? { action: 'start-from-1', startEpisode: 1, labelKey: 'anime.startFromEp1' }
      : watch
  }
  // Fully watched terminal — list status picks the verb.
  if (isFull) {
    return isCompleted
      ? { action: 'rewatch', startEpisode: 1, labelKey: 'anime.resume.rewatch' }
      : { action: 'mark-watched', startEpisode: 1, labelKey: 'anime.markAsWatched' }
  }
  // Partial progress (also the unknown-total path) — real progress wins.
  return continueFrom(last + 1)
}

function resolveBanner(a: {
  last: number
  total: number
  episodesAired: number
  loadedEpisodes: number
  status: string
  nextEpisodeAt?: string
  formatEta?: (iso: string) => string
  nowMs: number
}): ResumeBanner {
  const { last, total, episodesAired, loadedEpisodes, status, nextEpisodeAt, formatEta, nowMs } = a

  if (last <= 0) return { kind: 'none' } // first-time
  if (total > 0 && last >= total) return { kind: 'none' } // finished — no surface

  // Is episode last+1 available? Released/completed → yes. Otherwise compare
  // against the effective aired count (max of Shikimori + what our sources hold).
  const aired = Math.max(episodesAired, loadedEpisodes)
  const isReleased = status === 'released' || status === 'completed'
  const nextIsAired = aired > 0 && aired > last
  if (isReleased || nextIsAired) {
    return { kind: 'just-finished', episode: last } // 'watching' breadcrumb
  }

  // Next episode not available yet (merged not-yet-aired + episode-not-loaded-yet).
  const nextEp = last + 1
  const t = nextEpisodeAt ? new Date(nextEpisodeAt).getTime() : Number.NaN
  const etaLabel =
    !Number.isNaN(t) && t > nowMs && formatEta ? formatEta(nextEpisodeAt as string) : undefined
  return etaLabel ? { kind: 'next-unavailable', episode: nextEp, etaLabel } : { kind: 'next-unavailable', episode: nextEp }
}

/**
 * Banner + CTA verb for the anime page. Pure. Replaces computeWatchCta + the
 * 5-kind resume state machine.
 */
export function resolveResumeState(input: ResumeStateInput): ResumeState {
  const last = clampLastWatched(input.lastWatched, input.totalEpisodes)
  const total = input.totalEpisodes > 0 ? input.totalEpisodes : 0
  const isFull = total > 0 && last >= total
  const isCompleted = input.listStatus === 'completed'

  const cta = resolveCta({ isAuthenticated: input.isAuthenticated, last, isFull, isCompleted })
  const banner = resolveBanner({
    last,
    total,
    episodesAired: input.episodesAired,
    loadedEpisodes: input.loadedEpisodes,
    status: input.status,
    nextEpisodeAt: input.nextEpisodeAt,
    formatEta: input.formatEta,
    nowMs: input.nowMs ?? Date.now(),
  })

  return { banner, cta }
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/watchState.spec.ts`
Expected: PASS (all describe blocks green).

- [ ] **Step 5: Commit**

```bash
cd /data/ae-episode-selection
git add frontend/web/src/composables/watchState.ts frontend/web/src/composables/__tests__/watchState.spec.ts
git commit -m "feat(web): pure watchState module (start-episode + resume state)

$(printf 'Co-Authored-By: Claude Code <noreply@anthropic.com>\nCo-Authored-By: 0neymik0 <0neymik0@gmail.com>\nCo-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>')"
```

---

## Task 2: Race-guard helper `shouldReselectEpisode`

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/episodeSelection.ts`
- Test: `frontend/web/src/composables/aePlayer/__tests__/episodeSelection.spec.ts` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `shouldReselectEpisode(currentNumber: number | null, initialEpisode: number | undefined, userPicked: boolean): boolean`.

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/composables/aePlayer/__tests__/episodeSelection.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { pickEpisodeForProvider, shouldReselectEpisode } from '../episodeSelection'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

const ep = (n: number): EpisodeOption => ({ key: n, label: n, number: n })

describe('shouldReselectEpisode — closes the mount race', () => {
  it('re-picks when initialEpisode resolves to a different number and user has not picked', () => {
    // mount default was 1; resume resolved to 6 → move to 6
    expect(shouldReselectEpisode(1, 6, false)).toBe(true)
  })
  it('does NOT re-pick once the user manually chose an episode', () => {
    expect(shouldReselectEpisode(1, 6, true)).toBe(false)
  })
  it('does NOT re-pick when initialEpisode is still undefined', () => {
    expect(shouldReselectEpisode(1, undefined, false)).toBe(false)
  })
  it('does NOT re-pick when already on the target episode (no churn)', () => {
    expect(shouldReselectEpisode(6, 6, false)).toBe(false)
  })
  it('re-picks from a null current selection', () => {
    expect(shouldReselectEpisode(null, 1, false)).toBe(true)
  })
})

// Guard the existing helper stays intact.
describe('pickEpisodeForProvider (regression)', () => {
  it('keeps an exact number match', () => {
    expect(pickEpisodeForProvider([ep(1), ep(2), ep(3)], 2, null)?.number).toBe(2)
  })
})
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/episodeSelection.spec.ts`
Expected: FAIL — `shouldReselectEpisode is not a function`.

- [ ] **Step 3: Add the helper**

Append to `frontend/web/src/composables/aePlayer/episodeSelection.ts` (after `pickEpisodeForProvider`):

```ts
/**
 * Decide whether the player should re-pick its episode when `initialEpisode`
 * changes AFTER mount. Resume/watch-progress resolves asynchronously, so the
 * prop flips from its mount value (default 1) to e.g. `lastWatched + 1` a tick
 * later. We re-select then — UNLESS the user has already manually chosen an
 * episode (never yank them off a deliberate pick), the prop is still absent,
 * or we are already on the target (no churn). Pure: testable in isolation.
 */
export function shouldReselectEpisode(
  currentNumber: number | null,
  initialEpisode: number | undefined,
  userPicked: boolean,
): boolean {
  if (initialEpisode == null || userPicked) return false
  return currentNumber !== initialEpisode
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/__tests__/episodeSelection.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/ae-episode-selection
git add frontend/web/src/composables/aePlayer/episodeSelection.ts frontend/web/src/composables/aePlayer/__tests__/episodeSelection.spec.ts
git commit -m "feat(aeplayer): shouldReselectEpisode race guard

$(printf 'Co-Authored-By: Claude Code <noreply@anthropic.com>\nCo-Authored-By: 0neymik0 <0neymik0@gmail.com>\nCo-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>')"
```

---

## Task 3: Composable `useWatchState`

**Files:**
- Create: `frontend/web/src/composables/useWatchState.ts`
- Test: `frontend/web/src/composables/__tests__/useWatchState.spec.ts`

**Interfaces:**
- Consumes: `resolveStartEpisode`, `resolveResumeState`, `clampLastWatched`, `ResumeBanner`, `WatchCta` (Task 1); `parseLastWatchedEpisode` from `@/composables/anime/animeFormatters`; `userApi` from `@/api/client`.
- Produces:
  ```ts
  useWatchState(options: UseWatchStateOptions): WatchState
  // UseWatchStateOptions: { animeId, totalEpisodes, episodesAired, status,
  //   nextEpisodeAt, loadedEpisodes, listStatus, isAuthenticated: Ref<…>; formatEta: (iso)=>string }
  // WatchState: { lastWatched: ComputedRef<number>; loaded: Ref<boolean>;
  //   startEpisode: ComputedRef<number>; banner: ComputedRef<ResumeBanner>;
  //   cta: ComputedRef<WatchCta>; init(prefetched?): Promise<void>; reset(): void }
  ```

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/composables/__tests__/useWatchState.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'
import { useWatchState } from '../useWatchState'

vi.mock('@/api/client', () => ({
  userApi: { getProgress: vi.fn() },
}))
import { userApi } from '@/api/client'

function opts(over: Record<string, unknown> = {}) {
  return {
    animeId: ref('anime-1'),
    totalEpisodes: ref(12),
    episodesAired: ref(12),
    status: ref('released'),
    nextEpisodeAt: ref<string | undefined>(undefined),
    loadedEpisodes: ref(0),
    listStatus: ref<string | null>(null),
    isAuthenticated: ref(true),
    formatEta: (iso: string) => `eta:${iso}`,
    ...over,
  }
}

beforeEach(() => {
  localStorage.clear()
  vi.mocked(userApi.getProgress).mockReset()
})

describe('useWatchState — authenticated', () => {
  it('derives lastWatched (max completed) from prefetched rows → startEpisode = last+1', async () => {
    const ws = useWatchState(opts())
    await ws.init([
      { episode_number: 3, completed: true },
      { episode_number: 5, completed: true },
      { episode_number: 6, completed: false },
    ])
    expect(ws.lastWatched.value).toBe(5)
    expect(ws.startEpisode.value).toBe(6)
    expect(ws.banner.value).toEqual({ kind: 'just-finished', episode: 5 })
    expect(ws.cta.value.action).toBe('continue')
  })

  it('first-time authed → startEpisode 1, banner none', async () => {
    const ws = useWatchState(opts())
    await ws.init([])
    expect(ws.startEpisode.value).toBe(1)
    expect(ws.banner.value).toEqual({ kind: 'none' })
  })
})

describe('useWatchState — anonymous (D-1 adapter)', () => {
  it('opens the in-progress episode from localStorage and keeps CTA consistent', async () => {
    // last-touched episode 6 (most recent updatedAt)
    localStorage.setItem(
      'watch_progress:anime-1',
      JSON.stringify({ '5': { updatedAt: 10 }, '6': { updatedAt: 20 } }),
    )
    const ws = useWatchState(opts({ isAuthenticated: ref(false) }))
    await ws.init()
    // lastWatched (completed) = parsedEp - 1 = 5  →  startEpisode = 6 (same ep as before)
    expect(ws.lastWatched.value).toBe(5)
    expect(ws.startEpisode.value).toBe(6)
    expect(ws.cta.value).toMatchObject({ action: 'continue', startEpisode: 6 })
  })

  it('anon with no localStorage → first-time, ep 1 (fixes the ep-12 default bug)', async () => {
    const ws = useWatchState(opts({ isAuthenticated: ref(false) }))
    await ws.init()
    expect(ws.lastWatched.value).toBe(0)
    expect(ws.startEpisode.value).toBe(1)
  })
})

describe('useWatchState — server fetch fallback', () => {
  it('fetches progress when no prefetch is supplied', async () => {
    vi.mocked(userApi.getProgress).mockResolvedValue({
      data: { data: [{ episode_number: 2, completed: true }] },
    } as never)
    const ws = useWatchState(opts())
    await ws.init()
    expect(userApi.getProgress).toHaveBeenCalledWith('anime-1')
    expect(ws.lastWatched.value).toBe(2)
  })
})
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useWatchState.spec.ts`
Expected: FAIL — `Cannot find module '../useWatchState'`.

- [ ] **Step 3: Write the implementation**

Create `frontend/web/src/composables/useWatchState.ts`:

```ts
import { ref, computed, type Ref, type ComputedRef } from 'vue'
import { userApi } from '@/api/client'
import { parseLastWatchedEpisode } from '@/composables/anime/animeFormatters'
import {
  resolveStartEpisode,
  resolveResumeState,
  clampLastWatched,
  type ResumeBanner,
  type WatchCta,
} from '@/composables/watchState'

/**
 * useWatchState — single source of truth for "where is the user in this anime".
 *
 * Resolves `lastWatched` (highest completed episode) for BOTH authed (server
 * watch_progress) and anonymous (localStorage) viewers, then derives the start
 * episode, the resume banner, and the CTA verb via the pure functions in
 * watchState.ts. Replaces useResumeStateMachine.ts + watchCta.ts.
 */
export interface UseWatchStateOptions {
  animeId: Ref<string>
  totalEpisodes: Ref<number>
  episodesAired: Ref<number>
  status: Ref<string>
  nextEpisodeAt: Ref<string | undefined>
  /** Highest episode actually loaded into our sources (player-emitted); 0 when unknown. */
  loadedEpisodes: Ref<number>
  listStatus: Ref<string | null>
  isAuthenticated: Ref<boolean>
  /** Localized future-air-time formatter (e.g. Anime.vue's formatNextEpisode). */
  formatEta: (iso: string) => string
}

export interface WatchState {
  lastWatched: ComputedRef<number>
  loaded: Ref<boolean>
  startEpisode: ComputedRef<number>
  banner: ComputedRef<ResumeBanner>
  cta: ComputedRef<WatchCta>
  init: (prefetched?: Array<{ episode_number?: number; completed?: boolean }>) => Promise<void>
  reset: () => void
}

/** Highest COMPLETED episode across the watch_progress rows; 0 when none. */
function deriveLastWatched(progress: Array<{ episode_number?: number; completed?: boolean }>): number {
  let max = 0
  for (const p of progress) {
    if (p?.completed && typeof p.episode_number === 'number' && p.episode_number > max) {
      max = p.episode_number
    }
  }
  return max
}

export function useWatchState(options: UseWatchStateOptions): WatchState {
  const loaded = ref(false)
  // Server-derived highest completed episode (authed only).
  const rawServerLastWatched = ref(0)

  // lastWatched (highest COMPLETED), unified across authed + anon.
  //  - authed: server watch_progress (max completed).
  //  - anon:   localStorage only records last-TOUCHED + position, no `completed`
  //            flag (D-1). The last-touched episode IS the in-progress one, so
  //            the highest *completed* is one below it. This makes anon open the
  //            SAME episode it always did while keeping the CTA consistent.
  const lastWatched = computed(() => {
    const total = options.totalEpisodes.value
    if (options.isAuthenticated.value) {
      return clampLastWatched(rawServerLastWatched.value, total)
    }
    const parsed = parseLastWatchedEpisode(localStorage.getItem(`watch_progress:${options.animeId.value}`))
    const completed = parsed && parsed > 0 ? parsed - 1 : 0
    return clampLastWatched(completed, total)
  })

  const startEpisode = computed(() => resolveStartEpisode(lastWatched.value, options.totalEpisodes.value))

  const state = computed(() =>
    resolveResumeState({
      lastWatched: lastWatched.value,
      totalEpisodes: options.totalEpisodes.value,
      episodesAired: options.episodesAired.value,
      loadedEpisodes: options.loadedEpisodes.value,
      status: options.status.value,
      nextEpisodeAt: options.nextEpisodeAt.value,
      listStatus: options.listStatus.value,
      isAuthenticated: options.isAuthenticated.value,
      formatEta: options.formatEta,
    }),
  )
  const banner = computed(() => state.value.banner)
  const cta = computed(() => state.value.cta)

  async function init(prefetched?: Array<{ episode_number?: number; completed?: boolean }>) {
    loaded.value = false
    if (!options.isAuthenticated.value) {
      // Anon reads localStorage lazily in the `lastWatched` computed — nothing to fetch.
      loaded.value = true
      return
    }
    if (prefetched) {
      rawServerLastWatched.value = deriveLastWatched(prefetched)
      loaded.value = true
      return
    }
    try {
      const res = await userApi.getProgress(options.animeId.value)
      const data = (res.data?.data ?? res.data ?? []) as Array<{
        episode_number?: number
        completed?: boolean
      }>
      rawServerLastWatched.value = deriveLastWatched(data)
    } catch {
      // 404 / network failure → first-time is a safe default.
      rawServerLastWatched.value = 0
    } finally {
      loaded.value = true
    }
  }

  function reset() {
    loaded.value = false
    rawServerLastWatched.value = 0
  }

  return { lastWatched, loaded, startEpisode, banner, cta, init, reset }
}
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useWatchState.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/ae-episode-selection
git add frontend/web/src/composables/useWatchState.ts frontend/web/src/composables/__tests__/useWatchState.spec.ts
git commit -m "feat(web): useWatchState composable (unified authed+anon resume)

$(printf 'Co-Authored-By: Claude Code <noreply@anthropic.com>\nCo-Authored-By: 0neymik0 <0neymik0@gmail.com>\nCo-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>')"
```

---

## Task 4: AePlayer race fix (internal — no prop interface change)

This task fixes the ep-12 bug on its own: default to ep 1 (never `episodesAired`) and consume `initialEpisode` reactively. The `ep` field stays on the prop type for now (still passed by `Anime.vue`); it is simply no longer read. Build stays green; the cutover that removes the prop is Task 5.

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`

**Interfaces:**
- Consumes: `shouldReselectEpisode` (Task 2).

- [ ] **Step 1: Import the guard and add the manual-pick flag**

In `AePlayer.vue`, the `episodeSelection` import currently brings in `pickEpisodeForProvider`. Update it to also import `shouldReselectEpisode`:

```ts
import { pickEpisodeForProvider, shouldReselectEpisode } from '@/composables/aePlayer/episodeSelection'
```

Right after `const selectedEpisode = ref<EpisodeOption | null>(null)` (line ~960) add:

```ts
// True once the user manually switches episodes — freezes the reactive
// initialEpisode re-pick so resume resolving late never yanks a deliberate pick.
const userPickedEpisode = ref(false)
```

- [ ] **Step 2: Default to ep 1 (never episodesAired) + reactive re-pick**

Replace `initSelectedEpisode` (lines ~1278-1290):

```ts
// Initialize selectedEpisode from initialEpisode (NEVER episodesAired).
function initSelectedEpisode() {
  const targetEp = props.initialEpisode ?? 1
  const found = episodes.value.find((e) => e.number === targetEp)
  if (found) {
    selectedEpisode.value = found
  } else if (episodes.value.length > 0) {
    selectedEpisode.value = episodes.value[0]
  } else {
    // Synthetic placeholder for when the episode list isn't loaded yet.
    selectedEpisode.value = { key: targetEp, label: targetEp, number: targetEp }
  }
}

// Resume / watch-progress resolves asynchronously AFTER mount, so initialEpisode
// flips from its default (1) to e.g. lastWatched+1 a tick later. Re-pick then —
// unless the user already chose an episode. Closes the mount race that used to
// fall back to episodesAired and open the latest aired episode instead of ep 1.
watch(
  () => props.initialEpisode,
  () => {
    if (shouldReselectEpisode(selectedEpisode.value?.number ?? null, props.initialEpisode, userPickedEpisode.value)) {
      initSelectedEpisode()
    }
  },
)
```

- [ ] **Step 3: Mark manual picks**

In `onSelectEpisode` (line ~1511), immediately after `selectedEpisode.value = ep` add:

```ts
  userPickedEpisode.value = true
```

- [ ] **Step 4: Remove every `episodesAired` selection fallback**

Replace `props.anime.ep` in these four reads with the `initialEpisode`-based fallback:

- Line ~1344 (provider-load target):
  ```ts
  const targetNum = selectedEpisode.value?.number ?? props.initialEpisode ?? 1
  ```
- Line ~1964 (`currentEpNumber`):
  ```ts
  () => selectedEpisode.value?.number ?? props.initialEpisode ?? 1,
  ```
- Line ~2207 (`subEpisode`):
  ```ts
  const subEpisode = computed(() => selectedEpisode.value?.number ?? props.initialEpisode ?? 1)
  ```
- The episode badge (lines ~92 and ~200 in the template):
  ```vue
  {{ $t('player.aePlayer.epAbbrev') }} {{ selectedEpisode?.number ?? initialEpisode ?? 1 }}
  ```
  ```vue
  :episode-label="selectedEpisode?.number ?? initialEpisode ?? 1"
  ```

Verify none remain: `grep -n "anime\.ep\b" frontend/web/src/components/player/aePlayer/AePlayer.vue` must return **only** the prop-type line (`ep: number`) and comments — no live reads.

- [ ] **Step 5: Type-check + run player specs**

Run:
```bash
cd frontend/web
bunx tsc --noEmit
bunx vitest run src/components/player/aePlayer/__tests__/
```
Expected: tsc clean; existing AePlayer specs PASS.

- [ ] **Step 6: Commit**

```bash
cd /data/ae-episode-selection
git add frontend/web/src/components/player/aePlayer/AePlayer.vue
git commit -m "fix(aeplayer): default ep 1 + reactive initial-episode (kills mount race)

Player no longer falls back to episodesAired when resume resolves after mount;
initialEpisode is consumed reactively and re-picks until the user chooses.

$(printf 'Co-Authored-By: Claude Code <noreply@anthropic.com>\nCo-Authored-By: 0neymik0 <0neymik0@gmail.com>\nCo-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>')"
```

---

## Task 5: Cutover — `ResumePill` + `AePlayer` prop + `Anime.vue` to `useWatchState`

Atomic interface change: `ResumePill` takes a single `banner: ResumeBanner`; `AePlayer` swaps `resumePill` → `resumeBanner`; `Anime.vue` swaps its resume glue to `useWatchState` and collapses the start-episode chain. After this task the new system is live and the old files are unreferenced.

**Files:**
- Modify: `frontend/web/src/components/player/ResumePill.vue`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`
- Modify: `frontend/web/src/views/Anime.vue`

**Interfaces:**
- Consumes: `useWatchState` (Task 3), `ResumeBanner` (Task 1).

- [ ] **Step 1: Rewrite `ResumePill.vue` to the `banner` prop**

Replace the whole file with:

```vue
<template>
  <div
    v-if="banner.kind !== 'none'"
    role="status"
    class="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg bg-muted/50 border border-border text-muted-foreground text-xs flex-wrap"
  >
    <template v-if="banner.kind === 'just-finished'">
      <Check class="size-3.5 text-brand-cyan flex-shrink-0" aria-hidden="true" />
      <span>{{ t('anime.resume.justFinished', { n: banner.episode }) }}</span>
    </template>

    <template v-else-if="banner.kind === 'next-unavailable'">
      <Clock class="size-3.5 text-warning/70 flex-shrink-0" aria-hidden="true" />
      <span v-if="banner.etaLabel">
        {{ t('anime.resume.notYetAvailableEta', { n: banner.episode, when: banner.etaLabel }) }}
      </span>
      <span v-else>
        {{ t('anime.resume.notYetAvailable', { n: banner.episode }) }}
      </span>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Check, Clock } from 'lucide-vue-next'
import type { ResumeBanner } from '@/composables/watchState'

defineProps<{ banner: ResumeBanner }>()

const { t } = useI18n()
</script>
```

- [ ] **Step 2: Swap `AePlayer`'s resume prop to `resumeBanner`**

In `AePlayer.vue`:

(a) Add the type import near the other imports (after the `ResumePill` import at line ~336):
```ts
import type { ResumeBanner } from '@/composables/watchState'
```

(b) Replace the `resumePill` prop (lines ~430-435) with:
```ts
  resumeBanner?: ResumeBanner
```

(c) Replace the `resumeBannerProps` overlay (lines ~2334-2340) — show ONLY the airing-status family over the video (never the just-finished breadcrumb):
```ts
const resumeBannerProps = computed<ResumeBanner>(() => {
  const b = props.resumeBanner
  return b && b.kind === 'next-unavailable' ? b : { kind: 'none' }
})
```

(d) Update the internal render (line ~130):
```vue
      <ResumePill :banner="resumeBannerProps" />
```

- [ ] **Step 3: Rewire `Anime.vue` imports**

Remove these imports:
```ts
import { useResumeStateMachine } from '@/composables/useResumeStateMachine'
import { computeWatchCta, type WatchCta } from '@/composables/watchCta'
```
Add:
```ts
import { useWatchState } from '@/composables/useWatchState'
```

- [ ] **Step 4: Replace the resume composable wiring**

Replace the `resume = useResumeStateMachine({...})` block (lines ~1167-1175) with `useWatchState`, adding `listStatus` and `formatEta` (wrap `formatNextEpisode` so it stays bound even though it is declared later in the file):

```ts
const watchState = useWatchState({
  animeId: resumeAnimeId,
  totalEpisodes: resumeTotal,
  episodesAired: resumeAired,
  status: resumeStatus,
  nextEpisodeAt: resumeNextAt,
  loadedEpisodes: resumeLoadedEpisodes,
  listStatus: currentListStatus,
  isAuthenticated: resumeAuth,
  formatEta: (iso: string) => formatNextEpisode(iso),
})
```

- [ ] **Step 5: Delete the hybrid `lastEpisode` plumbing**

Remove:
- `const lastEpisode = ref<number | undefined>(undefined)` (line ~1150).
- the `function loadLastEpisode(...)` definition (lines ~1177-1180).
- the `watch(() => resume.lastWatched.value, ...)` mirror (lines ~1184-1186).
- the `loadLastEpisode(fetched.id)` call (line ~2175).

- [ ] **Step 6: Collapse `watchCta`, the start-episode chain, and the banner**

Replace the `watchCta` computed (lines ~1021-1028) with:
```ts
const watchCta = computed(() => watchState.cta.value)
```
(`watchCtaLabel` at line ~1030 stays — it reads `watchCta.value`.)

Replace the `resumeStartEpisode` computed (lines ~1237-1249) with the single chain:
```ts
// Single start-episode authority: explicit overrides win, else the unified
// watchState. Always a number >= 1 — never episodesAired.
const resumeStartEpisode = computed<number>(() => {
  if (resumeOverrideEpisode.value && resumeOverrideEpisode.value > 0) {
    return resumeOverrideEpisode.value
  }
  if (queryEpisode.value !== undefined) {
    return queryEpisode.value
  }
  return watchState.startEpisode.value
})
```

Delete `resumeNextEpisodeNumber` (lines ~1272-1280) and the `resumePillProps` computed (lines ~1285-1312). Add a banner passthrough:
```ts
const resumeBanner = computed(() => watchState.banner.value)
```

- [ ] **Step 7: Update the player bindings**

`<AePlayer>` (lines ~404-409): drop `ep` from the `:anime` object and swap the resume prop:
```vue
            :anime="{ title: anime.title, eps: (anime.totalEpisodes || anime.episodesAired || 1), still: anime.coverImage }"
            :theater="theaterMode"
            :is-hentai="isHentai"
            :initial-episode="resumeStartEpisode"
            :resume-banner="resumeBanner"
```

`<KodikPlayer>` slot (line ~445):
```vue
              <ResumePill :banner="resumeBanner" />
```
Its `:initial-episode="resumeStartEpisode"` (line ~441) is unchanged.

- [ ] **Step 8: Repoint the `resume.*` call sites**

Replace remaining references:
- `await resume.init()` in `startRewatchFlow` (line ~1047) → `await watchState.init()`.
- `void resume.init(viewerCtx.progress ?? [])` (line ~2180) → `void watchState.init(viewerCtx.progress ?? [])`.
- `void resume.init()` (line ~2191) → `void watchState.init()`.
- If `resume.reset()` appears (grep), → `watchState.reset()`.

Verify nothing dangles: `grep -n "\bresume\.\|resumePillProps\|lastEpisode\|computeWatchCta\|useResumeStateMachine" frontend/web/src/views/Anime.vue` must return **no matches**.

- [ ] **Step 9: Remove `ep` from the `AePlayer` prop type**

Now that no `props.anime.ep` reads remain (Task 4) and `Anime.vue` no longer passes `ep`, drop it from the `anime` prop type (line ~406):
```ts
  anime: { title: string; eps: number; still?: string }
```

- [ ] **Step 10: Type-check, build, and run the affected specs**

Run:
```bash
cd frontend/web
bunx tsc --noEmit
bunx vitest run src/components/player/ src/views/__tests__/ src/composables/__tests__/
bun run build
```
Expected: tsc clean; specs PASS (note: `useResumeStateMachine.spec.ts` and `watchCta.spec.ts` still pass — they are deleted in Task 6); `bun run build` succeeds.

- [ ] **Step 11: Commit**

```bash
cd /data/ae-episode-selection
git add frontend/web/src/components/player/ResumePill.vue frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/views/Anime.vue
git commit -m "refactor(web): cut episode-selection over to useWatchState (one source of truth)

ResumePill takes a single ResumeBanner; AePlayer takes resumeBanner; Anime.vue
drops the lastEpisode hybrid, the 4-way start-episode chain, and computeWatchCta.

$(printf 'Co-Authored-By: Claude Code <noreply@anthropic.com>\nCo-Authored-By: 0neymik0 <0neymik0@gmail.com>\nCo-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>')"
```

---

## Task 6: Delete dead files + i18n cleanup + migrate the resume-pill spec

**Files:**
- Delete: `frontend/web/src/composables/watchCta.ts`, `frontend/web/src/composables/__tests__/watchCta.spec.ts`
- Delete: `frontend/web/src/composables/useResumeStateMachine.ts`, `frontend/web/src/composables/__tests__/useResumeStateMachine.spec.ts`
- Modify: `frontend/web/src/locales/{en,ru,ja}.json`
- Modify: `frontend/web/src/components/player/aePlayer/__tests__/AePlayer.resumepill.spec.ts`

- [ ] **Step 1: Migrate the AePlayer resume-pill spec to the new prop**

Read `AePlayer.resumepill.spec.ts` and replace every `resumePill` mount prop with `resumeBanner`, converting fixtures:
- `resumePill: { kind: 'episode-not-loaded-yet', nextEpisodeNumber: N, airedAgoLabel: '…' }` → `resumeBanner: { kind: 'next-unavailable', episode: N }`
- `resumePill: { kind: 'not-yet-aired', nextEpisodeNumber: N, nextEpisodeEtaLabel: '…' }` → `resumeBanner: { kind: 'next-unavailable', episode: N, etaLabel: '…' }`
- `resumePill: { kind: 'watching', finishedEpisode: N }` → `resumeBanner: { kind: 'just-finished', episode: N }` (and assert it is NOT rendered over the video — `resumeBannerProps` shows only `next-unavailable`).

Assertion for the overlay behavior (the in-player pill shows only the airing family):
```ts
// just-finished must NOT render over the video
expect(wrapper.findComponent(ResumePill).exists()).toBe(false) // or: props().banner.kind === 'none'
```
Run it to confirm green:
```bash
cd frontend/web && bunx vitest run src/components/player/aePlayer/__tests__/AePlayer.resumepill.spec.ts
```
Expected: PASS.

- [ ] **Step 2: Remove the `episodeNotLoaded` key from all three locales**

In `en.json`, `ru.json`, `ja.json` delete the `"episodeNotLoaded": "…"` line inside `anime.resume` (line ~120 in each). Ensure the preceding line keeps valid JSON (no trailing comma left dangling — `notYetAvailableEta` becomes the last key in the block, so it must NOT end with a comma).

Verify no references remain:
```bash
grep -rn "episodeNotLoaded\|airedAgoLabel\|episodeAiredAgoMs" frontend/web/src/
```
Expected: no matches.

- [ ] **Step 3: Delete the replaced files**

```bash
cd /data/ae-episode-selection
git rm frontend/web/src/composables/watchCta.ts \
       frontend/web/src/composables/__tests__/watchCta.spec.ts \
       frontend/web/src/composables/useResumeStateMachine.ts \
       frontend/web/src/composables/__tests__/useResumeStateMachine.spec.ts
```

Verify nothing imports them:
```bash
grep -rn "watchCta\|useResumeStateMachine" frontend/web/src/ | grep -v "watchState"
```
Expected: no matches.

- [ ] **Step 4: Type-check + targeted tests + build**

```bash
cd frontend/web
bunx tsc --noEmit
bunx vitest run src/composables/__tests__/ src/components/player/ src/views/__tests__/
bun run build
```
Expected: all clean/green.

- [ ] **Step 5: Commit**

```bash
cd /data/ae-episode-selection
git add -A frontend/web/src/locales frontend/web/src/components/player/aePlayer/__tests__/AePlayer.resumepill.spec.ts
git commit -m "chore(web): delete watchCta + resume state machine; drop episodeNotLoaded i18n

$(printf 'Co-Authored-By: Claude Code <noreply@anthropic.com>\nCo-Authored-By: 0neymik0 <0neymik0@gmail.com>\nCo-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>')"
```

---

## Task 7: Full verification + frontend-verify gate

**Files:** none (verification only).

- [ ] **Step 1: Full unit-test run**

```bash
cd frontend/web && bunx vitest run
```
Expected: green. (If unrelated pre-existing failures appear, note them — do not fix out of scope.)

- [ ] **Step 2: Run the FE pre-flight gate**

Invoke the `/frontend-verify` skill (DS-lint `ERRORS=0`, i18n en/ru/ja parity, real `bun run build`, lucide/TS2614/Tailwind-v4 traps). Fix any reported violations within this plan's files.

- [ ] **Step 3: Manual smoke (the original bug)**

Confirm, as a **logged-in** user opening an anime for the FIRST time, the player mounts on **episode 1** (not the latest aired). Then as a **returning** user (progress through ep N), it mounts on **ep N+1** with the resume chip. The `/frontend-verify` opt-in Chrome smoke covers this — run it for this change since it is a non-trivial player behavior change.

- [ ] **Step 4: Final commit (if the gate produced fixes)**

```bash
cd /data/ae-episode-selection
git add -A
git commit -m "test(web): frontend-verify fixes for episode-selection refactor

$(printf 'Co-Authored-By: Claude Code <noreply@anthropic.com>\nCo-Authored-By: 0neymik0 <0neymik0@gmail.com>\nCo-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>')"
```

Then hand off to `/animeenigma-after-update` (simplify pass, redeploy-web, changelog in Russian Trump-mode, push) per the project workflow.

---

## Self-Review

**Spec coverage:**
- §3 architecture (watchState + useWatchState, deletions) → Tasks 1, 3, 6. ✓
- §3.1 single start-episode chain → Task 5 Step 6. ✓
- §3.2 race fix (default 1, reactive, remove `episodesAired`) → Tasks 2, 4. ✓
- §4 behavior table (ep-12 fix authed+anon, manual-pick guard, banner collapse) → Tasks 1, 3, 4, 5. ✓
- §5 banner/CTA taxonomy (2 banners, 5 CTA preserved) → Task 1 (logic) + Task 5 Step 1 (render). ✓
- §6 tests/i18n/migration/risks; keep `loadedEpisodes`; anon parity → Tasks 1, 3 (D-1/D-2), 6. ✓
- §7 file inventory → File Structure table + all tasks. ✓

**Placeholder scan:** No "TBD/handle edge cases/similar to Task N". Each code step shows full code. ✓

**Type consistency:** `resolveStartEpisode`, `resolveResumeState`, `clampLastWatched`, `ResumeBanner`, `WatchCta`, `shouldReselectEpisode`, `useWatchState`/`WatchState` names are identical across Tasks 1-5. `ResumeBanner` kinds (`none`/`just-finished`/`next-unavailable`) match between `watchState.ts`, `ResumePill.vue`, and `AePlayer.resumeBannerProps`. ✓

**Note for the implementer:** the reactive re-pick (Task 4) can briefly resolve a stream for ep 1 before resume flips `initialEpisode` to `last+1` for returning users (one extra resolve, pre-play). This is correct and cheap; do not add gating complexity to avoid it.
