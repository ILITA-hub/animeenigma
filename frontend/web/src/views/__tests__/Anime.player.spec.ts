/**
 * Plan B — Anime.vue player-surface collapse.
 *
 * Verifies the retire-legacy-players model:
 *   - AePlayer is the DEFAULT mounted player.
 *   - The "Classic Kodik" toggle flips the surface to the iframe KodikPlayer.
 *   - None of the six retired players are referenced.
 *
 * The view has a large dependency surface; every heavy composable / store /
 * API client is stubbed so the player branch renders deterministically. The
 * localStorage-normalization logic itself is unit-tested separately in
 * animePlayerPrefs.spec.ts.
 */
import { describe, it, expect, vi, beforeEach, afterEach, beforeAll } from 'vitest'
import { ref } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createRouter, createWebHistory } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import en from '@/locales/en.json'

// --- A ready, RELEASED anime so notReleasedYet === false -------------------
const ANIME = {
  id: 'anime-uuid-1',
  title: 'Test Anime',
  coverImage: 'cover.jpg',
  status: 'released',
  hasVideo: true,
  episodesAired: 12,
  totalEpisodes: 12,
  episodeDuration: 24,
  shikimoriId: '12345',
  rawGenres: [],
  airedOn: null,
  nextEpisodeAt: null,
}
const animeRef = ref<Record<string, unknown> | null>(null)
// Per-test override for fetchAnime's resolved payload (defaults to ANIME).
// Lets a single test simulate an announced/upcoming title (notReleasedYet)
// without disturbing every other test's RELEASED baseline.
let animeOverride: Record<string, unknown> | null = null

// The view's loadAnimeData() resets anime.value = null then awaits
// fetchAnime(), so the mock must REPOPULATE the ref (mirrors real behavior).
vi.mock('@/composables/useAnime', () => ({
  useAnime: () => ({
    anime: animeRef,
    loading: ref(false),
    error: ref(null),
    fetchAnime: vi.fn(async () => {
      animeRef.value = { ...(animeOverride ?? ANIME) }
      return animeRef.value
    }),
  }),
}))

// Watch preferences — resolvedCombo drives KodikPlayer :preferred-combo.
vi.mock('@/composables/useWatchPreferences', () => ({
  useWatchPreferences: () => ({
    resolvedCombo: ref(null),
    resolve: vi.fn().mockResolvedValue(undefined),
  }),
}))

vi.mock('@/composables/useContextMenu', () => ({
  useContextMenu: () => ({
    contextMenu: ref({ visible: false, x: 0, y: 0, items: [], target: null }),
    openAtElement: vi.fn(),
  }),
}))
vi.mock('@/composables/useSiteRatings', () => ({
  useSiteRatings: () => ({ siteRating: ref(null), loadRating: vi.fn(), submitRating: vi.fn() }),
}))
vi.mock('@/composables/useUserTimezone', () => ({
  useUserTimezone: () => ({ timezone: ref('UTC') }),
}))
vi.mock('@/composables/useCharacters', () => ({
  useCharacters: () => ({ characters: ref([]), loading: ref(false), fetchCharacters: vi.fn() }),
}))
vi.mock('@/composables/useImageProxy', () => ({
  getImageUrl: (u: string) => u,
  getImageFallbackUrl: (u: string) => u,
}))
vi.mock('@/composables/useToast', () => ({ useToast: () => ({ push: vi.fn(), show: vi.fn() }) }))
vi.mock('@/composables/useConfirm', () => ({ useConfirm: () => ({ confirm: vi.fn() }) }))

// Theater-mode reduced-motion check (Anime.vue's onToggleTheater). Partial-mock
// with importOriginal + spread — a bare replacement of @vueuse/core would drop
// unrelated exports other parts of the dependency graph rely on (established
// pattern: HeroSpotlightBlock.spec.ts, RandomTailCard.spec.ts).
const mockReducedMotion = ref(false)
vi.mock('@vueuse/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@vueuse/core')>()
  return {
    ...actual,
    useMediaQuery: (q: string) => {
      if (q.includes('prefers-reduced-motion')) return mockReducedMotion
      return ref(false)
    },
  }
})

