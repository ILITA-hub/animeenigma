// Samples <video>-element debug stats (buffer window, readyState, dropped
// frames, resolution) on a 500ms interval while `enabled` — feeds the
// hacker-mode DebugHud and the settings-menu mini-stats.

import { ref, watch, onUnmounted, type Ref } from 'vue'

export interface PlaybackStats {
  readyState: number
  bufferAheadSec: number
  bufferBehindSec: number
  droppedFrames: number
  totalFrames: number
  /** e.g. "1920×1080"; empty until the first frame is decoded */
  resolution: string
}

const EMPTY: PlaybackStats = {
  readyState: 0,
  bufferAheadSec: 0,
  bufferBehindSec: 0,
  droppedFrames: 0,
  totalFrames: 0,
  resolution: '',
}

export function usePlaybackStats(
  videoEl: Ref<HTMLVideoElement | null>,
  enabled: Ref<boolean>,
) {
  const stats = ref<PlaybackStats>({ ...EMPTY })
  let timer: ReturnType<typeof setInterval> | null = null

  function sample() {
    const v = videoEl.value
    if (!v) {
      stats.value = { ...EMPTY }
      return
    }
    const t = v.currentTime
    let ahead = 0
    let behind = 0
    for (let i = 0; i < v.buffered.length; i++) {
      const s = v.buffered.start(i)
      const e = v.buffered.end(i)
      if (t >= s && t <= e) {
        ahead = e - t
        behind = t - s
        break
      }
    }
    const q =
      typeof v.getVideoPlaybackQuality === 'function' ? v.getVideoPlaybackQuality() : null
    stats.value = {
      readyState: v.readyState,
      bufferAheadSec: ahead,
      bufferBehindSec: behind,
      droppedFrames: q?.droppedVideoFrames ?? 0,
      totalFrames: q?.totalVideoFrames ?? 0,
      resolution: v.videoWidth ? `${v.videoWidth}×${v.videoHeight}` : '',
    }
  }

  watch(
    enabled,
    (on) => {
      if (on && timer === null) {
        sample()
        timer = setInterval(sample, 500)
      }
      if (!on && timer !== null) {
        clearInterval(timer)
        timer = null
      }
    },
    { immediate: true },
  )

  onUnmounted(() => {
    if (timer !== null) clearInterval(timer)
  })

  return { stats, sample }
}
