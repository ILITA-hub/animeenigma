import { describe, it, expect, beforeEach, vi } from 'vitest'

// The tap must be NON-INTERRUPTIVE: it analyses a captureStream() fork and never
// routes the element's own audio through the graph, so it can never silence
// playback (the createMediaElementSource bug). These tests pin that contract.

class FakeNode { connect = vi.fn(); disconnect = vi.fn() }
class FakeAnalyser extends FakeNode {
  fftSize = 2048; frequencyBinCount = 1024
  getByteFrequencyData = vi.fn((arr: Uint8Array) => arr.fill(0))
}
let lastSource: FakeNode | null = null
let lastAnalyser: FakeAnalyser | null = null
class FakeCtx {
  sampleRate = 48000; state = 'running'; destination = new FakeNode()
  createMediaStreamSource = vi.fn(() => (lastSource = new FakeNode()))
  createAnalyser = vi.fn(() => (lastAnalyser = new FakeAnalyser()))
  resume = vi.fn(async () => {})
  close = vi.fn(async () => { this.state = 'closed' })
}

function fakeStream(audioTracks = 1) {
  const tracks = Array.from({ length: audioTracks }, () => ({ stop: vi.fn() }))
  return { getAudioTracks: () => tracks, getTracks: () => tracks } as unknown as MediaStream
}

function fakeVideo(capture?: () => MediaStream): HTMLVideoElement {
  const el = document.createElement('video')
  Object.defineProperty(el, 'paused', { get: () => false, configurable: true })
  Object.defineProperty(el, 'seeking', { get: () => false, configurable: true })
  Object.defineProperty(el, 'currentTime', { get: () => 2.5, configurable: true })
  if (capture) (el as unknown as { captureStream: () => MediaStream }).captureStream = capture
  return el
}

beforeEach(() => {
  lastSource = null
  lastAnalyser = null
  vi.stubGlobal('AudioContext', FakeCtx as unknown as typeof AudioContext)
  vi.stubGlobal('requestAnimationFrame', () => 1)
  vi.stubGlobal('cancelAnimationFrame', () => {})
})

describe('createAudioTap', () => {
  it('analyses a captureStream fork and NEVER connects to the element output', async () => {
    let rafCb: ((t: number) => void) | null = null
    vi.stubGlobal('requestAnimationFrame', (fn: (t: number) => void) => { rafCb = fn; return 2 })

    const { createAudioTap } = await import('../subtitleAudioTap')
    const el = fakeVideo(() => fakeStream(1))
    const tap = createAudioTap(el)
    rafCb!(0) // first playing tick wires the fork

    // Source is the captureStream fork (a MediaStreamSource), connected ONLY to
    // the analyser. The analyser is a passive leaf — never connected onward to
    // ctx.destination — so the graph produces no audio and can't silence sound.
    expect(lastSource).not.toBeNull()
    expect(lastSource!.connect).toHaveBeenCalledWith(lastAnalyser)
    expect(lastAnalyser!.connect).not.toHaveBeenCalled()
    tap.dispose()
  })

  it('throws (caller marks auto-sync unsupported) when captureStream is unavailable', async () => {
    const { createAudioTap } = await import('../subtitleAudioTap')
    const el = document.createElement('video') // no captureStream (e.g. Safari)
    expect(() => createAudioTap(el)).toThrow()
  })

  it('idles (no onFrame) until the fork has an audio track, then analyses', async () => {
    let rafCb: ((t: number) => void) | null = null
    vi.stubGlobal('requestAnimationFrame', (fn: (t: number) => void) => { rafCb = fn; return 2 })

    const { createAudioTap } = await import('../subtitleAudioTap')
    let tracks = 0
    const stream = {
      getAudioTracks: () => Array.from({ length: tracks }, () => ({ stop: vi.fn() })),
      getTracks: () => [],
    } as unknown as MediaStream
    const el = fakeVideo(() => stream)
    const tap = createAudioTap(el)
    const calls: Array<[number, boolean]> = []
    tap.onFrame((t, s) => calls.push([t, s]))

    rafCb!(0) // no audio track yet → idle, playback untouched
    expect(calls).toHaveLength(0)

    tracks = 1 // audio track now present on the live fork
    rafCb!(100) // > minGap → wires + analyses
    expect(calls).toHaveLength(1)
    expect(calls[0][0]).toBe(2.5)
    tap.dispose()
  })

  it('calls onFrame on throttled ticks, skips when paused', async () => {
    let rafCb: ((t: number) => void) | null = null
    vi.stubGlobal('requestAnimationFrame', (fn: (t: number) => void) => { rafCb = fn; return 2 })

    const { createAudioTap } = await import('../subtitleAudioTap')
    const paused = { value: false }
    const el = fakeVideo(() => fakeStream(1))
    Object.defineProperty(el, 'paused', { get: () => paused.value, configurable: true })

    const tap = createAudioTap(el)
    const calls: Array<[number, boolean]> = []
    tap.onFrame((t, s) => calls.push([t, s]))

    rafCb!(0) // first tick: lastTick=-Infinity → fires
    expect(calls).toHaveLength(1)
    expect(calls[0][0]).toBe(2.5)

    rafCb!(30) // 30ms < 50ms minGap → throttled
    expect(calls).toHaveLength(1)

    rafCb!(55) // > minGap → fires
    expect(calls).toHaveLength(2)

    paused.value = true
    rafCb!(110) // paused → skip
    expect(calls).toHaveLength(2)
    tap.dispose()
  })

  it('dispose() stops the captured tracks and never throws', async () => {
    let rafCb: ((t: number) => void) | null = null
    vi.stubGlobal('requestAnimationFrame', (fn: (t: number) => void) => { rafCb = fn; return 2 })

    const { createAudioTap } = await import('../subtitleAudioTap')
    const track = { stop: vi.fn() }
    const stream = { getAudioTracks: () => [track], getTracks: () => [track] } as unknown as MediaStream
    const el = fakeVideo(() => stream)
    const tap = createAudioTap(el)
    rafCb!(0) // wire the fork so there's something to tear down

    expect(() => tap.dispose()).not.toThrow()
    expect(track.stop).toHaveBeenCalled()
  })
})
