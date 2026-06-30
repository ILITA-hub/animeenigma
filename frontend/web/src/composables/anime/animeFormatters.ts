// Pure, non-reactive formatting helpers extracted from Anime.vue.
//
// These are stateless functions: they take their reactive inputs (the i18n
// translate fn `t` and/or the active `locale` string) as plain arguments and
// return a value. No refs, no module state, no side effects — so they're safe
// to unit-test in isolation and to share. Behavior is byte-identical to the
// inline versions that previously lived in Anime.vue's <script setup>.

/** Minimal structural type for vue-i18n's `t` (the subset these helpers use). */
type TranslateFn = (key: string, named?: Record<string, unknown>) => string

/** Subset of the review shape these formatters read. */
export interface ReviewStatsInput {
  status?: string
  episodes?: number
  is_rewatching?: boolean
  anime?: {
    episodes_count?: number
  }
}

/** Subset of the anime shape the episode-count formatter reads. */
export interface EpisodeCountInput {
  episodesAired?: number
  totalEpisodes?: number
  status?: string
}

/**
 * Rune count (UTF-8 code points; matches the backend's 1–2000 rune validation
 * rather than UTF-16 `.length`).
 */
export const runeLen = (s: string): number => [...s].length

/**
 * Steam-style review context line ("watched / total · status", with optional
 * rewatch segment). Selects one of several i18n key variants by progress state.
 */
export function formatReviewStats(review: ReviewStatsInput, t: TranslateFn): string {
  const status = review.status || 'watching'
  const episodes = review.episodes ?? 0
  const total = review.anime?.episodes_count ?? 0

  // Map raw status enum -> existing watchlist.* i18n keys.
  const statusKeyMap: Record<string, string> = {
    watching: 'profile.watchlist.watching',
    completed: 'profile.watchlist.completed',
    on_hold: 'profile.watchlist.onHold',
    dropped: 'profile.watchlist.dropped',
    plan_to_watch: 'profile.watchlist.planToWatch',
  }
  const statusLabel = t(statusKeyMap[status] || statusKeyMap.watching)

  // Pick template variant: closed (total known) vs open (total unknown);
  // flagged (plan_to_watch OR episodes==0) vs normal.
  const flagged = status === 'plan_to_watch' || episodes === 0
  const open = total === 0

  let key: string
  if (flagged && status === 'plan_to_watch' && open) {
    key = 'anime.reviewStats.planToWatchOpenFlag'
  } else if (flagged && status === 'plan_to_watch') {
    key = 'anime.reviewStats.planToWatchFlag'
  } else if (flagged && open) {
    key = 'anime.reviewStats.noProgressOpen'
  } else if (flagged) {
    key = 'anime.reviewStats.noProgress'
  } else if (open) {
    key = 'anime.reviewStats.watchedOpen'
  } else {
    key = 'anime.reviewStats.watched'
  }

  const base = t(key, { watched: episodes, total, status: statusLabel })
  // Append the rewatch segment when the reviewer is rewatching.
  return review.is_rewatching ? `${base} · ${t('anime.reviewStats.rewatch')}` : base
}

/** Whether a review's progress should be visually flagged (warning tint). */
export function isReviewFlagged(review: ReviewStatsInput): boolean {
  const status = review.status || 'watching'
  const episodes = review.episodes ?? 0
  return status === 'plan_to_watch' || episodes === 0
}

/** "aired / total" (ongoing), "total" (completed), or unknown-state variants. */
export function formatEpisodeCount(anime: EpisodeCountInput, t: TranslateFn): string {
  const aired = anime.episodesAired || 0
  const total = anime.totalEpisodes || 0

  if (total > 0) {
    // Total known - show "aired / total" for ongoing, or just "total" for completed
    if (anime.status === 'ongoing' && aired > 0 && aired < total) {
      return t('anime.episodeProgress', { aired, total })
    }
    return t('anime.episodeTotal', { total })
  } else if (aired > 0) {
    // Total unknown but some aired
    return t('anime.episodeAiredUnknown', { aired })
  }
  // Nothing known
  return t('anime.episodeUnknown')
}

/**
 * Compact watchers count ("1.2K", "12K") via Intl.NumberFormat, locale-aware.
 * Falls back to the plain number string on old browsers / unknown locales.
 */
export function formatCount(n: number, locale: string): string {
  try {
    return new Intl.NumberFormat(locale, { notation: 'compact', maximumFractionDigits: 1 }).format(n)
  } catch {
    // Old browser / unknown locale — graceful degradation.
    return n.toString()
  }
}

/**
 * Parse the raw `watch_progress:{animeId}` localStorage JSON and return the
 * episode number with the most-recent `updatedAt`, or undefined when there's
 * no usable entry / the data is corrupt. Pure: takes the raw string, performs
 * no I/O. Returns undefined for null/empty input so the caller can early-out.
 */
export function parseLastWatchedEpisode(raw: string | null): number | undefined {
  if (!raw) return undefined
  try {
    const data = JSON.parse(raw) as Record<string, { updatedAt?: number }>
    let latest = 0
    let latestEp: number | undefined
    for (const [ep, info] of Object.entries(data)) {
      if (info.updatedAt && info.updatedAt > latest) {
        latest = info.updatedAt
        // WR-10: explicit radix 10 to defend against the historic octal-on-
        // leading-zero foot-gun ("08" -> 0 in pre-ES5 engines) and to satisfy
        // ESLint's `radix` rule.
        latestEp = parseInt(ep, 10)
      }
    }
    if (latestEp && !isNaN(latestEp)) return latestEp
  } catch { /* ignore corrupted data */ }
  return undefined
}
