// TS mirror of the Go domain.CapabilityReport (services/catalog/internal/domain/capability.go).
// Snake_case keys match the JSON wire shape exactly.

export interface CapabilityReport {
  anime_id: string
  families: SourceFamily[]
}

export interface SourceFamily {
  family: string // 'ourenglish' | 'kodik' | 'animelib' | 'hanime'
  providers: ProviderCap[]
}

export interface ProviderCap {
  provider: string
  display_name: string
  enabled: boolean
  /** Soft-degraded: the player ranks it last, never auto-selects it, and only
   *  offers it (behind a "degraded" pill) when hacker mode is on (AUTO-484). */
  degraded?: boolean
  health: 'up' | 'down' | 'unknown'
  playable?: boolean
  rank: number
  variants: CapVariant[]
}

export interface CapVariant {
  category: 'sub' | 'dub' | 'raw'
  team?: { id?: string; name: string }
  sub_delivery: 'soft' | 'hard' | 'none'
  qualities?: string[]
  quality_source: 'hls_master' | 'discrete' | 'unknown' | 'trait'
  source: 'trait' | 'discovered'
}
