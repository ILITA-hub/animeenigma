import { describe, it, expect, beforeEach } from 'vitest'
import { registerClickForTrace, stampTrace, _resetForTest } from '../traceContext'
import type { AnalyticsEvent } from '../types'

function clickEvent(): AnalyticsEvent {
  return { event_type: 'click', timestamp: new Date().toISOString(), path: '/x' }
}

describe('traceContext', () => {
  beforeEach(() => _resetForTest())

  it('stamps a registered click event that has no trace_id yet', () => {
    const e = clickEvent()
    registerClickForTrace(e)
    stampTrace('abc123', 1500)
    expect(e.trace_id).toBe('abc123')
  })

  it('does not overwrite a trace_id that is already set', () => {
    const e = clickEvent()
    e.trace_id = 'first'
    registerClickForTrace(e)
    stampTrace('second', 1500)
    expect(e.trace_id).toBe('first')
  })

  it('ignores clicks older than the window', () => {
    const e = clickEvent()
    registerClickForTrace(e, Date.now() - 5000) // 5s ago
    stampTrace('late', 1500, Date.now())
    expect(e.trace_id).toBeUndefined()
  })

  // AR-FE-02 end-to-end proof: the trace_id minted by the axios interceptor for
  // a call (and emitted on the source='fe' call row, Plan 03 Task 1) is the SAME
  // id back-filled onto the click that triggered the call — so the click, the FE
  // call, and the downstream BE effects all join on one trace_id.
  describe('click↔call trace stamp (AR-FE-02)', () => {
    // The interceptor's trace_id (matches newTraceparent()'s 32-hex shape).
    const CALL_TRACE_ID = 'aabbccddeeff00112233445566778899'

    it('back-fills the click with the call trace_id when stamped within the window', () => {
      const t0 = 1_000_000
      const click = clickEvent()
      // Click registered at t0 (no trace_id yet — clicks ship un-stamped).
      registerClickForTrace(click, t0)
      // The API call the click triggered fires 800ms later, still in the 1500ms
      // window; the interceptor calls stampTrace with the call's trace_id.
      stampTrace(CALL_TRACE_ID, 1500, t0 + 800)
      // The click now carries the call's trace_id — they share one trace_id.
      expect(click.trace_id).toBe(CALL_TRACE_ID)
    })

    it('does NOT back-fill when the call fires outside the 1500ms window', () => {
      const t0 = 1_000_000
      const click = clickEvent()
      registerClickForTrace(click, t0)
      // Call fires 2000ms later — past the window; no association.
      stampTrace(CALL_TRACE_ID, 1500, t0 + 2000)
      expect(click.trace_id).toBeUndefined()
    })
  })
})
