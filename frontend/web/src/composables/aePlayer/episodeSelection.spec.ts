import { describe, it, expect } from 'vitest'
import { pickEpisodeForProvider, providerMissesTargetEpisode } from './episodeSelection'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

const list = (nums: number[]): EpisodeOption[] =>
  nums.map((n) => ({ key: n, label: n, number: n }))

describe('pickEpisodeForProvider', () => {
  it('keeps the same episode number across a provider switch (the core ask)', () => {
    const eps = list([1, 2, 3, 12, 13, 24])
    const prev: EpisodeOption = { key: 'gogo-12', label: 12, number: 12 }
    const picked = pickEpisodeForProvider(eps, 12, prev)
    expect(picked?.number).toBe(12)
    // It returns the NEW provider's episode object (its own opaque key), not prev.
    expect(picked?.key).toBe(12)
  })

  it('does NOT snap back to episode 1 when the exact number is missing', () => {
    // New provider only has 1..10; user was on 12.
    const eps = list([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])
    const picked = pickEpisodeForProvider(eps, 12, null)
    expect(picked?.number).toBe(10) // nearest ≤ target, not 1
  })

  it('lands on the highest episode at or below the target', () => {
    const eps = list([1, 5, 9, 20])
    expect(pickEpisodeForProvider(eps, 12, null)?.number).toBe(9)
  })

  it('falls back to the first episode for an offset list that starts past target', () => {
    const eps = list([13, 14, 15]) // a second-season list
    expect(pickEpisodeForProvider(eps, 12, null)?.number).toBe(13)
  })

  it('keeps the previous selection when the new list is empty', () => {
    const prev: EpisodeOption = { key: 7, label: 7, number: 7 }
    expect(pickEpisodeForProvider([], 7, prev)).toBe(prev)
  })

  it('returns null only when there is nothing to play', () => {
    expect(pickEpisodeForProvider([], 7, null)).toBeNull()
  })

  it('preserves a non-first exact match (regression for the reported bug)', () => {
    const eps = list([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14])
    expect(pickEpisodeForProvider(eps, 12, null)?.number).toBe(12)
  })
})

describe('providerMissesTargetEpisode', () => {
  it('partial late-only library, first-timer wants ep 1 → misses (the Frieren ep 27 bug)', () => {
    // ae holds only ep 27; a first-time viewer targets ep 1.
    expect(providerMissesTargetEpisode(list([27]), 1)).toBe(true)
  })

  it('late-only range, target below it → misses (would snap UP otherwise)', () => {
    expect(providerMissesTargetEpisode(list([10, 11, 12]), 1)).toBe(true)
  })

  it('hole in the middle of an otherwise-covering list → misses', () => {
    expect(providerMissesTargetEpisode(list([1, 2, 4, 5]), 3)).toBe(true)
  })

  it('source has the exact episode → not missing', () => {
    expect(providerMissesTargetEpisode(list([1, 2, 3, 12]), 12)).toBe(false)
  })

  it('target ABOVE the newest episode (not aired here yet) → kept, not missing', () => {
    // Caught up on an ongoing anime: next episode (13) is above what any source has.
    expect(providerMissesTargetEpisode(list([1, 2, 3, 4, 5]), 13)).toBe(false)
  })

  it('empty list → not missing (caller handles no-episodes separately)', () => {
    expect(providerMissesTargetEpisode([], 1)).toBe(false)
  })
})
