export type ShowcaseBlockType =
  | 'about'
  | 'favorite_anime'
  | 'stats'
  | 'favorite_character'
  | 'card_collection'

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

export interface ShowcaseBlock {
  type: ShowcaseBlockType
  order: number
  config: AboutConfig | FavoriteAnimeConfig | FavoriteCharacterConfig | CardCollectionConfig | StatsConfig
}

export const MAX_SHOWCASE_BLOCKS = 12
export const MAX_SHOWCASE_ITEMS = 12
