/**
 * Type definitions for the aggregated subtitle sources surfaced by
 * OtherSubsPanel (Jimaku, OpenSubtitles, etc.). Backed by the catalog's
 * /api/anime/{id}/subtitles[/all] endpoints.
 */

/**
 * One aggregated subtitle track returned by the catalog's
 * /api/anime/{id}/subtitles[/all] endpoints.
 */
export interface SubtitleTrack {
  url: string
  lang: string
  label: string
  format?: string
  provider: 'jimaku' | 'opensubtitles' | string
  release?: string
  // Provenance signature stamped by the catalog (streamsign) on EXTERNAL track
  // URLs (today only jimaku.cc) so the HLS proxy trusts the host without a
  // static allowlist entry. Same-origin /api/... tracks carry no signature.
  exp?: string
  sig?: string
}

export interface ProviderHealth {
  provider: string
  status: 'degraded' | 'down'
  latency_ms?: number
}

/** Subtitle response grouped by ISO 639-1 language code. */
export interface GroupedSubs {
  languages: Record<string, SubtitleTrack[]>
  episode: number
  providers_down?: string[]
  provider_health?: ProviderHealth[]
}
