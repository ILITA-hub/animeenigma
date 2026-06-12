// frontend/web/src/components/schedule/ScheduleFilters.spec.ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive, ref, nextTick } from 'vue'
import ScheduleFilters from './ScheduleFilters.vue'
import { emptyFilters } from '@/composables/schedule/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k, locale: ref('ru') }),
  // The ui-index import chain (Chip) reaches src/i18n.ts, which calls
  // createI18n at module scope — give it an inert instance.
  createI18n: () => ({
    install: () => {},
    global: { t: (k: string) => k, locale: { value: 'ru' }, availableLocales: ['ru'] },
  }),
}))

function mountFilters(over = {}) {
  const filters = reactive(emptyFilters())
  return {
    filters,
    w: mount(ScheduleFilters, {
      props: { filters, genres: [{ name: 'Action', name_ru: 'Экшен' }], loggedIn: true, matchCount: 5, total: 12, ...over },
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
    await w.get('button').trigger('click')
    expect(filters.myList).toBe(true)
  })
  it('hides My List for guests', () => {
    const { w } = mountFilters({ loggedIn: false })
    expect(w.text()).not.toContain('myList')
  })
  it('chip remove is a native button and clears the filter', async () => {
    const { filters, w } = mountFilters()
    filters.myList = true
    await nextTick()
    const removeBtn = w.findAll('[data-testid="chip-remove"]')
    expect(removeBtn.length).toBe(1)
    await removeBtn[0].trigger('click')
    expect(filters.myList).toBe(false)
  })
  it('reset-all is a native button', async () => {
    const { filters, w } = mountFilters()
    filters.myList = true
    await nextTick()
    const reset = w.findAll('button').filter((b) => b.text().includes('resetAll'))
    expect(reset.length).toBe(1)
    await reset[0].trigger('click')
    expect(w.emitted('reset')).toBeTruthy()
  })
})
