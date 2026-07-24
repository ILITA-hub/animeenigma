/**
 * Typed API client for the notifications service.
 *
 * Routes (gateway-proxied, JWT-required):
 *   GET    /api/notifications?status=unread|all|history&limit=&offset=
 *   POST   /api/notifications/{id}/read
 *   POST   /api/notifications/mark-all-read
 *   POST   /api/notifications/{id}/dismiss
 *   POST   /api/notifications/{id}/delete
 *   POST   /api/notifications/{id}/click
 *
 * The notifications service uses `libs/httputil.JSON` for all responses,
 * which wraps the payload in `{success: true, data: T}`. Every helper
 * here unwraps the envelope so callers never see the wrapper, matching
 * the project pattern in `stores/auth.ts:70` and `stores/watchlist.ts:46`.
 *
 * Workstream: notifications, Phase 3.
 */

import { apiClient } from '@/api/client'
import type {
  NotificationListResponse,
  MarkAllReadResponse,
} from '@/types/notification'

/** `history` = active + dismissed rows (the "view older" modal). */
export type ListStatus = 'unread' | 'all' | 'history'

/**
 * Unwrap the standard `{success, data}` envelope. Backends sometimes
 * return the bare payload when the envelope helper is bypassed; the
 * `?? raw` fallback keeps us robust against either shape.
 */
function unwrap<T>(raw: unknown): T {
  if (raw && typeof raw === 'object' && 'data' in (raw as Record<string, unknown>)) {
    const data = (raw as { data?: unknown }).data
    if (data !== undefined && data !== null) {
      return data as T
    }
  }
  return raw as T
}

/**
 * GET /api/notifications?status=...&limit=...&offset=...
 *
 * `status` defaults to `'unread'` — matches the backend default. The
 * bell/dropdown store passes `'all'` so already-read notifications stay
 * visible (rendered tinted); the unread badge comes from the response's
 * `unread_count`, not the row list.
 */
export async function listNotifications(
  status: ListStatus = 'unread',
  limit = 20,
  offset = 0,
): Promise<NotificationListResponse> {
  const response = await apiClient.get('/notifications', {
    params: { status, limit, offset },
  })
  return unwrap<NotificationListResponse>(response.data)
}

/** POST /api/notifications/{id}/read */
export async function markRead(id: string): Promise<void> {
  await apiClient.post(`/notifications/${encodeURIComponent(id)}/read`)
}

/** POST /api/notifications/mark-all-read → number of rows updated. */
export async function markAllRead(): Promise<number> {
  const response = await apiClient.post('/notifications/mark-all-read')
  const data = unwrap<MarkAllReadResponse>(response.data)
  return data?.updated ?? 0
}

/** POST /api/notifications/{id}/dismiss */
export async function dismiss(id: string): Promise<void> {
  await apiClient.post(`/notifications/${encodeURIComponent(id)}/dismiss`)
}

/**
 * POST /api/notifications/{id}/delete — the "bin" action in the
 * All-notifications history modal. Soft-removes the row from EVERY surface
 * (unread, all, history); distinct from dismiss, which keeps it in history.
 */
export async function deleteNotification(id: string): Promise<void> {
  await apiClient.post(`/notifications/${encodeURIComponent(id)}/delete`)
}

/**
 * POST /api/notifications/{id}/click — fire-and-forget click telemetry.
 *
 * The frontend navigates to the watch URL on its own and does not wait
 * for a meaningful response. Failures are logged-and-swallowed by the
 * caller; navigation must not be blocked on telemetry.
 */
export async function click(id: string): Promise<void> {
  await apiClient.post(`/notifications/${encodeURIComponent(id)}/click`)
}
