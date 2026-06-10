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
          // Stub CardViewer3D so we can assert on its props without rendering canvas/teleport
          CardViewer3D: {
            props: ['active', 'cards', 'startIndex', 'mode'],
            emits: ['done'],
            template: '<div data-testid="card-viewer-3d-stub" :data-active="active" :data-start="startIndex" :data-mode="mode" :data-cards="cards?.length" @click="$emit(\'done\')" />',
          },
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

  it('renders OWNED cards only — no unowned/silhouette cards', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()
    // No unowned testid markup should ever render.
    expect(wrapper.findAll('[data-testid="collection-card-unowned"]')).toHaveLength(0)
    // The fixture has 2 owned cards (SSR Hero, R Mage) and 2 unowned.
    expect(wrapper.findAll('[data-testid="collection-card-owned"]')).toHaveLength(2)
  })

  it('does NOT render ??? placeholders or brightness(0) silhouettes', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()
    expect(wrapper.html()).not.toContain('???')
    expect(wrapper.html()).not.toContain('brightness(0)')
  })

  it('renders only owned card names (not unowned ones)', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()
    expect(wrapper.html()).toContain('SSR Hero') // owned
    expect(wrapper.html()).toContain('R Mage')   // owned
    expect(wrapper.html()).not.toContain('SR Knight') // unowned → hidden
    expect(wrapper.html()).not.toContain('N Minion')  // unowned → hidden
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

  // ── Inspect mode ─────────────────────────────────────────────────────────

  it('CardViewer3D is rendered in inspect mode', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()
    const stub = wrapper.find('[data-testid="card-viewer-3d-stub"]')
    expect(stub.exists()).toBe(true)
    expect(stub.attributes('data-mode')).toBe('inspect')
  })

  it('clicking a card opens the viewer with active=true', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()

    // Viewer should start inactive
    const stub = wrapper.find('[data-testid="card-viewer-3d-stub"]')
    expect(stub.attributes('data-active')).toBe('false')

    // Click the first owned card
    const firstCard = wrapper.find('[data-testid="collection-card-owned"]')
    await firstCard.trigger('click')

    expect(stub.attributes('data-active')).toBe('true')
  })

  it('clicking the SSR card opens viewer at startIndex=0 (SSR is first in order)', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()

    // First owned card in the fixture is SSR Hero (SSR section comes first)
    const firstCard = wrapper.find('[data-testid="collection-card-owned"]')
    await firstCard.trigger('click')

    const stub = wrapper.find('[data-testid="card-viewer-3d-stub"]')
    expect(stub.attributes('data-start')).toBe('0')
  })

  it('viewer done event closes the viewer (active goes back to false)', async () => {
    const store = useGachaStore()
    store.collection = makeCollection()
    store.loadingCollection = false
    const wrapper = mountComponent()

    // Open viewer
    const firstCard = wrapper.find('[data-testid="collection-card-owned"]')
    await firstCard.trigger('click')

    const stub = wrapper.find('[data-testid="card-viewer-3d-stub"]')
    expect(stub.attributes('data-active')).toBe('true')

    // Simulate done (our stub emits done when clicked)
    await stub.trigger('click')
    expect(stub.attributes('data-active')).toBe('false')
  })
})
