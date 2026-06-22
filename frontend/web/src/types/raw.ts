/**
 * Type definitions for the workstream raw-jp video provider.
 * Workstream raw-jp, Phase 03.
 */

export interface RawEpisode {
  id: string
  number: number
  title: string
}

export interface RawSubtitle {
  url: string
  lang: string
  label: string
}

export interface RawStream {
  url: string
  type: 'hls' | 'mp4'
  quality?: string
  subtitles?: RawSubtitle[]
  expires_at: string
}

export interface RawEpisodesResponse {
  episodes: RawEpisode[]
  available: boolean
  source: string
}

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
