import type { ProviderCap } from '@/types/capabilities'
import type { ProviderVerify } from '@/types/contentVerify'
import type { TrackLang } from '@/types/aePlayer'
import { isUnverified, verifiedDubLangs, verifiedHardsubLangs } from './verifiedCaps'

/**
 * One consolidated stream badge (owner taxonomy 2026-07-17):
 *  - `sub`  = the ORIGINAL-audio option; `langs` are its burned-in subtitle
 *    languages → renders as "SUB BURNED-IN RU". Soft delivery renders as
 *    "SUB · selectable".
 *  - `dub`  = a re-voiced option; `langs` are the verified dub languages →
 *    renders as "DUB RU". (DUB JA is legal — e.g. a JP re-dub of a donghua.)
 * One badge per kind — never parallel per-fact badges (the old model stacked
 * "SUB · burned-in" + "DUB" + "DUB RU ✓" + "Burned-in (EN)" + "Burned-in (RU)"
 * into five chips for one provider).
 */
export interface StreamBadge {
  kind: 'sub' | 'dub'
  langs: TrackLang[]
  burnedIn: boolean
  soft: boolean
}

export interface CapLabels {
  badges: StreamBadge[]
  quality: string | null // max quality ceiling across variants, e.g. '1080p'
  /** True when this (non-firstparty) provider has no verified probe verdict —
   *  badges are suppressed in favour of the "unverified" marker
   *  (owner-approved gate, content-verify spec §5). */
  unverified: boolean
}

// Highest → lowest. Index 0 is the best resolution.
const Q_ORDER = ['2160p', '1440p', '1080p', '720p', '480p', '360p']

/**
 * Derive the consolidated chip labels from a provider's capability variants
 * blended with its content-verify verdict: at most one SUB badge (original
 * audio, carrying its burned-in sub langs) + one DUB badge (verified dub
 * langs) + the quality ceiling. Returns null when there's nothing to show.
 *
 * Non-firstparty badges only ever state what the probe PROVED (owner gate:
 * no claims without verification); `firstparty` (ae) trusts its own variants
 * — its verify row is synthesized from library truth, not a probe.
 */
export function deriveCapLabels(
  cap: ProviderCap | undefined,
  verify: ProviderVerify | null = null,
): CapLabels | null {
  if (!cap || !Array.isArray(cap.variants) || cap.variants.length === 0) return null

  const unverified = isUnverified(cap, verify)

  const sub = cap.variants.find((v) => v.category === 'sub')
  const subDelivery =
    sub && (sub.sub_delivery === 'soft' || sub.sub_delivery === 'hard') ? sub.sub_delivery : null

  const badges: StreamBadge[] = []
  if (cap.group === 'firstparty') {
    for (const kind of ['sub', 'dub'] as const) {
      if (cap.variants.some((v) => v.category === kind)) {
        badges.push({
          kind,
          langs: [],
          burnedIn: kind === 'sub' && subDelivery === 'hard',
          soft: kind === 'sub' && subDelivery === 'soft',
        })
      }
    }
  } else if (!unverified && verify) {
    const hardsub = verifiedHardsubLangs(verify)
    if (verify.raw || verify.status === 'partial') {
      badges.push({
        kind: 'sub',
        langs: hardsub,
        burnedIn: hardsub.length > 0 || subDelivery === 'hard',
        soft: hardsub.length === 0 && subDelivery === 'soft',
      })
    }
    const dub = verifiedDubLangs(verify)
    if (dub.length > 0) {
      badges.push({ kind: 'dub', langs: dub, burnedIn: false, soft: false })
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

  return { badges, quality, unverified }
}
