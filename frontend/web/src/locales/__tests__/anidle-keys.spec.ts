import { describe, it, expect } from 'vitest'
import en from '../en.json'
import ru from '../ru.json'
import ja from '../ja.json'

// Recursively collect every leaf key path in an object
function leafPaths(obj: unknown, prefix = ''): string[] {
  if (obj === null || typeof obj !== 'object' || Array.isArray(obj)) return [prefix]
  return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) => {
    const next = prefix ? `${prefix}.${k}` : k
    return leafPaths(v, next)
  })
}

function get(obj: unknown, path: string): unknown {
  return path.split('.').reduce<unknown>((acc, seg) => {
    if (acc == null || typeof acc !== 'object') return undefined
    return (acc as Record<string, unknown>)[seg]
  }, obj)
}

describe('anidle i18n parity', () => {
  const enAnidle = (en as Record<string, unknown>).anidle
  const ruAnidle = (ru as Record<string, unknown>).anidle
  const jaAnidle = (ja as Record<string, unknown>).anidle

  it('en.json has a top-level anidle namespace', () => {
    expect(enAnidle).toBeTypeOf('object')
    expect(enAnidle).not.toBeNull()
  })

  it('ru.json has a top-level anidle namespace', () => {
    expect(ruAnidle).toBeTypeOf('object')
    expect(ruAnidle).not.toBeNull()
  })

  it('ja.json has a top-level anidle namespace', () => {
    expect(jaAnidle).toBeTypeOf('object')
    expect(jaAnidle).not.toBeNull()
  })

  it('en and ru anidle key sets are identical', () => {
    const enKeys = leafPaths(enAnidle).sort()
    const ruKeys = leafPaths(ruAnidle).sort()
    expect(ruKeys).toEqual(enKeys)
  })

  it('en and ja anidle key sets are identical', () => {
    const enKeys = leafPaths(enAnidle).sort()
    const jaKeys = leafPaths(jaAnidle).sort()
    expect(jaKeys).toEqual(enKeys)
  })

  it('all three locales have identical anidle key sets', () => {
    const enKeys = leafPaths(enAnidle).sort()
    const ruKeys = leafPaths(ruAnidle).sort()
    const jaKeys = leafPaths(jaAnidle).sort()
    expect(ruKeys).toEqual(enKeys)
    expect(jaKeys).toEqual(enKeys)
  })

  it('every anidle leaf value is a non-empty string in en.json', () => {
    const paths = leafPaths(enAnidle)
    for (const p of paths) {
      const v = get(enAnidle, p)
      expect(typeof v, `en anidle.${p}`).toBe('string')
      expect((v as string).trim().length, `en anidle.${p} non-empty`).toBeGreaterThan(0)
    }
  })

  it('every anidle leaf value is a non-empty string in ru.json', () => {
    const paths = leafPaths(ruAnidle)
    for (const p of paths) {
      const v = get(ruAnidle, p)
      expect(typeof v, `ru anidle.${p}`).toBe('string')
      expect((v as string).trim().length, `ru anidle.${p} non-empty`).toBeGreaterThan(0)
    }
  })

  it('every anidle leaf value is a non-empty string in ja.json', () => {
    const paths = leafPaths(jaAnidle)
    for (const p of paths) {
      const v = get(jaAnidle, p)
      expect(typeof v, `ja anidle.${p}`).toBe('string')
      expect((v as string).trim().length, `ja anidle.${p} non-empty`).toBeGreaterThan(0)
    }
  })

  // Specific critical keys — explicit assertions so deletions are caught
  const criticalKeys = [
    'nav_item', 'page_title', 'search_placeholder', 'column_genres',
    'column_year', 'result_win_title', 'stats_guest_notice', 'leaderboard_title',
    'give_up_button', 'result_share_button',
  ] as const

  it.each(criticalKeys)('en.json has anidle.%s', (key) => {
    expect(typeof (enAnidle as Record<string, unknown>)[key]).toBe('string')
  })

  it.each(criticalKeys)('ru.json has anidle.%s', (key) => {
    expect(typeof (ruAnidle as Record<string, unknown>)[key]).toBe('string')
  })

  it.each(criticalKeys)('ja.json has anidle.%s', (key) => {
    expect(typeof (jaAnidle as Record<string, unknown>)[key]).toBe('string')
  })

  it('en.json has nav.anidle', () => {
    const nav = (en as Record<string, Record<string, unknown>>).nav
    expect(typeof nav.anidle).toBe('string')
  })

  it('ru.json has nav.anidle', () => {
    const nav = (ru as Record<string, Record<string, unknown>>).nav
    expect(typeof nav.anidle).toBe('string')
  })

  it('ja.json has nav.anidle', () => {
    const nav = (ja as Record<string, Record<string, unknown>>).nav
    expect(typeof nav.anidle).toBe('string')
  })
})
