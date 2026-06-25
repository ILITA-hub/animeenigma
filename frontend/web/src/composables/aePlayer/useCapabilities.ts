import { ref, watch, type Ref } from 'vue'
import { capabilitiesApi } from '@/api/client'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'

/**
 * Flatten a capability report into a providerId→ProviderCap map (for chip
 * labels). Pure + defensive: a null/malformed report yields an empty map.
 * Ordering is backend-authoritative (the `order` field), so the FE keeps no
 * rank list of its own.
 */
export function flattenCapabilities(report: CapabilityReport | null): Map<string, ProviderCap> {
  const capMap = new Map<string, ProviderCap>()
  if (!report || !Array.isArray(report.families)) return capMap
  for (const fam of report.families) {
    for (const p of fam.providers ?? []) capMap.set(p.provider, p)
  }
  return capMap
}

/**
 * Fetch the capability report for an anime id (re-fetches when the id changes).
 * Decoration + ordering signal only — every failure degrades to empty, never
 * throws.
 */
export function useCapabilities(animeId: Ref<string>) {
  const report = ref<CapabilityReport | null>(null)
  const capMap = ref<Map<string, ProviderCap>>(new Map())
  const loaded = ref(false)
  const error = ref(false)

  let lastId = ''
  async function load(id: string) {
    if (!id || id === lastId) return
    lastId = id
    loaded.value = false
    error.value = false
    try {
      const res = await capabilitiesApi.get(id)
      // catalog {success,data} envelope
      const rep = (res.data?.data ?? res.data ?? null) as CapabilityReport | null
      report.value = rep
      capMap.value = flattenCapabilities(rep)
    } catch {
      report.value = null
      capMap.value = new Map()
      error.value = true
    } finally {
      loaded.value = true
    }
  }

  watch(animeId, (id) => { void load(id) }, { immediate: true })

  return { report, capMap, loaded, error }
}
