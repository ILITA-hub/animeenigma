import { ref, watch, onUnmounted, type Ref } from 'vue'
import { scraperApi } from '@/api/client'
import { PROVIDER_REGISTRY } from '@/components/player/aePlayer/providerRegistry'
import type {
  ProviderRow,
  ProviderDef,
  ScraperProviderHealth,
  AudioKind,
  TrackLang,
  ContentKind,
} from '@/types/aePlayer'

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
    // Relevance check: content kind must match; audio + language must match too
    // EXCEPT for 18+ sources on a hentai title — they play a single combo-
    // agnostic source, so they stay visible in the menu (with their badge)
    // regardless of the audio/lang toggle.
    const isHentaiTitle = filter.content === 'hentai'
    const isAdultSource = def.content.includes('hentai')
    const relevant =
      isHentaiTitle && isAdultSource
        ? true
        : def.audios.includes(filter.audio) &&
          def.langs.includes(filter.lang) &&
          def.content.includes(filter.content)
    if (!relevant) {
      return { def, state: 'irrelevant', reason: relevanceReason(def, filter) }
    }
    // For scraper-backed providers, consult live health.
    if (def.scraper) {
      const h = byName.get(def.id)
      // Recovering: backend signals the provider is coming back online (Task 11).
      // Takes priority over status=degraded — recovering is the more specific
      // live signal; the provider is actively healing, so rank it above degraded.
      // Selectable in hacker mode (same as degraded) but ranked above it.
      if (h && h.health === 'recovering') {
        return { def, state: 'recovering', reason: h.reason || h.description }
      }
      // Soft-degraded wins over enabled/up: registered + manually selectable in
      // hacker mode, but never auto-used and ranked last (AUTO-484).
      if (h && h.status === 'degraded') {
        return { def, state: 'degraded', reason: h.reason || h.description }
      }
      if (h && !h.enabled) {
        return { def, state: 'disabled', reason: h.reason || h.description }
      }
      if (h && !h.up) {
        return { def, state: 'down', reason: 'Temporarily unreachable' } // up: absent ⇒ pessimistic 'down'
      }
    }
    // Non-scraper, non-disabled providers that are relevant fall through to 'active'.
    return { def, state: 'active' }
  })
}

function relevanceReason(def: Pick<ProviderDef, 'content'>, f: RowFilter): string {
  if (def.content.includes('hentai') && f.content !== 'hentai') return 'Only for 18+ titles'
  return `No ${f.audio}/${f.lang} stream from this source`
}

// ─── Shared singleton poller (page-fetch optimization 2026-06-11) ────────────
// The poll loop used to live per-composable-instance, so every mounted player
// (and every remount across tab switches) spawned its OWN 30s interval — the
// anime page fired /scraper/health 5+ times on load. The health state and the
// interval are now module-level: N subscribers share one loop, remounts within
// MIN_POLL_GAP_MS reuse the last result, and the loop pauses while the tab is
// hidden (immediate refresh on return when stale).

const MIN_POLL_GAP_MS = 15_000

const sharedHealth = ref<ScraperProviderHealth[]>([])
let sharedTimer: ReturnType<typeof setInterval> | null = null
let sharedIntervalMs = 30_000
let subscribers = 0
let lastPollAt = 0
let pollInFlight: Promise<void> | null = null

function pollShared(): Promise<void> {
  if (pollInFlight) return pollInFlight
  pollInFlight = (async () => {
    try {
      const resp = await scraperApi.getHealth()
      // The scraper handler uses httputil.OK → { success, data: { providers, playable } }.
      // The catalog writePassthrough forwards that body verbatim.
      // Axios puts the parsed JSON in resp.data, so the envelope is at resp.data.data.
      const rawProviders = (resp.data?.data?.providers ?? {}) as Record<
        string,
        { enabled?: boolean; status?: string; health?: string; up?: boolean; reason?: string; description?: string }
      >
      sharedHealth.value = Object.entries(rawProviders).map(([name, v]) => ({
        name,
        enabled: v.enabled ?? true,
        status: (v.status as 'enabled' | 'degraded' | 'disabled' | undefined) ?? undefined,
        health: (v.health as 'up' | 'recovering' | 'down' | undefined) ?? undefined,
        up: v.up ?? false,
        reason: v.reason,
        description: v.description,
      }))
      lastPollAt = Date.now()
    } catch {
      // Fail soft: keep last known health; registry-static states still render.
    } finally {
      pollInFlight = null
    }
  })()
  return pollInFlight
}

function startSharedLoop() {
  if (sharedTimer) return
  // Skip the immediate poll when a fresh result already exists (player
  // remounts on tab switches; no need to re-ask within the gap).
  if (Date.now() - lastPollAt >= MIN_POLL_GAP_MS) void pollShared()
  sharedTimer = setInterval(() => { void pollShared() }, sharedIntervalMs)
}

function stopSharedLoop() {
  if (sharedTimer) {
    clearInterval(sharedTimer)
    sharedTimer = null
  }
}

// Pause polling while the tab is hidden; refresh immediately on return when
// the last result has gone stale. Registered once per module load.
if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') {
      stopSharedLoop()
    } else if (subscribers > 0) {
      startSharedLoop()
    }
  })
}

/** Live composable: subscribes to the SHARED /scraper/health poll loop and
 *  exposes rows derived from registry + live health + this instance's filter. */
export function useProviderHealth(filter: Ref<RowFilter>, intervalMs = 30_000) {
  const rows = ref<ProviderRow[]>([])
  let subscribed = false

  const recompute = () => {
    rows.value = computeProviderRows(sharedHealth.value, filter.value)
  }

  // Re-derive rows immediately when the filter changes (e.g. audio/language
  // toggle) or when any subscriber's poll lands fresh health data.
  watch([filter, sharedHealth], recompute)

  const start = () => {
    if (subscribed) return // guard against double-start
    subscribed = true
    subscribers++
    sharedIntervalMs = intervalMs
    if (typeof document === 'undefined' || document.visibilityState !== 'hidden') startSharedLoop()
    recompute()
  }

  const stop = () => {
    if (!subscribed) return
    subscribed = false
    subscribers--
    if (subscribers <= 0) {
      subscribers = 0
      stopSharedLoop()
    }
  }

  onUnmounted(stop)

  return { rows, recompute, start, stop }
}
