// Display helpers for anime production metadata.

// Shikimori age-rating codes → classification badge. These are codes, not
// translated prose, so they are the same in every locale.
const RATING_LABELS: Record<string, string> = {
  g: 'G',
  pg: 'PG',
  pg_13: 'PG-13',
  r: 'R-17',
  r_plus: 'R+',
  rx: 'Rx',
}

export function ratingLabel(raw?: string): string {
  if (!raw) return ''
  return RATING_LABELS[raw] ?? ''
}

// Known Shikimori adaptation sources. Returns the i18n key suffix under
// anime.sources.*, or null when unknown/empty (caller falls back to the raw
// value or hides the row).
const KNOWN_SOURCES = new Set([
  'original',
  'manga',
  'web_manga',
  'novel',
  'light_novel',
  'visual_novel',
  'game',
  'card_game',
  'music',
  'book',
  'other',
])

export function sourceLabelKey(raw?: string): string | null {
  if (!raw) return null
  return KNOWN_SOURCES.has(raw) ? raw : null
}
