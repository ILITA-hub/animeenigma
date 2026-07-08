import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import SubtitleOverlay from './SubtitleOverlay.vue'

function mockFetchText(text: string) {
  const spy = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: () => Promise.resolve(text),
  })
  vi.stubGlobal('fetch', spy)
  return spy
}

describe('SubtitleOverlay URL fetching', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('fetches same-origin (/api/...) URLs directly without the hls-proxy', async () => {
    const fetchSpy = mockFetchText('WEBVTT\n')
    mount(SubtitleOverlay, {
      props: {
        videoElement: null,
        subtitleUrl: '/api/anime/abc/subtitles/opensubtitles/file/42',
        format: 'vtt',
        visible: true,
        fullscreenContainer: null,
      },
    })
    await flushPromises()
    expect(fetchSpy).toHaveBeenCalledTimes(1)
    expect(fetchSpy.mock.calls[0][0]).toBe('/api/anime/abc/subtitles/opensubtitles/file/42')
  })

  it('wraps external URLs in the hls-proxy', async () => {
    const fetchSpy = mockFetchText('WEBVTT\n')
    mount(SubtitleOverlay, {
      props: {
        videoElement: null,
        subtitleUrl: 'https://jimaku.cc/file.srt',
        format: 'srt',
        visible: true,
        fullscreenContainer: null,
      },
    })
    await flushPromises()
    expect(fetchSpy.mock.calls[0][0]).toContain('/api/streaming/hls-proxy?url=')
    expect(fetchSpy.mock.calls[0][0]).toContain(encodeURIComponent('https://jimaku.cc/file.srt'))
  })
})

describe('SubtitleOverlay appearance (size + background)', () => {
  beforeEach(() => vi.restoreAllMocks())

  // A cue spanning 0→30s is active at the default currentTime (0), so no rAF
  // driving is needed to render it.
  const VTT = 'WEBVTT\n\n00:00.000 --> 00:30.000\nHello\n'
  // clientHeight 400 → baseFontSize = max(16, min(48, 400*0.035=14)) = 16px.
  const fakeVideo = { currentTime: 0, clientHeight: 400, videoWidth: 1280, videoHeight: 720 } as unknown as HTMLVideoElement

  it('scales font-size and background opacity from the sizeScale/bgOpacity props', async () => {
    mockFetchText(VTT)
    const wrapper = mount(SubtitleOverlay, {
      props: {
        videoElement: fakeVideo,
        subtitleUrl: '/api/x.vtt',
        format: 'vtt',
        visible: true,
        sizeScale: 200,
        bgOpacity: 100,
      },
    })
    await flushPromises()
    await nextTick()

    const span = wrapper.find('.subtitle-text')
    expect(span.exists()).toBe(true)
    const style = span.attributes('style') || ''
    // base 16px × (200/100) = 32px
    expect(style).toContain('font-size: 32px')
    // bgOpacity 100 → alpha = 1.00 × 0.85 = 0.85
    expect(style).toMatch(/rgba\(0,\s*0,\s*0,\s*0\.85\)/)
  })

  it('defaults to base size (100%) and honors a lower background opacity', async () => {
    mockFetchText(VTT)
    const wrapper = mount(SubtitleOverlay, {
      props: {
        videoElement: fakeVideo,
        subtitleUrl: '/api/x.vtt',
        format: 'vtt',
        visible: true,
        bgOpacity: 0,
      },
    })
    await flushPromises()
    await nextTick()

    const span = wrapper.find('.subtitle-text')
    expect(span.exists()).toBe(true)
    const style = span.attributes('style') || ''
    // sizeScale omitted → defaults to 100 → base 16px unchanged
    expect(style).toContain('font-size: 16px')
    // bgOpacity 0 → fully transparent background
    expect(style).toMatch(/rgba\(0,\s*0,\s*0,\s*0(\.00)?\)/)
  })
})