// Viewer-context store (player reads the aggregate on load).
vi.mock('@/stores/viewerContext', () => ({
  useViewerContextStore: () => ({ load: vi.fn().mockResolvedValue(null) }),
}))

// API clients — all no-ops; the player branch doesn't depend on real data.
vi.mock('@/api/client', () => ({
  animeApi: {
    resolveMAL: vi.fn().mockResolvedValue({ data: {} }),
    getRelated: vi.fn().mockResolvedValue({ data: { data: [] } }),
    getWatchersCount: vi.fn().mockResolvedValue({ data: { data: { count: 0 } } }),
  },
  episodeApi: {},
  userApi: {
    getListEntry: vi.fn().mockResolvedValue({ data: { data: null } }),
    startRewatch: vi.fn().mockResolvedValue({ data: {} }),
    migrateListEntry: vi.fn().mockResolvedValue({ data: {} }),
  },
  reviewApi: { list: vi.fn().mockResolvedValue({ data: { data: [] } }) },
  commentApi: { list: vi.fn().mockResolvedValue({ data: { data: [] } }) },
}))

const i18n = createI18n({ legacy: false, locale: 'en', fallbackLocale: 'en', messages: { en } })
const router = createRouter({ history: createWebHistory(), routes: [{ path: '/anime/:id', name: 'anime', component: { template: '<div/>' } }] })

const PLAYER_STUBS = {
  AePlayer: { name: 'AePlayer', template: '<div data-test="ae-player" />' },
  KodikPlayer: { name: 'KodikPlayer', template: '<div data-test="kodik-player" />' },
}

async function mountView() {
  router.push('/anime/anime-uuid-1')
  await router.isReady()
  const wrapper = mount(
    (await import('@/views/Anime.vue')).default,
    {
      global: {
        plugins: [i18n, router],
        stubs: {
          ...PLAYER_STUBS,
          teleport: true,
          transition: false,
          InviteButton: true,
          ResumePill: true,
          Carousel: true,
          PosterCard: true,
          PosterImage: true,
          CharacterCard: true,
          AnimeContextMenu: true,
          ReviewReactions: true,
          GenreChip: true,
        },
      },
    },
  )
  await flushPromises()
  return wrapper
}

