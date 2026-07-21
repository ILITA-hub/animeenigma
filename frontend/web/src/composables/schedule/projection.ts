// frontend/web/src/composables/schedule/projection.ts
import type { ScheduleAnime, Occurrence } from './types'
import { wallClockDate } from './timezone'

const WEEK_MS = 7 * 86400000

/**
 * Project an anime's weekly airings into [windowStart, windowEnd).
 * Anchor = next_episode_at (concrete next airing). Episode at anchor = episodes_aired + 1.
 * Each week k (0, 1, 2 …): date = anchor + k weeks, episode = episodes_aired + 1 + k.
 * The anchor is the provider's known next airing, so occurrences before it must
 * not be inferred: a hiatus would otherwise create phantom weekly episodes.
 * Included when ep >= 1 and (episodes_count <= 0 || ep <= episodes_count).
 *
 * `tz` shifts the anchor into that zone's wall-clock space BEFORE windowing,
 * so occurrence dates (and therefore day grouping + displayed HH:MM) follow
 * the user's chosen timezone. The window is wall-clock day boundaries, which
 * keeps the comparison consistent. No tz → browser-local (legacy behavior).
 */
export function projectOccurrences(
  anime: ScheduleAnime,
  windowStart: Date,
  windowEnd: Date,
  tz?: string,
): Occurrence[] {
  if (!anime.next_episode_at) return []
  const anchor = new Date(anime.next_episode_at)
  if (Number.isNaN(anchor.getTime())) return []
  const anchorMs = wallClockDate(anchor, tz).getTime()

  const aired = anime.episodes_aired ?? 0
  const total = anime.episodes_count ?? 0
  const out: Occurrence[] = []

  const startMs = windowStart.getTime()
  const endMs = windowEnd.getTime()
  const kFrom = Math.max(0, Math.floor((startMs - anchorMs) / WEEK_MS) - 1)
  const kTo = Math.ceil((endMs - anchorMs) / WEEK_MS) + 1

  for (let k = kFrom; k <= kTo; k++) {
    const ms = anchorMs + k * WEEK_MS
    if (ms < startMs || ms >= endMs) continue
    const episode = aired + 1 + k
    if (episode < 1) continue
    if (total > 0 && episode > total) continue
    out.push({ anime, episode, date: new Date(ms) })
  }
  return out
}

/** Flatten projections for many anime into one window. */
export function occurrencesInRange(
  animes: ScheduleAnime[],
  windowStart: Date,
  windowEnd: Date,
  tz?: string,
): Occurrence[] {
  return animes.flatMap((a) => projectOccurrences(a, windowStart, windowEnd, tz))
}
