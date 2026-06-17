import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CardCollectionBlock from '../CardCollectionBlock.vue'

// Mock gacha API so onMounted no-ops on empty config
vi.mock('@/api/gacha', () => ({
  gachaApi: {
    getCollection: vi.fn().mockResolvedValue({ data: { data: { cards: [], progress: {} } } }),
  },
  cardImageUrl: vi.fn((p: string) => p ?? ''),
}))

describe('CardCollectionBlock', () => {
  it('mounts without crashing', () => {
    const w = mount(CardCollectionBlock, {
      props: { config: { card_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.exists()).toBe(true)
  })

  it('renders the card collection title key', () => {
    const w = mount(CardCollectionBlock, {
      props: { config: { card_ids: [] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    expect(w.text()).toContain('showcase.block.card_collection')
  })
})
