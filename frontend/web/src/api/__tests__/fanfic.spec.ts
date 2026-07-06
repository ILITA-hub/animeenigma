import { describe, it, expect, vi } from 'vitest'
import { parseSSEBuffer, handleSSEEvent } from '../fanfic'

describe('parseSSEBuffer', () => {
  it('extracts complete events and returns the remainder', () => {
    const chunk =
      'event: meta\ndata: {"id":"x","model":"m"}\n\n' +
      'event: delta\ndata: {"text":"# T"}\n\n' +
      'event: delta\ndata: {"text":"partial'
    const { events, rest } = parseSSEBuffer(chunk)
    expect(events).toHaveLength(2)
    expect(events[0]).toEqual({ event: 'meta', data: { id: 'x', model: 'm' } })
    expect(events[1]).toEqual({ event: 'delta', data: { text: '# T' } })
    expect(rest).toContain('partial')
  })

  it('parses multiple complete events with no remainder', () => {
    const chunk =
      'event: done\ndata: {"id":"x","title":"T","token_usage":42}\n\n'
    const { events, rest } = parseSSEBuffer(chunk)
    expect(events).toEqual([
      { event: 'done', data: { id: 'x', title: 'T', token_usage: 42 } },
    ])
    expect(rest).toBe('')
  })

  it('ignores malformed JSON data lines instead of throwing', () => {
    const chunk = 'event: delta\ndata: {not json}\n\n'
    expect(() => parseSSEBuffer(chunk)).not.toThrow()
    const { events } = parseSSEBuffer(chunk)
    expect(events).toHaveLength(0)
  })

  it('defaults to event "message" when no event line is present', () => {
    const chunk = 'data: {"text":"hi"}\n\n'
    const { events } = parseSSEBuffer(chunk)
    expect(events).toEqual([{ event: 'message', data: { text: 'hi' } }])
  })
})

describe('handleSSEEvent', () => {
  it('dispatches deltas in order via onDelta', () => {
    const onDelta = vi.fn()
    handleSSEEvent({ event: 'delta', data: { text: 'hi' } }, { onDelta })
    expect(onDelta).toHaveBeenCalledWith('hi')
  })

  it('dispatches meta via onMeta with (id, model)', () => {
    const onMeta = vi.fn()
    handleSSEEvent({ event: 'meta', data: { id: 'x', model: 'llama' } }, { onMeta })
    expect(onMeta).toHaveBeenCalledWith('x', 'llama')
  })

  it('dispatches done via onDone with (id, title, tokenUsage)', () => {
    const onDone = vi.fn()
    handleSSEEvent(
      { event: 'done', data: { id: 'x', title: 'The Title', token_usage: 123 } },
      { onDone },
    )
    expect(onDone).toHaveBeenCalledWith('x', 'The Title', 123)
  })

  it('dispatches error via onError with (message)', () => {
    const onError = vi.fn()
    handleSSEEvent({ event: 'error', data: { message: 'boom' } }, { onError })
    expect(onError).toHaveBeenCalledWith('boom')
  })

  it('does not throw when the matching handler is not provided', () => {
    expect(() => handleSSEEvent({ event: 'delta', data: { text: 'hi' } }, {})).not.toThrow()
  })

  it('ignores unknown event types', () => {
    const onDelta = vi.fn()
    handleSSEEvent({ event: 'ping', data: {} }, { onDelta })
    expect(onDelta).not.toHaveBeenCalled()
  })
})
