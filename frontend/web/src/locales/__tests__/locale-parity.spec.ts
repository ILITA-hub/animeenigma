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

// Flatten to a dot-key -> string-value map (arrays recurse by index, same as
// leafPaths) so we can compare interpolation vars per key across locales.
function leafValues(obj: unknown, prefix = '', out: Record<string, string> = {}): Record<string, string> {
  if (obj === null || typeof obj !== 'object') {
    out[prefix] = String(obj)
  } else {
    for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
      leafValues(v, prefix ? `${prefix}.${k}` : k, out)
    }
  }
  return out
}

// ICU named/list placeholders: {name}, {count}, {0}. NOT vue-i18n linked
// messages (@:foo) or escaped literals ({'@'}) — those aren't interpolation vars.
function placeholders(s: string): string[] {
  const set = new Set<string>()
  for (const m of s.matchAll(/\{\s*([a-zA-Z0-9_]+)\s*\}/g)) set.add(m[1])
  return [...set].sort()
}

describe('full-tree ICU placeholder parity (en/ru/ja)', () => {
  const enVals = leafValues(en)
  const ruVals = leafValues(ru)
  const jaVals = leafValues(ja)

  // Restricted to keys present in all three (key parity is asserted above; this
  // keeps placeholder-drift failures from doubling up with missing-key ones).
  const common = Object.keys(enVals).filter((k) => k in ruVals && k in jaVals)

  it('every shared key has matching {placeholder} vars across locales', () => {
    const mismatches: Array<{ key: string; en: string[]; ru: string[]; ja: string[] }> = []
    for (const key of common) {
      const enP = placeholders(enVals[key])
      const ruP = placeholders(ruVals[key])
      const jaP = placeholders(jaVals[key])
      const same =
        enP.length === ruP.length &&
        enP.length === jaP.length &&
        enP.every((p, i) => p === ruP[i] && p === jaP[i])
      if (!same) mismatches.push({ key, en: enP, ru: ruP, ja: jaP })
    }
    expect(mismatches).toEqual([])
  })
})
