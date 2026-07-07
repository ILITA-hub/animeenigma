import type { ProviderRow, TrackLang } from '@/types/aePlayer'
import { langsForCap } from './providerGroups'

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
 *  "never cross language" rule while the language slider stays DUB-only.
 *  Uses `langsForCap` so a row with a real per-title `lang` (ae's probed
 *  dub) is matched on that language, not its group's full nominal set. */
export function pickRawBiased(rows: ProviderRow[], lang: TrackLang): ProviderRow | null {
  const inLang = rows.filter(r => langsForCap(r).includes(lang))
  return pickSmartDefault(inLang) ?? pickSmartDefault(rows)
}

/** Dead-player guard: when no source is `active`, take the top-ranked
 *  SELECTABLE row (degraded/recovering included) so the player still attempts
 *  playback instead of showing a dead "no source" panel. */
export function pickSelectableFallback(rows: ProviderRow[]): ProviderRow | null {
  return [...rows].filter(r => r.selectable).sort((a, b) => b.order - a.order)[0] ?? null
}

/** Candidate pool for the SMART DEFAULT (not a manual pick).
 *
 *  The first-party `ae` library is auto-cached and can be PARTIAL — sometimes
 *  only a late episode (e.g. Frieren ep 27 of 28) — yet it ranks highest
 *  (order 100), so it would win every default and open its lone late file
 *  instead of episode 1. On a fresh open with no requested episode we drop a
 *  PARTIAL ae (`partialLibrary`, backend-flagged when the library lacks ep 1)
 *  so the player lands on episode 1 of a full source — BUT only while a real
 *  alternative remains (an active, selectable non-firstparty source); if the
 *  partial ae is the only thing that can play, we keep it rather than dead-end.
 *  A COMPLETE ae library (covers ep 1) is never dropped and stays the preferred
 *  default. A specified episode (resume / deep-link) leaves even a partial ae
 *  eligible — the per-episode failover handles a miss — and ae is ALWAYS still
 *  manually selectable in the Source panel. */
export function defaultPool(rows: ProviderRow[], episodeSpecified: boolean): ProviderRow[] {
  if (episodeSpecified) return rows
  const isPartialAe = (r: ProviderRow) => r.group === 'firstparty' && r.partialLibrary === true
  const withoutPartialAe = rows.filter(r => !isPartialAe(r))
  if (withoutPartialAe.length === rows.length) return rows // no partial ae to drop
  const hasActiveAlternative = withoutPartialAe.some(r => r.state === 'active' && r.selectable)
  return hasActiveAlternative ? withoutPartialAe : rows
}
