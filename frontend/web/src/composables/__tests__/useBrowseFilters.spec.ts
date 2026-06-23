import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

// Mutable route query + spy router, controlled per test. Pattern mirrors
// src/views/Schedule.spec.ts:7.
const mockState = vi.hoisted(() => ({ query: {} as Record<string, string> }))
const mockReplace = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRoute: () => ({
    get query() {
      return mockState.query
    },
  }),
  useRouter: () => ({ replace: mockReplace }),
}))

import { useBrowseFilters } from '@/composables/useBrowseFilters'

function harness() {
  let api!: ReturnType<typeof useBrowseFilters>
  const Cmp = defineComponent({
    setup() {
      api = useBrowseFilters()
      return () => null
    },
  })
  const wrapper = mount(Cmp)
  return { api, wrapper }
}

beforeEach(() => {
  mockState.query = {}
  mockReplace.mockClear()
})

describe('useBrowseFilters — season', () => {
  it('reads a valid season from the URL into apiParams', () => {
    mockState.query = { season: 'summer' }
    const { api } = harness()
    api.readUrl()
    expect(api.season.value).toBe('summer')
    expect(api.apiParams.value.season).toBe('summer')
  })

  it('drops an invalid season value', () => {
    mockState.query = { season: 'bogus' }
    const { api } = harness()
    api.readUrl()
    expect(api.season.value).toBe('')
    expect(api.apiParams.value.season).toBeUndefined()
  })

  it('writes the season to the URL query', () => {
    const { api } = harness()
    api.season.value = 'fall'
    api.writeUrl()
    expect(mockReplace).toHaveBeenCalledWith({
      query: expect.objectContaining({ season: 'fall' }),
    })
  })

  it('counts season as an active filter', () => {
    const { api } = harness()
    expect(api.activeCount.value).toBe(0)
    api.season.value = 'winter'
    expect(api.activeCount.value).toBe(1)
  })

  it('reset clears the season', () => {
    const { api } = harness()
    api.season.value = 'spring'
    api.reset()
    expect(api.season.value).toBe('')
  })
})

describe('useBrowseFilters — kinds', () => {
  it('reads a comma-separated kind list from the URL into apiParams', () => {
    mockState.query = { kind: 'tv,movie' }
    const { api } = harness()
    api.readUrl()
    expect(api.kinds.value).toEqual(['tv', 'movie'])
    expect(api.apiParams.value.kind).toBe('tv,movie')
  })

  it('drops invalid kind values', () => {
    mockState.query = { kind: 'tv,bogus' }
    const { api } = harness()
    api.readUrl()
    expect(api.kinds.value).toEqual(['tv'])
  })

  it('writes the kinds joined to the URL query', () => {
    const { api } = harness()
    api.kinds.value = ['ova', 'ona']
    api.writeUrl()
    expect(mockReplace).toHaveBeenCalledWith({
      query: expect.objectContaining({ kind: 'ova,ona' }),
    })
  })

  it('counts a non-empty kinds list as one active filter', () => {
    const { api } = harness()
    expect(api.activeCount.value).toBe(0)
    api.kinds.value = ['tv', 'movie']
    expect(api.activeCount.value).toBe(1)
  })

  it('reset clears the kinds', () => {
    const { api } = harness()
    api.kinds.value = ['special']
    api.reset()
    expect(api.kinds.value).toEqual([])
  })
})
