// frontend/web/src/components/schedule/DayModal.spec.ts
import { describe, it, expect, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'
import DayModal from './DayModal.vue'

// Mock vue-i18n so useI18n() in <script setup> resolves without a plugin.
// $t in the template is still provided via global.mocks below.
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

const occ = [
  { anime: { id: '1', name: 'Kaiju', name_ru: 'Кайдзю', poster_url: '/p.jpg', score: 8.4 }, episode: 10, date: new Date('2026-06-13T17:00:00+03:00') },
  { anime: { id: '2', name: 'Frieren', name_ru: 'Фрирен', poster_url: '/q.jpg', score: 9.3 }, episode: 23, date: new Date('2026-06-13T22:00:00+03:00') },
]

function mountModal() {
  return mount(DayModal, {
    props: { modelValue: true, date: new Date(2026, 5, 13), occurrences: occ },
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        Modal: { template: '<div><slot /></div>' },
        Button: { template: '<button><slot /></button>' },
      },
      mocks: { $t: (k: string) => k },
    },
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
    expect(w.text()).toContain('Фрирен')
  })
})
