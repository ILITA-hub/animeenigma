import axios, { AxiosInstance, InternalAxiosRequestConfig, AxiosResponse } from 'axios'

const BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8000/api'

export const apiClient: AxiosInstance = axios.create({
  baseURL: BASE_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

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

// Response interceptor
apiClient.interceptors.response.use(
  (response: AxiosResponse) => {
    return response
  },
  (error) => {
    if (error.response?.status === 401) {
      // Token expired or invalid
      localStorage.removeItem('token')
      window.location.href = '/'
    }
    return Promise.reject(error)
  }
)

// API endpoints
export const animeApi = {
  getAll: (params?: any) => apiClient.get('/anime', { params }),
  getById: (id: string) => apiClient.get(`/anime/${id}`),
  search: (query: string) => apiClient.get('/anime/search', { params: { q: query } }),
  getTrending: () => apiClient.get('/anime/trending'),
  getPopular: () => apiClient.get('/anime/popular'),
  getRecent: () => apiClient.get('/anime/recent')
}

export const episodeApi = {
  getByAnimeId: (animeId: string) => apiClient.get(`/anime/${animeId}/episodes`),
  getById: (id: string) => apiClient.get(`/episodes/${id}`),
  getSources: (id: string) => apiClient.get(`/episodes/${id}/sources`)
}

export const userApi = {
  getProfile: () => apiClient.get('/users/profile'),
  updateProfile: (data: any) => apiClient.patch('/users/profile', data),
  getWatchlist: () => apiClient.get('/users/watchlist'),
  addToWatchlist: (animeId: string) => apiClient.post('/users/watchlist', { animeId }),
  removeFromWatchlist: (animeId: string) => apiClient.delete(`/users/watchlist/${animeId}`),
  getWatchHistory: () => apiClient.get('/users/history'),
  updateProgress: (data: any) => apiClient.post('/users/progress', data)
}

export const gameApi = {
  getRooms: () => apiClient.get('/game/rooms'),
  getRoom: (id: string) => apiClient.get(`/game/rooms/${id}`),
  createRoom: (data: any) => apiClient.post('/game/rooms', data),
  joinRoom: (id: string) => apiClient.post(`/game/rooms/${id}/join`),
  leaveRoom: (id: string) => apiClient.post(`/game/rooms/${id}/leave`)
}

export default apiClient
