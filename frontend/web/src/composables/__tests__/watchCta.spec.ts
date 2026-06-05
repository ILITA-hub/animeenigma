import { describe, it, expect } from 'vitest'
import { computeWatchCta, type WatchCtaInput } from '../watchCta'

// Group 1 — the anime-page primary play-button label/action.
//
// Governing principle (see design 2026-06-05): the VERB is driven by actual
// episode progress (watch_progress → lastWatched), and list status only
// disambiguates the fully-watched terminal state:
//   not-completed + full  → mark-watched
//   completed     + full  → rewatch
// `full` requires total > 0 (unknown-total shows can never be classified full).

function input(over: Partial<WatchCtaInput> = {}): WatchCtaInput {
  return {
    isAuthenticated: true,
    lastWatched: 0,
    totalEpisodes: 12,
    listStatus: null,
    ...over,
  }
}

describe('computeWatchCta — authenticated, in progress', () => {
  it('#1 not in list, nothing watched → watch', () => {
    const cta = computeWatchCta(input({ listStatus: null, lastWatched: 0 }))
    expect(cta.action).toBe('watch')
    expect(cta.startEpisode).toBe(1)
    expect(cta.labelKey).toBe('anime.watchNow')
  })

  it('#2 status=watching, nothing watched → watch (≠completed)', () => {
    const cta = computeWatchCta(input({ listStatus: 'watching', lastWatched: 0 }))
    expect(cta.action).toBe('watch')
    expect(cta.startEpisode).toBe(1)
  })

  it('#3 status=completed, nothing watched → start-from-1', () => {
    const cta = computeWatchCta(input({ listStatus: 'completed', lastWatched: 0 }))
    expect(cta.action).toBe('start-from-1')
    expect(cta.startEpisode).toBe(1)
    expect(cta.labelKey).toBe('anime.startFromEp1')
  })

  it('#4 partial progress → continue from next episode', () => {
    const cta = computeWatchCta(input({ listStatus: 'watching', lastWatched: 5 }))
    expect(cta.action).toBe('continue')
    expect(cta.startEpisode).toBe(6)
    expect(cta.labelKey).toBe('anime.continueEp')
    expect(cta.labelParams).toEqual({ n: 6 })
  })

  it('#5 status=completed but progress partial → continue (Case 3b, real progress wins)', () => {
    const cta = computeWatchCta(input({ listStatus: 'completed', lastWatched: 5 }))
    expect(cta.action).toBe('continue')
    expect(cta.startEpisode).toBe(6)
  })
})

describe('computeWatchCta — authenticated, fully watched terminal', () => {
  it('#6 watched all, status≠completed → mark-watched', () => {
    const cta = computeWatchCta(input({ listStatus: 'watching', lastWatched: 12 }))
    expect(cta.action).toBe('mark-watched')
    expect(cta.labelKey).toBe('anime.markAsWatched')
  })

  it('#7 watched all, status=completed → rewatch', () => {
    const cta = computeWatchCta(input({ listStatus: 'completed', lastWatched: 12 }))
    expect(cta.action).toBe('rewatch')
    expect(cta.labelKey).toBe('anime.resume.rewatch')
  })

  it('#8 watched all, not in list → mark-watched (click creates the entry)', () => {
    const cta = computeWatchCta(input({ listStatus: null, lastWatched: 12 }))
    expect(cta.action).toBe('mark-watched')
  })

  it('#9 lastWatched > total (data anomaly) → clamp to total, treat as full', () => {
    const cta = computeWatchCta(input({ listStatus: 'completed', lastWatched: 14, totalEpisodes: 12 }))
    expect(cta.action).toBe('rewatch')
  })

  it('#12 watched all but status=dropped → mark-watched (≠completed)', () => {
    const cta = computeWatchCta(input({ listStatus: 'dropped', lastWatched: 12 }))
    expect(cta.action).toBe('mark-watched')
  })
})

describe('computeWatchCta — unknown total (episodes_count = 0)', () => {
  it('#10 unknown total can never be "full" → continue regardless of how many watched', () => {
    const cta = computeWatchCta(input({ listStatus: 'watching', lastWatched: 20, totalEpisodes: 0 }))
    expect(cta.action).toBe('continue')
    expect(cta.startEpisode).toBe(21)
  })

  it('#11 unknown total, nothing watched, status=completed → start-from-1', () => {
    const cta = computeWatchCta(input({ listStatus: 'completed', lastWatched: 0, totalEpisodes: 0 }))
    expect(cta.action).toBe('start-from-1')
    expect(cta.startEpisode).toBe(1)
  })

  it('unknown total, nothing watched, not in list → watch', () => {
    const cta = computeWatchCta(input({ listStatus: null, lastWatched: 0, totalEpisodes: 0 }))
    expect(cta.action).toBe('watch')
  })
})

describe('computeWatchCta — status label does not change the verb', () => {
  it('#13 on_hold + partial → continue', () => {
    const cta = computeWatchCta(input({ listStatus: 'on_hold', lastWatched: 5 }))
    expect(cta.action).toBe('continue')
    expect(cta.startEpisode).toBe(6)
  })
})

describe('computeWatchCta — anonymous (no list, never mark/rewatch)', () => {
  it('#14 anon, nothing watched → watch', () => {
    const cta = computeWatchCta(input({ isAuthenticated: false, listStatus: null, lastWatched: 0 }))
    expect(cta.action).toBe('watch')
    expect(cta.startEpisode).toBe(1)
  })

  it('#15 anon, partial → continue', () => {
    const cta = computeWatchCta(input({ isAuthenticated: false, listStatus: null, lastWatched: 5 }))
    expect(cta.action).toBe('continue')
    expect(cta.startEpisode).toBe(6)
  })

  it('#16 anon, watched all → never mark-watched/rewatch (no account) → watch', () => {
    const cta = computeWatchCta(input({ isAuthenticated: false, listStatus: null, lastWatched: 12 }))
    expect(cta.action).not.toBe('mark-watched')
    expect(cta.action).not.toBe('rewatch')
    expect(cta.action).toBe('watch')
  })
})
