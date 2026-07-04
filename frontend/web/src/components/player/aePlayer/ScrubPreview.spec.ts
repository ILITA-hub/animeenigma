import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ScrubPreview from './ScrubPreview.vue'
import { scrubDebug, sreset } from '@/composables/aePlayer/scrubPreviewDebug'

// hls.js is dynamically imported only on the hls path — mock it so the mp4
// tests never pull the real module. `hlsMockState` is read/written by the
// mock factory (hoisted above this module's imports by vi.mock), so it must
// be created via vi.hoisted rather than a plain module-scope `let`.
const hlsMockState = vi.hoisted(() => ({
  isSupported: false,
  lastConfig: null as Record<string, unknown> | null,
}))

vi.mock('hls.js', () => ({
  default: class MockHls {
    static isSupported() {
      return hlsMockState.isSupported // jsdom — false falls back to native-src path
    }
    static Events = { MANIFEST_PARSED: 'hlsManifestParsed' }
    constructor(config: Record<string, unknown>) {
      hlsMockState.lastConfig = config
    }
    loadSource() {}
    attachMedia() {}
    on() {}
    destroy() {}
  },
}))

describe('ScrubPreview (thumbnail-cache v2)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    sreset()
    hlsMockState.isSupported = false
    hlsMockState.lastConfig = null
  })
  afterEach(() => vi.useRealTimers())

  function make(props: Partial<InstanceType<typeof ScrubPreview>['$props']> = {}) {
    return mount(ScrubPreview, {
      props: {
        timeSec: 0,
        visible: false,
        streamUrl: null,
        streamType: null,
        ...props,
      },
    })
  }

  /** Instrument the shadow video: capture currentTime writes, fake readiness. */
  function instrument(w: ReturnType<typeof make>) {
    const video = w.find('[data-test="preview-video"]').element as HTMLVideoElement
    const writes: number[] = []
    Object.defineProperty(video, 'currentTime', {
      get: () => writes[writes.length - 1] ?? 0,
      set: (v: number) => void writes.push(v),
    })
    Object.defineProperty(video, 'readyState', { get: () => 2, configurable: true })
    return { video, writes }
  }

  it('shows the static still fallback before any frame is cached', () => {
    const w = make({ stillUrl: 'https://x/still.jpg' })
    expect(w.find('[data-test="preview-still"]').exists()).toBe(true)
    expect(w.find('[data-test="preview-still"]').attributes('style')).toContain('still.jpg')
    expect((w.find('[data-test="preview-canvas"]').element as HTMLElement).style.display).toBe(
      'none',
    )
  })

  it('initializes EAGERLY after a startup-grace delay, without any hover', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    const video = w.find('[data-test="preview-video"]').element as HTMLVideoElement
    // Immediately after mount: not yet — the main player wins startup bandwidth.
    expect(video.getAttribute('src')).toBeNull()
    await vi.advanceTimersByTimeAsync(4000)
    expect(video.getAttribute('src')).toBe('https://x/ep.mp4')
  })

  it('warms spread thumbnails in the background with zero hovering', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await vi.advanceTimersByTimeAsync(4000) // eager init
    const { video, writes } = instrument(w)
    const DUR = 1100
    const POINTS = 16 // mirrors PREFETCH_POINTS in ScrubPreview.vue
    Object.defineProperty(video, 'duration', { get: () => DUR, configurable: true })

    // Evenly-spaced prefetch points, each snapped to the 5s bucket grid.
    const expected = Array.from({ length: POINTS }, (_, i) =>
      Math.max(0, Math.round((DUR * (i + 1)) / (POINTS + 1) / 5)) * 5,
    )

    // Duration known → prefetch arms and pumps the first point on its own.
    video.dispatchEvent(new Event('loadedmetadata'))
    await w.vm.$nextTick()
    expect(writes.length).toBe(1)
    expect(writes[0]).toBe(expected[0])

    // Each decoded thumbnail pumps the next point — chain runs to completion.
    for (let i = 1; i < POINTS; i++) {
      video.dispatchEvent(new Event('seeked'))
      await w.vm.$nextTick()
      expect(writes[writes.length - 1]).toBe(expected[i])
    }
    video.dispatchEvent(new Event('seeked'))
    await w.vm.$nextTick()
    await vi.advanceTimersByTimeAsync(2000)
    expect(writes.length).toBe(POINTS) // queue exhausted — no runaway seeking
  })

  it('a pump blocked by an active hover RETRIES on a timer instead of stalling', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await vi.advanceTimersByTimeAsync(4000)
    const { video, writes } = instrument(w)
    Object.defineProperty(video, 'duration', { get: () => 1100, configurable: true })

    // User starts hovering an uncached spot → settle timer active.
    await w.setProps({ visible: true, timeSec: 42 })
    video.dispatchEvent(new Event('loadedmetadata')) // pump blocked by settle
    await w.vm.$nextTick()
    expect(writes.length).toBe(0)

    // Settle fires (user seek), its frame decodes…
    await vi.advanceTimersByTimeAsync(200)
    expect(writes[0]).toBe(40) // bucket 8 — user's hover seek
    video.dispatchEvent(new Event('seeked'))
    await w.vm.$nextTick()
    // …and the pump self-recovers: prefetch continues without further input.
    await vi.advanceTimersByTimeAsync(600)
    expect(writes.length).toBeGreaterThanOrEqual(2)
    // First background prefetch point: 1100 × 1/17 ≈ 64.7 → bucket 13 → t=65.
    expect(writes[1]).toBe(65)
  })

  it('sets the mp4 src on first hover', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 30 })
    await vi.advanceTimersByTimeAsync(0)
    const video = w.find('[data-test="preview-video"]').element as HTMLVideoElement
    expect(video.getAttribute('src')).toBe('https://x/ep.mp4')
  })

  it('tags shadow-engine HLS requests via xhrSetup with the aescrub=1 marker', async () => {
    hlsMockState.isSupported = true
    const w = make({ streamUrl: 'https://x/master.m3u8', streamType: 'hls' })
    await w.setProps({ visible: true, timeSec: 5 })
    await vi.advanceTimersByTimeAsync(0)

    expect(hlsMockState.lastConfig).toBeTruthy()
    const xhrSetup = hlsMockState.lastConfig?.xhrSetup as
      | ((xhr: XMLHttpRequest, url: string) => void)
      | undefined
    expect(typeof xhrSetup).toBe('function')

    const proxyUrl =
      '/api/streaming/hls-proxy?url=' + encodeURIComponent('https://cdn.example/v/segment_001.ts')
    const fakeXhr = { open: vi.fn() } as unknown as XMLHttpRequest
    xhrSetup!(fakeXhr, proxyUrl)
    expect(fakeXhr.open).toHaveBeenCalledTimes(1)
    const [method, calledUrl, isAsync] = (fakeXhr.open as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(method).toBe('GET')
    expect(calledUrl).toContain('aescrub=1')
    expect(isAsync).toBe(true)

    // A non-proxy URL must be left completely untouched — no open() call.
    const otherUrl = 'https://cdn.example/other/master.m3u8'
    const fakeXhrUntouched = { open: vi.fn() } as unknown as XMLHttpRequest
    xhrSetup!(fakeXhrUntouched, otherUrl)
    expect(fakeXhrUntouched.open).not.toHaveBeenCalled()
  })

  it('a fast hover sweep issues NO seeks until the pointer settles, then ONE', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 10 })
    await vi.advanceTimersByTimeAsync(0)
    const { writes } = instrument(w)
    for (let t = 11; t <= 60; t++) {
      await w.setProps({ timeSec: t })
      await vi.advanceTimersByTimeAsync(20) // 20ms between moves — never settles
    }
    // The v1 bug: every throttle tick wrote currentTime, aborting the previous
    // fetch. v2 must not seek at all while the pointer keeps moving.
    expect(writes.length).toBe(0)

    await vi.advanceTimersByTimeAsync(300) // pointer rests → one settle seek
    expect(writes.length).toBe(1)
    expect(writes[0]).toBe(60) // bucket center of t=60 (bucket 12 × 5s)
  })

  it('reveals the canvas after a frame decodes and caches it', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4', stillUrl: 'https://x/s.jpg' })
    await w.setProps({ visible: true, timeSec: 5 })
    await vi.advanceTimersByTimeAsync(0)
    const { video, writes } = instrument(w)
    await vi.advanceTimersByTimeAsync(300)
    expect(writes.length).toBe(1) // settle seek to bucket 1 (t=5)

    video.dispatchEvent(new Event('seeked'))
    await w.vm.$nextTick()
    expect(
      (w.find('[data-test="preview-canvas"]').element as HTMLElement).style.display,
    ).not.toBe('none')
    expect(w.find('[data-test="preview-still"]').exists()).toBe(false)
  })

  it('re-hovering a cached bucket issues NO new seek (zero network)', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 5 })
    await vi.advanceTimersByTimeAsync(0)
    const { video, writes } = instrument(w)
    await vi.advanceTimersByTimeAsync(300)
    expect(writes.length).toBe(1)
    video.dispatchEvent(new Event('seeked')) // caches bucket 1 (currentTime=5)
    await w.vm.$nextTick()

    // Leave and come back to the same bucket.
    await w.setProps({ timeSec: 6 }) // still bucket 1 (round(6/5)=1)
    await vi.advanceTimersByTimeAsync(1000)
    expect(writes.length).toBe(1) // no refetch — the v1 bug refetched everything
  })

  it('hovering an uncached spot shows the NEAREST cached frame instantly', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4', stillUrl: 'https://x/s.jpg' })
    await w.setProps({ visible: true, timeSec: 5 })
    await vi.advanceTimersByTimeAsync(0)
    const { video } = instrument(w)
    await vi.advanceTimersByTimeAsync(300)
    video.dispatchEvent(new Event('seeked')) // cache bucket 1
    await w.vm.$nextTick()

    await w.setProps({ timeSec: 500 }) // far uncached position
    await w.vm.$nextTick()
    // Canvas (nearest cached neighbour) stays visible — NOT the poster still.
    expect(
      (w.find('[data-test="preview-canvas"]').element as HTMLElement).style.display,
    ).not.toBe('none')
    expect(w.find('[data-test="preview-still"]').exists()).toBe(false)
  })

  it('records the seek→capture pipeline in the scrubDebug channel', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 5 })
    await vi.advanceTimersByTimeAsync(0)
    expect(scrubDebug.engine).toBe('loading')
    expect(scrubDebug.streamType).toBe('mp4')
    expect(scrubDebug.events.some((e) => e.includes('init mp4'))).toBe(true)

    const { video } = instrument(w)
    await vi.advanceTimersByTimeAsync(300) // settle seek
    expect(scrubDebug.seeks).toBe(1)
    expect(scrubDebug.events.some((e) => e.includes('seek →5s b1 (hover)'))).toBe(true)

    video.dispatchEvent(new Event('seeked'))
    await w.vm.$nextTick()
    expect(scrubDebug.captures).toBe(1)
    expect(scrubDebug.engine).toBe('ready')
    expect(scrubDebug.lastCaptureMs).not.toBeNull()
    expect(scrubDebug.cacheSize).toBe(1)
  })

  it('counts a watchdog timeout when the provider never answers a seek', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 5 })
    await vi.advanceTimersByTimeAsync(0)
    instrument(w)
    await vi.advanceTimersByTimeAsync(300) // settle seek issued — never decodes
    expect(scrubDebug.seeks).toBe(1)

    await vi.advanceTimersByTimeAsync(8100)
    expect(scrubDebug.watchdogs).toBe(1)
    expect(scrubDebug.captures).toBe(0)
    expect(scrubDebug.events.some((e) => e.includes('WATCHDOG'))).toBe(true)
  })

  it('a media error marks the engine dead — visibly, not silently', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 5 })
    await vi.advanceTimersByTimeAsync(0)
    const { video } = instrument(w)
    video.dispatchEvent(new Event('error'))
    expect(scrubDebug.engine).toBe('error')
    expect(scrubDebug.errors).toBe(1)
    expect(scrubDebug.events.some((e) => e.includes('media error'))).toBe(true)
  })

  it('tears down, clears the cache, and re-arms when the stream URL changes', async () => {
    const w = make({ streamUrl: 'https://x/ep1.mp4', streamType: 'mp4', stillUrl: 'https://x/s.jpg' })
    await w.setProps({ visible: true, timeSec: 5 })
    await vi.advanceTimersByTimeAsync(0)
    const { video } = instrument(w)
    await vi.advanceTimersByTimeAsync(300)
    video.dispatchEvent(new Event('seeked')) // cache a frame for ep1
    await w.vm.$nextTick()

    await w.setProps({ streamUrl: 'https://x/ep2.mp4', timeSec: 6 })
    await vi.advanceTimersByTimeAsync(0)
    const v2 = w.find('[data-test="preview-video"]').element as HTMLVideoElement
    expect(v2.getAttribute('src')).toBe('https://x/ep2.mp4')
    // ep1 frames must not leak into ep2's bubble — back to the still.
    expect(w.find('[data-test="preview-still"]').exists()).toBe(true)
  })

  // ── storyboard sprite mode ─────────────────────────────────────────────────
  // When a WebVTT thumbnail track is present, the scrub preview draws sprite
  // crops from pre-baked sheets and NEVER starts its shadow hls.js engine.
  describe('storyboard mode', () => {
    // Two cues over [0,5) and [5,10); t=2 lands in the first.
    const SAMPLE_VTT = `WEBVTT

00:00:00.000 --> 00:00:05.000
/api/streaming/hls-proxy?url=a&exp=1&sig=b#xywh=0,0,160,90

00:00:05.000 --> 00:00:10.000
/api/streaming/hls-proxy?url=a&exp=1&sig=b#xywh=160,0,160,90
`

    // Sheet images "load" the instant their src is set: complete/naturalWidth
    // flip synchronously (so the first renderStoryboard draws immediately) and
    // onload ALSO fires on a microtask, so a handler assigned right after
    // `.src =` (as the component does) still runs.
    class MockImage {
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      complete = false
      naturalWidth = 0
      naturalHeight = 0
      private _src = ''
      set src(v: string) {
        this._src = v
        this.complete = true
        this.naturalWidth = 320
        this.naturalHeight = 180
        queueMicrotask(() => this.onload?.())
      }
      get src() {
        return this._src
      }
    }

    afterEach(() => {
      vi.unstubAllGlobals()
    })

    it('fetches the storyboard VTT exactly once on mount', async () => {
      const fetchMock = vi.fn(() =>
        Promise.resolve({ ok: true, text: () => Promise.resolve(SAMPLE_VTT) }),
      )
      vi.stubGlobal('fetch', fetchMock)
      vi.stubGlobal('Image', MockImage)

      make({
        streamUrl: 'https://x/master.m3u8',
        streamType: 'hls',
        storyboardUrl: 'https://x/sb.vtt',
      })
      await flushPromises()

      expect(fetchMock).toHaveBeenCalledTimes(1)
      expect(fetchMock).toHaveBeenCalledWith('https://x/sb.vtt')
    })

    it('renders a sprite crop on hover WITHOUT constructing the shadow hls.js engine', async () => {
      hlsMockState.isSupported = true // the hls path WOULD construct if it ran
      const fetchMock = vi.fn(() =>
        Promise.resolve({ ok: true, text: () => Promise.resolve(SAMPLE_VTT) }),
      )
      vi.stubGlobal('fetch', fetchMock)
      vi.stubGlobal('Image', MockImage)

      const w = make({
        streamUrl: 'https://x/master.m3u8',
        streamType: 'hls',
        storyboardUrl: 'https://x/sb.vtt',
      })
      await flushPromises() // VTT settles → sprite mode active, storyPending false

      await w.setProps({ visible: true, timeSec: 2 }) // inside cue [0,5)
      await flushPromises()

      // Canvas is revealed (sprite drawn) …
      expect(
        (w.find('[data-test="preview-canvas"]').element as HTMLElement).style.display,
      ).not.toBe('none')
      // … and the whole point: the shadow engine is never booted.
      expect(hlsMockState.lastConfig).toBeNull()
      const video = w.find('[data-test="preview-video"]').element as HTMLVideoElement
      expect(video.getAttribute('src')).toBeNull()
    })

    it('a hover DURING the VTT load does not boot the shadow engine (storyPending gate)', async () => {
      hlsMockState.isSupported = true
      let resolveVtt: (v: { ok: boolean; text: () => Promise<string> }) => void = () => {}
      const fetchMock = vi.fn(
        () =>
          new Promise<{ ok: boolean; text: () => Promise<string> }>((res) => {
            resolveVtt = res
          }),
      )
      vi.stubGlobal('fetch', fetchMock)
      vi.stubGlobal('Image', MockImage)

      const w = make({
        streamUrl: 'https://x/master.m3u8',
        streamType: 'hls',
        storyboardUrl: 'https://x/sb.vtt',
      })
      // Hover while the fetch is still in flight.
      await w.setProps({ visible: true, timeSec: 2 })
      await w.vm.$nextTick()
      expect(hlsMockState.lastConfig).toBeNull() // gated — no engine yet

      // Let the VTT resolve → sprite mode; still no engine.
      resolveVtt({ ok: true, text: () => Promise.resolve(SAMPLE_VTT) })
      await flushPromises()
      expect(hlsMockState.lastConfig).toBeNull()
    })

    it('falls back to the shadow engine when the VTT fetch rejects', async () => {
      hlsMockState.isSupported = true
      const fetchMock = vi.fn(() => Promise.reject(new Error('network down')))
      vi.stubGlobal('fetch', fetchMock)
      vi.stubGlobal('Image', MockImage)

      const w = make({
        streamUrl: 'https://x/master.m3u8',
        streamType: 'hls',
        storyboardUrl: 'https://x/sb.vtt',
      })
      await flushPromises() // load rejects → storyCues=null, storyPending=false

      await w.setProps({ visible: true, timeSec: 5 })
      await flushPromises()

      // No sprite mode → the hover falls through to ensureEngine's hls path.
      expect(hlsMockState.lastConfig).toBeTruthy()
    })

    it('with storyboardUrl=null it behaves exactly as before (no fetch, eager shadow init)', async () => {
      const fetchMock = vi.fn()
      vi.stubGlobal('fetch', fetchMock)

      const w = make({
        streamUrl: 'https://x/ep.mp4',
        streamType: 'mp4',
        storyboardUrl: null,
      })
      await vi.advanceTimersByTimeAsync(4000) // eager-init grace window

      expect(fetchMock).not.toHaveBeenCalled()
      const video = w.find('[data-test="preview-video"]').element as HTMLVideoElement
      expect(video.getAttribute('src')).toBe('https://x/ep.mp4')
    })
  })
})
