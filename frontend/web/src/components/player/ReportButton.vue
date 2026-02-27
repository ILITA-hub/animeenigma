<template>
  <div v-if="authStore.isAuthenticated" class="mt-3">
    <button
      class="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg transition-colors"
      :class="buttonClasses"
      @click="showModal = true"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
      </svg>
      {{ $t('player.reportNotWorking') || 'Сообщить о проблеме' }}
    </button>

    <Modal v-model="showModal" :title="$t('player.reportTitle') || 'Сообщить о проблеме'" size="lg">
      <div v-if="submitted" class="text-center py-4">
        <svg class="w-12 h-12 mx-auto mb-3 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <p class="text-white text-lg font-medium">{{ $t('player.reportSent') || 'Отчёт отправлен' }}</p>
        <p class="text-white/60 mt-1 text-sm">{{ $t('player.reportThankYou') || 'Спасибо! Мы изучим проблему.' }}</p>
      </div>

      <template v-else>
        <!-- Auto-collected context -->
        <div class="space-y-2 mb-4 p-3 rounded-lg bg-white/5 text-sm text-white/60">
          <div class="flex gap-2">
            <span class="text-white/40 w-20 flex-shrink-0">{{ $t('player.reportPlayer') || 'Плеер' }}:</span>
            <span>{{ playerType }}</span>
          </div>
          <div class="flex gap-2">
            <span class="text-white/40 w-20 flex-shrink-0">{{ $t('player.reportAnime') || 'Аниме' }}:</span>
            <span class="truncate">{{ animeName || animeId }}</span>
          </div>
          <div v-if="episodeNumber" class="flex gap-2">
            <span class="text-white/40 w-20 flex-shrink-0">{{ $t('player.reportEpisode') || 'Серия' }}:</span>
            <span>{{ episodeNumber }}</span>
          </div>
          <div v-if="serverName" class="flex gap-2">
            <span class="text-white/40 w-20 flex-shrink-0">{{ $t('player.reportServer') || 'Сервер' }}:</span>
            <span>{{ serverName }}</span>
          </div>
          <div v-if="errorMessage" class="flex gap-2">
            <span class="text-white/40 w-20 flex-shrink-0">{{ $t('player.reportError') || 'Ошибка' }}:</span>
            <span class="text-pink-400 truncate">{{ errorMessage }}</span>
          </div>
        </div>

        <!-- User description -->
        <textarea
          v-model="description"
          class="w-full h-24 bg-white/5 border border-white/10 rounded-lg p-3 text-white text-sm placeholder-white/30 focus:outline-none focus:border-white/20 resize-none"
          :placeholder="$t('player.reportDescriptionPlaceholder') || 'Опишите проблему (необязательно)...'"
        />

        <p class="mt-2 text-xs text-white/30">
          {{ $t('player.reportDisclaimer') || 'Будут отправлены данные браузера и логи для диагностики.' }}
        </p>

        <div v-if="submitError" class="mt-2 text-sm text-pink-400">
          {{ submitError }}
        </div>
      </template>

      <template #footer>
        <button
          v-if="!submitted"
          class="px-4 py-2 text-sm rounded-lg bg-white/10 text-white/60 hover:bg-white/15 transition-colors"
          @click="showModal = false"
        >
          {{ $t('common.cancel') || 'Отмена' }}
        </button>
        <button
          v-if="!submitted"
          class="px-4 py-2 text-sm rounded-lg font-medium transition-colors disabled:opacity-50"
          :class="submitButtonClasses"
          :disabled="submitting"
          @click="submitReport"
        >
          <span v-if="submitting" class="flex items-center gap-2">
            <span class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />
            {{ $t('player.reportSending') || 'Отправка...' }}
          </span>
          <span v-else>{{ $t('player.reportSubmit') || 'Отправить' }}</span>
        </button>
        <button
          v-if="submitted"
          class="px-4 py-2 text-sm rounded-lg bg-white/10 text-white/60 hover:bg-white/15 transition-colors"
          @click="showModal = false; submitted = false"
        >
          {{ $t('common.close') || 'Закрыть' }}
        </button>
      </template>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { userApi } from '@/api/client'
import { collectDiagnostics } from '@/utils/diagnostics'
import Modal from '@/components/ui/Modal.vue'

interface Props {
  playerType: string
  animeId: string
  animeName: string
  episodeNumber?: number | null
  serverName?: string | null
  streamUrl?: string | null
  errorMessage?: string | null
  accentColor?: string
}

const props = withDefaults(defineProps<Props>(), {
  accentColor: '#a855f7',
})

const authStore = useAuthStore()
const showModal = ref(false)
const description = ref('')
const submitting = ref(false)
const submitted = ref(false)
const submitError = ref<string | null>(null)

const buttonClasses = computed(() => {
  return 'bg-white/5 hover:bg-white/10 text-white/50 hover:text-white/70 border border-white/10'
})

const submitButtonClasses = computed(() => {
  return `bg-[${props.accentColor}]/20 text-[${props.accentColor}] hover:bg-[${props.accentColor}]/30`
})

async function submitReport() {
  submitting.value = true
  submitError.value = null

  try {
    const report = collectDiagnostics(
      {
        playerType: props.playerType,
        animeId: props.animeId,
        animeName: props.animeName,
        episodeNumber: props.episodeNumber,
        serverName: props.serverName,
        streamUrl: props.streamUrl,
        errorMessage: props.errorMessage,
      },
      description.value,
      authStore.user?.id ?? null,
      authStore.user?.username ?? null,
    )

    await userApi.reportError(report)
    submitted.value = true
    description.value = ''
  } catch (err: any) {
    submitError.value = err.response?.data?.error?.message || err.message || 'Ошибка отправки'
  } finally {
    submitting.value = false
  }
}
</script>
