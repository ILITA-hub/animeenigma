// Admin feedback browser types — mirror the player service contract in
// services/player/internal/handler/admin_reports.go.

// 'ai_done' is a transparent automation state: an AI agent believes the item is
// done, pending human verification before promotion to 'resolved'.
export type FeedbackStatus = 'new' | 'in_progress' | 'ai_done' | 'resolved' | 'not_relevant'

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
}

export interface FeedbackListResponse {
  items: FeedbackListItem[]
  total: number
  page: number
  page_size: number
}

// FeedbackDetail is the full on-disk report (raw map) with id/status injected.
export interface FeedbackDetail extends FeedbackListItem {
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
}
