// Skip-usage telemetry (owner directive 2026-07-18): every taken skip —
// manual chip click or auto-skip — must ship a `skip_used` player event with
// action/side/source dimensions, so the month-out review has real usage data
// per source (aniskip vs detected) before AniSkip submission is built.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref, nextTick } from 'vue'

const recordPlayerEvent = vi.fn()
vi.mock('@/utils/playerTelemetry', () => ({
  recordPlayerEvent: (e: unknown) => recordPlayerEvent(e),
}))

const opening = ref<{ start: number; end: number; source?: string } | null>(null)
const ending = ref<{ start: number; end: number; source?: string } | null>(null)
vi.mock('@/composables/useSkipTimes', () => ({
  useSkipTimes: () => ({ opening, ending }),
}))

import { useSkipIntro } from './useSkipIntro'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

function build(autoSkip = false) {
  const video = { currentTime: 0 } as HTMLVideoElement
  const currentTime = ref(0)
  const deps = {
    getMalId: () => '59970',
    selectedEpisode: ref({ number: 11 } as EpisodeOption),
    state: { autoSkip: ref(autoSkip) } as unknown as PlayerState,
    videoRef: ref<HTMLVideoElement | null>(video),
    currentTime,
    duration: ref(1440),
    writeProgress: vi.fn(),
    getCombo: () => ({ animeId: 'anime-uuid', provider: 'kodik', team: 'AniLibria.TV' }),
  }
  return { skip: useSkipIntro(deps), video, currentTime }
}

beforeEach(() => {
  recordPlayerEvent.mockReset()
  opening.value = null
  ending.value = null
})

describe('useSkipIntro skip-usage telemetry', () => {
  it('records a manual skip with side/source/team dimensions', () => {
    opening.value = { start: 100, end: 190, source: 'detected' }
    const { skip, currentTime, video } = build()
    currentTime.value = 120 // inside the OP window → skipTarget active

    skip.onSkipSegment()

    expect(video.currentTime).toBe(190)
    expect(recordPlayerEvent).toHaveBeenCalledTimes(1)
    expect(recordPlayerEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: 'skip_used',
        provider: 'kodik',
        anime_id: 'anime-uuid',
        episode: 11,
        detail: expect.objectContaining({
          action: 'manual', side: 'op', source: 'detected', team: 'AniLibria.TV',
          start: 100, end: 190,
        }),
      }),
    )
  })

  it('records an auto skip once per episode, defaulting source to aniskip', async () => {
    opening.value = { start: 100, end: 190 } // no source field (pre-blend cache)
    const { currentTime, video } = build(true)

    currentTime.value = 120
    await nextTick()

    expect(video.currentTime).toBe(190)
    expect(recordPlayerEvent).toHaveBeenCalledTimes(1)
    expect(recordPlayerEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: 'skip_used',
        detail: expect.objectContaining({ action: 'auto', side: 'op', source: 'aniskip' }),
      }),
    )

    // Re-entering the window in the same episode must not double-fire.
    currentTime.value = 121
    await nextTick()
    expect(recordPlayerEvent).toHaveBeenCalledTimes(1)
  })

  it('records nothing when no skip is taken', () => {
    const { skip } = build()
    skip.onSkipSegment() // no active target
    expect(recordPlayerEvent).not.toHaveBeenCalled()
  })
})
