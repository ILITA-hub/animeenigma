import { computed, watch, type Ref } from 'vue'
import { useSkipTimes, type SkipSegment, type SkipTimesComboContext } from '@/composables/useSkipTimes'
import { segmentsToChapters, activeSkipSegment } from '@/composables/aePlayer/skipSegments'
import { recordPlayerEvent } from '@/utils/playerTelemetry'
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

  // Skip-usage telemetry (owner directive 2026-07-18): every actual skip —
  // manual chip click or auto-skip — ships a `skip_used` player event so the
  // month-out review has real usage/UX data per source (aniskip vs detected)
  // before we commit to submitting our timings back to AniSkip. DurationMS
  // (BE side) carries the skipped span; never throws (playerTelemetry
  // contract).
  function recordSkipUsage(action: 'manual' | 'auto', side: 'op' | 'ed', seg: SkipSegment) {
    const combo = deps.getCombo?.()
    if (!combo?.provider) return // no provider → whitelisted-provider check would drop it anyway
    recordPlayerEvent({
      kind: 'skip_used',
      provider: combo.provider,
      anime_id: combo.animeId ?? '',
      episode: epNumber.value ?? undefined,
      detail: {
        action,
        side,
        source: seg.source ?? 'aniskip',
        team: combo.team ?? '',
        start: seg.start,
        end: seg.end,
      },
    })
  }

  function onSkipSegment() {
    const v = videoRef.value
    const target = skipTarget.value
    if (!v || !target) return
    v.currentTime = target.end
    deps.writeProgress()
    const side = target.kind === 'outro' ? 'ed' : 'op'
    const seg = side === 'ed' ? ending.value : opening.value
    if (seg) recordSkipUsage('manual', side, seg)
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
        recordSkipUsage('auto', 'op', op)
      }
    }
  })

  // opening/ending are exposed for the hacker-mode debug stats — each
  // segment carries its `source` ("aniskip" | "detected") so the HUD can
  // show where the skip windows came from.
  return { chapters, skipTarget, onSkipSegment, opening, ending }
}
