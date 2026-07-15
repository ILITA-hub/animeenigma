import { computed, watch, type Ref } from 'vue'
import { KODIK_QUALITY_PREF_KEY } from '@/composables/aePlayer/useProviderResolver'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { useVideoEngine } from '@/composables/aePlayer/useVideoEngine'
import type { StreamResult } from '@/types/aePlayer'

// ── Quality ladder ────────────────────────────────────────────────────────────
// HLS: data-driven from hls.js levels. MP4: only when the provider returned
// multiple URL-valued qualities. Per-URL HLS (Kodik: one manifest per quality,
// numeric values): switching re-resolves the stream instead of changing an
// hls.js level. Single-variant streams stay Auto-only (D-05).

export interface QualityControlDeps {
  state: PlayerState
  engine: ReturnType<typeof useVideoEngine>
  videoRef: Ref<HTMLVideoElement | null>
  currentStream: Ref<StreamResult | null>
  attemptPlay: () => void
  /** Late-bound: same-episode position-preserving re-resolve (resolution cluster). */
  resolveStreamForCurrentEpisode: () => Promise<void>
}

export function useQualityControl(deps: QualityControlDeps) {
  const { state, engine, videoRef, currentStream } = deps

  const mp4Qualities = computed(() => {
    const s = currentStream.value
    if (!s || s.type !== 'mp4' || !s.qualities) return []
    return s.qualities.filter(
      (q) => typeof q.value === 'string' && /^(https?:|\/)/.test(q.value as string),
    )
  })

  const perUrlHlsQualities = computed(() => {
    const s = currentStream.value
    if (!s || s.type !== 'hls' || !s.qualities) return []
    return s.qualities.filter((q) => typeof q.value === 'number')
  })

  const qualities = computed(() => {
    if (mp4Qualities.value.length > 1) return ['Auto', ...mp4Qualities.value.map((q) => q.label)]
    if (perUrlHlsQualities.value.length > 1) {
      return ['Auto', ...perUrlHlsQualities.value.map((q) => q.label)]
    }
    return ['Auto', ...engine.levels.value.map((l) => l.label)]
  })

  // While auto-switching, show what is actually playing: hls.js's current level,
  // or for per-URL ladders the quality the provider reported serving.
  const qualityDisplay = computed(() => {
    const served = engine.currentLevelLabel.value || currentStream.value?.qualityLabel
    return state.quality.value === 'Auto' && served
      ? `Auto · ${served}`
      : state.quality.value
  })

  // New stream may not offer the previously-chosen quality — snap back to Auto.
  // If it DOES offer it, re-apply: each load() creates a fresh hls instance
  // that starts at auto, so a pinned level must be re-pinned. (Per-URL ladders
  // need no re-apply — the resolved URL already carries the pinned quality.)
  watch(qualities, (qs) => {
    if (!qs.includes(state.quality.value)) {
      state.quality.value = 'Auto'
    } else if (
      state.quality.value !== 'Auto' &&
      mp4Qualities.value.length === 0 &&
      perUrlHlsQualities.value.length === 0
    ) {
      engine.setLevel(state.quality.value)
    }
  })

  function swapMp4Source(url: string) {
    const v = videoRef.value
    if (!v) return
    const t = v.currentTime
    const wasPlaying = !v.paused
    v.addEventListener(
      'loadedmetadata',
      () => {
        v.currentTime = t
        if (wasPlaying) deps.attemptPlay()
      },
      { once: true },
    )
    v.src = url
  }

  function onSetQuality(q: string) {
    state.quality.value = q
    const mq = mp4Qualities.value.find((x) => x.label === q)
    if (mq) {
      swapMp4Source(mq.value as string)
      return
    }
    if (q === 'Auto' && currentStream.value?.type === 'mp4') {
      // mp4 has no auto ladder — Auto = the originally-resolved URL
      swapMp4Source(currentStream.value.url)
      return
    }
    if (perUrlHlsQualities.value.length > 0) {
      // Per-URL ladder (Kodik): persist the choice (the adapter reads it on the
      // next resolve), then re-resolve the stream at the new quality in place.
      const pq = perUrlHlsQualities.value.find((x) => x.label === q)
      if (pq) localStorage.setItem(KODIK_QUALITY_PREF_KEY, String(pq.value))
      else if (q === 'Auto') localStorage.removeItem(KODIK_QUALITY_PREF_KEY)
      // Same episode, new manifest URL → resolveStreamForCurrentEpisode keeps the
      // playhead (keepPosition) so the quality swap is seamless.
      void deps.resolveStreamForCurrentEpisode()
      return
    }
    engine.setLevel(q)
  }

  return { qualities, qualityDisplay, onSetQuality }
}
