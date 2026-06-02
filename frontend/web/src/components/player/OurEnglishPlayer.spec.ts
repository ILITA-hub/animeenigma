import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import OurEnglishPlayer from './OurEnglishPlayer.vue'

// Regression guard for the OurEnglish-player "Выберите серию" bug: the FE must
// pin servers/stream to the provider that PRODUCED the episode list
// (env.meta.provider), NOT the last entry of meta.tried (the lowest-priority
// fallback). Provider episode/server IDs are opaque + provider-specific, so a
// mismatched pin breaks the whole servers/stream chain.

const getEpisodes = vi.fn()
const getServers = vi.fn()
const getStream = vi.fn()

vi.mock('@/api/client', () => ({
  scraperApi: {
    getEpisodes: (...a: unknown[]) => getEpisodes(...a),
    getServers: (...a: unknown[]) => getServers(...a),
    getStream: (...a: unknown[]) => getStream(...a),
  },
}))

// hls.js stub. isSupported()=true so attachStream takes the MSE branch and
// calls loadSource — the loadSourceSpy lets us assert the stream actually
// attached (i.e. attachStream did NOT early-return on a null videoRef).
const loadSourceSpy = vi.fn()
vi.mock('hls.js', () => ({
  default: class {
    static isSupported() { return true }
    loadSource(...a: unknown[]) { loadSourceSpy(...a) }
    attachMedia() {}
    on() {}
    destroy() {}
  },
}))

// The sync bridge is only wired when a room prop is passed; default tests pass
// no room, but stub it defensively so the import never touches real WS code.
vi.mock('@/composables/usePlayerSyncBridge', () => ({
  usePlayerSyncBridge: () => {},
}))

const mountPlayer = () =>
  mount(OurEnglishPlayer, {
    props: { animeId: 'anime-uuid' },
    global: {
      mocks: { $t: (k: string) => k },
      stubs: {
        SubtitleOverlay: { template: '<div />' },
        SubtitleSettingsMenu: true,
      },
    },
  })

describe('OurEnglishPlayer provider pinning', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Episodes resolved by animepahe; meta.tried lists nineanime LAST (the old
    // buggy code would have pinned nineanime).
    getEpisodes.mockResolvedValue({
      data: {
        data: {
          episodes: [{ id: 'opaque-hash-73', number: 73 }],
          meta: {
            tried: ['animepahe', 'allanime', 'animefever', 'miruro', 'nineanime'],
            provider: 'animepahe',
          },
        },
      },
    })
    getServers.mockResolvedValue({
      data: { data: { servers: [{ id: 'https://kwik.cx/e/abc', name: 'kwik', type: 'sub' }], meta: { tried: [] } } },
    })
    getStream.mockResolvedValue({
      data: { data: { stream: { sources: [{ url: 'https://cdn/uwu.m3u8', type: 'hls' }] }, meta: { tried: [] } } },
    })
  })

  it('pins servers + stream to the winning provider from meta.provider, not tried[-1]', async () => {
    mountPlayer()
    await flushPromises()
    await flushPromises()

    expect(getEpisodes).toHaveBeenCalledTimes(1)
    expect(getServers).toHaveBeenCalled()
    // 3rd positional arg to getServers is `prefer`.
    const serversPrefer = getServers.mock.calls[0][2]
    expect(serversPrefer).toBe('animepahe')
    expect(serversPrefer).not.toBe('nineanime')

    // getStream's `prefer` (last positional arg) must also be the winner.
    expect(getStream).toHaveBeenCalled()
    const streamArgs = getStream.mock.calls[0]
    expect(streamArgs[streamArgs.length - 1]).toBe('animepahe')
  })

  it('falls back to auto (undefined prefer) when meta.provider is absent', async () => {
    getEpisodes.mockResolvedValueOnce({
      data: {
        data: {
          episodes: [{ id: 'opaque-hash-1', number: 1 }],
          meta: { tried: ['animepahe', 'nineanime'] }, // no provider field
        },
      },
    })
    mountPlayer()
    await flushPromises()
    await flushPromises()

    // activeProvider === '' → prefer resolves to undefined (auto), never the
    // bogus tried[-1]='nineanime'.
    const serversPrefer = getServers.mock.calls[0][2]
    expect(serversPrefer).toBeUndefined()
    expect(serversPrefer).not.toBe('nineanime')
  })

  // Regression guard for the "controls visible, frozen at 0:00" bug: on the
  // initial auto-load, loadingEpisodes was still true while selectEpisode ran,
  // so the <video> (in the v-else branch) wasn't mounted, videoRef was null,
  // and attachStream early-returned — the stream resolved but never attached.
  // The fix defers auto-select until after loadingEpisodes is false + nextTick,
  // so attachStream reaches hls.loadSource.
  it('attaches the stream on initial auto-load (reaches hls.loadSource)', async () => {
    mountPlayer()
    await flushPromises()
    await flushPromises()
    await flushPromises()

    expect(getStream).toHaveBeenCalled()
    // loadSource being called proves attachStream did NOT early-return on a
    // null videoRef — i.e. the <video> was mounted by the time it ran.
    expect(loadSourceSpy).toHaveBeenCalled()
  })
})
