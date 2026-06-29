import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fetchAndParseCues } from '../subtitle-parser'

const SRT = `1\n00:00:08,000 --> 00:00:10,000\nhello\n\n2\n00:00:18,000 --> 00:00:21,000\nworld\n`

describe('fetchAndParseCues', () => {
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, text: async () => SRT }))) })
  it('fetches and parses by explicit format', async () => {
    const cues = await fetchAndParseCues('https://cdn.example/x.srt', 'srt')
    expect(cues.length).toBe(2)
    expect(cues[0]).toMatchObject({ start: 8, end: 10 })
  })
  it('throws on a non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 404, text: async () => '' })))
    await expect(fetchAndParseCues('https://cdn.example/x.srt', 'srt')).rejects.toThrow()
  })
})
