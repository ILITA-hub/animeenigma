/**
 * Anidle anime-guessing game API.
 *
 * All responses are wrapped in { success: boolean, data: T } by httputil.OK.
 * Callers unwrap via `response.data?.data ?? response.data`.
 */
import { apiClient } from '@/api/client'

// ─── Shared types ──────────────────────────────────────────────────────────

/** A genre, studio, or tag with id and name. */
export interface Taxon {
  id: string
  name: string
}

/**
 * Full anime attributes — returned by search, daily state, and guess responses.
 * RECONCILED 2026-06-15: backend returns complete attributes in guess/resume
 * responses so the grid can render each cell's value alongside the server's status.
 */
export interface VisibleAnime {
  id: string
  name_ru: string
  name_en: string
  name_jp: string
  poster_url: string
  year: number
  episodes: number
  score: number
  status: string
  rating: string
  genres: Taxon[]
  studios: Taxon[]
  tags: Taxon[]
}

/** Status of one comparison column. */
export interface ColumnResult {
  status: 'correct' | 'partial' | 'wrong'
  hint?: 'higher' | 'lower'
}

/** Per-column comparison result for one guess. */
export interface GuessComparison {
  genres: ColumnResult
  studios: ColumnResult
  year: ColumnResult
  episodes: ColumnResult
  score: ColumnResult
  status: ColumnResult
  rating: ColumnResult
  tags: ColumnResult
}

/** One guess attempt with full anime attributes and comparison results. */
export interface GuessOutcome {
  anime: VisibleAnime
  result: GuessComparison
  solved: boolean
  attempt: number
  answer?: VisibleAnime
}

/** Daily game state from GET /api/anidle/daily. */
export interface DailyState {
  date: string
  solved: boolean
  gave_up: boolean
  guesses: GuessOutcome[]
  answer?: VisibleAnime
}

/** Search result item — same shape as VisibleAnime. */
export type SearchResultItem = VisibleAnime

/** Search results array. */
export type SearchResult = SearchResultItem[]

/** Authenticated user stats. */
export interface UserStats {
  user_id: string
  games_played: number
  games_won: number
  current_streak: number
  max_streak: number
  guess_distribution: Record<string, number>
  last_played_date: string
  updated_at: string
}

/** Leaderboard entry for today's daily game. */
export interface LeaderEntry {
  username: string
  attempts: number
}

// ─── Share helper ──────────────────────────────────────────────────────────

const COLUMN_ORDER: Array<keyof GuessComparison> = [
  'genres', 'studios', 'year', 'episodes', 'score', 'status', 'rating', 'tags',
]

/** Build a spoiler-free shareable emoji grid from guess outcomes. */
export function buildShareText(guesses: GuessOutcome[], date: string, solved: boolean): string {
  const header = `Anidle ${date} — ${guesses.length} попыток 🎯`
  const rows = guesses
    .slice()
    .reverse()
    .map((g) =>
      COLUMN_ORDER.map((col) => {
        const s = g.result[col]?.status
        if (s === 'correct') return '🟩'
        if (s === 'partial') return '🟨'
        return '⬜'
      }).join(''),
    )
  if (!solved) {
    rows.push('❌ Не угадал')
  }
  return [header, '', ...rows].join('\n')
}

// ─── API object ────────────────────────────────────────────────────────────

export const anidleApi = {
  /** GET /api/anidle/daily — current daily state (guest: empty guesses array). */
  getDailyState: () =>
    apiClient.get<{ data: DailyState }>('/anidle/daily'),

  /** POST /api/anidle/daily/guess — submit a daily guess by anime id. */
  dailyGuess: (animeId: string) =>
    apiClient.post<{ data: GuessOutcome }>('/anidle/daily/guess', { anime_id: animeId }),

  /** POST /api/anidle/daily/giveup — reveal the answer and end today's game. */
  dailyGiveUp: () =>
    apiClient.post<{ data: VisibleAnime }>('/anidle/daily/giveup', {}),

  /** GET /api/anidle/search?q= — search the guessable pool. */
  search: (q: string) =>
    apiClient.get<{ data: SearchResult }>('/anidle/search', { params: { q } }),

  /** POST /api/anidle/endless/new — start a new endless round. */
  endlessNew: () =>
    apiClient.post<{ data: { round_token: string } }>('/anidle/endless/new', {}),

  /** POST /api/anidle/endless/guess — submit an endless guess. */
  endlessGuess: (roundToken: string, animeId: string) =>
    apiClient.post<{ data: GuessOutcome }>('/anidle/endless/guess', {
      round_token: roundToken,
      anime_id: animeId,
    }),

  /** GET /api/anidle/stats — authenticated user stats (204 for guests). */
  getStats: () =>
    apiClient.get<{ data: UserStats }>('/anidle/stats'),

  /** GET /api/anidle/leaderboard?date= — today's top solvers. */
  getLeaderboard: (date: string) =>
    apiClient.get<{ data: LeaderEntry[] }>('/anidle/leaderboard', { params: { date } }),
}