describe('Anime.vue player surface (Plan B)', () => {
  // Warm the heavy Anime.vue dependency graph ONCE so the first real test
  // doesn't bear the cold dynamic-import cost and race the default 5s timeout
  // (cold import of the full view tree can exceed it on a busy CI box).
  beforeAll(async () => {
    await import('@/views/Anime.vue')
  }, 30000)

  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    animeRef.value = null
    animeOverride = null
    mockReducedMotion.value = false
    vi.stubGlobal('IntersectionObserver', class {
      observe() {}
      unobserve() {}
      disconnect() {}
    })
  })

  // Only the aePlayerEnabled=false test stubs this; unstub unconditionally so
  // a failed assertion there can't leak the override into later tests.
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('mounts AePlayer by default (no localStorage preference)', async () => {
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="ae-player"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="kodik-player"]').exists()).toBe(false)
  })

  it('flips to Classic Kodik (KodikPlayer) when the toggle is clicked', async () => {
    const wrapper = await mountView()
    const toggle = wrapper.findAll('button').find(b => b.text().includes('Classic Kodik'))
    expect(toggle).toBeTruthy()
    await toggle!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="kodik-player"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="ae-player"]').exists()).toBe(false)
  })

  it('persists the Classic Kodik choice to localStorage', async () => {
    const wrapper = await mountView()
    const toggle = wrapper.findAll('button').find(b => b.text().includes('Classic Kodik'))
    await toggle!.trigger('click')
    await flushPromises()
    expect(localStorage.getItem('classic_kodik_selected')).toBe('true')
  })

  it('boots into Classic Kodik when classic_kodik_selected=true is stored', async () => {
    localStorage.setItem('classic_kodik_selected', 'true')
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="kodik-player"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="ae-player"]').exists()).toBe(false)
  })

  // Finding 2 (pre-merge review) — theater is an aePlayer-only feature.
  // A classicKodik=true + theaterMode=1 combo persisted from before the
  // guard existed leaves AePlayer unmounted (no theater button anywhere)
  // while the theater CSS still applies to the heading-less, toggle-less
  // Kodik iframe — a dead end. Anime.vue must force theater off for this
  // combo on mount, not just on a later classicKodik change.
  it('forces theater off on mount when classicKodik + theaterMode are both persisted true', async () => {
    localStorage.setItem('classic_kodik_selected', 'true')
    localStorage.setItem('theaterMode', '1')
    const wrapper = await mountView()
    expect(document.body.classList.contains('theater-mode')).toBe(false)
    wrapper.unmount()
  })

  // Finding 1 (pre-merge review) — the theater-off guard must key off
  // AePlayer's REAL mount condition (`!classicKodik && aePlayerEnabled &&
  // !notReleasedYet`), not `classicKodik` alone. An announced/upcoming title
  // (notReleasedYet===true) swaps AePlayer for the premiere notice, removing
  // the only theater-exit control, while classicKodik never changes — so the
  // old classicKodik-only guard could never fire here. notReleasedYet only
  // resolves after the async fetchAnime() completes, which mountView()'s
  // trailing flushPromises() waits out.
  it('forces theater off once the fetched anime resolves to notReleasedYet (announced title)', async () => {
    animeOverride = { ...ANIME, status: 'announced', hasVideo: false }
    localStorage.setItem('theaterMode', '1')
    const wrapper = await mountView()
    expect(document.body.classList.contains('theater-mode')).toBe(false)
    wrapper.unmount()
  })

  // Finding 1, second hole — VITE_AE_PLAYER_ENABLED=false makes Classic Kodik
  // the ONLY surface (the aePlayerEnabled=false template branch), and also
  // hides the Classic-Kodik toggle button itself (`v-if="aePlayerEnabled"`),
  // so there is no way back from a stale theaterMode=1 on such a deploy.
  // aePlayerEnabled is read synchronously at setup from import.meta.env, so
  // (unlike the notReleasedYet case above) this must be caught by the
  // guard's `immediate: true` firing before the first paint, without waiting
  // on any fetch.
  it('forces theater off on mount when VITE_AE_PLAYER_ENABLED=false and theaterMode is persisted true', async () => {
    vi.stubEnv('VITE_AE_PLAYER_ENABLED', 'false')
    localStorage.setItem('theaterMode', '1')
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="kodik-player"]').exists()).toBe(true)
    expect(document.body.classList.contains('theater-mode')).toBe(false)
    wrapper.unmount()
  })

  it('migrates a legacy preferred_video_provider=kodik into Classic Kodik', async () => {
    localStorage.setItem('preferred_video_provider', 'kodik')
    const wrapper = await mountView()
    expect(wrapper.find('[data-test="kodik-player"]').exists()).toBe(true)
  })

  it('does not reference any retired player component', async () => {
    const wrapper = await mountView()
    const html = wrapper.html()
    for (const retired of ['AnimeLibPlayer', 'OurEnglishPlayer', 'HanimePlayer', 'Anime18Player', 'RawPlayer', 'KodikAdFreePlayer']) {
      expect(html).not.toContain(retired)
    }
  })

  // Theater mode — onToggleTheater() scrolls the player section into view.
  // jsdom does not implement scrollIntoView, so it's stubbed on the element
  // prototype and asserted on directly.
  describe('theater-mode scroll behavior', () => {
    let scrollIntoViewMock: ReturnType<typeof vi.fn<(arg?: boolean | ScrollIntoViewOptions) => void>>

    beforeEach(() => {
      scrollIntoViewMock = vi.fn()
      Element.prototype.scrollIntoView = scrollIntoViewMock
    })

    it("jumps instantly (behavior: 'instant') when prefers-reduced-motion is set", async () => {
      mockReducedMotion.value = true
      const wrapper = await mountView()
      await wrapper.findComponent({ name: 'AePlayer' }).vm.$emit('toggle-theater')
      await flushPromises()
      expect(scrollIntoViewMock).toHaveBeenCalledWith({ behavior: 'instant', block: 'start' })
    })

    it("smooth-scrolls (behavior: 'smooth') when reduced motion is not preferred", async () => {
      mockReducedMotion.value = false
      const wrapper = await mountView()
      await wrapper.findComponent({ name: 'AePlayer' }).vm.$emit('toggle-theater')
      await flushPromises()
      expect(scrollIntoViewMock).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' })
    })
  })
})
