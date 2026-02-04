<template>
  <div class="min-h-screen flex items-center justify-center px-4">
    <div class="max-w-md w-full">
      <div class="glass-card p-8 text-center">
        <div class="w-20 h-20 mx-auto mb-6 rounded-full bg-cyan-500/20 flex items-center justify-center">
          <svg class="w-10 h-10 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
          </svg>
        </div>

        <h1 class="text-2xl font-bold text-white mb-2">Настройка профиля</h1>
        <p class="text-white/60 mb-6">Создай уникальную ссылку на свой профиль, чтобы делиться списком аниме</p>

        <div class="space-y-4">
          <div>
            <label class="block text-white/60 text-sm mb-2 text-left">Ссылка на профиль</label>
            <div class="flex items-center bg-white/10 border border-white/10 rounded-lg overflow-hidden">
              <span class="px-3 text-white/40 text-sm">/user/</span>
              <input
                v-model="publicId"
                type="text"
                placeholder="your-username"
                class="flex-1 bg-transparent py-3 pr-3 text-white placeholder-white/40 focus:outline-none"
                :disabled="saving"
                @keyup.enter="save"
              />
            </div>
            <p v-if="error" class="text-pink-400 text-xs mt-2 text-left">{{ error }}</p>
            <p class="text-white/40 text-xs mt-2 text-left">
              Только латинские буквы, цифры и дефис. Минимум 3 символа.
            </p>
          </div>

          <Button
            variant="primary"
            full-width
            :disabled="!publicId || saving"
            @click="save"
          >
            <svg v-if="saving" class="w-4 h-4 animate-spin mr-2" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            {{ saving ? 'Сохранение...' : 'Создать профиль' }}
          </Button>

          <Button variant="ghost" full-width @click="$router.push('/')">
            Позже
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { Button } from '@/components/ui'
import { userApi } from '@/api/client'

const router = useRouter()
const authStore = useAuthStore()

const publicId = ref('')
const saving = ref(false)
const error = ref<string | null>(null)

const save = async () => {
  if (!publicId.value) return

  const validPattern = /^[a-zA-Z0-9-]{3,32}$/
  if (!validPattern.test(publicId.value)) {
    error.value = 'Только латинские буквы, цифры и дефис (3-32 символа)'
    return
  }

  saving.value = true
  error.value = null

  try {
    await userApi.updatePublicId(publicId.value)
    await authStore.fetchUser()
    router.push(`/user/${publicId.value}`)
  } catch (err: any) {
    const message = err.response?.data?.message || err.response?.data?.error
    if (message?.includes('already taken') || message?.includes('уже занят')) {
      error.value = 'Эта ссылка уже занята'
    } else {
      error.value = message || 'Не удалось сохранить'
    }
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  // If user already has public_id, redirect to their profile
  if (!authStore.user) {
    await authStore.fetchUser()
  }
  if (authStore.user?.public_id) {
    router.replace(`/user/${authStore.user.public_id}`)
  } else if (!authStore.user) {
    router.replace('/auth')
  } else {
    // Suggest username as default
    publicId.value = authStore.user.username?.toLowerCase().replace(/[^a-z0-9-]/g, '-') || ''
  }
})
</script>
