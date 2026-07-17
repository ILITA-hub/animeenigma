// TS mirror of the catalog content-verify proxy response
// (GET /api/anime/{uuid}/content-verify). Snake_case keys on the wire match
// the backend probe report exactly; the FE-normalized VerifyReport folds the
// wire's `providers` array into a Record keyed by provider id for O(1) lookup
// from the Source panel / badges.

export interface VerifyUnit {
  key: { team?: string; server?: string; category?: string; track?: string }
  episode: number
  status: 'verified' | 'inconclusive' | 'unreachable'
  audio?: { lang?: string; confidence: number; verified: boolean }
  hardsub?: { present: boolean; lang?: string; confidence: number; verified: boolean }
  /** Episodes this unit had ready at enumeration time; absent = unknown. */
  episodes?: number
  probed_at?: string
}

export interface ProviderVerify {
  status: 'unverified' | 'partial' | 'verified'
  raw: boolean
  dub_langs: string[]
  hardsub_langs: string[]
  /** Provider-level episodes-ready count (max across units); absent = unknown. */
  episodes?: number
  units?: VerifyUnit[]
}

/**
 * Summary-only shape of ProviderVerify (no per-unit detail) — mirrors the Go
 * domain.VerifySummary the catalog blends into ProviderCap.verify on the
 * /capabilities wire (services/catalog/internal/domain/capability.go). The
 * dynamic /content-verify poll (useContentVerify) carries per-unit detail too;
 * this trimmed shape is what's available before that poll ever resolves.
 */
export type VerifySummary = Omit<ProviderVerify, 'units'>

export interface VerifyReport {
  animeId: string
  providers: Record<string, ProviderVerify>
}
