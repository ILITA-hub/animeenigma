import axios, { AxiosInstance, InternalAxiosRequestConfig, AxiosResponse, AxiosError } from 'axios'
import { hookAxiosDiagnostics } from '@/utils/diagnostics'
import { getOrCreateAnonId } from '@/utils/anonId'
import type { WatchCombo, ResolveResponse, ResolvedCombo } from '@/types/preference'

const BASE_URL = import.meta.env.VITE_API_URL || '/api'

export const apiClient: AxiosInstance = axios.create({
  baseURL: BASE_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  },
  withCredentials: true // Send cookies with requests (for refresh token)
})

// Flag to prevent multiple refresh attempts
let isRefreshing = false
let refreshPromise: Promise<string | null> | null = null
let failedQueue: Array<{
  resolve: (token: string) => void
  reject: (error: AxiosError) => void
}> = []

const processQueue = (error: AxiosError | null, token: string | null = null) => {
  failedQueue.forEach(prom => {
    if (error) {
      prom.reject(error)
    } else {
      prom.resolve(token!)
    }
  })
  failedQueue = []
}

// Check if JWT token is expired or about to expire (within 30s)
function isTokenExpired(token: string): boolean {
  try {
    const payload = JSON.parse(atob(token.split('.')[1]))
    return payload.exp * 1000 < Date.now() + 30_000
  } catch {
    return true
  }
}

// Wipes auth-related localStorage on a confirmed logout (refresh 401/403).
// Mirrors what stores/auth.ts logout() does to the same keys.
function clearAuthLocalStorage() {
  localStorage.removeItem('token')
  localStorage.removeItem('user')
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const key = localStorage.key(i)
    if (key && (key.startsWith('pref:') || key === 'prefs_version')) {
      localStorage.removeItem(key)
    }
  }
}

// Fires after localStorage is cleared so the Pinia auth store can drop its
// in-memory refs without a hard navigation. Same-tab notification only;
// other tabs observe the localStorage delta via the 'storage' event.
function dispatchAuthExpired() {
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent('auth:expired'))
  }
}

// Actually performs the /auth/refresh POST. Kept separate so the
// cross-tab lock wrapper in doTokenRefresh can short-circuit when another
// tab already minted a fresh token while we were waiting on the lock.
async function performRefresh(): Promise<string | null> {
  try {
    const response = await axios.post(`${BASE_URL}/auth/refresh`, {}, {
      withCredentials: true,
    })
    const data = response.data?.data || response.data
    const newAccessToken = data.access_token
    localStorage.setItem('token', newAccessToken)
    processQueue(null, newAccessToken)
    return newAccessToken as string
  } catch (refreshError) {
    processQueue(refreshError as AxiosError, null)
    // Only treat 401/403 as a real auth failure. Network/5xx errors
    // (VPN reconnect, transient backend hiccup) leave the session intact.
    const status = (refreshError as AxiosError)?.response?.status
    if (status === 401 || status === 403) {
      // Soft logout: clear storage + notify the Pinia store. No
      // window.location redirect — that interrupts mid-action recovery
      // and races with the cross-tab listener.
      clearAuthLocalStorage()
      dispatchAuthExpired()
    }
    return null
  }
}

// Shared token refresh — single-flight both within a tab (module-level
// refreshPromise) and across tabs (Web Locks). The cross-tab guard is the
// fix for the rotation race: the backend single-uses each refresh token,
// so two tabs both POSTing /auth/refresh would have the second tab's RT1
// blacklisted by the first → spurious 401 → logout.
async function doTokenRefresh(): Promise<string | null> {
  if (refreshPromise) return refreshPromise

  isRefreshing = true
  refreshPromise = (async () => {
    try {
      if (typeof navigator !== 'undefined' && 'locks' in navigator) {
        return await navigator.locks.request('auth-refresh', async () => {
          // Another tab may have refreshed while we waited on the lock.
          // If localStorage now holds a still-valid token, use it instead
          // of burning our refresh-token round-trip.
          const stored = localStorage.getItem('token')
          if (stored && !isTokenExpired(stored)) {
            processQueue(null, stored)
            return stored
          }
          return await performRefresh()
        })
      }
      return await performRefresh()
    } finally {
      isRefreshing = false
      refreshPromise = null
    }
  })()

  return refreshPromise
}

