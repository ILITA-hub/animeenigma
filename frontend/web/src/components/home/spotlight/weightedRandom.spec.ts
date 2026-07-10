import { describe, it, expect } from 'vitest'
import { weightedRandomIndex } from './weightedRandom'
import type { SpotlightCard } from '@/types/spotlight'

const card = (type: string, priority?: number) =>
  ({ type, priority, data: {} }) as unknown as SpotlightCard

describe('weightedRandomIndex', () => {
  it('returns 0 for an empty array', () => {
    expect(weightedRandomIndex([])).toBe(0)
  })

  it('is uniform when every weight is the default (missing priority)', () => {
    const cards = [card('a'), card('b'), card('c')]
    expect(weightedRandomIndex(cards, () => 0)).toBe(0)
    expect(weightedRandomIndex(cards, () => 0.999)).toBe(2)
  })

  it('biases toward the higher-priority card (weights 1,1.5,1 → total 3.5)', () => {
    const cards = [card('a', 1), card('b', 1.5), card('c', 1)]
    // rng*3.5: 0→0 (idx0), 0.5→1.75 (idx1, the 1.0–2.5 bucket), 0.8→2.8 (idx2)
    expect(weightedRandomIndex(cards, () => 0)).toBe(0)
    expect(weightedRandomIndex(cards, () => 0.5)).toBe(1)
    expect(weightedRandomIndex(cards, () => 0.8)).toBe(2)
  })
})
