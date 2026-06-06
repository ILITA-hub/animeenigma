import { ref, onUnmounted, type Ref } from 'vue'
import { scraperApi } from '@/api/client'
import { PROVIDER_REGISTRY } from '@/components/player/unified/providerRegistry'
import type {
  ProviderRow,
  ProviderDef,
  ScraperProviderHealth,
  AudioKind,
  TrackLang,
  ContentKind,
} from '@/types/unifiedPlayer'

export interface RowFilter { audio: AudioKind; lang: TrackLang; content: ContentKind }

/** Pure: registry + live scraper health + active filter → rendered rows. */
export function computeProviderRows(
  scraperHealth: ScraperProviderHealth[],
  filter: RowFilter,
): ProviderRow[] {
  const byName = new Map(scraperHealth.map(h => [h.name, h]))
  return PROVIDER_REGISTRY.map((def): ProviderRow => {
    // WIP always wins before anything else.
    if (def.staticDisabled?.wip) {
      return { def, state: 'wip', reason: def.staticDisabled.description }
    }
    // Hard-disabled (non-scraper) comes next.
    if (def.staticDisabled) {
      return { def, state: 'disabled', reason: def.staticDisabled.description }
    }
    // Relevance check: audio, language, and content kind must all match.
    const relevant =
      def.audios.includes(filter.audio) &&
      def.langs.includes(filter.lang) &&
      def.content.includes(filter.content)
    if (!relevant) {
      return { def, state: 'irrelevant', reason: relevanceReason(def, filter) }
    }
    // For scraper-backed providers, consult live health.
    if (def.scraper) {
      const h = byName.get(def.id)
      if (h && !h.enabled) {
        return { def, state: 'disabled', reason: h.reason || h.description }
      }
      if (h && !h.up) {
        return { def, state: 'down', reason: 'Temporarily unreachable' }
      }
    }
    return { def, state: 'active' }
  })
}

function relevanceReason(def: Pick<ProviderDef, 'content'>, f: RowFilter): string {
  if (def.content.includes('hentai') && f.content !== 'hentai') return 'Only for 18+ titles'
  return `No ${f.audio}/${f.lang} stream from this source`
}

/** Live composable: polls /scraper/health, exposes rows derived from registry + filter. */
export function useProviderHealth(filter: Ref<RowFilter>, intervalMs = 30_000) {
  const health = ref<ScraperProviderHealth[]>([])
  const rows = ref<ProviderRow[]>([])

  const recompute = () => {
    rows.value = computeProviderRows(health.value, filter.value)
  }

  async function poll() {
    try {
      const resp = await scraperApi.getHealth()
      // The scraper handler uses httputil.OK → { success, data: { providers, playable } }.
      // The catalog writePassthrough forwards that body verbatim.
      // Axios puts the parsed JSON in resp.data, so the envelope is at resp.data.data.
      const rawProviders = (resp.data?.data?.providers ?? {}) as Record<
        string,
        { enabled?: boolean; up?: boolean; reason?: string; description?: string }
      >
      health.value = Object.entries(rawProviders).map(([name, v]) => ({
        name,
        enabled: v.enabled ?? true,
        up: v.up ?? false,
        reason: v.reason,
        description: v.description,
      }))
    } catch {
      // Fail soft: keep last known health; registry-static states still render.
    }
    recompute()
  }

  let timer: ReturnType<typeof setInterval> | null = null

  const start = () => {
    void poll()
    timer = setInterval(() => { void poll() }, intervalMs)
  }

  const stop = () => {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  onUnmounted(stop)

  return { rows, recompute, start, stop }
}
