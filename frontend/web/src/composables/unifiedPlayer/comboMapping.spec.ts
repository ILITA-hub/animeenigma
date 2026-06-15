import { describe, it, expect } from 'vitest'
import { providerToLegacyPlayer, comboToWatchCombo, watchComboToPartialCombo } from './comboMapping'

describe('providerToLegacyPlayer', () => {
  it('maps EN scraper ids to english', () => {
    for (const id of ['allanime', 'animepahe', 'gogoanime', 'nineanime', 'animefever', 'miruro']) {
      expect(providerToLegacyPlayer(id)).toBe('english')
    }
  })
  it('maps 1:1 providers', () => {
    expect(providerToLegacyPlayer('kodik')).toBe('kodik')
    expect(providerToLegacyPlayer('raw')).toBe('raw')
    expect(providerToLegacyPlayer('ae')).toBe('ae')
    expect(providerToLegacyPlayer('18anime')).toBe('hanime')
    expect(providerToLegacyPlayer('animelib')).toBe('animelib')
  })
  it('returns null for unknown', () => {
    expect(providerToLegacyPlayer('nope')).toBeNull()
  })
})

describe('comboToWatchCombo', () => {
  it('maps a unified combo to a legacy WatchCombo (team -> translation_title)', () => {
    expect(comboToWatchCombo({ audio: 'dub', lang: 'ru', provider: 'kodik', server: '', team: 'AniLibria' }))
      .toEqual({ player: 'kodik', language: 'ru', watch_type: 'dub', translation_id: '', translation_title: 'AniLibria' })
  })
  it('maps ja-lang raw correctly', () => {
    expect(comboToWatchCombo({ audio: 'sub', lang: 'ja', provider: 'raw', server: '', team: null }))
      .toEqual({ player: 'raw', language: 'ja', watch_type: 'sub', translation_id: '', translation_title: '' })
  })
  it('returns null when provider has no legacy mapping', () => {
    expect(comboToWatchCombo({ audio: 'sub', lang: 'en', provider: 'nope', server: '', team: null })).toBeNull()
  })
})

describe('watchComboToPartialCombo', () => {
  it('maps a resolved combo back to unified audio/lang/team (provider left to caller)', () => {
    expect(watchComboToPartialCombo({ player: 'kodik', language: 'ru', watch_type: 'dub', translation_id: '1', translation_title: 'AniLibria', tier: 'per_anime', tier_number: 1 }))
      .toEqual({ audio: 'dub', lang: 'ru', team: 'AniLibria' })
  })
  it('maps english resolved combo to en lang, no team', () => {
    expect(watchComboToPartialCombo({ player: 'english', language: 'en', watch_type: 'sub', translation_id: '', translation_title: '', tier: 'community', tier_number: 3 }))
      .toEqual({ audio: 'sub', lang: 'en', team: null })
  })
})
