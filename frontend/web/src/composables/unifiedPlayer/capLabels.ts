import type { ProviderCap } from '@/types/capabilities'

export interface CapLabels {
  categories: ('sub' | 'dub' | 'raw')[]
  quality: string | null // max quality ceiling across variants, e.g. '1080p'
  subDelivery: 'soft' | 'hard' | null // delivery of the SUB variant, if any
}

// Highest → lowest. Index 0 is the best resolution.
const Q_ORDER = ['2160p', '1440p', '1080p', '720p', '480p', '360p']

/**
 * Derive the compact chip labels from a provider's capability variants:
 * the category set (SUB/DUB/RAW), the best quality ceiling, and the SUB
 * variant's subtitle delivery (soft=selectable / hard=burned-in). Returns null
 * when there's nothing to show.
 */
export function deriveCapLabels(cap: ProviderCap | undefined): CapLabels | null {
  if (!cap || !Array.isArray(cap.variants) || cap.variants.length === 0) return null

  const categories: ('sub' | 'dub' | 'raw')[] = []
  for (const c of ['sub', 'dub', 'raw'] as const) {
    if (cap.variants.some((v) => v.category === c)) categories.push(c)
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

  return { categories, quality, subDelivery }
}
