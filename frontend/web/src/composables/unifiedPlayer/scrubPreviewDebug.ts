import { reactive } from 'vue'

/**
 * scrubPreviewDebug — shared debug channel for the scrub-bar thumbnail engine.
 *
 * Why this exists: "preview shows stale frames" has two very different causes
 * that look identical on screen — (a) frontend misbehavior (pump wedged, hls
 * shadow instance died silently, hover never settles) vs (b) provider
 * propagation delay (each low-level fragment takes seconds through the HLS
 * proxy, so prefetch is simply still in flight). This channel records enough
 * to tell them apart:
 *
 *   - per-seek latency (seek issued → frame captured) — the end-to-end cost
 *   - per-fragment network time + size — the pure provider/proxy cost
 *   - watchdog timeouts — seeks the provider never answered
 *   - hls error events — the previously-UNOBSERVED silent death of the
 *     shadow engine (fatal error ⇒ every later seek no-ops)
 *   - hover hit/miss + cache/queue gauges — pump health
 *
 * ScrubPreview.vue writes; DebugHud.vue renders the PREVIEW section.
 * Console mirroring (prefix `[ScrubPreview]`): enable hacker mode in player
 * settings, or set `localStorage.ae_scrub_debug = "1"` and reload.
 */

export interface ScrubDebugState {
  /** mirror events to the browser console (driven by hacker mode) */
  console: boolean
  /** idle | loading | ready | error — coarse engine lifecycle */
  engine: 'idle' | 'loading' | 'ready' | 'error'
  streamType: string
  cacheSize: number
  queueLen: number
  seeks: number
  captures: number
  watchdogs: number
  hoverHits: number
  hoverMisses: number
  /** last seek-issue → frame-capture latency, ms */
  lastCaptureMs: number | null
  /** rolling average capture latency, ms */
  avgCaptureMs: number
  lastFragMs: number | null
  lastFragKb: number | null
  errors: number
  lastError: string
  /** timestamped ring buffer of the last events (newest last) */
  events: string[]
}

const EVENTS_MAX = 40

function freshState(): Omit<ScrubDebugState, 'console'> {
  return {
    engine: 'idle',
    streamType: '',
    cacheSize: 0,
    queueLen: 0,
    seeks: 0,
    captures: 0,
    watchdogs: 0,
    hoverHits: 0,
    hoverMisses: 0,
    lastCaptureMs: null,
    avgCaptureMs: 0,
    lastFragMs: null,
    lastFragKb: null,
    errors: 0,
    lastError: '',
    events: [],
  }
}

export const scrubDebug: ScrubDebugState = reactive({ console: false, ...freshState() })

let epoch = typeof performance !== 'undefined' ? performance.now() : 0

let consoleForced = false
try {
  consoleForced =
    typeof localStorage !== 'undefined' && localStorage.getItem('ae_scrub_debug') === '1'
} catch {
  /* storage blocked — console stays hacker-mode-only */
}

/** Log one event: ring buffer always, console when enabled. */
export function slog(msg: string): void {
  const t = typeof performance !== 'undefined' ? performance.now() : 0
  const line = `${((t - epoch) / 1000).toFixed(1)}s ${msg}`
  scrubDebug.events.push(line)
  if (scrubDebug.events.length > EVENTS_MAX) scrubDebug.events.shift()
  if (scrubDebug.console || consoleForced) {
    // eslint-disable-next-line no-console
    console.info('[ScrubPreview]', line)
  }
}

/** Record a completed seek→capture round-trip. */
export function srecordCapture(ms: number | null): void {
  scrubDebug.captures++
  if (ms !== null) {
    scrubDebug.lastCaptureMs = Math.round(ms)
    scrubDebug.avgCaptureMs = Math.round(
      scrubDebug.avgCaptureMs === 0 ? ms : scrubDebug.avgCaptureMs * 0.7 + ms * 0.3,
    )
  }
}

/** Reset on engine (re)init — a new stream gets a clean ledger. */
export function sreset(): void {
  epoch = typeof performance !== 'undefined' ? performance.now() : 0
  Object.assign(scrubDebug, freshState())
}
