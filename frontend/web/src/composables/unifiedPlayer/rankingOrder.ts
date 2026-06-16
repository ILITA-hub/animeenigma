import type { SourceRanking } from '@/types/sourceRanking'

/**
 * Flattens a SourceRanking into a deduped, best-first provider-id order:
 * fix (same-day override) → per-anime ranking → global ranking. Records are
 * assumed already sorted best-first by the backend. The result is meant to be
 * PREPENDED to CURATED_TIER and passed to pickSmartDefault (which filters to
 * active rows + applies the availability probe). First occurrence wins.
 */
export function rankingToOrder(r: SourceRanking | null | undefined): string[] {
  if (!r) return []
  const out: string[] = []
  const seen = new Set<string>()
  const push = (id: string) => {
    if (id && !seen.has(id)) {
      seen.add(id)
      out.push(id)
    }
  }
  push(r.fix)
  for (const rec of r.perAnime ?? []) push(rec.provider)
  for (const rec of r.global ?? []) push(rec.provider)
  return out
}
