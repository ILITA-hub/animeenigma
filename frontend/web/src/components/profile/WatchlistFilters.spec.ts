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
      stubs: {
        // Render trigger + content inline so we can assert on inner markup.
        Popover: { template: '<div><slot name="trigger" /><slot /></div>' },
      },
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

  it('shows the active-filter count badge (1 genre + 1 kind + 1 year range = 3)', () => {
    const wrapper = mountWith({ genreIds: ['g-action'], kinds: ['tv'], yearMin: 2015 })
    expect(wrapper.text()).toContain('3')
  })

  it('clear-all emits resets for every dimension', async () => {
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
