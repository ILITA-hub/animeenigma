// Public analytics API. A single module-level instance is initialized once at
// app bootstrap. Before init(), all methods are no-ops (consent/flag gate is
// applied by the caller in main.ts).
import type { AnalyticsConfig, AnalyticsEvent, EventType } from './types'
import { Transport } from './transport'
import { extractClick } from './autocapture'
import { getUserId, setUserId, clearUserId, resetAnon } from './identity'
import { registerClickForTrace } from './traceContext'
import { initRum } from './rum'

class Analytics {
  private transport: Transport | null = null
  private heartbeatMs = 15000
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null
  private lastBeat = 0
  private clickListener: ((e: MouseEvent) => void) | null = null
  private visibilityListener: (() => void) | null = null
  private pagehideListener: (() => void) | null = null

  init(cfg: AnalyticsConfig): void {
    if (this.transport) return // idempotent
    this.transport = new Transport(cfg)
    this.heartbeatMs = cfg.heartbeatMs ?? 15000
    this.transport.startAutoFlush()

    // Autocapture clicks via one delegated listener.
    this.clickListener = (e: MouseEvent) => {
      const target = e.target as Element | null
      if (!target) return
      const desc = extractClick(target)
      if (!desc) return
      const evt = { event_type: 'click' as const, timestamp: nowISO(), path: location.pathname, ...desc }
      this.enqueue(evt)
      // Best-effort: the next API call within ~1.5s back-fills evt.trace_id.
      registerClickForTrace(evt)
    }
    document.addEventListener('click', this.clickListener, { capture: true })

    // Heartbeat while foregrounded.
    this.lastBeat = Date.now()
    this.startHeartbeat()
    this.visibilityListener = () => {
      if (document.visibilityState === 'hidden') {
        this.stopHeartbeat()
        this.flushNow()
      } else {
        this.lastBeat = Date.now()
        this.startHeartbeat()
      }
    }
    document.addEventListener('visibilitychange', this.visibilityListener)

    this.pagehideListener = () => this.flushNow()
    window.addEventListener('pagehide', this.pagehideListener)

    // RUM resource-timing observer (browser→3rd-party host timings, byte-poor).
    initRum()

    // Initial pageview.
    this.page()
  }

  page(props?: Record<string, unknown>): void {
    this.enqueue({
      event_type: 'pageview',
      timestamp: nowISO(),
      url: location.href,
      path: location.pathname,
      referrer: document.referrer,
      title: document.title,
      properties: props,
    })
  }

  track(name: string, props?: Record<string, unknown>): void {
    // Lift activity-register fields (source, trace_id, operation, target, …) to
    // the TOP LEVEL of the event so the collector's wireEvent reads them directly
    // (AR-FE-01/AR-FE-03). Without this, register fields stay buried in
    // `properties` and the FE→BE trace_id join returns 0 rows. Arbitrary
    // user-supplied props stay under `properties`.
    const { register, rest } = liftRegisterFields(props)
    this.enqueue({
      event_type: 'custom',
      event_name: name,
      timestamp: nowISO(),
      path: location.pathname,
      ...register,
      properties: rest,
    })
  }

  identify(userId: string): void {
    if (!userId || userId === getUserId()) return
    setUserId(userId)
    this.enqueue({ event_type: 'identify', timestamp: nowISO(), path: location.pathname })
  }

  reset(): void {
    clearUserId()
    resetAnon()
  }

  flushNow(): void {
    this.transport?.flush('manual')
  }

  private enqueue(e: AnalyticsEvent): void {
    if (!this.transport) return
    this.transport.enqueue(e)
  }

  private startHeartbeat(): void {
    if (this.heartbeatTimer || !this.transport) return
    this.heartbeatTimer = setInterval(() => {
      const now = Date.now()
      const active = now - this.lastBeat
      this.lastBeat = now
      this.enqueue({ event_type: 'heartbeat', timestamp: nowISO(), path: location.pathname, active_ms: active })
    }, this.heartbeatMs)
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
  }
}

function nowISO(): string {
  return new Date().toISOString()
}

// The activity-register keys the collector's wireEvent reads at the TOP LEVEL of
// an analytics event (services/analytics handler/collect.go). These must NOT be
// buried under `properties` or the FE→BE trace_id join returns 0 rows. `route`
// is FE-only context the collector folds into operation; it is carried top-level
// too since the collector tolerates it. Keep this list in sync with the
// wireEvent + AnalyticsEvent register fields.
const REGISTER_KEYS = [
  'source',
  'trace_id',
  'operation',
  'action',
  'route',
  'target',
  'target_kind',
  'requests',
  'duration_ms',
] as const

// liftRegisterFields splits a props bag into the typed register fields (lifted to
// the event top level) and the remaining arbitrary props (kept under
// `properties`). Returns rest=undefined when nothing arbitrary remains so the
// serialized event omits an empty properties object.
function liftRegisterFields(props?: Record<string, unknown>): {
  register: Partial<AnalyticsEvent>
  rest: Record<string, unknown> | undefined
} {
  if (!props) return { register: {}, rest: undefined }
  const register: Record<string, unknown> = {}
  const rest: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(props)) {
    if (v === undefined) continue // skip undefined so JSON omits the key entirely
    if ((REGISTER_KEYS as readonly string[]).includes(k)) {
      register[k] = v
    } else {
      rest[k] = v
    }
  }
  return {
    register: register as Partial<AnalyticsEvent>,
    rest: Object.keys(rest).length > 0 ? rest : undefined,
  }
}

// type re-export for callers
export type { EventType }

export const analytics = new Analytics()
