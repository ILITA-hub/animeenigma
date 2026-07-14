/**
 * Fanfic engine API client (spec 2026-07-06).
 *
 * Routes (gateway-proxied under /api/fanfic/*, JWT-required, guest-blocked,
 * access resolved per-request via the policy-service ruleset —
 * FeatureGate("fanfic", ...) — see services/gateway/internal/
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
import type {
  Fanfic,
  FanficLang,
  FanficRating,
  FanficTag,
  GenerateInput,
  StreamHandlers,
} from '@/types/fanfic'

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
      const d = evt.data as { id: string; model: string; part?: number }
      h.onMeta?.(d.id, d.model, d.part)
      break
    }
    case 'delta': {
      const d = evt.data as { text: string }
      h.onDelta?.(d.text)
      break
    }
    case 'done': {
      const d = evt.data as { id: string; title: string; token_usage: number; part?: number }
      h.onDone?.(d.id, d.title, d.token_usage, d.part)
      break
    }
    case 'error': {
      const d = evt.data as { message: string }
      h.onError?.(d.message)
      break
    }
  }
}

/**
 * Shared SSE-over-fetch plumbing for the /generate and /{id}/continue
 * streaming endpoints. Uses fetch + ReadableStream to consume SSE (axios has
 * no streaming-body reader in the browser), so the Authorization header is
 * attached manually here, mirroring the EXACT scheme apiClient's request
 * interceptor uses (`Authorization: Bearer <token>`, see src/api/client.ts)
 * — this raw call never passes through that interceptor.
 */
async function streamSSE(
  path: string,
  body: unknown,
  handlers: StreamHandlers,
  signal?: AbortSignal,
): Promise<void> {
  const auth = useAuthStore()
  const base = apiClient.defaults.baseURL ?? '/api'

  async function attempt(token: string | null): Promise<Response> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (token) {
      headers.Authorization = `Bearer ${token}`
    }
    return fetch(`${base}${path}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(body ?? {}),
      credentials: 'include',
      signal,
    })
  }

  let res = await attempt(auth.token)
  if (res.status === 401) {
    // Safe to retry: a 401 is rejected at the gateway JWT middleware BEFORE
    // it reaches the fanfic generation logic, so no fanfic row is created
    // on a rejected attempt — refreshing and retrying once cannot
    // double-generate.
    const refreshed = await auth.refreshAccessToken()
    if (refreshed) {
      res = await attempt(auth.token)
    }
  }
  if (!res.ok || !res.body) {
    handlers.onError?.(`HTTP ${res.status}`)
    return
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  try {
    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const { events, rest } = parseSSEBuffer(buffer)
      buffer = rest
      for (const evt of events) handleSSEEvent(evt, handlers)
    }
    // Final flush: emit any bytes the stream-mode decoder was holding back
    // for a pending multi-byte sequence. A well-formed stream terminates its
    // last event with a trailing "\n\n" (see services/fanfic/internal/
    // handler/fanfic.go's emit closure) and so has already dispatched every
    // event inside the loop above; this is a safety net for a connection
    // that ends mid-event (client abort, proxy cutoff) without that
    // terminator — force one so parseSSEBuffer treats the dangling event as
    // complete and we still get to dispatch it.
    buffer += decoder.decode()
    if (buffer) {
      const { events } = parseSSEBuffer(buffer + '\n\n')
      for (const evt of events) handleSSEEvent(evt, handlers)
    }
  } catch (err) {
    // An aborted read (regenerate / component unmount via AbortController)
    // rejects with an AbortError — that's an intentional cancellation, not
    // a failure, so stay silent rather than surfacing it via onError.
    if ((err instanceof DOMException && err.name === 'AbortError') || signal?.aborted) {
      return
    }
    handlers.onError?.(err instanceof Error ? err.message : String(err))
  }
}

/**
 * Wire shape of `GET /api/fanfic/daily` (the `publicDaily` struct in
 * services/fanfic/internal/handler/daily.go): the compact spotlight DTO
 * (snake_case, `fanfic_title` not `title`, no anime/user UUIDs, no
 * length/pov/tags/characters/prompt/model/token_usage/canon) plus the full
 * `content` and explicit-gating metadata. An explicit pick always comes back
 * with `content:""` and `gated:true` — `gate_reason` is `"login"` for an
 * anonymous reader or `"adult_setting"` for a logged-in one (the backend
 * gates every explicit daily pick unconditionally today; there is no
 * opt-in "show adult content" setting yet to unlock it).
 */
interface DailyFanficResponse {
  id: string
  fanfic_title: string
  anime_title: string
  anime_japanese: string
  anime_poster: string
  excerpt: string
  rating: FanficRating
  language: FanficLang
  explicit: boolean
  author_username: string
  credited: boolean
  ai_generated: boolean
  part_count: number
  created_at: string
  content: string
  gated: boolean
  gate_reason?: string
}

export const fanficApi = {
  /** Stream a generation. Uses fetch + ReadableStream to consume SSE. */
  async generate(input: GenerateInput, handlers: StreamHandlers, signal?: AbortSignal): Promise<void> {
    return streamSSE('/fanfic/generate', input, handlers, signal)
  },

  /** Stream a continuation of a saved fanfic (empty body — params reused server-side). */
  async continueStory(id: string, handlers: StreamHandlers, signal?: AbortSignal): Promise<void> {
    return streamSSE(`/fanfic/${encodeURIComponent(id)}/continue`, undefined, handlers, signal)
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

  /**
   * GET /api/fanfic/daily — the public "Фанфик дня" reader (wires
   * DailyFanficCard's "Читать" CTA at /fanfics?daily=1). Public route (no
   * mandatory JWT at the fanfic service); the shared apiClient still attaches
   * the bearer token when one is present, so a logged-in reader's identity
   * reaches the backend's explicit-gating check.
   *
   * The daily DTO isn't a Fanfic (see DailyFanficResponse above) — this maps
   * it onto the shape the existing reader Modal in FanficsView.vue already
   * knows how to render (mirrors onOpenFanfic's `fanficApi.get(id)` path),
   * filling fields the daily DTO doesn't carry with UI-inert placeholders.
   * `status:'complete'` is honest (the daily pool is `WHERE status =
   * complete`, see repo/fanfic.go) — FanficsView additionally guards its
   * owner-scoped "Продолжить" footer button off a separate `readerIsDaily`
   * flag rather than lying about status here, since /continue 404s for
   * anyone but the fanfic's own author.
   */
  async getDaily(): Promise<Fanfic & { gated?: boolean; gate_reason?: string }> {
    const res = await apiClient.get('/fanfic/daily')
    const raw = unwrap<DailyFanficResponse>(res.data)
    return {
      id: raw.id,
      anime_id: '',
      anime_shikimori_id: '',
      anime_title: raw.anime_title,
      anime_japanese: raw.anime_japanese,
      anime_poster: raw.anime_poster,
      characters: [],
      tags: [],
      length: 'oneshot',
      pov: 'third',
      rating: raw.rating,
      language: raw.language,
      prompt: '',
      title: raw.fanfic_title,
      content: raw.content,
      model: '',
      token_usage: 0,
      status: 'complete',
      created_at: raw.created_at,
      canon: false,
      part_count: raw.part_count,
      gated: raw.gated,
      gate_reason: raw.gate_reason,
    }
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
