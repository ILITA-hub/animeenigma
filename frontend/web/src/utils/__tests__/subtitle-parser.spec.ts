import { describe, it, expect, vi, beforeEach } from 'vitest'

const { currentBase, ownsUrl } = vi.hoisted(() => ({
  currentBase: vi.fn(() => ''),
  ownsUrl: vi.fn((url: string) => url.startsWith('/')),
}))

vi.mock('@/utils/protocolLadder', () => ({
  ladder: { currentBase, ownsUrl },
}))

import { fetchAndParseCues } from '../subtitle-parser'

const SRT = `1\n00:00:08,000 --> 00:00:10,000\nhello\n\n2\n00:00:18,000 --> 00:00:21,000\nworld\n`

describe('fetchAndParseCues', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, text: async () => SRT })))
    currentBase.mockReturnValue('')
    ownsUrl.mockImplementation((url: string) => url.startsWith('/'))
  })
  it('fetches and parses by explicit format', async () => {
    const cues = await fetchAndParseCues('https://cdn.example/x.srt', 'srt')
    expect(cues.length).toBe(2)
    expect(cues[0]).toMatchObject({ start: 8, end: 10 })
  })
  it('throws on a non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 404, text: async () => '' })))
    await expect(fetchAndParseCues('https://cdn.example/x.srt', 'srt')).rejects.toThrow()
  })
  it('fetches an already-masked absolute proxy URL directly, without double-wrapping through hlsProxyUrl (AePlayerPlaybackFailures regression)', async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, text: async () => SRT }))
    vi.stubGlobal('fetch', fetchMock)
    ownsUrl.mockImplementation((url: string) => url.startsWith('https://stream2.animeenigma.org'))
    const maskedUrl = 'https://stream2.animeenigma.org/api/streaming/m/TOKEN/track_0_eng.vtt'
    await fetchAndParseCues(maskedUrl, 'vtt')
    expect(fetchMock).toHaveBeenCalledWith(maskedUrl, undefined)
  })
})
