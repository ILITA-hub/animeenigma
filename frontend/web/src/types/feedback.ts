// Admin feedback browser types — mirror the player service contract in
// services/player/internal/handler/admin_reports.go.

// 'ai_done' is a transparent automation state: an AI agent believes the item is
// done, pending human verification before promotion to 'resolved'.
export type FeedbackStatus = 'new' | 'in_progress' | 'ai_done' | 'resolved' | 'not_relevant'

// Item nature (Project Board). 'feedback' = inbound from users; 'todo'/'idea'
// are internal notebook items.
export type FeedbackKind = 'feedback' | 'todo' | 'idea'

// Normalized channel the item entered the system through.
export type FeedbackSource = 'feedback_form' | 'telegram' | 'api' | 'manual'

// FeedbackListItem is one light list row (no heavy diagnostics).
export interface FeedbackListItem {
  id: string
  timestamp: string
  username: string
  user_id: string
  player_type: string
  category: string
  anime_name: string
  episode_number?: number | null
  url: string
  description: string
  status: FeedbackStatus
  kind?: FeedbackKind
  // 'telegram' for entries mirrored by the maintenance bot
  source?: FeedbackSource
  // Stored attachment filenames (served via /admin/reports/{id}/attachments/{name})
  attachments?: string[]
}

export interface FeedbackListResponse {
  items: FeedbackListItem[]
  total: number
  page: number
  page_size: number
}

// FeedbackDetail is the full on-disk report (raw map) with id/status injected.
/** One triage transition (admin shape includes the actor). */
export interface StatusTransition {
  from: FeedbackStatus | string
  to: FeedbackStatus | string
  at: string
  by?: string
}

export interface FeedbackDetail extends FeedbackListItem {
  status_history?: StatusTransition[]
  anime_id?: string
  server_name?: string
  stream_url?: string
  error_message?: string
  user_agent?: string
  screen_size?: string
  language?: string
  console_logs?: unknown
  network_logs?: unknown
  page_html?: string
  status_updated_at?: string
  status_updated_by?: string
  // Telegram-sourced entries (maintenance bot mirror)
  telegram_meta?: {
    message_id?: number
    chat_id?: number
    forwarded_from?: string
    reply_to?: string
    from_admin?: boolean
  }
}

/** User-facing "my feedback" row (GET /api/users/reports). */
export interface MyFeedbackItem {
  status_history?: Array<{ from: string; to: string; at: string }>
  id: string
  timestamp: string
  player_type: string
  category: string
  anime_name?: string
  episode_number?: number | null
  description: string
  status: FeedbackStatus
  status_updated_at?: string
}

export interface MyFeedbackResponse {
  items: MyFeedbackItem[]
  total: number
  page: number
  page_size: number
}
