import type { ProviderRow } from '@/types/aePlayer'

/** Pick the default provider: the highest-`order` row that is actually
 *  selectable and `active`. Rows arrive pre-sorted by order, but sort
 *  defensively. Returns null when nothing is active (caller shows the
 *  empty/error state).
 *
 *  The backend feed already carries everything needed: `ae` with no local copy
 *  arrives as `state:'no_content'` (filtered out here), degraded/recovering
 *  providers are `selectable:false` outside hacker mode, and disabled providers
 *  never reach the FE at all. So a plain sync filter over the rows is the whole
 *  smart-default policy — no registry, no async availability probe. */
export function pickSmartDefault(rows: ProviderRow[]): ProviderRow | null {
  return [...rows]
    .filter(r => r.state === 'active' && r.selectable)
    .sort((a, b) => b.order - a.order)[0] ?? null
}
