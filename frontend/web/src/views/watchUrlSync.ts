import type { LocationQuery } from 'vue-router'

/** The live source/team/episode AePlayer emits via `url-sync`. provider/team are
 *  EMPTY for an auto/smart-default selection and populated only for a user-pinned
 *  source (manual pick or `?provider=` deep-link). */
export interface WatchUrlState {
  provider: string
  team: string
  episode: number
}

/**
 * Merge a player `url-sync` emission into the current route query, preserving
 * unrelated params. Empty provider/team (auto/smart-default) and episode ≤ 0
 * REMOVE that param — so a plain reload of an auto-selected source carries no
 * `?provider` and re-runs the deterministic BEST default (the product rule a
 * previously-watched source must not override). Only a user-pinned source
 * writes provider/team, making the link shareable + bookmarkable.
 */
export function nextWatchQuery(current: LocationQuery, s: WatchUrlState): Record<string, string> {
  const next: Record<string, string> = {}
  for (const [k, v] of Object.entries(current)) {
    if (typeof v === 'string') next[k] = v
  }
  if (s.provider) next.provider = s.provider
  else delete next.provider
  if (s.team) next.team = s.team
  else delete next.team
  if (s.episode > 0) next.episode = String(s.episode)
  else delete next.episode
  return next
}

/** True when the merged query differs from the current one on a synced param —
 *  used to avoid a no-op router.replace (and the history churn it causes). */
export function watchQueryChanged(current: LocationQuery, next: Record<string, string>): boolean {
  return (
    next.provider !== current.provider ||
    next.team !== current.team ||
    next.episode !== current.episode
  )
}
