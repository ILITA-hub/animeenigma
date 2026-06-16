import type { ProviderRow } from '@/types/unifiedPlayer'

export interface SmartDefaultOpts {
  /** Provider ids that must pass `isAvailable` before they can be picked (e.g. 'ae'). */
  needsCheck: Set<string>
  /** Async availability probe for gated providers. Resolves true ⇒ pickable. */
  isAvailable: (id: string) => Promise<boolean>
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

  const ordered = [
    ...curated.filter(id => activeIds.has(id)),
    ...rows.filter(r => r.state === 'active' && !curated.includes(r.def.id)).map(r => r.def.id),
  ]

  for (const id of ordered) {
    if (opts.needsCheck.has(id)) {
      if (await opts.isAvailable(id)) return id
      continue
    }
    return id
  }
  return null
}
