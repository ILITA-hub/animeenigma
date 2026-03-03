import axios, { AxiosInstance, InternalAxiosRequestConfig, AxiosResponse, AxiosError } from 'axios'
import { hookAxiosDiagnostics } from '@/utils/diagnostics'

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

// Shared token refresh logic — deduplicates concurrent refresh calls
async function doTokenRefresh(): Promise<string | null> {
  if (refreshPromise) return refreshPromise

  isRefreshing = true
  refreshPromise = (async () => {
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
      localStorage.removeItem('token')
      window.location.href = '/'
      return null
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
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// Response interceptor — handles 401 errors and X-Token-Expired fallback
apiClient.interceptors.response.use(
  async (response: AxiosResponse) => {
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
        // Queue request while refresh is in progress
        return new Promise((resolve, reject) => {
          failedQueue.push({ resolve, reject })
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

// API endpoints
export const animeApi = {
  getAll: (params?: Record<string, unknown>) => apiClient.get('/anime', { params }),
  getById: (id: string) => apiClient.get(`/anime/${id}`),
  search: (query: string, source?: string) => apiClient.get('/anime/search', { params: { q: query, ...(source && { source }) } }),
  getTrending: () => apiClient.get('/anime/trending'),
  getPopular: () => apiClient.get('/anime/popular'),
  getRecent: () => apiClient.get('/anime/recent'),
  getSchedule: () => apiClient.get('/anime/schedule'),
  getOngoing: () => apiClient.get('/anime/ongoing'),
  getAnnounced: (limit = 20) => apiClient.get('/anime', { params: { status: 'announced', page_size: limit } }),
  getTop: (limit = 20) => apiClient.get('/anime', { params: { sort: 'score', order: 'desc', page_size: limit } }),
  refresh: (id: string) => apiClient.post(`/anime/${id}/refresh`),
  resolveMAL: (malId: string) => apiClient.get(`/anime/mal/${malId}`),
  getGenres: () => apiClient.get('/genres'),
}

export const episodeApi = {
  getByAnimeId: (animeId: string) => apiClient.get(`/anime/${animeId}/episodes`),
  getById: (id: string) => apiClient.get(`/episodes/${id}`),
  getSources: (id: string) => apiClient.get(`/episodes/${id}/sources`)
}

export const userApi = {
  getProfile: () => apiClient.get('/users/profile'),
  updateProfile: (data: Record<string, unknown>) => apiClient.patch('/users/profile', data),
  getWatchlist: (status?: string) => apiClient.get('/users/watchlist', { params: status ? { status } : {} }),
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
  markEpisodeWatched: (animeId: string, episode: number) =>
    apiClient.post(`/users/watchlist/${animeId}/episode`, { episode }),
  getWatchHistory: () => apiClient.get('/users/history'),
  updateProgress: (data: Record<string, unknown>) => apiClient.post('/users/progress', data),
  getMyReviews: () => apiClient.get('/users/reviews'),
  importMAL: (username: string) => apiClient.post('/users/import/mal', { username }),
  importShikimori: (nickname: string) => apiClient.post('/users/import/shikimori', { nickname }),
  getImportJobStatus: (jobId: string) => apiClient.get(`/users/import/${jobId}`),
  getSyncStatus: () => apiClient.get('/users/sync/status'),
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
}

export const publicApi = {
  // Get public user profile by public_id
  getUserProfile: (publicId: string) => apiClient.get(`/auth/users/${publicId}`),
  // Get public watchlist
  getPublicWatchlist: (userId: string, statuses?: string[]) =>
    apiClient.get(`/users/${userId}/watchlist/public`, {
      params: statuses?.length ? { statuses: statuses.join(',') } : {}
    }),
}

export const adminApi = {
  // Hide/unhide anime globally
  hideAnime: (animeId: string) => apiClient.post(`/admin/anime/${animeId}/hide`),
  unhideAnime: (animeId: string) => apiClient.delete(`/admin/anime/${animeId}/hide`),
  // Update shikimori_id
  updateShikimoriId: (animeId: string, shikimoriId: string) =>
    apiClient.patch(`/admin/anime/${animeId}/shikimori`, { shikimori_id: shikimoriId }),
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

export default apiClient
