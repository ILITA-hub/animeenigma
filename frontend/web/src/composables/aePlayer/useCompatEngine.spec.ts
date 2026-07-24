import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { useCompatEngine, __setCompatLoaderForTest, toMs, toSec } from './useCompatEngine'

// Fake AVPlayer capturing constructor opts + API calls.
class FakeAVPlayer {
  static instances: FakeAVPlayer[] = []
  opts: Record<string, unknown>
  handlers: Record<string, ((...a: unknown[]) => void)[]> = {}
  currentTime = 0n
  load = vi.fn(async () => {})
  play = vi.fn(async () => {})
  pause = vi.fn(async () => {})
  seek = vi.fn(async (_ms: bigint) => {})
  stop = vi.fn(async () => {})
  destroy = vi.fn(async () => {})
  setVolume = vi.fn()
  setPlaybackRate = vi.fn()
  resize = vi.fn()
  getDuration = vi.fn(() => 1_440_000n) // 24 min
  getStats = vi.fn(() => ({ videoDecodeFramerate: 24, videoRenderFramerate: 24, width: 1920, height: 1080 }))
  constructor(opts: Record<string, unknown>) {
    this.opts = opts
    FakeAVPlayer.instances.push(this)
  }
  on(ev: string, cb: (...a: unknown[]) => void) {
    ;(this.handlers[ev] ??= []).push(cb)
  }
  trigger(ev: string, ...a: unknown[]) {
    ;(this.handlers[ev] || []).forEach((cb) => cb(...a))
  }
}

const container = () => document.createElement('div') as HTMLDivElement

describe('useCompatEngine', () => {
  beforeEach(() => {
    FakeAVPlayer.instances.length = 0
    __setCompatLoaderForTest(async () => ({ default: FakeAVPlayer as never }))
    vi.stubGlobal('requestAnimationFrame', vi.fn(() => 1))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
  })
  afterEach(() => {
    __setCompatLoaderForTest(null)
    vi.unstubAllGlobals()
  })

  it('toMs/toSec convert between seconds and int64 milliseconds', () => {
    expect(toMs(90)).toBe(90000n)
    expect(toMs(1.5)).toBe(1500n)
    expect(toMs(-3)).toBe(0n)
    expect(toSec(90000n)).toBe(90)
  })

  it('activates with a pure-wasm config (no MSE, no hardware, no WebCodecs)', async () => {
    const engine = useCompatEngine()
    const ok = await engine.activate({ container: container(), url: '/m/x/playlist.m3u8', volume: 80, muted: false, rate: 1 })
    expect(ok).toBe(true)
    expect(engine.active.value).toBe(true)
    const p = FakeAVPlayer.instances[0]
    expect(p.opts.enableHardware).toBe(false)
    expect(p.opts.enableWebCodecs).toBe(false)
    expect((p.opts.checkUseMSE as () => boolean)()).toBe(false)
    expect(p.opts.wasmBaseUrl).toBe('/libmedia')
    expect(p.load).toHaveBeenCalledWith('/m/x/playlist.m3u8')
    expect(p.play).toHaveBeenCalled()
    expect(p.setVolume).toHaveBeenCalledWith(0.8)
    expect(engine.duration.value).toBe(1440)
  })

  it('seeks to the salvaged position (>=1s) with BigInt millis; skips sub-second', async () => {
    const engine = useCompatEngine()
    await engine.activate({ container: container(), url: '/pl', startAt: 33.3, volume: 100, muted: false, rate: 1 })
    expect(FakeAVPlayer.instances[0].seek).toHaveBeenCalledWith(33300n)

    await engine.destroy()
    await engine.activate({ container: container(), url: '/pl', startAt: 0.4, volume: 100, muted: false, rate: 1 })
    expect(FakeAVPlayer.instances[1].seek).not.toHaveBeenCalled()
  })

  it('mute is volume 0 with the previous level remembered', async () => {
    const engine = useCompatEngine()
    await engine.activate({ container: container(), url: '/pl', volume: 60, muted: true, rate: 1 })
    const p = FakeAVPlayer.instances[0]
    expect(p.setVolume).toHaveBeenLastCalledWith(0)
    engine.setMuted(false)
    expect(p.setVolume).toHaveBeenLastCalledWith(0.6)
  })

  it('surfaces engine errors and reports activation failure on load reject', async () => {
    const engine = useCompatEngine()
    await engine.activate({ container: container(), url: '/pl', volume: 100, muted: false, rate: 1 })
    FakeAVPlayer.instances[0].trigger('error', { message: 'decode blew up' })
    expect(engine.error.value).toContain('decode blew up')

    __setCompatLoaderForTest(async () => ({
      default: class extends FakeAVPlayer {
        load = vi.fn(async () => {
          throw new Error('404 playlist')
        })
      } as never,
    }))
    const engine2 = useCompatEngine()
    const ok = await engine2.activate({ container: container(), url: '/dead', volume: 100, muted: false, rate: 1 })
    expect(ok).toBe(false)
    expect(engine2.active.value).toBe(false)
    expect(engine2.error.value).toContain('404 playlist')
  })

  it('clockElement exposes the duck-typed video properties SubtitleOverlay reads', async () => {
    const engine = useCompatEngine()
    await engine.activate({ container: container(), url: '/pl', volume: 100, muted: false, rate: 1 })
    engine.currentTime.value = 12.5
    expect(engine.clockElement.currentTime).toBe(12.5)
    expect(engine.clockElement.duration).toBe(1440)
    expect(engine.clockElement.videoWidth).toBe(1920)
    expect(engine.clockElement.paused).toBe(false)
  })

  it('resizes to the container real dimensions once visible (AUTO-629 1x1-canvas regression)', async () => {
    // The host div is `v-show`-hidden until `active` flips, so at `new
    // AVPlayer()` time it measures 0x0; this simulates the container as it
    // is AFTER Vue paints it visible (what nextTick() should wait for).
    const mount = container()
    Object.defineProperty(mount, 'offsetWidth', { value: 960, configurable: true })
    Object.defineProperty(mount, 'offsetHeight', { value: 540, configurable: true })
    const engine = useCompatEngine()
    await engine.activate({ container: mount, url: '/pl', volume: 100, muted: false, rate: 1 })
    expect(FakeAVPlayer.instances[0].resize).toHaveBeenCalledWith(960, 540)
  })

  it('destroy tears the player down and deactivates', async () => {
    const engine = useCompatEngine()
    await engine.activate({ container: container(), url: '/pl', volume: 100, muted: false, rate: 1 })
    const p = FakeAVPlayer.instances[0]
    await engine.destroy()
    expect(p.destroy).toHaveBeenCalled()
    expect(engine.active.value).toBe(false)
    expect(engine.paused.value).toBe(true)
  })

  it('seekTo clamps into [0, duration] and updates the clock optimistically', async () => {
    const engine = useCompatEngine()
    await engine.activate({ container: container(), url: '/pl', volume: 100, muted: false, rate: 1 })
    engine.seekTo(9999)
    expect(engine.currentTime.value).toBe(1440)
    expect(FakeAVPlayer.instances[0].seek).toHaveBeenLastCalledWith(1_440_000n)
    engine.seekTo(-5)
    expect(engine.currentTime.value).toBe(0)
  })
})
