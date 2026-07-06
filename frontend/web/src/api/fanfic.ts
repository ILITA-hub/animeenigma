/**
 * Fanfic engine API client (spec 2026-07-06).
 *
 * Routes (gateway-proxied under /api/fanfic/*, JWT-required, guest-blocked,
 * admin-gated while FANFIC_ADMIN_ONLY — see services/gateway/internal/
 * transport/router.go):
 *   POST   /api/fanfic/generate   — SSE stream (meta/delta/done/error)
 *   GET    /api/fanfic            — list (paginated)
 *   GET    /api/fanfic/{id}       — get one
 *   DELETE /api/fanfic/{id}       — soft-delete
 *   GET    /api/fanfic/tags       — curated tag list
 *
 * The fanfic service wraps every JSON response in the standard
 * `{success, data}` envelope (libs/httputil.JSON/.OK) — mirrors the unwrap
 * convention in src/api/sessions.ts / src/api/notifications.ts so callers
 * get clean typed values instead of re-deriving `.data.data` everywhere.
 *
 * `generate()` bypasses axios (fetch + ReadableStream, since axios has no
 * streaming-body reader in the browser) so the Authorization header is
 * attached manually here, mirroring the EXACT scheme apiClient's request
 * interceptor uses (`Authorization: Bearer <token>`, see src/api/client.ts)
 * — this raw call never passes through that interceptor.
 */
import { apiClient } from './client'
import { useAuthStore } from '@/stores/auth'
import type { Fanfic, FanficTag, GenerateInput, StreamHandlers } from '@/types/fanfic'

export interface SSEEvent {
  event: string
  data: unknown
}

/** Unwrap the standard `{success, data}` envelope; tolerate a bare payload. */
function unwrap<T>(raw: unknown): T {
  if (raw && typeof raw === 'object' && 'data' in (raw as Record<string, unknown>)) {
    const data = (raw as { data?: unknown }).data
    if (data !== undefined && data !== null) {
      return data as T
    }
  }
  return raw as T
}

/** Split an SSE text buffer into complete events plus the unparsed remainder. */
export function parseSSEBuffer(buffer: string): { events: SSEEvent[]; rest: string } {
  const events: SSEEvent[] = []
  const parts = buffer.split('\n\n')
  const rest = parts.pop() ?? ''
  for (const block of parts) {
    let event = 'message'
    let data = ''
    for (const line of block.split('\n')) {
      if (line.startsWith('event: ')) event = line.slice(7).trim()
      else if (line.startsWith('data: ')) data += line.slice(6)
    }
    if (!data) continue
    try {
      events.push({ event, data: JSON.parse(data) })
    } catch {
      /* ignore malformed */
    }
  }
  return { events, rest }
}

/** Dispatch one parsed SSE event to the matching StreamHandlers callback. */
export function handleSSEEvent(evt: SSEEvent, h: StreamHandlers): void {
  switch (evt.event) {
    case 'meta': {
      const d = evt.data as { id: string; model: string }
      h.onMeta?.(d.id, d.model)
      break
    }
    case 'delta': {
      const d = evt.data as { text: string }
      h.onDelta?.(d.text)
      break
    }
    case 'done': {
      const d = evt.data as { id: string; title: string; token_usage: number }
      h.onDone?.(d.id, d.title, d.token_usage)
      break
    }
    case 'error': {
      const d = evt.data as { message: string }
      h.onError?.(d.message)
      break
    }
  }
}

export const fanficApi = {
  /** Stream a generation. Uses fetch + ReadableStream to consume SSE. */
  async generate(input: GenerateInput, handlers: StreamHandlers, signal?: AbortSignal): Promise<void> {
    const auth = useAuthStore()
    // fetch bypasses apiClient's axios interceptor (no streaming-body reader
    // on axios in the browser), so the Bearer header + base URL are built
    // here to match it exactly — same scheme as utils/authBeacon.ts, the
    // other raw-fetch-with-Bearer-token call site in this codebase.
    const base = apiClient.defaults.baseURL ?? '/api'
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (auth.token) {
      headers.Authorization = `Bearer ${auth.token}`
    }
    const res = await fetch(`${base}/fanfic/generate`, {
      method: 'POST',
      headers,
      body: JSON.stringify(input),
      credentials: 'include',
      signal,
    })
    if (!res.ok || !res.body) {
      handlers.onError?.(`HTTP ${res.status}`)
      return
    }
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const { events, rest } = parseSSEBuffer(buffer)
      buffer = rest
      for (const evt of events) handleSSEEvent(evt, handlers)
    }
    // Flush any bytes the decoder was holding back mid multi-byte sequence.
    // The server always terminates its last event with a trailing "\n\n"
    // (see services/fanfic/internal/handler/fanfic.go's emit closure), so a
    // well-formed stream has already fired every event by this point; a
    // non-empty `buffer` here means the connection ended mid-event (client
    // abort, proxy cutoff) and there is no complete event left to dispatch.
    buffer += decoder.decode()
  },

  /** GET /api/fanfic?page=&limit= */
  async list(page = 1, limit = 20): Promise<{ items: Fanfic[]; total: number }> {
    const res = await apiClient.get('/fanfic', { params: { page, limit } })
    return unwrap<{ items: Fanfic[]; total: number }>(res.data)
  },

  /** GET /api/fanfic/{id} */
  async get(id: string): Promise<Fanfic> {
    const res = await apiClient.get(`/fanfic/${encodeURIComponent(id)}`)
    return unwrap<Fanfic>(res.data)
  },

  /** DELETE /api/fanfic/{id} — 204 No Content, nothing to unwrap. */
  async remove(id: string): Promise<void> {
    await apiClient.delete(`/fanfic/${encodeURIComponent(id)}`)
  },

  /** GET /api/fanfic/tags */
  async tags(): Promise<FanficTag[]> {
    const res = await apiClient.get('/fanfic/tags')
    return unwrap<FanficTag[]>(res.data)
  },
}
