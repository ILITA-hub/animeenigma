// Analytics clickstream types. The envelope/event shapes mirror Plan 1's
// backend wire contract (services/analytics handler/collect.go wireEnvelope).
export type EventType = 'pageview' | 'click' | 'heartbeat' | 'identify' | 'custom'

export interface AnalyticsEvent {
  event_type: EventType
  event_name?: string
  timestamp: string // ISO 8601
  url?: string
  path?: string
  referrer?: string
  title?: string
  el_selector?: string
  el_text?: string
  el_tag?: string
  el_attrs?: Record<string, string>
  active_ms?: number
  trace_id?: string // stamped by the axios interceptor (Plan 3), links click → backend trace
  properties?: Record<string, unknown>
}

export interface AnalyticsContext {
  user_agent: string
  screen_w: number
  screen_h: number
}

export interface AnalyticsEnvelope {
  anonymous_id: string
  user_id: string | null
  session_id: string
  events: AnalyticsEvent[]
  ctx: AnalyticsContext
}

export interface AnalyticsConfig {
  endpoint: string // full URL of POST /api/analytics/collect
  heartbeatMs?: number // default 15000
  flushMs?: number // default 5000
  maxBatch?: number // default 20
}
