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
  email: string
  password: string
}

export interface RegisterData {
  username: string
  email: string
  password: string
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const token = ref<string | null>(localStorage.getItem('token'))
  const loading = ref(false)
  const error = ref<string | null>(null)

  const isAuthenticated = computed(() => !!token.value)

  const login = async (credentials: LoginCredentials) => {
    loading.value = true
    error.value = null

    try {
      const response = await apiClient.post('/auth/login', credentials)
      token.value = response.data.token
      user.value = response.data.user
      localStorage.setItem('token', response.data.token)
      return true
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Login failed'
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
      token.value = response.data.token
      user.value = response.data.user
      localStorage.setItem('token', response.data.token)
      return true
    } catch (err: any) {
      error.value = err.response?.data?.message || 'Registration failed'
      return false
    } finally {
      loading.value = false
    }
  }

  const logout = () => {
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
    login,
    register,
    logout,
    fetchUser,
    updateProfile
  }
})
