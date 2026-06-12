import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import ScrubPreview from './ScrubPreview.vue'

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
  beforeEach(() => vi.useFakeTimers())
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

  it('does NOT initialize the shadow video while never hovered (lazy)', () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    const video = w.find('[data-test="preview-video"]').element as HTMLVideoElement
    expect(video.getAttribute('src')).toBeNull()
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

  it('prefetches evenly-spaced timeline points in the background once idle', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 0 })
    await vi.advanceTimersByTimeAsync(0)
    const { video, writes } = instrument(w)
    Object.defineProperty(video, 'duration', { get: () => 1000, configurable: true })

    await vi.advanceTimersByTimeAsync(300)
    expect(writes.length).toBe(1) // settle seek for bucket 0

    // Frame decodes → capture arms the prefetch queue and pumps the first point.
    video.dispatchEvent(new Event('seeked'))
    await w.vm.$nextTick()
    expect(writes.length).toBe(2)
    expect(writes[1]).toBe(100) // duration*1/10 → bucket 20 → t=100

    // Each completed prefetch pumps the next point.
    video.dispatchEvent(new Event('seeked'))
    await w.vm.$nextTick()
    expect(writes.length).toBe(3)
    expect(writes[2]).toBe(200)
  })

  it('user hover interrupts the prefetch pump (hover always wins)', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 0 })
    await vi.advanceTimersByTimeAsync(0)
    const { video, writes } = instrument(w)
    Object.defineProperty(video, 'duration', { get: () => 1000, configurable: true })
    await vi.advanceTimersByTimeAsync(300)
    video.dispatchEvent(new Event('seeked')) // arms prefetch, pumps t=100
    await w.vm.$nextTick()
    expect(writes[writes.length - 1]).toBe(100)

    // User hovers an uncached spot — settle pending blocks the pump.
    await w.setProps({ timeSec: 700 })
    video.dispatchEvent(new Event('seeked')) // the t=100 prefetch completes
    await w.vm.$nextTick()
    // Pump must NOT fire (settle timer active); next write is the user's seek.
    await vi.advanceTimersByTimeAsync(300)
    expect(writes[writes.length - 1]).toBe(700)
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
