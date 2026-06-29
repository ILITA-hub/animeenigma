import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { ProviderRow, AudioKind, TrackLang, ContentKind } from '@/types/aePlayer'
import { GROUP_LANGS, GROUP_CONTENT } from './providerGroups'

export interface RowFilter { audio: AudioKind; lang: TrackLang; content: ContentKind }

function relevant(cap: ProviderCap, f: RowFilter): boolean {
  const g = cap.group
  // 18+ sources stay visible on a hentai title regardless of audio/lang toggle.
  if (GROUP_CONTENT[g].includes('hentai') && f.content === 'hentai') return true
  if (!GROUP_CONTENT[g].includes(f.content)) return false
  if (f.audio === 'dub') {
    return cap.audios.includes('dub') && GROUP_LANGS[g].includes(f.lang)
  }
  // RAW (audio === 'sub'): original voices — any language group, sub OR raw caps.
  // The language slider is hidden under RAW, so the lang filter is intentionally
  // dropped: EN-sub, RU-sub and pure-JP sources all surface in one list.
  return cap.audios.includes('sub') || cap.audios.includes('raw')
}

function toRow(cap: ProviderCap): ProviderRow {
  // 'raw' caps are original-audio → surface as a 'sub' (RAW) row so a raw-only
  // provider is selectable and contributes a sub combo to availability.
  const audios = [...new Set(cap.audios.map((a) => (a === 'dub' ? 'dub' : 'sub')))]
    .filter((a): a is AudioKind => a === 'sub' || a === 'dub')
  return {
    id: cap.provider, label: cap.display_name, group: cap.group, state: cap.state,
    selectable: cap.selectable, hackerOnly: cap.hacker_only, order: cap.order,
    audios,
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
