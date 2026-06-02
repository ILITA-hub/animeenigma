// Public analytics API. A single module-level instance is initialized once at
// app bootstrap. Before init(), all methods are no-ops (consent/flag gate is
// applied by the caller in main.ts).
import type { AnalyticsConfig, AnalyticsEvent, EventType } from './types'
import { Transport } from './transport'
import { extractClick } from './autocapture'
import { setUserId, clearUserId, resetAnon } from './identity'

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
      this.enqueue({ event_type: 'click', timestamp: nowISO(), path: location.pathname, ...desc })
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
    this.enqueue({ event_type: 'custom', event_name: name, timestamp: nowISO(), path: location.pathname, properties: props })
  }

  identify(userId: string): void {
    if (!userId) return
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

// type re-export for callers
export type { EventType }

export const analytics = new Analytics()
