// Shared types for the anime detail/watch page (views/Anime.vue) and its
// page-scoped composables. Extracted verbatim from Anime.vue's <script setup>.

export interface AnimeWithExtras {
  japaneseTitle?: string
  type?: string
  hidden?: boolean
  shikimoriId?: string
}

export interface RelatedAnime {
  id: string
  title: string
  name?: string
  nameRu?: string
  coverImage: string
  rating?: number
  releaseYear?: number
  episodes?: number
  genres?: string[]
  relationLabel?: string
}

export interface Review {
  id: string
  user_id: string
  anime_id: string
  username: string
  // Author's CURRENT avatar (read-time join in the player service, not
  // snapshotted). Absent → Avatar primitive falls back to initials.
  user_avatar?: string
  score: number
  review_text: string
  created_at: string
  // Steam-style review context (2026-05-21). Live values from anime_list
  // row — NOT snapshotted at review time. `anime` carries episodes_count
  // for the "watched / total" rendering; backend preloads it.
  status?: string
  episodes?: number
  // True when the reviewer is on a rewatch — renders a "🔁 On rewatch"
  // segment after the watch stats (repo-todo 19:00:01).
  is_rewatching?: boolean
  anime?: {
    episodes_count?: number
  }
  // Emoji reactions (AUTO-408). `reactions` carries per-emoji counts +
  // reacted_by_me; `my_reactions` is the viewer's reacted-emoji subset.
  reactions?: { emoji: string; count: number; reacted_by_me: boolean; users?: string[] }[]
  my_reactions?: string[]
}

export interface Comment {
  id: string
  user_id: string
  anime_id: string
  username: string
  // Author's CURRENT avatar — same read-time join as reviews/activity feed.
  user_avatar?: string
  body: string
  created_at: string
  updated_at: string
}

export const UGC_ALLOWED = ['reviews', 'comments'] as const
export type UgcTab = typeof UGC_ALLOWED[number]

export interface AnimeRating {
  anime_id: string
  average_score: number
  total_reviews: number
}

export interface ApiError {
  response?: { status?: number; data?: { error?: string; message?: string } }
}
