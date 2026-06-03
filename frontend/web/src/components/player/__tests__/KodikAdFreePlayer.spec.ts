import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import KodikAdFreePlayer from '../KodikAdFreePlayer.vue'

// vue-i18n needs app.use() — stub it so the component's useI18n() call resolves
// without a real i18n instance (same pattern as passing $t in global.mocks).
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

// ── API client mock ──────────────────────────────────────────────────────────
const getTranslations = vi.fn()
const getStream = vi.fn()
const getPinnedTranslations = vi.fn()
const pinTranslation = vi.fn()
const unpinTranslation = vi.fn()
const reportError = vi.fn()
const markEpisodeWatched = vi.fn()
const updateProgress = vi.fn()

vi.mock('@/api/client', () => ({
  kodikApi: {
    getTranslations: (...a: unknown[]) => getTranslations(...a),
    getStream: (...a: unknown[]) => getStream(...a),
    getPinnedTranslations: (...a: unknown[]) => getPinnedTranslations(...a),
    pinTranslation: (...a: unknown[]) => pinTranslation(...a),
    unpinTranslation: (...a: unknown[]) => unpinTranslation(...a),
  },
  userApi: {
    reportError: (...a: unknown[]) => reportError(...a),
    markEpisodeWatched: (...a: unknown[]) => markEpisodeWatched(...a),
    updateProgress: (...a: unknown[]) => updateProgress(...a),
  },
}))

// ── hls.js stub ──────────────────────────────────────────────────────────────
// isSupported()=true so attachStream takes the MSE branch and calls loadSource.
const loadSourceSpy = vi.fn()
vi.mock('hls.js', () => ({
  default: class {
    static isSupported() { return true }
    static Events = { MANIFEST_PARSED: 'manifestParsed', ERROR: 'error' }
    loadSource(...a: unknown[]) { loadSourceSpy(...a) }
    attachMedia() {}
    on() {}
    destroy() {}
  },
}))

// ── composable stubs ─────────────────────────────────────────────────────────
const refreshWatched = vi.fn().mockResolvedValue(undefined)
vi.mock('@/composables/useWatchedEpisodes', () => ({
  useWatchedEpisodes: () => ({
    watchedUpTo: { value: 0 },
    refresh: refreshWatched,
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isAuthenticated: true,
  }),
}))

vi.mock('@/composables/useWatchSession', () => ({
  useWatchSession: () => ({
    sessionId: { value: 'test-session-id' },
    newSession: vi.fn(),
  }),
}))

// ── Fixture data ─────────────────────────────────────────────────────────────
const TRANSLATIONS = [
  { id: 1215, title: 'AniDUB', type: 'voice', episodes_count: 12 },
  { id: 1216, title: 'AniSub', type: 'subtitles', episodes_count: 12 },
]
const STREAM_RESPONSE = {
  stream_url: 'https://cloud.solodcdn.com/useruploads/x/y/720.mp4:hls:manifest.m3u8',
  referer: 'https://kodikplayer.com/',
  quality: 720,
  qualities: [360, 480, 720],
  episode: 1,
  translation_id: 1215,
  translation: 'AniDUB',
}

const mountPlayer = (props = {}) =>
  mount(KodikAdFreePlayer, {
    props: { animeId: 'anime-uuid', ...props },
    global: {
      mocks: { $t: (k: string) => k },
      stubs: {
        EpisodeSelector: { template: '<div />' },
      },
    },
  })

