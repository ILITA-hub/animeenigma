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
        <!-- Tabs -->
        <div class="flex mb-6 bg-white/5 rounded-lg p-1">
          <button
            class="flex-1 py-2.5 rounded-md text-sm font-medium transition-all"
            :class="mode === 'login' ? 'bg-cyan-500 text-black' : 'text-white/60 hover:text-white'"
            @click="mode = 'login'"
          >
            Вход
          </button>
          <button
            class="flex-1 py-2.5 rounded-md text-sm font-medium transition-all"
            :class="mode === 'register' ? 'bg-cyan-500 text-black' : 'text-white/60 hover:text-white'"
            @click="mode = 'register'"
          >
            Регистрация
          </button>
        </div>

        <!-- Login Form -->
        <form v-if="mode === 'login'" @submit.prevent="handleLogin" class="space-y-4">
          <div>
            <label class="block text-white/60 text-sm mb-2">Имя пользователя</label>
            <input
              v-model="loginForm.username"
              type="text"
              required
              class="w-full bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-white placeholder-white/30 focus:outline-none focus:border-cyan-500 transition-colors"
              placeholder="username"
            />
          </div>
          <div>
            <label class="block text-white/60 text-sm mb-2">Пароль</label>
            <input
              v-model="loginForm.password"
              type="password"
              required
              class="w-full bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-white placeholder-white/30 focus:outline-none focus:border-cyan-500 transition-colors"
              placeholder="••••••••"
            />
          </div>

          <!-- Error -->
          <div v-if="authStore.error" class="p-3 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400 text-sm">
            {{ authStore.error }}
          </div>

          <button
            type="submit"
            :disabled="authStore.loading"
            class="w-full py-3 bg-cyan-500 hover:bg-cyan-400 text-black font-medium rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <span v-if="authStore.loading" class="flex items-center justify-center gap-2">
              <svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Вход...
            </span>
            <span v-else>Войти</span>
          </button>
        </form>

        <!-- Register Form -->
        <form v-else @submit.prevent="handleRegister" class="space-y-4">
          <div>
            <label class="block text-white/60 text-sm mb-2">Имя пользователя</label>
            <input
              v-model="registerForm.username"
              type="text"
              required
              minlength="3"
              maxlength="32"
              class="w-full bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-white placeholder-white/30 focus:outline-none focus:border-cyan-500 transition-colors"
              placeholder="username (3-32 символа)"
            />
          </div>
          <div>
            <label class="block text-white/60 text-sm mb-2">Пароль</label>
            <input
              v-model="registerForm.password"
              type="password"
              required
              minlength="6"
              class="w-full bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-white placeholder-white/30 focus:outline-none focus:border-cyan-500 transition-colors"
              placeholder="Минимум 6 символов"
            />
          </div>
          <div>
            <label class="block text-white/60 text-sm mb-2">Подтвердите пароль</label>
            <input
              v-model="registerForm.confirmPassword"
              type="password"
              required
              class="w-full bg-white/5 border border-white/10 rounded-lg px-4 py-3 text-white placeholder-white/30 focus:outline-none focus:border-cyan-500 transition-colors"
              placeholder="••••••••"
            />
          </div>

          <!-- Password mismatch -->
          <div v-if="passwordMismatch" class="p-3 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400 text-sm">
            Пароли не совпадают
          </div>

          <!-- Error -->
          <div v-if="authStore.error" class="p-3 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400 text-sm">
            {{ authStore.error }}
          </div>

          <button
            type="submit"
            :disabled="authStore.loading || passwordMismatch"
            class="w-full py-3 bg-cyan-500 hover:bg-cyan-400 text-black font-medium rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <span v-if="authStore.loading" class="flex items-center justify-center gap-2">
              <svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              Регистрация...
            </span>
            <span v-else>Зарегистрироваться</span>
          </button>
        </form>

        <!-- Divider -->
        <div class="my-6 flex items-center">
          <div class="flex-1 border-t border-white/10"></div>
          <span class="px-4 text-white/30 text-sm">или</span>
          <div class="flex-1 border-t border-white/10"></div>
        </div>

        <!-- Social Login -->
        <div class="space-y-3">
          <!-- Telegram Login Widget Container -->
          <div ref="telegramLoginContainer" class="flex justify-center">
            <!-- Telegram widget will be inserted here -->
          </div>

          <button
            type="button"
            class="w-full py-3 bg-white/5 hover:bg-white/10 border border-white/10 rounded-lg text-white/70 font-medium transition-colors flex items-center justify-center gap-3"
            @click="socialLogin('shikimori')"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/>
            </svg>
            Войти через Shikimori
          </button>
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
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore, type TelegramAuthData } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const mode = ref<'login' | 'register'>('login')
const telegramLoginContainer = ref<HTMLElement | null>(null)

// Telegram bot name from environment or default
const TELEGRAM_BOT_NAME = import.meta.env.VITE_TELEGRAM_BOT_NAME || ''

const loginForm = ref({
  username: '',
  password: ''
})

const registerForm = ref({
  username: '',
  password: '',
  confirmPassword: ''
})

const passwordMismatch = computed(() => {
  return registerForm.value.password !== registerForm.value.confirmPassword && registerForm.value.confirmPassword !== ''
})

const handleLogin = async () => {
  const success = await authStore.login(loginForm.value)
  if (success) {
    router.push('/')
  }
}

const handleRegister = async () => {
  if (passwordMismatch.value) return

  const success = await authStore.register({
    username: registerForm.value.username,
    password: registerForm.value.password
  })
  if (success) {
    router.push('/')
  }
}

const socialLogin = (provider: string) => {
  // TODO: Implement OAuth login
  console.log('Social login:', provider)
}

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
