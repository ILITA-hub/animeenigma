// frontend/web/src/composables/aePlayer/useSubtitleAutoSync.ts
import { ref, computed, watch, onUnmounted, type Ref } from 'vue'
import { cuesToIntervals, bestOffset, round1, SEARCH, type Interval, type SyncEvent, type SpeechTap } from './subtitleAlign'
import { createAudioTap } from './subtitleAudioTap'

export type { SpeechTap }

export interface AutoSyncConfig {
  minSpeech: number      // seconds of accumulated speech before first lock
  confMin: number        // min peak prominence to act
  resyncDelta: number    // min offset change (s) to adopt a re-sync
  maxEvents: number      // change-log cap
  seekGapSec: number     // frame gap that counts as a seek (don't bridge)
  windowSec: number      // sliding speech-window length kept for correlation
}

export const DEFAULT_AUTOSYNC_CONFIG: AutoSyncConfig = {
  minSpeech: 8, confMin: 0.15, resyncDelta: 0.5, maxEvents: 10, seekGapSec: 1, windowSec: 120,
}

type Status = 'idle' | 'listening' | 'locked' | 'unsupported'

export function useSubtitleAutoSync(opts: {
  videoElement: Ref<HTMLVideoElement | null>
  cues: Ref<{ start: number; end: number }[]>
  enabled: Ref<boolean>
  episodeKey: Ref<string>
  createTap?: (el: HTMLVideoElement) => SpeechTap
  config?: Partial<AutoSyncConfig>
}) {
  const cfg: AutoSyncConfig = { ...DEFAULT_AUTOSYNC_CONFIG, ...(opts.config ?? {}) }
  const makeTap = opts.createTap ?? createAudioTap

  const autoOffset = ref(0)
  const confidence = ref(0)
  const status = ref<Status>('idle')
  const syncEvents = ref<SyncEvent[]>([])

  const cueIntervals = computed<Interval[]>(() => cuesToIntervals(opts.cues.value))

  let speech: Interval[] = []
  let openStart: number | null = null
  let lastT = -Infinity
  let totalSpeech = 0
  let tap: SpeechTap | null = null
  let active = false

  function resetData() {
    speech = []; openStart = null; lastT = -Infinity; totalSpeech = 0
    autoOffset.value = 0; confidence.value = 0; syncEvents.value = []
  }

  function pruneWindow() {
    const cutoff = lastT - cfg.windowSec
    if (speech.length && speech[0].end < cutoff) {
      let i = 0
      while (i < speech.length && speech[i].end < cutoff) i++
      speech = speech.slice(i)
    }
  }

  function apply(offset: number, conf: number, reason: 'lock' | 'resync') {
    if (offset === autoOffset.value) return       // bestOffset already rounds to 0.1
    const ev: SyncEvent = {
      delta: round1(offset - autoOffset.value), confidence: conf,
      windowStart: speech[0]?.start ?? 0, windowEnd: lastT, reason,
    }
    syncEvents.value = [ev, ...syncEvents.value].slice(0, cfg.maxEvents)
    autoOffset.value = offset; confidence.value = conf; status.value = 'locked'
  }

  function evaluate() {
    const locked = status.value === 'locked'   // single source of truth for the lock-vs-resync gate
    if (!locked && totalSpeech < cfg.minSpeech) return    // skip the sweep until warmed up
    if (!speech.length || !cueIntervals.value.length) return
    const r = bestOffset(speech, cueIntervals.value, SEARCH)
    if (!locked) {
      if (r.confidence >= cfg.confMin) apply(r.offset, r.confidence, 'lock')
    } else if (r.confidence >= cfg.confMin && Math.abs(r.offset - autoOffset.value) >= cfg.resyncDelta) {
      apply(r.offset, r.confidence, 'resync')
    }
  }

  function ingest(t: number, speaking: boolean) {
    if (!active) return
    if (t < lastT || t - lastT > cfg.seekGapSec) {        // seek / discontinuity: close, don't bridge
      if (openStart !== null) { speech.push({ start: openStart, end: lastT }); openStart = null }
    }
    if (speaking) {
      if (openStart === null) openStart = t
    } else if (openStart !== null) {
      const seg = { start: openStart, end: t }
      if (seg.end > seg.start) { speech.push(seg); totalSpeech += seg.end - seg.start }
      openStart = null
      pruneWindow()
      evaluate()
    }
    lastT = t
  }

  // The tap analyses a non-interruptive captureStream fork (subtitleAudioTap.ts)
  // and never touches the element's own audio output, so it can NEVER silence
  // playback. It's created once and kept alive purely to avoid re-acquiring the
  // capture on every toggle; disable / episode-change just stop accumulating
  // (active=false). A creation failure (no captureStream — Safari) marks
  // auto-sync 'unsupported'; playback is unaffected in every case.
  function ensureTap() {
    if (tap || !opts.videoElement.value) return
    try { tap = makeTap(opts.videoElement.value); tap.onFrame(ingest) }
    catch { status.value = 'unsupported' }
  }

  function arm() {
    if (opts.enabled.value && opts.videoElement.value) {
      if (status.value === 'unsupported') return
      ensureTap()
      if (tap) { active = true; if (status.value !== 'locked') status.value = 'listening' }
    } else {
      active = false
      resetData()
      status.value = 'idle'
    }
  }

  watch(opts.episodeKey, () => {
    resetData()
    if (status.value !== 'unsupported') status.value = 'idle'
    arm()
  })
  watch([opts.enabled, opts.videoElement], arm, { immediate: true })
  watch(cueIntervals, () => { if (status.value !== 'locked') evaluate() })   // cues may arrive after speech
  onUnmounted(() => { tap?.dispose(); tap = null })

  return { autoOffset, status, confidence, syncEvents }
}