// Request interceptor — proactively refresh expired tokens before sending
apiClient.interceptors.request.use(
  async (config: InternalAxiosRequestConfig) => {
    // Skip refresh for auth endpoints to avoid loops
    if (config.url?.includes('/auth/refresh') || config.url?.includes('/auth/login')) {
      return config
    }

    let token = localStorage.getItem('token')
    if (token && isTokenExpired(token)) {
      const newToken = await doTokenRefresh()
      token = newToken
    }
    config.headers = config.headers || {}
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    // Always-set X-Anon-ID per PATTERNS.md. The backend OptionalAuthMiddleware reads
    // JWT first regardless of X-Anon-ID presence, so always-set is harmless on JWT
    // routes (handlers ignore unknown headers) and removes a class of subtle bugs
    // where X-Anon-ID would be missing if the user had a JWT but the JWT was rejected
    // downstream. See .planning/phases/01-instrumentation-baseline/01-PATTERNS.md
    // and 01-RESEARCH.md §Pattern 7.
    config.headers['X-Anon-ID'] = getOrCreateAnonId()
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// Phase 7 D-03 — bust the 24h pref:* localStorage cache when the backend
// signals a new prefs_version on any preference-touching response. This makes
// cross-device prefs changes visible immediately on the next API call instead
// of after the 24h composable TTL.
const PREFS_VERSION_KEY = 'prefs_version'
function maybeBustPrefsCache(headerVal: string | undefined) {
  if (!headerVal) return
  const incoming = headerVal.trim()
  if (!incoming) return
  const cached = localStorage.getItem(PREFS_VERSION_KEY)
  if (cached === incoming) return
  // Version drifted — wipe every cached resolved-combo entry. The next
  // resolve call repopulates from server.
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const key = localStorage.key(i)
    if (key && key.startsWith('pref:')) {
      localStorage.removeItem(key)
    }
  }
  localStorage.setItem(PREFS_VERSION_KEY, incoming)
}

// Response interceptor — handles 401 errors and X-Token-Expired fallback
apiClient.interceptors.response.use(
  async (response: AxiosResponse) => {
    // Cross-device prefs freshness signal (Phase 7 D-03)
    maybeBustPrefsCache(response.headers['x-prefs-version'] as string | undefined)

    // Fallback: backend signals token was present but expired on optional-auth endpoints
    if (response.headers['x-token-expired'] === 'true') {
      const newToken = await doTokenRefresh()
      if (newToken) {
        const config = response.config as InternalAxiosRequestConfig & { _tokenRetry?: boolean }
        if (!config._tokenRetry) {
          config._tokenRetry = true
          config.headers.Authorization = `Bearer ${newToken}`
          return apiClient(config)
        }
      }
    }
    return response
  },
  async (error: AxiosError) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean }

    // Skip refresh for auth endpoints to avoid loops
    if (error.response?.status === 401 &&
        originalRequest &&
        !originalRequest._retry &&
        !originalRequest.url?.includes('/auth/refresh') &&
        !originalRequest.url?.includes('/auth/login')) {

      if (isRefreshing) {
        // Queue request while refresh is in progress (10s timeout to prevent hanging)
        return new Promise<string>((resolve, reject) => {
          const timeout = setTimeout(() => reject(new Error('Token refresh timeout')), 10_000)
          failedQueue.push({
            resolve: (token: string) => { clearTimeout(timeout); resolve(token) },
            reject: (err: AxiosError) => { clearTimeout(timeout); reject(err) }
          })
        }).then(token => {
          originalRequest.headers.Authorization = `Bearer ${token}`
          return apiClient(originalRequest)
        }).catch(err => {
          return Promise.reject(err)
        })
      }

      originalRequest._retry = true
      const newToken = await doTokenRefresh()
      if (newToken) {
        originalRequest.headers.Authorization = `Bearer ${newToken}`
        return apiClient(originalRequest)
      }
    }

    return Promise.reject(error)
  }
)

