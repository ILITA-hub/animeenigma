import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { apiClient } from '@/api/client'

export interface User {
  id: string
  username: string
  email: string
  avatar?: string
  role: string
}

export interface LoginCredentials {
  username: string
  password: string
}

export interface RegisterData {
  username: string
  password: string
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const token = ref<string | null>(localStorage.getItem('token'))
  const loading = ref(false)
  const error = ref<string | null>(null)
  const isRefreshing = ref(false)

  const isAuthenticated = computed(() => !!token.value)

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
      user.value = data.user
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
      user.value = respData.user
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
      user.value = data.user
      return true
    } catch {
      logout()
      return false
    } finally {
      isRefreshing.value = false
    }
  }

  const logout = async () => {
    // Call logout endpoint to clear the httpOnly cookie
    try {
      await apiClient.post('/auth/logout')
    } catch {
      // Ignore errors, we're logging out anyway
    }
    user.value = null
    token.value = null
    localStorage.removeItem('token')
  }

  const fetchUser = async () => {
    if (!token.value) return

    loading.value = true
    try {
      const response = await apiClient.get('/auth/me')
      user.value = response.data
    } catch (err) {
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
      user.value = response.data
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
    isRefreshing,
    login,
    register,
    logout,
    fetchUser,
    updateProfile,
    refreshAccessToken
  }
})
