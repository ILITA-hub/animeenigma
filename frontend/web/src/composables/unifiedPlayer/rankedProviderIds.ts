import type { ProviderRow } from '@/types/unifiedPlayer'

/**
 * Merge the server capability ranking with the registry rows into one ordered
 * id list for the smart default and the panel sort:
 *   1. first-party rows (e.g. `ae`) — first-party content is preferred over any
 *      scraped source, so it LEADS. The async availability probe in
 *      `pickSmartDefault` drops `ae` when the title isn't in the on-prem library,
 *      so leading here is safe (an unavailable `ae` is skipped, not picked).
 *   2. capability-ranked ids that exist as rows (best first, by third-party stats),
 *   3. remaining rows (raw/18anime…), in `curated` order, unknown ids alpha-last.
 */
export function rankedProviderIds(
  rows: ProviderRow[],
  rankedIds: string[],
  curated: string[],
): string[] {
  const rowIds = new Set(rows.map((r) => r.def.id))
  const out: string[] = []
  const seen = new Set<string>()
  const push = (id: string) => {
    if (rowIds.has(id) && !seen.has(id)) {
      out.push(id)
      seen.add(id)
    }
  }
  const curatedPos = (id: string) => {
    const i = curated.indexOf(id)
    return i === -1 ? Number.MAX_SAFE_INTEGER : i
  }

  // 1. First-party rows lead (curated order among them).
  rows
    .filter((r) => r.def.group === 'first-party')
    .map((r) => r.def.id)
    .sort((a, b) => curatedPos(a) - curatedPos(b))
    .forEach(push)

  // 2. Capability-ranked rows (best first).
  for (const id of rankedIds) push(id)

  // 3. Remaining rows, curated order then alpha.
  const remaining = [...rowIds].filter((id) => !seen.has(id))
  remaining.sort((a, b) => curatedPos(a) - curatedPos(b) || a.localeCompare(b))
  remaining.forEach(push)
  return out
}