// Hook diagnostics capture for network logs
hookAxiosDiagnostics(apiClient)

// Phase 17 (UX-33) — editorial collections. Mirror the backend
// services/catalog/internal/domain/collection.go shape.
export interface CollectionItem {
  id: string
  collection_id: string
  anime_id: string
  // The backend preloads Anime on detail views; list views omit it.
  anime?: {
    id: string
    name?: string
    name_ru?: string
    name_jp?: string
    poster_url?: string
    episodes_count?: number
    episodes_aired?: number
    score?: number
    year?: number
    status?: string
  }
  sort_order: number
  created_at: string
}

export interface Collection {
  id: string
  slug: string
  title: string
  title_ru?: string
  title_jp?: string
  description?: string
  description_ru?: string
  description_jp?: string
  cover_image_url?: string
  published: boolean
  created_by?: string
  items?: CollectionItem[]
  item_count: number
  created_at: string
  updated_at: string
}

export interface CreateCollectionRequest {
  slug?: string
  title: string
  title_ru?: string
  title_jp?: string
  description?: string
  description_ru?: string
  description_jp?: string
  cover_image_url?: string
  published?: boolean
}

export type UpdateCollectionRequest = Partial<CreateCollectionRequest>

export interface AddCollectionItemRequest {
  anime_id: string
  sort_order?: number
}

// API endpoints
export const animeApi = {
  getAll: (params?: Record<string, unknown>) => apiClient.get('/anime', { params }),
  getById: (id: string) => apiClient.get(`/anime/${id}`),
  search: (query: string, source?: string, pageSize?: number, signal?: AbortSignal) => apiClient.get('/anime/search', { params: { q: query, ...(source && { source }), ...(pageSize && { page_size: pageSize }) }, signal }),
  getTrending: () => apiClient.get('/anime/trending'),
  getPopular: () => apiClient.get('/anime/popular'),
  getRecent: () => apiClient.get('/anime/recent'),
  getSchedule: () => apiClient.get('/anime/schedule'),
  getOngoing: (params?: { sort?: string; order?: 'asc' | 'desc'; recent?: boolean }) =>
    apiClient.get('/anime/ongoing', { params }),
  getAnnounced: (limit = 20) => apiClient.get('/anime', { params: { status: 'announced', page_size: limit } }),
  getTop: (limit = 20) => apiClient.get('/anime/trending', { params: { page_size: limit } }),
  refresh: (id: string) => apiClient.post(`/anime/${id}/refresh`),
  resolveMAL: (malId: string) => apiClient.get(`/anime/mal/${malId}`),
  getGenres: () => apiClient.get('/genres'),
  getNews: () => apiClient.get('/anime/news'),
  getRelated: (animeId: string) => apiClient.get(`/anime/${animeId}/related`),
  // Phase 14 / UX-28 — soft social-proof: how many users have this anime
  // in their list with status='watching'. Public, no auth.
  getWatchersCount: (animeId: string) =>
    apiClient.get<{ count: number } | { data: { count: number } }>(`/anime/${animeId}/watchers-count`),
  // Phase 17 (UX-33) — editorial collections.
  listCollections: (limit = 12) =>
    apiClient.get<Collection[] | { data: Collection[] }>('/collections', { params: { limit } }),
  getCollection: (slug: string) =>
    apiClient.get<Collection | { data: Collection }>(`/collections/${encodeURIComponent(slug)}`),
  // Phase 18 (UX-34) — Skip-Intro / Skip-Outro CTAs. Public, no auth.
  // Backend proxies api.aniskip.com with a 7d cache. Empty response shape
  // when MAL ID is missing from aniskip's crowdsourced DB:
  //   { found: false, results: [] }
  getSkipTimes: (malId: string, episode: number) =>
    apiClient.get(`/skip-times/${encodeURIComponent(malId)}/${episode}`),
}

