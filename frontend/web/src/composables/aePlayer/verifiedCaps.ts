import type { ProviderVerify, VerifyReport } from '@/types/contentVerify'
import type { TrackLang } from '@/types/aePlayer'
import type { CapabilityReport } from '@/types/capabilities'

/**
 * Owner-approved hard gate (spec §5): a source with no probe verdict is
 * assumed RAW and marked "unverified"; the DUB facet lists only providers
 * with a verified dub language. First-party (ae) is exempt — its audio/lang
 * comes from our own library ingest and is trusted as-is.
 */
export function verifyFor(report: VerifyReport | null, provider: string): ProviderVerify | null {
  return report?.providers[provider] ?? null
}

export function isUnverified(cap: { group: string }, v: ProviderVerify | null): boolean {
  if (cap.group === 'firstparty') return false
  return !v || v.status === 'unverified'
}

export function verifiedDubLangs(v: ProviderVerify | null): TrackLang[] {
  return (v?.dub_langs ?? []).filter((l): l is TrackLang => l === 'en' || l === 'ru' || l === 'ja')
}

export function verifiedHardsubLangs(v: ProviderVerify | null): TrackLang[] {
  return (v?.hardsub_langs ?? []).filter((l): l is TrackLang => l === 'en' || l === 'ru' || l === 'ja')
}

/**
 * Seeds a VerifyReport straight from the capability feed's own blended
 * `verify` summaries (ProviderCap.verify — services/content-verify's rollup,
 * forwarded through the catalog's /capabilities wire), for the very first
 * render, before useContentVerify's own poll has resolved. Units are always
 * empty — per-unit detail (hacker-mode HUD, download-dialog unit picking)
 * only ever comes from the dynamic /content-verify poll itself.
 *
 * Returns null when there's no report yet, or no provider row carries a
 * verify blend — nothing to seed with, so callers fall through to treating
 * every non-firstparty row as unverified (unchanged pre-fix behavior).
 */
export function seedVerifyFromReport(report: CapabilityReport | null): VerifyReport | null {
  if (!report) return null
  const providers: Record<string, ProviderVerify> = {}
  for (const family of report.families) {
    for (const cap of family.providers) {
      if (!cap.verify) continue
      providers[cap.provider] = { ...cap.verify, units: [] }
    }
  }
  return Object.keys(providers).length ? { animeId: report.anime_id, providers } : null
}

export function effectiveAudios(
  cap: { group: string; audios: ('sub' | 'dub')[] },
  v: ProviderVerify | null,
): ('sub' | 'dub')[] {
  if (cap.group === 'firstparty') {
    return [...new Set(cap.audios.map((a) => (a === 'dub' ? 'dub' : 'sub')))]
  }
  if (!v || v.status === 'unverified') return ['sub']
  const out = new Set<'sub' | 'dub'>()
  if (v.raw) out.add('sub')
  if (verifiedDubLangs(v).length) out.add('dub')
  if (v.status === 'partial') out.add('sub') // unverified units stay RAW-assumed
  return out.size ? [...out] : ['sub']
}
