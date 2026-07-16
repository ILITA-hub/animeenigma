import { getCurrentInstance, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { contentVerifyApi } from '@/api/client'
import type { ProviderVerify, VerifyReport, VerifyUnit } from '@/types/contentVerify'

interface WireProvider {
  provider: string
  summary: Omit<ProviderVerify, 'units'>
  units?: VerifyUnit[]
}

/**
 * Maps the catalog content-verify wire shape ({anime_id, providers: [{provider,
 * summary, units}]}) into the FE's Record-keyed VerifyReport. Pure + defensive:
 * a null/malformed payload (missing anime_id or a non-array providers list)
 * yields null so callers can leave the previous report untouched.
 */
export function normalizeVerify(raw: unknown): VerifyReport | null {
  const r = raw as { anime_id?: string; providers?: WireProvider[] } | null
  if (!r || typeof r.anime_id !== 'string' || !Array.isArray(r.providers)) return null
  const providers: Record<string, ProviderVerify> = {}
  for (const p of r.providers) {
    if (!p?.provider || !p.summary) continue
    providers[p.provider] = { ...p.summary, units: p.units ?? [] }
  }
  return { animeId: r.anime_id, providers }
}

/**
 * Dynamic content-verify feed: fetch on activation, re-poll every pollMs while
 * `active` (player open, playback not started) and the page is visible. When
 * `active` flips false the poll stops but the last report stays — badges keep
 * rendering, only combo correction is off the table.
 */
export function useContentVerify(animeId: Ref<string>, active: Ref<boolean>, pollMs = 45000) {
  const report = ref<VerifyReport | null>(null)
  let timer: ReturnType<typeof setInterval> | null = null

  async function refresh() {
    if (!animeId.value || (typeof document !== 'undefined' && document.hidden)) return
    try {
      const res = await contentVerifyApi.get(animeId.value)
      const normalized = normalizeVerify(res.data?.data ?? res.data)
      if (normalized) report.value = normalized
    } catch {
      /* best-effort — absent report just means "all unverified" */
    }
  }

  function stop() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  function start() {
    stop()
    if (!active.value || !animeId.value) return
    void refresh()
    timer = setInterval(() => {
      if (active.value) void refresh()
      else stop()
    }, pollMs)
  }

  watch([animeId, active], ([, isActive]) => {
    if (isActive) start()
    else stop()
  }, { immediate: true })

  if (getCurrentInstance()) onBeforeUnmount(stop)

  return { report, refresh }
}
