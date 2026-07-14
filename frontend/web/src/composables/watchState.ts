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
 * Shared "is last+1 actually available" gate. Released/completed catalogs are
 * always available — `episodes_aired` can lag the true finale count, so never
 * gate those on it. Ongoing catalogs gate on aired vs. provider-loaded count.
 * Used by both the CTA and the banner so they can't drift out of sync.
 */
function isNextEpisodeAvailable(a: {
  last: number
  status: string
  episodesAired: number
  loadedEpisodes: number
}): boolean {
  const aired = Math.max(a.episodesAired, a.loadedEpisodes)
  const isReleased = a.status === 'released' || a.status === 'completed'
  return isReleased || (aired > 0 && aired > a.last)
}

/**
 * The episode the player should mount on. The ONLY start-episode authority.
 *   first-time (0)            → 1
 *   fully watched (last >= total) → 1   (re-open from the start, a fresh rewatch)
 *   next episode not out yet  → last   (re-open what's already there)
 *   otherwise                 → last + 1
 * Always returns a number >= 1.
 *
 * A fully-watched anime re-opens on episode 1, matching the "rewatch" CTA — the
 * old "re-load the final episode" behaviour stranded finished viewers on the
 * last episode instead of letting them start over.
 *
 * `availability` is optional so existing callers that don't have air-date data
 * keep their old (ungated) behaviour.
 */
export function resolveStartEpisode(
  lastWatched: number,
  totalEpisodes: number,
  availability?: { status: string; episodesAired: number; loadedEpisodes: number },
): number {
  const last = clampLastWatched(lastWatched, totalEpisodes)
  if (last <= 0) return 1
  const total = totalEpisodes > 0 ? totalEpisodes : 0
  if (total > 0 && last >= total) return 1
  if (availability && !isNextEpisodeAvailable({ last, ...availability })) return last
  return last + 1
}

function resolveCta(a: {
  isAuthenticated: boolean
  last: number
  isFull: boolean
  isCompleted: boolean
  /** Whether episode last+1 is actually out (mirrors the banner's check). */
  nextAvailable: boolean
}): WatchCta {
  const { isAuthenticated, last, isFull, isCompleted, nextAvailable } = a
  const watch: WatchCta = { action: 'watch', startEpisode: 1, labelKey: 'anime.watchNow' }
  const continueFrom = (ep: number): WatchCta => ({
    action: 'continue',
    startEpisode: ep,
    labelKey: 'anime.continueEp',
    labelParams: { n: ep },
  })
  // Nothing new is out yet — re-open the last completed episode instead of
  // dangling a "Continue ep. N+1" button on an episode with no source.
  const reopenLast: WatchCta = { ...watch, startEpisode: last }

  // Anonymous: no list, so the list-aware terminals never apply.
  if (!isAuthenticated) {
    if (last <= 0 || isFull) return watch
    return nextAvailable ? continueFrom(last + 1) : reopenLast
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
  // Partial progress (also the unknown-total path) — real progress wins,
  // but only offer "continue" to an episode that's actually out.
  return nextAvailable ? continueFrom(last + 1) : reopenLast
}

function resolveBanner(a: {
  last: number
  total: number
  nextAvailable: boolean
  nextEpisodeAt?: string
  formatEta?: (iso: string) => string
  nowMs: number
}): ResumeBanner {
  const { last, total, nextAvailable, nextEpisodeAt, formatEta, nowMs } = a

  if (last <= 0) return { kind: 'none' } // first-time
  if (total > 0 && last >= total) return { kind: 'none' } // finished — no surface

  if (nextAvailable) {
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
  const nextAvailable = isNextEpisodeAvailable({
    last,
    status: input.status,
    episodesAired: input.episodesAired,
    loadedEpisodes: input.loadedEpisodes,
  })

  const cta = resolveCta({
    isAuthenticated: input.isAuthenticated,
    last,
    isFull,
    isCompleted,
    nextAvailable,
  })
  const banner = resolveBanner({
    last,
    total,
    nextAvailable,
    nextEpisodeAt: input.nextEpisodeAt,
    formatEta: input.formatEta,
    nowMs: input.nowMs ?? Date.now(),
  })

  return { banner, cta }
}