export const episodeApi = {
  getByAnimeId: (animeId: string) => apiClient.get(`/anime/${animeId}/episodes`),
  getById: (id: string) => apiClient.get(`/episodes/${id}`),
  getSources: (id: string) => apiClient.get(`/episodes/${id}/sources`)
}

export const userApi = {
  getProfile: () => apiClient.get('/users/profile'),
  updateProfile: (data: Record<string, unknown>) => apiClient.patch('/users/profile', data),
  getWatchlist: (params?: { status?: string; page?: number; per_page?: number; sort?: string; order?: string; q?: string }) =>
    apiClient.get('/users/watchlist', { params }),
  getWatchlistStatuses: () => apiClient.get('/users/watchlist/statuses'),
  getWatchlistEntry: (animeId: string) => apiClient.get(`/users/watchlist/${animeId}`),
  addToWatchlist: (animeId: string, status: string = 'plan_to_watch') =>
    apiClient.post('/users/watchlist', { anime_id: animeId, status }),
  updateWatchlistStatus: (animeId: string, status: string) =>
    apiClient.put('/users/watchlist', { anime_id: animeId, status }),
  updateWatchlistEntry: (data: {
    anime_id: string
    status: string
    started_at?: string | null
    completed_at?: string | null
    score?: number
    episodes?: number
    notes?: string
  }) => apiClient.put('/users/watchlist', data),
  removeFromWatchlist: (animeId: string) => apiClient.delete(`/users/watchlist/${animeId}`),
  markEpisodeWatched: (animeId: string, episode: number, combo?: Partial<WatchCombo>, sessionId?: string) =>
    apiClient.post(`/users/watchlist/${animeId}/episode`, {
      episode,
      ...combo,
      ...(sessionId ? { session_id: sessionId } : {}),
    }),
  getWatchHistory: () => apiClient.get('/users/history'),
  updateProgress: (data: Record<string, unknown>) => apiClient.post('/users/progress', data),
  getProgress: (animeId: string) => apiClient.get(`/users/progress/${animeId}`),
  // Phase 8 (UX-15 / UA-061) — Continue-Watching row. JWT-protected; returns
  // one item per anime (latest in-progress episode), ordered by last_watched_at DESC.
  getContinueWatching: (limit?: number) =>
    apiClient.get('/users/continue-watching', {
      params: typeof limit === 'number' ? { limit } : undefined,
    }),
  // Phase 9 (UX-16) — bulk per-card progress map. JWT-protected; ids capped
  // at 50 server-side. Returns { [animeId]: ProgressEntry } for animes the
  // user has progress on (missing animes omitted).
  getAnimeProgress: (ids: string[]) =>
    apiClient.get('/users/anime-progress', {
      params: { ids: ids.join(',') },
    }),
  getMyReviews: () => apiClient.get('/users/reviews'),
  importMAL: (username: string) => apiClient.post('/users/import/mal', { username }),
  importShikimori: (nickname: string) => apiClient.post('/users/import/shikimori', { nickname }),
  getImportJobStatus: (jobId: string) => apiClient.get(`/users/import/${jobId}`),
  getSyncStatus: () => apiClient.get('/users/sync/status'),
  exportJSON: () => apiClient.get('/users/export/json', { responseType: 'blob' }),
  migrateListEntry: (oldAnimeId: string, newAnimeId: string) =>
    apiClient.post('/users/watchlist/migrate', {
      old_anime_id: oldAnimeId,
      new_anime_id: newAnimeId,
    }),
  // Privacy settings
  updatePublicId: (publicId: string) => apiClient.put('/auth/profile/public-id', { public_id: publicId }),
  updatePrivacy: (publicStatuses: string[]) => apiClient.put('/auth/profile/privacy', { public_statuses: publicStatuses }),
  // Avatar
  updateAvatar: (avatar: string) => apiClient.put('/auth/profile/avatar', { avatar }),
  // Error reporting
  reportError: (data: Record<string, unknown>) => apiClient.post('/users/report', data),
  // API Key management
  generateApiKey: () => apiClient.post('/auth/api-key'),
  revokeApiKey: () => apiClient.delete('/auth/api-key'),
  hasApiKey: () => apiClient.get('/auth/api-key'),
  // Watch preferences
  resolvePreference: (animeId: string, available: WatchCombo[]) =>
    apiClient.post<ResolveResponse>('/preferences/resolve', { anime_id: animeId, available }),
  recordOverride: (data: {
    anime_id: string
    load_session_id: string
    dimension: 'language' | 'player' | 'team' | 'episode'
    original_combo: ResolvedCombo | null
    new_combo: Partial<WatchCombo> & { episode?: number }
    ms_since_load: number
    tier: string | null
    tier_number: number | null
    player: 'kodik' | 'animelib' | 'hianime' | 'consumet' | 'english'
  }) => apiClient.post('/preferences/override', data),
  getAnimePreference: (animeId: string) =>
    apiClient.get<WatchCombo & { updated_at: string }>(`/users/preferences/${animeId}`),
  getGlobalPreferences: () =>
    apiClient.get<{ top_combos: (WatchCombo & { count: number })[] }>('/users/preferences/global'),
  // Phase 7 — Advanced Settings + cross-device freshness
  getTier2DebugView: () =>
    apiClient.get<{
      coarse: Array<{ language: string; watch_type: string; weight: number }>
      fine: Array<{
        language: string
        watch_type: string
        player: string
        translation_id: string
        translation_title: string
        weight: number
      }>
      total_weight: number
      min_confidence: number
      half_life_days: number
      lock: {
        language: string
        watch_type: string
        top_translation_title: string
        confidence: number
      } | null
    }>('/users/preferences/tier2'),
  forceCombo: (animeId: string, combo: WatchCombo) =>
    apiClient.post(`/users/preferences/${animeId}/force`, combo),
  resetLearnedPreferences: () =>
    apiClient.delete<{ prefs_version: number }>('/users/preferences/learned'),
}

