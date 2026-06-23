// Canonical Shikimori anime "kind" enum — the single source of truth for
// both the Browse anime-type filter (FilterCheckboxList options) and the
// backend whitelist (services/catalog handler). Order drives display order.
// Labels live in i18n under common.filters.kind.* (shared by Browse + List).
export const ANIME_KINDS = [
  'tv', 'movie', 'ova', 'ona', 'special', 'tv_special', 'music', 'cm', 'pv',
] as const

export type AnimeKind = (typeof ANIME_KINDS)[number]
