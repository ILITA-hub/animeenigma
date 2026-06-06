import { describe, it, expect } from 'vitest'
import { chooseLoadStrategy } from './useVideoEngine'

describe('chooseLoadStrategy', () => {
  it('uses native src for mp4', () => {
    expect(chooseLoadStrategy({ url: 'a.mp4', type: 'mp4' }, false)).toBe('native')
  })
  it('uses native src for hls when the browser supports it (Safari)', () => {
    expect(chooseLoadStrategy({ url: 'a.m3u8', type: 'hls' }, true)).toBe('native')
  })
  it('uses hls.js for hls when the browser lacks native HLS', () => {
    expect(chooseLoadStrategy({ url: 'a.m3u8', type: 'hls' }, false)).toBe('hlsjs')
  })
})
