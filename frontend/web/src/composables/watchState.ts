// Pure decision functions for episode selection + resume presentation.
//
// Single source of truth for "which episode does the player open" and
// "what banner / CTA verb to show". Replaces watchCta.ts and the
// startEpisode/kind logic of useResumeStateMachine.ts.
//
// Pinned by src/composables/__tests__/watchState.spec.ts.

export type WatchCtaAction = 'watch' | 'start-from-1' | 'continue' | 'mark-watched' | 'rewatch'

export interface WatchCta {
  action: WatchCtaAction
  /** Episode the player should mount on. */
  startEpisode: number
  /** i18n key for the button label. */
  labelKey: string
  /** Optional i18n interpolation params (e.g. { n } for "continue from ep N"). */
  labelParams?: Record<string, number>
}

export type ResumeBanner =
  | { kind: 'none' }
  | { kind: 'just-finished'; episode: number }
  | { kind: 'next-unavailable'; episode: number; etaLabel?: string }

export interface ResumeStateInput {
  /** Highest COMPLETED episode for this user; 0 when none. */
  lastWatched: number
  /** Total episodes; 0 when unknown (then "full" is unreachable). */
  totalEpisodes: number
  /** Shikimori episodes_aired; 0 when unknown. */
  episodesAired: number
  /** Highest episode loaded into our sources (overrides lagging episodesAired); 0 when unknown. */
  loadedEpisodes: number
  /** anime.status — 'ongoing' | 'released' | 'completed' | … */
  status: string
  /** ISO air time of the next episode, or undefined. */
  nextEpisodeAt?: string
  /** anime_list status, or null when not in list / anonymous. */
  listStatus: string | null
  /** Whether the viewer is logged in (drives list-aware CTA actions). */
  isAuthenticated: boolean
  /** Localized formatter for a FUTURE air time → ETA label. Omitted ⇒ no ETA. */
  formatEta?: (iso: string) => string
  /** Injectable clock for the future/past air-time comparison (defaults to Date.now()). */
  nowMs?: number
}

export interface ResumeState {
  banner: ResumeBanner
  cta: WatchCta
}

/** Clamp lastWatched into [0, total] when total is known; else floor at 0. */
export function clampLastWatched(lastWatched: number, totalEpisodes: number): number {
  const total = totalEpisodes > 0 ? totalEpisodes : 0
  return total > 0 ? Math.min(Math.max(lastWatched, 0), total) : Math.max(lastWatched, 0)
}

/**
 * The episode the player should mount on. The ONLY start-episode authority.
 *   first-time (0)            → 1
 *   fully watched (last >= total) → 1   (re-open from the start, a fresh rewatch)
 *   otherwise                 → last + 1
 * Always returns a number >= 1.
 *
 * A fully-watched anime re-opens on episode 1, matching the "rewatch" CTA — the
 * old "re-load the final episode" behaviour stranded finished viewers on the
 * last episode instead of letting them start over.
 */
export function resolveStartEpisode(lastWatched: number, totalEpisodes: number): number {
  const last = clampLastWatched(lastWatched, totalEpisodes)
  if (last <= 0) return 1
  const total = totalEpisodes > 0 ? totalEpisodes : 0
  if (total > 0 && last >= total) return 1
  return last + 1
}

function resolveCta(a: {
  isAuthenticated: boolean
  last: number
  isFull: boolean
  isCompleted: boolean
}): WatchCta {
  const { isAuthenticated, last, isFull, isCompleted } = a
  const watch: WatchCta = { action: 'watch', startEpisode: 1, labelKey: 'anime.watchNow' }
  const continueFrom = (ep: number): WatchCta => ({
    action: 'continue',
    startEpisode: ep,
    labelKey: 'anime.continueEp',
    labelParams: { n: ep },
  })

  // Anonymous: no list, so the list-aware terminals never apply.
  if (!isAuthenticated) {
    return last > 0 && !isFull ? continueFrom(last + 1) : watch
  }
  // Nothing watched yet.
  if (last === 0) {
    return isCompleted
      ? { action: 'start-from-1', startEpisode: 1, labelKey: 'anime.startFromEp1' }
      : watch
  }
  // Fully watched terminal — list status picks the verb.
  if (isFull) {
    return isCompleted
      ? { action: 'rewatch', startEpisode: 1, labelKey: 'anime.resume.rewatch' }
      : { action: 'mark-watched', startEpisode: 1, labelKey: 'anime.markAsWatched' }
  }
  // Partial progress (also the unknown-total path) — real progress wins.
  return continueFrom(last + 1)
}

function resolveBanner(a: {
  last: number
  total: number
  episodesAired: number
  loadedEpisodes: number
  status: string
  nextEpisodeAt?: string
  formatEta?: (iso: string) => string
  nowMs: number
}): ResumeBanner {
  const { last, total, episodesAired, loadedEpisodes, status, nextEpisodeAt, formatEta, nowMs } = a

  if (last <= 0) return { kind: 'none' } // first-time
  if (total > 0 && last >= total) return { kind: 'none' } // finished — no surface

  // Is episode last+1 available? Released/completed → yes. Otherwise compare
  // against the effective aired count (max of Shikimori + what our sources hold).
  const aired = Math.max(episodesAired, loadedEpisodes)
  const isReleased = status === 'released' || status === 'completed'
  const nextIsAired = aired > 0 && aired > last
  if (isReleased || nextIsAired) {
    return { kind: 'just-finished', episode: last } // 'watching' breadcrumb
  }

  // Next episode not available yet (merged not-yet-aired + episode-not-loaded-yet).
  const nextEp = last + 1
  const t = nextEpisodeAt ? new Date(nextEpisodeAt).getTime() : Number.NaN
  const etaLabel =
    !Number.isNaN(t) && t > nowMs && formatEta ? formatEta(nextEpisodeAt as string) : undefined
  return etaLabel ? { kind: 'next-unavailable', episode: nextEp, etaLabel } : { kind: 'next-unavailable', episode: nextEp }
}

/**
 * Banner + CTA verb for the anime page. Pure. Replaces computeWatchCta + the
 * 5-kind resume state machine.
 */
export function resolveResumeState(input: ResumeStateInput): ResumeState {
  const last = clampLastWatched(input.lastWatched, input.totalEpisodes)
  const total = input.totalEpisodes > 0 ? input.totalEpisodes : 0
  const isFull = total > 0 && last >= total
  const isCompleted = input.listStatus === 'completed'

  const cta = resolveCta({ isAuthenticated: input.isAuthenticated, last, isFull, isCompleted })
  const banner = resolveBanner({
    last,
    total,
    episodesAired: input.episodesAired,
    loadedEpisodes: input.loadedEpisodes,
    status: input.status,
    nextEpisodeAt: input.nextEpisodeAt,
    formatEta: input.formatEta,
    nowMs: input.nowMs ?? Date.now(),
  })

  return { banner, cta }
}
