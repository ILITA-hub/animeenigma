import { describe, it, expect } from 'vitest'
import { resolveInitialPlayerPref } from './animePlayerPrefs'

// Pure helper that decides whether the watch page should boot into the
// "Classic Kodik" iframe fallback instead of the default AePlayer.
//
// Decision order (most → least authoritative):
//   1. New key `classic_kodik_selected` present → use it verbatim.
//   2. Legacy `unified_player_selected` truthy → AePlayer (classicKodik:false).
//   3. Legacy `preferred_video_provider === 'kodik'` (and NOT unified) →
//      classicKodik:true (the user last watched on the iframe Kodik surface).
//   4. Any other / deleted-provider value, or nothing → AePlayer default.
describe('resolveInitialPlayerPref', () => {
  it('honors the new classic_kodik_selected=true key', () => {
    expect(resolveInitialPlayerPref({ classic_kodik_selected: 'true' })).toEqual({ classicKodik: true })
  })

  it('honors the new classic_kodik_selected=false key (beats legacy kodik)', () => {
    expect(
      resolveInitialPlayerPref({ classic_kodik_selected: 'false', preferred_video_provider: 'kodik' }),
    ).toEqual({ classicKodik: false })
  })

  it('maps legacy preferred_video_provider=kodik → classicKodik:true', () => {
    expect(resolveInitialPlayerPref({ preferred_video_provider: 'kodik' })).toEqual({ classicKodik: true })
  })

  it('legacy unified_player_selected=1 → AePlayer even with kodik provider', () => {
    expect(
      resolveInitialPlayerPref({ unified_player_selected: '1', preferred_video_provider: 'kodik' }),
    ).toEqual({ classicKodik: false })
  })

  it('legacy unified_player_selected=true → AePlayer', () => {
    expect(resolveInitialPlayerPref({ unified_player_selected: 'true' })).toEqual({ classicKodik: false })
  })

  it('retired EN provider value (ourenglish) → AePlayer default', () => {
    expect(resolveInitialPlayerPref({ preferred_video_provider: 'ourenglish' })).toEqual({ classicKodik: false })
  })

  it('retired provider value (animelib) → AePlayer default', () => {
    expect(resolveInitialPlayerPref({ preferred_video_provider: 'animelib' })).toEqual({ classicKodik: false })
  })

  it('empty storage → AePlayer default', () => {
    expect(resolveInitialPlayerPref({})).toEqual({ classicKodik: false })
  })

  it('null values → AePlayer default', () => {
    expect(
      resolveInitialPlayerPref({
        classic_kodik_selected: null,
        unified_player_selected: null,
        preferred_video_provider: null,
      }),
    ).toEqual({ classicKodik: false })
  })

  it('a garbage classic_kodik_selected value is not truthy → AePlayer', () => {
    expect(resolveInitialPlayerPref({ classic_kodik_selected: 'nope' })).toEqual({ classicKodik: false })
  })
})
