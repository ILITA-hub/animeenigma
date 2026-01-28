import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import type { LoginCredentials, RegisterData } from '@/stores/auth'

export function useAuth() {
  const authStore = useAuthStore()
  const router = useRouter()
  const isLoading = ref(false)

  const user = computed(() => authStore.user)
  const isAuthenticated = computed(() => authStore.isAuthenticated)
  const error = computed(() => authStore.error)

  const login = async (credentials: LoginCredentials) => {
    isLoading.value = true
    try {
      const success = await authStore.login(credentials)
      if (success) {
        await authStore.fetchUser()
        router.push('/')
      }
      return success
    } finally {
      isLoading.value = false
    }
  }

  const register = async (data: RegisterData) => {
    isLoading.value = true
    try {
      const success = await authStore.register(data)
      if (success) {
        await authStore.fetchUser()
        router.push('/')
      }
      return success
    } finally {
      isLoading.value = false
    }
  }

  const logout = async () => {
    authStore.logout()
    router.push('/')
  }

  const updateProfile = async (data: any) => {
    isLoading.value = true
    try {
      return await authStore.updateProfile(data)
    } finally {
      isLoading.value = false
    }
  }

  return {
    user,
    isAuthenticated,
    isLoading,
    error,
    login,
    register,
    logout,
    updateProfile
  }
}
