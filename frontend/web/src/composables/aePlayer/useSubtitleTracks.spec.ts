import { describe, it, expect, vi } from 'vitest'
import { ref } from 'vue'

const allMock = vi.fn()
vi.mock('@/api/client', () => ({ subtitlesApi: { all: (...a: unknown[]) => allMock(...a) } }))

import { useSubtitleTracks } from './useSubtitleTracks'


const flush = () => new Promise((r) => setTimeout(r, 0))

describe('useSubtitleTracks', () => {
  it('merges aggregation + provider tracks, deduped by url', async () => {
    allMock.mockResolvedValue({ data: { data: {
      languages: {
        ja: [{ url: '/api/j1.ass', lang: 'ja', label: 'Jimaku 1', format: 'ass', provider: 'jimaku' }],
        en: [{ url: '/api/os1.srt', lang: 'en', label: 'OS', format: 'srt', provider: 'opensubtitles' }],
      },
      episode: 8,
      providers_down: [],
    } } })
    const providerSubs = ref([{ url: '/proxy?url=en.vtt', provider: 'gogoanime', lang: 'en', label: 'Provider EN', format: 'vtt' }])
    const s = useSubtitleTracks('anime-1', ref(8), providerSubs)
    await s.ensureLoaded()
    await flush()
    expect(s.tracks.value).toHaveLength(3)
    expect(s.tracks.value.map((t) => t.provider).sort()).toEqual(['gogoanime', 'jimaku', 'opensubtitles'])
  })

  it('fails soft: aggregation error sets error but keeps provider tracks', async () => {
    allMock.mockRejectedValue(new Error('jimaku down'))
    const providerSubs = ref([{ url: '/p', provider: 'gogoanime', lang: 'en', label: 'P', format: 'vtt' }])
    const s = useSubtitleTracks('anime-1', ref(8), providerSubs)
    await s.ensureLoaded()
    await flush()
    expect(s.error.value).toBeTruthy()
    expect(s.tracks.value).toHaveLength(1) // provider track survives
  })

  it('surfaces providers_down', async () => {
    allMock.mockResolvedValue({ data: { data: { languages: {}, episode: 8, providers_down: ['jimaku'] } } })
    const s = useSubtitleTracks('anime-1', ref(8), ref([]))
    await s.ensureLoaded()
    await flush()
    expect(s.providersDown.value).toEqual(['jimaku'])
  })
})
