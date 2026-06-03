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
})
