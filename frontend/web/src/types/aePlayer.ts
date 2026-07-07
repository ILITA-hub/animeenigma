// frontend/web/src/types/aePlayer.ts
// Single source of truth for aePlayer types. Imported by composables,
// the provider registry, and components — keep names stable across tasks.

export type AudioKind = 'sub' | 'dub'
export type TrackLang = 'en' | 'ru' | 'ja'
export type ContentKind = 'common' | 'hentai'
// Wire `group` from the backend capability feed. Single source of truth: the FE
// no longer carries a provider registry — group/state/order/audios all arrive
// from /api/anime/{id}/capabilities.
export type ProviderGroup = 'en' | 'ru' | 'adult' | 'firstparty'

// State emitted by the backend feed (derived from the live policy/health authority):
//   active     — in the auto-failover chain; selectable + auto-eligible, ranked by `order`.
//   recovering — auto policy, health healing; selectable, but the normal Source list
//                renders only `active` rows, so recovering surfaces in hacker mode only.
//   degraded   — pinned out of the auto chain (policy=manual); hacker-mode-only (hackerOnly).
//   no_content — provider has no episodes for this title (e.g. first-party `ae`
//                before encoding); tinted + never selectable.
export type ChipState = 'active' | 'recovering' | 'degraded' | 'no_content'

/** A provider as rendered in the Source panel — fields come straight from the
 *  backend capability feed (single source of truth). No FE-side registry. */
export interface ProviderRow {
  id: string
  label: string
  group: ProviderGroup
  state: ChipState
  selectable: boolean
  hackerOnly: boolean
  order: number
  audios: AudioKind[]
  /** Real per-title language override (Phase C source-panel truth — set ONLY
   *  for the first-party `ae` provider's probed dub variant). Mirrors
   *  `ProviderCap.lang`; see `langsForCap` in providerGroups.ts for how it
   *  narrows the group's default language set. */
  lang?: TrackLang
  reason?: string
}

/** The user's current source selection. */
export interface Combo {
  audio: AudioKind
  lang: TrackLang
  provider: string
  server: string
  team: string | null
}

/** A subtitle track shipped alongside a provider stream (proxied + signed). */
export interface SubtitleTrack {
  url: string       // ready-to-fetch (proxied + signed for provider tracks)
  provider: string  // 'gogoanime' | 'jimaku' | 'opensubtitles' | ...
  lang: string      // 'en' | 'ja' | 'ru' | ...
  label: string
  format: string    // 'vtt' | 'srt' | 'ass'
}

/** Normalised stream descriptor returned by a provider adapter. */
export interface StreamResult {
  url: string
  type: 'hls' | 'mp4'
  headers?: Record<string, string>
  qualities?: { label: string; value: number | string }[]
  servers?: { id: string; label: string }[]
  /** Quality the provider actually served (e.g. "720p") — shown next to Auto
   *  for per-URL ladders where hls.js has no level to report. */
  qualityLabel?: string
  /** Subtitle tracks the provider shipped alongside the stream (signed). */
  subtitles?: SubtitleTrack[]
  /** Proxied WebVTT thumbnail-track URL (library content only). When set, the
   *  scrub preview uses sprite sheets and never starts a shadow engine. */
  storyboardUrl?: string
}
