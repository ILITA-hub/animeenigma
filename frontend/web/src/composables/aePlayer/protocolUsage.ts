// Pure helpers for the protocol_usage telemetry event — the dropped-frame math
// and the detail-payload mapping, kept out of AePlayer.vue so they are unit
// testable without mounting the player.

import type { TierResidency } from '@/utils/protocolLadder'

export interface QualitySample {
  dropped: number
  total: number
}

export interface ProtocolUsageContext {
  animeName: string
  combo: string
  /** Ephemeral, non-identifying per-mount id that groups a session's tier rows. */
  sess: string
}

function round2(n: number): number {
  return Math.round(n * 100) / 100
}

/** Reads dropped/total video frames from a video element, defensively. */
export function readVideoQuality(v: HTMLVideoElement | null): QualitySample {
  const q =
    v && typeof v.getVideoPlaybackQuality === 'function' ? v.getVideoPlaybackQuality() : null
  return { dropped: q?.droppedVideoFrames ?? 0, total: q?.totalVideoFrames ?? 0 }
}

/** Dropped-frame percentage over one tier residency, from the cumulative
 *  quality counters at tier start vs end. A negative total delta means the
 *  underlying <video> element (and its cumulative frame counters) was
 *  recreated mid-residency — e.g. a source switch — so the "end" sample is
 *  already relative to that new element's own start; it's used directly
 *  instead of the otherwise-bogus negative delta. Either way the result is
 *  clamped non-negative and the total floors at 1 to avoid /0. */
export function droppedFramesPct(start: QualitySample, end: QualitySample): number {
  let dropDelta = end.dropped - start.dropped
  let totalDelta = end.total - start.total
  if (totalDelta < 0) {
    dropDelta = end.dropped
    totalDelta = end.total
  }
  dropDelta = Math.max(0, dropDelta)
  totalDelta = Math.max(1, totalDelta)
  return round2((dropDelta / totalDelta) * 100)
}

/** Builds the `detail` payload merged verbatim into the analytics
 *  `properties` JSON for a protocol_usage event. */
export function buildProtocolUsageDetail(
  r: TierResidency,
  droppedPct: number,
  ctx: ProtocolUsageContext,
): Record<string, unknown> {
  return {
    schema_version: 1,
    protocol: r.protocol,
    tier: r.tierId,
    segments: r.segments,
    avg_mbps: round2(r.avgMbps),
    needed_mbps: round2(r.neededMbps),
    dropped_frames_pct: droppedPct,
    seg_timeouts: r.timeouts,
    tier_ms: Math.round(r.tierMs),
    trail: r.trail,
    probe: r.probe,
    anime_name: ctx.animeName,
    combo: ctx.combo,
    sess: ctx.sess,
  }
}
