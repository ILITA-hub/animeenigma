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
    for (const r of cal.tableRows.value) {
      expect(r.date >= new Date(2026, 5, 8)).toBe(true)
      expect(r.date < new Date(2026, 5, 15)).toBe(true)
    }
  })
})
