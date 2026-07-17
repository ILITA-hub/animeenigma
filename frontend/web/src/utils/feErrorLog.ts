/**
 * Frontend error → backend log pipeline.
 *
 * Captures FE failures (uncaught JS errors, unhandled promise rejections, Vue
 * component errors, failed HTTP responses, explicit player-source failures) and
 * ships them to the backend purely as LOGS via POST /api/analytics/client-errors
 * (the analytics service emits one structured Warnw line per error → ClickHouse
 * otel_logs / Grafana). This is NOT the /admin/feedback report path.
 *
 * Volume control is mandatory: a single broken stream can fire the same error
 * 10+ times. We dedup by signature and periodically emit one `suppressed`
 * summary instead of one line per occurrence, and hard-cap per-minute /
 * per-session volume.
 */

import { isMaskedAnalyticsUrl, shipAnalyticsPayload } from './analyticsTransport'

export type FeErrorKind =
  | 'js'
  | 'unhandledrejection'
  | 'vue'
  | 'http'
  | 'player'
  | 'suppressed'
  | 'cap'

export interface FeErrorEvent {
  kind: FeErrorKind
  message: string
  stack?: string
  source?: string
  url?: string
  path?: string
  method?: string
  status?: number
  provider?: string
  anime_id?: string
  count?: number
  ts: string
}

const ENABLED = import.meta.env.VITE_FE_ERROR_LOG_ENABLED !== 'false'

const MAX_BATCH = 20
const FLUSH_MS = 5000
const RATE_PER_MIN = 20
const SESSION_CAP = 100
const MAX_MESSAGE = 500
const MAX_STACK = 2000

// --- module state -----------------------------------------------------------
let buf: FeErrorEvent[] = []
let timer: ReturnType<typeof setInterval> | null = null
let pagehideArmed = false

// dedup: signature -> running totals. `total` is every occurrence; `reported`
// is how many we've already shipped (the first real line + any summaries).
const dedup = new Map<string, { total: number; reported: number; sample: FeErrorEvent }>()

let minuteCount = 0
let minuteResetAt = 0
let sessionCount = 0
let capMarkerSentThisMinute = false

function clampLen(s: string | undefined, n: number): string {
  if (!s) return ''
  return s.length <= n ? s : s.slice(0, n)
}

function firstStackFrame(stack?: string): string {
  if (!stack) return ''
  const line = stack.split('\n').find((l) => l.includes('at ') || l.includes('@')) || ''
  return line.trim().slice(0, 120)
}

function signature(e: FeErrorEvent): string {
  return `${e.kind}|${e.message}|${e.status ?? ''}|${firstStackFrame(e.stack)}`
}

// isOwnTraffic prevents an infinite loop: a failure of the beacon endpoint (or
// the clickstream collector) must never itself be reported as an FE error.
function isOwnTraffic(url?: string): boolean {
  if (!url) return false
  return (
    url.includes('/analytics/client-errors') ||
    url.includes('/analytics/collect') ||
    isMaskedAnalyticsUrl(url)
  )
}

function nowMs(): number {
  return Date.now()
}

/** Resets per-minute rate accounting when the window rolls over. */
function rollMinute(): void {
  const t = nowMs()
  if (t - minuteResetAt >= 60_000) {
    minuteResetAt = t
    minuteCount = 0
    capMarkerSentThisMinute = false
  }
}

/**
 * pushes an event onto the buffer if rate/session caps allow. When capped,
 * emits a single `cap` marker for the minute so the gap is visible in logs.
 */
