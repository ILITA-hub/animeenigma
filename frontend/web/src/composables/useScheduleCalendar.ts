// frontend/web/src/composables/useScheduleCalendar.ts
import { computed, reactive, ref, type Ref } from 'vue'
import type { ScheduleAnime, Occurrence, TableSortKey } from './schedule/types'
import { emptyFilters } from './schedule/types'
import { occurrencesInRange } from './schedule/projection'
import { applyFilters, availableGenres, sortCellHybrid } from './schedule/filterSort'
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
  const view = ref<ScheduleView>('week')
  const viewDate = ref<Date>(startOfDay(opts.now.value))
  const filters = reactive(emptyFilters())
  const sortKey = ref<TableSortKey>('date')
  const sortDir = ref<1 | -1>(1)

  const isPriority = (a: ScheduleAnime) =>
    opts.loggedIn.value && ['watching', 'planned', 'plan_to_watch'].includes(opts.statusOf(a.id) ?? '')

  const filteredAnimes = computed(() => applyFilters(opts.animes.value, filters, opts.statusOf))
  const genres = computed(() => availableGenres(opts.animes.value))

  const monthCells = computed<DayCellModel[]>(() => {
    void opts.loggedIn.value // track login state so priority re-sorts on auth change
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
    void opts.loggedIn.value // track login state (keeps parity with monthCells)
    const days = weekDays(viewDate.value)
    const start = startOfDay(days[0])
    const end = new Date(start.getTime() + WEEK_MS)
    const all = occurrencesInRange(filteredAnimes.value, start, end)
    return days.map((date) => ({
      date,
      inCurrentMonth: date.getMonth() === viewDate.value.getMonth(),
      isToday: isSameDay(date, opts.now.value),
      // Hybrid sort (user's list first, then by time) so watched/planned titles
      // float to the top of each day — matters now that the week view is the
      // default and caps each day at 4 rows.
      occurrences: sortCellHybrid(all.filter((o) => isSameDay(o.date, date)), isPriority),
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
