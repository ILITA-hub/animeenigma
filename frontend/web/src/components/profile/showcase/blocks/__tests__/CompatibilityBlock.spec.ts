import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import CompatibilityBlock from '../CompatibilityBlock.vue'

const mockGetCompat = vi.fn()

vi.mock('@/api/client', () => ({
  showcaseApi: {
    getCompatibility: (...args: unknown[]) => mockGetCompat(...args),
  },
}))

describe('CompatibilityBlock', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when isOwner=true', async () => {
    mockGetCompat.mockResolvedValue({ data: { percent: 73, shared_count: 47, shared_sample: [] } })

    const w = mount(CompatibilityBlock, {
      props: { userId: 'other', isOwner: true },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    // no compat ring, no API call
    expect(w.find('.compat-ring').exists()).toBe(false)
    expect(mockGetCompat).not.toHaveBeenCalled()
  })

  it('renders percent ring when getCompatibility resolves', async () => {
    mockGetCompat.mockResolvedValue({
      data: { percent: 73, shared_count: 47, shared_sample: [] },
    })

    const w = mount(CompatibilityBlock, {
      props: { userId: 'other' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    expect(w.find('.compat-ring').exists()).toBe(true)
    expect(w.text()).toContain('73%')
  })

  it('renders nothing when self=true in response', async () => {
    mockGetCompat.mockResolvedValue({
      data: { percent: 100, shared_count: 0, shared_sample: [], self: true },
    })

    const w = mount(CompatibilityBlock, {
      props: { userId: 'me' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    expect(w.find('.compat-ring').exists()).toBe(false)
  })

  it('renders nothing when getCompatibility throws', async () => {
    mockGetCompat.mockRejectedValue(new Error('network error'))

    const w = mount(CompatibilityBlock, {
      props: { userId: 'other' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    expect(w.find('.compat-ring').exists()).toBe(false)
  })

  it('displays shared_count in the side text', async () => {
    mockGetCompat.mockResolvedValue({
      data: { percent: 55, shared_count: 12, shared_sample: [] },
    })

    const w = mount(CompatibilityBlock, {
      props: { userId: 'other' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    expect(w.text()).toContain('12')
  })

  it('renders block title i18n key', async () => {
    mockGetCompat.mockResolvedValue({
      data: { percent: 80, shared_count: 30, shared_sample: [] },
    })

    const w = mount(CompatibilityBlock, {
      props: { userId: 'other' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    expect(w.text()).toContain('showcase.block.compatibility')
  })
})
