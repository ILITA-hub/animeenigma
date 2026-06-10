import { describe, it, expect } from 'vitest'
import en from '../en.json'
import ru from '../ru.json'

// NOTE: ja.json is intentionally NOT imported here — the gacha namespace is
// en + ru only for the initial dark-ship phase (admin-only). Japanese locale
// parity will land alongside the global release.

// Recursively collect every leaf key path in an object, e.g.
// { a: { b: 'x', c: 'y' } } -> ['a.b', 'a.c']
function leafPaths(obj: unknown, prefix = ''): string[] {
  if (obj === null || typeof obj !== 'object') return [prefix]
  return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) => {
    const next = prefix ? `${prefix}.${k}` : k
    return leafPaths(v, next)
  })
}

// Walk to a value via dot-path. Returns undefined if any segment missing.
function get(obj: unknown, path: string): unknown {
  return path.split('.').reduce<unknown>((acc, seg) => {
    if (acc == null || typeof acc !== 'object') return undefined
    return (acc as Record<string, unknown>)[seg]
  }, obj)
}

describe('gacha i18n parity', () => {
  const enGacha = (en as Record<string, unknown>).gacha
  const ruGacha = (ru as Record<string, unknown>).gacha

  it('en.json has a top-level gacha object', () => {
    expect(enGacha).toBeTypeOf('object')
    expect(enGacha).not.toBeNull()
  })

  it('ru.json has a top-level gacha object', () => {
    expect(ruGacha).toBeTypeOf('object')
    expect(ruGacha).not.toBeNull()
  })

  it('en and ru gacha key sets are identical', () => {
    const enKeys = leafPaths(enGacha).sort()
    const ruKeys = leafPaths(ruGacha).sort()
    expect(ruKeys).toEqual(enKeys)
  })

  it('every gacha.* leaf value is a non-empty string in en.json', () => {
    const paths = leafPaths(enGacha)
    for (const p of paths) {
      const v = get(enGacha, p)
      expect(typeof v, `en gacha.${p}`).toBe('string')
      expect((v as string).trim().length, `en gacha.${p} non-empty`).toBeGreaterThan(0)
    }
  })

  it('every gacha.* leaf value is a non-empty string in ru.json', () => {
    const paths = leafPaths(ruGacha)
    for (const p of paths) {
      const v = get(ruGacha, p)
      expect(typeof v, `ru gacha.${p}`).toBe('string')
      expect((v as string).trim().length, `ru gacha.${p} non-empty`).toBeGreaterThan(0)
    }
  })

  // Explicit expected top-level keys (excludes nested admin.* sub-keys).
  const expectedTopKeys = [
    'nav_item',
    'balance_tooltip',
    'balance_chip_aria',
    'banner_list_title',
    'banner_list_empty',
    'banner_standard_badge',
    'banner_open',
    'daily_claim_title',
    'daily_claim_button',
    'daily_claimed_text',
    'daily_streak_label',
    'daily_amount',
    'spin_title',
    'spin_pity_label',
    'spin_x1',
    'spin_x1_cost',
    'spin_x10',
    'spin_x10_cost',
    'spin_insufficient',
    'result_dialog_title',
    'result_new_badge',
    'result_dupe_badge',
    'result_close',
    'pool_title',
    'pool_owned_badge',
    'collection_tab',
    'collection_progress',
    'collection_unknown',
    'collection_section_ssr',
    'collection_section_sr',
    'collection_section_r',
    'collection_section_n',
    'collection_empty',
    'rarity_n',
    'rarity_r',
    'rarity_sr',
    'rarity_ssr',
  ] as const

  it.each(expectedTopKeys)('en.json has gacha.%s as a string', (key) => {
    expect(typeof (enGacha as Record<string, unknown>)[key]).toBe('string')
  })

  it.each(expectedTopKeys)('ru.json has gacha.%s as a string', (key) => {
    expect(typeof (ruGacha as Record<string, unknown>)[key]).toBe('string')
  })

  // Interpolation-token preservation.
  it('gacha.spin_pity_label preserves {n} and {max} in both locales', () => {
    expect((enGacha as Record<string, string>).spin_pity_label).toContain('{n}')
    expect((enGacha as Record<string, string>).spin_pity_label).toContain('{max}')
    expect((ruGacha as Record<string, string>).spin_pity_label).toContain('{n}')
    expect((ruGacha as Record<string, string>).spin_pity_label).toContain('{max}')
  })

  it('gacha.balance_chip_aria preserves {n} in both locales', () => {
    expect((enGacha as Record<string, string>).balance_chip_aria).toContain('{n}')
    expect((ruGacha as Record<string, string>).balance_chip_aria).toContain('{n}')
  })

  it('gacha.collection_progress preserves {owned} and {total} in both locales', () => {
    expect((enGacha as Record<string, string>).collection_progress).toContain('{owned}')
    expect((enGacha as Record<string, string>).collection_progress).toContain('{total}')
    expect((ruGacha as Record<string, string>).collection_progress).toContain('{owned}')
    expect((ruGacha as Record<string, string>).collection_progress).toContain('{total}')
  })
})
