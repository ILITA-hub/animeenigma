import { describe, it, expect } from 'vitest'
import {
  readVideoQuality,
  droppedFramesPct,
  buildProtocolUsageDetail,
} from './protocolUsage'
import type { TierResidency } from '@/utils/protocolLadder'

const RESIDENCY: TierResidency = {
  tierId: 'h2',
  protocol: 'h2',
  segments: 214,
  avgMbps: 3.2049,
  neededMbps: 2.1,
  timeouts: 2,
  tierMs: 184000,
  trail: 'h3→h2 (first-frag projected 17s)',
  probe: 'h3 2.1 Mbps @03:24 — rejected (<1.1× h2)',
}

describe('droppedFramesPct', () => {
  it('computes the delta percentage over a residency', () => {
    expect(droppedFramesPct({ dropped: 10, total: 1000 }, { dropped: 16, total: 1100 })).toBe(6)
  })
  it('clamps negative deltas (video element recreated on a source switch)', () => {
    expect(droppedFramesPct({ dropped: 50, total: 5000 }, { dropped: 3, total: 300 })).toBe(1)
  })
  it('never divides by zero when no frames advanced', () => {
    expect(droppedFramesPct({ dropped: 0, total: 0 }, { dropped: 0, total: 0 })).toBe(0)
  })
})

describe('readVideoQuality', () => {
  it('reads dropped/total from getVideoPlaybackQuality', () => {
    const v = { getVideoPlaybackQuality: () => ({ droppedVideoFrames: 3, totalVideoFrames: 1200 }) }
    expect(readVideoQuality(v as unknown as HTMLVideoElement)).toEqual({ dropped: 3, total: 1200 })
  })
  it('degrades to zeros when unavailable', () => {
    expect(readVideoQuality(null)).toEqual({ dropped: 0, total: 0 })
  })
})

describe('buildProtocolUsageDetail', () => {
  it('maps and rounds a residency into the detail payload', () => {
    const d = buildProtocolUsageDetail(RESIDENCY, 0.42, {
      animeName: 'Naruto',
      combo: 'sub·en·gogoanime',
      sess: 's_abc123',
    })
    expect(d).toEqual({
      schema_version: 1,
      protocol: 'h2',
      tier: 'h2',
      segments: 214,
      avg_mbps: 3.2,
      needed_mbps: 2.1,
      dropped_frames_pct: 0.42,
      seg_timeouts: 2,
      tier_ms: 184000,
      trail: 'h3→h2 (first-frag projected 17s)',
      probe: 'h3 2.1 Mbps @03:24 — rejected (<1.1× h2)',
      anime_name: 'Naruto',
      combo: 'sub·en·gogoanime',
      sess: 's_abc123',
    })
  })
})
