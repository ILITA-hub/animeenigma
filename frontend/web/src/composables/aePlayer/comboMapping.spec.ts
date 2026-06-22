import { describe, it, expect } from 'vitest'
import { providerToLegacyPlayer, comboToWatchCombo, watchComboToPartialCombo, comboToToken, tokenToCombo, type WtComboFields } from './comboMapping'

describe('providerToLegacyPlayer', () => {
  it('maps EN scraper ids to english', () => {
    for (const id of ['allanime', 'okru', 'animepahe', 'gogoanime', 'nineanime', 'animefever', 'miruro']) {
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

describe('comboToToken / tokenToCombo (WT room translation_id)', () => {
  it('round-trips a full combo (sub/en/team/allanime/wixmp)', () => {
    const fields: WtComboFields = {
      audio: 'sub',
      lang: 'en',
      team: 'SubsPlease',
      provider: 'allanime',
      server: 'wixmp',
    }
    expect(tokenToCombo(comboToToken(fields))).toEqual(fields)
  })

  it('round-trips a null team (dub/ru/null/kodik/empty server)', () => {
    const fields: WtComboFields = {
      audio: 'dub',
      lang: 'ru',
      team: null,
      provider: 'kodik',
      server: '',
    }
    expect(tokenToCombo(comboToToken(fields))).toEqual(fields)
  })

  it('returns null for non-JSON input', () => {
    expect(tokenToCombo('not-json')).toBeNull()
  })

  it('returns null when provider is not a string', () => {
    expect(tokenToCombo(JSON.stringify({ provider: 42, audio: 'sub', lang: 'en' }))).toBeNull()
  })

  it('coerces missing team to null and missing server to empty string', () => {
    expect(tokenToCombo(JSON.stringify({ provider: 'miruro', audio: 'sub', lang: 'ja' }))).toEqual({
      provider: 'miruro',
      audio: 'sub',
      lang: 'ja',
      team: null,
      server: '',
    })
  })
})
