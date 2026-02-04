import axios, { AxiosInstance, InternalAxiosRequestConfig, AxiosResponse, AxiosError } from 'axios'

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

// Request interceptor
apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = localStorage.getItem('token')
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// Response interceptor with token refresh
apiClient.interceptors.response.use(
  (response: AxiosResponse) => {
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
      isRefreshing = true

      try {
        // Refresh token is sent automatically via httpOnly cookie
        const response = await axios.post(`${BASE_URL}/auth/refresh`, {}, {
          withCredentials: true
        })

        const data = response.data?.data || response.data
        const newAccessToken = data.access_token

        localStorage.setItem('token', newAccessToken)

        originalRequest.headers.Authorization = `Bearer ${newAccessToken}`
        processQueue(null, newAccessToken)

        return apiClient(originalRequest)
      } catch (refreshError) {
        processQueue(refreshError as AxiosError, null)
        localStorage.removeItem('token')
        window.location.href = '/'
        return Promise.reject(refreshError)
      } finally {
        isRefreshing = false
      }
    }

    return Promise.reject(error)
  }
)

// API endpoints
export const animeApi = {
  getAll: (params?: any) => apiClient.get('/anime', { params }),
  getById: (id: string) => apiClient.get(`/anime/${id}`),
  search: (query: string, source?: string) => apiClient.get('/anime/search', { params: { q: query, ...(source && { source }) } }),
  getTrending: () => apiClient.get('/anime/trending'),
  getPopular: () => apiClient.get('/anime/popular'),
  getRecent: () => apiClient.get('/anime/recent'),
  getSchedule: () => apiClient.get('/anime/schedule'),
  getOngoing: (limit = 20) => apiClient.get('/anime/ongoing', { params: { page_size: limit } }),
  getAnnounced: (limit = 20) => apiClient.get('/anime', { params: { status: 'announced', page_size: limit } }),
  getTop: (limit = 20) => apiClient.get('/anime', { params: { sort: 'score', order: 'desc', page_size: limit } }),
  refresh: (id: string) => apiClient.post(`/anime/${id}/refresh`)
}

export const episodeApi = {
  getByAnimeId: (animeId: string) => apiClient.get(`/anime/${animeId}/episodes`),
  getById: (id: string) => apiClient.get(`/episodes/${id}`),
  getSources: (id: string) => apiClient.get(`/episodes/${id}/sources`)
}

export const userApi = {
  getProfile: () => apiClient.get('/users/profile'),
  updateProfile: (data: any) => apiClient.patch('/users/profile', data),
  getWatchlist: (status?: string) => apiClient.get('/users/watchlist', { params: status ? { status } : {} }),
  getWatchlistEntry: (animeId: string) => apiClient.get(`/users/watchlist/${animeId}`),
  addToWatchlist: (animeId: string, status: string = 'plan_to_watch', animeTitle?: string, animeCover?: string, animeTotalEpisodes?: number) =>
    apiClient.post('/users/watchlist', { anime_id: animeId, status, anime_title: animeTitle, anime_cover: animeCover, anime_total_episodes: animeTotalEpisodes }),
  updateWatchlistStatus: (animeId: string, status: string, animeTitle?: string, animeCover?: string, animeTotalEpisodes?: number) =>
    apiClient.put('/users/watchlist', { anime_id: animeId, status, anime_title: animeTitle, anime_cover: animeCover, anime_total_episodes: animeTotalEpisodes }),
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
  markEpisodeWatched: (animeId: string, episode: number) => apiClient.post(`/users/watchlist/${animeId}/episode`, { episode }),
  getWatchHistory: () => apiClient.get('/users/history'),
  updateProgress: (data: any) => apiClient.post('/users/progress', data),
  getMyReviews: () => apiClient.get('/users/reviews'),
  importMAL: (username: string) => apiClient.post('/users/import/mal', { username }),
  // Privacy settings
  updatePublicId: (publicId: string) => apiClient.put('/auth/profile/public-id', { public_id: publicId }),
  updatePrivacy: (publicStatuses: string[]) => apiClient.put('/auth/profile/privacy', { public_statuses: publicStatuses }),
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
  createReview: (animeId: string, score: number, reviewText: string, animeTitle?: string, animeCover?: string) =>
    apiClient.post(`/anime/${animeId}/reviews`, {
      anime_id: animeId,
      score,
      review_text: reviewText,
      anime_title: animeTitle,
      anime_cover: animeCover,
    }),
  // Delete a review
  deleteReview: (animeId: string) => apiClient.delete(`/anime/${animeId}/reviews`),
}

export const gameApi = {
  getRooms: () => apiClient.get('/game/rooms'),
  getRoom: (id: string) => apiClient.get(`/game/rooms/${id}`),
  createRoom: (data: any) => apiClient.post('/game/rooms', data),
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

export default apiClient