describe('KodikAdFreePlayer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    loadSourceSpy.mockReset()
    refreshWatched.mockReset().mockResolvedValue(undefined)
    markEpisodeWatched.mockReset().mockResolvedValue(undefined)
    updateProgress.mockReset().mockResolvedValue(undefined)
    // jsdom does not implement media playback.
    Object.defineProperty(HTMLMediaElement.prototype, 'play', {
      configurable: true,
      value: vi.fn().mockResolvedValue(undefined),
    })
    Object.defineProperty(HTMLMediaElement.prototype, 'load', {
      configurable: true,
      value: vi.fn(),
    })
    getTranslations.mockResolvedValue({ data: { data: TRANSLATIONS } })
    getPinnedTranslations.mockResolvedValue({ data: { data: [] } })
    getStream.mockResolvedValue({ data: { data: STREAM_RESPONSE } })
  })

  // ── Assertion 1: renders a <video> (not an iframe) ────────────────────────
  it('renders a <video> element, not an iframe', async () => {
    const wrapper = mountPlayer()
    await flushPromises()

    expect(wrapper.find('video').exists()).toBe(true)
    expect(wrapper.find('iframe').exists()).toBe(false)
  })

  // ── Assertion 2: calls kodikApi.getStream on episode+translation selection ─
  it('calls kodikApi.getStream when a translation is auto-selected on mount', async () => {
    mountPlayer()
    await flushPromises()

    // Auto-selects first voice translation (id=1215) + episode 1
    expect(getStream).toHaveBeenCalledWith('anime-uuid', 1, 1215)
  })

  // ── Assertion 3: builds a /api/streaming/hls-proxy URL with stream_url + referer
  it('builds an /api/streaming/hls-proxy URL encoding stream_url and referer', async () => {
    const wrapper = mountPlayer()
    await flushPromises()
    await flushPromises()

    // The intro plays first; fire ended to proceed to attachStream.
    const video = wrapper.find('video').element as HTMLVideoElement
    video.dispatchEvent(new Event('ended'))
    await flushPromises()

    // loadSourceSpy is called with the proxy URL after the intro ends
    expect(loadSourceSpy).toHaveBeenCalled()
    const proxyUrl: string = loadSourceSpy.mock.calls[0][0]
    expect(proxyUrl).toContain('/api/streaming/hls-proxy?')
    expect(proxyUrl).toContain(encodeURIComponent(STREAM_RESPONSE.stream_url))
    expect(proxyUrl).toContain('referer=' + encodeURIComponent(STREAM_RESPONSE.referer))
  })

  // ── Assertion 4: shows the extractError block when getStream rejects ───────
  it('shows the extractError block when getStream rejects', async () => {
    getStream.mockRejectedValue(new Error('Network error'))
    const wrapper = mountPlayer()
    await flushPromises()
    await flushPromises()

    expect(wrapper.text()).toContain('player.kodikAdfree.extractError')
  })

  // ── Assertion 5: renders the report button in the error state ─────────────
  it('renders the report button ([data-testid=report-button]) in the error state', async () => {
    getStream.mockRejectedValue(new Error('Network error'))
    const wrapper = mountPlayer()
    await flushPromises()
    await flushPromises()

    expect(wrapper.find('[data-testid="report-button"]').exists()).toBe(true)
  })

  // ── Assertion 6: intro — <video>.src set to /branding/intro.mp4 before stream
  it('sets video.src to /branding/intro.mp4 before attaching the real stream', async () => {
    const wrapper = mountPlayer()
    await flushPromises()
    await flushPromises()

    const video = wrapper.find('video').element as HTMLVideoElement
    // The intro plays first (if not already shown for this key) — src is intro
    // before the ended/onerror callback fires. jsdom doesn't run media events
    // automatically, so src stays at intro after mount.
    expect(video.src).toContain('/branding/intro.mp4')
  })

  // ── Assertion 7: intro ended event -> attachStream builds the hls-proxy URL
  it('after intro ended, attachStream calls hls.loadSource with the hls-proxy URL', async () => {
    const wrapper = mountPlayer()
    await flushPromises()
    await flushPromises()

    const video = wrapper.find('video').element as HTMLVideoElement
    // Fire the ended event to simulate intro completion
    video.dispatchEvent(new Event('ended'))
    await flushPromises()

    expect(loadSourceSpy).toHaveBeenCalled()
    const proxyUrl: string = loadSourceSpy.mock.calls[0][0]
    expect(proxyUrl).toContain('/api/streaming/hls-proxy?')
  })

  // ── Assertion 8: Skip button hidden initially, shown after the 3s timer ───
  it('skip button is hidden initially then shown after the 3s timer', async () => {
    vi.useFakeTimers()
    const wrapper = mountPlayer()
    await flushPromises()
    await flushPromises()

    // introPlaying=true but showSkip is false initially
    expect(wrapper.find('button[class*="bottom-6"]').exists()).toBe(false)

    // Advance time by 3 seconds to trigger the showSkip timer
    vi.advanceTimersByTime(3000)
    await wrapper.vm.$nextTick()

    // The skip button should now be visible
    expect(wrapper.find('button[class*="bottom-6"]').exists()).toBe(true)
    vi.useRealTimers()
  })

  // ── Assertion 9: second load of the SAME episode skips the intro ──────────
  it('second load of the same episode skips the intro (introShownFor guard)', async () => {
    const wrapper = mountPlayer()
    await flushPromises()
    await flushPromises()

    const video = wrapper.find('video').element as HTMLVideoElement

    // Simulate intro ended -> attachStream fires -> loadSource called once
    video.dispatchEvent(new Event('ended'))
    await flushPromises()

    const firstLoadCount = loadSourceSpy.mock.calls.length

    // Reset intro state to simulate selecting the same ep/translation again
    // The introShownFor Set should prevent the intro from playing again.
    loadSourceSpy.mockClear()

    // Re-trigger loadStream with the same episode/translation via selectEpisode
    const vmAny = wrapper.vm as unknown as { selectEpisode: (ep: number) => void }
    // Reset selectedEpisode to force re-trigger
    ;(wrapper.vm as unknown as Record<string, unknown>)['selectedEpisode'] = 0
    vmAny.selectEpisode(1)
    await flushPromises()
    await flushPromises()

    // With the introShownFor guard, the intro should be skipped and stream
    // attached directly — no intro.mp4 assigned this time.
    // The video.src should now be the proxy URL (not intro.mp4) because
    // introShownFor already has '1215:1'.
    expect(loadSourceSpy).toHaveBeenCalled()
    const proxyUrl: string = loadSourceSpy.mock.calls[0][0]
    expect(proxyUrl).toContain('/api/streaming/hls-proxy?')
    // And it should NOT be the intro URL
    expect(video.src).not.toContain('/branding/intro.mp4')
    // Total calls: just the stream load, not the intro
    void firstLoadCount
  })

  // ── Assertion 10: timeupdate past 0.9*duration calls markEpisodeWatched ───
  it('timeupdate past 0.9*duration triggers markEpisodeWatched and refreshWatched', async () => {
    const wrapper = mountPlayer()
    await flushPromises()
    await flushPromises()

    const video = wrapper.find('video').element as HTMLVideoElement

    // End the intro so introPlaying=false (handleTimeUpdate guards on introPlaying)
    video.dispatchEvent(new Event('ended'))
    await flushPromises()

    // Simulate real video with duration=1000s, currentTime at 95% (past 0.9*duration)
    Object.defineProperty(video, 'currentTime', { configurable: true, get: () => 950 })
    Object.defineProperty(video, 'duration', { configurable: true, get: () => 1000 })

    video.dispatchEvent(new Event('timeupdate'))
    await flushPromises()

    expect(markEpisodeWatched).toHaveBeenCalledWith(
      'anime-uuid',
      1,
      expect.anything(),
      expect.anything(),
    )
    expect(refreshWatched).toHaveBeenCalled()
  })

  // ── Assertion 11: clicking the mark-watched button calls markEpisodeWatched
  it('clicking the mark-watched button calls userApi.markEpisodeWatched', async () => {
    const wrapper = mountPlayer()
    await flushPromises()

    // The mark-watched button is rendered when isAuthenticated=true (mocked)
    const btn = wrapper.find('button.ml-auto')
    expect(btn.exists()).toBe(true)

    await btn.trigger('click')
    await flushPromises()

    expect(markEpisodeWatched).toHaveBeenCalledWith(
      'anime-uuid',
      1,
      expect.anything(),
      expect.anything(),
    )
  })
})
