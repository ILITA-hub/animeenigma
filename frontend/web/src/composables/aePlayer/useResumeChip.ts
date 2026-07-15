import { computed, ref, type Ref } from 'vue'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { useAuthStore } from '@/stores/auth'

// ── Resume-from-saved-position chip + shared-link `?t=` one-shot seek ────────
// Saved position for the current episode: server watch_progress first (logged
// in), localStorage fallback (anonymous parity with KodikPlayer). The chip
// offers the jump — it never auto-seeks.

export interface ResumeChipDeps {
  videoRef: Ref<HTMLVideoElement | null>
  auth: ReturnType<typeof useAuthStore>
  getAnimeId: () => string
  /** Shared-link `?t=` initial position (seconds), captured once at setup. */
  initialTimestamp: number | undefined
  selectedEpisode: Ref<EpisodeOption | null>
  epProgress: Ref<Record<number, { pct: number; sec: number; completed: boolean }>>
  sourceError: Ref<string | null>
  isResolving: Ref<boolean>
  duration: Ref<number>
  currentTime: Ref<number>
  hasStarted: Ref<boolean>
  attemptPlay: () => void
  writeProgress: () => void
}

export function useResumeChip(deps: ResumeChipDeps) {
  const { videoRef, auth, selectedEpisode, epProgress, sourceError, isResolving, duration, currentTime, hasStarted } = deps

  const resumeChipDismissed = ref(false)
  const resumeChipUsed = ref(false)

  // Shared-link `?t=` → seek the video to this position on the FIRST stream load,
  // once. While it is pending it also suppresses the passive resume chip below:
  // the sharer's explicit position wins over the viewer's own saved progress for
  // this load. Cleared (→ 0) the moment the seek is applied, restoring normal
  // resume-chip behavior for every later episode.
  const initialSeekSec = ref(Math.max(0, deps.initialTimestamp ?? 0))
  function applyInitialSeek() {
    if (initialSeekSec.value <= 0) return
    const target = initialSeekSec.value
    initialSeekSec.value = 0 // consume once
    const v = videoRef.value
    if (!v) return
    const seek = () => {
      try {
        v.currentTime = target
      } catch {
        /* element not seekable yet — best-effort */
      }
    }
    if (v.readyState >= 1) seek()
    else v.addEventListener('loadedmetadata', seek, { once: true })
  }

  function localResumeSec(ep: number): number {
    try {
      const data = JSON.parse(localStorage.getItem(`watch_progress:${deps.getAnimeId()}`) || '{}')
      return Number(data[ep]?.time) || 0
    } catch {
      return 0
    }
  }

  const resumePosSec = computed(() => {
    const ep = selectedEpisode.value?.number
    if (!ep) return 0
    const server = epProgress.value[ep]
    if (server && !server.completed && server.sec > 0) return server.sec
    if (!auth.isAuthenticated) return localResumeSec(ep)
    return 0
  })

  const resumeChipVisible = computed(() => {
    if (initialSeekSec.value > 0) return false // shared-link position takes over
    if (resumeChipDismissed.value || resumeChipUsed.value) return false
    if (sourceError.value || isResolving.value) return false
    const pos = resumePosSec.value
    if (pos < 30) return false // too little progress to bother
    // Once near the end the next-episode flow takes over instead.
    if (duration.value > 0 && pos >= 0.95 * duration.value) return false
    // The offer expires once the user has clearly chosen to watch from here.
    if (hasStarted.value && currentTime.value > 5) return false
    return true
  })

  function onResumeFromSaved() {
    const v = videoRef.value
    if (!v) return
    resumeChipUsed.value = true
    v.currentTime = resumePosSec.value
    if (v.paused) deps.attemptPlay()
    deps.writeProgress()
  }

  return {
    resumeChipDismissed,
    resumeChipUsed,
    applyInitialSeek,
    resumePosSec,
    resumeChipVisible,
    onResumeFromSaved,
  }
}
