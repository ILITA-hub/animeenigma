/**
 * Fanfic engine types (spec 2026-07-06).
 *
 * Mirrors the backend domain shapes: services/fanfic/internal/domain/request.go
 * (GenerateRequest/AnimeRef/CharacterRef) and .../domain/fanfic.go (Fanfic),
 * .../domain/tags.go (Tag).
 */

export type FanficRating = 'teen' | 'mature' | 'explicit'
export type FanficLength = 'drabble' | 'oneshot' | 'short'
export type FanficPOV = 'first' | 'third'
export type FanficLang = 'ru' | 'en'

export interface FanficCharacterRef {
  id?: string
  name: string
}

export interface FanficAnimeRef {
  id?: string
  shikimori_id?: string
  title: string
  japanese?: string
  poster?: string
}

/** POST /api/fanfic/generate body. */
export interface GenerateInput {
  anime: FanficAnimeRef
  characters: FanficCharacterRef[]
  tags: string[]
  length: FanficLength
  pov: FanficPOV
  rating: FanficRating
  language: FanficLang
  prompt: string
  canon?: boolean
}

/** One generated fanfic, as returned by list()/get(). */
export interface Fanfic {
  id: string
  anime_id: string
  anime_shikimori_id: string
  anime_title: string
  anime_japanese: string
  anime_poster: string
  characters: FanficCharacterRef[]
  tags: string[]
  length: FanficLength
  pov: FanficPOV
  rating: FanficRating
  language: FanficLang
  prompt: string
  title: string
  content: string
  model: string
  token_usage: number
  status: 'generating' | 'complete' | 'failed'
  /** Set when status:'failed' (Go: ErrorMsg string `json:"error,omitempty"`). */
  error?: string
  created_at: string
  canon: boolean
  part_count: number
}

/** A curated tag suggestion (GET /api/fanfic/tags). */
export interface FanficTag {
  slug: string
  ru: string
  en: string
}

/** Callbacks dispatched from the /generate SSE stream, in event order. */
export interface StreamHandlers {
  onMeta?: (id: string, model: string, part?: number) => void
  onDelta?: (text: string) => void
  onDone?: (id: string, title: string, tokenUsage: number, part?: number) => void
  onError?: (message: string) => void
}
