import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import FavoriteCharacterBlock from '../FavoriteCharacterBlock.vue'

// Mock API deps so onMounted no-ops
vi.mock('@/api/client', () => ({
  charactersApi: {
    getCharacter: vi.fn().mockResolvedValue({ data: {} }),
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
}))

describe('FavoriteCharacterBlock', () => {
  it('mounts without crashing', () => {
    const w = mount(FavoriteCharacterBlock, {
      props: { config: { character_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.exists()).toBe(true)
  })

  it('renders the favorite character title key', () => {
    const w = mount(FavoriteCharacterBlock, {
      props: { config: { character_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.text()).toContain('showcase.block.favorite_character')
  })
})
