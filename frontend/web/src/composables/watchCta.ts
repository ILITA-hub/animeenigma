// Pure decision function for the anime-page primary play button.
//
// The contract is pinned by src/composables/__tests__/watchCta.spec.ts;
// design: docs/superpowers/specs/2026-06-05-rewatch-and-watched-button-design.md.
// Consumed by views/Anime.vue (hero button + player placeholder).

export type WatchCtaAction =
  | 'watch'
  | 'start-from-1'
  | 'continue'
  | 'mark-watched'
  | 'rewatch'

export interface WatchCtaInput {
  /** Whether the viewer is logged in (drives list-aware actions). */
  isAuthenticated: boolean
  /** Highest completed episode for this user; 0 when none. */
  lastWatched: number
  /** Total episodes; 0 when unknown (then "full" is unreachable). */
  totalEpisodes: number
  /** anime_list status, or null when not in list / anonymous. */
  listStatus: string | null
}

export interface WatchCta {
  action: WatchCtaAction
  /** Episode the player should mount on. */
  startEpisode: number
  /** i18n key for the button label. */
  labelKey: string
  /** Optional i18n interpolation params (e.g. { n } for "continue from ep N"). */
  labelParams?: Record<string, number>
}

export function computeWatchCta(input: WatchCtaInput): WatchCta {
  const { isAuthenticated, lastWatched, totalEpisodes, listStatus } = input

  const total = totalEpisodes > 0 ? totalEpisodes : 0
  // Clamp to [0, total] when total is known (defends against a stale/shrunk
  // total leaving lastWatched > total); otherwise just floor at 0.
  const last = total > 0 ? Math.min(Math.max(lastWatched, 0), total) : Math.max(lastWatched, 0)
  const isFull = total > 0 && last >= total
  const isCompleted = listStatus === 'completed'

  const watch: WatchCta = { action: 'watch', startEpisode: 1, labelKey: 'anime.watchNow' }
  const continueFrom = (ep: number): WatchCta => ({
    action: 'continue',
    startEpisode: ep,
    labelKey: 'anime.continueEp',
    labelParams: { n: ep },
  })

  // Anonymous viewers have no list, so the list-aware terminals (mark-watched /
  // rewatch) never apply — fall back to the pre-existing watch/continue path.
  if (!isAuthenticated) {
    return last > 0 && !isFull ? continueFrom(last + 1) : watch
  }

  // P0 — nothing watched yet.
  if (last === 0) {
    return isCompleted
      ? { action: 'start-from-1', startEpisode: 1, labelKey: 'anime.startFromEp1' }
      : watch
  }

  // Pf — fully watched terminal (requires a known total). List status picks the
  // verb: not-completed → offer to mark it; completed → offer a rewatch.
  if (isFull) {
    return isCompleted
      ? { action: 'rewatch', startEpisode: 1, labelKey: 'anime.resume.rewatch' }
      : { action: 'mark-watched', startEpisode: 1, labelKey: 'anime.markAsWatched' }
  }

  // Pp — partial progress (also the unknown-total path). Real progress wins
  // over list status, so a "completed" entry mid-cycle still continues.
  return continueFrom(last + 1)
}
