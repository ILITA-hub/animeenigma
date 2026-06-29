/**
 * Showcase tab/button visibility wiring in Profile.vue (per-user opt-in model).
 *
 * One rule: the showcase TAB is shown ⟺ `showcase_state === 'visible'` (for
 * everyone). The only owner-specific surface is a header button next to Share,
 * shown when the owner's showcase is NOT visible (`none` → "Add Showcase",
 * `hidden` → "Edit Showcase"). Everything stays under the `profileWallVisible`
 * dark-ship gate.
 *
 * Full behavioural coverage of the showcase components themselves lives in
 * src/components/profile/showcase/__tests__/. These tests focus on the
 * Profile.vue integration seam.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createRouter, createWebHistory } from 'vue-router'
import en from '@/locales/en.json'
import Profile from '../Profile.vue'

// --------------------------------------------------------------------------
// Configurable gate mock — default OPEN so the opt-in model is exercised.
// --------------------------------------------------------------------------
let gateOpen = true
vi.mock('@/utils/profileWallGate', () => ({
  PROFILE_WALL_ADMIN_ONLY: true,
  useProfileWallVisible: () => ({ get value() { return gateOpen } }),
}))

vi.mock('@/utils/gachaGate', () => ({
  GACHA_ADMIN_ONLY: true,
  useGachaVisible: () => ({ value: false }),
}))

// --------------------------------------------------------------------------
// Configurable auth mock — flip `authUser` per test to be owner / visitor.
// --------------------------------------------------------------------------
let authUser: Record<string, unknown> | null = null
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get user() { return authUser },
    fetchUser: vi.fn().mockResolvedValue(undefined),
  }),
}))

// Watchlist store (Profile.vue uses it heavily) — minimal stub.
vi.mock('@/stores/watchlist', () => ({
  useWatchlistStore: () => ({
    watchlistMap: new Map(),
    entries: [],
    fetchStatuses: vi.fn().mockResolvedValue(undefined),
    invalidate: vi.fn(),
    setStatusOptimistic: vi.fn().mockResolvedValue(undefined),
    setScoreOptimistic: vi.fn().mockResolvedValue(undefined),
    removeEntryOptimistic: vi.fn().mockResolvedValue(undefined),
  }),
}))

// Configurable showcase_state on the public profile.
let publicProfile: Record<string, unknown> = {}
vi.mock('@/api/client', () => ({
  userApi: {
    getProfile: vi.fn().mockResolvedValue({ success: true, data: null }),
    getSessions: vi.fn().mockResolvedValue({ success: true, data: [] }),
    getWatchlist: vi.fn().mockResolvedValue({ data: { items: [], total: 0 } }),
    getWatchlistFacets: vi.fn().mockResolvedValue({ data: {} }),
    getSyncStatus: vi.fn().mockResolvedValue({ data: {} }),
    hasApiKey: vi.fn().mockResolvedValue({ data: { has_key: false } }),
  },
  publicApi: {
    getUserProfile: vi.fn().mockImplementation(() => Promise.resolve({ data: publicProfile })),
    getPublicWatchlist: vi.fn().mockResolvedValue({ data: { items: [], total: 0 } }),
    getPublicWatchlistStats: vi.fn().mockResolvedValue({ data: {} }),
    getPublicWatchlistFacets: vi.fn().mockResolvedValue({ data: {} }),
  },
  showcaseApi: {
    getShowcase: vi.fn().mockResolvedValue({ data: { blocks: [], enabled: false } }),
    saveShowcase: vi.fn().mockResolvedValue({ data: { blocks: [], enabled: false } }),
  },
}))

vi.mock('@/composables/useToast', () => ({ useToast: () => ({ show: vi.fn(), push: vi.fn() }) }))
vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => ({ confirm: vi.fn() }) }))
vi.mock('@/composables/useImageProxy', () => ({ getImageUrl: (u: string) => u }))
vi.mock('@/composables/useContextMenu', () => ({
  useContextMenu: () => ({ contextMenu: { value: null }, openAtElement: vi.fn(), onTouchstart: vi.fn(), onTouchmove: vi.fn(), onTouchend: vi.fn() }),
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
// Tabs stub: renders the showcase slot AND exposes the tab list so we can
// assert whether the 'showcase' tab was registered.
// --------------------------------------------------------------------------
const TabsStub = {
  name: 'Tabs',
  props: ['modelValue', 'tabs'],
  template: `<div data-testid="stub-tabs">
    <span v-for="t in tabs" :key="t.value" :data-tab="t.value" />
    <slot name="showcase" />
    <slot name="watchlist" />
  </div>`,
}

const globalStubs = {
  Spinner: { template: '<div />' },
  Avatar: { props: ['src', 'name', 'size'], template: '<div><slot /></div>' },
  Badge: { template: '<span><slot /></span>' },
  Button: { template: '<button><slot /></button>' },
  Checkbox: { template: '<input type="checkbox" />' },
  Chip: { template: '<span><slot /></span>' },
  EmptyState: { template: '<div><slot /><slot name="action" /></div>' },
  Input: { template: '<input />' },
  Modal: { props: ['modelValue'], template: '<div v-if="modelValue"><slot /></div>' },
  Tabs: TabsStub,
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
  ProfileShowcase: { name: 'ProfileShowcase', props: ['userId', 'isOwner', 'autoEdit'], template: '<div data-testid="profile-showcase" />' },
  TriangleAlert: { template: '<svg />' },
  Share2: { template: '<svg />' },
  LayoutGrid: { template: '<svg />' },
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

async function mountProfile(publicIdParam = 'testuser') {
  await router.push(`/profile/${publicIdParam}`)
  await router.isReady()
  const w = mount(Profile, { global: { plugins: [i18n, router, createPinia()], stubs: globalStubs } })
  await flushPromises()
  return w
}

describe('Profile.vue — showcase opt-in visibility', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    gateOpen = true
    authUser = null
    publicProfile = {}
  })

  it('gate closed → no showcase tab, no entry button (dark-ship default)', async () => {
    gateOpen = false
    publicProfile = { id: 'v1', username: 'V', public_id: 'testuser', showcase_state: 'visible' }
    const w = await mountProfile()
    expect(w.find('[data-tab="showcase"]').exists()).toBe(false)
    expect(w.findComponent({ name: 'ProfileShowcase' }).exists()).toBe(false)
    expect(w.text()).not.toContain('Add Showcase')
    expect(w.text()).not.toContain('Edit Showcase')
  })

  it('visitor + visible → showcase tab present', async () => {
    publicProfile = { id: 'v1', username: 'V', public_id: 'testuser', showcase_state: 'visible' }
    const w = await mountProfile()
    expect(w.find('[data-tab="showcase"]').exists()).toBe(true)
    expect(w.findComponent({ name: 'ProfileShowcase' }).exists()).toBe(true)
    // visitor never sees the owner entry button
    expect(w.text()).not.toContain('Add Showcase')
    expect(w.text()).not.toContain('Edit Showcase')
  })

  it('visitor + hidden → no tab, no button', async () => {
    publicProfile = { id: 'v1', username: 'V', public_id: 'testuser', showcase_state: 'hidden' }
    const w = await mountProfile()
    expect(w.find('[data-tab="showcase"]').exists()).toBe(false)
    expect(w.findComponent({ name: 'ProfileShowcase' }).exists()).toBe(false)
    expect(w.text()).not.toContain('Edit Showcase')
  })

  it('visitor + none → no tab, no button', async () => {
    publicProfile = { id: 'v1', username: 'V', public_id: 'testuser', showcase_state: 'none' }
    const w = await mountProfile()
    expect(w.find('[data-tab="showcase"]').exists()).toBe(false)
    expect(w.text()).not.toContain('Add Showcase')
  })

  it('owner + none → "Add Showcase" button, no tab', async () => {
    authUser = { id: 'o1', username: 'Owner', public_id: 'testuser' }
    publicProfile = { id: 'o1', username: 'Owner', public_id: 'testuser', showcase_state: 'none' }
    const w = await mountProfile()
    expect(w.find('[data-tab="showcase"]').exists()).toBe(false)
    expect(w.text()).toContain('Add Showcase')
    expect(w.text()).not.toContain('Edit Showcase')
  })

  it('owner + hidden → "Edit Showcase" button, no tab', async () => {
    authUser = { id: 'o1', username: 'Owner', public_id: 'testuser' }
    publicProfile = { id: 'o1', username: 'Owner', public_id: 'testuser', showcase_state: 'hidden' }
    const w = await mountProfile()
    expect(w.find('[data-tab="showcase"]').exists()).toBe(false)
    expect(w.text()).toContain('Edit Showcase')
    expect(w.text()).not.toContain('Add Showcase')
  })

  it('owner + visible → tab present, no entry button', async () => {
    authUser = { id: 'o1', username: 'Owner', public_id: 'testuser' }
    publicProfile = { id: 'o1', username: 'Owner', public_id: 'testuser', showcase_state: 'visible' }
    const w = await mountProfile()
    expect(w.find('[data-tab="showcase"]').exists()).toBe(true)
    expect(w.text()).not.toContain('Add Showcase')
    expect(w.text()).not.toContain('Edit Showcase')
  })

  it('owner clicks Add → tab is revealed (force-edit)', async () => {
    authUser = { id: 'o1', username: 'Owner', public_id: 'testuser' }
    publicProfile = { id: 'o1', username: 'Owner', public_id: 'testuser', showcase_state: 'none' }
    const w = await mountProfile()
    const btn = w.findAll('button').find((b) => b.text().includes('Add Showcase'))
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    await flushPromises()
    expect(w.find('[data-tab="showcase"]').exists()).toBe(true)
    expect(w.findComponent({ name: 'ProfileShowcase' }).props('autoEdit')).toBe(true)
  })

  it('owner cancels Add (editorClosed) → tab collapses, Add button returns', async () => {
    authUser = { id: 'o1', username: 'Owner', public_id: 'testuser' }
    publicProfile = { id: 'o1', username: 'Owner', public_id: 'testuser', showcase_state: 'none' }
    const w = await mountProfile()
    const btn = w.findAll('button').find((b) => b.text().includes('Add Showcase'))
    await btn!.trigger('click')
    await flushPromises()
    expect(w.find('[data-tab="showcase"]').exists()).toBe(true)
    // Editor closed without enabling → force-reveal drops, tab disappears, and
    // the active tab must fall back to watchlist (not a blank showcase panel).
    w.findComponent({ name: 'ProfileShowcase' }).vm.$emit('editorClosed')
    await flushPromises()
    expect(w.find('[data-tab="showcase"]').exists()).toBe(false)
    expect(w.text()).toContain('Add Showcase')
  })
})