export const publicApi = {
  // Get public user profile by public_id
  getUserProfile: (publicId: string) => apiClient.get(`/auth/users/${publicId}`),
  // Get public watchlist
  getPublicWatchlist: (userId: string, params?: { status?: string; statuses?: string; page?: number; per_page?: number; sort?: string; order?: string; q?: string }) =>
    apiClient.get(`/users/${userId}/watchlist/public`, { params }),
  // Get public watchlist stats (avg score, total episodes)
  getPublicWatchlistStats: (userId: string, statuses?: string[]) =>
    apiClient.get(`/users/${userId}/watchlist/public/stats`, { params: statuses?.length ? { statuses: statuses.join(',') } : undefined }),
}

export const adminApi = {
  // Hide/unhide anime globally
  hideAnime: (animeId: string) => apiClient.post(`/admin/anime/${animeId}/hide`),
  unhideAnime: (animeId: string) => apiClient.delete(`/admin/anime/${animeId}/hide`),
  // Update shikimori_id
  updateShikimoriId: (animeId: string, shikimoriId: string) =>
    apiClient.patch(`/admin/anime/${animeId}/shikimori`, { shikimori_id: shikimoriId }),
  // Phase 17 (UX-33) — editorial collections admin CRUD + item picker.
  listCollections: () =>
    apiClient.get<Collection[] | { data: Collection[] }>('/admin/collections'),
  getCollection: (id: string) =>
    apiClient.get<Collection | { data: Collection }>(`/admin/collections/${id}`),
  createCollection: (body: CreateCollectionRequest) =>
    apiClient.post<Collection | { data: Collection }>('/admin/collections', body),
  updateCollection: (id: string, body: UpdateCollectionRequest) =>
    apiClient.put<Collection | { data: Collection }>(`/admin/collections/${id}`, body),
  deleteCollection: (id: string) =>
    apiClient.delete<void>(`/admin/collections/${id}`),
  addCollectionItem: (id: string, body: AddCollectionItemRequest) =>
    apiClient.post<CollectionItem | { data: CollectionItem }>(`/admin/collections/${id}/items`, body),
  removeCollectionItem: (id: string, animeId: string) =>
    apiClient.delete<void>(`/admin/collections/${id}/items/${animeId}`),
}

