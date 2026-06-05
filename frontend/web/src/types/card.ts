// The single view-model every unified card (PosterCard / PosterRow / MediaTile)
// renders. Source-API shapes are normalised into this by src/utils/toCardModel.ts.
// Design contract: docs/superpowers/specs/2026-06-05-unified-anime-card-design.md

export type ListStatus =
  | 'watching'
  | 'plan_to_watch'
  | 'completed'
  | 'on_hold'
  | 'dropped'

export interface AnimeCardModel {
  id: string
  href: string                 // /anime/:id  (continue-watching appends ?episode=N)
  title: string                // already localized
  coverImage: string
  year?: number
  episodes?: number
  primaryGenre?: string        // already localized
  malScore?: number            // ★ amber  (Shikimori/MAL)
  siteScore?: number           // ◆ diamond cyan (AnimeEnigma reviews)
  quality?: string             // e.g. "1080p" — neutral overlay badge
  hasDub?: boolean             // DUB overlay badge
  listStatus?: ListStatus | null
  progress?: { current: number; total: number } | null
  airing?: boolean             // ONGOING (green + pulse) — only while true
  nextEpisode?: { ep: number; when: string } | null  // Row / MediaTile
}

// Per-user / per-listing overlays that don't live on the raw anime object
// (site rating map, watchlist status map, progress map). Merged in by the
// parent when it builds a model.
export interface CardExtras {
  siteScore?: number
  listStatus?: ListStatus | null
  progress?: { current: number; total: number } | null
}
