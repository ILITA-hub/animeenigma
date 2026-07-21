// frontend/web/src/composables/schedule/__tests__/projection.spec.ts
import { describe, it, expect } from 'vitest'
import { projectOccurrences, occurrencesInRange } from '../projection'
import type { ScheduleAnime } from '../types'

function anime(over: Partial<ScheduleAnime> = {}): ScheduleAnime {
  return {
    id: 'a1',
    name: 'Test',
    next_episode_at: '2026-06-08T17:00:00Z',
    episodes_aired: 9,
    episodes_count: 12,
    ...over,
  }
}

const d = (s: string) => new Date(s)

describe('projectOccurrences', () => {
  it('returns the next episode at the anchor date', () => {
    const occ = projectOccurrences(anime(), d('2026-06-08T00:00:00Z'), d('2026-06-09T00:00:00Z'))
    expect(occ).toHaveLength(1)
    expect(occ[0].episode).toBe(10)
    expect(occ[0].date.toISOString()).toBe('2026-06-08T17:00:00.000Z')
  })

  it('projects future weeks with incrementing episode numbers', () => {
    const occ = projectOccurrences(anime(), d('2026-06-08T00:00:00Z'), d('2026-06-23T00:00:00Z'))
    expect(occ.map(o => o.episode)).toEqual([10, 11, 12])
  })

  it('caps projection at episodes_count (no episodes past the finale)', () => {
    const occ = projectOccurrences(anime({ episodes_aired: 11, episodes_count: 12 }), d('2026-06-08T00:00:00Z'), d('2026-07-06T00:00:00Z'))
    expect(occ.map(o => o.episode)).toEqual([12])
  })

  it('does not invent past weekly episodes before a future next-airing anchor', () => {
    const occ = projectOccurrences(anime(), d('2026-05-25T00:00:00Z'), d('2026-06-09T00:00:00Z'))
    expect(occ.map(o => o.episode)).toEqual([10])
  })

  it('keeps a hiatus title out of weeks before its confirmed next airing', () => {
    const occ = projectOccurrences(
      anime({ next_episode_at: '2026-08-12T13:00:00Z', episodes_aired: 11, episodes_count: 19 }),
      d('2026-07-20T00:00:00Z'),
      d('2026-07-27T00:00:00Z'),
    )
    expect(occ).toEqual([])
  })

  it('projects forward from a recently stale anchor', () => {
    const occ = projectOccurrences(
      anime({ next_episode_at: '2026-07-13T17:00:00Z', episodes_aired: 9 }),
      d('2026-07-20T00:00:00Z'),
      d('2026-07-27T00:00:00Z'),
    )
    expect(occ.map(o => o.episode)).toEqual([11])
  })

  it('treats episodes_count <= 0 as unknown (no upper cap)', () => {
    const occ = projectOccurrences(anime({ episodes_count: 0 }), d('2026-06-08T00:00:00Z'), d('2026-06-23T00:00:00Z'))
    expect(occ.map(o => o.episode)).toEqual([10, 11, 12])
  })

  it('returns [] when next_episode_at is missing or invalid', () => {
    expect(projectOccurrences(anime({ next_episode_at: null }), d('2026-06-01T00:00:00Z'), d('2026-07-01T00:00:00Z'))).toEqual([])
    expect(projectOccurrences(anime({ next_episode_at: 'not-a-date' }), d('2026-06-01T00:00:00Z'), d('2026-07-01T00:00:00Z'))).toEqual([])
  })

  it('window end is exclusive', () => {
    const occ = projectOccurrences(anime(), d('2026-06-08T00:00:00Z'), d('2026-06-08T17:00:00Z'))
    expect(occ).toHaveLength(0)
  })

  describe('with a display timezone', () => {
    // Wall-clock windows, like the calendar grid builds them (env-TZ-independent).
    const wallDay = (day: number) => new Date(2026, 5, day) // June 2026

    it('shifts displayed time into the zone (17:00 UTC → 02:00 Tokyo next day)', () => {
      const occ = projectOccurrences(anime(), wallDay(8), wallDay(15), 'Asia/Tokyo')
      expect(occ).toHaveLength(1)
      expect(occ[0].date.getHours()).toBe(2)
      expect(occ[0].date.getDate()).toBe(9) // crossed midnight — lands on the next calendar day
    })

    it('UTC+3: 17:00 UTC reads as 20:00 same day', () => {
      const occ = projectOccurrences(anime(), wallDay(8), wallDay(9), 'Europe/Moscow')
      expect(occ).toHaveLength(1)
      expect(occ[0].date.getHours()).toBe(20)
      expect(occ[0].date.getDate()).toBe(8)
    })

    it('windowing respects the zone day-shift (Tokyo occurrence excluded from the UTC-day window)', () => {
      // In Tokyo the airing is June 9 02:00, so a window covering only June 8 misses it.
      const occ = projectOccurrences(anime(), wallDay(8), wallDay(9), 'Asia/Tokyo')
      expect(occ).toHaveLength(0)
    })
  })
})

describe('occurrencesInRange', () => {
  it('flattens occurrences across all anime', () => {
    const list = [
      anime({ id: 'a', next_episode_at: '2026-06-08T17:00:00Z' }),
      anime({ id: 'b', next_episode_at: '2026-06-10T20:00:00Z' }),
    ]
    const occ = occurrencesInRange(list, d('2026-06-08T00:00:00Z'), d('2026-06-12T00:00:00Z'))
    expect(occ.map(o => o.anime.id).sort()).toEqual(['a', 'b'])
  })
})
