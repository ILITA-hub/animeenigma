import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { ProviderRow, AudioKind, TrackLang, ContentKind } from '@/types/aePlayer'
import { GROUP_CONTENT, langsForCap } from './providerGroups'

export interface RowFilter { audio: AudioKind; lang: TrackLang; content: ContentKind }

function relevant(cap: ProviderCap, f: RowFilter): boolean {
  const g = cap.group
  // 18+ sources stay visible on a hentai title regardless of audio/lang toggle.
  if (GROUP_CONTENT[g].includes('hentai') && f.content === 'hentai') return true
  if (!GROUP_CONTENT[g].includes(f.content)) return false
  if (f.audio === 'dub') {
    // A cap's real per-title `lang` (Phase C source-panel truth — set only for
    // ae's probed dub) overrides the group's default language set, so an ae
    // English dub matches DUB+EN only, not every language `firstparty`
    // nominally serves.
    return cap.audios.includes('dub') && langsForCap(cap).includes(f.lang)
  }
  // RAW (audio === 'sub'): original voices — any language group, sub caps.
  // The language slider is hidden under RAW, so the lang filter is intentionally
  // dropped: EN-sub, RU-sub and pure-JP sources all surface in one list.
  return cap.audios.includes('sub')
}

function toRow(cap: ProviderCap): ProviderRow {
  // The audios map yields only 'sub'|'dub', so no post-filter is needed.
  const audios = [...new Set(cap.audios.map((a): AudioKind => (a === 'dub' ? 'dub' : 'sub')))]
  return {
    id: cap.provider, label: cap.display_name, group: cap.group, state: cap.state,
    selectable: cap.selectable, hackerOnly: cap.hacker_only, order: cap.order,
    audios,
    lang: cap.lang,
    reason: cap.reason,
  }
}

/** Flatten the capability report into rendered rows, relevance-filtered and
 *  sorted by backend `order` (desc). The backend already omitted disabled
 *  providers, so anything here is a real, backend-sanctioned source. A
 *  null/malformed report yields an empty list. */
export function rowsFromReport(report: CapabilityReport | null, filter: RowFilter): ProviderRow[] {
  const rows: ProviderRow[] = []
  if (!report || !Array.isArray(report.families)) return rows
  for (const fam of report.families) {
    for (const cap of fam.providers ?? []) {
      if (relevant(cap, filter)) rows.push(toRow(cap))
    }
  }
  return rows.sort((a, b) => b.order - a.order)
}

/** Provider id → its backend `group` ('en' | 'ru' | 'adult' | 'firstparty'), read
 *  straight from the capability feed. Single source of truth for "which language
 *  facet / backend serves this provider" — the FE no longer reads the coarse
 *  `family` label. */
export function groupOfProvider(report: CapabilityReport | null, providerId: string): string | undefined {
  if (!report || !Array.isArray(report.families)) return undefined
  for (const fam of report.families) {
    for (const cap of fam.providers ?? []) {
      if (cap.provider === providerId) return cap.group
    }
  }
  return undefined
}
