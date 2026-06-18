import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import OpEdBlock from '../OpEdBlock.vue'

const mockGet = vi.fn()

vi.mock('@/api/client', () => ({
  themesApi: {
    get: (...args: unknown[]) => mockGet(...args),
  },
}))

const makeTheme = (id: string, type = 'OP') => ({
  id,
  poster_url: `https://example.com/${id}.jpg`,
  anime_name: `Anime ${id}`,
  song_title: `Song ${id}`,
  artist_name: `Artist ${id}`,
  theme_type: type,
})

describe('OpEdBlock', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when config has no theme_ids', async () => {
    const w = mount(OpEdBlock, {
      props: { config: { theme_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.find('.grid').exists()).toBe(false)
  })

  it('renders one card per resolved theme_id', async () => {
    mockGet
      .mockResolvedValueOnce({ data: makeTheme('t1', 'OP') })
      .mockResolvedValueOnce({ data: makeTheme('t2', 'ED') })

    const w = mount(OpEdBlock, {
      props: { config: { theme_ids: ['t1', 't2'] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    const grid = w.find('.grid')
    expect(grid.exists()).toBe(true)
    expect(grid.findAll(':scope > div').length).toBe(2)
  })

  it('shows OP badge for OP theme and ED badge for ED theme', async () => {
    mockGet
      .mockResolvedValueOnce({ data: makeTheme('t1', 'OP') })
      .mockResolvedValueOnce({ data: makeTheme('t2', 'ED') })

    const w = mount(OpEdBlock, {
      props: { config: { theme_ids: ['t1', 't2'] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    const badges = w.findAll('span.absolute')
    expect(badges[0].text()).toBe('OP')
    expect(badges[1].text()).toBe('ED')
  })

  it('skips failed theme fetches, renders only successes', async () => {
    mockGet
      .mockResolvedValueOnce({ data: makeTheme('t1', 'OP') })
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValueOnce({ data: makeTheme('t3', 'ED') })

    const w = mount(OpEdBlock, {
      props: { config: { theme_ids: ['t1', 't-bad', 't3'] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    const grid = w.find('.grid')
    expect(grid.findAll(':scope > div').length).toBe(2)
  })

  it('renders block title i18n key', async () => {
    mockGet.mockResolvedValueOnce({ data: makeTheme('t1') })

    const w = mount(OpEdBlock, {
      props: { config: { theme_ids: ['t1'] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    expect(w.text()).toContain('showcase.block.op_ed')
  })

  it('renders song title and anime name in card meta', async () => {
    mockGet.mockResolvedValueOnce({ data: makeTheme('t1', 'OP') })

    const w = mount(OpEdBlock, {
      props: { config: { theme_ids: ['t1'] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()

    expect(w.text()).toContain('Song t1')
    expect(w.text()).toContain('Anime t1')
  })
})
