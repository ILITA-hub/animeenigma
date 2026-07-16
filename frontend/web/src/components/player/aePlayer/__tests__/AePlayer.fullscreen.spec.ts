import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { ref, nextTick } from 'vue'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'

// Fullscreen routing. iPhone Safari exposes Element.prototype.requestFullscreen
// (so the feature-detect passes) but never honours it for a non-<video> element,
// so probing it is a bet that silently loses: the pre-6b729abb `void` call
// swallowed the rejection and the tap did nothing at all (tNeymik screenshot,
// 2026-07-14). iPhone therefore routes straight to the CSS takeover instead of
// asking a capability that lies. Guards both that routing and the
// .then()-on-undefined crash that would kill the toggle on other WebKit builds.
const gogoCap = {
  provider: 'gogoanime', display_name: 'GogoAnime',
  state: 'active' as const, selectable: true, hacker_only: false,
  order: 90, group: 'en' as const, audios: ['sub', 'dub'] as ('sub' | 'dub')[],
  variants: [],
}
const report: CapabilityReport = {
  anime_id: 'anime-uuid',
  families: [{ family: 'others', providers: [gogoCap] }],
}

vi.mock('@/composables/aePlayer/useCapabilities', () => ({
  useCapabilities: () => ({
    report: ref(report),
    capMap: ref(new Map<string, ProviderCap>([['gogoanime', gogoCap]])),
  }),
}))
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({
    listEpisodes: vi.fn().mockResolvedValue([{ key: 1, label: 1, number: 1 }]),
    listTeams: vi.fn().mockResolvedValue([]),
    resolveStream: vi.fn().mockResolvedValue({ type: 'hls', url: 'https://example.test/playlist.m3u8', servers: [] }),
  }),
  KODIK_QUALITY_PREF_KEY: 'pl_kodik_q',
}))
vi.mock('@/composables/aePlayer/useVideoEngine', () => ({
  useVideoEngine: () => ({
    fatal: ref(null), lastKnownPlayback: ref(null),
    load: vi.fn().mockResolvedValue(undefined), destroy: vi.fn(),
    levels: ref([]), currentLevelLabel: ref('Auto'), setLevel: vi.fn(),
    fragStats: ref([]), bandwidthEstimate: ref(0), fragLoadedCount: ref(0),
  }),
}))
vi.mock('@/composables/useWatchPreferences', () => ({
  useWatchPreferences: () => ({ resolve: vi.fn().mockResolvedValue(undefined), resolvedCombo: ref(null) }),
}))
vi.mock('@/composables/aePlayer/useWatchTracking', () => ({
  useWatchTracking: () => ({
    maxTime: ref(0), episodeMarked: ref(false), marking: ref(false), onTick: vi.fn(),
    saveNow: vi.fn(), beaconSave: vi.fn(), markWatched: vi.fn().mockResolvedValue(undefined), resetEpisode: vi.fn(),
  }),
}))
vi.mock('@/composables/aePlayer/usePlaybackStats', () => ({ usePlaybackStats: () => ({ stats: ref(null), sample: vi.fn() }) }))
vi.mock('@/composables/useWatchedEpisodes', () => ({ useWatchedEpisodes: () => ({ watchedUpTo: ref(0), refresh: vi.fn().mockResolvedValue(undefined) }) }))
vi.mock('@/composables/useSkipTimes', () => ({
  useSkipTimes: () => ({ opening: ref(null), ending: ref(null), loading: ref(false), error: ref(null), refresh: vi.fn() }),
}))
vi.mock('@/composables/useToast', () => ({ useToast: () => ({ push: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k, locale: { value: 'en' } }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: false, user: null }) }))
vi.mock('@/stores/viewerContext', () => ({ useViewerContextStore: () => ({ whenLoaded: vi.fn().mockResolvedValue(null) }) }))
vi.mock('@/api/client', () => ({
  userApi: { getProgress: vi.fn().mockResolvedValue({ data: { data: null } }) },
  aeApi: { getEpisodes: vi.fn().mockResolvedValue({ data: { data: { available: false, episodes: [] } } }) },
  scraperApi: {
    getEpisodes: vi.fn().mockResolvedValue({ data: { data: { episodes: [] } } }),
    getServers: vi.fn().mockResolvedValue({ data: { data: { servers: [] } } }),
    getStream: vi.fn().mockResolvedValue({ data: { data: { stream: { sources: [] } } } }),
  },
}))
vi.mock('@/utils/playerTelemetry', () => ({ recordPlayerEvent: vi.fn() }))

