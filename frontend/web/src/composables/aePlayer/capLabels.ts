import type { ProviderCap } from '@/types/capabilities'
import type { ProviderVerify } from '@/types/contentVerify'
import type { TrackLang } from '@/types/aePlayer'
import { isUnverified, verifiedDubLangs, verifiedHardsubLangs } from './verifiedCaps'

export interface CapLabels {
  categories: ('sub' | 'dub')[]
  quality: string | null // max quality ceiling across variants, e.g. '1080p'
  subDelivery: 'soft' | 'hard' | null // delivery of the SUB variant, if any
  /** True when this (non-firstparty) provider has no verified probe verdict —
   *  its old asserted SUB/DUB category chips are suppressed in favour of the
   *  "unverified" marker (owner-approved gate, content-verify spec §5). */
  unverified: boolean
  /** Dub languages the content-verify probe actually confirmed for this
   *  provider — rendered as "DUB {lang} ✓" badges. */
  verifiedDub: TrackLang[]
  /** Hardsub languages the probe confirmed — rendered as "Burned-in ({lang})". */
  verifiedHardsub: TrackLang[]
}

// Highest → lowest. Index 0 is the best resolution.
const Q_ORDER = ['2160p', '1440p', '1080p', '720p', '480p', '360p']

/**
 * Derive the compact chip labels from a provider's capability variants:
 * the category set (SUB/DUB), the best quality ceiling, and the SUB
 * variant's subtitle delivery (soft=selectable / hard=burned-in). Returns null
 * when there's nothing to show.
 *
 * `verify` (content-verify probe summary, Task 13/14) additionally derives
 * `unverified`/`verifiedDub`/`verifiedHardsub`, and — once the probe has an
 * actual verdict (status !== 'unverified') — overrides `categories` with what
 * was proven instead of the variants' asserted claims (owner-approved gate:
 * no claims without verification). `firstparty` (ae) is exempt from this
 * override even though it CAN carry a verify row (the backend worker
 * synthesizes ae's verdict from library truth, not a probe) — firstparty
 * always trusts its own variants, per the same exemption `isUnverified`/
 * `effectiveAudios` apply.
 */
export function deriveCapLabels(
  cap: ProviderCap | undefined,
  verify: ProviderVerify | null = null,
): CapLabels | null {
  if (!cap || !Array.isArray(cap.variants) || cap.variants.length === 0) return null

  const unverified = isUnverified(cap, verify)
  const verifiedDub = verifiedDubLangs(verify)
  const verifiedHardsub = verifiedHardsubLangs(verify)

  let categories: ('sub' | 'dub')[]
  if (cap.group !== 'firstparty' && verify && verify.status !== 'unverified') {
    categories = []
    if (verify.raw || verify.status === 'partial') categories.push('sub')
    if (verifiedDub.length > 0) categories.push('dub')
  } else {
    categories = []
    for (const c of ['sub', 'dub'] as const) {
      if (cap.variants.some((v) => v.category === c)) categories.push(c)
    }
  }

  let quality: string | null = null
  let bestIdx = Number.MAX_SAFE_INTEGER
  for (const v of cap.variants) {
    for (const q of v.qualities ?? []) {
      const idx = Q_ORDER.indexOf(q)
      if (idx !== -1 && idx < bestIdx) {
        bestIdx = idx
        quality = q
      }
    }
  }

  const sub = cap.variants.find((v) => v.category === 'sub')
  const subDelivery =
    sub && (sub.sub_delivery === 'soft' || sub.sub_delivery === 'hard') ? sub.sub_delivery : null

  return { categories, quality, subDelivery, unverified, verifiedDub, verifiedHardsub }
}
