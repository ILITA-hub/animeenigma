import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import {
  sourceFallbackDebug,
  recordFallbackIntent,
  resetFallbackIntents,
} from './sourceFallbackDebug'

describe('sourceFallbackDebug', () => {
  beforeEach(() => {
    resetFallbackIntents()
    vi.spyOn(console, 'info').mockImplementation(() => undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('records an intent (logged-only) into the ring buffer', () => {
    recordFallbackIntent({ from: 'ae', to: 'allanime', reason: 'saved source unavailable', acted: false })
    expect(sourceFallbackDebug.intents).toHaveLength(1)
    const it0 = sourceFallbackDebug.intents[0]
    expect(it0.from).toBe('ae')
    expect(it0.to).toBe('allanime')
    expect(it0.acted).toBe(false)
    expect(typeof it0.at).toBe('number')
  })

  it('mirrors every intent to the console, distinguishing intent vs switch', () => {
    recordFallbackIntent({ from: 'ae', to: 'allanime', reason: 'saved source unavailable', acted: false })
    recordFallbackIntent({ from: 'ae', to: 'allanime', reason: 'saved source unavailable', acted: true })
    expect(console.info).toHaveBeenCalledTimes(2)
    expect((console.info as ReturnType<typeof vi.fn>).mock.calls[0][0]).toContain('INTENT (hacker — not switching)')
    expect((console.info as ReturnType<typeof vi.fn>).mock.calls[1][0]).toContain('SWITCH')
  })

  it('renders a null candidate as "(no candidate)" in the console line', () => {
    recordFallbackIntent({ from: 'kodik', to: null, reason: 'saved source unavailable', acted: false })
    expect((console.info as ReturnType<typeof vi.fn>).mock.calls[0][0]).toContain('(no candidate)')
  })

  it('caps the ring buffer at 20 entries, dropping the oldest', () => {
    for (let i = 0; i < 25; i++) {
      recordFallbackIntent({ from: `p${i}`, to: 'allanime', reason: 'x', acted: false })
    }
    expect(sourceFallbackDebug.intents).toHaveLength(20)
    // oldest (p0..p4) dropped; newest retained
    expect(sourceFallbackDebug.intents[0].from).toBe('p5')
    expect(sourceFallbackDebug.intents[19].from).toBe('p24')
  })

  it('resetFallbackIntents clears the ledger', () => {
    recordFallbackIntent({ from: 'ae', to: 'allanime', reason: 'x', acted: false })
    resetFallbackIntents()
    expect(sourceFallbackDebug.intents).toHaveLength(0)
  })
})
