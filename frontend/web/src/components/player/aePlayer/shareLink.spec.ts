import { describe, it, expect } from 'vitest'
import { buildShareUrl } from './shareLink'
import type { Combo } from '@/types/aePlayer'

const ORIGIN = 'https://animeenigma.org'

// Builds a full Combo (as the player actually passes state.combo.value) so we
// can prove `server` never leaks into the URL despite being present on the combo.
function combo(over: Partial<Combo> = {}): Combo {
  return { audio: 'sub', lang: 'en', provider: 'allanime', server: 'rotating-server', team: null, ...over }
}

describe('buildShareUrl', () => {
  it('encodes the full combo, episode, and floored timestamp — but never server', () => {
    const url = buildShareUrl({
      origin: ORIGIN,
      animeId: 'abc-123',
      combo: combo({ provider: 'allanime', team: 'Jimaku' }),
      episode: 7,
      timeSec: 512.83,
    })
    const u = new URL(url)
    expect(u.pathname).toBe('/anime/abc-123')
    expect(u.searchParams.get('provider')).toBe('allanime')
    expect(u.searchParams.get('team')).toBe('Jimaku')
    expect(u.searchParams.get('audio')).toBe('sub')
    expect(u.searchParams.get('lang')).toBe('en')
    expect(u.searchParams.get('episode')).toBe('7')
    expect(u.searchParams.get('t')).toBe('512') // floored
    expect(u.searchParams.has('server')).toBe(false)
  })

  it('omits team when there is no provider', () => {
    const url = buildShareUrl({
      origin: ORIGIN,
      animeId: 'x',
      combo: combo({ audio: 'dub', lang: 'ru', provider: '', team: 'Ghost' }),
      episode: 1,
      timeSec: 0,
    })
    const u = new URL(url)
    expect(u.searchParams.has('provider')).toBe(false)
    expect(u.searchParams.has('team')).toBe(false)
    expect(u.searchParams.get('audio')).toBe('dub')
    expect(u.searchParams.get('lang')).toBe('ru')
  })

  it('omits episode and timestamp when non-positive', () => {
    const url = buildShareUrl({
      origin: ORIGIN,
      animeId: 'x',
      combo: combo({ lang: 'ja', provider: 'kodik' }),
      episode: 0,
      timeSec: 0,
    })
    const u = new URL(url)
    expect(u.searchParams.has('episode')).toBe(false)
    expect(u.searchParams.has('t')).toBe(false)
    expect(u.searchParams.get('provider')).toBe('kodik')
  })

  it('URL-encodes team titles with spaces', () => {
    const url = buildShareUrl({
      origin: ORIGIN,
      animeId: 'x',
      combo: combo({ lang: 'ru', provider: 'kodik', team: 'Studio Band' }),
      episode: 3,
      timeSec: 60,
    })
    expect(new URL(url).searchParams.get('team')).toBe('Studio Band')
    expect(url).toContain('team=Studio+Band')
  })
})
