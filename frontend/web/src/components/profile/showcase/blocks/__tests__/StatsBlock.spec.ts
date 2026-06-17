import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import StatsBlock from '../StatsBlock.vue'

// Mock publicApi so onMounted never fires a network call
vi.mock('@/api/client', () => ({
  publicApi: {
    getPublicWatchlistStats: vi.fn().mockResolvedValue({ data: {} }),
  },
}))

describe('StatsBlock', () => {
  it('mounts without crashing', () => {
    const w = mount(StatsBlock, {
      props: { userId: 'test-user' },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.exists()).toBe(true)
  })

  it('renders the stats block title key', () => {
    const w = mount(StatsBlock, {
      props: { userId: 'test-user' },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.text()).toContain('showcase.block.stats')
  })
})
