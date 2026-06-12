import { apiClient } from '@/api/client'

/**
 * Authenticated page-close save — the sendBeacon replacement.
 *
 * `navigator.sendBeacon` cannot carry an `Authorization` header, so every
 * beacon aimed at a JWT-protected endpoint (e.g. POST /users/progress) was
 * rejected with 401 and the data silently lost. `fetch` with
 * `keepalive: true` survives pagehide/unload exactly like a beacon (≤64KB
 * body) AND takes headers, so the Bearer token rides along.
 *
 * Best-effort by design: never throws, never blocks unload. If the access
 * token is missing/expired at close time the request may still 401 — the
 * regular in-page heartbeats (which refresh via the axios interceptor) are
 * the primary save path; this only covers the final seconds.
 */
export function postKeepalive(relPath: string, body: Record<string, unknown>): void {
  try {
    const base = apiClient.defaults.baseURL ?? '/api'
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    const token = localStorage.getItem('token')
    if (token) headers.Authorization = `Bearer ${token}`
    void fetch(`${base}${relPath}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
      keepalive: true,
      credentials: 'include',
    }).catch(() => undefined)
  } catch {
    /* page is unloading — nothing actionable */
  }
}
