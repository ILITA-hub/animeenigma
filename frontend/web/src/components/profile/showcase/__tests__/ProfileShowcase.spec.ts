import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

vi.mock('@/api/client', () => ({
  showcaseApi: {
    getShowcase: vi.fn().mockResolvedValue({ data: { blocks: [{ type: 'about', order: 0, config: { text: 'hi' } }] } }),
    saveShowcase: vi.fn().mockResolvedValue({ data: { blocks: [] } }),
  },
  publicApi: { getPublicWatchlistStats: vi.fn().mockResolvedValue({ data: {} }) },
  animeApi: { getById: vi.fn().mockResolvedValue({ data: {} }) },
  apiClient: { get: vi.fn().mockResolvedValue({ data: {} }) },
}))

import ProfileShowcase from '../ProfileShowcase.vue'

const mountSc = (isOwner: boolean) =>
  mount(ProfileShowcase, {
    props: { userId: 'u1', isOwner },
    global: { mocks: { $t: (k: string) => k }, stubs: { ShowcaseEditor: true } },
  })

describe('ProfileShowcase', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders fetched blocks', async () => {
    const w = mountSc(false)
    await flushPromises()
    expect(w.text()).toContain('hi')
  })

  it('shows edit button only for owner', async () => {
    const owner = mountSc(true)
    await flushPromises()
    expect(owner.text()).toContain('showcase.edit')
    const visitor = mountSc(false)
    await flushPromises()
    expect(visitor.text()).not.toContain('showcase.edit')
  })
})
