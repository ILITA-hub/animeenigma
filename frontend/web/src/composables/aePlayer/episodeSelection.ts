import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

/**
 * Pick which episode to play from a freshly-loaded provider list, preserving
 * the user's current selection across a provider switch.
 *
 * Priority:
 *  1. Exact same episode NUMBER in the new provider's list (the common case —
 *     "I chose EP 12, switch source, still EP 12").
 *  2. Nearest available episode whose number is ≤ the target (a provider that
 *     simply has fewer episodes shouldn't snap the user all the way back to
 *     EP 1 — it lands on its highest episode that isn't past where they were).
 *  3. The first episode the provider offers (e.g. an offset/second-season list
 *     that starts above the target).
 *  4. The previous selection itself, so a transient empty list never nulls it.
 *
 * Returns null only when there is genuinely nothing to play (empty list AND no
 * prior selection) — the caller surfaces a "no episodes" error for that.
 */
export function pickEpisodeForProvider(
  eps: EpisodeOption[],
  targetNumber: number,
  previous: EpisodeOption | null,
): EpisodeOption | null {
  // 1. Exact number match — keep the user on the same episode.
  const exact = eps.find((e) => e.number === targetNumber)
  if (exact) return exact

  // 2. Nearest episode at or below the target (closest from below).
  let below: EpisodeOption | null = null
  for (const e of eps) {
    if (e.number <= targetNumber && (below === null || e.number > below.number)) {
      below = e
    }
  }
  if (below) return below

  // 3. First available (offset list that starts past the target), else
  // 4. fall back to the previous selection so we never null out unexpectedly.
  return eps[0] ?? previous ?? null
}

/**
 * Decide whether the player should re-pick its episode when `initialEpisode`
 * changes AFTER mount. Resume/watch-progress resolves asynchronously, so the
 * prop flips from its mount value (default 1) to e.g. `lastWatched + 1` a tick
 * later. We re-select then — UNLESS the user has already manually chosen an
 * episode (never yank them off a deliberate pick), the prop is still absent,
 * or we are already on the target (no churn). Pure: testable in isolation.
 */
export function shouldReselectEpisode(
  currentNumber: number | null,
  initialEpisode: number | undefined,
  userPicked: boolean,
): boolean {
  if (initialEpisode == null || userPicked) return false
  return currentNumber !== initialEpisode
}
