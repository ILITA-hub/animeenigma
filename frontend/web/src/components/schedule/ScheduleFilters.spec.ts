// frontend/web/src/components/schedule/ScheduleFilters.spec.ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import ScheduleFilters from './ScheduleFilters.vue'
import { emptyFilters } from '@/composables/schedule/types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k, locale: ref('ru') }) }))

function mountFilters(over = {}) {
  const filters = reactive(emptyFilters())
  return {
    filters,
    w: mount(ScheduleFilters, {
      props: { filters, genres: [{ name: 'Action', russian: 'Экшен' }], loggedIn: true, matchCount: 5, total: 12, ...over },
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
})
