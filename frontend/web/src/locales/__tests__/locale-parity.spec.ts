import { describe, it, expect } from 'vitest'
import en from '../en.json'
import ru from '../ru.json'
import ja from '../ja.json'

// Full-tree i18n parity gate. The per-namespace specs (spotlight/gacha/
// watch-together) only cover their sub-trees, so en/ru/ja could drift in any
// OTHER namespace and stay green until a `make redeploy-web` i18n-lint run
// caught it. This asserts the ENTIRE leaf-key set is identical across all three
// locales so tsc/vitest catch a missing/extra key immediately.

// Recursively collect every leaf key path, e.g. { a: { b: 'x' } } -> ['a.b'].
function leafPaths(obj: unknown, prefix = ''): string[] {
  if (obj === null || typeof obj !== 'object') return [prefix]
  return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) => {
    const next = prefix ? `${prefix}.${k}` : k
    return leafPaths(v, next)
  })
}

const enKeys = new Set(leafPaths(en))
const ruKeys = new Set(leafPaths(ru))
const jaKeys = new Set(leafPaths(ja))

const missing = (base: Set<string>, other: Set<string>) =>
  [...base].filter((k) => !other.has(k)).sort()

describe('full-tree i18n parity (en/ru/ja)', () => {
  it('ru.json has no keys missing vs en.json', () => {
    expect(missing(enKeys, ruKeys)).toEqual([])
  })

  it('ru.json has no extra keys vs en.json', () => {
    expect(missing(ruKeys, enKeys)).toEqual([])
  })

  it('ja.json has no keys missing vs en.json', () => {
    expect(missing(enKeys, jaKeys)).toEqual([])
  })

  it('ja.json has no extra keys vs en.json', () => {
    expect(missing(jaKeys, enKeys)).toEqual([])
  })

  it('all three locales have the same leaf-key count', () => {
    expect(ruKeys.size).toBe(enKeys.size)
    expect(jaKeys.size).toBe(enKeys.size)
  })
})
