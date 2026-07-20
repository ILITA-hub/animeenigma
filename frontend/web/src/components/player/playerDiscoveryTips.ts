export const PLAYER_DISCOVERY_TIP_KEYS = [
  'player.discoveryTips.items.downloads',
  'player.discoveryTips.items.feedback',
  'player.discoveryTips.items.subtitleTiming',
  'player.discoveryTips.items.secretFeature',
  'player.discoveryTips.items.rawDub',
  'player.discoveryTips.items.watchTogether',
  'player.discoveryTips.items.streamReport',
  'player.discoveryTips.items.languageLearning',
  'player.discoveryTips.items.help',
] as const

/**
 * Pick a random tip while avoiding the currently visible one.
 *
 * Keeping this pure makes the randomness contract deterministic in tests and
 * lets the component use the same path for its first tip and manual shuffle.
 */
export function pickDiscoveryTipIndex(
  count: number,
  previousIndex = -1,
  random: () => number = Math.random,
): number {
  if (!Number.isInteger(count) || count <= 0) return -1
  if (count === 1) return 0

  const hasPrevious = previousIndex >= 0 && previousIndex < count
  const candidateCount = hasPrevious ? count - 1 : count
  const sample = random()
  const unit = Number.isFinite(sample)
    ? Math.min(Math.max(sample, 0), 1 - Number.EPSILON)
    : 0
  let candidate = Math.floor(unit * candidateCount)

  // The random range omits one slot. Shift indices at/after that slot so the
  // previous tip can never be selected twice in a row.
  if (hasPrevious && candidate >= previousIndex) candidate += 1
  return candidate
}
