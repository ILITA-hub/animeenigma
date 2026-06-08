# Schedule Calendar Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the day-grouped list at `/schedule` with a calendar (Month / Week / Table views) featuring a day modal, weekly-projection fill, and filters — frontend-only, no backend changes.

**Architecture:** All scheduling logic (weekly projection, grouping, filtering, sorting, grid math) lives in pure, unit-tested functions under `src/composables/schedule/`. A thin `useScheduleCalendar` composable wires those to the live `GET /api/anime/schedule` endpoint, the watchlist store, and URL-query state. Presentational SFCs (`MonthView`, `WeekView`, `TableView`, `DayCell`, `EpisodeRow`, `DayModal`, `ScheduleFilters`) render the computed view models. `Schedule.vue` becomes the shell.

**Tech Stack:** Vue 3 `<script setup lang="ts">`, Pinia (`useWatchlistStore`), vue-router (query sync), vue-i18n, Vitest + `@vue/test-utils`, Tailwind v4 / Neon-Tokyo design tokens, `bun`/`bunx`.

**Design spec:** `docs/superpowers/specs/2026-06-08-schedule-calendar-redesign-design.md`
**Reference prototype:** `docs/superpowers/specs/2026-06-08-schedule-calendar-redesign.prototype.html` (open in a browser to see target behavior + styling)

---

## File Structure

**Create:**
- `frontend/web/src/composables/schedule/types.ts` — shared types (`ScheduleAnime`, `Occurrence`, `ScheduleFilterState`).
- `frontend/web/src/composables/schedule/projection.ts` — `projectOccurrences`, `occurrencesInRange`.
- `frontend/web/src/composables/schedule/filterSort.ts` — `applyFilters`, `sortByTime`, `sortCellHybrid`, `availableGenres`.
- `frontend/web/src/composables/schedule/calendarGrid.ts` — `startOfDay`, `isSameDay`, `weekStart`, `weekDays`, `monthGridDays`, `monthGridRange`.
- `frontend/web/src/composables/schedule/__tests__/projection.spec.ts`
- `frontend/web/src/composables/schedule/__tests__/filterSort.spec.ts`
- `frontend/web/src/composables/schedule/__tests__/calendarGrid.spec.ts`
- `frontend/web/src/composables/useScheduleCalendar.ts` — orchestrator composable.
- `frontend/web/src/components/schedule/EpisodeRow.vue` + `.spec.ts`
- `frontend/web/src/components/schedule/DayCell.vue` + `.spec.ts`
- `frontend/web/src/components/schedule/DayModal.vue` + `.spec.ts`
- `frontend/web/src/components/schedule/MonthView.vue`
- `frontend/web/src/components/schedule/WeekView.vue`
- `frontend/web/src/components/schedule/TableView.vue` + `.spec.ts`
- `frontend/web/src/components/schedule/FilterDropdown.vue`
- `frontend/web/src/components/schedule/ScheduleFilters.vue` + `.spec.ts`

**Modify:**
- `frontend/web/src/views/Schedule.vue` — rewrite as shell.
- `frontend/web/src/locales/en.json`, `ru.json`, `ja.json` — extend `schedule.*`.

**No backend changes.** All data comes from the existing `GET /api/anime/schedule` (`animeApi.getSchedule()` in `src/api/client.ts`, wrapped by `useAnime().fetchSchedule`).

---

## Task 1: Schedule types

**Files:**
- Create: `frontend/web/src/composables/schedule/types.ts`

- [ ] **Step 1: Write the types file**

```ts
// frontend/web/src/composables/schedule/types.ts

/** Genre as returned by the catalog API (subset we use). */
export interface ScheduleGenre {
  id?: string
  name?: string
  russian?: string
}

/** One ongoing anime row from GET /api/anime/schedule. */
export interface ScheduleAnime {
  id: string
  name?: string
  name_ru?: string
  name_en?: string
  name_jp?: string
  poster_url?: string
  next_episode_at?: string | null
  episodes_aired?: number
  episodes_count?: number
  score?: number
  kind?: string
  genres?: ScheduleGenre[]
}

/** A single projected episode airing on a concrete date/time. */
export interface Occurrence {
  anime: ScheduleAnime
  episode: number
  /** Local Date including the air time (hours/minutes from next_episode_at). */
  date: Date
}

export type TableSortKey = 'name' | 'date' | 'episode' | 'score'

export interface ScheduleFilterState {
  search: string
  myList: boolean
  genres: Set<string> // genre `name` values
  types: Set<string>  // `kind` values
}

export function emptyFilters(): ScheduleFilterState {
  return { search: '', myList: false, genres: new Set(), types: new Set() }
}
```

- [ ] **Step 2: Type-check passes**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: PASS (no errors referencing `schedule/types.ts`).

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/composables/schedule/types.ts
git commit -m "feat(schedule): shared calendar types"
```

---

## Task 2: Weekly projection (pure)

**Files:**
- Create: `frontend/web/src/composables/schedule/projection.ts`
- Test: `frontend/web/src/composables/schedule/__tests__/projection.spec.ts`

- [ ] **Step 1: Write failing tests**

```ts
// frontend/web/src/composables/schedule/__tests__/projection.spec.ts
import { describe, it, expect } from 'vitest'
import { projectOccurrences, occurrencesInRange } from '../projection'
import type { ScheduleAnime } from '../types'

function anime(over: Partial<ScheduleAnime> = {}): ScheduleAnime {
  return {
    id: 'a1',
    name: 'Test',
    next_episode_at: '2026-06-08T17:00:00Z',
    episodes_aired: 9,
    episodes_count: 12,
    ...over,
  }
}

// window helpers (UTC dates for determinism)
const d = (s: string) => new Date(s)

describe('projectOccurrences', () => {
  it('returns the next episode at the anchor date', () => {
    const occ = projectOccurrences(anime(), d('2026-06-08T00:00:00Z'), d('2026-06-09T00:00:00Z'))
    expect(occ).toHaveLength(1)
    expect(occ[0].episode).toBe(10) // episodes_aired + 1
    expect(occ[0].date.toISOString()).toBe('2026-06-08T17:00:00.000Z')
  })

  it('projects future weeks with incrementing episode numbers', () => {
    const occ = projectOccurrences(anime(), d('2026-06-08T00:00:00Z'), d('2026-06-23T00:00:00Z'))
    expect(occ.map(o => o.episode)).toEqual([10, 11, 12]) // 06-08, 06-15, 06-22
  })

  it('caps projection at episodes_count (no episodes past the finale)', () => {
    const occ = projectOccurrences(anime({ episodes_aired: 11, episodes_count: 12 }), d('2026-06-08T00:00:00Z'), d('2026-07-06T00:00:00Z'))
    expect(occ.map(o => o.episode)).toEqual([12]) // 06-08 only; 06-15 would be ep 13 > 12
  })

  it('back-projects past episodes within the window (ep >= 1)', () => {
    const occ = projectOccurrences(anime(), d('2026-05-25T00:00:00Z'), d('2026-06-09T00:00:00Z'))
    expect(occ.map(o => o.episode)).toEqual([8, 9, 10]) // 05-25, 06-01, 06-08
  })

  it('does not back-project below episode 1', () => {
    const occ = projectOccurrences(anime({ episodes_aired: 0 }), d('2026-05-25T00:00:00Z'), d('2026-06-09T00:00:00Z'))
    expect(occ.map(o => o.episode)).toEqual([1]) // only 06-08 (ep1); earlier weeks are ep<=0
  })

  it('treats episodes_count <= 0 as unknown (no upper cap)', () => {
    const occ = projectOccurrences(anime({ episodes_count: 0 }), d('2026-06-08T00:00:00Z'), d('2026-06-23T00:00:00Z'))
    expect(occ.map(o => o.episode)).toEqual([10, 11, 12])
  })

  it('returns [] when next_episode_at is missing or invalid', () => {
    expect(projectOccurrences(anime({ next_episode_at: null }), d('2026-06-01T00:00:00Z'), d('2026-07-01T00:00:00Z'))).toEqual([])
    expect(projectOccurrences(anime({ next_episode_at: 'not-a-date' }), d('2026-06-01T00:00:00Z'), d('2026-07-01T00:00:00Z'))).toEqual([])
  })

  it('window end is exclusive', () => {
    const occ = projectOccurrences(anime(), d('2026-06-08T00:00:00Z'), d('2026-06-08T17:00:00Z'))
    expect(occ).toHaveLength(0) // 17:00 is the end bound, excluded
  })
})

