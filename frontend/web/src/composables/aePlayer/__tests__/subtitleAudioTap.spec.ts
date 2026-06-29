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
})
