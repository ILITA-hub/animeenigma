import { describe, it, expect } from 'vitest'
import { chooseLoadStrategy } from './useVideoEngine'

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
