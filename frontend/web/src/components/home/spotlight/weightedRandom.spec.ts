import { describe, it, expect } from 'vitest'
import {
  weightedRandomIndex,
  pinnedFirst,
  openingSlideIndex,
  isPinned,
  PINNED_PRIORITY_MIN,
} from './weightedRandom'
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

  it('falls back to uniform when all weights are zero/non-positive', () => {
    const cards = [card('a', 0), card('b', 0)]
    // total <= 0 → Math.floor(rng() * length)
    expect(weightedRandomIndex(cards, () => 0)).toBe(0)
    expect(weightedRandomIndex(cards, () => 0.9)).toBe(1)
  })
})

describe('pinned cards', () => {
  it('isPinned: threshold is PINNED_PRIORITY_MIN, curated 1.5 stays unpinned', () => {
    expect(isPinned(card('gacha_promo', PINNED_PRIORITY_MIN))).toBe(true)
    expect(isPinned(card('gacha_promo', 5))).toBe(true)
    expect(isPinned(card('curated', 1.5))).toBe(false)
    expect(isPinned(card('featured'))).toBe(false)
  })

  it('pinnedFirst moves the pinned card to the front, rest keep server order', () => {
    const cards = [card('a'), card('b', 1.5), card('gacha_promo', 5), card('c')]
    const out = pinnedFirst(cards)
    expect(out.map((c) => c.type)).toEqual(['gacha_promo', 'a', 'b', 'c'])
  })

  it('pinnedFirst returns the input untouched when nothing is pinned', () => {
    const cards = [card('a'), card('b', 1.5)]
    expect(pinnedFirst(cards)).toBe(cards)
  })

  it('pinnedFirst orders multiple pinned cards by priority, highest first', () => {
    const cards = [card('p2', 3), card('a'), card('p1', 5)]
    expect(pinnedFirst(cards).map((c) => c.type)).toEqual(['p1', 'p2', 'a'])
  })

  it('openingSlideIndex always opens on a pinned first card (rng ignored)', () => {
    const cards = pinnedFirst([card('a'), card('gacha_promo', 5), card('b')])
    expect(openingSlideIndex(cards, () => 0.999)).toBe(0)
    expect(openingSlideIndex(cards, () => 0)).toBe(0)
  })

  it('openingSlideIndex falls back to the weighted pick without a pinned card', () => {
    const cards = [card('a'), card('b'), card('c')]
    expect(openingSlideIndex(cards, () => 0.999)).toBe(2)
    expect(openingSlideIndex(cards, () => 0)).toBe(0)
  })
})
