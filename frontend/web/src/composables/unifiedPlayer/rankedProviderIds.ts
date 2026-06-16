import type { ProviderRow } from '@/types/unifiedPlayer'

/**
 * Merge the server capability ranking with the registry rows into one ordered
 * id list for the smart default and the panel sort:
 *   1. capability-ranked ids that exist as rows (best first),
 *   2. rows absent from the ranking (ae/raw/18anime…), in `curated` order,
 *      with unknown ids alphabetised last.
 */
export function rankedProviderIds(
  rows: ProviderRow[],
  rankedIds: string[],
  curated: string[],
): string[] {
  const rowIds = new Set(rows.map((r) => r.def.id))
  const out: string[] = []
  const seen = new Set<string>()
  for (const id of rankedIds) {
    if (rowIds.has(id) && !seen.has(id)) {
      out.push(id)
      seen.add(id)
    }
  }
  const curatedPos = (id: string) => {
    const i = curated.indexOf(id)
    return i === -1 ? Number.MAX_SAFE_INTEGER : i
  }
  const remaining = [...rowIds].filter((id) => !seen.has(id))
  remaining.sort((a, b) => curatedPos(a) - curatedPos(b) || a.localeCompare(b))
  out.push(...remaining)
  return out
}
