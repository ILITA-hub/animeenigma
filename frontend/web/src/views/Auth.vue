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
        <h2 class="text-center text-white text-lg font-medium mb-6">{{ $t('auth.telegramLogin') }}</h2>

        <!-- Error -->
        <div v-if="authStore.error" class="mb-4 p-3 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400 text-sm">
          {{ authStore.error }}
        </div>

        <!-- Loading state -->
        <div v-if="state === 'loading'" class="flex flex-col items-center gap-3">
          <div class="animate-spin h-8 w-8 border-2 border-cyan-400 border-t-transparent rounded-full" />
          <span class="text-white/40 text-sm">{{ $t('auth.loading') }}</span>
        </div>

        <!-- QR + Deep link -->
        <div v-else-if="state === 'ready'" class="flex flex-col items-center gap-5">
          <!-- QR Code -->
          <div class="bg-white rounded-xl p-3">
            <canvas ref="qrCanvas" />
          </div>

          <!-- Open in Telegram button -->
          <a
            :href="deeplinkUrl"
            target="_blank"
            rel="noopener"
            class="inline-flex items-center gap-2 px-6 py-3 bg-[#54a9eb] hover:bg-[#4a96d2] text-white font-medium rounded-lg transition-colors w-full justify-center"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
              <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/>
            </svg>
            {{ $t('auth.openInTelegram') }}
          </a>

          <!-- Timer -->
          <p class="text-white/30 text-xs">
            {{ $t('auth.expiresIn', { seconds: remainingSeconds }) }}
          </p>

          <!-- Troubleshooting hint (shows after 30 seconds) -->
          <p v-if="showTroubleshootingHint" class="text-white/30 text-xs text-center">
            {{ $t('auth.troubleshootingHint') }}
          </p>
        </div>

        <!-- Expired state -->
        <div v-else-if="state === 'expired'" class="flex flex-col items-center gap-4">
          <p class="text-white/50 text-sm">{{ $t('auth.sessionExpired') }}</p>
          <button
            class="px-6 py-3 bg-cyan-500 hover:bg-cyan-600 text-white font-medium rounded-lg transition-colors"
            @click="startAuth"
          >
            {{ $t('auth.tryAgain') }}
          </button>
        </div>
      </div>

      <!-- Back to home -->
      <p class="text-center mt-6 text-white/40 text-sm">
        <router-link to="/" class="hover:text-white transition-colors">
          {{ '\u2190 ' + $t('auth.backHome') }}
        </router-link>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import QRCode from 'qrcode'

useI18n()

const router = useRouter()
const authStore = useAuthStore()
const qrCanvas = ref<HTMLCanvasElement | null>(null)

const state = ref<'loading' | 'ready' | 'expired'>('loading')
const deeplinkUrl = ref('')
const authToken = ref('')
const remainingSeconds = ref(300)
const showTroubleshootingHint = ref(false)

let pollInterval: ReturnType<typeof setInterval> | null = null
let countdownInterval: ReturnType<typeof setInterval> | null = null
let troubleshootingTimeout: ReturnType<typeof setTimeout> | null = null
let pollCount = 0
const MAX_POLLS = 150

async function startAuth() {
  state.value = 'loading'
  showTroubleshootingHint.value = false
  pollCount = 0
  cleanup()

  const result = await authStore.requestDeepLink()
  if (!result) {
    state.value = 'expired'
    return
  }

  authToken.value = result.token
  deeplinkUrl.value = result.deeplink_url
  remainingSeconds.value = result.expires_in
  state.value = 'ready'

  // Render QR code after DOM update
  await nextTick()
  if (qrCanvas.value) {
    QRCode.toCanvas(qrCanvas.value, result.deeplink_url, {
      width: 200,
      margin: 0,
      color: { dark: '#000000', light: '#ffffff' },
    })
  }

  // Start polling
  pollInterval = setInterval(pollForAuth, 2000)

  // Start countdown
  countdownInterval = setInterval(() => {
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) {
      state.value = 'expired'
      cleanup()
    }
  }, 1000)

  // Show troubleshooting hint after 30 seconds
  troubleshootingTimeout = setTimeout(() => {
    showTroubleshootingHint.value = true
  }, 30000)
}

async function pollForAuth() {
  if (!authToken.value || pollCount >= MAX_POLLS) {
    state.value = 'expired'
    cleanup()
    return
  }

  pollCount++
  const result = await authStore.checkDeepLink(authToken.value)

  if (result?.status === 'confirmed') {
    cleanup()
    const returnUrl = sessionStorage.getItem('returnUrl')
    sessionStorage.removeItem('returnUrl')
    router.push(returnUrl || '/')
  } else if (result?.status === 'expired') {
    state.value = 'expired'
    cleanup()
  }
}

function cleanup() {
  if (pollInterval) { clearInterval(pollInterval); pollInterval = null }
  if (countdownInterval) { clearInterval(countdownInterval); countdownInterval = null }
  if (troubleshootingTimeout) { clearTimeout(troubleshootingTimeout); troubleshootingTimeout = null }
}

onMounted(() => {
  // UA-027: if user is already authed, bounce them out rather than show a fresh QR.
  if (authStore.isAuthenticated) {
    const returnUrl = sessionStorage.getItem('returnUrl')
    sessionStorage.removeItem('returnUrl')
    router.replace(returnUrl || '/')
    return
  }
  startAuth()
})

onUnmounted(() => {
  cleanup()
})
</script>
