// frontend/web/src/composables/schedule/projection.ts
import type { ScheduleAnime, ScheduleConfirmedOccurrence, Occurrence } from './types'
import { wallClockDate } from './timezone'

/**
 * Return only the confirmed next airing when it falls in [windowStart, windowEnd).
 * Historical episodes come from provider-confirmed occurrence records; deriving
 * them from this future anchor would invent episodes across broadcast hiatuses.
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

/** Merge exact historical records with each anime's single confirmed next airing. */
export function occurrencesInRange(
  visibleAnimes: ScheduleAnime[],
  upcomingAnimes: ScheduleAnime[],
  confirmed: ScheduleConfirmedOccurrence[],
  windowStart: Date,
  windowEnd: Date,
  tz?: string,
): Occurrence[] {
  const animeByID = new Map(visibleAnimes.map((anime) => [anime.id, anime]))
  const history: Occurrence[] = []
  const seen = new Set<string>()

  for (const record of confirmed) {
    const anime = animeByID.get(record.anime_id)
    if (!anime || record.episode < 1) continue
    const airedAt = new Date(record.aired_at)
    if (Number.isNaN(airedAt.getTime())) continue
    const date = wallClockDate(airedAt, tz)
    const ms = date.getTime()
    if (ms < windowStart.getTime() || ms >= windowEnd.getTime()) continue
    const key = `${anime.id}:${record.episode}`
    if (seen.has(key)) continue
    seen.add(key)
    history.push({ anime, episode: record.episode, date })
  }

  const upcoming = upcomingAnimes
    .flatMap((anime) => projectOccurrences(anime, windowStart, windowEnd, tz))
    .filter((occurrence) => !seen.has(`${occurrence.anime.id}:${occurrence.episode}`))

  return [...history, ...upcoming]
}