describe('occurrencesInRange', () => {
  it('flattens occurrences across all anime', () => {
    const list = [
      anime({ id: 'a', next_episode_at: '2026-06-08T17:00:00Z' }),
      anime({ id: 'b', next_episode_at: '2026-06-10T20:00:00Z' }),
    ]
    const occ = occurrencesInRange(list, d('2026-06-08T00:00:00Z'), d('2026-06-12T00:00:00Z'))
    expect(occ.map(o => o.anime.id).sort()).toEqual(['a', 'b'])
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/schedule/__tests__/projection.spec.ts`
Expected: FAIL ("Failed to resolve import ../projection" / functions not defined).

- [ ] **Step 3: Implement projection.ts**

```ts
// frontend/web/src/composables/schedule/projection.ts
import type { ScheduleAnime, Occurrence } from './types'

const WEEK_MS = 7 * 86400000

/**
 * Project an anime's weekly airings into [windowStart, windowEnd).
 * Anchor = next_episode_at (concrete next airing). Episode at anchor = episodes_aired + 1.
 * Each week k (… -1, 0, 1 …): date = anchor + k weeks, episode = episodes_aired + 1 + k.
 * Included when ep >= 1 and (episodes_count <= 0 || ep <= episodes_count).
 */
export function projectOccurrences(
  anime: ScheduleAnime,
  windowStart: Date,
  windowEnd: Date,
): Occurrence[] {
  if (!anime.next_episode_at) return []
  const anchor = new Date(anime.next_episode_at)
  const anchorMs = anchor.getTime()
  if (Number.isNaN(anchorMs)) return []

  const aired = anime.episodes_aired ?? 0
  const total = anime.episodes_count ?? 0
  const out: Occurrence[] = []

  const startMs = windowStart.getTime()
  const endMs = windowEnd.getTime()
  const kFrom = Math.floor((startMs - anchorMs) / WEEK_MS) - 1
  const kTo = Math.ceil((endMs - anchorMs) / WEEK_MS) + 1

  for (let k = kFrom; k <= kTo; k++) {
    const ms = anchorMs + k * WEEK_MS
    if (ms < startMs || ms >= endMs) continue
    const episode = aired + 1 + k
    if (episode < 1) continue
    if (total > 0 && episode > total) continue
    out.push({ anime, episode, date: new Date(ms) })
  }
  return out
}

/** Flatten projections for many anime into one window. */
export function occurrencesInRange(
  animes: ScheduleAnime[],
  windowStart: Date,
  windowEnd: Date,
): Occurrence[] {
  return animes.flatMap((a) => projectOccurrences(a, windowStart, windowEnd))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/schedule/__tests__/projection.spec.ts`
Expected: PASS (all 9 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/schedule/projection.ts frontend/web/src/composables/schedule/__tests__/projection.spec.ts
git commit -m "feat(schedule): weekly-projection of episode occurrences"
```

---

## Task 3: Calendar grid math (pure)

**Files:**
- Create: `frontend/web/src/composables/schedule/calendarGrid.ts`
- Test: `frontend/web/src/composables/schedule/__tests__/calendarGrid.spec.ts`

- [ ] **Step 1: Write failing tests**

```ts
// frontend/web/src/composables/schedule/__tests__/calendarGrid.spec.ts
import { describe, it, expect } from 'vitest'
import { startOfDay, isSameDay, weekStart, weekDays, monthGridDays, monthGridRange } from '../calendarGrid'

const iso = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`

describe('startOfDay / isSameDay', () => {
  it('zeros the time', () => {
    const s = startOfDay(new Date(2026, 5, 8, 17, 30))
    expect(s.getHours()).toBe(0)
    expect(s.getMinutes()).toBe(0)
  })
  it('isSameDay ignores time', () => {
    expect(isSameDay(new Date(2026, 5, 8, 1), new Date(2026, 5, 8, 23))).toBe(true)
    expect(isSameDay(new Date(2026, 5, 8), new Date(2026, 5, 9))).toBe(false)
  })
})

describe('weekStart (Monday-first)', () => {
  it('Monday returns itself', () => {
    expect(iso(weekStart(new Date(2026, 5, 8)))).toBe('2026-06-08') // Mon
  })
  it('Sunday returns the preceding Monday', () => {
    expect(iso(weekStart(new Date(2026, 5, 14)))).toBe('2026-06-08') // Sun 14 -> Mon 8
  })
})

describe('weekDays', () => {
  it('returns Mon..Sun (7 days)', () => {
    const days = weekDays(new Date(2026, 5, 10)) // Wed
    expect(days).toHaveLength(7)
    expect(iso(days[0])).toBe('2026-06-08')
    expect(iso(days[6])).toBe('2026-06-14')
  })
})

describe('monthGridDays', () => {
  it('June 2026 starts on Monday -> first cell is June 1', () => {
    const days = monthGridDays(new Date(2026, 5, 8))
    expect(iso(days[0])).toBe('2026-06-01')
    expect(days.length % 7).toBe(0)
  })
  it('July 2026 starts Wednesday -> grid begins on June 29 (Mon)', () => {
    const days = monthGridDays(new Date(2026, 6, 1))
    expect(iso(days[0])).toBe('2026-06-29')
    expect(days.length % 7).toBe(0)
  })
})

describe('monthGridRange', () => {
  it('start is inclusive first cell, end is exclusive day after last cell', () => {
    const { start, end } = monthGridRange(new Date(2026, 6, 1))
    expect(iso(start)).toBe('2026-06-29')
    // last cell of July grid is Sun Aug 2 -> end is Aug 3
    expect(iso(end)).toBe('2026-08-03')
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/schedule/__tests__/calendarGrid.spec.ts`
Expected: FAIL (import unresolved).

- [ ] **Step 3: Implement calendarGrid.ts**

```ts
// frontend/web/src/composables/schedule/calendarGrid.ts

export function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate())
}

export function isSameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
}

/** Monday-of-week (Monday-first calendar). */
export function weekStart(d: Date): Date {
  const off = (d.getDay() + 6) % 7 // Mon=0 … Sun=6
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() - off)
}

/** 7 dates Mon..Sun of the week containing `d`. */
export function weekDays(d: Date): Date[] {
  const s = weekStart(d)
  return Array.from({ length: 7 }, (_, i) => new Date(s.getFullYear(), s.getMonth(), s.getDate() + i))
}

/** All day cells (7 * N) for the month grid containing `viewDate`. */
export function monthGridDays(viewDate: Date): Date[] {
  const y = viewDate.getFullYear()
  const m = viewDate.getMonth()
  const first = new Date(y, m, 1)
  const lead = (first.getDay() + 6) % 7 // Mon-first offset of the 1st
  const daysInMonth = new Date(y, m + 1, 0).getDate()
  const weeks = Math.ceil((lead + daysInMonth) / 7)
  return Array.from({ length: weeks * 7 }, (_, i) => new Date(y, m, 1 - lead + i))
}

/** [start, end) covering the whole month grid (end exclusive). */
export function monthGridRange(viewDate: Date): { start: Date; end: Date } {
  const days = monthGridDays(viewDate)
  const start = startOfDay(days[0])
  const last = days[days.length - 1]
  const end = new Date(last.getFullYear(), last.getMonth(), last.getDate() + 1)
  return { start, end }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/schedule/__tests__/calendarGrid.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/schedule/calendarGrid.ts frontend/web/src/composables/schedule/__tests__/calendarGrid.spec.ts
git commit -m "feat(schedule): Monday-first calendar grid math"
```

---

## Task 4: Filter + sort (pure)

**Files:**
- Create: `frontend/web/src/composables/schedule/filterSort.ts`
- Test: `frontend/web/src/composables/schedule/__tests__/filterSort.spec.ts`

- [ ] **Step 1: Write failing tests**

```ts
// frontend/web/src/composables/schedule/__tests__/filterSort.spec.ts
import { describe, it, expect } from 'vitest'
import { applyFilters, sortByTime, sortCellHybrid, availableGenres } from '../filterSort'
import { emptyFilters, type ScheduleAnime, type Occurrence } from '../types'

const A = (o: Partial<ScheduleAnime>): ScheduleAnime => ({ id: 'x', name: 'X', ...o })

const list: ScheduleAnime[] = [
  A({ id: '1', name: 'Kaiju No. 8', name_ru: 'Кайдзю №8', kind: 'TV', genres: [{ name: 'Action' }] }),
  A({ id: '2', name: 'Frieren', name_ru: 'Фрирен', kind: 'TV', genres: [{ name: 'Fantasy' }, { name: 'Drama' }] }),
  A({ id: '3', name: 'Dandadan', name_ru: 'Данданадан', kind: 'ONA', genres: [{ name: 'Comedy' }] }),
]

describe('applyFilters', () => {
  const statusOf = (id: string) => (id === '1' ? 'watching' : id === '2' ? 'planned' : null)

  it('no filters returns all', () => {
    expect(applyFilters(list, emptyFilters(), statusOf)).toHaveLength(3)
  })
  it('search matches russian or original (case-insensitive substring)', () => {
    const f = { ...emptyFilters(), search: 'frie' }
    expect(applyFilters(list, f, statusOf).map(a => a.id)).toEqual(['2'])
    const f2 = { ...emptyFilters(), search: 'кайдзю' }
    expect(applyFilters(list, f2, statusOf).map(a => a.id)).toEqual(['1'])
  })
  it('myList keeps only watching/planned', () => {
    const f = { ...emptyFilters(), myList: true }
    expect(applyFilters(list, f, statusOf).map(a => a.id).sort()).toEqual(['1', '2'])
  })
  it('genre filter matches any selected genre', () => {
    const f = { ...emptyFilters(), genres: new Set(['Drama']) }
    expect(applyFilters(list, f, statusOf).map(a => a.id)).toEqual(['2'])
  })
  it('type filter matches kind', () => {
    const f = { ...emptyFilters(), types: new Set(['ONA']) }
    expect(applyFilters(list, f, statusOf).map(a => a.id)).toEqual(['3'])
  })
  it('combines filters (AND across dimensions)', () => {
    const f = { ...emptyFilters(), myList: true, types: new Set(['TV']) }
    expect(applyFilters(list, f, statusOf).map(a => a.id).sort()).toEqual(['1', '2'])
  })
})

describe('availableGenres', () => {
  it('returns distinct genres by name, sorted', () => {
    expect(availableGenres(list).map(g => g.name)).toEqual(['Action', 'Comedy', 'Drama', 'Fantasy'])
  })
})

describe('sorting', () => {
  const occ = (id: string, mins: number): Occurrence => ({
    anime: A({ id, name: id }),
    episode: 1,
    date: new Date(2026, 5, 13, 0, mins),
  })

  it('sortByTime is ascending by date', () => {
    const out = sortByTime([occ('late', 120), occ('early', 30)])
    expect(out.map(o => o.anime.id)).toEqual(['early', 'late'])
  })

  it('sortCellHybrid puts priority anime first, then by time', () => {
    const isPriority = (a: ScheduleAnime) => a.id === 'fav'
    const out = sortCellHybrid([occ('a', 30), occ('fav', 120), occ('b', 10)], isPriority)
    expect(out.map(o => o.anime.id)).toEqual(['fav', 'b', 'a'])
  })

  it('sortCellHybrid with no priority falls back to time', () => {
    const out = sortCellHybrid([occ('a', 30), occ('b', 10)], () => false)
    expect(out.map(o => o.anime.id)).toEqual(['b', 'a'])
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/schedule/__tests__/filterSort.spec.ts`
Expected: FAIL (import unresolved).

- [ ] **Step 3: Implement filterSort.ts**

```ts
// frontend/web/src/composables/schedule/filterSort.ts
import type { ScheduleAnime, Occurrence, ScheduleFilterState, ScheduleGenre } from './types'

type StatusResolver = (animeId: string) => string | null

const PRIORITY = new Set(['watching', 'planned', 'plan_to_watch'])

export function applyFilters(
  animes: ScheduleAnime[],
  f: ScheduleFilterState,
  statusOf: StatusResolver,
): ScheduleAnime[] {
  const q = f.search.trim().toLowerCase()
  return animes.filter((a) => {
    if (q) {
      const hay = `${a.name ?? ''} ${a.name_ru ?? ''} ${a.name_en ?? ''}`.toLowerCase()
      if (!hay.includes(q)) return false
    }
    if (f.myList && !PRIORITY.has(statusOf(a.id) ?? '')) return false
    if (f.genres.size && !(a.genres ?? []).some((g) => g.name && f.genres.has(g.name))) return false
    if (f.types.size && !(a.kind && f.types.has(a.kind))) return false
    return true
  })
}

/** Distinct genres across the dataset, keyed + sorted by `name`. */
export function availableGenres(animes: ScheduleAnime[]): ScheduleGenre[] {
  const byName = new Map<string, ScheduleGenre>()
  for (const a of animes) {
    for (const g of a.genres ?? []) {
      if (g.name && !byName.has(g.name)) byName.set(g.name, g)
    }
  }
  return [...byName.values()].sort((x, y) => (x.name ?? '').localeCompare(y.name ?? ''))
}

export function sortByTime(occ: Occurrence[]): Occurrence[] {
  return [...occ].sort((x, y) => x.date.getTime() - y.date.getTime())
}

/** Priority anime (in user's list) first, then by air time. */
export function sortCellHybrid(
  occ: Occurrence[],
  isPriority: (a: ScheduleAnime) => boolean,
): Occurrence[] {
  return [...occ].sort((x, y) => {
    const px = isPriority(x.anime) ? 0 : 1
    const py = isPriority(y.anime) ? 0 : 1
    if (px !== py) return px - py
    return x.date.getTime() - y.date.getTime()
  })
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/schedule/__tests__/filterSort.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/schedule/filterSort.ts frontend/web/src/composables/schedule/__tests__/filterSort.spec.ts
git commit -m "feat(schedule): filter + hybrid/time sort helpers"
```

---

## Task 5: `useScheduleCalendar` orchestrator composable

**Files:**
- Create: `frontend/web/src/composables/useScheduleCalendar.ts`
- Test: `frontend/web/src/composables/schedule/__tests__/useScheduleCalendar.spec.ts`

This composable holds reactive state and exposes computed view models. It accepts the fetched anime list and "now" as injectable refs so logic is testable without router/store. URL-query sync and fetching are wired in `Schedule.vue` (Task 12) via the setters this composable exposes.

- [ ] **Step 1: Write failing test**

```ts
// frontend/web/src/composables/schedule/__tests__/useScheduleCalendar.spec.ts
import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import { useScheduleCalendar } from '../../useScheduleCalendar'
import type { ScheduleAnime } from '../types'

const data: ScheduleAnime[] = [
  { id: '1', name: 'Kaiju', name_ru: 'Кайдзю', kind: 'TV', episodes_aired: 9, episodes_count: 12, next_episode_at: '2026-06-08T17:00:00Z', genres: [{ name: 'Action' }] },
  { id: '2', name: 'Frieren', name_ru: 'Фрирен', kind: 'TV', episodes_aired: 22, episodes_count: 28, next_episode_at: '2026-06-10T20:00:00Z', genres: [{ name: 'Drama' }] },
]

describe('useScheduleCalendar', () => {
  it('month view groups occurrences by day within the grid', () => {
    const cal = useScheduleCalendar({
      animes: ref(data),
      now: ref(new Date(2026, 5, 8)),
      statusOf: () => null,
      loggedIn: ref(false),
    })
    cal.setView('month')
    const cells = cal.monthCells.value
    expect(cells.length % 7).toBe(0)
    const withEps = cells.filter((c) => c.occurrences.length > 0)
    expect(withEps.length).toBeGreaterThan(0)
  })

  it('search filter narrows the dataset across all views', () => {
    const cal = useScheduleCalendar({
      animes: ref(data),
      now: ref(new Date(2026, 5, 8)),
      statusOf: () => null,
      loggedIn: ref(false),
    })
    cal.setView('table')
    cal.filters.search = 'frieren'
    const ids = new Set(cal.tableRows.value.map((r) => r.anime.id))
    expect([...ids]).toEqual(['2'])
  })

  it('table view is limited to the current week', () => {
    const cal = useScheduleCalendar({
      animes: ref(data),
      now: ref(new Date(2026, 5, 8)),
      statusOf: () => null,
      loggedIn: ref(false),
    })
    cal.setView('table')
    // all rows fall within Mon 8 .. Sun 14
    for (const r of cal.tableRows.value) {
      expect(r.date >= new Date(2026, 5, 8)).toBe(true)
      expect(r.date < new Date(2026, 5, 15)).toBe(true)
    }
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/schedule/__tests__/useScheduleCalendar.spec.ts`
Expected: FAIL (import unresolved).

- [ ] **Step 3: Implement useScheduleCalendar.ts**

```ts
// frontend/web/src/composables/useScheduleCalendar.ts
import { computed, reactive, ref, type Ref } from 'vue'
import type { ScheduleAnime, Occurrence, TableSortKey } from './schedule/types'
import { emptyFilters } from './schedule/types'
import { occurrencesInRange } from './schedule/projection'
import { applyFilters, availableGenres, sortByTime, sortCellHybrid } from './schedule/filterSort'
import { monthGridDays, monthGridRange, weekDays, weekStart, startOfDay, isSameDay } from './schedule/calendarGrid'

export type ScheduleView = 'month' | 'week' | 'table'

export interface DayCellModel {
  date: Date
  inCurrentMonth: boolean
  isToday: boolean
  occurrences: Occurrence[]
}

export interface UseScheduleCalendarOptions {
  animes: Ref<ScheduleAnime[]>
  now: Ref<Date>
  statusOf: (animeId: string) => string | null
  loggedIn: Ref<boolean>
}

const WEEK_MS = 7 * 86400000

export function useScheduleCalendar(opts: UseScheduleCalendarOptions) {
  const view = ref<ScheduleView>('month')
  const viewDate = ref<Date>(startOfDay(opts.now.value))
  const filters = reactive(emptyFilters())
  const sortKey = ref<TableSortKey>('date')
  const sortDir = ref<1 | -1>(1)

  const isPriority = (a: ScheduleAnime) =>
    opts.loggedIn.value && ['watching', 'planned', 'plan_to_watch'].includes(opts.statusOf(a.id) ?? '')

  const filteredAnimes = computed(() => applyFilters(opts.animes.value, filters, opts.statusOf))
  const genres = computed(() => availableGenres(opts.animes.value))

  const monthCells = computed<DayCellModel[]>(() => {
    const days = monthGridDays(viewDate.value)
    const { start, end } = monthGridRange(viewDate.value)
    const all = occurrencesInRange(filteredAnimes.value, start, end)
    const m = viewDate.value.getMonth()
    return days.map((date) => {
      const inMonth = date.getMonth() === m
      const occ = inMonth ? all.filter((o) => isSameDay(o.date, date)) : []
      return {
        date,
        inCurrentMonth: inMonth,
        isToday: isSameDay(date, opts.now.value),
        occurrences: sortCellHybrid(occ, isPriority),
      }
    })
  })

  const weekColumns = computed<DayCellModel[]>(() => {
    const days = weekDays(viewDate.value)
    const start = startOfDay(days[0])
    const end = new Date(start.getTime() + WEEK_MS)
    const all = occurrencesInRange(filteredAnimes.value, start, end)
    return days.map((date) => ({
      date,
      inCurrentMonth: true,
      isToday: isSameDay(date, opts.now.value),
      occurrences: sortByTime(all.filter((o) => isSameDay(o.date, date))),
    }))
  })

  const tableRows = computed<Occurrence[]>(() => {
    const days = weekDays(viewDate.value)
    const start = startOfDay(days[0])
    const end = new Date(start.getTime() + WEEK_MS)
    const all = occurrencesInRange(filteredAnimes.value, start, end)
    const cmp: Record<TableSortKey, (x: Occurrence, y: Occurrence) => number> = {
      name: (x, y) => (x.anime.name ?? '').localeCompare(y.anime.name ?? ''),
      date: (x, y) => x.date.getTime() - y.date.getTime(),
      episode: (x, y) => x.episode - y.episode,
      score: (x, y) => (x.anime.score ?? 0) - (y.anime.score ?? 0),
    }
    return [...all].sort((x, y) => cmp[sortKey.value](x, y) * sortDir.value)
  })

  function setView(v: ScheduleView) { view.value = v }
  function shift(dir: -1 | 1) {
    if (view.value === 'week' || view.value === 'table') {
      const s = weekStart(viewDate.value)
      viewDate.value = new Date(s.getTime() + dir * WEEK_MS)
    } else {
      viewDate.value = new Date(viewDate.value.getFullYear(), viewDate.value.getMonth() + dir, 1)
    }
  }
  function goToday() { viewDate.value = startOfDay(opts.now.value) }
  function setSort(key: TableSortKey) {
    if (sortKey.value === key) sortDir.value = (sortDir.value === 1 ? -1 : 1)
    else { sortKey.value = key; sortDir.value = 1 }
  }
  function resetFilters() {
    filters.search = ''
    filters.myList = false
    filters.genres = new Set()
    filters.types = new Set()
  }

  const activeFilterCount = computed(() =>
    (filters.search ? 1 : 0) + (filters.myList ? 1 : 0) + filters.genres.size + filters.types.size)

  return {
    view, viewDate, filters, sortKey, sortDir,
    filteredAnimes, genres, monthCells, weekColumns, tableRows, activeFilterCount,
    setView, shift, goToday, setSort, resetFilters,
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/schedule/__tests__/useScheduleCalendar.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/useScheduleCalendar.ts frontend/web/src/composables/schedule/__tests__/useScheduleCalendar.spec.ts
git commit -m "feat(schedule): useScheduleCalendar orchestrator composable"
```

---

## Task 6: `EpisodeRow.vue` (shared row block)

**Files:**
- Create: `frontend/web/src/components/schedule/EpisodeRow.vue`
- Test: `frontend/web/src/components/schedule/EpisodeRow.spec.ts`

- [ ] **Step 1: Write failing test**

```ts
// frontend/web/src/components/schedule/EpisodeRow.spec.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import EpisodeRow from './EpisodeRow.vue'

const occ = {
  anime: { id: '1', name: 'Kaiju No. 8', name_ru: 'Кайдзю №8', poster_url: '/p.jpg' },
  episode: 10,
  date: new Date(2026, 5, 8, 17, 0),
}

describe('EpisodeRow', () => {
  it('renders the localized title, episode number and time', () => {
    const w = mount(EpisodeRow, { props: { occurrence: occ } })
    expect(w.text()).toContain('Кайдзю №8')
    expect(w.text()).toContain('10')
    expect(w.text()).toContain('17:00')
  })
  it('renders the poster with alt text', () => {
    const w = mount(EpisodeRow, { props: { occurrence: occ } })
    const img = w.get('img')
    expect(img.attributes('src')).toBe('/p.jpg')
    expect(img.attributes('alt')).toBe('Кайдзю №8')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/schedule/EpisodeRow.spec.ts`
Expected: FAIL (component missing).

- [ ] **Step 3: Implement EpisodeRow.vue**

```vue
<!-- frontend/web/src/components/schedule/EpisodeRow.vue -->
<template>
  <div class="flex items-center gap-2 py-1.5 border-t border-white/5 first:border-t-0 first:pt-0">
    <img
      :src="occurrence.anime.poster_url || '/placeholder.svg'"
      :alt="title"
      class="w-7 h-10 rounded object-cover flex-none bg-muted"
      loading="lazy"
    />
    <div class="min-w-0 flex-1">
      <div class="text-[10px] leading-tight font-medium text-foreground line-clamp-2">{{ title }}</div>
      <div class="text-[9px] text-muted-foreground mt-0.5">
        <span class="text-primary font-semibold">{{ occurrence.episode }}</span>
        {{ $t('schedule.episodeShort') }} · {{ time }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Occurrence } from '@/composables/schedule/types'
import { getLocalizedTitle } from '@/utils/title'
import { formatAirTime } from '@/composables/schedule/format'

const props = defineProps<{ occurrence: Occurrence }>()
const title = computed(() => getLocalizedTitle(props.occurrence.anime.name, props.occurrence.anime.name_ru, props.occurrence.anime.name_jp))
const time = computed(() => formatAirTime(props.occurrence.date))
</script>
```

- [ ] **Step 4: Create the time formatter referenced above**

```ts
// frontend/web/src/composables/schedule/format.ts
/** HH:MM in Europe/Moscow (project standard, mirrors the old Schedule.vue). */
export function formatAirTime(date: Date): string {
  return date.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Moscow' })
}
```

> Note: tests run in jsdom with the project's timezone; `formatAirTime` pins `Europe/Moscow` so the rendered value is deterministic. The `17:00` assertion in Step 1 assumes the input `new Date(2026, 5, 8, 17, 0)` is constructed in the test runner's local tz. If the runner tz is not MSK, change the test input to `new Date('2026-06-08T17:00:00+03:00')` and keep the assertion `17:00`.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/schedule/EpisodeRow.spec.ts`
Expected: PASS. (If the time assertion fails due to runner tz, apply the Step 4 note fix, then re-run.)

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/schedule/EpisodeRow.vue frontend/web/src/composables/schedule/format.ts frontend/web/src/components/schedule/EpisodeRow.spec.ts
git commit -m "feat(schedule): EpisodeRow shared block + air-time formatter"
```

---

## Task 7: `DayCell.vue` (month cell)

**Files:**
- Create: `frontend/web/src/components/schedule/DayCell.vue`
- Test: `frontend/web/src/components/schedule/DayCell.spec.ts`

- [ ] **Step 1: Write failing test**

```ts
// frontend/web/src/components/schedule/DayCell.spec.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DayCell from './DayCell.vue'
import type { DayCellModel } from '@/composables/useScheduleCalendar'

const a = (id: string) => ({ id, name: 'N' + id, poster_url: '/p.jpg' })
function model(over: Partial<DayCellModel> = {}): DayCellModel {
  return {
    date: new Date(2026, 5, 13),
    inCurrentMonth: true,
    isToday: false,
    occurrences: [],
    ...over,
  }
}

describe('DayCell', () => {
  it('renders up to 3 episode rows', () => {
    const occ = [1, 2, 3, 4, 5].map((n) => ({ anime: a(String(n)), episode: n, date: new Date(2026, 5, 13, n) }))
    const w = mount(DayCell, { props: { cell: model({ occurrences: occ }) } })
    expect(w.findAllComponents({ name: 'EpisodeRow' }).length).toBe(3)
  })
  it('shows "+N more" when there are more than 3', () => {
    const occ = [1, 2, 3, 4, 5].map((n) => ({ anime: a(String(n)), episode: n, date: new Date(2026, 5, 13, n) }))
    const w = mount(DayCell, { props: { cell: model({ occurrences: occ }) } })
    expect(w.text()).toContain('2') // +2
  })
  it('emits open with the date when a cell with episodes is clicked', async () => {
    const occ = [{ anime: a('1'), episode: 1, date: new Date(2026, 5, 13, 1) }]
    const w = mount(DayCell, { props: { cell: model({ occurrences: occ }) } })
    await w.trigger('click')
    expect(w.emitted('open')).toBeTruthy()
    expect((w.emitted('open') as unknown[][])[0][0]).toBeInstanceOf(Date)
  })
  it('does not emit open for empty cells', async () => {
    const w = mount(DayCell, { props: { cell: model({ occurrences: [] }) } })
    await w.trigger('click')
    expect(w.emitted('open')).toBeFalsy()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/schedule/DayCell.spec.ts`
Expected: FAIL (component missing).

- [ ] **Step 3: Implement DayCell.vue**

```vue
<!-- frontend/web/src/components/schedule/DayCell.vue -->
<template>
  <div
    class="relative min-h-[150px] rounded-xl border border-white/[0.06] bg-white/[0.025] p-2.5 overflow-hidden transition-colors"
    :class="[
      cell.inCurrentMonth ? (hasEpisodes ? 'cursor-pointer hover:bg-white/[0.045] hover:border-white/12' : '') : 'bg-transparent border-white/[0.03]',
      cell.isToday ? 'today-bar' : '',
    ]"
    @click="onClick"
  >
    <div
      class="text-xs mb-1.5 font-display"
      :class="cell.isToday ? 'text-primary font-bold' : cell.inCurrentMonth ? 'text-muted-foreground' : 'text-white/20'"
    >
      {{ cell.date.getDate() }}
    </div>
    <EpisodeRow v-for="o in visible" :key="o.anime.id + ':' + o.episode" :occurrence="o" />
    <div v-if="overflow > 0" class="text-[10px] text-muted-foreground mt-1.5">
      {{ $t('schedule.more', { n: overflow }) }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import EpisodeRow from './EpisodeRow.vue'
import type { DayCellModel } from '@/composables/useScheduleCalendar'

const props = defineProps<{ cell: DayCellModel }>()
const emit = defineEmits<{ open: [date: Date] }>()

const hasEpisodes = computed(() => props.cell.inCurrentMonth && props.cell.occurrences.length > 0)
const visible = computed(() => props.cell.occurrences.slice(0, 3))
const overflow = computed(() => Math.max(0, props.cell.occurrences.length - 3))

function onClick() {
  if (hasEpisodes.value) emit('open', props.cell.date)
}
</script>

<style scoped>
.today-bar::before {
  content: '';
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 3px;
  background: linear-gradient(90deg, var(--brand-cyan), var(--brand-violet));
}
</style>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/schedule/DayCell.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/schedule/DayCell.vue frontend/web/src/components/schedule/DayCell.spec.ts
git commit -m "feat(schedule): DayCell month cell (3 rows + +N more)"
```

---

## Task 8: `DayModal.vue`

**Files:**
- Create: `frontend/web/src/components/schedule/DayModal.vue`
- Test: `frontend/web/src/components/schedule/DayModal.spec.ts`

Uses the existing `Modal.vue` (`modelValue` + `title`). Each episode card links to `/anime/{id}` via `<router-link>`.

- [ ] **Step 1: Write failing test**

```ts
// frontend/web/src/components/schedule/DayModal.spec.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { RouterLinkStub } from '@vue/test-utils'
import DayModal from './DayModal.vue'

const occ = [
  { anime: { id: '1', name: 'Kaiju', name_ru: 'Кайдзю', poster_url: '/p.jpg', score: 8.4 }, episode: 10, date: new Date(2026, 5, 13, 17, 0) },
  { anime: { id: '2', name: 'Frieren', name_ru: 'Фрирен', poster_url: '/q.jpg', score: 9.3 }, episode: 23, date: new Date(2026, 5, 13, 22, 0) },
]

function mountModal() {
  return mount(DayModal, {
    props: { modelValue: true, date: new Date(2026, 5, 13), occurrences: occ },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('DayModal', () => {
  it('renders one card per occurrence with a link to the anime', () => {
    const w = mountModal()
    const links = w.findAllComponents(RouterLinkStub)
    expect(links.length).toBe(2)
    expect(links[0].props('to')).toBe('/anime/1')
  })
  it('shows episode numbers and titles', () => {
    const w = mountModal()
    expect(w.text()).toContain('Кайдзю')
    expect(w.text()).toContain('10')
    expect(w.text()).toContain('Фрирен')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/schedule/DayModal.spec.ts`
Expected: FAIL (component missing).

- [ ] **Step 3: Implement DayModal.vue**

```vue
<!-- frontend/web/src/components/schedule/DayModal.vue -->
<template>
  <Modal :model-value="modelValue" :title="headerTitle" size="md" @update:model-value="$emit('update:modelValue', $event)">
    <p class="text-xs text-primary -mt-2 mb-3">{{ subtitle }}</p>
    <div class="space-y-2.5">
      <router-link
        v-for="o in sorted"
        :key="o.anime.id + ':' + o.episode"
        :to="`/anime/${o.anime.id}`"
        class="flex items-center gap-3 rounded-xl border border-white/[0.06] bg-white/[0.045] p-2.5 hover:bg-white/[0.08] transition-colors"
      >
        <img :src="o.anime.poster_url || '/placeholder.svg'" :alt="titleOf(o)" class="w-[54px] h-[76px] rounded-lg object-cover flex-none bg-muted" />
        <div class="min-w-0 flex-1">
          <div class="text-sm font-semibold text-foreground line-clamp-2">{{ titleOf(o) }}</div>
          <div class="text-xs text-primary mt-1">{{ $t('schedule.episode', { n: o.episode }) }}</div>
          <div class="text-[11px] text-muted-foreground mt-0.5">{{ formatAirTime(o.date) }} {{ $t('schedule.mskSuffix') }} · ★ {{ (o.anime.score ?? 0).toFixed(1) }}</div>
        </div>
        <Button variant="default" size="sm" class="ml-auto flex-none" tabindex="-1">{{ $t('schedule.watch') }}</Button>
      </router-link>
    </div>
  </Modal>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Modal from '@/components/ui/Modal.vue'
import Button from '@/components/ui/Button.vue'
import type { Occurrence } from '@/composables/schedule/types'
import { getLocalizedTitle } from '@/utils/title'
import { formatAirTime } from '@/composables/schedule/format'
import { sortByTime } from '@/composables/schedule/filterSort'
import { formatDayTitle } from '@/composables/schedule/format'

const props = defineProps<{ modelValue: boolean; date: Date | null; occurrences: Occurrence[] }>()
defineEmits<{ 'update:modelValue': [value: boolean] }>()
const { t } = useI18n()

const sorted = computed(() => sortByTime(props.occurrences))
const headerTitle = computed(() => (props.date ? formatDayTitle(props.date, t) : ''))
const subtitle = computed(() => t('schedule.episodeCountPlural', props.occurrences.length, { n: props.occurrences.length }))
const titleOf = (o: Occurrence) => getLocalizedTitle(o.anime.name, o.anime.name_ru, o.anime.name_jp)
</script>
```

- [ ] **Step 4: Add `formatDayTitle` to format.ts**

```ts
// append to frontend/web/src/composables/schedule/format.ts
type T = (key: string, named?: Record<string, unknown>) => string

/** e.g. "Суббота, 13 июня" — weekday + day + genitive month from i18n. */
export function formatDayTitle(date: Date, t: T): string {
  const dowIdx = (date.getDay() + 6) % 7 // Mon=0
  const dowKeys = ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday']
  const monKeys = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec']
  const weekday = t(`schedule.days.${dowKeys[dowIdx]}`)
  const month = t(`schedule.monthsGenitive.${monKeys[date.getMonth()]}`)
  return `${weekday}, ${date.getDate()} ${month}`
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/schedule/DayModal.spec.ts`
Expected: PASS. (i18n is auto-stubbed by global config; if `$t` is undefined in mount, add `global.mocks: { $t: (k: string) => k }` to `mountModal`.)

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/schedule/DayModal.vue frontend/web/src/composables/schedule/format.ts frontend/web/src/components/schedule/DayModal.spec.ts
git commit -m "feat(schedule): DayModal (variant C) with per-episode anime links"
```

---

## Task 9: `MonthView.vue` + `WeekView.vue`

**Files:**
- Create: `frontend/web/src/components/schedule/MonthView.vue`
- Create: `frontend/web/src/components/schedule/WeekView.vue`

These are thin layout wrappers; they render models from the composable and re-emit `open(date)` upward. No new logic → covered by `DayCell` + `useScheduleCalendar` tests; a render smoke is added in Task 13's view test.

- [ ] **Step 1: Implement MonthView.vue**

```vue
<!-- frontend/web/src/components/schedule/MonthView.vue -->
<template>
  <div>
    <div class="grid grid-cols-7 gap-1.5 mb-1.5">
      <div v-for="d in dows" :key="d" class="text-center text-[10px] uppercase tracking-wide text-muted-foreground">{{ d }}</div>
    </div>
    <div class="grid grid-cols-7 gap-1.5">
      <DayCell v-for="(c, i) in cells" :key="i" :cell="c" @open="$emit('open', $event)" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DayCell from './DayCell.vue'
import type { DayCellModel } from '@/composables/useScheduleCalendar'

defineProps<{ cells: DayCellModel[] }>()
defineEmits<{ open: [date: Date] }>()
const { t } = useI18n()
const dows = computed(() => ['monday','tuesday','wednesday','thursday','friday','saturday','sunday'].map(k => t(`schedule.daysShort.${k}`)))
</script>
```

- [ ] **Step 2: Implement WeekView.vue**

```vue
<!-- frontend/web/src/components/schedule/WeekView.vue -->
<template>
  <div class="grid grid-cols-7 gap-1.5">
    <div
      v-for="(c, i) in columns"
      :key="i"
      class="relative min-h-[220px] rounded-xl border border-white/[0.06] bg-white/[0.025] p-2.5 overflow-hidden transition-colors"
      :class="[c.occurrences.length ? 'cursor-pointer hover:bg-white/[0.045] hover:border-white/12' : '', c.isToday ? 'today-bar' : '']"
      @click="c.occurrences.length && $emit('open', c.date)"
    >
      <div class="text-center pb-2 mb-2 border-b border-white/5">
        <div class="text-[10px] uppercase tracking-wide" :class="c.isToday ? 'text-primary' : 'text-muted-foreground'">{{ dows[i] }}</div>
        <div class="text-base font-semibold font-display" :class="c.isToday ? 'text-primary' : 'text-foreground'">{{ c.date.getDate() }}</div>
      </div>
      <div v-if="!c.occurrences.length" class="text-[10px] text-white/20 text-center pt-3">—</div>
      <EpisodeRow v-for="o in c.occurrences" :key="o.anime.id + ':' + o.episode" :occurrence="o" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import EpisodeRow from './EpisodeRow.vue'
import type { DayCellModel } from '@/composables/useScheduleCalendar'

defineProps<{ columns: DayCellModel[] }>()
defineEmits<{ open: [date: Date] }>()
const { t } = useI18n()
const dows = computed(() => ['monday','tuesday','wednesday','thursday','friday','saturday','sunday'].map(k => t(`schedule.daysShort.${k}`)))
</script>

<style scoped>
.today-bar::before {
  content: '';
  position: absolute; top: 0; left: 0; right: 0; height: 3px;
  background: linear-gradient(90deg, var(--brand-cyan), var(--brand-violet));
}
</style>
```

- [ ] **Step 3: Type-check passes**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/components/schedule/MonthView.vue frontend/web/src/components/schedule/WeekView.vue
git commit -m "feat(schedule): MonthView + WeekView layout wrappers"
```

---

## Task 10: `TableView.vue`

**Files:**
- Create: `frontend/web/src/components/schedule/TableView.vue`
- Test: `frontend/web/src/components/schedule/TableView.spec.ts`

- [ ] **Step 1: Write failing test**

```ts
// frontend/web/src/components/schedule/TableView.spec.ts
import { describe, it, expect } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import TableView from './TableView.vue'

const rows = [
  { anime: { id: '1', name: 'Kaiju', name_ru: 'Кайдзю', poster_url: '/p.jpg', score: 8.4, episodes_count: 12 }, episode: 10, date: new Date(2026, 5, 8, 17, 0) },
  { anime: { id: '2', name: 'Frieren', name_ru: 'Фрирен', poster_url: '/q.jpg', score: 9.3, episodes_count: 28 }, episode: 23, date: new Date(2026, 5, 10, 22, 0) },
]

function mountTable() {
  return mount(TableView, {
    props: { rows, sortKey: 'date', sortDir: 1 },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('TableView', () => {
  it('renders a row per occurrence', () => {
    const w = mountTable()
    expect(w.findAll('tbody tr.dt-row').length).toBe(2)
  })
  it('emits sort when a header is clicked', async () => {
    const w = mountTable()
    await w.get('[data-sort="score"]').trigger('click')
    expect(w.emitted('sort')).toBeTruthy()
    expect((w.emitted('sort') as unknown[][])[0][0]).toBe('score')
  })
  it('shows day-group separators when sorted by date', () => {
    const w = mountTable()
    expect(w.findAll('tr.group-row').length).toBe(2) // two distinct days
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/schedule/TableView.spec.ts`
Expected: FAIL (component missing).

- [ ] **Step 3: Implement TableView.vue**

```vue
<!-- frontend/web/src/components/schedule/TableView.vue -->
<template>
  <table class="w-full border-collapse">
    <thead>
      <tr>
        <th class="text-left text-[11px] uppercase tracking-wide text-muted-foreground p-3 border-b border-white/10 cursor-pointer w-[42%]" data-sort="name" @click="$emit('sort', 'name')">
          {{ $t('schedule.col.anime') }}<span v-if="sortKey==='name'" class="text-primary text-[10px]">{{ arrow }}</span>
        </th>
        <th class="text-left text-[11px] uppercase tracking-wide text-muted-foreground p-3 border-b border-white/10 cursor-pointer whitespace-nowrap" data-sort="date" @click="$emit('sort', 'date')">
          {{ $t('schedule.col.datetime') }}<span v-if="sortKey==='date'" class="text-primary text-[10px]">{{ arrow }}</span>
        </th>
        <th class="text-left text-[11px] uppercase tracking-wide text-muted-foreground p-3 border-b border-white/10 cursor-pointer" data-sort="episode" @click="$emit('sort', 'episode')">
          {{ $t('schedule.col.episode') }}<span v-if="sortKey==='episode'" class="text-primary text-[10px]">{{ arrow }}</span>
        </th>
        <th class="hidden md:table-cell text-left text-[11px] uppercase tracking-wide text-muted-foreground p-3 border-b border-white/10 cursor-pointer" data-sort="score" @click="$emit('sort', 'score')">
          {{ $t('schedule.col.score') }}<span v-if="sortKey==='score'" class="text-primary text-[10px]">{{ arrow }}</span>
        </th>
        <th class="hidden md:table-cell p-3 border-b border-white/10"></th>
      </tr>
    </thead>
    <tbody>
      <template v-for="(r, i) in rows" :key="r.anime.id + ':' + r.episode">
        <tr v-if="sortKey==='date' && isNewDay(i)" class="group-row">
          <td colspan="5" class="bg-white/[0.02] text-[11px] uppercase tracking-wide text-muted-foreground px-3 py-1.5">{{ dayLabel(r.date) }}</td>
        </tr>
        <tr class="dt-row border-b border-white/5 hover:bg-white/[0.04] cursor-pointer" @click="go(r.anime.id)">
          <td class="p-3">
            <div class="flex items-center gap-2.5">
              <img :src="r.anime.poster_url || '/placeholder.svg'" :alt="titleOf(r)" class="w-[30px] h-10 rounded object-cover flex-none bg-muted" />
              <div>
                <div class="font-semibold text-sm">{{ titleOf(r) }}</div>
                <div class="text-[11px] text-muted-foreground">{{ r.anime.name }}</div>
              </div>
            </div>
          </td>
          <td class="p-3 whitespace-nowrap text-sm">
            <span class="text-muted-foreground text-[11px]">{{ dowShort(r.date) }}</span>
            {{ r.date.getDate() }} {{ monthGen(r.date) }} · <span class="text-primary tabular-nums">{{ time(r.date) }}</span>
          </td>
          <td class="p-3 tabular-nums">{{ r.episode }}<span v-if="(r.anime.episodes_count ?? 0) > 0" class="text-muted-foreground"> / {{ r.anime.episodes_count }}</span></td>
          <td class="hidden md:table-cell p-3 tabular-nums"><span class="text-warning">★</span> {{ (r.anime.score ?? 0).toFixed(1) }}</td>
          <td class="hidden md:table-cell p-3">
            <RouterLink :to="`/anime/${r.anime.id}`" class="inline-block" @click.stop>
              <Button variant="default" size="sm" tabindex="-1">{{ $t('schedule.watch') }}</Button>
            </RouterLink>
          </td>
        </tr>
      </template>
    </tbody>
  </table>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from '@/components/ui/Button.vue'
import type { Occurrence, TableSortKey } from '@/composables/schedule/types'
import { getLocalizedTitle } from '@/utils/title'
import { formatAirTime, formatDayTitle } from '@/composables/schedule/format'
import { isSameDay } from '@/composables/schedule/calendarGrid'

const props = defineProps<{ rows: Occurrence[]; sortKey: TableSortKey; sortDir: 1 | -1 }>()
defineEmits<{ sort: [key: TableSortKey] }>()
const router = useRouter()
const { t } = useI18n()

const arrow = computed(() => (props.sortDir === 1 ? '▲' : '▼'))
const titleOf = (r: Occurrence) => getLocalizedTitle(r.anime.name, r.anime.name_ru, r.anime.name_jp)
const time = (d: Date) => formatAirTime(d)
const monthGen = (d: Date) => t(`schedule.monthsGenitive.${['jan','feb','mar','apr','may','jun','jul','aug','sep','oct','nov','dec'][d.getMonth()]}`)
const dowShort = (d: Date) => t(`schedule.daysShort.${['monday','tuesday','wednesday','thursday','friday','saturday','sunday'][(d.getDay()+6)%7]}`)
const dayLabel = (d: Date) => formatDayTitle(d, t)
const isNewDay = (i: number) => i === 0 || !isSameDay(props.rows[i].date, props.rows[i - 1].date)
const go = (id: string) => router.push(`/anime/${id}`)
</script>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/schedule/TableView.spec.ts`
Expected: PASS. (Add `global.mocks: { $t: (k: string) => k }` and a router stub if the runner doesn't auto-provide them — mirror the pattern used in existing `src/components/**/**.spec.ts`.)

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/schedule/TableView.vue frontend/web/src/components/schedule/TableView.spec.ts
git commit -m "feat(schedule): TableView (week-scoped, sortable, day groups)"
```

---

## Task 11: Filters — `FilterDropdown.vue` + `ScheduleFilters.vue`

**Files:**
- Create: `frontend/web/src/components/schedule/FilterDropdown.vue`
- Create: `frontend/web/src/components/schedule/ScheduleFilters.vue`
- Test: `frontend/web/src/components/schedule/ScheduleFilters.spec.ts`

- [ ] **Step 1: Implement FilterDropdown.vue (self-contained multiselect popover)**

```vue
<!-- frontend/web/src/components/schedule/FilterDropdown.vue -->
<template>
  <div ref="root" class="relative">
    <button
      type="button"
      class="flex items-center gap-1.5 text-xs rounded-lg border px-2.5 py-1.5 whitespace-nowrap transition-colors"
      :class="selected.size ? 'bg-primary/15 border-primary/50 text-primary' : 'bg-white/[0.06] border-white/10 text-foreground/80 hover:bg-white/10'"
      @click.stop="open = !open"
    >
      {{ label }}<span class="opacity-50 text-[10px]">▾</span>
    </button>
    <div v-if="open" class="absolute top-[calc(100%+6px)] left-0 z-30 w-52 rounded-xl border border-white/14 bg-elevated p-1.5 shadow-2xl">
      <button
        v-for="opt in options"
        :key="opt.value"
        type="button"
        class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-sm text-left hover:bg-white/5"
        @click.stop="toggle(opt.value)"
      >
        <span class="w-[15px] h-[15px] rounded border flex items-center justify-center text-[10px] flex-none"
          :class="selected.has(opt.value) ? 'bg-primary border-primary text-primary-foreground' : 'border-white/30'">
          {{ selected.has(opt.value) ? '✓' : '' }}
        </span>
        {{ opt.label }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'

const props = defineProps<{ label: string; options: { value: string; label: string }[]; selected: Set<string> }>()
const emit = defineEmits<{ toggle: [value: string] }>()
const open = ref(false)
const root = ref<HTMLElement | null>(null)

function toggle(v: string) { emit('toggle', v) }
function onDocClick(e: MouseEvent) { if (root.value && !root.value.contains(e.target as Node)) open.value = false }
onMounted(() => document.addEventListener('click', onDocClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))
</script>
```

- [ ] **Step 2: Implement ScheduleFilters.vue**

```vue
<!-- frontend/web/src/components/schedule/ScheduleFilters.vue -->
<template>
  <div>
    <div class="flex items-center gap-2 flex-wrap p-2.5 rounded-xl border border-white/[0.06] bg-white/[0.025] mb-2.5">
      <div class="flex-1 min-w-[160px] flex items-center gap-2 rounded-lg bg-white/[0.06] px-2.5 py-1.5">
        <span class="opacity-50">🔍</span>
        <input
          :value="filters.search"
          :placeholder="$t('schedule.searchPlaceholder')"
          class="bg-transparent border-0 outline-none text-foreground text-sm w-full placeholder:text-muted-foreground"
          @input="filters.search = ($event.target as HTMLInputElement).value"
        />
      </div>
      <button
        v-if="loggedIn"
        type="button"
        class="flex items-center gap-1.5 text-xs rounded-lg border px-2.5 py-1.5 transition-colors"
        :class="filters.myList ? 'bg-primary/15 border-primary/50 text-primary' : 'bg-white/[0.06] border-white/10 text-foreground/80 hover:bg-white/10'"
        @click="filters.myList = !filters.myList"
      >★ {{ $t('schedule.myList') }}</button>
      <FilterDropdown :label="$t('schedule.genre')" :options="genreOptions" :selected="filters.genres" @toggle="toggleSet(filters.genres, $event)" />
      <FilterDropdown :label="$t('schedule.type')" :options="typeOptions" :selected="filters.types" @toggle="toggleSet(filters.types, $event)" />
    </div>

    <div class="flex items-center gap-2 flex-wrap mb-3 min-h-6">
      <template v-if="activeChips.length">
        <span class="text-[11px] text-muted-foreground">{{ $t('schedule.activeFilters') }}</span>
        <span v-for="chip in activeChips" :key="chip.key" class="flex items-center gap-1.5 text-xs text-primary bg-primary/15 border border-primary/35 rounded-full px-2.5 py-1">
          {{ chip.label }}<span class="cursor-pointer opacity-70" @click="chip.remove()">✕</span>
        </span>
        <span class="text-xs text-muted-foreground underline cursor-pointer" @click="$emit('reset')">{{ $t('schedule.resetAll') }}</span>
      </template>
      <span v-else class="text-[11px] text-muted-foreground">{{ $t('schedule.noFilters') }}</span>
      <span class="text-[11px] text-white/35 ml-auto">{{ $t('schedule.countOf', { n: matchCount, total }) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import FilterDropdown from './FilterDropdown.vue'
import type { ScheduleFilterState, ScheduleGenre } from '@/composables/schedule/types'

const props = defineProps<{
  filters: ScheduleFilterState
  genres: ScheduleGenre[]
  loggedIn: boolean
  matchCount: number
  total: number
}>()
defineEmits<{ reset: [] }>()
const { t, locale } = useI18n()

const TYPES = ['TV', 'ONA', 'Movie', 'OVA']
const genreOptions = computed(() => props.genres.filter(g => g.name).map(g => ({ value: g.name as string, label: (locale.value === 'ru' && g.russian) ? g.russian : (g.name as string) })))
const typeOptions = computed(() => TYPES.map(v => ({ value: v, label: v })))

function toggleSet(set: Set<string>, v: string) { set.has(v) ? set.delete(v) : set.add(v) }

const activeChips = computed(() => {
  const chips: { key: string; label: string; remove: () => void }[] = []
  if (props.filters.search) chips.push({ key: 'q', label: `${t('schedule.searchChip')}: ${props.filters.search}`, remove: () => (props.filters.search = '') })
  if (props.filters.myList) chips.push({ key: 'mine', label: `★ ${t('schedule.myList')}`, remove: () => (props.filters.myList = false) })
  props.filters.genres.forEach(g => chips.push({ key: 'g:' + g, label: genreOptions.value.find(o => o.value === g)?.label ?? g, remove: () => props.filters.genres.delete(g) }))
  props.filters.types.forEach(ty => chips.push({ key: 't:' + ty, label: ty, remove: () => props.filters.types.delete(ty) }))
  return chips
})
</script>
```

- [ ] **Step 3: Write test for ScheduleFilters**

```ts
// frontend/web/src/components/schedule/ScheduleFilters.spec.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive } from 'vue'
import ScheduleFilters from './ScheduleFilters.vue'
import { emptyFilters } from '@/composables/schedule/types'

function mountFilters(over = {}) {
  const filters = reactive(emptyFilters())
  return {
    filters,
    w: mount(ScheduleFilters, {
      props: { filters, genres: [{ name: 'Action', russian: 'Экшен' }], loggedIn: true, matchCount: 5, total: 12, ...over },
      global: { mocks: { $t: (k: string, n?: Record<string, unknown>) => (n ? `${k}:${JSON.stringify(n)}` : k) } },
    }),
  }
}

describe('ScheduleFilters', () => {
  it('typing in search updates the filter', async () => {
    const { filters, w } = mountFilters()
    await w.get('input').setValue('frieren')
    expect(filters.search).toBe('frieren')
  })
  it('clicking My List toggles myList', async () => {
    const { filters, w } = mountFilters()
    await w.get('button').trigger('click') // first button is the My List toggle
    expect(filters.myList).toBe(true)
  })
  it('hides My List for guests', () => {
    const { w } = mountFilters({ loggedIn: false })
    expect(w.text()).not.toContain('myList')
  })
})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/schedule/ScheduleFilters.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/schedule/FilterDropdown.vue frontend/web/src/components/schedule/ScheduleFilters.vue frontend/web/src/components/schedule/ScheduleFilters.spec.ts
git commit -m "feat(schedule): filter bar (search / my-list / genre / type) + chips"
```

---

## Task 12: i18n keys (en / ru / ja) + parity

**Files:**
- Modify: `frontend/web/src/locales/en.json` (`schedule` object)
- Modify: `frontend/web/src/locales/ru.json` (`schedule` object)
- Modify: `frontend/web/src/locales/ja.json` (`schedule` object)

The existing `schedule` object already has `title`, `today`, `noData`, `hint`, `episode`, `timeMsk`, `daysShort.*`, `days.*`. Add the new keys below. Keep `daysShort`/`days` as-is.

- [ ] **Step 1: Extend `ru.json` `schedule` with the new keys**

Add these keys inside the existing `"schedule": { … }` object in `frontend/web/src/locales/ru.json`:

```json
"viewMonth": "Месяц",
"viewWeek": "Неделя",
"viewTable": "Таблица",
"todayBtn": "Сегодня",
"episodeShort": "серия",
"more": "+{n} ещё",
"mskSuffix": "МСК",
"watch": "Смотреть",
"searchPlaceholder": "Поиск по названию…",
"searchChip": "Поиск",
"myList": "Мой список",
"genre": "Жанр",
"type": "Тип",
"activeFilters": "Активно:",
"noFilters": "Фильтры не выбраны",
"resetAll": "Сбросить всё",
"countOf": "{n} из {total} тайтлов",
"empty": "Ничего не найдено",
"col": { "anime": "Аниме", "datetime": "Дата · время", "episode": "Серия", "score": "Оценка" },
"episodeCountPlural": "{n} серия | {n} серии | {n} серий",
"monthsNominative": { "jan": "Январь", "feb": "Февраль", "mar": "Март", "apr": "Апрель", "may": "Май", "jun": "Июнь", "jul": "Июль", "aug": "Август", "sep": "Сентябрь", "oct": "Октябрь", "nov": "Ноябрь", "dec": "Декабрь" },
"monthsGenitive": { "jan": "января", "feb": "февраля", "mar": "марта", "apr": "апреля", "may": "мая", "jun": "июня", "jul": "июля", "aug": "августа", "sep": "сентября", "oct": "октября", "nov": "ноября", "dec": "декабря" }
```

- [ ] **Step 2: Extend `en.json` `schedule` with the same keys (English values)**

```json
"viewMonth": "Month",
"viewWeek": "Week",
"viewTable": "Table",
"todayBtn": "Today",
"episodeShort": "ep.",
"more": "+{n} more",
"mskSuffix": "MSK",
"watch": "Watch",
"searchPlaceholder": "Search by title…",
"searchChip": "Search",
"myList": "My list",
"genre": "Genre",
"type": "Type",
"activeFilters": "Active:",
"noFilters": "No filters",
"resetAll": "Reset all",
"countOf": "{n} of {total} titles",
"empty": "Nothing found",
"col": { "anime": "Anime", "datetime": "Date · time", "episode": "Episode", "score": "Score" },
"episodeCountPlural": "{n} episode | {n} episodes",
"monthsNominative": { "jan": "January", "feb": "February", "mar": "March", "apr": "April", "may": "May", "jun": "June", "jul": "July", "aug": "August", "sep": "September", "oct": "October", "nov": "November", "dec": "December" },
"monthsGenitive": { "jan": "January", "feb": "February", "mar": "March", "apr": "April", "may": "May", "jun": "June", "jul": "July", "aug": "August", "sep": "September", "oct": "October", "nov": "November", "dec": "December" }
```

- [ ] **Step 3: Extend `ja.json` `schedule` with the same keys (Japanese values)**

```json
"viewMonth": "月",
"viewWeek": "週",
"viewTable": "表",
"todayBtn": "今日",
"episodeShort": "話",
"more": "他{n}件",
"mskSuffix": "MSK",
"watch": "見る",
"searchPlaceholder": "タイトルで検索…",
"searchChip": "検索",
"myList": "マイリスト",
"genre": "ジャンル",
"type": "タイプ",
"activeFilters": "適用中:",
"noFilters": "フィルターなし",
"resetAll": "すべてリセット",
"countOf": "{total}件中{n}件",
"empty": "見つかりませんでした",
"col": { "anime": "アニメ", "datetime": "日付・時刻", "episode": "話数", "score": "評価" },
"episodeCountPlural": "{n}話",
"monthsNominative": { "jan": "1月", "feb": "2月", "mar": "3月", "apr": "4月", "may": "5月", "jun": "6月", "jul": "7月", "aug": "8月", "sep": "9月", "oct": "10月", "nov": "11月", "dec": "12月" },
"monthsGenitive": { "jan": "1月", "feb": "2月", "mar": "3月", "apr": "4月", "may": "5月", "jun": "6月", "jul": "7月", "aug": "8月", "sep": "9月", "oct": "10月", "nov": "11月", "dec": "12月" }
```

- [ ] **Step 4: Validate JSON + i18n parity**

Run: `cd frontend/web && node -e "['en','ru','ja'].forEach(l=>{const s=require('./src/locales/'+l+'.json').schedule; console.log(l, Object.keys(s).length)})"`
Expected: all three print the SAME key count. If a project i18n-parity test exists, run it:
Run: `bunx vitest run src/locales/__tests__`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(schedule): i18n keys for calendar views/filters/modal (en/ru/ja)"
```

---

## Task 13: Rewrite `Schedule.vue` shell + wire everything

**Files:**
- Modify: `frontend/web/src/views/Schedule.vue` (full rewrite)
- Test: `frontend/web/src/views/Schedule.spec.ts`

The shell: header (title + segmented view toggle + nav arrows + Today), filter bar, active view, and the day modal. It fetches via `useAnime().fetchSchedule`, reads watchlist statuses, and syncs `view`/`date` to the URL query.

- [ ] **Step 1: Rewrite Schedule.vue**

```vue
<!-- frontend/web/src/views/Schedule.vue -->
<template>
  <div class="min-h-screen bg-background pt-20">
    <div class="container mx-auto px-4 py-8">
      <!-- Header -->
      <div class="flex items-center justify-between gap-3 flex-wrap mb-5">
        <div class="flex items-center gap-3 flex-wrap">
          <h1 class="text-2xl font-bold text-foreground font-display min-w-[130px]">{{ headerTitle }}</h1>
          <div class="flex gap-1.5">
            <button class="h-8 w-8 rounded-lg bg-white/[0.06] hover:bg-white/12 flex items-center justify-center" @click="cal.shift(-1)">‹</button>
            <button class="h-8 w-8 rounded-lg bg-white/[0.06] hover:bg-white/12 flex items-center justify-center" @click="cal.shift(1)">›</button>
          </div>
          <button class="h-8 px-3 rounded-lg text-primary border border-primary/40 text-xs" @click="cal.goToday()">{{ $t('schedule.todayBtn') }}</button>
        </div>
        <div class="flex bg-white/[0.06] rounded-lg p-0.5">
          <button v-for="v in views" :key="v" class="text-xs px-3.5 py-1.5 rounded-md transition-colors"
            :class="cal.view.value === v ? 'bg-primary text-primary-foreground font-semibold' : 'text-muted-foreground'"
            @click="cal.setView(v)">{{ $t('schedule.view' + cap(v)) }}</button>
        </div>
      </div>

      <ScheduleFilters
        :filters="cal.filters"
        :genres="cal.genres.value"
        :logged-in="loggedIn"
        :match-count="cal.filteredAnimes.value.length"
        :total="schedule.length"
        @reset="cal.resetFilters()"
      />

      <div v-if="loading" class="flex justify-center py-12">
        <div class="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin"></div>
      </div>

      <template v-else>
        <MonthView v-if="cal.view.value === 'month'" :cells="cal.monthCells.value" @open="openDay" />
        <WeekView v-else-if="cal.view.value === 'week'" :columns="cal.weekColumns.value" @open="openDay" />
        <TableView v-else :rows="cal.tableRows.value" :sort-key="cal.sortKey.value" :sort-dir="cal.sortDir.value" @sort="cal.setSort($event)" />

        <div v-if="isEmpty" class="text-center py-12 text-muted-foreground">{{ $t('schedule.empty') }}</div>
      </template>
    </div>

    <DayModal v-model="modalOpen" :date="modalDate" :occurrences="modalOccurrences" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAnime } from '@/composables/useAnime'
import { useWatchlistStore } from '@/stores/watchlist'
import { useAuthStore } from '@/stores/auth'
import { useScheduleCalendar, type ScheduleView } from '@/composables/useScheduleCalendar'
import type { ScheduleAnime, Occurrence } from '@/composables/schedule/types'
import { isSameDay } from '@/composables/schedule/calendarGrid'
import ScheduleFilters from '@/components/schedule/ScheduleFilters.vue'
import MonthView from '@/components/schedule/MonthView.vue'
import WeekView from '@/components/schedule/WeekView.vue'
import TableView from '@/components/schedule/TableView.vue'
import DayModal from '@/components/schedule/DayModal.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { fetchSchedule, loading } = useAnime()
const watchlist = useWatchlistStore()
const auth = useAuthStore()

const schedule = ref<ScheduleAnime[]>([])
const now = ref(new Date())
const loggedIn = computed(() => auth.isAuthenticated)

const cal = useScheduleCalendar({
  animes: computed(() => schedule.value) as unknown as import('vue').Ref<ScheduleAnime[]>,
  now,
  statusOf: (id: string) => watchlist.getStatus(id),
  loggedIn,
})

const views: ScheduleView[] = ['month', 'week', 'table']
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)
const headerTitle = computed(() => formatHeader())

const isEmpty = computed(() => {
  if (cal.view.value === 'table') return cal.tableRows.value.length === 0
  if (cal.view.value === 'week') return cal.weekColumns.value.every(c => c.occurrences.length === 0)
  return cal.monthCells.value.every(c => c.occurrences.length === 0)
})

function formatHeader(): string {
  const months = ['jan','feb','mar','apr','may','jun','jul','aug','sep','oct','nov','dec']
  const d = cal.viewDate.value
  if (cal.view.value === 'week' || cal.view.value === 'table') {
    const off = (d.getDay() + 6) % 7
    const s = new Date(d.getFullYear(), d.getMonth(), d.getDate() - off)
    const e = new Date(s.getFullYear(), s.getMonth(), s.getDate() + 6)
    return `${s.getDate()}–${e.getDate()} ${t('schedule.monthsGenitive.' + months[e.getMonth()])}`
  }
  return `${t('schedule.monthsNominative.' + months[d.getMonth()])} ${d.getFullYear()}`
}

// ---- day modal ----
const modalOpen = ref(false)
const modalDate = ref<Date | null>(null)
function openDay(date: Date) {
  modalDate.value = date
  modalOpen.value = true
}
const modalOccurrences = computed<Occurrence[]>(() => {
  if (!modalDate.value) return []
  const src = cal.view.value === 'week' ? cal.weekColumns.value : cal.monthCells.value
  const cell = src.find(c => isSameDay(c.date, modalDate.value as Date))
  return cell ? cell.occurrences : []
})

// ---- URL query sync ----
function readQuery() {
  const v = route.query.view as string | undefined
  if (v === 'month' || v === 'week' || v === 'table') cal.setView(v)
  const dstr = route.query.date as string | undefined
  if (dstr) {
    const d = new Date(dstr)
    if (!Number.isNaN(d.getTime())) cal.viewDate.value = d
  }
}
watch([cal.view, cal.viewDate], () => {
  const d = cal.viewDate.value
  const dstr = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  router.replace({ query: { ...route.query, view: cal.view.value, date: dstr } })
})

onMounted(async () => {
  readQuery()
  if (loggedIn.value) watchlist.fetchStatuses().catch(() => {})
  try {
    schedule.value = await fetchSchedule()
  } catch (err) {
    console.error('Failed to fetch schedule:', err)
  }
})
</script>
```

> Note on the `animes` prop: `useScheduleCalendar` expects a `Ref<ScheduleAnime[]>`. Passing `computed(() => schedule.value)` works at runtime; the `as unknown as Ref<…>` cast silences the `ComputedRef` vs `Ref` variance. If `vue-tsc` complains, change the composable's `animes` option type to `Ref<ScheduleAnime[]> | ComputedRef<ScheduleAnime[]>` (import `ComputedRef` from `vue`) and drop the cast.

- [ ] **Step 2: Verify auth store exposes `isAuthenticated`**

Run: `cd frontend/web && grep -n "isAuthenticated" src/stores/auth.ts`
Expected: a getter/computed named `isAuthenticated`. If it's named differently (e.g. `isLoggedIn`), update the `loggedIn` computed in Step 1 accordingly.

- [ ] **Step 3: Write a view smoke test**

```ts
// frontend/web/src/views/Schedule.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import { createTestingPinia } from '@pinia/testing'
import Schedule from './Schedule.vue'

vi.mock('@/composables/useAnime', () => ({
  useAnime: () => ({
    loading: { value: false },
    fetchSchedule: vi.fn().mockResolvedValue([
      { id: '1', name: 'Kaiju', name_ru: 'Кайдзю', kind: 'TV', episodes_aired: 9, episodes_count: 12, next_episode_at: '2026-06-08T17:00:00Z', genres: [{ name: 'Action' }] },
    ]),
  }),
}))

const $t = (k: string, a?: Record<string, unknown>) => (a ? `${k}` : k)

function mountView() {
  return mount(Schedule, {
    global: {
      plugins: [createTestingPinia({ createSpy: vi.fn })],
      stubs: { RouterLink: RouterLinkStub },
      mocks: {
        $t,
        $route: { query: {} },
        $router: { replace: vi.fn(), push: vi.fn() },
      },
    },
  })
}

describe('Schedule view', () => {
  it('mounts and renders the view toggle', () => {
    const w = mountView()
    expect(w.text()).toContain('schedule.viewMonth')
    expect(w.text()).toContain('schedule.viewWeek')
    expect(w.text()).toContain('schedule.viewTable')
  })
})
```

> If the project already has a test harness/util for mounting views with router+i18n (check `src/test/` or existing `*.spec.ts` under `src/views/`), prefer that helper over hand-rolled mocks to match conventions.

- [ ] **Step 4: Run the view test**

Run: `cd frontend/web && bunx vitest run src/views/Schedule.spec.ts`
Expected: PASS. Fix mock/stub mismatches until green (router/i18n/pinia wiring is the usual culprit — mirror an existing view spec).

- [ ] **Step 5: Run the full schedule test suite + type-check**

Run: `cd frontend/web && bunx vitest run src/composables/schedule src/components/schedule src/views/Schedule.spec.ts && bunx vue-tsc --noEmit`
Expected: ALL PASS, no type errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/views/Schedule.vue frontend/web/src/views/Schedule.spec.ts
git commit -m "feat(schedule): calendar shell — views, nav, filters, day modal, URL sync"
```

---

## Task 14: Design-system lint, responsive polish, in-browser smoke

**Files:**
- Modify (only if lint flags them): any `frontend/web/src/components/schedule/*.vue`, `frontend/web/src/views/Schedule.vue`

- [ ] **Step 1: Run the design-system lint (build gate)**

Run: `cd frontend/web && bash scripts/design-system-lint.sh`
Expected: `ERRORS: 0`. If it flags off-palette classes (e.g. a raw `text-yellow-*`), migrate to a semantic token (`text-warning`, `text-success`, `text-muted-foreground`, …). `cyan`/`violet`/`pink` brand hues and the `var(--brand-*)` usages in `today-bar` are allowed. Do NOT disable the gate.

- [ ] **Step 2: Confirm the star color token**

The TableView/DayModal use `text-warning` for the ★ (Step in Task 8/10 uses `text-warning`). Verify `--warning` exists (it does per DESIGN-SYSTEM.md). If lint flags any literal color, replace with the token.

- [ ] **Step 3: Build the frontend**

Run: `cd frontend/web && bun run build`
Expected: build succeeds (this also runs the lint gate via `make`-equivalent path; a green build confirms no type/lint breakage).

- [ ] **Step 4: In-browser smoke (DS-NF-06 — REQUIRED, jsdom can't catch Tailwind v4 cascade bugs)**

Start the dev server and verify in a real browser at desktop + mobile widths:

Run: `cd frontend/web && bun run dev` (note the local URL)

Manually verify (desktop ~1280px and mobile ~390px):
- Month view: today cell has the cyan→violet top bar; a busy day shows 3 rows + "+N ещё"; clicking a day opens the modal; modal "Смотреть" / card navigates to `/anime/{id}`; ✕/Esc/backdrop close it.
- Week view: 7 columns, all episodes listed; clicking a column opens the modal.
- Table view: week-scoped; clicking column headers sorts (arrow flips); day-group separators show when sorted by date; row click navigates.
- Filters: search narrows live; My List (logged in) toggles; Genre/Type dropdowns multiselect; chips remove individually; "Сбросить всё" clears; count "N из M" updates. (Озвучка filter is intentionally absent.)
- Nav: ‹ › move month (month view) / week (week+table); "Сегодня" returns to today; `?view=&date=` in the URL updates and survives reload.
- Mobile (~390px): month cells are compact (posters, names hidden); table hides Score + Watch columns; everything tappable.

- [ ] **Step 5: Commit any lint/responsive fixes**

```bash
git add frontend/web/src/components/schedule frontend/web/src/views/Schedule.vue
git commit -m "fix(schedule): design-system token + responsive polish"
```

- [ ] **Step 6: Run the after-update skill (project requirement)**

Invoke `/animeenigma-after-update` to lint, redeploy `web`, health-check, write the Russian Trump-mode changelog entry, and push. (Per CLAUDE.md this is mandatory after implementation work.)

---

## Self-Review notes (for the implementer)

- **Mobile compact month** (hiding titles, posters only) is implemented via Tailwind responsive classes inside `EpisodeRow.vue`/`DayCell.vue`. If the prototype's exact compact look is desired, add a `@media (max-width:640px)` rule in `DayCell.vue` `<style scoped>` mirroring the prototype's `.row{flex-direction:column}` + hide-text behavior. Verify in Step 4 of Task 14, not in jsdom.
- **Genre `kind` value for "Movie"**: the type filter uses raw `kind` values from the API (`TV`/`ONA`/`Movie`/`OVA`/…). Confirm the actual `kind` strings the catalog returns (run `grep -rn "kind" services/catalog/internal/parser` or inspect a live `/api/anime/schedule` response) and adjust the `TYPES` array in `ScheduleFilters.vue` if they differ (e.g. lowercase `tv`).
- **No backend, no AnimeContextMenu/kebab** — the old context menu is intentionally dropped (clicking a day opens the modal; the modal links to the anime page). This matches the approved prototype.
```