function tryPush(e: FeErrorEvent): void {
  rollMinute()
  if (sessionCount >= SESSION_CAP || minuteCount >= RATE_PER_MIN) {
    if (!capMarkerSentThisMinute) {
      capMarkerSentThisMinute = true
      buf.push({
        kind: 'cap',
        message: 'fe error rate cap reached — further errors dropped this minute',
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
  if (buf.length >= MAX_BATCH) flushFeErrors('size')
}

/** Arms the periodic flush timer + pagehide flush once, lazily. */
function armLifecycle(): void {
  if (!timer && typeof setInterval !== 'undefined') {
    timer = setInterval(() => {
      emitSuppressedSummaries()
      flushFeErrors('interval')
    }, FLUSH_MS)
  }
  if (!pagehideArmed && typeof window !== 'undefined' && window.addEventListener) {
    pagehideArmed = true
    window.addEventListener('pagehide', () => {
      emitSuppressedSummaries()
      flushFeErrors('pagehide')
    })
  }
}

/**
 * For every signature seen more than its already-reported count, emit ONE
 * `suppressed` summary carrying the delta. Keeps repeated errors to ~1 line per
 * signature per flush interval instead of one per occurrence.
 */
function emitSuppressedSummaries(): void {
  for (const [, rec] of dedup) {
    // delta = occurrences seen since we last shipped a line for this signature
    // (the first occurrence already shipped as a real line). Only summarize
    // when ≥2 repeats accumulated, to avoid a summary line per single repeat.
    const delta = rec.total - rec.reported
    if (delta >= 2) {
      tryPush({
        kind: 'suppressed',
        message: rec.sample.message,
        source: rec.sample.source,
        url: rec.sample.url,
        status: rec.sample.status,
        provider: rec.sample.provider,
        anime_id: rec.sample.anime_id,
        count: delta,
        ts: new Date().toISOString(),
      })
    }
    rec.reported = rec.total
  }
}

/**
 * Public entry point. Records a frontend error for backend logging. Safe to
 * call before init (lazily arms the flush loop) and a no-op when disabled.
 */
export function reportFeError(partial: Partial<FeErrorEvent> & { kind: FeErrorKind }): void {
  if (!ENABLED) return
  try {
    const url = clampLen(partial.url, MAX_MESSAGE)
    if (isOwnTraffic(url)) return

    const e: FeErrorEvent = {
      kind: partial.kind,
      message: clampLen(partial.message, MAX_MESSAGE) || '(no message)',
      stack: clampLen(partial.stack, MAX_STACK) || undefined,
      source: partial.source,
      url: url || undefined,
      path:
        clampLen(partial.path, MAX_MESSAGE) ||
        (typeof location !== 'undefined' ? location.pathname : undefined),
      method: partial.method,
      status: partial.status,
      provider: partial.provider,
      anime_id: partial.anime_id,
      ts: new Date().toISOString(),
    }

    const sig = signature(e)
    const rec = dedup.get(sig)
    if (rec) {
      rec.total++
      // Repeat occurrence: counted now, shipped later as a `suppressed` summary.
      return
    }
    dedup.set(sig, { total: 1, reported: 1, sample: e })
    tryPush(e)
  } catch {
    // Error logging must never throw into the caller's path.
  }
}

/** Flushes the buffer via fetch keepalive (masked-alias retry on block —
 *  shipAnalyticsPayload owns the transport, AUTO-629). */
export function flushFeErrors(_reason = 'manual'): void {
  if (buf.length === 0) return
  const errors = buf
  buf = []
  shipAnalyticsPayload(
    'client-errors',
    JSON.stringify({
      errors,
      ctx: { user_agent: typeof navigator !== 'undefined' ? navigator.userAgent : '' },
    }),
  )
}

/**
 * Installs the global window-level traps (uncaught errors). Call once at
 * startup. Unhandled rejections are reported from main.ts's existing handler so
 * its chunk-reload recovery runs first. No-op when disabled.
 */
export function installFeErrorTraps(): void {
  if (!ENABLED || typeof window === 'undefined' || !window.addEventListener) return
  window.addEventListener('error', (ev: ErrorEvent) => {
    // Resource-load errors (img/script) surface here with no `error` object and
    // a generic message — skip them; only report real script errors.
    if (!ev.error && !ev.message) return
    reportFeError({
      kind: 'js',
      message: ev.message || String(ev.error),
      stack: ev.error?.stack,
      url: ev.filename || (typeof location !== 'undefined' ? location.href : undefined),
    })
  })
  armLifecycle()
}

/** Test-only: clears all internal state. */
export function __resetFeErrorLogForTest(): void {
  buf = []
  dedup.clear()
  minuteCount = 0
  minuteResetAt = 0
  sessionCount = 0
  capMarkerSentThisMinute = false
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

/** Test-only: peek at the pending buffer. */
export function __getFeErrorBufferForTest(): FeErrorEvent[] {
  return buf.slice()
}

/** Test-only: force the periodic suppressed-summary pass. */
export function __runSuppressedPassForTest(): void {
  emitSuppressedSummaries()
}
