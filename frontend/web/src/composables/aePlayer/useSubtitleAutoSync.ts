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
  let locked = false
  let tap: SpeechTap | null = null

  function resetData() {
    speech = []; openStart = null; lastT = -Infinity; totalSpeech = 0; locked = false
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
    autoOffset.value = offset; confidence.value = conf; locked = true; status.value = 'locked'
    // Clear the speech buffer after a lock/resync so subsequent evaluations use
    // only fresh speech — prevents old aligned segments from suppressing the
    // confidence of future re-syncs when the offset shifts mid-episode.
    speech = []; totalSpeech = 0
  }

  function pickBest(ivl: Interval[]) {
    // Primary: full search range — handles large legitimate drifts.
    const r = bestOffset(speech, ivl, SEARCH)
    if (r.confidence >= cfg.confMin) return r

    // Fallback: when the full range is ambiguous (e.g. aliased by a distant cue
    // whose shifted position coincidentally matches speech), restrict to a ±10 s
    // neighbourhood anchored on the expected offset (0 before first lock, the
    // current autoOffset after).  This breaks ties toward the smallest plausible
    // shift — a conservative bias that is still overridden by a high-confidence
    // full-range result.
    const anchor = locked ? autoOffset.value : 0
    const near = { min: anchor - 10, max: anchor + 10, step: SEARCH.step }
    const rNear = bestOffset(speech, ivl, near)
    return rNear.confidence > r.confidence ? rNear : r
  }

  function evaluate() {
    if (!locked && totalSpeech < cfg.minSpeech) return    // skip the sweep until warmed up
    if (!speech.length || !cueIntervals.value.length) return
    const chosen = pickBest(cueIntervals.value)
    if (!locked) {
      if (chosen.confidence >= cfg.confMin) apply(chosen.offset, chosen.confidence, 'lock')
    } else if (chosen.confidence >= cfg.confMin && Math.abs(chosen.offset - autoOffset.value) >= cfg.resyncDelta) {
      apply(chosen.offset, chosen.confidence, 'resync')
    }
  }

  function ingest(t: number, speaking: boolean) {
    // Detect seeks: backward jump always closes open segment; forward gap only
    // resets when we are NOT inside an open speech segment.  During speech,
    // frames can arrive > seekGapSec apart (sparse VAD or coarse test sampling)
    // without the user actually seeking — closing the segment prematurely would
    // fragment it into zero-length pieces and under-count totalSpeech.
    const isBackwardSeek = t < lastT
    const isForwardSeek  = openStart === null && t - lastT > cfg.seekGapSec
    if (isBackwardSeek || isForwardSeek) {
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

  function startTap() {
    if (tap || !opts.videoElement.value) return
    try { tap = makeTap(opts.videoElement.value); tap.onFrame(ingest); status.value = 'listening' }
    catch { status.value = 'unsupported' }
  }
  function stopTap() { tap?.dispose(); tap = null }

  function arm() {
    if (opts.enabled.value && opts.videoElement.value) {
      if (status.value !== 'unsupported') startTap()       // startTap owns the listening/unsupported transition
    } else {
      stopTap(); resetData(); status.value = 'idle'
    }
  }

  watch(opts.episodeKey, () => { stopTap(); resetData(); status.value = 'idle'; arm() })
  watch([opts.enabled, opts.videoElement], arm, { immediate: true })
  watch(cueIntervals, () => { if (!locked) evaluate() })   // cues may arrive after speech
  onUnmounted(stopTap)

  return { autoOffset, status, confidence, syncEvents }
}
