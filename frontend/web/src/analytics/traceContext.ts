// Best-effort click↔trace association (design spec §2, "v1 honesty"). A click
// is enqueued WITHOUT a trace_id; when the API call it triggers fires, the
// axios interceptor calls stampTrace(traceId), which back-fills the trace_id
// onto recent un-stamped click events. Because flush is delayed (≥5s / size
// 20), the in-place mutation lands before the event is shipped.
import type { AnalyticsEvent } from './types'

interface Pending {
  evt: AnalyticsEvent
  ts: number
}

let pending: Pending[] = []

// registerClickForTrace records a click event so the next API call within the
// window can stamp it. `at` is injectable for tests (defaults to now).
export function registerClickForTrace(evt: AnalyticsEvent, at: number = Date.now()): void {
  pending.push({ evt, ts: at })
  // Bound memory: keep only the most recent 50 entries.
  if (pending.length > 50) pending = pending.slice(-50)
}

// stampTrace assigns traceId to every pending click within `withinMs` that has
// no trace_id yet, then prunes entries older than the window. `now` is
// injectable for tests.
export function stampTrace(traceId: string, withinMs = 1500, now: number = Date.now()): void {
  for (const p of pending) {
    if (!p.evt.trace_id && now - p.ts <= withinMs) {
      p.evt.trace_id = traceId
    }
  }
  pending = pending.filter((p) => now - p.ts <= withinMs)
}

// Test-only reset.
export function _resetForTest(): void {
  pending = []
}
