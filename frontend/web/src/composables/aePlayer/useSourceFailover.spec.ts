import { describe, it, expect } from 'vitest'
import { hasMediaArrived } from './useSourceFailover'

// The silent-stall watchdog's native-path progress signal. hls.js counters
// (engine.fragLoadedCount) and the ladder's xhr taps see NOTHING on native
// playback (iPhone HLS via AVFoundation, plain MP4 src) — the media element
// itself is the only witness that bytes are arriving. Without this guard the
// watchdog declared healthy, buffering native streams "silent stall" and
// churned through every provider to all_exhausted while segments were
// visibly flowing in the gateway logs (report 2026-07-16T15-20-22 +
// player_failed 14:56 attempt trail).
describe('hasMediaArrived', () => {
  const el = (readyState: number, bufferedLen: number) =>
    ({ readyState, buffered: { length: bufferedLen } }) as unknown as HTMLVideoElement

  it('null element → nothing arrived', () => {
    expect(hasMediaArrived(null)).toBe(false)
  })

  it('readyState HAVE_NOTHING with empty buffer → dead so far', () => {
    expect(hasMediaArrived(el(0, 0))).toBe(false)
  })

  it('HAVE_METADATA proves the playlist/init parsed → media is arriving', () => {
    expect(hasMediaArrived(el(1, 0))).toBe(true)
  })

  it('any buffered range proves bytes decoded → media is arriving', () => {
    expect(hasMediaArrived(el(0, 1))).toBe(true)
  })
})
