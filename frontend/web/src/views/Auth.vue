<template>
  <div class="min-h-screen flex items-center justify-center px-4 py-12">
    <!-- Background -->
    <div class="fixed inset-0 -z-10">
      <div class="absolute inset-0 bg-gradient-to-br from-base via-surface to-base" />
      <div class="absolute top-1/4 left-1/4 w-96 h-96 bg-cyan-500/10 rounded-full blur-3xl" />
      <div class="absolute bottom-1/4 right-1/4 w-96 h-96 bg-pink-500/10 rounded-full blur-3xl" />
    </div>

    <div class="w-full max-w-md">
      <!-- Logo -->
      <div class="text-center mb-8">
        <router-link to="/" class="inline-flex items-center gap-2 text-2xl font-bold">
          <span class="text-cyan-400">Anime</span>
          <span class="text-white">Enigma</span>
        </router-link>
      </div>

      <!-- Auth Card -->
      <div class="glass-card p-6 md:p-8">
        <h2 class="text-center text-white text-lg font-medium mb-6">Войти через Telegram</h2>

        <!-- Error -->
        <div v-if="authStore.error" class="mb-4 p-3 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400 text-sm">
          {{ authStore.error }}
        </div>

        <!-- Telegram Login Widget -->
        <div ref="telegramLoginContainer" class="flex justify-center">
          <!-- Telegram widget will be inserted here -->
        </div>
      </div>

      <!-- Back to home -->
      <p class="text-center mt-6 text-white/40 text-sm">
        <router-link to="/" class="hover:text-white transition-colors">
          ← Вернуться на главную
        </router-link>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore, type TelegramAuthData } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const telegramLoginContainer = ref<HTMLElement | null>(null)

// Telegram bot name from environment or default
const TELEGRAM_BOT_NAME = import.meta.env.VITE_TELEGRAM_BOT_NAME || ''

// Telegram Login Widget callback
const handleTelegramAuth = async (telegramUser: TelegramAuthData) => {
  const success = await authStore.loginWithTelegram(telegramUser)
  if (success) {
    router.push('/')
  }
}

// Expose callback to window for Telegram widget
declare global {
  interface Window {
    onTelegramAuth: (user: TelegramAuthData) => void
  }
}

onMounted(() => {
  // Set up global callback for Telegram widget
  window.onTelegramAuth = handleTelegramAuth

  // Only load widget if bot name is configured
  if (TELEGRAM_BOT_NAME && telegramLoginContainer.value) {
    // Create and append Telegram Login Widget script
    const script = document.createElement('script')
    script.async = true
    script.src = 'https://telegram.org/js/telegram-widget.js?22'
    script.setAttribute('data-telegram-login', TELEGRAM_BOT_NAME)
    script.setAttribute('data-size', 'large')
    script.setAttribute('data-radius', '8')
    script.setAttribute('data-onauth', 'onTelegramAuth(user)')
    script.setAttribute('data-request-access', 'write')
    script.setAttribute('data-userpic', 'false')
    telegramLoginContainer.value.appendChild(script)
  }
})
</script>
