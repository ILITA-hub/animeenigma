// frontend/web/src/composables/schedule/projection.ts
import type { ScheduleAnime, Occurrence } from './types'

const WEEK_MS = 7 * 86400000

/**
 * Project an anime's weekly airings into [windowStart, windowEnd).
 * Anchor = next_episode_at (concrete next airing). Episode at anchor = episodes_aired + 1.
 * Each week k (… -1, 0, 1 …): date = anchor + k weeks, episode = episodes_aired + 1 + k.
 * Included when ep >= 1 and (episodes_count <= 0 || ep <= episodes_count).
 */
export function projectOccurrences(
  anime: ScheduleAnime,
  windowStart: Date,
  windowEnd: Date,
): Occurrence[] {
  if (!anime.next_episode_at) return []
  const anchor = new Date(anime.next_episode_at)
  const anchorMs = anchor.getTime()
  if (Number.isNaN(anchorMs)) return []

  const aired = anime.episodes_aired ?? 0
  const total = anime.episodes_count ?? 0
  const out: Occurrence[] = []

  const startMs = windowStart.getTime()
  const endMs = windowEnd.getTime()
  const kFrom = Math.floor((startMs - anchorMs) / WEEK_MS) - 1
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
): Occurrence[] {
  return animes.flatMap((a) => projectOccurrences(a, windowStart, windowEnd))
}
