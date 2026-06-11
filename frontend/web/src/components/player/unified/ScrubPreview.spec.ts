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

describe('ScrubPreview', () => {
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

  it('shows the static still fallback before any frame decodes', () => {
    const w = make({ stillUrl: 'https://x/still.jpg' })
    expect(w.find('[data-test="preview-still"]').exists()).toBe(true)
    expect(w.find('[data-test="preview-still"]').attributes('style')).toContain('still.jpg')
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

  it('throttles seeks — many hover moves collapse to few currentTime writes', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 10 })
    await vi.advanceTimersByTimeAsync(0)
    const video = w.find('[data-test="preview-video"]').element as HTMLVideoElement
    const writes: number[] = []
    Object.defineProperty(video, 'currentTime', {
      get: () => writes[writes.length - 1] ?? 0,
      set: (v: number) => void writes.push(v),
    })
    for (let t = 11; t <= 30; t++) {
      await w.setProps({ timeSec: t })
    }
    expect(writes.length).toBeLessThan(5)
    await vi.advanceTimersByTimeAsync(300)
    // trailing seek lands on the final hover position
    expect(writes[writes.length - 1]).toBe(30)
  })

  it('reveals the video only after a frame is decoded (seeked + readyState)', async () => {
    const w = make({ streamUrl: 'https://x/ep.mp4', streamType: 'mp4', stillUrl: 'https://x/s.jpg' })
    await w.setProps({ visible: true, timeSec: 5 })
    await vi.advanceTimersByTimeAsync(0)
    const video = w.find('[data-test="preview-video"]').element as HTMLVideoElement
    expect((w.find('[data-test="preview-video"]').element as HTMLElement).style.display).toBe('none')
    Object.defineProperty(video, 'readyState', { get: () => 2 })
    video.dispatchEvent(new Event('seeked'))
    await w.vm.$nextTick()
    expect((w.find('[data-test="preview-video"]').element as HTMLElement).style.display).not.toBe('none')
    expect(w.find('[data-test="preview-still"]').exists()).toBe(false)
  })

  it('tears down and re-arms when the stream URL changes', async () => {
    const w = make({ streamUrl: 'https://x/ep1.mp4', streamType: 'mp4' })
    await w.setProps({ visible: true, timeSec: 5 })
    await vi.advanceTimersByTimeAsync(0)
    await w.setProps({ streamUrl: 'https://x/ep2.mp4', timeSec: 6 })
    await vi.advanceTimersByTimeAsync(0)
    const video = w.find('[data-test="preview-video"]').element as HTMLVideoElement
    expect(video.getAttribute('src')).toBe('https://x/ep2.mp4')
  })
})
