import { describe, it, expect } from 'vitest'
import { pickEpisodeForProvider, shouldReselectEpisode } from '../episodeSelection'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

const ep = (n: number): EpisodeOption => ({ key: n, label: n, number: n })

describe('shouldReselectEpisode — closes the mount race', () => {
  it('re-picks when initialEpisode resolves to a different number and user has not picked', () => {
    // mount default was 1; resume resolved to 6 → move to 6
    expect(shouldReselectEpisode(1, 6, false)).toBe(true)
  })
  it('does NOT re-pick once the user manually chose an episode', () => {
    expect(shouldReselectEpisode(1, 6, true)).toBe(false)
  })
  it('does NOT re-pick when initialEpisode is still undefined', () => {
    expect(shouldReselectEpisode(1, undefined, false)).toBe(false)
  })
  it('does NOT re-pick when already on the target episode (no churn)', () => {
    expect(shouldReselectEpisode(6, 6, false)).toBe(false)
  })
  it('re-picks from a null current selection', () => {
    expect(shouldReselectEpisode(null, 1, false)).toBe(true)
  })
})

// Guard the existing helper stays intact.
describe('pickEpisodeForProvider (regression)', () => {
  it('keeps an exact number match', () => {
    expect(pickEpisodeForProvider([ep(1), ep(2), ep(3)], 2, null)?.number).toBe(2)
  })
})
