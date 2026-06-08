// frontend/web/src/components/schedule/DayCell.spec.ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DayCell from './DayCell.vue'
import EpisodeRow from './EpisodeRow.vue'
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
const mountCell = (cell: DayCellModel) =>
  mount(DayCell, { props: { cell }, global: { mocks: { $t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${p.n}` : k) } } })

describe('DayCell', () => {
  it('renders up to 3 episode rows', () => {
    const occ = [1, 2, 3, 4, 5].map((n) => ({ anime: a(String(n)), episode: n, date: new Date(2026, 5, 13, n) }))
    const w = mountCell(model({ occurrences: occ }))
    expect(w.findAllComponents(EpisodeRow).length).toBe(3)
  })
  it('shows "+N more" when there are more than 3', () => {
    const occ = [1, 2, 3, 4, 5].map((n) => ({ anime: a(String(n)), episode: n, date: new Date(2026, 5, 13, n) }))
    const w = mountCell(model({ occurrences: occ }))
    expect(w.text()).toContain('2') // +2
  })
  it('emits open with the date when a cell with episodes is clicked', async () => {
    const occ = [{ anime: a('1'), episode: 1, date: new Date(2026, 5, 13, 1) }]
    const w = mountCell(model({ occurrences: occ }))
    await w.trigger('click')
    expect(w.emitted('open')).toBeTruthy()
    expect((w.emitted('open') as unknown[][])[0][0]).toBeInstanceOf(Date)
  })
  it('does not emit open for empty cells', async () => {
    const w = mountCell(model({ occurrences: [] }))
    await w.trigger('click')
    expect(w.emitted('open')).toBeFalsy()
  })
  it('does not emit when out of month, even with episodes', async () => {
    const occ = [{ anime: a('1'), episode: 1, date: new Date(2026, 5, 13) }]
    const w = mountCell(model({ inCurrentMonth: false, occurrences: occ }))
    await w.trigger('click')
    expect(w.emitted('open')).toBeFalsy()
  })
})
