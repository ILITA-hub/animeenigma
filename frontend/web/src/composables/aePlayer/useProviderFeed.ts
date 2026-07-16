import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { ProviderRow, AudioKind, TrackLang, ContentKind } from '@/types/aePlayer'
import type { ProviderVerify, VerifyReport } from '@/types/contentVerify'
import { GROUP_CONTENT, langsForCap } from './providerGroups'
import { effectiveAudios, verifiedDubLangs, verifyFor } from './verifiedCaps'

export interface RowFilter { audio: AudioKind; lang: TrackLang; content: ContentKind }

function relevant(cap: ProviderCap, f: RowFilter, v: ProviderVerify | null): boolean {
  const g = cap.group
  // 18+ sources stay visible on a hentai title regardless of audio/lang toggle.
  if (GROUP_CONTENT[g].includes('hentai') && f.content === 'hentai') return true
  if (!GROUP_CONTENT[g].includes(f.content)) return false
  // Owner-approved verified gate (spec §5): an unverified non-firstparty cap is
  // assumed RAW only; the DUB facet lists only providers with a verified dub
  // language for the current title. `effectiveAudios`/`verifiedDubLangs`
  // encode the firstparty exemption (trusts cap.audios/cap.lang as-is).
  const audios = effectiveAudios(cap, v)
  if (f.audio === 'dub') {
    // A cap's real per-title `lang` (Phase C source-panel truth — set only for
    // ae's probed dub) overrides the group's default language set, so an ae
    // English dub matches DUB+EN only, not every language `firstparty`
    // nominally serves. Non-firstparty dub language comes from the verified
    // dub_langs — no verified lang, no DUB row.
    const langs = cap.group === 'firstparty' ? langsForCap(cap) : verifiedDubLangs(v)
    return audios.includes('dub') && langs.includes(f.lang)
  }
  // RAW (audio === 'sub'): original voices — any language group, sub caps.
  // The language slider is hidden under RAW, so the lang filter is intentionally
  // dropped: EN-sub, RU-sub and pure-JP sources all surface in one list.
  return audios.includes('sub')
}

function toRow(cap: ProviderCap, v: ProviderVerify | null): ProviderRow {
  return {
    id: cap.provider, label: cap.display_name, group: cap.group, state: cap.state,
    selectable: cap.selectable, hackerOnly: cap.hacker_only, order: cap.order,
    audios: effectiveAudios(cap, v),
    lang: cap.lang,
    reason: cap.reason,
    playability_index: cap.playability_index,
    partialLibrary: cap.partial_library,
    verify: v,
  }
}

/** Flatten the capability report into rendered rows, relevance-filtered and
 *  sorted by backend `order` (desc). The backend already omitted disabled
 *  providers, so anything here is a real, backend-sanctioned source. A
 *  null/malformed report yields an empty list. `verify` (content-verify probe
 *  report, Task 13) gates which audios each non-firstparty row may claim —
 *  see `verifiedCaps.ts` for the exact semantics. */
export function rowsFromReport(
  report: CapabilityReport | null,
  filter: RowFilter,
  verify: VerifyReport | null = null,
): ProviderRow[] {
  const rows: ProviderRow[] = []
  if (!report || !Array.isArray(report.families)) return rows
  for (const fam of report.families) {
    for (const cap of fam.providers ?? []) {
      const v = verifyFor(verify, cap.provider)
      if (relevant(cap, filter, v)) rows.push(toRow(cap, v))
    }
  }
  return rows.sort((a, b) => b.order - a.order)
}

/** Find a provider's cap anywhere in the report (shared scan for the
 *  per-field getters below). */
function capOfProvider(report: CapabilityReport | null, providerId: string): ProviderCap | undefined {
  if (!report || !Array.isArray(report.families)) return undefined
  for (const fam of report.families) {
    for (const cap of fam.providers ?? []) {
      if (cap.provider === providerId) return cap
    }
  }
  return undefined
}

/** Provider id → its backend `group` ('en' | 'ru' | 'adult' | 'firstparty'), read
 *  straight from the capability feed. Single source of truth for "which language
 *  facet / backend serves this provider" — the FE no longer reads the coarse
 *  `family` label. */
export function groupOfProvider(report: CapabilityReport | null, providerId: string): string | undefined {
  return capOfProvider(report, providerId)?.group
}

/** The roster player_key for a provider id, from the capability report (AUTO-608). */
export function playerKeyOfProvider(report: CapabilityReport | null, providerId: string): string | undefined {
  return capOfProvider(report, providerId)?.player_key
}
