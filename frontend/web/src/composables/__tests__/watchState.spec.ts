import { describe, it, expect } from 'vitest'
import {
  resolveStartEpisode,
  resolveResumeState,
  type ResumeStateInput,
} from '../watchState'

// ── resolveStartEpisode — the ONLY start-episode authority ──────────────────
describe('resolveStartEpisode', () => {
  it('first-time (0 watched) → ep 1', () => {
    expect(resolveStartEpisode(0, 12)).toBe(1)
  })
  it('watching (last < total) → last + 1', () => {
    expect(resolveStartEpisode(5, 12)).toBe(6)
  })
  it('caught up (last == total) → last (re-load the final ep)', () => {
    expect(resolveStartEpisode(12, 12)).toBe(12)
  })
  it('clamps a stale last > total to total', () => {
    expect(resolveStartEpisode(14, 12)).toBe(12)
  })
  it('unknown total (0) treats any positive last as watching', () => {
    expect(resolveStartEpisode(20, 0)).toBe(21)
  })
  it('never returns < 1', () => {
    expect(resolveStartEpisode(-3, 12)).toBe(1)
  })
})

// ── resolveResumeState — CTA verb (ported from computeWatchCta, all 16) ──────
function rs(over: Partial<ResumeStateInput> = {}): ResumeStateInput {
  return {
    lastWatched: 0,
    totalEpisodes: 12,
    episodesAired: 12,
    loadedEpisodes: 0,
    status: 'released',
    nextEpisodeAt: undefined,
    listStatus: null,
    isAuthenticated: true,
    nowMs: 1_000,
    ...over,
  }
}

describe('resolveResumeState.cta — authenticated, in progress', () => {
  it('#1 not in list, nothing watched → watch', () => {
    const { cta } = resolveResumeState(rs({ listStatus: null, lastWatched: 0 }))
    expect(cta.action).toBe('watch')
    expect(cta.startEpisode).toBe(1)
    expect(cta.labelKey).toBe('anime.watchNow')
  })
  it('#2 status=watching, nothing watched → watch', () => {
    expect(resolveResumeState(rs({ listStatus: 'watching', lastWatched: 0 })).cta.action).toBe('watch')
  })
  it('#3 status=completed, nothing watched → start-from-1', () => {
    const { cta } = resolveResumeState(rs({ listStatus: 'completed', lastWatched: 0 }))
    expect(cta.action).toBe('start-from-1')
    expect(cta.labelKey).toBe('anime.startFromEp1')
  })
  it('#4 partial progress → continue from next episode', () => {
    const { cta } = resolveResumeState(rs({ listStatus: 'watching', lastWatched: 5 }))
    expect(cta.action).toBe('continue')
    expect(cta.startEpisode).toBe(6)
    expect(cta.labelKey).toBe('anime.continueEp')
    expect(cta.labelParams).toEqual({ n: 6 })
  })
  it('#5 completed list but partial progress → continue (real progress wins)', () => {
    expect(resolveResumeState(rs({ listStatus: 'completed', lastWatched: 5 })).cta.action).toBe('continue')
  })
})

describe('resolveResumeState.cta — fully watched terminal', () => {
  it('#6 watched all, status≠completed → mark-watched', () => {
    const { cta } = resolveResumeState(rs({ listStatus: 'watching', lastWatched: 12 }))
    expect(cta.action).toBe('mark-watched')
    expect(cta.labelKey).toBe('anime.markAsWatched')
  })
  it('#7 watched all, status=completed → rewatch', () => {
    const { cta } = resolveResumeState(rs({ listStatus: 'completed', lastWatched: 12 }))
    expect(cta.action).toBe('rewatch')
    expect(cta.labelKey).toBe('anime.resume.rewatch')
  })
  it('#8 watched all, not in list → mark-watched', () => {
    expect(resolveResumeState(rs({ listStatus: null, lastWatched: 12 })).cta.action).toBe('mark-watched')
  })
  it('#9 last > total (anomaly) → clamp, treat as full → rewatch', () => {
    expect(resolveResumeState(rs({ listStatus: 'completed', lastWatched: 14 })).cta.action).toBe('rewatch')
  })
  it('#12 watched all but status=dropped → mark-watched', () => {
    expect(resolveResumeState(rs({ listStatus: 'dropped', lastWatched: 12 })).cta.action).toBe('mark-watched')
  })
})

