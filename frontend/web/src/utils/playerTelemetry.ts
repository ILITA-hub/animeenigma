/**
 * Player telemetry beacon — Stage 2a, Smart Source Selection.
 *
 * Buffers player events (resolve / stall) and ships them in batches to
 * POST /api/analytics/player-events via sendBeacon → fetch keepalive fallback.
 *
 * Design mirrors feErrorLog.ts: same buffer+timer+pagehide lifecycle, same
 * rate-cap (tryPush), same never-throw contract. Body shape differs:
 * { events: PlayerEvent[] } instead of { errors, ctx }.
 *
 * DO NOT use this for user-facing flows — telemetry must be invisible to the app.
 */

export interface PlayerEvent {
  kind: 'resolve' | 'stall' | 'playback_start_rejected'
  provider: string
  anime_id: string
  episode?: number
  outcome?: 'ok' | 'fail'
  reached_playback?: boolean
  error_kind?: string
  latency_ms?: number
  stall_ms?: number
  audio?: string
  lang?: string
  ts?: string
}

const ENDPOINT = `${import.meta.env.VITE_API_URL || '/api'}/analytics/player-events`

const MAX_BATCH = 20
const FLUSH_MS = 5000
const RATE_PER_MIN = 60
const SESSION_CAP = 500

// --- module state -----------------------------------------------------------
let buf: PlayerEvent[] = []
let timer: ReturnType<typeof setInterval> | null = null
let pagehideArmed = false

let minuteCount = 0
let minuteResetAt = 0
let sessionCount = 0
let capMarkerSentThisMinute = false

function nowMs(): number {
  return Date.now()
}

/** Resets per-minute rate window when it rolls over. */
function rollMinute(): void {
  const t = nowMs()
  if (t - minuteResetAt >= 60_000) {
    minuteResetAt = t
    minuteCount = 0
    capMarkerSentThisMinute = false
  }
}

/**
 * Pushes an event onto the buffer if rate/session caps allow.
 * When capped, emits a single sentinel cap marker for the minute.
 */
function tryPush(e: PlayerEvent): void {
  rollMinute()
  if (sessionCount >= SESSION_CAP || minuteCount >= RATE_PER_MIN) {
    if (!capMarkerSentThisMinute) {
      capMarkerSentThisMinute = true
      buf.push({
        kind: 'resolve',
        provider: '__cap__',
        anime_id: '',
        error_kind: 'player telemetry rate cap reached — further events dropped this minute',
        ts: new Date().toISOString(),
      })
      maybeFlushBySize()
    }
    return
  }
  minuteCount++
  sessionCount++
  buf.push(e)
  maybeFlushBySize()
}

function maybeFlushBySize(): void {
  armLifecycle()
  if (buf.length >= MAX_BATCH) flushPlayerTelemetry('size')
}

/** Arms the periodic flush timer + pagehide handler once, lazily. */
function armLifecycle(): void {
  if (!timer && typeof setInterval !== 'undefined') {
    timer = setInterval(() => {
      flushPlayerTelemetry('interval')
    }, FLUSH_MS)
  }
  if (!pagehideArmed && typeof window !== 'undefined' && window.addEventListener) {
    pagehideArmed = true
    window.addEventListener('pagehide', () => {
      flushPlayerTelemetry('pagehide')
    })
  }
}

/**
 * Records a player telemetry event. Validates that provider is non-empty and
 * kind is one of the two allowed values, then rate-caps and buffers.
 * Never throws.
 */
export function recordPlayerEvent(e: PlayerEvent): void {
  try {
    if (!e) return
    if (!e.provider || !e.provider.trim()) return
    if (e.kind !== 'resolve' && e.kind !== 'stall' && e.kind !== 'playback_start_rejected') return

    const event: PlayerEvent = {
      ...e,
      ts: new Date().toISOString(),
    }
    tryPush(event)
  } catch {
    // Telemetry must never throw into the caller's path.
  }
}

/** Flushes the buffer to the backend via sendBeacon (fetch keepalive fallback). */
export function flushPlayerTelemetry(_reason = 'manual'): void {
  if (buf.length === 0) return
  const events = buf
  buf = []
  const payload = JSON.stringify({ events })
  try {
    const blob = new Blob([payload], { type: 'text/plain' })
    if (typeof navigator !== 'undefined' && navigator.sendBeacon && navigator.sendBeacon(ENDPOINT, blob)) {
      return
    }
  } catch {
    // fall through to fetch
  }
  try {
    void fetch(ENDPOINT, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: payload,
      keepalive: true,
      credentials: 'include',
    }).catch(() => undefined)
  } catch {
    // give up silently — telemetry must never break the app
  }
}

/** Test-only: clears all internal state including timers. */
export function __resetPlayerTelemetryForTest(): void {
  buf = []
  minuteCount = 0
  minuteResetAt = 0
  sessionCount = 0
  capMarkerSentThisMinute = false
  pagehideArmed = false
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}
