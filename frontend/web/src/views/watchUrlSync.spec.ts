import { describe, expect, it } from 'vitest'
import type { LocationQuery } from 'vue-router'
import { nextWatchQuery, watchQueryChanged } from './watchUrlSync'

const q = (o: Record<string, string>): LocationQuery => o as LocationQuery

describe('nextWatchQuery', () => {
  it('writes provider/team/episode for a user-pinned source', () => {
    expect(nextWatchQuery(q({}), { provider: 'kodik', team: 'AniLibria', episode: 12 }))
      .toEqual({ provider: 'kodik', team: 'AniLibria', episode: '12' })
  })

  it('REMOVES provider/team for an auto/smart-default source (the revert-bug guard)', () => {
    // Empty provider/team ⇒ the player auto-picked BEST; the URL must not pin it,
    // so a plain reload re-runs the deterministic BEST default.
    expect(nextWatchQuery(q({ provider: 'kodik', team: 'AniLibria', episode: '5' }), { provider: '', team: '', episode: 5 }))
      .toEqual({ episode: '5' })
  })

  it('removes episode when it is 0/unknown', () => {
    expect(nextWatchQuery(q({ episode: '3' }), { provider: '', team: '', episode: 0 }))
      .toEqual({})
  })

  it('preserves unrelated query params', () => {
    expect(nextWatchQuery(q({ utm: 'tg', episode: '1' }), { provider: 'gogoanime', team: '', episode: 2 }))
      .toEqual({ utm: 'tg', provider: 'gogoanime', episode: '2' })
  })
})

describe('watchQueryChanged', () => {
  it('is false when none of the synced params change', () => {
    const cur = q({ provider: 'kodik', episode: '12' })
    expect(watchQueryChanged(cur, nextWatchQuery(cur, { provider: 'kodik', team: '', episode: 12 }))).toBe(false)
  })

  it('is true when a synced param changes', () => {
    const cur = q({ provider: 'kodik', episode: '12' })
    expect(watchQueryChanged(cur, nextWatchQuery(cur, { provider: '', team: '', episode: 12 }))).toBe(true)
  })
})
