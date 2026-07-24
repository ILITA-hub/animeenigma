/**
 * Notifications engine — v1.0 TypeScript types.
 *
 * Mirrors `services/notifications/internal/domain/notification.go` verbatim.
 * Snake_case field names match the backend JSON tags; do NOT camelCase them
 * here or the response envelope unwrap will silently drop fields.
 *
 * Workstream: notifications, Phase 3 (frontend bell + dropdown + toast).
 */

/**
 * NotificationType enumerates the kinds of notifications the engine emits.
 * v1.0 ships only `new_episode`; later phases add more types (e.g.
 * `ongoing_resumed`, `system_message`) without altering the schema.
 *
 * The store + renderer registry are payload-agnostic — adding a new type
 * is purely additive: ship a new renderer, define its payload type, no
 * changes to the bell/dropdown/toast.
 */
export type NotificationType =
  | 'new_episode'
  | 'feedback_created'
  | 'feedback_in_progress'
  | 'feedback_ai_done'
  | string

/**
 * Payload shape for `type === 'new_episode'`. Mirror of
 * `domain.NewEpisodePayload`. All fields lowercase_snake_case per the
 * project's JSON convention.
 *
 * `watch_url` ships as `/anime/{id}/watch?player=X&episode=N&translation=Y`
 * from the backend; the frontend route is `/anime/:id` with `?episode=N`
 * query — see `translateWatchUrl` in `stores/notifications.ts` and the
 * router alias in `router/index.ts`.
 */
export interface NewEpisodePayload {
  anime_id: string
  shikimori_id?: string
  anime_title: string
  anime_poster_url?: string
  first_unwatched_episode: number
  latest_available_episode: number
  player: string
  language: string
  watch_type: string
  translation_id: string
  translation_title?: string
  watch_url: string
}

/**
 * Payload shape for the three `feedback_*` types (AUTO-417 feedback triage
 * loop). Mirror of `domain.FeedbackStatusPayload` in the notifications
 * service. One card component renders all three stages; the stage itself is
 * carried both in `notification.type` and in `status`.
 */
export interface FeedbackStatusPayload {
  report_id: string
  category?: string // bug | issue | feature
  description?: string // truncated snippet of the user's original feedback
  status: string // created | in_progress | ai_done
}

/**
 * UserNotification — per-user notification row.
 *
 * `read_at` / `dismissed_at` / `deleted_at` / `clicked_at` are nullable
 * timestamps acting as both state flags and lightweight telemetry. The bell
 * badge counts rows where `read_at === null && dismissed_at === null`.
 * `dismissed_at` ("cleared from the bell, still shown in history") is distinct
 * from `deleted_at` ("binned from the All-notifications modal — gone from
 * history too").
 *
 * `payload` is typed `unknown` here because the engine carries every type
 * in the same table; downstream renderers cast to the concrete payload
 * shape (`as NewEpisodePayload`).
 */
export interface UserNotification {
  id: string
  user_id: string
  type: NotificationType
  dedupe_key: string
  payload: unknown
  read_at: string | null
  dismissed_at: string | null
  deleted_at: string | null
  clicked_at: string | null
  created_at: string
  updated_at: string
}

/**
 * Response shape for `GET /api/notifications?status=unread|all&limit=...`.
 * One round-trip surfaces the rows plus both counts the bell badge needs.
 */
export interface NotificationListResponse {
  notifications: UserNotification[]
  unread_count: number
  total: number
}

/** Response shape for `POST /api/notifications/mark-all-read`. */
export interface MarkAllReadResponse {
  updated: number
}
