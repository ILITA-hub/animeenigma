import { describe, it, expect } from 'vitest'
import { SHOWCASE_VARIANTS, VARIANT_SIZE, sizeFor, spanClasses } from '@/types/showcase'

describe('VARIANT_SIZE parity', () => {
  it('has a bound for every (type, variant) in SHOWCASE_VARIANTS', () => {
    for (const t of Object.keys(SHOWCASE_VARIANTS) as Array<keyof typeof SHOWCASE_VARIANTS>) {
      for (const v of SHOWCASE_VARIANTS[t]) {
        expect(VARIANT_SIZE[t]?.[v], `${t}.${v}`).toBeDefined()
      }
    }
  })
})

describe('sizeFor', () => {
  it('falls back to the default variant when variant missing', () => {
    expect(sizeFor('about')).toEqual(VARIANT_SIZE.about.quote)
    expect(sizeFor('about', 'nope')).toEqual(VARIANT_SIZE.about.quote)
  })
  it('stats tiles are fixed 2x1', () => {
    const b = sizeFor('stats', 'tiles')
    expect([b.minW, b.maxW, b.minH, b.maxH]).toEqual([2, 2, 1, 1])
  })
})

describe('spanClasses', () => {
  it('maps width/height to static tailwind span classes', () => {
    expect(spanClasses(4, 1)).toBe('col-span-2 md:col-span-4 row-span-1')
    expect(spanClasses(1, 2)).toBe('col-span-1 md:col-span-1 row-span-2')
    expect(spanClasses(3, 3)).toBe('col-span-2 md:col-span-3 row-span-3')
  })
})
