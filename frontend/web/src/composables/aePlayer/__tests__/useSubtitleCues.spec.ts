import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref, nextTick } from 'vue'
import { useSubtitleCues } from '../useSubtitleCues'

const SRT = `1\n00:00:08,000 --> 00:00:10,000\nhello\n\n2\n00:00:18,000 --> 00:00:21,000\nworld\n`

describe('useSubtitleCues', () => {
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, text: async () => SRT }))) })
  it('fetches and parses cues when url is set', async () => {
    const { cues } = useSubtitleCues(ref<string | null>('https://cdn.example/x.srt'), ref('srt'))
    await nextTick(); await Promise.resolve(); await nextTick()
    expect(cues.value.length).toBe(2)
    expect(cues.value[0]).toMatchObject({ start: 8, end: 10 })
  })
  it('clears cues when url becomes null', async () => {
    const url = ref<string | null>('https://cdn.example/x.srt')
    const { cues } = useSubtitleCues(url, ref('srt'))
    await nextTick(); await Promise.resolve(); await nextTick()
    expect(cues.value.length).toBe(2)
    url.value = null; await nextTick()
    expect(cues.value).toEqual([])
  })
})
