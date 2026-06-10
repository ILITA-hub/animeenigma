import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import { usePlaybackStats } from './usePlaybackStats'

// Minimal fake <video> — only what sample() reads.
function fakeVideo(over: Record<string, unknown> = {}) {
  return {
    currentTime: 50,
    readyState: 4,
    videoWidth: 1280,
    videoHeight: 720,
    buffered: { length: 1, start: () => 40, end: () => 95 },
    getVideoPlaybackQuality: () => ({ droppedVideoFrames: 3, totalVideoFrames: 1200 }),
    ...over,
  } as unknown as HTMLVideoElement
}

describe('usePlaybackStats', () => {
  it('samples buffer ahead/behind around currentTime', () => {
    const { stats, sample } = usePlaybackStats(ref(fakeVideo()), ref(false))
    sample()
    expect(stats.value.bufferAheadSec).toBe(45)
    expect(stats.value.bufferBehindSec).toBe(10)
    expect(stats.value.readyState).toBe(4)
    expect(stats.value.resolution).toBe('1280×720')
    expect(stats.value.droppedFrames).toBe(3)
    expect(stats.value.totalFrames).toBe(1200)
  })

  it('reports zeros when currentTime is outside every buffered range', () => {
    const v = fakeVideo({ currentTime: 200 })
    const { stats, sample } = usePlaybackStats(ref(v), ref(false))
    sample()
    expect(stats.value.bufferAheadSec).toBe(0)
    expect(stats.value.bufferBehindSec).toBe(0)
  })

  it('degrades when getVideoPlaybackQuality is unavailable', () => {
    const v = fakeVideo({ getVideoPlaybackQuality: undefined })
    const { stats, sample } = usePlaybackStats(ref(v), ref(false))
    sample()
    expect(stats.value.droppedFrames).toBe(0)
  })

  it('reports empty stats with no video element', () => {
    const { stats, sample } = usePlaybackStats(ref(null), ref(false))
    sample()
    expect(stats.value.readyState).toBe(0)
    expect(stats.value.resolution).toBe('')
  })
})
