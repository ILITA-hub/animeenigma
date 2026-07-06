import { describe, it, expect, vi, beforeEach } from 'vitest'

vi.mock('@/api/client', () => ({
  apiClient: { defaults: { baseURL: '/api' } },
}))

let currentToken: string | null = 'token-1'
const refreshAccessToken = vi.fn()
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get token() {
      return currentToken
    },
    refreshAccessToken,
  }),
}))

import { parseSSEBuffer, handleSSEEvent, fanficApi } from '../fanfic'
import type { GenerateInput } from '@/types/fanfic'

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

/** Builds the SSE bytes for a full meta -> delta -> delta -> done stream. */
function fullSSEText(): string {
  return (
    'event: meta\ndata: {"id":"f1","model":"llama-3"}\n\n' +
    'event: delta\ndata: {"text":"Hello "}\n\n' +
    'event: delta\ndata: {"text":"world"}\n\n' +
    'event: done\ndata: {"id":"f1","title":"T","token_usage":10}\n\n'
  )
}

/** A fake ReadableStreamDefaultReader yielding one Uint8Array chunk per call. */
function fakeReader(chunks: Uint8Array[]) {
  let i = 0
  return {
    read: async () => {
      if (i < chunks.length) {
        return { value: chunks[i++], done: false }
      }
      return { value: undefined, done: true }
    },
  }
}

/** A minimal Response-shaped object carrying a fake streaming body. */
function fakeStreamResponse(textChunks: string[]) {
  const encoder = new TextEncoder()
  const chunks = textChunks.map((c) => encoder.encode(c))
  return {
    ok: true,
    status: 200,
    body: { getReader: () => fakeReader(chunks) },
  } as unknown as Response
}

const TEST_INPUT: GenerateInput = {
  anime: { title: 'Test Anime' },
  characters: [{ name: 'Char A' }],
  tags: ['fluff'],
  length: 'oneshot',
  pov: 'third',
  rating: 'teen',
  language: 'en',
  prompt: 'a prompt',
}

describe('fanficApi.generate', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    refreshAccessToken.mockReset()
    currentToken = 'token-1'
    vi.stubGlobal('fetch', fetchMock)
  })

  it('drives onMeta/onDelta/onDone from an SSE stream split mid-event across two chunks', async () => {
    const full = fullSSEText()
    // Split inside the second delta event's data payload, proving a partial
    // event straddling a chunk boundary is buffered, not dropped.
    const splitAt = full.indexOf('"world"') + 2
    fetchMock.mockResolvedValueOnce(fakeStreamResponse([full.slice(0, splitAt), full.slice(splitAt)]))

    const onMeta = vi.fn()
    const onDelta = vi.fn()
    const onDone = vi.fn()
    const onError = vi.fn()
    await fanficApi.generate(TEST_INPUT, { onMeta, onDelta, onDone, onError })

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0][1].headers.Authorization).toBe('Bearer token-1')
    expect(onMeta).toHaveBeenCalledWith('f1', 'llama-3')
    expect(onDelta).toHaveBeenNthCalledWith(1, 'Hello ')
    expect(onDelta).toHaveBeenNthCalledWith(2, 'world')
    expect(onDone).toHaveBeenCalledWith('f1', 'T', 10)
    expect(onError).not.toHaveBeenCalled()
    expect(refreshAccessToken).not.toHaveBeenCalled()
  })

  it('refreshes and retries once on a 401, then drives the retried stream to completion', async () => {
    refreshAccessToken.mockImplementationOnce(async () => {
      currentToken = 'token-2'
      return true
    })
    fetchMock
      .mockResolvedValueOnce({ ok: false, status: 401 } as unknown as Response)
      .mockResolvedValueOnce(fakeStreamResponse([fullSSEText()]))

    const onMeta = vi.fn()
    const onDelta = vi.fn()
    const onDone = vi.fn()
    const onError = vi.fn()
    await fanficApi.generate(TEST_INPUT, { onMeta, onDelta, onDone, onError })

    expect(refreshAccessToken).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[0][1].headers.Authorization).toBe('Bearer token-1')
    expect(fetchMock.mock.calls[1][1].headers.Authorization).toBe('Bearer token-2')
    expect(onDone).toHaveBeenCalledWith('f1', 'T', 10)
    expect(onError).not.toHaveBeenCalled()
  })
})
