// frontend/web/src/composables/useScheduleCalendar.ts
import { computed, reactive, ref, type Ref } from 'vue'
import type { ScheduleAnime, ScheduleConfirmedOccurrence, Occurrence, TableSortKey } from './schedule/types'
import { emptyFilters } from './schedule/types'
import { occurrencesInRange } from './schedule/projection'
import { applyFilters, availableGenres, sortCellHybrid, statusRank } from './schedule/filterSort'
import { monthGridDays, monthGridRange, weekDays, weekStart, startOfDay, isSameDay } from './schedule/calendarGrid'
import { wallClockDate } from './schedule/timezone'

export type ScheduleView = 'month' | 'week' | 'table'

export interface DayCellModel {
  date: Date
  inCurrentMonth: boolean
  isToday: boolean
  occurrences: Occurrence[]
}

export interface UseScheduleCalendarOptions {
  animes: Ref<ScheduleAnime[]>
  confirmedOccurrences: Ref<ScheduleConfirmedOccurrence[]>
  now: Ref<Date>
  statusOf: (animeId: string) => string | null
  loggedIn: Ref<boolean>
  /** IANA display timezone; occurrences + "today" follow it. Omitted → browser-local. */
  timezone?: Ref<string | undefined>
}

const WEEK_MS = 7 * 86400000

export function useScheduleCalendar(opts: UseScheduleCalendarOptions) {
  const view = ref<ScheduleView>('week')
  const viewDate = ref<Date>(startOfDay(opts.now.value))
  const filters = reactive(emptyFilters())
  const sortKey = ref<TableSortKey>('date')
  const sortDir = ref<1 | -1>(1)

  // Watching → plan-to-watch → rest (logged out: everything is "rest").
  const rankOf = (a: ScheduleAnime) =>
    opts.loggedIn.value ? statusRank(opts.statusOf(a.id)) : 2

  const allAnimes = computed(() => {
    const byID = new Map(opts.animes.value.map((anime) => [anime.id, anime]))
    for (const occurrence of opts.confirmedOccurrences.value) {
      if (!byID.has(occurrence.anime.id)) byID.set(occurrence.anime.id, occurrence.anime)
    }
    return [...byID.values()]
  })
  const filteredAnimes = computed(() => applyFilters(allAnimes.value, filters, opts.statusOf))
  const filteredUpcomingAnimes = computed(() => applyFilters(opts.animes.value, filters, opts.statusOf))
  const genres = computed(() => availableGenres(allAnimes.value))

  const tz = () => opts.timezone?.value
  // "now" expressed in the display timezone's wall clock, so the today
  // highlight moves with the selected zone (a new day starts at ITS midnight).
  const zonedNow = () => wallClockDate(opts.now.value, tz())

  const monthCells = computed<DayCellModel[]>(() => {
    void opts.loggedIn.value // track login state so priority re-sorts on auth change
    const days = monthGridDays(viewDate.value)
    const { start, end } = monthGridRange(viewDate.value)
    const all = occurrencesInRange(filteredAnimes.value, filteredUpcomingAnimes.value, opts.confirmedOccurrences.value, start, end, tz())
    const m = viewDate.value.getMonth()
    return days.map((date) => {
      const inMonth = date.getMonth() === m
      const occ = inMonth ? all.filter((o) => isSameDay(o.date, date)) : []
      return {
        date,
        inCurrentMonth: inMonth,
        isToday: isSameDay(date, zonedNow()),
        occurrences: sortCellHybrid(occ, rankOf),
      }
    })
  })

  const weekColumns = computed<DayCellModel[]>(() => {
    void opts.loggedIn.value // track login state (keeps parity with monthCells)
    const days = weekDays(viewDate.value)
    const start = startOfDay(days[0])
    const end = new Date(start.getTime() + WEEK_MS)
    const all = occurrencesInRange(filteredAnimes.value, filteredUpcomingAnimes.value, opts.confirmedOccurrences.value, start, end, tz())
    return days.map((date) => ({
      date,
      inCurrentMonth: date.getMonth() === viewDate.value.getMonth(),
      isToday: isSameDay(date, zonedNow()),
      // Tier sort (watching → plan-to-watch → rest, by time within a tier) so
      // the user's titles float to the top of each day — matters now that the
      // week view is the default and caps each day at 4 rows.
      occurrences: sortCellHybrid(all.filter((o) => isSameDay(o.date, date)), rankOf),
    }))
  })

  const tableRows = computed<Occurrence[]>(() => {
    const days = weekDays(viewDate.value)
    const start = startOfDay(days[0])
    const end = new Date(start.getTime() + WEEK_MS)
    const all = occurrencesInRange(filteredAnimes.value, filteredUpcomingAnimes.value, opts.confirmedOccurrences.value, start, end, tz())
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
  function goToday() { viewDate.value = startOfDay(zonedNow()) }
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

  const visibleRange = computed(() => {
    if (view.value === 'month') return monthGridRange(viewDate.value)
    const start = weekStart(viewDate.value)
    return { start, end: new Date(start.getTime() + WEEK_MS) }
  })

  return {
    view, viewDate, filters, sortKey, sortDir,
    allAnimes, filteredAnimes, genres, monthCells, weekColumns, tableRows, activeFilterCount, visibleRange,
    setView, shift, goToday, setSort, resetFilters,
  }
}
