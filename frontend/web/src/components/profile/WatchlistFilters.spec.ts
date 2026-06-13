import { describe, it, expect, vi } from 'vitest'
import { ref } from 'vue'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ locale: ref('en') }),
}))

import WatchlistFilters from './WatchlistFilters.vue'
import type { WatchlistFacets } from '@/types/watchlist-facets'

const facets: WatchlistFacets = {
  genres: [
    { id: 'g-action', name: 'Action', name_ru: 'Экшен', count: 5 },
    { id: 'g-comedy', name: 'Comedy', name_ru: 'Комедия', count: 3 },
  ],
  kinds: [
    { kind: 'tv', count: 7 },
    { kind: 'movie', count: 2 },
  ],
  years: { min: 2010, max: 2024 },
}

// Panel-only component: the trigger + open state live in Profile.vue, so the
// panel content renders immediately on mount here.
function mountWith(props: Record<string, unknown> = {}) {
  return mount(WatchlistFilters, {
    props: {
      facets,
      genreIds: [],
      kinds: [],
      yearMin: null,
      yearMax: null,
      ...props,
    },
    global: {
      mocks: { $t: (k: string) => k },
    },
  })
}

describe('WatchlistFilters', () => {
  it('renders a genre row per facet genre with its count', () => {
    const wrapper = mountWith()
    expect(wrapper.text()).toContain('Action')
    expect(wrapper.text()).toContain('5')
    expect(wrapper.text()).toContain('Comedy')
  })

  it('renders a row per kind with its count', () => {
    const wrapper = mountWith()
    expect(wrapper.text()).toContain('profile.filters.kind.tv')
    expect(wrapper.text()).toContain('7')
    expect(wrapper.text()).toContain('profile.filters.kind.movie')
  })

  it('emits update:genreIds when a genre is toggled on', async () => {
    const wrapper = mountWith()
    const checkbox = wrapper.findAllComponents({ name: 'Checkbox' })[0]
    await checkbox.vm.$emit('update:modelValue', true)
    expect(wrapper.emitted('update:genreIds')?.[0]).toEqual([['g-action']])
  })

  it('removes a genre when toggled off', async () => {
    const wrapper = mountWith({ genreIds: ['g-action'] })
    const checkbox = wrapper.findAllComponents({ name: 'Checkbox' })[0]
    await checkbox.vm.$emit('update:modelValue', false)
    expect(wrapper.emitted('update:genreIds')?.[0]).toEqual([[]])
  })

  it('disables clear-all when no filter is active', () => {
    const wrapper = mountWith()
    const clearBtn = wrapper.findAll('button').find((b) => b.text().includes('profile.filters.clear'))
    expect(clearBtn).toBeTruthy()
    expect(clearBtn!.attributes('disabled')).toBeDefined()
  })

  it('clear-all emits resets for every dimension when filters are active', async () => {
    const wrapper = mountWith({ genreIds: ['g-action'], kinds: ['tv'], yearMin: 2015, yearMax: 2020 })
    const clearBtn = wrapper.findAll('button').find((b) => b.text().includes('profile.filters.clear'))
    expect(clearBtn).toBeTruthy()
    await clearBtn!.trigger('click')
    expect(wrapper.emitted('update:genreIds')?.[0]).toEqual([[]])
    expect(wrapper.emitted('update:kinds')?.[0]).toEqual([[]])
    expect(wrapper.emitted('update:yearMin')?.[0]).toEqual([null])
    expect(wrapper.emitted('update:yearMax')?.[0]).toEqual([null])
  })

  it('renders AND hint for genres and OR hint for types', () => {
    const wrapper = mountWith()
    expect(wrapper.text()).toContain('profile.filters.genresHint')
    expect(wrapper.text()).toContain('profile.filters.typesHint')
  })
})
