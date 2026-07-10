import type { SpotlightCard } from '@/types/spotlight'

/**
 * Picks a card index at random, biased by each card's `priority` weight
 * (default 1). A priority-1.5 card is 1.5× likelier to be chosen than a
 * default-1.0 card. When every weight is equal this is a plain uniform pick,
 * preserving the carousel's original random-start-for-variety behaviour.
 *
 * `rng` is injectable so tests are deterministic; production passes the
 * default Math.random. Returns 0 for an empty array (callers guard n>0).
 */
export function weightedRandomIndex(
  cards: SpotlightCard[],
  rng: () => number = Math.random,
): number {
  if (cards.length === 0) return 0
  const weights = cards.map((c) => {
    const w = c.priority ?? 1
    return w > 0 ? w : 0
  })
  const total = weights.reduce((a, w) => a + w, 0)
  if (total <= 0) return Math.floor(rng() * cards.length)
  let r = rng() * total
  for (let i = 0; i < weights.length; i++) {
    r -= weights[i]
    if (r < 0) return i
  }
  return cards.length - 1
}
