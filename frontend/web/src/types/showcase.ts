export type ShowcaseBlockType =
  | 'about'
  | 'favorite_anime'
  | 'stats'
  | 'favorite_character'
  | 'card_collection'
  | 'continue_watching'
  | 'op_ed'
  | 'anime_dna'
  | 'compatibility'

export interface AboutConfig {
  title?: string
  text?: string
}
export interface FavoriteAnimeConfig {
  anime_ids: string[]
}
export interface FavoriteCharacterConfig {
  character_ids: number[]
}
export interface CardCollectionConfig {
  card_ids: string[]
}
export type StatsConfig = Record<string, never>

export interface OpEdConfig { theme_ids: string[] }
// continue_watching / anime_dna / compatibility carry no config:
export type AutoConfig = Record<string, never>

export interface ShowcaseBlock {
  type: ShowcaseBlockType
  variant?: string
  order: number
  config: AboutConfig | FavoriteAnimeConfig | FavoriteCharacterConfig
        | CardCollectionConfig | StatsConfig | OpEdConfig | AutoConfig
}

export const MAX_SHOWCASE_BLOCKS = 12
export const MAX_SHOWCASE_ITEMS = 12

// Allowlist — MUST mirror domain.VariantAllowlist (Go). First entry = default.
export const SHOWCASE_VARIANTS: Record<ShowcaseBlockType, string[]> = {
  about: ['quote', 'bio', 'terminal', 'minimal', 'vn'],
  favorite_anime: ['row', 'podium', 'grid', 'list', 'banner'],
  favorite_character: ['circles', 'portraits', 'hero', 'hex'],
  card_collection: ['row', 'fan', 'grid', 'hero', 'tilt3d'],
  stats: ['tiles', 'rings', 'bars', 'strip'],
  continue_watching: ['cards'],
  op_ed: ['grid'],
  anime_dna: ['bars'],
  compatibility: ['ring'],
}
export const defaultVariant = (t: ShowcaseBlockType) => SHOWCASE_VARIANTS[t][0]
