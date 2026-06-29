import { describe, it, expect, beforeEach, vi } from 'vitest'

class FakeNode { connect = vi.fn(); disconnect = vi.fn() }
class FakeGain extends FakeNode { gain = { value: 1 } }
class FakeAnalyser extends FakeNode {
  fftSize = 2048; frequencyBinCount = 1024
  getByteFrequencyData = vi.fn((arr: Uint8Array) => arr.fill(0))
}
let lastGain: FakeGain
class FakeCtx {
  sampleRate = 48000; state = 'running'; destination = new FakeNode()
  createMediaElementSource = vi.fn(() => new FakeNode())
  createGain = vi.fn(() => (lastGain = new FakeGain()))
  createAnalyser = vi.fn(() => new FakeAnalyser())
  close = vi.fn(async () => { this.state = 'closed' })
}

beforeEach(() => {
  vi.stubGlobal('AudioContext', FakeCtx as unknown as typeof AudioContext)
  vi.stubGlobal('requestAnimationFrame', () => 1)
  vi.stubGlobal('cancelAnimationFrame', () => {})
})

describe('createAudioTap', () => {
  it('mirrors element volume/mute into the gain node', async () => {
    const { createAudioTap } = await import('../subtitleAudioTap')
    const el = document.createElement('video')
    Object.defineProperty(el, 'volume', { value: 0.5, configurable: true })
    const tap = createAudioTap(el)
    expect(lastGain.gain.value).toBeCloseTo(0.5, 5)
    Object.defineProperty(el, 'muted', { value: true, configurable: true })
    el.dispatchEvent(new Event('volumechange'))
    expect(lastGain.gain.value).toBe(0)
    tap.dispose()
  })
  it('dispose() runs without throwing', async () => {
    const { createAudioTap } = await import('../subtitleAudioTap')
    expect(() => createAudioTap(document.createElement('video')).dispose()).not.toThrow()
  })
  it('calls onFrame on throttled ticks, skips when paused', async () => {
    let rafCb: ((t: number) => void) | null = null
    vi.stubGlobal('requestAnimationFrame', (fn: (t: number) => void) => { rafCb = fn; return 2 })

    const { createAudioTap } = await import('../subtitleAudioTap')
    const el = document.createElement('video')
    Object.defineProperty(el, 'paused', { get: () => false, configurable: true })
    Object.defineProperty(el, 'seeking', { get: () => false, configurable: true })
    Object.defineProperty(el, 'currentTime', { get: () => 2.5, configurable: true })

    const tap = createAudioTap(el)
    const calls: Array<[number, boolean]> = []
    tap.onFrame((t, s) => calls.push([t, s]))

    rafCb!(0)                       // first tick: lastTick=-Infinity → always fires
    expect(calls).toHaveLength(1)
    expect(calls[0][0]).toBe(2.5)   // currentTime forwarded

    rafCb!(30)                      // 30ms < 50ms minGap → throttled
    expect(calls).toHaveLength(1)

    rafCb!(55)                      // > minGap → fires
    expect(calls).toHaveLength(2)

    Object.defineProperty(el, 'paused', { get: () => true, configurable: true })
    rafCb!(110)                     // paused → skip
    expect(calls).toHaveLength(2)

    tap.dispose()
  })
})