export const reviewApi = {
  // Get all reviews for an anime (public)
  getAnimeReviews: (animeId: string) => apiClient.get(`/anime/${animeId}/reviews`),
  // Get average rating for an anime (public)
  getAnimeRating: (animeId: string) => apiClient.get(`/anime/${animeId}/rating`),
  // Get current user's review for an anime
  getMyReview: (animeId: string) => apiClient.get(`/anime/${animeId}/reviews/me`),
  // Create or update a review
  createReview: (animeId: string, score: number, reviewText: string) =>
    apiClient.post(`/anime/${animeId}/reviews`, {
      anime_id: animeId,
      score,
      review_text: reviewText,
    }),
  // Delete a review
  deleteReview: (animeId: string) => apiClient.delete(`/anime/${animeId}/reviews`),
  // Get batch ratings for multiple anime
  getBatchRatings: (animeIds: string[]) =>
    apiClient.post('/anime/ratings/batch', { anime_ids: animeIds }),
}

export const commentApi = {
  // Get paginated comments for an anime (public, newest-first; cursor is opaque)
  getAnimeComments: (animeId: string, params?: { cursor?: string; limit?: number }) =>
    apiClient.get(`/anime/${animeId}/comments`, { params }),
  // Create a new comment on an anime (auth required, 1–2000 chars, 10/hr/(user,anime))
  createComment: (animeId: string, body: string) =>
    apiClient.post(`/anime/${animeId}/comments`, { body }),
  // Update an existing comment (owner only)
  updateComment: (animeId: string, commentId: string, body: string) =>
    apiClient.patch(`/anime/${animeId}/comments/${commentId}`, { body }),
  // Soft-delete a comment (owner or admin)
  deleteComment: (animeId: string, commentId: string) =>
    apiClient.delete(`/anime/${animeId}/comments/${commentId}`),
}

export const activityApi = {
  getFeed: (limit: number = 10, before?: string) =>
    apiClient.get('/activity/feed', {
      params: { limit, ...(before && { before }) }
    }),
}

export const gameApi = {
  getRooms: () => apiClient.get('/game/rooms'),
  getRoom: (id: string) => apiClient.get(`/game/rooms/${id}`),
  createRoom: (data: Record<string, unknown>) => apiClient.post('/game/rooms', data),
  joinRoom: (id: string) => apiClient.post(`/game/rooms/${id}/join`),
  leaveRoom: (id: string) => apiClient.post(`/game/rooms/${id}/leave`)
}

export const kodikApi = {
  getTranslations: (animeId: string) => apiClient.get(`/anime/${animeId}/kodik/translations`),
  getVideo: (animeId: string, episode: number, translationId: number) =>
    apiClient.get(`/anime/${animeId}/kodik/video`, {
      params: { episode, translation: translationId }
    }),
  search: (query: string) => apiClient.get('/kodik/search', { params: { q: query } }),
  getPinnedTranslations: (animeId: string) => apiClient.get(`/anime/${animeId}/pinned-translations`),
  pinTranslation: (animeId: string, translationId: number, title: string, type: string) =>
    apiClient.post(`/anime/${animeId}/pin-translation`, {
      translation_id: translationId,
      translation_title: title,
      translation_type: type
    }),
  unpinTranslation: (animeId: string, translationId: number) =>
    apiClient.delete(`/anime/${animeId}/pin-translation/${translationId}`)
}

