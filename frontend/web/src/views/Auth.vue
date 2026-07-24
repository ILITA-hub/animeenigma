<template>
  <div class="min-h-[calc(100vh-var(--header-offset))] flex items-center justify-center px-4 py-12">
    <!-- Background -->
    <div class="fixed inset-0 -z-10">
      <div class="absolute inset-0 bg-gradient-to-br from-base via-surface to-base" />
      <div class="absolute top-1/4 left-1/4 w-96 h-96 bg-cyan-500/10 rounded-full blur-3xl" />
      <div class="absolute bottom-1/4 right-1/4 w-96 h-96 bg-pink-500/10 rounded-full blur-3xl" />
    </div>

    <div class="w-full max-w-md">
      <!-- Logo -->
      <div class="text-center mb-8">
        <router-link to="/" class="inline-flex items-center gap-2 text-2xl font-semibold">
          <span class="text-cyan-400">Anime</span>
          <span class="text-white">Enigma</span>
        </router-link>
      </div>

      <!-- Auth Card -->
      <div class="glass-card p-6 md:p-8">
        <h1 class="sr-only">{{ $t('auth.heading') }}</h1>
        <h2 class="text-center text-white text-lg font-medium mb-6">{{ $t('auth.telegramLogin') }}</h2>

        <!-- Error -->
        <div v-if="authStore.error" class="mb-4 p-3 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400 text-sm">
          {{ authStore.error }}
        </div>

        <!-- Expired state -->
        <div v-if="expired" class="flex flex-col items-center gap-4">
          <p class="text-white/50 text-sm">{{ $t('auth.sessionExpired') }}</p>
          <Button
            variant="default"
            size="md"
            @click="startAuth"
          >
            {{ $t('auth.tryAgain') }}
          </Button>
        </div>

        <!-- Active state (QR card visible immediately; canvas paints when token arrives) -->
        <div v-else class="flex flex-col items-center gap-5">
          <!-- QR Code (renders empty placeholder + spinner until token arrives) -->
          <div class="bg-white rounded-xl p-3 relative w-[216px] h-[216px] flex items-center justify-center">
            <canvas ref="qrCanvas" role="img" :aria-label="$t('auth.qrAlt')" :class="['transition-opacity duration-200', tokenReady ? 'opacity-100' : 'opacity-0']" />
            <div
              v-if="!tokenReady"
              class="absolute inset-0 flex items-center justify-center"
              aria-hidden="true"
            >
              <Spinner size="md" tone="mono" />
            </div>
          </div>

          <!-- Open in Telegram (tg:// for instant native app launch) -->
          <a
            :href="appUrl || '#'"
            :class="[
              'inline-flex items-center gap-2 px-6 py-3 bg-[#54a9eb] hover:bg-[#4a96d2] text-white font-medium rounded-lg transition-colors w-full justify-center',
              { 'opacity-50 pointer-events-none': !tokenReady },
            ]"
            target="_blank"
            rel="noopener"
            @click="trackClicked"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
              <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z"/>
            </svg>
            {{ $t('auth.openInTelegram') }}
          </a>

          <!-- Browser fallback for users without the native app -->
          <a
            v-if="tokenReady"
            :href="webUrl"
            target="_blank"
            rel="noopener"
            class="text-white/60 hover:text-white/70 text-xs underline-offset-2 hover:underline transition-colors"
          >
            {{ $t('auth.openInBrowser') }}
          </a>

          <!-- Timer -->
          <p v-if="tokenReady" class="text-white/30 text-xs">
            {{ $t('auth.expiresIn', { seconds: remainingSeconds }) }}
          </p>

          <!-- Troubleshooting hint (shows after 30 seconds) -->
          <p v-if="showTroubleshootingHint" class="text-white/30 text-xs text-center">
            {{ $t('auth.troubleshootingHint') }}
          </p>

          <!-- Telegram Web manual fallback. Always available because tg-web does
               not always auto-execute /start payloads from t.me links (returning
               users who have chatted with the bot before silently drop the start
               param). Copy-paste works universally. -->
          <details v-if="tokenReady" class="w-full text-xs">
            <summary class="cursor-pointer text-white/60 hover:text-white/70 transition-colors select-none text-center">
              {{ $t('auth.tgWebToggle') }}
            </summary>
            <div class="mt-3 p-3 bg-black/30 border border-white/10 rounded-lg space-y-2">
              <p class="text-white/60 leading-relaxed">
                {{ $t('auth.tgWebInstructions') }}
              </p>
              <div class="flex items-center gap-2">
                <code class="flex-1 font-mono text-cyan-300 break-all select-all px-2 py-1.5 bg-black/40 rounded">{{ manualCommand }}</code>
                <button
                  type="button"
                  class="px-2 py-1.5 rounded bg-cyan-500/20 hover:bg-cyan-500/30 text-cyan-300 text-xs transition-colors whitespace-nowrap"
                  @click="copyManual"
                >
                  {{ copied ? $t('auth.tgWebCopied') : $t('auth.tgWebCopy') }}
                </button>
              </div>
            </div>
          </details>

          <!-- Passkey login (spec 2026-07-24): small secondary action -->
          <button
            v-if="passkeySupported"
            type="button"
            class="inline-flex items-center gap-2 text-white/60 hover:text-white text-sm transition-colors"
            @click="loginWithPasskey"
          >
            <KeyRound class="w-4 h-4" aria-hidden="true" />
            {{ $t('auth.passkeyLogin') }}
          </button>
        </div>
      </div>

      <!-- Back to home -->
      <p class="text-center mt-6 text-white/60 text-sm">
        <router-link to="/" class="hover:text-white transition-colors">
          {{ '← ' + $t('auth.backHome') }}
        </router-link>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import Button from '@/components/ui/Button.vue'
