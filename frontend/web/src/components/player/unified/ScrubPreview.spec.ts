import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ScrubPreview from './ScrubPreview.vue'
import { scrubDebug, sreset } from '@/composables/unifiedPlayer/scrubPreviewDebug'

// hls.js is dynamically imported only on the hls path — mock it so the mp4
// tests never pull the real module.
vi.mock('hls.js', () => ({
  default: class MockHls {
    static isSupported() {
      return false // jsdom — fall back to the native-src path
    }
    static Events = { MANIFEST_PARSED: 'hlsManifestParsed' }
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

  it('warms 10 spread thumbnails in the background with zero hovering', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await vi.advanceTimersByTimeAsync(4000) // eager init
    const { video, writes } = instrument(w)
    Object.defineProperty(video, 'duration', { get: () => 1100, configurable: true })

    // Duration known → prefetch arms and pumps the first point on its own.
    video.dispatchEvent(new Event('loadedmetadata'))
    await w.vm.$nextTick()
    expect(writes.length).toBe(1)
    expect(writes[0]).toBe(100) // 1100 × 1/11 → bucket 20 → t=100

    // Each decoded thumbnail pumps the next point — chain runs to completion.
    for (let i = 2; i <= 10; i++) {
      video.dispatchEvent(new Event('seeked'))
      await w.vm.$nextTick()
      expect(writes[writes.length - 1]).toBe(i * 100)
    }
    video.dispatchEvent(new Event('seeked'))
    await w.vm.$nextTick()
    await vi.advanceTimersByTimeAsync(2000)
    expect(writes.length).toBe(10) // queue exhausted — no runaway seeking
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
    expect(writes[1]).toBe(100)
  })

  it('sets the mp4 src on first hover', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 30 })
    await vi.advanceTimersByTimeAsync(0)
    const video = w.find('[data-test="preview-video"]').element as HTMLVideoElement
    expect(video.getAttribute('src')).toBe('https://x/ep.mp4')
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
})
