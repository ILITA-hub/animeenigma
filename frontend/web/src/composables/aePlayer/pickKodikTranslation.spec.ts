import { describe, it, expect } from 'vitest'
import { pickKodikTranslation } from './pickKodikTranslation'

const sub = (title: string) => ({ title, type: 'subtitles' })
const dub = (title: string) => ({ title, type: 'voice' })

describe('pickKodikTranslation', () => {
  it('returns undefined for an empty list', () => {
    expect(pickKodikTranslation([], { team: null, audio: 'sub' })).toBeUndefined()
  })

  it('honors an explicit team pick when it exists', () => {
    const ts = [dub('AniLibria'), sub('AniDUB-sub')]
    expect(pickKodikTranslation(ts, { team: 'AniDUB-sub', audio: 'sub' })?.title).toBe('AniDUB-sub')
  })

  it('SUB request does NOT auto-select a leading DUB team (the bug)', () => {
    // translations[0] is a DUB voice — must be skipped for a SUB selection.
    const ts = [dub('AniLibria'), dub('AnimeVost'), sub('Subs-Team')]
    expect(pickKodikTranslation(ts, { team: null, audio: 'sub' })?.title).toBe('Subs-Team')
  })

  it('DUB request picks the first DUB team, not a leading SUB team', () => {
    const ts = [sub('Subs-Team'), dub('AniLibria')]
    expect(pickKodikTranslation(ts, { team: null, audio: 'dub' })?.title).toBe('AniLibria')
  })

  it('falls back to the first translation when none match the audio', () => {
    // DUB-only title, user asked for SUB → still plays something rather than crash.
    const ts = [dub('AniLibria'), dub('AnimeVost')]
    expect(pickKodikTranslation(ts, { team: null, audio: 'sub' })?.title).toBe('AniLibria')
  })

  it('an explicit pick beats the audio fallback even across categories', () => {
    // A pinned team is the user's explicit choice — honored even if its category
    // differs from the audio facet (the facet/team are reconciled by the caller).
    const ts = [sub('Subs-Team'), dub('AniLibria')]
    expect(pickKodikTranslation(ts, { team: 'AniLibria', audio: 'sub' })?.title).toBe('AniLibria')
  })
})
