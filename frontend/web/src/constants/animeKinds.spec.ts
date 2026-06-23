import { describe, it, expect } from 'vitest'
import { ANIME_KINDS } from './animeKinds'

describe('ANIME_KINDS', () => {
  it('carries exactly the 9 canonical Shikimori kinds in order', () => {
    expect([...ANIME_KINDS]).toEqual([
      'tv', 'movie', 'ova', 'ona', 'special', 'tv_special', 'music', 'cm', 'pv',
    ])
  })

  it('has no duplicates', () => {
    expect(new Set(ANIME_KINDS).size).toBe(ANIME_KINDS.length)
  })
})
