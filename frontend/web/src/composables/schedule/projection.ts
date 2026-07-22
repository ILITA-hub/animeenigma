// frontend/web/src/composables/schedule/projection.ts
import type { ScheduleAnime, Occurrence } from './types'
import { wallClockDate } from './timezone'

/**
 * Return an anime's confirmed next airing when it falls in [windowStart, windowEnd).
 * `next_episode_at` describes one concrete episode, not a promise that every
 * later episode will air at seven-day intervals. Projecting beyond that anchor
 * creates false dates whenever a season pauses or its broadcast plan changes.
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
  const startMs = windowStart.getTime()
  const endMs = windowEnd.getTime()
  if (anchorMs < startMs || anchorMs >= endMs) return []

  const episode = (anime.episodes_aired ?? 0) + 1
  const total = anime.episodes_count ?? 0
  if (episode < 1 || (total > 0 && episode > total)) return []

  return [{ anime, episode, date: new Date(anchorMs) }]
}

/** Flatten confirmed next-airing occurrences for many anime into one window. */
export function occurrencesInRange(
  animes: ScheduleAnime[],
  windowStart: Date,
  windowEnd: Date,
  tz?: string,
): Occurrence[] {
  return animes.flatMap((a) => projectOccurrences(a, windowStart, windowEnd, tz))
}
