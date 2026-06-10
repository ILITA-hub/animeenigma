import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createRouter, createWebHistory } from 'vue-router'
import en from '@/locales/en.json'
import ru from '@/locales/ru.json'
import GachaBanner from './GachaBanner.vue'
import { useGachaStore } from '@/stores/gacha'
import type { BannerView, GachaWallet } from '@/api/gacha'

// ── Mock API module (prevents real HTTP) ─────────────────────────────────────
vi.mock('@/api/gacha', () => ({
  gachaApi: {
    getWallet: vi.fn(),
    getBanners: vi.fn(),
    claimDaily: vi.fn(),
    pull: vi.fn(),
    getCollection: vi.fn(),
  },
  gachaAdminApi: {
    listCards: vi.fn(),
    createCard: vi.fn(),
    updateCard: vi.fn(),
    deleteCard: vi.fn(),
    listGroups: vi.fn(),
    createGroup: vi.fn(),
    renameGroup: vi.fn(),
    deleteGroup: vi.fn(),
    addCardsToGroup: vi.fn(),
    removeCardFromGroup: vi.fn(),
    listBanners: vi.fn(),
    getBanner: vi.fn(),
    createBanner: vi.fn(),
    updateBanner: vi.fn(),
    deleteBanner: vi.fn(),
    setBannerCards: vi.fn(),
    addBannerCards: vi.fn(),
    addGroupCardsToBanner: vi.fn(),
    uploadFile: vi.fn(),
    uploadUrl: vi.fn(),
  },
  cardImageUrl: (path: string) => `/api/gacha/images/${path}`,
}))

// ── i18n ─────────────────────────────────────────────────────────────────────
const i18n = createI18n({ locale: 'en', legacy: false, messages: { en, ru } })

// ── Router stub ───────────────────────────────────────────────────────────────
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/gacha', component: { template: '<div/>' } },
    { path: '/gacha/:id', component: GachaBanner },
  ],
})

// ── Test fixtures ─────────────────────────────────────────────────────────────
function makeBanner(overrides: Partial<BannerView> = {}): BannerView {
  return {
    id: 'banner-1',
    name: 'Test Banner',
    description: 'Test banner description',
    art_path: '',
    is_standard: false,
    cards: [
      {
        id: 'card-1',
        name: 'Test Card',
        rarity: 'SSR',
        image_path: 'cards/test.webp',
        owned: false,
      },
    ],
    my_pity: 5,
    pity_threshold: 90,
    ...overrides,
  }
}

function makeWallet(balance: number): GachaWallet {
  return {
    user_id: 'u1',
    balance,
    starter_granted: false,
    daily_streak: 0,
    last_daily_at: null,
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
  }
}

describe('GachaBanner', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(async () => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.clearAllMocks()
    await router.push('/gacha/banner-1')
    await router.isReady()
  })

  /**
   * Mount with store pre-seeded. We stub the store actions that onMounted
   * calls so they're no-ops and don't overwrite our pre-seeded state.
   */
  async function mountWithState(banners: BannerView[], balance: number, loadingBanners = false) {
    const store = useGachaStore()
    // Stub lifecycle actions to be no-ops so they don't overwrite our fixtures
    vi.spyOn(store, 'fetchBanners').mockResolvedValue(undefined)
    vi.spyOn(store, 'refreshWallet').mockResolvedValue(undefined)
    // Seed state
    store.banners = banners
    store.wallet = makeWallet(balance)
    store.loadingBanners = loadingBanners

    const wrapper = mount(GachaBanner, {
      global: {
        plugins: [i18n, router, pinia],
        stubs: {
          Spinner: { template: '<div data-testid="spinner" />' },
          Badge: { template: '<span><slot /></span>' },
          Alert: { template: '<div data-testid="alert"><slot /></div>' },
          Modal: { template: '<div data-testid="modal"><slot /></div>' },
          Button: {
            props: ['disabled'],
            template: '<button :disabled="disabled" data-testid="btn"><slot /></button>',
          },
          Gem: { template: '<span />' },
        },
      },
    })
    await flushPromises()
    await wrapper.vm.$nextTick()
    return wrapper
  }

  it('shows spinner while banners are loading', async () => {
    const store = useGachaStore()
    vi.spyOn(store, 'fetchBanners').mockResolvedValue(undefined)
    vi.spyOn(store, 'refreshWallet').mockResolvedValue(undefined)
    store.loadingBanners = true

    const wrapper = mount(GachaBanner, {
      global: {
        plugins: [i18n, router, pinia],
        stubs: {
          Spinner: { template: '<div data-testid="spinner" />' },
          Badge: { template: '<span><slot /></span>' },
          Modal: { template: '<div data-testid="modal"><slot /></div>' },
          Gem: { template: '<span />' },
          Button: { props: ['disabled'], template: '<button :disabled="disabled"><slot /></button>' },
        },
      },
    })
    expect(wrapper.find('[data-testid="spinner"]').exists()).toBe(true)
  })

  it('x1 pull button is disabled when balance is below COST_X1 (100)', async () => {
    const wrapper = await mountWithState([makeBanner()], 50)
    const buttons = wrapper.findAll('[data-testid="btn"]')
    expect(buttons.length).toBeGreaterThanOrEqual(1)
    expect(buttons[0].attributes('disabled')).toBeDefined()
  })

  it('x10 pull button is disabled when balance is below COST_X10 (900)', async () => {
    const wrapper = await mountWithState([makeBanner()], 500)
    const buttons = wrapper.findAll('[data-testid="btn"]')
    expect(buttons.length).toBeGreaterThanOrEqual(2)
    expect(buttons[1].attributes('disabled')).toBeDefined()
  })

  it('x1 pull button is enabled when balance >= 100', async () => {
    const wrapper = await mountWithState([makeBanner()], 100)
    const buttons = wrapper.findAll('[data-testid="btn"]')
    expect(buttons[0].attributes('disabled')).toBeUndefined()
  })

  it('x10 pull button is enabled when balance >= 900', async () => {
    const wrapper = await mountWithState([makeBanner()], 1000)
    const buttons = wrapper.findAll('[data-testid="btn"]')
    expect(buttons[1].attributes('disabled')).toBeUndefined()
  })

  it('renders banner name from store', async () => {
    const wrapper = await mountWithState([makeBanner({ name: 'My Special Banner' })], 0)
    expect(wrapper.html()).toContain('My Special Banner')
  })
})
