// Transport batches events and ships them as one envelope via sendBeacon
// (fetch+keepalive fallback). Mirrors the useWatchSession.ts beacon pattern.
import type { AnalyticsConfig, AnalyticsEnvelope, AnalyticsEvent } from './types'
import { getAnonId, getUserId } from './identity'
import { getSessionId } from './session'

const BEACON_LIMIT = 60 * 1024 // stay under the ~64 KB sendBeacon cap

export class Transport {
  private buf: AnalyticsEvent[] = []
  private timer: ReturnType<typeof setInterval> | null = null
  private readonly endpoint: string
  private readonly maxBatch: number
  private readonly flushMs: number

  constructor(cfg: AnalyticsConfig) {
    this.endpoint = cfg.endpoint
    this.maxBatch = cfg.maxBatch ?? 20
    this.flushMs = cfg.flushMs ?? 5000
  }

  enqueue(e: AnalyticsEvent): void {
    this.buf.push(e)
    if (this.buf.length >= this.maxBatch) this.flush('size')
  }

  startAutoFlush(): void {
    if (this.timer) return
    this.timer = setInterval(() => this.flush('interval'), this.flushMs)
  }

  stopAutoFlush(): void {
    if (this.timer) {
      clearInterval(this.timer)
      this.timer = null
    }
  }

  flush(_reason: string): void {
    if (this.buf.length === 0) return
    const events = this.buf
    this.buf = []
    const envelope: AnalyticsEnvelope = {
      anonymous_id: getAnonId(),
      user_id: getUserId(),
      session_id: getSessionId(),
      events,
      ctx: {
        user_agent: navigator.userAgent,
        screen_w: window.screen?.width ?? 0,
        screen_h: window.screen?.height ?? 0,
      },
    }
    this.send(envelope)
  }

  private send(envelope: AnalyticsEnvelope): void {
    const payload = JSON.stringify(envelope)
    // Oversized batch: split events in half and recurse.
    if (payload.length > BEACON_LIMIT && envelope.events.length > 1) {
      const mid = Math.ceil(envelope.events.length / 2)
      this.send({ ...envelope, events: envelope.events.slice(0, mid) })
      this.send({ ...envelope, events: envelope.events.slice(mid) })
      return
    }
    // text/plain keeps it a CORS "simple" request (no preflight); the backend
    // reads the raw body regardless of content-type.
    try {
      const blob = new Blob([payload], { type: 'text/plain' })
      const ok = navigator.sendBeacon(this.endpoint, blob)
      if (ok) return
    } catch {
      // fall through to fetch
    }
    void fetch(this.endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: payload,
      keepalive: true,
      credentials: 'include',
    }).catch(() => undefined)
  }
}
