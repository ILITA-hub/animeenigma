// frontend/web/src/components/schedule/TableView.spec.ts
import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import TableView from './TableView.vue'

vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k }) }))

const rows = [
  { anime: { id: '1', name: 'Kaiju', name_ru: 'Кайдзю', poster_url: '/p.jpg', score: 8.4, episodes_count: 12 }, episode: 10, date: new Date(2026, 5, 8, 17, 0) },
  { anime: { id: '2', name: 'Frieren', name_ru: 'Фрирен', poster_url: '/q.jpg', score: 9.3, episodes_count: 28 }, episode: 23, date: new Date(2026, 5, 10, 22, 0) },
]

function mountTable() {
  return mount(TableView, {
    props: { rows, sortKey: 'date', sortDir: 1 },
    global: { stubs: { RouterLink: RouterLinkStub }, mocks: { $t: (k: string) => k } },
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
    expect(w.findAll('tr.group-row').length).toBe(2)
  })
})
