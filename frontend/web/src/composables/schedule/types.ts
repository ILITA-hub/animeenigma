// frontend/web/src/composables/schedule/types.ts

/** Genre as returned by the catalog API (subset we use). */
export interface ScheduleGenre {
  id?: string
  name?: string
  /** Russian label — the catalog serializes this as `name_ru`. */
  name_ru?: string
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
