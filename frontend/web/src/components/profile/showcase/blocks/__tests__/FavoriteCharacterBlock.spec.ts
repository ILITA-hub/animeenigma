import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import FavoriteCharacterBlock from '../FavoriteCharacterBlock.vue'

const mockChar = (id: string, name: string, image: string) => ({
  shikimori_id: id,
  name,
  name_ru: `${name} RU`,
  name_jp: `${name} JP`,
  poster_url: image,
})

const getCharacterMock = vi.fn()

vi.mock('@/api/client', () => ({
  charactersApi: {
    getCharacter: (...args: unknown[]) => getCharacterMock(...args),
  },
}))

vi.mock('@/components/anime/CharacterCard.vue', () => ({
  default: { template: '<div data-testid="character-card" />' },
}))

vi.mock('@/utils/title', () => ({
  getLocalizedTitle: vi.fn((n) => n ?? ''),
}))

vi.mock('@/composables/useImageProxy', () => ({
  getImageUrl: vi.fn((u) => u ?? ''),
  // CharacterImage (child) resizes/falls back through these.
  cardPosterUrl: (u: string) => u,
  getImageFallbackUrl: (u: string) => u,
}))

describe('FavoriteCharacterBlock', () => {
  it('mounts without crashing', () => {
    getCharacterMock.mockResolvedValue({ data: {} })
    const w = mount(FavoriteCharacterBlock, {
      props: { config: { character_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.exists()).toBe(true)
  })

  it('renders the favorite character title key', () => {
    getCharacterMock.mockResolvedValue({ data: {} })
    const w = mount(FavoriteCharacterBlock, {
      props: { config: { character_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.text()).toContain('showcase.block.favorite_character')
  })

  it('shows empty message when no ids provided', () => {
    getCharacterMock.mockResolvedValue({ data: {} })
    const w = mount(FavoriteCharacterBlock, {
      props: { config: { character_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.text()).toContain('showcase.empty')
  })

  it('default (circles) variant renders CharacterCard for each resolved character', async () => {
    getCharacterMock.mockImplementation((id: string) =>
      Promise.resolve({ data: { data: mockChar(id, `Char ${id}`, `https://img/${id}.jpg`) } }),
    )
    const w = mount(FavoriteCharacterBlock, {
      props: { config: { character_ids: [1, 2, 3] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    const cards = w.findAll('[data-testid="character-card"]')
    expect(cards.length).toBe(3)
  })

  it('portraits variant renders name overlays for each resolved character', async () => {
    getCharacterMock.mockImplementation((id: string) =>
      Promise.resolve({ data: { data: mockChar(id, `Hero ${id}`, `https://img/${id}.jpg`) } }),
    )
    const w = mount(FavoriteCharacterBlock, {
      props: { config: { character_ids: [1, 2] }, variant: 'portraits' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    const names = w.findAll('[data-testid="portrait-name"]')
    expect(names.length).toBe(2)
    expect(names[0].text()).toContain('Hero 1')
  })

  it('hero variant renders big card and ranked list items', async () => {
    getCharacterMock.mockImplementation((id: string) =>
      Promise.resolve({ data: { data: mockChar(id, `Char ${id}`, `https://img/${id}.jpg`) } }),
    )
    const w = mount(FavoriteCharacterBlock, {
      props: { config: { character_ids: [1, 2, 3] }, variant: 'hero' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.find('[data-testid="hero-big-card"]').exists()).toBe(true)
    const listItems = w.findAll('[data-testid="hero-list-item"]')
    expect(listItems.length).toBeGreaterThan(0)
  })

  it('hex variant renders clipped hexagon containers for each character', async () => {
    getCharacterMock.mockImplementation((id: string) =>
      Promise.resolve({ data: { data: mockChar(id, `Char ${id}`, `https://img/${id}.jpg`) } }),
    )
    const w = mount(FavoriteCharacterBlock, {
      props: { config: { character_ids: [1, 2, 3, 4] }, variant: 'hex' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    const hexItems = w.findAll('[data-testid="hex-item"]')
    expect(hexItems.length).toBe(4)
  })

  it('hero variant shows ♥ badge on the big card', async () => {
    getCharacterMock.mockImplementation((id: string) =>
      Promise.resolve({ data: { data: mockChar(id, `Char ${id}`, `https://img/${id}.jpg`) } }),
    )
    const w = mount(FavoriteCharacterBlock, {
      props: { config: { character_ids: [1, 2] }, variant: 'hero' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.text()).toContain('♥')
  })
})
