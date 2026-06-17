import type { ProviderRow } from '@/types/unifiedPlayer'

export interface SmartDefaultOpts {
  /** Provider ids that must pass `isAvailable` before they can be picked (e.g. 'ae'). */
  needsCheck: Set<string>
  /** Async availability probe for gated providers. Resolves true ⇒ pickable. */
  isAvailable: (id: string) => Promise<boolean>
  /**
   * Optional synchronous playability gate. Return false to exclude a provider
   * the server's capability stats marked unplayable (e.g. allanime with
   * `playable:false`) — it stays manually selectable, just never auto-defaulted.
   * Providers absent from the capability report (ae/raw/kodik) return true.
   */
  isPlayable?: (id: string) => boolean
}

/**
 * Choose the default provider id from live rows.
 *
 * Walks `curated` order, considering only providers whose row state is
 * 'active'. For an id in `opts.needsCheck`, awaits `opts.isAvailable(id)` and
 * skips it when false (this is how an empty first-party `ae` auto-drops).
 * Providers active but absent from `curated` are tried last, in row order.
 * Returns null when nothing is selectable.
 */
export async function pickSmartDefault(
  rows: ProviderRow[],
  curated: string[],
  opts: SmartDefaultOpts,
): Promise<string | null> {
  const activeIds = new Set(rows.filter(r => r.state === 'active').map(r => r.def.id))

  // Dedup while preserving first-seen order: callers may now pass
  // [...rankingOrder, ...CURATED_TIER], which can repeat ids. Curated/ranked
  // ids (that are active) come first in the given order, then any remaining
  // active rows absent from `curated`, in row order.
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const id of curated) {
    if (activeIds.has(id) && !seen.has(id)) { seen.add(id); ordered.push(id) }
  }
  for (const r of rows) {
    if (r.state === 'active' && !seen.has(r.def.id)) { seen.add(r.def.id); ordered.push(r.def.id) }
  }

  for (const id of ordered) {
    if (opts.isPlayable && !opts.isPlayable(id)) continue
    if (opts.needsCheck.has(id)) {
      if (await opts.isAvailable(id)) return id
      continue
    }
    return id
  }
  return null
}
