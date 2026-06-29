export type ShowcaseState = 'none' | 'hidden' | 'visible'

/**
 * Derive the showcase visibility state from a (enabled, block count) pair.
 * Mirrors the backend mapping (auth GetShowcaseState): empty ⇒ 'none',
 * otherwise 'visible' when published / 'hidden' when not. Single FE source of
 * truth so the post-save state update can't drift from the backend rule.
 */
export function deriveShowcaseState(enabled: boolean, count: number): ShowcaseState {
  if (count === 0) return 'none'
  return enabled ? 'visible' : 'hidden'
}

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
  w?: number
  h?: number
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

export interface SizeBound { minW: number; maxW: number; minH: number; maxH: number; defW: number; defH: number }
const sb = (minW: number, maxW: number, minH: number, maxH: number, defW: number, defH: number): SizeBound =>
  ({ minW, maxW, minH, maxH, defW, defH })

// MUST mirror Go domain.VariantSizeAllowlist (services/player/internal/domain/showcase.go).
export const VARIANT_SIZE: Record<ShowcaseBlockType, Record<string, SizeBound>> = {
  about: { quote: sb(2,4,1,2,2,1), bio: sb(2,4,1,2,2,2), terminal: sb(2,4,1,2,2,2), minimal: sb(2,4,1,2,2,1), vn: sb(2,2,1,1,2,1) },
  favorite_anime: { row: sb(2,4,1,1,4,1), podium: sb(2,2,2,2,2,2), grid: sb(2,4,1,3,4,2), list: sb(2,2,1,3,2,2), banner: sb(2,4,1,3,2,2) },
  favorite_character: { circles: sb(1,4,1,3,2,1), portraits: sb(2,4,1,3,2,2), hero: sb(2,4,1,3,2,2), hex: sb(1,4,1,3,2,2) },
  card_collection: { row: sb(2,4,1,1,2,1), fan: sb(2,4,2,3,2,2), grid: sb(2,4,1,3,2,2), hero: sb(2,4,1,2,2,2), tilt3d: sb(2,4,2,3,3,2) },
  stats: { tiles: sb(2,2,1,1,2,1), rings: sb(2,2,1,1,2,1), bars: sb(2,2,1,1,2,1), strip: sb(2,2,1,1,2,1) },
  continue_watching: { cards: sb(2,4,1,3,2,2) },
  op_ed: { grid: sb(2,4,1,3,2,2) },
  anime_dna: { bars: sb(1,2,1,3,1,2) },
  compatibility: { ring: sb(1,2,1,1,2,1) },
}

export const sizeFor = (t: ShowcaseBlockType, variant?: string): SizeBound =>
  VARIANT_SIZE[t][variant ?? ''] ?? VARIANT_SIZE[t][SHOWCASE_VARIANTS[t][0]]

const W_CLASS: Record<number, string> = {
  1: 'col-span-1 md:col-span-1', 2: 'col-span-2 md:col-span-2',
  3: 'col-span-2 md:col-span-3', 4: 'col-span-2 md:col-span-4',
}
const H_CLASS: Record<number, string> = { 1: 'row-span-1', 2: 'row-span-2', 3: 'row-span-3' }
export const spanClasses = (w: number, h: number): string =>
  `${W_CLASS[w] ?? W_CLASS[2]} ${H_CLASS[h] ?? H_CLASS[1]}`

export const clampSize = (t: ShowcaseBlockType, variant: string | undefined, w: number, h: number) => {
  const b = sizeFor(t, variant)
  const c = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v))
  return { w: c(w || b.defW, b.minW, b.maxW), h: c(h || b.defH, b.minH, b.maxH) }
}
