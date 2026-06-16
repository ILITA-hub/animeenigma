import { describe, it, expect, beforeEach } from 'vitest'
import { readCache, writeCache, chooseBase, demoteAssetEdgeToOrigin } from './assetEdge'

describe('assetEdge cache', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('writeCache + readCache round-trips a valid entry', () => {
    writeCache('https://edge:8443', 60_000, 1_000)
    expect(readCache(2_000)).toEqual({ base: 'https://edge:8443', exp: 61_000 })
  })

  it('readCache returns null for an expired entry', () => {
    writeCache('https://edge:8443', 1_000, 1_000) // exp = 2_000
    expect(readCache(5_000)).toBeNull()
  })

  it('readCache returns null for malformed JSON', () => {
    localStorage.setItem('ae.assetEdge', '{not json')
    expect(readCache()).toBeNull()
  })

  it('readCache returns null when nothing is stored', () => {
    expect(readCache()).toBeNull()
  })
})

describe('chooseBase', () => {
  const EDGE = 'https://edge:8443'

  it('routes to the edge when it is clearly faster', () => {
    expect(chooseBase(250, 20, EDGE)).toBe(EDGE)
  })

  it('stays on origin when the edge is slower', () => {
    expect(chooseBase(20, 250, EDGE)).toBe('')
  })

  it('stays on origin when the edge is unreachable (Infinity)', () => {
    expect(chooseBase(250, Infinity, EDGE)).toBe('')
  })

  it('stays on origin within the anti-flap margin', () => {
    expect(chooseBase(30, 25, EDGE)).toBe('') // 25 + 10 not < 30
  })
})

describe('demoteAssetEdgeToOrigin', () => {
  beforeEach(() => {
    localStorage.clear()
    window.__AE_ASSET_BASE__ = undefined
  })

  it('clears the edge base and caches origin when the edge was active', () => {
    window.__AE_ASSET_BASE__ = 'https://edge:8443'
    demoteAssetEdgeToOrigin()
    expect(window.__AE_ASSET_BASE__).toBe('')
    expect(readCache()?.base).toBe('')
  })

  it('is a no-op when already on origin', () => {
    window.__AE_ASSET_BASE__ = ''
    demoteAssetEdgeToOrigin()
    expect(localStorage.getItem('ae.assetEdge')).toBeNull()
  })
})
