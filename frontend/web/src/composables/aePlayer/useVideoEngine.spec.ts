import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'
import {
  chooseLoadStrategy,
  buildLevelLabels,
  shouldFatalOnNetworkError,
  snapshotPlayback,
  useVideoEngine,
} from './useVideoEngine'

// hls.js is dynamically imported only inside load(); mock it so the fatal-error
// test never pulls the real module. `hlsMockState` is read/written by the mock
// factory (hoisted above this module's imports by vi.mock), so it must be
// created via vi.hoisted rather than a plain module-scope `let`.
const hlsMockState = vi.hoisted(() => ({ instances: [] as any[] }))

vi.mock('hls.js', () => {
  class MockHls {
    static isSupported() {
      return true
    }
    static Events = { MANIFEST_PARSED: 'hlsManifestParsed', LEVEL_SWITCHED: 'hlsLevelSwitched', FRAG_LOADED: 'hlsFragLoaded', ERROR: 'hlsError' }
    static ErrorTypes = { NETWORK_ERROR: 'networkError', MEDIA_ERROR: 'mediaError', OTHER_ERROR: 'otherError' }
    static ErrorDetails = { MANIFEST_LOAD_ERROR: 'manifestLoadError', MANIFEST_LOAD_TIMEOUT: 'manifestLoadTimeOut', MANIFEST_PARSING_ERROR: 'manifestParsingError', LEVEL_LOAD_ERROR: 'levelLoadError', LEVEL_LOAD_TIMEOUT: 'levelLoadTimeOut' }
    handlers: Record<string, ((...args: unknown[]) => void)[]> = {}
    media: any = null
    constructor() {
      hlsMockState.instances.push(this)
    }
    loadSource() {}
    attachMedia(media: any) {
      this.media = media
    }
    startLoad() {}
    on(event: string, cb: (...args: unknown[]) => void) {
      (this.handlers[event] ??= []).push(cb)
    }
    // Mirrors the real hls.js: destroy() -> detachMedia() -> BufferController
    // strips the element's src and calls media.load(), which zeroes currentTime
    // synchronously. This is the exact clobber snapshotPlayback must beat.
    destroy() {
      if (this.media) {
        this.media.currentTime = 0
        this.media.paused = true
      }
    }
    trigger(event: string, data: unknown) {
      (this.handlers[event] || []).forEach((cb) => cb(null, data))
    }
  }
  return { default: MockHls }
})

describe('shouldFatalOnNetworkError', () => {
  it('bails immediately on a dead playlist (manifest/level load), no retries', () => {
    // A CDN host that 403/502s the master.m3u8 is unrecoverable for this source —
    // the player must switch rather than loop startLoad() forever.
    expect(shouldFatalOnNetworkError(true, 0)).toBe(true)
  })

  it('retries transient (non-playlist) network errors up to the cap', () => {
    expect(shouldFatalOnNetworkError(false, 0)).toBe(false)
    expect(shouldFatalOnNetworkError(false, 1)).toBe(false)
    expect(shouldFatalOnNetworkError(false, 2)).toBe(true) // cap reached → give up
  })
})

describe('chooseLoadStrategy', () => {
  it('uses native src for mp4 regardless of hls.js support', () => {
    expect(chooseLoadStrategy({ url: 'a.mp4', type: 'mp4' }, true)).toBe('native')
    expect(chooseLoadStrategy({ url: 'a.mp4', type: 'mp4' }, false)).toBe('native')
  })
  it('uses hls.js for hls whenever hls.js is supported (Chrome/Firefox/Edge)', () => {
    // Regression: Chrome reports canPlayType('application/vnd.apple.mpegurl')
    // === 'maybe' but CANNOT play HLS natively. The decision MUST be driven by
    // Hls.isSupported(), not canPlayType, or every HLS stream stalls on Chrome.
    expect(chooseLoadStrategy({ url: 'a.m3u8', type: 'hls' }, true)).toBe('hlsjs')
  })
  it('falls back to native HLS only when hls.js is unsupported (Safari/iOS)', () => {
    expect(chooseLoadStrategy({ url: 'a.m3u8', type: 'hls' }, false)).toBe('native')
  })
})

describe('buildLevelLabels', () => {
  it('labels levels by height, sorted high to low, keeping hls indexes', () => {
    expect(
      buildLevelLabels([{ height: 480 }, { height: 1080 }, { height: 720 }]),
    ).toEqual([
      { label: '1080p', index: 1 },
      { label: '720p', index: 2 },
      { label: '480p', index: 0 },
    ])
  })

  it('dedupes same-height levels keeping the first index', () => {
    expect(
      buildLevelLabels([{ height: 720, bitrate: 2_000_000 }, { height: 720, bitrate: 1_000_000 }]),
    ).toEqual([{ label: '720p', index: 0 }])
  })

  it('falls back to bitrate labels when height is missing', () => {
    expect(buildLevelLabels([{ bitrate: 1_500_000 }])).toEqual([{ label: '1500k', index: 0 }])
  })

  it('returns empty for empty/unlabelable input', () => {
    expect(buildLevelLabels([])).toEqual([])
    expect(buildLevelLabels([{}])).toEqual([])
  })
})

describe('snapshotPlayback', () => {
  it('reads position + play state as-is', () => {
    expect(snapshotPlayback({ currentTime: 312.4, paused: false })).toEqual({ time: 312.4, wasPlaying: true })
    expect(snapshotPlayback({ currentTime: 0, paused: true })).toEqual({ time: 0, wasPlaying: false })
  })
})

describe('useVideoEngine — fatal error salvages the playhead', () => {
  beforeEach(() => {
    hlsMockState.instances.length = 0
  })

  it('captures position + play state BEFORE destroy() zeroes them on a fatal network error', async () => {
    // Bug repro (2026-07-09 tNeymik report): a mid-episode fatal error used to
    // leave the retry/failover path reading currentTime AFTER hls.js's destroy()
    // already reset it to 0, restarting playback at 0:00 despite real progress.
    const video: any = { currentTime: 312.4, paused: false }
    const videoRef = ref(video)
    const engine = useVideoEngine(videoRef)

    await engine.load({ url: 'https://example.test/master.m3u8', type: 'hls' })
    expect(engine.lastKnownPlayback.value).toBeNull() // fresh load — nothing salvaged yet

    const hlsInstance = hlsMockState.instances[0]
    hlsInstance.trigger('hlsError', { fatal: true, type: 'networkError', details: 'manifestLoadError' })

    expect(engine.lastKnownPlayback.value).toEqual({ time: 312.4, wasPlaying: true })
    expect(engine.fatal.value).toBe('network')
    // Confirms destroy() really did clobber the live element — proving the caller
    // cannot recover this position by reading videoRef after the fact.
    expect(video.currentTime).toBe(0)
  })

  it('resets the salvaged snapshot on the next load so a stale value cannot leak forward', async () => {
    const video: any = { currentTime: 100, paused: true }
    const videoRef = ref(video)
    const engine = useVideoEngine(videoRef)

    await engine.load({ url: 'https://example.test/a.m3u8', type: 'hls' })
    hlsMockState.instances[0].trigger('hlsError', { fatal: true, type: 'networkError', details: 'manifestLoadError' })
    expect(engine.lastKnownPlayback.value).not.toBeNull()

    await engine.load({ url: 'https://example.test/b.m3u8', type: 'hls' })
    expect(engine.lastKnownPlayback.value).toBeNull()
  })
})
