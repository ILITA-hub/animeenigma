/**
 * Gate-wiring test for ProfileShowcase in Profile.vue.
 *
 * Approach: mount Profile.vue with all heavy dependencies stubbed out
 * and the profile wall gate mocked to `false` (closed, dark-ship default).
 * Assert that ProfileShowcase is absent in the DOM.
 *
 * Full behavioural coverage of ProfileShowcase itself lives in
 * src/components/profile/showcase/__tests__/ (Task 8). This test focuses
 * only on verifying the gate wiring at the integration seam.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createRouter, createWebHistory } from 'vue-router'
import en from '@/locales/en.json'
import Profile from '../Profile.vue'

// --------------------------------------------------------------------------
// Mock the gate to CLOSED so the assertion is deterministic regardless of
// environment variables or auth state.
// --------------------------------------------------------------------------
vi.mock('@/utils/profileWallGate', () => ({
  PROFILE_WALL_ADMIN_ONLY: true,
  useProfileWallVisible: () => ({ value: false }),
}))

// Mock the gacha gate as well (used by Profile.vue's tab logic)
vi.mock('@/utils/gachaGate', () => ({
  GACHA_ADMIN_ONLY: true,
  useGachaVisible: () => ({ value: false }),
}))

// Mock all API modules to prevent real network calls
vi.mock('@/api/client', () => ({
  userApi: {
    getProfile: vi.fn().mockResolvedValue({ success: true, data: null }),
    getSessions: vi.fn().mockResolvedValue({ success: true, data: [] }),
  },
  publicApi: {
    getUserProfile: vi.fn().mockResolvedValue({ success: true, data: null }),
    getPublicWatchlist: vi.fn().mockResolvedValue({ success: true, data: { items: [], total: 0 } }),
    getPublicWatchlistStats: vi.fn().mockResolvedValue({ success: true, data: {} }),
    getPublicWatchlistFacets: vi.fn().mockResolvedValue({ success: true, data: {} }),
  },
  showcaseApi: {
    getShowcase: vi.fn().mockResolvedValue({ success: true, data: null }),
  },
}))

vi.mock('@/composables/useToast', () => ({ useToast: () => ({ show: vi.fn() }) }))
vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => ({ confirm: vi.fn() }) }))
vi.mock('@/composables/useImageProxy', () => ({ getImageUrl: (u: string) => u }))
vi.mock('@/composables/useContextMenu', () => ({
  useContextMenu: () => ({ contextMenu: { value: null }, openContextMenu: vi.fn(), closeContextMenu: vi.fn() }),
}))
vi.mock('@/composables/useSkipIntroSettings', () => ({
  useSkipIntroSettings: () => ({ skipIntroEnabled: { value: false } }),
}))
vi.mock('@/utils/title', () => ({ getLocalizedTitle: (a: Record<string, string>) => a?.russian ?? '' }))
vi.mock('@/utils/toCardModel', () => ({ fromWatchlistEntry: vi.fn() }))
vi.mock('@/types/watchlist-facets', () => ({
  EMPTY_FILTER_STATE: {},
  filterParams: () => ({}),
  filterKey: () => '',
  activeFilterCount: () => 0,
}))

// --------------------------------------------------------------------------
// Stubs for heavy child components
// --------------------------------------------------------------------------
const globalStubs = {
  Spinner: { template: '<div data-testid="stub-spinner" />' },
  Avatar: { props: ['src', 'name', 'size'], template: '<div><slot /></div>' },
  Badge: { template: '<span><slot /></span>' },
  Button: { template: '<button><slot /></button>' },
  Checkbox: { template: '<input type="checkbox" />' },
  Chip: { template: '<span><slot /></span>' },
  EmptyState: { template: '<div data-testid="stub-empty-state"><slot /><slot name="action" /></div>' },
  Input: { template: '<input />' },
  Modal: { props: ['modelValue'], template: '<div v-if="modelValue"><slot /></div>' },
  Tabs: { props: ['modelValue', 'tabs'], template: '<div data-testid="stub-tabs"><slot name="watchlist" /></div>' },
  Select: { template: '<select />' },
  PaginationBar: { template: '<div />' },
  ScoreDiamond: { template: '<span />' },
  SegmentedControl: { template: '<div />' },
  ActiveSessionsCard: { template: '<div />' },
  TimezoneCard: { template: '<div />' },
  GachaCollection: { template: '<div />' },
  AnimeContextMenu: { template: '<div />' },
  PosterCard: { template: '<div />' },
  WatchlistRow: { template: '<div />' },
  WatchlistFilters: { template: '<div />' },
  WatchlistBulkBar: { template: '<div />' },
  ProfileShowcase: { name: 'ProfileShowcase', props: ['userId', 'isOwner'], template: '<div data-testid="profile-showcase" />' },
  // lucide icons
  TriangleAlert: { template: '<svg />' },
  Share2: { template: '<svg />' },
  Pencil: { template: '<svg />' },
  RouterLink: { template: '<a><slot /></a>' },
}

const i18n = createI18n({ locale: 'en', legacy: false, messages: { en } })

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/profile/:publicId', name: 'public-profile', component: Profile },
    { path: '/u/:publicId', name: 'public-profile-u', component: Profile },
  ],
})

// Auth and watchlist stores are provided by pinia plugin; Profile.vue imports
// them via useAuthStore/useWatchlistStore from their canonical paths — no stub
// needed here since we rely on pinia auto-creation with empty default state.

describe('Profile.vue — ProfileShowcase gate wiring', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(async () => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.clearAllMocks()
    await router.push('/profile/testuser')
    await router.isReady()
  })

  it('does NOT render ProfileShowcase when the gate is closed (dark-ship default)', async () => {
    const w = mount(Profile, {
      global: {
        plugins: [i18n, router, pinia],
        stubs: globalStubs,
      },
    })
    await flushPromises()
    // Gate mock returns false → component must be absent from DOM
    expect(w.findComponent({ name: 'ProfileShowcase' }).exists()).toBe(false)
  })
})
