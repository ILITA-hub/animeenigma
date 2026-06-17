import { describe, it, expect } from 'vitest'
import { chooseLoadStrategy, buildLevelLabels, shouldFatalOnNetworkError } from './useVideoEngine'

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
