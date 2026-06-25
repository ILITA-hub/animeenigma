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

  // ─── Phase-1 capability feed (single source of truth) ──────────────────────
  // The backend now emits a self-describing per-provider state. The player
  // renders these verbatim — there is no FE-side provider registry. Disabled
  // providers are OMITTED from the feed entirely, so anything present here is a
  // real, backend-sanctioned source.
  /** Render/selection state derived backend-side from (policy, health, content). */
  state: 'active' | 'recovering' | 'degraded' | 'no_content'
  /** Whether a user can pick this row (degraded/recovering are hacker-mode-only). */
  selectable: boolean
  /** When true, the row only appears/selects in hacker mode. */
  hacker_only: boolean
  /** Backend ordering weight — higher wins (smart default + panel sort). */
  order: number
  /** Lang/content family the provider belongs to (drives the relevance filter). */
  group: 'en' | 'ru' | 'adult' | 'jp' | 'firstparty'
  /** Audio kinds this provider can serve for the current title. */
  audios: ('sub' | 'dub' | 'raw')[]
  /** Human-readable explanation for a non-active state (tooltip text). */
  reason?: string

  // ─── Decoration / variant labels (still consumed by deriveCapLabels) ────────
  variants: CapVariant[]

  // ─── Legacy fields (pre-Phase-1; kept optional for back-compat) ─────────────
  enabled?: boolean
  degraded?: boolean
  health?: 'up' | 'down' | 'unknown'
  playable?: boolean
  rank?: number
}

export interface CapVariant {
  category: 'sub' | 'dub' | 'raw'
  team?: { id?: string; name: string }
  sub_delivery: 'soft' | 'hard' | 'none'
  qualities?: string[]
  quality_source: 'hls_master' | 'discrete' | 'unknown' | 'trait'
  source: 'trait' | 'discovered'
}
