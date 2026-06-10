import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createRouter, createWebHistory, type Router } from 'vue-router'
import en from '@/locales/en.json'
import Gacha from './Gacha.vue'
import { useGachaStore } from '@/stores/gacha'
import type { BannerView, GachaWallet } from '@/api/gacha'

vi.mock('@/api/gacha', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/gacha')>()
  return { ...actual, cardImageUrl: (p: string) => (p ? `/api/gacha/images/${p}` : '') }
})

// Force ceremony OFF by default (jsdom: matchMedia returns false → motion on).
// Individual tests can re-mock useMediaQuery.
vi.mock('@vueuse/core', () => ({ useMediaQuery: () => ({ value: false }) }))

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

const router: Router = createRouter({
  history: createWebHistory(),
  routes: [{ path: '/gacha', component: Gacha }],
})

function makeBanner(over: Partial<BannerView> = {}): BannerView {
  return {
    id: 'b1',
    name: 'Banner One',
    description: 'desc',
    backdrop_path: '',
    is_standard: false,
    cards: [],
    my_pity: 5,
    pity_threshold: 90,
    ...over,
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

const stubs = {
  Spinner: { template: '<div data-testid="spinner" />' },
  Alert: { template: '<div data-testid="alert"><slot /></div>' },
  Button: { props: ['disabled'], template: '<button :disabled="disabled"><slot /></button>' },
  Gem: { template: '<span />' },
  GachaSlider: {
    props: ['banners', 'modelValue'],
    template: '<div data-testid="slider">idx:{{ modelValue }}</div>',
  },
  SpinDock: { props: ['pity', 'pityThreshold', 'balance'], template: '<div data-testid="dock" />' },
  DropsModal: { props: ['modelValue'], template: '<div data-testid="drops" />' },
  GemCeremony: { props: ['active', 'topTier'], template: '<div data-testid="ceremony" />' },
  CardViewer3D: { props: ['active', 'cards'], template: '<div data-testid="viewer" />' },
  PullSummary: { props: ['modelValue'], template: '<div data-testid="summary" />' },
}

let pinia: ReturnType<typeof createPinia>

async function mountView(banners: BannerView[], balance = 1000) {
  const store = useGachaStore()
  vi.spyOn(store, 'fetchBanners').mockResolvedValue(undefined)
  vi.spyOn(store, 'refreshWallet').mockResolvedValue(undefined)
  store.banners = banners
  store.wallet = makeWallet(balance)
  const wrapper = mount(Gacha, { global: { plugins: [i18n, router, pinia], stubs } })
  await flushPromises()
  return wrapper
}

describe('Gacha view (Variant C)', () => {
  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.clearAllMocks()
  })

  it('renders the slider + dock when banners exist', async () => {
    await router.push('/gacha')
    await router.isReady()
    const w = await mountView([makeBanner()])
    expect(w.find('[data-testid="slider"]').exists()).toBe(true)
    expect(w.find('[data-testid="dock"]').exists()).toBe(true)
  })

  it('shows the empty state when there are no banners', async () => {
    await router.push('/gacha')
    await router.isReady()
    const w = await mountView([])
    expect(w.html()).toContain('No active banners')
  })

  it('preselects the banner from the ?banner= query', async () => {
    await router.push('/gacha?banner=b2')
    await router.isReady()
    const w = await mountView([
      makeBanner({ id: 'b1', name: 'First' }),
      makeBanner({ id: 'b2', name: 'Second' }),
      makeBanner({ id: 'b3', name: 'Third' }),
    ])
    // Slider stub renders the bound modelValue index.
    expect(w.find('[data-testid="slider"]').text()).toContain('idx:1')
  })

  it('defaults to index 0 when the query banner id is unknown', async () => {
    await router.push('/gacha?banner=missing')
    await router.isReady()
    const w = await mountView([makeBanner({ id: 'b1' }), makeBanner({ id: 'b2' })])
    expect(w.find('[data-testid="slider"]').text()).toContain('idx:0')
  })

  it('renders the gem ceremony component (pull-flow surface)', async () => {
    await router.push('/gacha')
    await router.isReady()
    const w = await mountView([makeBanner()])
    expect(w.find('[data-testid="ceremony"]').exists()).toBe(true)
    expect(w.find('[data-testid="viewer"]').exists()).toBe(true)
  })
})
