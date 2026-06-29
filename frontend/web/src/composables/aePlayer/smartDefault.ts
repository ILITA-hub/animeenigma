import type { ProviderRow, TrackLang } from '@/types/aePlayer'
import { GROUP_LANGS } from './providerGroups'

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

/** RAW (original-audio) default: prefer the best active source whose group
 *  serves the requested language (don't yank a RU-sub watcher onto an EN
 *  source), then fall back to the global best active source. Honors the
 *  "never cross language" rule while the language slider stays DUB-only. */
export function pickRawBiased(rows: ProviderRow[], lang: TrackLang): ProviderRow | null {
  const inLang = rows.filter(r => GROUP_LANGS[r.group].includes(lang))
  return pickSmartDefault(inLang) ?? pickSmartDefault(rows)
}

/** Dead-player guard: when no source is `active`, take the top-ranked
 *  SELECTABLE row (degraded/recovering included) so the player still attempts
 *  playback instead of showing a dead "no source" panel. */
export function pickSelectableFallback(rows: ProviderRow[]): ProviderRow | null {
  return [...rows].filter(r => r.selectable).sort((a, b) => b.order - a.order)[0] ?? null
}