export const hiAnimeApi = {
  getEpisodes: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/hianime/episodes`),
  getServers: (animeId: string, episodeId: string) =>
    apiClient.get(`/anime/${animeId}/hianime/servers`, {
      params: { episode: episodeId }
    }),
  getStream: (animeId: string, episodeId: string, serverId: string, category: string) =>
    apiClient.get(`/anime/${animeId}/hianime/stream`, {
      params: { episode: episodeId, server: serverId, category }
    }),
  search: (query: string) => apiClient.get('/hianime/search', { params: { q: query } }),
}

/**
 * Phase 16 — scraperApi targets the new /api/anime/{id}/scraper/* routes
 * served by the catalog→scraper pipeline. Replaces hiAnimeApi + consumetApi
 * end-to-end; those two remain in place (and reachable via ?legacy=1) until
 * Phase 20 cutover.
 *
 * The `prefer` parameter is the per-anime user override from the Source
 * dropdown inside EnglishPlayer; when omitted, the orchestrator picks its
 * default (currently AnimePahe — Phase 18 will add 9anime).
 */
export const scraperApi = {
  getEpisodes: (animeId: string, prefer?: string) =>
    apiClient.get(`/anime/${animeId}/scraper/episodes`, {
      params: prefer ? { prefer } : undefined,
    }),
  getServers: (animeId: string, episodeId: string, prefer?: string) =>
    apiClient.get(`/anime/${animeId}/scraper/servers`, {
      params: { episode: episodeId, ...(prefer && { prefer }) },
    }),
  getStream: (
    animeId: string,
    episodeId: string,
    serverId: string,
    category: 'sub' | 'dub',
    prefer?: string,
  ) =>
    apiClient.get(`/anime/${animeId}/scraper/stream`, {
      params: { episode: episodeId, server: serverId, category, ...(prefer && { prefer }) },
    }),
  // Health is per-service (not per-anime) but the catalog route is templated
  // on animeId for routing reasons. The catalog forwards to scraper without
  // touching animeId for the health path. Pass any UUID (e.g. an underscore
  // placeholder).
  getHealth: () => apiClient.get(`/anime/_/scraper/health`),
}

export const jimakuApi = {
  getSubtitles: (animeId: string, episode: number) =>
    apiClient.get(`/anime/${animeId}/jimaku/subtitles`, {
      params: { episode }
    }),
}

export const consumetApi = {
  getEpisodes: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/consumet/episodes`),
  getServers: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/consumet/servers`),
  getStream: (animeId: string, episodeId: string, serverName?: string) =>
    apiClient.get(`/anime/${animeId}/consumet/stream`, {
      params: { episode: episodeId, ...(serverName && { server: serverName }) }
    }),
  search: (query: string) => apiClient.get('/consumet/search', { params: { q: query } }),
}

export const animeLibApi = {
  getEpisodes: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/animelib/episodes`),
  getTranslations: (animeId: string, episodeId: number) =>
    apiClient.get(`/anime/${animeId}/animelib/translations`, {
      params: { episode: episodeId }
    }),
  getStream: (animeId: string, episodeId: number, translationId: number) =>
    apiClient.get(`/anime/${animeId}/animelib/stream`, {
      params: { episode: episodeId, translation: translationId }
    }),
  search: (query: string) => apiClient.get('/animelib/search', { params: { q: query } }),
}

export const hanimeApi = {
  getEpisodes: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/hanime/episodes`),
  getStream: (animeId: string, slug: string) =>
    apiClient.get(`/anime/${animeId}/hanime/stream`, {
      params: { slug }
    }),
}

export const themesApi = {
  list: (params?: { year?: number; season?: string; type?: string; sort?: string }) =>
    apiClient.get('/themes', { params }),
  get: (id: string) => apiClient.get(`/themes/${id}`),
  rate: (id: string, score: number) => apiClient.post(`/themes/${id}/rate`, { score }),
  unrate: (id: string) => apiClient.delete(`/themes/${id}/rate`),
  myRatings: (params?: { year?: number; season?: string }) =>
    apiClient.get('/themes/my-ratings', { params }),
}

export const adminThemesApi = {
  sync: () => apiClient.post('/themes/admin/sync'),
  syncStatus: () => apiClient.get('/themes/admin/sync/status'),
}

export const statusApi = {
  getStatus: () => apiClient.get('/status'),
}

export default apiClient