import { Spinner } from '@/components/ui'
import QRCode from 'qrcode'
import { KeyRound } from 'lucide-vue-next'

useI18n()

const router = useRouter()
const authStore = useAuthStore()
const qrCanvas = ref<HTMLCanvasElement | null>(null)

// Two URLs derived from the backend's https://t.me/{bot}?start={token}:
// - appUrl (tg://resolve?...) — instant native-app launch, bypasses t.me bouncer
// - webUrl (https://t.me/...) — universal fallback for users without the app
const webUrl = ref('')
const appUrl = ref('')
const authToken = ref('')

const expired = ref(false)
const tokenReady = computed(() => !!authToken.value && !expired.value)
const remainingSeconds = ref(300)
const showTroubleshootingHint = ref(false)
const copied = ref(false)

// Telegram Web fallback: users already logged into web.telegram.org cannot rely
// on t.me auto-executing /start, so they paste this command into the bot chat.
const manualCommand = computed(() => (authToken.value ? `/start ${authToken.value}` : ''))

let pollInterval: ReturnType<typeof setInterval> | null = null
let countdownInterval: ReturnType<typeof setInterval> | null = null
let troubleshootingTimeout: ReturnType<typeof setTimeout> | null = null
let pollCount = 0
const MAX_POLLS = 150

function parseDeepLink(url: string): { botName: string; token: string } | null {
  try {
    const u = new URL(url)
    const botName = u.pathname.replace(/^\//, '')
    const token = u.searchParams.get('start') || ''
    if (!botName || !token) return null
    return { botName, token }
  } catch {
    return null
  }
}

async function startAuth() {
  expired.value = false
  showTroubleshootingHint.value = false
  pollCount = 0
  authToken.value = ''
  webUrl.value = ''
  appUrl.value = ''
  cleanup()

  const result = await authStore.requestDeepLink()
  if (!result) {
    expired.value = true
    return
  }

  const parsed = parseDeepLink(result.deeplink_url)
  if (!parsed) {
    expired.value = true
    return
  }

  authToken.value = result.token
  webUrl.value = result.deeplink_url
  appUrl.value = `tg://resolve?domain=${parsed.botName}&start=${parsed.token}`
  remainingSeconds.value = result.expires_in

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
      expired.value = true
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
    expired.value = true
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
    expired.value = true
    cleanup()
  }
}

async function copyManual() {
  if (!manualCommand.value) return
  try {
    await navigator.clipboard.writeText(manualCommand.value)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    // Clipboard API unavailable (insecure context); user can still long-press to copy.
  }
}

function trackClicked() {
  // Placeholder for future analytics (tg:// click-through rate vs https://t.me/ fallback).
}

const passkeySupported = typeof window !== 'undefined' && !!window.PublicKeyCredential

async function loginWithPasskey() {
  const ok = await authStore.passkeyLogin()
  if (ok) {
    cleanup()
    const returnUrl = sessionStorage.getItem('returnUrl')
    sessionStorage.removeItem('returnUrl')
    router.push(returnUrl || '/')
  }
}

function cleanup() {
  if (pollInterval) { clearInterval(pollInterval); pollInterval = null }
  if (countdownInterval) { clearInterval(countdownInterval); countdownInterval = null }
  if (troubleshootingTimeout) { clearTimeout(troubleshootingTimeout); troubleshootingTimeout = null }
}

/** Consumes the pending post-login redirect target (defaulting to home) and navigates there in place. */
function replaceWithReturnUrl() {
  const returnUrl = sessionStorage.getItem('returnUrl')
  sessionStorage.removeItem('returnUrl')
  router.replace(returnUrl || '/')
}

onMounted(async () => {
  // UA-027: if user is already authed, bounce them out rather than show a fresh QR.
  if (authStore.isAuthenticated) {
    replaceWithReturnUrl()
    return
  }

  // TLS-cert auto-login (spec 2026-07-24): probe WITHOUT blocking startAuth —
  // the cert-picker dialog can stay open for many seconds (probe timeout is
  // sized for that), and the QR/password login must render immediately
  // underneath it. On success we bounce out to returnUrl mid-page.
  void import('@/composables/useCertAutoLogin').then(async ({ tryCertAutoLogin }) => {
    if (!(await tryCertAutoLogin())) return
    // The picker can sit open for a while — if the user logged in manually
    // (or navigated away) meanwhile, don't yank them anywhere.
    if (router.currentRoute.value.name !== 'auth') return
    replaceWithReturnUrl()
  })

  startAuth()
})

onUnmounted(() => {
  cleanup()
})
</script>
