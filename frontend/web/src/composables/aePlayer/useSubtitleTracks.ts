import { ref, computed, unref, watch, type Ref, type ComputedRef } from 'vue'
import { subtitlesApi } from '@/api/client'
import { signedSubtitleUrl } from '@/utils/subtitleProxy'
import type { SubtitleTrack } from '@/types/aePlayer'
// SubTrack (the modal's prop shape) is field-identical to SubtitleTrack. Alias to
// the .ts type rather than importing a named type from a .vue file — the ambient
// `declare module '*.vue'` (default-export-only) shadows named exports under some
// vue-tsc versions (TS2614), which breaks the production build.
type SubTrack = SubtitleTrack

export interface BackendSubTrack {
  url: string; lang: string; label: string; format?: string; provider: string; release?: string
  // Provenance signature (streamsign) — present only on external track URLs
  // (jimaku.cc); forwarded to the HLS proxy as top-level exp/sig params.
  exp?: string; sig?: string
}
export interface AggregateSubsResponse {
  languages: Record<string, BackendSubTrack[]>
  episode: number
  providers_down?: string[]
}

/** Flatten the /subtitles/all languages map into SubtitleTrack[] — shared
 *  with the offline download engine (external subtitle capture). */
export function flattenAggregateSubs(data: AggregateSubsResponse | null | undefined): SubtitleTrack[] {
  const flat: SubtitleTrack[] = []
  for (const [lang, list] of Object.entries(data?.languages ?? {})) {
    for (const t of list) {
      flat.push({
        // Catalog-signed external tracks (jimaku.cc) are pre-wrapped in the
        // signed proxy URL so the un-allowlisted host loads without a 502;
        // same-origin tracks pass through raw.
        url: signedSubtitleUrl(t),
        provider: t.provider,
        lang: t.lang || lang,
        label: t.label || t.release || t.provider,
        format: (t.format || t.url.split('?')[0].split('.').pop() || 'srt').toLowerCase(),
      })
    }
  }
  return flat
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
      const data: AggregateSubsResponse = resp.data?.data ?? resp.data
      aggTracks.value = flattenAggregateSubs(data)
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
