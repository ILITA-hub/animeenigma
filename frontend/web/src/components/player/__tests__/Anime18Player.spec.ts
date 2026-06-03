import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import Anime18Player from '../Anime18Player.vue'

const getEpisodes = vi.fn()
const getStream = vi.fn()
const markEpisodeWatched = vi.fn()

vi.mock('@/api/client', () => ({
  anime18Api: {
    getEpisodes: (...a: unknown[]) => getEpisodes(...a),
    getStream: (...a: unknown[]) => getStream(...a),
  },
  userApi: {
    markEpisodeWatched: (...a: unknown[]) => markEpisodeWatched(...a),
  },
}))

const loadSourceSpy = vi.fn()
vi.mock('hls.js', () => {
  class HlsMock {
    static isSupported() { return true }
    static Events = { MANIFEST_PARSED: 'manifestParsed', ERROR: 'error' }
    static ErrorTypes = { NETWORK_ERROR: 'networkError', MEDIA_ERROR: 'mediaError' }
    loadSource(...a: unknown[]) { loadSourceSpy(...a) }
    attachMedia() {}
    on() {}
    startLoad() {}
    recoverMediaError() {}
    destroy() {}
  }
  return { default: HlsMock }
})

vi.mock('@/composables/usePlayerSyncBridge', () => ({
  usePlayerSyncBridge: () => {},
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAuthenticated: false }),
}))

const MP4_SOURCE = { url: 'https://a4.mp4upload.com:183/d/tok/video.mp4', referer: 'https://www.mp4upload.com/', is_hls: false, quality: 'FullHD' }
const HLS_SOURCE = { url: 'https://cdn4.turboviplay.com/data3/x/x.m3u8', referer: '', is_hls: true, quality: 'FullHD' }

const EPISODES = [
  { slug: '1166-foo-feat-episode-1', url: 'https://18anime.me/hentai/1166-foo-feat-episode-1.html', number: 1 },
  { slug: '1167-foo-feat-episode-2', url: 'https://18anime.me/hentai/1167-foo-feat-episode-2.html', number: 2 },
]

const mountPlayer = () =>
  mount(Anime18Player, {
    props: { animeId: 'anime-uuid' },
    global: { mocks: { $t: (k: string) => k } },
  })

describe('Anime18Player', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    loadSourceSpy.mockReset()
    // jsdom does not implement media playback.
    Object.defineProperty(HTMLMediaElement.prototype, 'play', {
      configurable: true,
      value: vi.fn().mockResolvedValue(undefined),
    })
    getEpisodes.mockResolvedValue({ data: { data: EPISODES } })
    getStream.mockResolvedValue({ data: { data: MP4_SOURCE } })
  })

  it('loads episodes from anime18Api for the given animeId', async () => {
    mountPlayer()
    await flushPromises()
    expect(getEpisodes).toHaveBeenCalledTimes(1)
    expect(getEpisodes).toHaveBeenCalledWith('anime-uuid')
  })

  it('auto-selects the first episode and requests its stream by slug', async () => {
    mountPlayer()
    await flushPromises()
    expect(getStream).toHaveBeenCalledWith('anime-uuid', '1166-foo-feat-episode-1')
  })

  it('builds an HLS-proxy URL that injects the mp4upload Referer for MP4 sources', () => {
    const wrapper = mountPlayer()
    const url = (wrapper.vm as unknown as { buildProxyUrl: (u: string, r?: string) => string })
      .buildProxyUrl(MP4_SOURCE.url, MP4_SOURCE.referer)
    expect(url).toContain('/api/streaming/hls-proxy?')
    expect(url).toContain('referer=https%3A%2F%2Fwww.mp4upload.com%2F')
    expect(url).toContain(encodeURIComponent(MP4_SOURCE.url))
  })

  it('omits the referer param when the source needs none (turbovid)', () => {
    const wrapper = mountPlayer()
    const url = (wrapper.vm as unknown as { buildProxyUrl: (u: string, r?: string) => string })
      .buildProxyUrl(HLS_SOURCE.url, HLS_SOURCE.referer)
    expect(url).not.toContain('referer=')
  })

  it('feeds turbovid HLS sources through hls.js (loadSource called with proxy URL)', async () => {
    getStream.mockResolvedValue({ data: { data: HLS_SOURCE } })
    mountPlayer()
    await flushPromises()
    await flushPromises()
    expect(loadSourceSpy).toHaveBeenCalled()
    expect(loadSourceSpy.mock.calls[0][0]).toContain('/api/streaming/hls-proxy?')
  })

  it('does NOT use hls.js for progressive mp4upload sources', async () => {
    getStream.mockResolvedValue({ data: { data: MP4_SOURCE } })
    mountPlayer()
    await flushPromises()
    await flushPromises()
    expect(loadSourceSpy).not.toHaveBeenCalled()
  })

  it('surfaces an explicit error when the stream resolve fails (no silent empty success)', async () => {
    getStream.mockRejectedValue({ response: { data: { error: { message: 'source temporarily unavailable' } } } })
    const wrapper = mountPlayer()
    await flushPromises()
    await flushPromises()
    expect(wrapper.text()).toContain('source temporarily unavailable')
  })
})
