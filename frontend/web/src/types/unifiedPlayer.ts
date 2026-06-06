// frontend/web/src/types/unifiedPlayer.ts
// Single source of truth for unified-player types. Imported by composables,
// the provider registry, and components — keep names stable across tasks.

export type AudioKind = 'sub' | 'dub'
export type TrackLang = 'en' | 'ru' | 'ja'
export type ContentKind = 'common' | 'hentai'
export type ProviderGroup = 'en' | 'ru' | 'adult' | 'raw' | 'first-party'

/** Static definition of a selectable backend provider. */
export interface ProviderDef {
  id: string                 // 'allanime', 'kodik', 'ae', ...
  name: string               // display label
  hue: string                // identity hue (brand-exempt: cyan/orange/pink/rose)
  group: ProviderGroup
  audios: AudioKind[]        // audio kinds this backend can serve
  langs: TrackLang[]         // track languages this backend serves
  content: ContentKind[]     // which content kinds it serves
  scraper: boolean           // true => live health comes from /scraper/health
  /** Non-scraper backends that are hard-disabled or WIP carry their reason here. */
  staticDisabled?: { reason: string; description: string; wip?: boolean }
}

export type ChipState = 'active' | 'disabled' | 'down' | 'irrelevant' | 'wip'

/** A provider as rendered in the Source panel: definition + computed state. */
export interface ProviderRow {
  def: ProviderDef
  state: ChipState
  /** Hover/tooltip text for non-active states. */
  reason?: string
}

/** Live + registry health for one scraper provider (from the backend). */
export interface ScraperProviderHealth {
  name: string
  enabled: boolean
  up: boolean
  reason?: string
  description?: string
}

/** The user's current source selection. */
export interface Combo {
  audio: AudioKind
  lang: TrackLang
  provider: string
  server: string
  team: string | null
}

/** Normalised stream descriptor returned by a provider adapter. */
export interface StreamResult {
  url: string
  type: 'hls' | 'mp4'
  headers?: Record<string, string>
  qualities?: { label: string; value: number | string }[]
  servers?: { id: string; label: string }[]
}