import AePlayer from '../AePlayer.vue'

const stubs = {
  PlayerControlBar: true, SourcePanel: true, EpisodesPanel: true, PlaybackSettingsMenu: true,
  SubtitlesMenu: true, BrowseSubsModal: true, BigPlayButton: true, BufferingOverlay: true,
  DebugHud: true, SkipIntroChip: true, NextEpisodeCard: true, NextEpisodeChip: true, WatchTogetherButton: true,
  SubtitleOverlay: true,
}

// The UA that filed the report: element fullscreen is present but inert here.
const IPHONE_UA =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.5.2 Mobile/15E148 Safari/604.1'
const DESKTOP_UA =
  'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36'

const realUserAgent = navigator.userAgent

function setUserAgent(ua: string) {
  Object.defineProperty(window.navigator, 'userAgent', { value: ua, configurable: true })
}

function mountPlayer() {
  return mount(AePlayer, {
    props: { animeId: 'anime-uuid', anime: { title: 'T', eps: 12 }, theater: false },
    global: { mocks: { $t: (k: string) => k }, stubs },
    attachTo: document.body,
  })
}

async function settle() {
  await flushPromises()
  await nextTick()
  await flushPromises()
}

/** The element carrying rootRef + the pseudo-FS class (`.pl-wrap` is the outer shell). */
function playerRoot(wrapper: ReturnType<typeof mountPlayer>) {
  return wrapper.find('.pl')
}

/** Install a requestFullscreen stub on the element the toggle actually targets. */
function stubRequestFullscreen(wrapper: ReturnType<typeof mountPlayer>, impl: () => unknown) {
  const el = playerRoot(wrapper).element as HTMLElement
  const spy = vi.fn(impl)
  Object.defineProperty(el, 'requestFullscreen', { value: spy, configurable: true, writable: true })
  return spy
}

async function tapFullscreen(wrapper: ReturnType<typeof mountPlayer>) {
  wrapper.getComponent({ name: 'PlayerControlBar' }).vm.$emit('toggle-fullscreen')
  await settle()
}

beforeEach(() => vi.clearAllMocks())
afterEach(() => {
  setUserAgent(realUserAgent)
  document.documentElement.classList.remove('pl-noscroll')
})

