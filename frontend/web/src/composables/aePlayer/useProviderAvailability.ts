import { ref, watch, type Ref } from 'vue'
import { scraperApi } from '@/api/client'
import type { ProviderRow } from '@/types/aePlayer'

export type AvailReason = 'not_found' | 'cdn_unreachable'
export interface ProviderAvailability {
  available: boolean
  reason?: AvailReason
}

/**
 * Hacker-mode-only, lazy + cached per-provider availability for the current
 * anime. `checkExists` does a single no-failover `getEpisodes(exclusive=true)`
 * "search" — a 404 means the provider genuinely lacks this anime. A resolve or
 * playback failure on a provider that DOES have it is recorded via
 * `markCdnUnreachable`. Never invoked for casual users (gated at the call site).
 */
export function useProviderAvailability(animeId: Ref<string>) {
  const cache = ref(new Map<string, ProviderAvailability>())
  const inflight = new Map<string, Promise<void>>()

  function reset() {
    cache.value = new Map()
    inflight.clear()
  }
  watch(animeId, reset)

  function get(providerId: string): ProviderAvailability | undefined {
    return cache.value.get(providerId)
  }

  function set(providerId: string, v: ProviderAvailability) {
    const next = new Map(cache.value)
    next.set(providerId, v)
    cache.value = next
  }

  function markCdnUnreachable(providerId: string) {
    // Do not downgrade a known not_found (provider truly lacks the title).
    if (cache.value.get(providerId)?.reason === 'not_found') return
    set(providerId, { available: false, reason: 'cdn_unreachable' })
  }

  function checkExists(providerId: string): Promise<void> {
    if (cache.value.has(providerId)) return Promise.resolve()
    const existing = inflight.get(providerId)
    if (existing) return existing
    const animeForReq = animeId.value
    const p = scraperApi
      .getEpisodes(animeForReq, providerId, true)
      .then((resp: { data?: { data?: { episodes?: unknown[] } } }) => {
        if (animeForReq !== animeId.value) return // anime changed mid-flight
        const eps = resp.data?.data?.episodes ?? []
        set(providerId, eps.length > 0 ? { available: true } : { available: false, reason: 'not_found' })
      })
      .catch((err: { response?: { status?: number } }) => {
        if (animeForReq !== animeId.value) return
        if (err?.response?.status === 404) set(providerId, { available: false, reason: 'not_found' })
        // 502/other: leave unknown — a real "cdn unreachable for this anime"
        // is recorded at playback time via markCdnUnreachable, not here.
      })
      .finally(() => inflight.delete(providerId))
    inflight.set(providerId, p)
    return p
  }

  return { get, checkExists, markCdnUnreachable, reset }
}

/**
 * Pure row overlay: maps an availability verdict onto a ProviderRow so the
 * SourcePanel/ProviderChip render the right state + tooltip. Returns the row
 * unchanged when available/unknown. Extracted so it is unit-testable without a
 * full player mount.
 */
export function overlayAvailability(
  row: ProviderRow,
  av: ProviderAvailability | undefined,
  t: (k: string) => string,
): ProviderRow {
  if (!av || av.available) return row
  return av.reason === 'not_found'
    ? { def: row.def, state: 'irrelevant', reason: t('player.sources.providerLacksAnime') }
    : { def: row.def, state: 'down', reason: t('player.sources.providerCdnUnreachable') }
}
