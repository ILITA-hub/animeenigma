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

  it('default variant renders tiles (4 tile divs)', () => {
    const w = mount(StatsBlock, {
      props: { userId: 'test-user' },
      global: { mocks: { $t: (k: string) => k } },
    })
    // tiles variant: grid with 4 children (no explicit variant = tiles)
    const grid = w.find('.grid')
    expect(grid.exists()).toBe(true)
    // 4 tile cells
    expect(grid.findAll(':scope > div').length).toBe(4)
  })

  it('variant:tiles also renders 4 tiles', () => {
    const w = mount(StatsBlock, {
      props: { userId: 'test-user', variant: 'tiles' },
      global: { mocks: { $t: (k: string) => k } },
    })
    const grid = w.find('.grid')
    expect(grid.exists()).toBe(true)
    expect(grid.findAll(':scope > div').length).toBe(4)
  })

  it('variant:rings renders 4 ring elements', () => {
    const w = mount(StatsBlock, {
      props: { userId: 'test-user', variant: 'rings' },
      global: { mocks: { $t: (k: string) => k } },
    })
    const rings = w.findAll('.stats-ring')
    expect(rings.length).toBe(4)
  })

  it('variant:rings renders 4 conic-gradient circles', () => {
    const w = mount(StatsBlock, {
      props: { userId: 'test-user', variant: 'rings' },
      global: { mocks: { $t: (k: string) => k } },
    })
    const circs = w.findAll('.stats-ring-circ')
    expect(circs.length).toBe(4)
  })

  it('variant:bars renders 4 bar rows', () => {
    const w = mount(StatsBlock, {
      props: { userId: 'test-user', variant: 'bars' },
      global: { mocks: { $t: (k: string) => k } },
    })
    // bars variant has flex-col with gap-3 containing 4 rows
    const barsContainer = w.find('.flex.flex-col.gap-3')
    expect(barsContainer.exists()).toBe(true)
    expect(barsContainer.findAll(':scope > div').length).toBe(4)
  })

  it('variant:strip renders 4 inline stat items', () => {
    const w = mount(StatsBlock, {
      props: { userId: 'test-user', variant: 'strip' },
      global: { mocks: { $t: (k: string) => k } },
    })
    const items = w.findAll('.stats-strip-item')
    expect(items.length).toBe(4)
  })

  it('variant:strip renders separator dividers', () => {
    const w = mount(StatsBlock, {
      props: { userId: 'test-user', variant: 'strip' },
      global: { mocks: { $t: (k: string) => k } },
    })
    // 3 separators between 4 items
    const seps = w.findAll('.bg-border.w-px')
    expect(seps.length).toBe(3)
  })

  it('renders all 4 i18n stat label keys in tiles', () => {
    const w = mount(StatsBlock, {
      props: { userId: 'test-user', variant: 'tiles' },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.text()).toContain('profile.stats.totalAnime')
    expect(w.text()).toContain('profile.stats.avgScore')
    expect(w.text()).toContain('profile.stats.episodesWatched')
    expect(w.text()).toContain('profile.stats.completed')
  })
})
