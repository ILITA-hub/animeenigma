import type { ProviderVerify, VerifyReport } from '@/types/contentVerify'
import type { TrackLang } from '@/types/aePlayer'

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
