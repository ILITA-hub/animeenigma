import { onUnmounted, watch, type Ref } from 'vue'
import { recordPlayerEvent, flushPlayerTelemetry } from '@/utils/playerTelemetry'
import { ladder, type TierResidency } from '@/utils/protocolLadder'
import { buildProtocolUsageDetail, readVideoQuality, droppedFramesPct } from '@/composables/aePlayer/protocolUsage'
import { probeH3 } from '@/utils/probeH3'
import type { PlayerState } from '@/composables/aePlayer/usePlayerState'
import type { useVideoEngine } from '@/composables/aePlayer/useVideoEngine'
import type { StreamResult } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

// ── Protocol ladder (h1/h2/h3) subscriptions + usage telemetry ───────────────
// Tier-change re-resolve, one anonymous protocol_usage event per
// (session × tier), and the sampled h3 upshift probe.

export interface ProtocolTelemetryDeps {
  videoRef: Ref<HTMLVideoElement | null>
  state: PlayerState
  engine: ReturnType<typeof useVideoEngine>
  currentStream: Ref<StreamResult | null>
  selectedEpisode: Ref<EpisodeOption | null>
  hasStarted: Ref<boolean>
  getAnimeId: () => string
  getAnimeTitle: () => string
  recordDecision: (reason: string) => void
  resolveStreamForEpisode: (ep: EpisodeOption, keepPosition?: boolean) => Promise<void>
}

export function useProtocolTelemetry(deps: ProtocolTelemetryDeps) {
  const { videoRef, state, engine, currentStream, selectedEpisode, hasStarted } = deps

  // Protocol ladder tier-change subscription: a downshift/upshift means the
  // active stream's base origin changed, so re-resolve at the current position
  // to pick up the new tier via hlsProxyUrl (the source itself hasn't changed).
  const unsubLadder = ladder.onChange((tier, reason) => {
    deps.recordDecision(`protocol ladder → ${tier.id} (${reason})`)
    const ep = selectedEpisode.value
    if (ep) void deps.resolveStreamForEpisode(ep, true) // position-preserving swap; new base flows via hlsProxyUrl
  })
  onUnmounted(unsubLadder)

  // One anonymous event per (session × tier): a residency summary from the ladder
  // + the dropped-video-frame delta over that residency. Emitted on every tier
  // switch (onResidencyEnd) and once at session end (pagehide / unmount).
  const telemetrySess =
    's_' +
    (typeof crypto !== 'undefined' && crypto.randomUUID
      ? crypto.randomUUID().slice(0, 8)
      : Math.random().toString(36).slice(2, 10))
  // Cumulative video-quality counters snapshotted at the current tier's start.
  let tierStartQuality = readVideoQuality(null) // { dropped: 0, total: 0 }

  function emitProtocolUsage(r: TierResidency): void {
    const q = readVideoQuality(videoRef.value)
    const pct = droppedFramesPct(tierStartQuality, q)
    tierStartQuality = q // the next tier's residency measures from here
    const c = state.combo.value
    recordPlayerEvent({
      kind: 'protocol_usage',
      provider: c.provider,
      anime_id: deps.getAnimeId(),
      episode: selectedEpisode.value?.number,
      audio: c.audio,
      lang: c.lang,
      detail: buildProtocolUsageDetail(r, pct, {
        animeName: deps.getAnimeTitle(),
        combo: `${c.audio}·${c.lang}·${c.provider}`,
        sess: telemetrySess,
      }),
    })
  }

  // Session-end flush of the final (never-switched-away) tier. consumeResidency
  // dedups a pagehide-then-unmount double call; the explicit flush guarantees the
  // last row ships even if this pagehide listener runs after telemetry's own.
  function flushProtocolUsage(): void {
    const r = ladder.consumeResidency()
    if (r) {
      emitProtocolUsage(r)
      flushPlayerTelemetry('protocol-usage-final')
    }
  }

  // pagehide with persisted=true means the page is entering the bfcache
  // (backgrounding, may be restored) — NOT terminal. Emitting+consuming the
  // residency here would double-count this tier's segments if playback resumes
  // and the tier later switches. Only flush on a real unload (persisted=false).
  function onProtocolUsagePageHide(e: PageTransitionEvent): void {
    if (e.persisted) return
    flushProtocolUsage()
  }

  const unsubResidency = ladder.onResidencyEnd(emitProtocolUsage)
  onUnmounted(unsubResidency)

  // h3 upshift probe: sampled once, 30s after playback actually starts (not on
  // mount — no point probing a stream that hasn't begun loading fragments).
  let h3ProbeTimer: ReturnType<typeof setTimeout> | null = null
  watch(hasStarted, (started) => {
    if (!started || h3ProbeTimer) return
    h3ProbeTimer = setTimeout(() => {
      if (!state.playing.value) return
      // C2: no fragment sample URL means an MP4/native-HLS session (no hls.js
      // XHR fragments ever populate lastFragUrl) — probing with '' resolves to
      // the h3 origin's root, and there's no EWMA baseline to sanity-check the
      // result against either. Leave the timer's one-shot semantics as-is
      // (never re-armed) and just skip this probe entirely.
      if (!engine.lastFragUrl.value) return
      void probeH3(ladder, engine.lastFragUrl.value, currentStream.value?.url ?? '')
    }, 30_000)
  })
  onUnmounted(() => { if (h3ProbeTimer) clearTimeout(h3ProbeTimer) })

  return { flushProtocolUsage, onProtocolUsagePageHide }
}
