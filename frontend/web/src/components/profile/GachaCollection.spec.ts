import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import GachaCollection from './GachaCollection.vue'
import { useGachaStore } from '@/stores/gacha'
import type { CollectionView } from '@/api/gacha'

// ── i18n stub ─────────────────────────────────────────────────────────────────
const i18n = createI18n({ locale: 'en', legacy: false, messages: { en, ru } })

// ── Mock router-link (not needed here, no router-links in component) ─────────

// ── Test fixtures ─────────────────────────────────────────────────────────────
function makeCollection(): CollectionView {
  return {
    cards: [
      {
        card: {
          id: 'card-ssr-1',
          name: 'SSR Hero',
          source_title: 'Anime A',
          image_path: 'cards/ssr-1.webp',
          rarity: 'SSR',
          enabled: true,
          created_at: '2026-06-01T00:00:00Z',
          updated_at: '2026-06-01T00:00:00Z',
        },
        owned: true,
        count: 2,
        first_obtained_at: '2026-06-05T00:00:00Z',
      },
      {
        card: {
          id: 'card-sr-1',
          name: 'SR Knight',
          source_title: 'Anime B',
          image_path: 'cards/sr-1.webp',
          rarity: 'SR',
          enabled: true,
          created_at: '2026-06-01T00:00:00Z',
          updated_at: '2026-06-01T00:00:00Z',
        },
        owned: false,
        count: 0,
      },
      {
        card: {
          id: 'card-r-1',
          name: 'R Mage',
          source_title: 'Anime C',
          image_path: 'cards/r-1.webp',
          rarity: 'R',
          enabled: true,
          created_at: '2026-06-01T00:00:00Z',
          updated_at: '2026-06-01T00:00:00Z',
        },
        owned: true,
        count: 1,
      },
      {
        card: {
          id: 'card-n-1',
          name: 'N Minion',
          source_title: 'Anime D',
          image_path: 'cards/n-1.webp',
          rarity: 'N',
          enabled: true,
          created_at: '2026-06-01T00:00:00Z',
          updated_at: '2026-06-01T00:00:00Z',
        },
        owned: false,
        count: 0,
      },
    ],
    progress: {
      SSR: { owned: 1, total: 1 },
      SR:  { owned: 0, total: 1 },
      R:   { owned: 1, total: 1 },
      N:   { owned: 0, total: 1 },
    },
  }
}

describe('GachaCollection', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  function mountComponent() {
    return mount(GachaCollection, {
      global: {
        plugins: [i18n],
        stubs: {
          Spinner: { template: '<div data-testid="spinner" />' },
          Alert: { template: '<div data-testid="alert"><slot /></div>' },
        },
      },
    })
  }

  it('shows spinner when loadingCollection is true', () => {
    const store = useGachaStore()
    store.loadingCollection = true
    const wrapper = mountComponent()
    expect(wrapper.find('[data-testid="spinner"]').exists()).toBe(true)
  })

  it('renders owned cards with data-testid collection-card-owned', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()
    const ownedCards = wrapper.findAll('[data-testid="collection-card-owned"]')
    expect(ownedCards.length).toBeGreaterThan(0)
  })

  it('renders unowned cards with data-testid collection-card-unowned and brightness(0) filter', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()
    const unownedCards = wrapper.findAll('[data-testid="collection-card-unowned"]')
    expect(unownedCards.length).toBeGreaterThan(0)
    // Check that the img inside unowned has brightness(0) filter
    const img = unownedCards[0].find('img')
    expect(img.attributes('style')).toContain('brightness(0)')
  })

  it('unowned cards show ??? text', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()
    expect(wrapper.html()).toContain('???')
  })

  it('owned cards with count > 1 show a dupe count badge', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()
    // SSR Hero has count=2
    expect(wrapper.html()).toContain('×2')
  })

  it('shows empty state when collection has no cards', async () => {
    const store = useGachaStore()
    store.collection = { cards: [], progress: { SSR: { owned: 0, total: 0 }, SR: { owned: 0, total: 0 }, R: { owned: 0, total: 0 }, N: { owned: 0, total: 0 } } }
    store.loadingCollection = false
    const wrapper = mountComponent()
    // The empty state message key is gacha.collection_empty
    expect(wrapper.html()).toContain('No cards in collection yet')
  })
})