describe('resolveResumeState.cta — unknown total', () => {
  it('#10 unknown total never "full" → continue', () => {
    const { cta } = resolveResumeState(rs({ listStatus: 'watching', lastWatched: 20, totalEpisodes: 0, episodesAired: 0 }))
    expect(cta.action).toBe('continue')
    expect(cta.startEpisode).toBe(21)
  })
  it('#11 unknown total, nothing watched, completed → start-from-1', () => {
    expect(resolveResumeState(rs({ listStatus: 'completed', lastWatched: 0, totalEpisodes: 0, episodesAired: 0 })).cta.action).toBe('start-from-1')
  })
})

describe('resolveResumeState.cta — anonymous', () => {
  it('#14 anon, nothing watched → watch', () => {
    expect(resolveResumeState(rs({ isAuthenticated: false, listStatus: null, lastWatched: 0 })).cta.action).toBe('watch')
  })
  it('#15 anon, partial → continue', () => {
    expect(resolveResumeState(rs({ isAuthenticated: false, listStatus: null, lastWatched: 5 })).cta.action).toBe('continue')
  })
  it('#16 anon, watched all → watch (never mark/rewatch)', () => {
    const { cta } = resolveResumeState(rs({ isAuthenticated: false, listStatus: null, lastWatched: 12 }))
    expect(cta.action).toBe('watch')
  })
})

// ── resolveResumeState — banner (collapsed 5 kinds → 3) ──────────────────────
describe('resolveResumeState.banner', () => {
  it('first-time → none', () => {
    expect(resolveResumeState(rs({ lastWatched: 0 })).banner).toEqual({ kind: 'none' })
  })
  it('finished (last >= total) → none', () => {
    expect(resolveResumeState(rs({ lastWatched: 12 })).banner).toEqual({ kind: 'none' })
  })
  it('watching (next aired) → just-finished{episode:last}', () => {
    const { banner } = resolveResumeState(rs({ lastWatched: 5, status: 'released' }))
    expect(banner).toEqual({ kind: 'just-finished', episode: 5 })
  })
  it('loadedEpisodes overrides lagging episodesAired → just-finished', () => {
    const { banner } = resolveResumeState(rs({
      lastWatched: 5, status: 'ongoing', episodesAired: 5, loadedEpisodes: 6,
    }))
    expect(banner).toEqual({ kind: 'just-finished', episode: 5 })
  })
  it('ongoing, next not aired, future ETA → next-unavailable with etaLabel', () => {
    const { banner } = resolveResumeState(rs({
      lastWatched: 5, totalEpisodes: 12, status: 'ongoing', episodesAired: 5, loadedEpisodes: 0,
      nextEpisodeAt: new Date(10_000).toISOString(), nowMs: 1_000,
      formatEta: () => 'in 2 days',
    }))
    expect(banner).toEqual({ kind: 'next-unavailable', episode: 6, etaLabel: 'in 2 days' })
  })
  it('ongoing, next air time PAST (aired-not-loaded) → next-unavailable, no eta', () => {
    const { banner } = resolveResumeState(rs({
      lastWatched: 5, totalEpisodes: 12, status: 'ongoing', episodesAired: 5, loadedEpisodes: 0,
      nextEpisodeAt: new Date(500).toISOString(), nowMs: 1_000,
      formatEta: () => 'should-not-be-used',
    }))
    expect(banner).toEqual({ kind: 'next-unavailable', episode: 6 })
  })
})
