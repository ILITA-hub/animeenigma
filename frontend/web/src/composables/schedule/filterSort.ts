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

/**
 * List-status display tier: Watching (0) → Plan to watch (1) → the rest (2).
 * 'planned' is the legacy alias for plan_to_watch kept since the old binary
 * PRIORITY set.
 */
export function statusRank(status: string | null): number {
  if (status === 'watching') return 0
  if (status === 'planned' || status === 'plan_to_watch') return 1
  return 2
}

/** Watching first, then plan-to-watch, then the rest; by air time within a tier. */
export function sortCellHybrid(
  occ: Occurrence[],
  rankOf: (a: ScheduleAnime) => number,
): Occurrence[] {
  return [...occ].sort((x, y) => {
    const rx = rankOf(x.anime)
    const ry = rankOf(y.anime)
    if (rx !== ry) return rx - ry
    return x.date.getTime() - y.date.getTime()
  })
}
