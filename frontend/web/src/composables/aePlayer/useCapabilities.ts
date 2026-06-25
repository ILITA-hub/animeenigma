import { ref, watch, type Ref } from 'vue'
import { capabilitiesApi } from '@/api/client'
import type { CapabilityReport, ProviderCap } from '@/types/capabilities'

/**
 * Flatten a capability report into a providerId→ProviderCap map (for chip
 * labels) plus a best-first ranked id list (rank desc, name tiebreak — mirrors
 * the backend stable sort). Pure + defensive: a null/malformed report yields
 * empties.
 */
export function flattenCapabilities(report: CapabilityReport | null): {
  capMap: Map<string, ProviderCap>
  rankedIds: string[]
} {
  const capMap = new Map<string, ProviderCap>()
  if (!report || !Array.isArray(report.families)) return { capMap, rankedIds: [] }
  for (const fam of report.families) {
    for (const p of fam.providers ?? []) capMap.set(p.provider, p)
  }
  const rankedIds = [...capMap.values()]
    .sort((a, b) => b.rank - a.rank || a.provider.localeCompare(b.provider))
    .map((p) => p.provider)
  return { capMap, rankedIds }
}

/**
 * Fetch the capability report for an anime id (re-fetches when the id changes).
 * Decoration + ordering signal only — every failure degrades to empty, never
 * throws.
 */
export function useCapabilities(animeId: Ref<string>) {
  const report = ref<CapabilityReport | null>(null)
  const capMap = ref<Map<string, ProviderCap>>(new Map())
  const rankedIds = ref<string[]>([])
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
      const flat = flattenCapabilities(rep)
      report.value = rep
      capMap.value = flat.capMap
      rankedIds.value = flat.rankedIds
    } catch {
      report.value = null
      capMap.value = new Map()
      rankedIds.value = []
      error.value = true
    } finally {
      loaded.value = true
    }
  }

  watch(animeId, (id) => { void load(id) }, { immediate: true })

  return { report, capMap, rankedIds, loaded, error }
}