describe('AePlayer — fullscreen routing', () => {
  it('iPhone takes the CSS takeover without consulting the lying feature-detect', async () => {
    setUserAgent(IPHONE_UA)
    const wrapper = mountPlayer()
    await settle()

    // Present AND rejecting — exactly what iPhone Safari does for a non-<video>.
    const req = stubRequestFullscreen(wrapper, () => Promise.reject(new TypeError('not supported')))
    await tapFullscreen(wrapper)

    expect(playerRoot(wrapper).classes()).toContain('pl--pseudo-fs')
    expect(document.documentElement.classList.contains('pl-noscroll')).toBe(true)
    // The whole point: we never place the bet we know loses.
    expect(req).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('iPhone toggles back out of the takeover and restores page scroll', async () => {
    setUserAgent(IPHONE_UA)
    const wrapper = mountPlayer()
    await settle()
    stubRequestFullscreen(wrapper, () => Promise.reject(new TypeError('not supported')))

    await tapFullscreen(wrapper)
    expect(playerRoot(wrapper).classes()).toContain('pl--pseudo-fs')

    await tapFullscreen(wrapper)
    expect(playerRoot(wrapper).classes()).not.toContain('pl--pseudo-fs')
    expect(document.documentElement.classList.contains('pl-noscroll')).toBe(false)
    wrapper.unmount()
  })

  it('desktop still uses native element fullscreen', async () => {
    setUserAgent(DESKTOP_UA)
    const wrapper = mountPlayer()
    await settle()

    const req = stubRequestFullscreen(wrapper, () => Promise.resolve())
    await tapFullscreen(wrapper)

    expect(req).toHaveBeenCalledTimes(1)
    expect(playerRoot(wrapper).classes()).not.toContain('pl--pseudo-fs')
    wrapper.unmount()
  })

  it('falls back to the takeover when a non-iPhone build rejects fullscreen', async () => {
    setUserAgent(DESKTOP_UA)
    const wrapper = mountPlayer()
    await settle()

    stubRequestFullscreen(wrapper, () => Promise.reject(new Error('denied')))
    await tapFullscreen(wrapper)

    expect(playerRoot(wrapper).classes()).toContain('pl--pseudo-fs')
    wrapper.unmount()
  })

  it('survives a WebKit build whose requestFullscreen returns undefined', async () => {
    setUserAgent(DESKTOP_UA)
    const wrapper = mountPlayer()
    await settle()

    // Non-thenable return: .then() on it would throw and kill the toggle.
    const req = stubRequestFullscreen(wrapper, () => undefined)
    await expect(tapFullscreen(wrapper)).resolves.not.toThrow()

    expect(req).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})

// The pseudo-FS takeover extends the video under the Dynamic Island / notch:
// env(safe-area-inset-*) is all zeros unless viewport-fit=cover is active, and
// browser mode deliberately ships WITHOUT cover (index.html), so enterPseudoFs
// opts into cover on the viewport meta for the takeover's lifetime and restores
// the exact previous content on exit (report 2026-07-15T12-52-24, iPhone).
describe('AePlayer — pseudo-FS viewport-fit=cover toggle', () => {
  const BROWSER_CONTENT = 'width=device-width, initial-scale=1.0'

  /** The toggle REPLACES the meta node (iOS ignores setAttribute-only changes),
   *  so tests must always re-query instead of holding a node reference. */
  function viewportContent() {
    return document.querySelector('meta[name="viewport"]')?.getAttribute('content')
  }

  function installViewportMeta(content: string) {
    document.querySelector('meta[name="viewport"]')?.remove()
    const meta = document.createElement('meta')
    meta.setAttribute('name', 'viewport')
    meta.setAttribute('content', content)
    document.head.appendChild(meta)
  }

  beforeEach(() => installViewportMeta(BROWSER_CONTENT))
  afterEach(() => document.querySelector('meta[name="viewport"]')?.remove())

  it('entering the takeover adds viewport-fit=cover, exiting restores the original content', async () => {
    setUserAgent(IPHONE_UA)
    const wrapper = mountPlayer()
    await settle()

    const before = document.querySelector('meta[name="viewport"]')
    await tapFullscreen(wrapper)
    expect(viewportContent()).toBe(`${BROWSER_CONTENT}, viewport-fit=cover`)
    // Node swap is the mechanism iOS actually honors — assert it happened.
    expect(document.querySelector('meta[name="viewport"]')).not.toBe(before)

    await tapFullscreen(wrapper)
    expect(viewportContent()).toBe(BROWSER_CONTENT)
    wrapper.unmount()
  })

  it('leaves an already-covered meta untouched (standalone PWA) and does not mangle it on exit', async () => {
    const PWA_CONTENT = `${BROWSER_CONTENT}, viewport-fit=cover`
    installViewportMeta(PWA_CONTENT)
    setUserAgent(IPHONE_UA)
    const wrapper = mountPlayer()
    await settle()

    await tapFullscreen(wrapper)
    expect(viewportContent()).toBe(PWA_CONTENT)

    await tapFullscreen(wrapper)
    expect(viewportContent()).toBe(PWA_CONTENT)
    wrapper.unmount()
  })

  it('unmounting mid-takeover (route change) restores the viewport meta', async () => {
    setUserAgent(IPHONE_UA)
    const wrapper = mountPlayer()
    await settle()

    await tapFullscreen(wrapper)
    expect(viewportContent()).toContain('viewport-fit=cover')

    wrapper.unmount()
    expect(viewportContent()).toBe(BROWSER_CONTENT)
  })
})
