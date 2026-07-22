// frontend/web/src/composables/schedule/projection.ts
import type { ScheduleAnime, Occurrence } from './types'
import { wallClockDate } from './timezone'

const WEEK_MS = 7 * 86400000

/**
 * Return an anime's past weekly airings and confirmed next airing in
 * [windowStart, windowEnd).
 * Anchor = next_episode_at (concrete next airing). Episode at anchor = episodes_aired + 1.
 * Each past week k (… -2, -1, 0): date = anchor + k weeks,
 * episode = episodes_aired + 1 + k.
 * Backward projection keeps already-aired entries visible when the user navigates
 * to an earlier week; the episode-number guard prevents projection before episode 1.
 * Dates after the anchor are not projected because `next_episode_at` only confirms
 * one airing and shows may pause or change their broadcast plan.
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
  const kFrom = Math.floor((startMs - anchorMs) / WEEK_MS) - 1
  const kTo = Math.min(0, Math.ceil((endMs - anchorMs) / WEEK_MS) + 1)

  const aired = anime.episodes_aired ?? 0
  const total = anime.episodes_count ?? 0
  const out: Occurrence[] = []

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

/** Flatten past and confirmed next-airing occurrences for many anime into one window. */
export function occurrencesInRange(
  animes: ScheduleAnime[],
  windowStart: Date,
  windowEnd: Date,
  tz?: string,
): Occurrence[] {
  return animes.flatMap((a) => projectOccurrences(a, windowStart, windowEnd, tz))
}
