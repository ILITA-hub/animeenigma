import { computed, ref, type ComputedRef, type Ref } from 'vue'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { useWatchTracking } from '@/composables/aePlayer/useWatchTracking'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

// ── Next episode logic ────────────────────────────────────────────────────────
// End-of-episode handling: autoplay-ON countdown card, autoplay-OFF manual
// chip, and the actual episode advance.

export interface NextEpisodeDeps {
  state: PlayerState
  tracking: ReturnType<typeof useWatchTracking>
  episodes: Ref<EpisodeOption[]>
  selectedEpisode: Ref<EpisodeOption | null>
  /** True once the playhead reaches the episode end (component-owned; also
   *  reset by the playback clock on resume / source swap). */
  reachedEpisodeEnd: Ref<boolean>
  skipTarget: ComputedRef<{ kind: 'intro' | 'outro'; end: number } | null>
  getInitialEpisode: () => number | undefined
  getTotalEps: () => number
  isEpisodeWatched: (n: number) => boolean
  resolveStreamForEpisode: (ep: EpisodeOption, keepPosition?: boolean) => Promise<void>
  resumeChipDismissed: Ref<boolean>
  resumeChipUsed: Ref<boolean>
}

export function useNextEpisode(deps: NextEpisodeDeps) {
  const { state, tracking, episodes, selectedEpisode, reachedEpisodeEnd, skipTarget, resumeChipDismissed, resumeChipUsed } = deps

  const showNextEpisode = ref(false)
  const nextEpCountdown = ref(5)
  let nextEpTimer: ReturnType<typeof setInterval> | null = null

  function onEnded() {
    state.playing.value = false
    // Reaching the end IS a completed watch — mark even if the 90% tick raced.
    tracking.saveNow()
    void tracking.markWatched()
    // Playhead hit the end. Autoplay-ON opens the countdown card; autoplay-OFF
    // surfaces the manual "Next episode" chip (via reachedEpisodeEnd → showNextEpChip).
    reachedEpisodeEnd.value = true
    if (anime_hasNextEp.value && state.autoNext.value) {
      startNextEpCountdown()
    }
  }

  // The episode the user is actually on — initialEpisode is the resume-resolved
  // starting point; selectedEpisode.value tracks in-session switches.
  const currentEpNumber = computed(
    () => selectedEpisode.value?.number ?? deps.getInitialEpisode() ?? 1,
  )

  // "Has a next episode" derived from the loaded episode list (authoritative for
  // the current source), falling back to the catalog ep/eps counts before the
  // list resolves. Using props.anime.ep here was the bug: after switching a few
  // episodes it still pointed at the mount episode, so the countdown could start
  // at the series end (and then goToNextEpisode found nothing → silent stall).
  const anime_hasNextEp = computed(() => {
    const sel = selectedEpisode.value
    if (episodes.value.length && sel) {
      const idx = episodes.value.findIndex((e) => e.number === sel.number)
      if (idx >= 0) return idx + 1 < episodes.value.length
    }
    return currentEpNumber.value < deps.getTotalEps()
  })

  // The actual next episode number for the "Up next" card label.
  const nextEpisodeNumber = computed(() => {
    const sel = selectedEpisode.value
    if (episodes.value.length && sel) {
      const idx = episodes.value.findIndex((e) => e.number === sel.number)
      if (idx >= 0 && idx + 1 < episodes.value.length) return episodes.value[idx + 1].number
    }
    return currentEpNumber.value + 1
  })

  // Manual "Next episode" chip — the autoplay-OFF affordance. The countdown card
  // owns the autoplay-ON path, so the two never show together. Visible from the
  // ending/outro segment through the episode end, whenever a next episode exists.
  const showNextEpChip = computed(
    () =>
      anime_hasNextEp.value &&
      !state.autoNext.value &&
      !showNextEpisode.value &&
      (reachedEpisodeEnd.value || skipTarget.value?.kind === 'outro'),
  )

  function startNextEpCountdown() {
    showNextEpisode.value = true
    nextEpCountdown.value = 5
    nextEpTimer = setInterval(() => {
      nextEpCountdown.value--
      if (nextEpCountdown.value <= 0) {
        clearNextEpTimer()
        goToNextEpisode()
      }
    }, 1000)
  }

  function clearNextEpTimer() {
    if (nextEpTimer) {
      clearInterval(nextEpTimer)
      nextEpTimer = null
    }
  }

  function goToNextEpisode() {
    showNextEpisode.value = false
    reachedEpisodeEnd.value = false // episode is changing — drop the end-of-ep chip
    clearNextEpTimer()
    // Find next episode in list
    const current = selectedEpisode.value
    if (!current) return
    const idx = episodes.value.findIndex((e) => e.number === current.number)
    const next = episodes.value[idx + 1]
    if (next) {
      tracking.saveNow()
      selectedEpisode.value = next
      tracking.resetEpisode(deps.isEpisodeWatched(next.number))
      resumeChipDismissed.value = false
      resumeChipUsed.value = false
      void deps.resolveStreamForEpisode(next)
    }
  }

  return {
    showNextEpisode,
    nextEpCountdown,
    onEnded,
    anime_hasNextEp,
    nextEpisodeNumber,
    showNextEpChip,
    clearNextEpTimer,
    goToNextEpisode,
  }
}
