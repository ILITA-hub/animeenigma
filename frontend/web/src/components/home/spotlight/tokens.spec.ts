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
  type SpotlightAccent,
  type SpotlightIconName,
  type SpotlightCardType,
} from './tokens'

// Authoritative list of every card variant currently in SpotlightCard.
// Kept in sync with frontend/web/src/types/spotlight.ts:246..257. Updating
// the union without updating this list (and cardTokens) will fail tsc.
const EXPECTED_TYPES: readonly SpotlightCardType[] = [
  'featured',
  'random_tail',
  'personal_pick',
  'telegram_news',
  'latest_news',
  'platform_stats',
  'now_watching',
  'not_time_yet',
  'continue_watching_new',
  'curated',
  'daily_fanfic',
  'daily_review',
  'upcoming_for_you',
] as const

// Brand triad only (DS alignment A-1, 2026-06-10, user-approved).
const VALID_ACCENTS: readonly SpotlightAccent[] = [
  'cyan',
  'pink',
  'violet',
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
  'star',
  'quote',
] as const

describe('cardTokens', () => {
  it('has exactly 13 entries — one per SpotlightCard variant', () => {
    expect(Object.keys(cardTokens)).toHaveLength(13)
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

describe('cardTokens — A-1 brand-triad mapping (DS alignment 2026-06-10)', () => {
  it('content-core cards are cyan', () => {
    expect(cardTokens.featured.accent).toBe('cyan')
    expect(cardTokens.personal_pick.accent).toBe('cyan')
    expect(cardTokens.platform_stats.accent).toBe('cyan')
  })

  it('live/personal cards are pink', () => {
    expect(cardTokens.now_watching.accent).toBe('pink')
    expect(cardTokens.continue_watching_new.accent).toBe('pink')
  })

  it('meta/service cards are violet', () => {
    expect(cardTokens.random_tail.accent).toBe('violet')
    expect(cardTokens.latest_news.accent).toBe('violet')
    expect(cardTokens.telegram_news.accent).toBe('violet')
    expect(cardTokens.not_time_yet.accent).toBe('violet')
  })

  it('the 12-hue genreColors rainbow is gone (genres render as neutral overlay badges)', () => {
    expect(
      (cardTokens.featured as unknown as Record<string, unknown>).genreColors,
    ).toBeUndefined()
  })
})

// ── v1.1-polish Phase 07 (HSB-V11-LN-01..04) ───────────────────────────
// The `latest_news` token slot is widened with iconByType / labelByType
// lookup tables. Tests below assert both shape AND specific entries
// required by LatestNewsCard's per-type accent rendering.
describe('cardTokens.latest_news extensions', () => {
  it('exposes an iconByType lookup with the expected keys', () => {
    const t = cardTokens.latest_news
    expect(t.iconByType).toBeTypeOf('object')
    expect(t.iconByType.feat).toBe('sparkles')
    expect(t.iconByType.fix).toBe('wrench')
    expect(t.iconByType.perf).toBe('lightning')
    expect(t.iconByType.docs).toBe('sparkles')
  })

  it('exposes a labelByType lookup with the expected keys', () => {
    const t = cardTokens.latest_news
    expect(t.labelByType).toBeTypeOf('object')
    expect(t.labelByType.feat).toBeDefined()
    expect(t.labelByType.fix).toBeDefined()
    expect(t.labelByType.perf).toBeDefined()
  })

  it('each labelByType entry has an i18nKey under spotlight.latestNews.* + Tailwind accent', () => {
    const t = cardTokens.latest_news
    for (const key of ['feat', 'fix', 'perf'] as const) {
      const badge = t.labelByType[key]
      expect(badge.i18nKey).toMatch(/^spotlight\.latestNews\./)
      expect(badge.accent).toMatch(/bg-/)
      expect(badge.accent).toMatch(/text-/)
    }
  })

  it('per-type accents are color-coded — brand cyan + semantic status tokens', () => {
    const t = cardTokens.latest_news
    expect(t.labelByType.feat.accent).toContain('cyan')
    expect(t.labelByType.fix.accent).toContain('success')
    expect(t.labelByType.perf.accent).toContain('warning')
  })

  it('every iconByType value is a registered SpotlightIconName', () => {
    const t = cardTokens.latest_news
    for (const icon of Object.values(t.iconByType)) {
      expect(VALID_ICONS).toContain(icon)
    }
  })

  it('covers the long-form synonyms shipped in changelog.json today', () => {
    // Real-world changelog.json carries 'feature' / 'improvement' /
    // 'infrastructure' alongside the conventional-commit short forms.
    // Without explicit mappings the per-entry pill would silently
    // disappear for the vast majority of live entries.
    const t = cardTokens.latest_news
    expect(t.iconByType.feature).toBeDefined()
    expect(t.iconByType.improvement).toBeDefined()
    expect(t.labelByType.feature).toBeDefined()
  })
})
