import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import CardCollectionBlock from '../CardCollectionBlock.vue'
import type { GachaCard } from '@/api/gacha'

// ─── Helpers ────────────────────────────────────────────────────────────────

function makeCards(n: number): GachaCard[] {
  const rarities = ['N', 'R', 'SR', 'SSR'] as const
  return Array.from({ length: n }, (_, i) => ({
    id: `card-${i + 1}`,
    name: `Card ${i + 1}`,
    source_title: `Anime ${i + 1}`,
    image_path: `cards/card-${i + 1}.webp`,
    rarity: rarities[i % rarities.length],
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }))
}

function makeCollection(n: number) {
  return makeCards(n).map((card) => ({ card, owned: true }))
}

// ─── Mock — use static vi.fn() inside factory; reassign via vi.mocked ────────

vi.mock('@/api/gacha', () => ({
  gachaApi: {
    getCollection: vi.fn().mockResolvedValue({
      data: { data: { cards: [], progress: {} } },
    }),
  },
  cardImageUrl: vi.fn((p: string) => p ?? ''),
}))

// ─── Tests ───────────────────────────────────────────────────────────────────

describe('CardCollectionBlock', () => {
  // Pull the mocked fn after import is resolved
  let getCollectionMock: ReturnType<typeof vi.fn>

  beforeEach(async () => {
    const gacha = await import('@/api/gacha')
    getCollectionMock = vi.mocked(gacha.gachaApi.getCollection)
    // Reset to empty collection by default
    getCollectionMock.mockResolvedValue({
      data: { data: { cards: [], progress: {} } },
    })
  })

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

  it('renders row layout by default (no variant)', async () => {
    const ids = makeCards(3).map((c) => c.id)
    getCollectionMock.mockResolvedValue({
      data: { data: { cards: makeCollection(3), progress: {} } },
    })
    const w = mount(CardCollectionBlock, {
      props: { config: { card_ids: ids } },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.find('.cc-row').exists()).toBe(true)
  })

  it('variant fan with 5 cards renders 5 fan-card elements', async () => {
    const ids = makeCards(5).map((c) => c.id)
    getCollectionMock.mockResolvedValue({
      data: { data: { cards: makeCollection(5), progress: {} } },
    })
    const w = mount(CardCollectionBlock, {
      props: { config: { card_ids: ids }, variant: 'fan' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.findAll('.cc-fan-card')).toHaveLength(5)
  })

  it('variant hero with 3 cards renders the info panel', async () => {
    const ids = makeCards(3).map((c) => c.id)
    getCollectionMock.mockResolvedValue({
      data: { data: { cards: makeCollection(3), progress: {} } },
    })
    const w = mount(CardCollectionBlock, {
      props: { config: { card_ids: ids }, variant: 'hero' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.find('.cc-hero-panel').exists()).toBe(true)
  })

  it('variant grid renders cards in a grid container', async () => {
    const ids = makeCards(4).map((c) => c.id)
    getCollectionMock.mockResolvedValue({
      data: { data: { cards: makeCollection(4), progress: {} } },
    })
    const w = mount(CardCollectionBlock, {
      props: { config: { card_ids: ids }, variant: 'grid' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.find('.cc-grid').exists()).toBe(true)
    expect(w.findAll('.cc-gcard')).toHaveLength(4)
  })

  it('variant tilt3d renders cards with tilt wrapper', async () => {
    const ids = makeCards(3).map((c) => c.id)
    getCollectionMock.mockResolvedValue({
      data: { data: { cards: makeCollection(3), progress: {} } },
    })
    const w = mount(CardCollectionBlock, {
      props: { config: { card_ids: ids }, variant: 'tilt3d' },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.find('.cc-tilt3d').exists()).toBe(true)
  })

  it('clicking a card opens the dialog', async () => {
    const ids = makeCards(2).map((c) => c.id)
    getCollectionMock.mockResolvedValue({
      data: { data: { cards: makeCollection(2), progress: {} } },
    })
    const w = mount(CardCollectionBlock, {
      props: { config: { card_ids: ids } },
      global: { mocks: { $t: (k: string) => k } },
      attachTo: document.body,
    })
    await flushPromises()
    const firstCard = w.find('.cc-gcard')
    await firstCard.trigger('click')
    // Dialog should exist in the document (Teleport to body)
    expect(document.querySelector('.cc-dialog')).not.toBeNull()
    w.unmount()
  })

  it('shows empty state when no cards loaded', async () => {
    const w = mount(CardCollectionBlock, {
      props: { config: { card_ids: ['nonexistent'] } },
      global: { mocks: { $t: (k: string) => k } },
    })
    await flushPromises()
    expect(w.text()).toContain('showcase.empty')
  })
})
