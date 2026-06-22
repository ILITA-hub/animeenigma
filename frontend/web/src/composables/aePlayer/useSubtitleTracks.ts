import { ref, computed, unref, watch, type Ref, type ComputedRef } from 'vue'
import { subtitlesApi } from '@/api/client'
import type { SubtitleTrack } from '@/types/aePlayer'
// SubTrack (the modal's prop shape) is field-identical to SubtitleTrack. Alias to
// the .ts type rather than importing a named type from a .vue file — the ambient
// `declare module '*.vue'` (default-export-only) shadows named exports under some
// vue-tsc versions (TS2614), which breaks the production build.
type SubTrack = SubtitleTrack

interface BackendSubTrack {
  url: string; lang: string; label: string; format?: string; provider: string; release?: string
}
interface AggregateResponse {
  languages: Record<string, BackendSubTrack[]>
  episode: number
  providers_down?: string[]
}

export function useSubtitleTracks(
  animeId: Ref<string> | string,
  episode: Ref<number | undefined>,
  providerSubtitles: Ref<SubtitleTrack[] | undefined>,
) {
  const aggTracks = ref<SubTrack[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const providersDown = ref<string[]>([])
  let loadedEpisode: number | null = null
  let inFlight: Promise<void> | null = null

  async function fetchFor(ep: number): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const resp = await subtitlesApi.all(unref(animeId), ep)
      const data: AggregateResponse = resp.data?.data ?? resp.data
      const flat: SubTrack[] = []
      for (const [lang, list] of Object.entries(data?.languages ?? {})) {
        for (const t of list) {
          flat.push({
            url: t.url,
            provider: t.provider,
            lang: t.lang || lang,
            label: t.label || t.release || t.provider,
            format: (t.format || t.url.split('?')[0].split('.').pop() || 'srt').toLowerCase(),
          })
        }
      }
      aggTracks.value = flat
      providersDown.value = data?.providers_down ?? []
      loadedEpisode = ep
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      // keep aggTracks as-is (provider tracks still merge below)
    } finally {
      loading.value = false
    }
  }

  async function ensureLoaded(): Promise<void> {
    const ep = episode.value
    if (ep == null) return
    if (loadedEpisode === ep && !error.value) return
    if (inFlight) return inFlight
    inFlight = fetchFor(ep).finally(() => { inFlight = null })
    return inFlight
  }

  async function refetch(): Promise<void> {
    loadedEpisode = null
    return ensureLoaded()
  }

  // Reset aggregation cache when the episode changes (provider tracks come
  // from the live stream and are merged reactively).
  watch(episode, () => { loadedEpisode = null; aggTracks.value = []; providersDown.value = [] })

  const tracks: ComputedRef<SubTrack[]> = computed(() => {
    const provider = (providerSubtitles.value ?? []) as SubTrack[]
    const seen = new Set<string>()
    const out: SubTrack[] = []
    for (const t of [...provider, ...aggTracks.value]) {
      if (seen.has(t.url)) continue
      seen.add(t.url)
      out.push(t)
    }
    return out
  })

  return { tracks, loading, error, providersDown, ensureLoaded, refetch }
}
