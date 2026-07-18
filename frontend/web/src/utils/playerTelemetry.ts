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

import { shipAnalyticsPayload } from './analyticsTransport'

export interface PlayerEvent {
  kind: 'resolve' | 'stall' | 'playback_start_rejected' | 'playback_failed' | 'protocol_usage' | 'skip_used'
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
  /** Rich diagnostic bundle for kind:'playback_failed' — serialized verbatim. */
  detail?: Record<string, unknown>
  ts?: string
}

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
 * kind is one of the allowed kinds, then rate-caps and buffers.
 * Never throws.
 */
export function recordPlayerEvent(e: PlayerEvent): void {
  try {
    if (!e) return
    if (!e.provider || !e.provider.trim()) return
    if (
      e.kind !== 'resolve' &&
      e.kind !== 'stall' &&
      e.kind !== 'playback_start_rejected' &&
      e.kind !== 'playback_failed' &&
      e.kind !== 'protocol_usage' &&
      e.kind !== 'skip_used'
    )
      return

    const event: PlayerEvent = {
      ...e,
      ts: new Date().toISOString(),
    }
    tryPush(event)
  } catch {
    // Telemetry must never throw into the caller's path.
  }
}

/** Flushes the buffer via fetch keepalive (masked-alias retry on block —
 *  shipAnalyticsPayload owns the transport, AUTO-629). */
export function flushPlayerTelemetry(_reason = 'manual'): void {
  if (buf.length === 0) return
  const events = buf
  buf = []
  shipAnalyticsPayload('player-events', JSON.stringify({ events }))
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
