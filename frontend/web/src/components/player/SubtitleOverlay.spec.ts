import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
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
