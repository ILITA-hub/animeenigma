/**
 * Workstream hero-spotlight — v1.1-polish Phase 01 (HSB-V11-CC-02).
 *
 * Parity guard: every variant in the `SpotlightCard` discriminated union
 * MUST have a matching entry in `cardTokens`. Adding a 10th card type
 * without bumping the map will trip this test.
 *
 * The TypeScript `Record<SpotlightCardType, CardToken>` annotation in
 * tokens.ts catches the absence at compile time, but the runtime test
 * here also catches stale keys (keys present in cardTokens that no longer
 * exist in the union — those slip past tsc's exhaustiveness check).
 */

import { describe, it, expect } from 'vitest'
import {
  cardTokens,
  accentDotBg,
  type SpotlightAccent,
  type SpotlightIconName,
  type SpotlightCardType,
} from './tokens'

// Authoritative list of every card variant currently in SpotlightCard.
// Kept in sync with frontend/web/src/types/spotlight.ts:215..224. Updating
// the union without updating this list (and cardTokens) will fail tsc.
const EXPECTED_TYPES: readonly SpotlightCardType[] = [
  'anime_of_day',
  'random_tail',
  'personal_pick',
  'telegram_news',
  'latest_news',
  'platform_stats',
  'now_watching',
  'not_time_yet',
  'continue_watching_new',
] as const

const VALID_ACCENTS: readonly SpotlightAccent[] = [
  'cyan',
  'purple',
  'sky',
  'amber',
  'teal',
  'green',
] as const

const VALID_ICONS: readonly SpotlightIconName[] = [
  'telegram',
  'sparkles',
  'chart',
  'pulse',
  'clock',
  'play',
  'shuffle',
  'wrench',
  'lightning',
] as const

describe('cardTokens', () => {
  it('has exactly 9 entries — one per SpotlightCard variant', () => {
    expect(Object.keys(cardTokens)).toHaveLength(9)
  })

  it.each(EXPECTED_TYPES)('has a token for %s', (type) => {
    expect(cardTokens[type]).toBeDefined()
    expect(cardTokens[type].accent).toBeTypeOf('string')
    expect(cardTokens[type].kickerKey).toBeTypeOf('string')
    expect(cardTokens[type].icon).toBeTypeOf('string')
  })

  it('uses only valid accents', () => {
    for (const type of EXPECTED_TYPES) {
      expect(VALID_ACCENTS).toContain(cardTokens[type].accent)
    }
  })

  it('uses only valid icon names', () => {
    for (const type of EXPECTED_TYPES) {
      expect(VALID_ICONS).toContain(cardTokens[type].icon)
    }
  })

  it('kicker keys are all spotlight.* namespace', () => {
    for (const type of EXPECTED_TYPES) {
      expect(cardTokens[type].kickerKey).toMatch(/^spotlight\./)
    }
  })

  it('has no stale keys outside SpotlightCard["type"]', () => {
    const actualKeys = Object.keys(cardTokens) as SpotlightCardType[]
    for (const key of actualKeys) {
      expect(EXPECTED_TYPES).toContain(key)
    }
  })
})

describe('accentDotBg', () => {
  it('has an entry for every valid accent', () => {
    for (const accent of VALID_ACCENTS) {
      expect(accentDotBg[accent]).toBeDefined()
      expect(accentDotBg[accent]).toBeTypeOf('string')
    }
  })

  it('every entry includes both background and text class', () => {
    for (const accent of VALID_ACCENTS) {
      expect(accentDotBg[accent]).toMatch(/bg-/)
      expect(accentDotBg[accent]).toMatch(/text-/)
    }
  })
})
