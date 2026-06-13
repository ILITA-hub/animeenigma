export interface FacetGenre {
  id: string
  name: string
  name_ru: string
  count: number
}

export interface FacetKind {
  kind: string
  count: number
}

export interface FacetYearRange {
  min: number | null
  max: number | null
}

export interface WatchlistFacets {
  genres: FacetGenre[]
  kinds: FacetKind[]
  years: FacetYearRange
}

/** Active filter selections, v-modeled by WatchlistFilters.vue. */
export interface WatchlistFilterState {
  genreIds: string[]
  kinds: string[]
  yearMin: number | null
  yearMax: number | null
}

export const EMPTY_FILTER_STATE: WatchlistFilterState = {
  genreIds: [],
  kinds: [],
  yearMin: null,
  yearMax: null,
}

/** True when no filter dimension is active. */
export function isFilterStateEmpty(s: WatchlistFilterState): boolean {
  return s.genreIds.length === 0 && s.kinds.length === 0 && s.yearMin === null && s.yearMax === null
}

/** Count of active filter dimensions (for the trigger badge). genres/kinds
 *  each count their selected entries; an active year range counts as 1. */
export function activeFilterCount(s: WatchlistFilterState): number {
  return s.genreIds.length + s.kinds.length + (s.yearMin !== null || s.yearMax !== null ? 1 : 0)
}

/** Serialize to query params for the watchlist list endpoints. */
export function filterParams(s: WatchlistFilterState): Record<string, string> {
  const p: Record<string, string> = {}
  if (s.genreIds.length) p.genres = s.genreIds.join(',')
  if (s.kinds.length) p.kind = s.kinds.join(',')
  if (s.yearMin !== null) p.year_min = String(s.yearMin)
  if (s.yearMax !== null) p.year_max = String(s.yearMax)
  return p
}

/** Stable string for the page-cache key (order-independent). */
export function filterKey(s: WatchlistFilterState): string {
  return [
    [...s.genreIds].sort().join('+'),
    [...s.kinds].sort().join('+'),
    s.yearMin ?? '',
    s.yearMax ?? '',
  ].join('|')
}
