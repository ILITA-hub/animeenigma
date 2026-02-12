import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { apiClient } from '@/api/client'

export interface User {
  id: string
  username: string
  email: string
  avatar?: string
  role: string
  public_id?: string
  public_statuses?: string[]
  created_at?: string
}

export interface LoginCredentials {
  username: string
  password: string
}

export interface RegisterData {
  username: string
  password: string
}

export interface TelegramAuthData {
  id: number
  first_name: string
  last_name?: string
  username?: string
  photo_url?: string
  auth_date: number
  hash: string
}

// Helper to safely parse user from localStorage
function loadUserFromStorage(): User | null {
  try {
    const stored = localStorage.getItem('user')
    if (stored) {
      return JSON.parse(stored)
    }
  } catch {
    localStorage.removeItem('user')
  }
  return null
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(loadUserFromStorage())
  const token = ref<string | null>(localStorage.getItem('token'))
  const loading = ref(false)
  const error = ref<string | null>(null)
  const isRefreshing = ref(false)

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  const setUser = (userData: User | null) => {
    user.value = userData
    if (userData) {
      localStorage.setItem('user', JSON.stringify(userData))
    } else {
      localStorage.removeItem('user')
    }
  }

  const setToken = (accessToken: string) => {
    token.value = accessToken
    localStorage.setItem('token', accessToken)
  }

  const login = async (credentials: LoginCredentials) => {
    loading.value = true
    error.value = null

    try {
      const response = await apiClient.post('/auth/login', credentials)
      const data = response.data?.data || response.data
      setToken(data.access_token)
      setUser(data.user)
      return true
    } catch (err: any) {
      error.value = err.response?.data?.error?.message || err.response?.data?.message || 'Ошибка входа'
      return false
    } finally {
      loading.value = false
    }
  }

  const register = async (data: RegisterData) => {
    loading.value = true
    error.value = null

    try {
      const response = await apiClient.post('/auth/register', data)
      const respData = response.data?.data || response.data
      setToken(respData.access_token)
      setUser(respData.user)
      return true
    } catch (err: any) {
      error.value = err.response?.data?.error?.message || err.response?.data?.message || 'Ошибка регистрации'
      return false
    } finally {
      loading.value = false
    }
  }

  const refreshAccessToken = async (): Promise<boolean> => {
    if (isRefreshing.value) {
      return false
    }

    isRefreshing.value = true
    try {
      // Refresh token is sent automatically via httpOnly cookie
      const response = await apiClient.post('/auth/refresh')
      const data = response.data?.data || response.data
      setToken(data.access_token)
      if (data.user) {
        setUser(data.user)
      }
      return true
    } catch {
      logout()
      return false
    } finally {
      isRefreshing.value = false
    }
  }

  const loginWithTelegram = async (telegramData: TelegramAuthData) => {
    loading.value = true
    error.value = null

    try {
      const response = await apiClient.post('/auth/telegram', telegramData)
      const data = response.data?.data || response.data
      setToken(data.access_token)
      setUser(data.user)
      return true
    } catch (err: any) {
      error.value = err.response?.data?.error?.message || err.response?.data?.message || 'Ошибка входа через Telegram'
      return false
    } finally {
      loading.value = false
    }
  }

  const logout = async () => {
    // Call logout endpoint to clear the httpOnly cookie
    try {
      await apiClient.post('/auth/logout')
    } catch {
      // Ignore errors, we're logging out anyway
    }
    setUser(null)
    token.value = null
    localStorage.removeItem('token')
  }

  const fetchUser = async () => {
    if (!token.value) return

    loading.value = true
    try {
      const response = await apiClient.get('/auth/me')
      const userData = response.data?.data || response.data
      setUser(userData)
    } catch {
      logout()
    } finally {
      loading.value = false
    }
  }

  const updateProfile = async (data: Partial<User>) => {
    loading.value = true
    error.value = null

    try {
      const response = await apiClient.patch('/auth/profile', data)
      const userData = response.data?.data || response.data
      setUser(userData)
      return true
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Update failed'
      return false
    } finally {
      loading.value = false
    }
  }

  return {
    user,
    token,
    loading,
    error,
    isAuthenticated,
    isAdmin,
    isRefreshing,
    login,
    register,
    loginWithTelegram,
    logout,
    fetchUser,
    updateProfile,
    refreshAccessToken
  }
})
