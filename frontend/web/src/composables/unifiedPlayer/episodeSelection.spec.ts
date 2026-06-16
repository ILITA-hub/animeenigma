import { describe, it, expect } from 'vitest'
import { pickEpisodeForProvider } from './episodeSelection'
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
