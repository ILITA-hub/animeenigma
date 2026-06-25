import type { CapabilityReport, ProviderCap } from '@/types/capabilities'
import type { ProviderRow, AudioKind, TrackLang, ContentKind, ProviderGroup } from '@/types/aePlayer'

export interface RowFilter { audio: AudioKind; lang: TrackLang; content: ContentKind }

// Backend groups → which (lang, content) they serve. Group is the wire group
// from the DB; relevance still filters by the active toggle so the menu shows
// only providers that can serve the current combo.
const GROUP_LANGS: Record<ProviderGroup, TrackLang[]> = {
  en: ['en'], ru: ['ru'], adult: ['en', 'ru'], jp: ['ja'], firstparty: ['en', 'ru', 'ja'],
}
const GROUP_CONTENT: Record<ProviderGroup, ContentKind[]> = {
  en: ['common'], ru: ['common'], adult: ['hentai'], jp: ['common'], firstparty: ['common'],
}

function relevant(cap: ProviderCap, f: RowFilter): boolean {
  const g = cap.group
  // 18+ sources stay visible on a hentai title regardless of audio/lang toggle.
  if (GROUP_CONTENT[g].includes('hentai') && f.content === 'hentai') return true
  return cap.audios.includes(f.audio) && GROUP_LANGS[g].includes(f.lang) && GROUP_CONTENT[g].includes(f.content)
}

function toRow(cap: ProviderCap): ProviderRow {
  return {
    id: cap.provider, label: cap.display_name, group: cap.group, state: cap.state,
    selectable: cap.selectable, hackerOnly: cap.hacker_only, order: cap.order,
    audios: cap.audios.filter((a): a is AudioKind => a === 'sub' || a === 'dub'),
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
