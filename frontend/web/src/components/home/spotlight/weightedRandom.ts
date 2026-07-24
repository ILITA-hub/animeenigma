import type { SpotlightCard } from '@/types/spotlight'

/** A card's effective priority weight (backend default is 1.0). */
function cardPriority(card: SpotlightCard): number {
  return card.priority ?? 1
}

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
    const w = cardPriority(c)
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

/**
 * Cards at or above this priority are *pinned*: `pinnedFirst` orders them to
 * the front of the deck and `openingSlideIndex` always opens the carousel on
 * the first of them (no random pick). Weights BELOW the threshold (e.g.
 * curated's 1.5) keep the classic "biased random" semantics. The backend
 * counterpart is documented in services/catalog/.../cards/gacha_promo.go.
 */
export const PINNED_PRIORITY_MIN = 2

export function isPinned(card: SpotlightCard): boolean {
  return cardPriority(card) >= PINNED_PRIORITY_MIN
}

/**
 * Stable-moves pinned cards to the front (highest priority first — ties keep
 * server order); non-pinned cards keep their relative server order. Returns
 * the input array untouched when nothing is pinned.
 */
export function pinnedFirst(cards: SpotlightCard[]): SpotlightCard[] {
  const pinned = cards
    .filter(isPinned)
    .sort((a, b) => cardPriority(b) - cardPriority(a))
  if (pinned.length === 0) return cards
  return [...pinned, ...cards.filter((c) => !isPinned(c))]
}

/**
 * Opening-slide pick for HeroSpotlightBlock: a deck whose first card is
 * pinned (i.e. already ordered by `pinnedFirst`) always opens on it;
 * otherwise fall back to the classic weighted-random pick.
 */
export function openingSlideIndex(
  cards: SpotlightCard[],
  rng: () => number = Math.random,
): number {
  const first = cards[0]
  if (first && isPinned(first)) return 0
  return weightedRandomIndex(cards, rng)
}
