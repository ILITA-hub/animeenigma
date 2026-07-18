import { computed, watch, type Ref } from 'vue'
import { useSkipTimes, type SkipTimesComboContext } from '@/composables/useSkipTimes'
import { segmentsToChapters, activeSkipSegment } from '@/composables/aePlayer/skipSegments'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

// ── Intro/outro skip (AniSkip via catalog proxy) ─────────────────────────────
// Skip chip + scrub-bar chapters + the auto-skip-intro setting.

export interface SkipIntroDeps {
  getMalId: () => string | number | undefined
  selectedEpisode: Ref<EpisodeOption | null>
  state: PlayerState
  videoRef: Ref<HTMLVideoElement | null>
  currentTime: Ref<number>
  duration: Ref<number>
  writeProgress: () => void
  // Task 10 (combo-aware skip times) — the current source selection, so a
  // provider/team switch fetches THIS encode's detected OP/ED window instead
  // of the crowdsourced (provider-agnostic) AniSkip fallback. Optional so
  // callers that don't care about per-encode timing (tests, future embeds)
  // can omit it and fall back to the plain malId+episode lookup.
  getCombo?: () => SkipTimesComboContext | null
}

export function useSkipIntro(deps: SkipIntroDeps) {
  const { selectedEpisode, state, videoRef, currentTime, duration } = deps

  const epNumber = computed(() => selectedEpisode.value?.number ?? null)
  const malIdRef = computed(() => deps.getMalId() ?? null)
  const comboRef = computed(() => deps.getCombo?.() ?? null)
  const { opening, ending } = useSkipTimes(malIdRef, epNumber, comboRef)

  const chapters = computed(() =>
    segmentsToChapters(opening.value, ending.value, duration.value),
  )

  const skipTarget = computed(() =>
    activeSkipSegment(currentTime.value, opening.value, ending.value),
  )

  function onSkipSegment() {
    const v = videoRef.value
    const target = skipTarget.value
    if (!v || !target) return
    v.currentTime = target.end
    deps.writeProgress()
  }

  // Auto-skip intro (settings toggle) — once per episode view so a manual
  // seek back into the OP isn't fought.
  let autoSkippedEp: number | null = null
  watch(epNumber, () => {
    autoSkippedEp = null
  })
  watch(currentTime, (t) => {
    if (!state.autoSkip.value) return
    const op = opening.value
    if (!op) return
    const ep = epNumber.value
    if (ep === null || autoSkippedEp === ep) return
    if (t >= op.start && t < op.end - 1) {
      autoSkippedEp = ep
      const v = videoRef.value
      if (v) {
        v.currentTime = op.end
        deps.writeProgress()
      }
    }
  })

  // opening/ending are exposed for the hacker-mode debug stats — each
  // segment carries its `source` ("aniskip" | "detected") so the HUD can
  // show where the skip windows came from.
  return { chapters, skipTarget, onSkipSegment, opening, ending }
}
