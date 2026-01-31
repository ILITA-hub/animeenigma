<template>
  <div class="kodik-player">
    <!-- Controls Row -->
    <div v-if="translations.length > 0" class="mb-4 flex flex-wrap gap-4">
      <div class="flex-1 min-w-48">
        <label class="block text-white/60 text-sm mb-2">{{ $t('player.translation') || 'Озвучка' }}</label>
        <select
          v-model="selectedTranslation"
          class="w-full bg-white/10 border border-white/10 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-cyan-500 transition-colors"
          :disabled="loadingVideo"
        >
          <option v-for="t in translations" :key="t.id" :value="t.id">
            {{ t.title }} ({{ t.type === 'voice' ? 'Озвучка' : 'Субтитры' }})
          </option>
        </select>
      </div>
      <div class="w-32">
        <label class="block text-white/60 text-sm mb-2">{{ $t('anime.episode') || 'Серия' }}</label>
        <select
          v-model="selectedEpisode"
          class="w-full bg-white/10 border border-white/10 rounded-lg px-4 py-2.5 text-white focus:outline-none focus:border-cyan-500 transition-colors"
          :disabled="loadingVideo"
        >
          <option v-for="ep in episodeRange" :key="ep" :value="ep">
            {{ ep }}
          </option>
        </select>
      </div>
    </div>

    <!-- Loading state for translations -->
    <div v-if="loadingTranslations" class="flex items-center justify-center py-12">
      <div class="w-10 h-10 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- No translations available -->
    <div v-else-if="translations.length === 0 && !loadingTranslations" class="text-center py-12 text-white/60">
      <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
      </svg>
      {{ $t('player.noTranslations') || 'Нет доступных озвучек' }}
    </div>

    <!-- Video Player (iframe) with loading overlay -->
    <div v-if="translations.length > 0" class="relative aspect-video bg-black rounded-xl overflow-hidden">
      <!-- Loading overlay -->
      <div
        v-if="loadingVideo"
        class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
      >
        <div class="text-center">
          <div class="w-10 h-10 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin mx-auto mb-3" />
          <p class="text-white/60 text-sm">Загрузка серии {{ selectedEpisode }}...</p>
        </div>
      </div>

      <!-- Iframe player -->
      <iframe
        v-if="embedUrl"
        ref="playerFrame"
        :src="embedUrl"
        class="absolute inset-0 w-full h-full"
        frameborder="0"
        allowfullscreen
        allow="autoplay; fullscreen; encrypted-media"
      />

      <!-- Placeholder when no video loaded yet -->
      <div
        v-else-if="!loadingVideo"
        class="absolute inset-0 flex items-center justify-center"
      >
        <div class="text-center text-white/40">
          <svg class="w-16 h-16 mx-auto mb-3" fill="currentColor" viewBox="0 0 24 24">
            <path d="M8 5v14l11-7z" />
          </svg>
          <p>Выберите озвучку для начала просмотра</p>
        </div>
      </div>
    </div>

    <!-- Error message -->
    <div v-if="error" class="mt-4 p-4 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400">
      {{ error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { kodikApi } from '@/api/client'

interface KodikTranslation {
  id: number
  title: string
  type: string
}

const props = defineProps<{
  animeId: string
  totalEpisodes?: number
  initialEpisode?: number
}>()

const translations = ref<KodikTranslation[]>([])
const selectedTranslation = ref<number | null>(null)
const selectedEpisode = ref(1)
const embedUrl = ref<string | null>(null)
const loadingTranslations = ref(false)
const loadingVideo = ref(false)
const error = ref<string | null>(null)
const isInitialized = ref(false)

const episodeRange = computed(() => {
  const count = props.totalEpisodes || 12
  return Array.from({ length: count }, (_, i) => i + 1)
})

const fetchTranslations = async () => {
  loadingTranslations.value = true
  error.value = null
  isInitialized.value = false

  try {
    const response = await kodikApi.getTranslations(props.animeId)
    const data = response.data?.data || response.data
    translations.value = Array.isArray(data) ? data : []

    if (translations.value.length > 0) {
      selectedTranslation.value = translations.value[0].id
      // Auto-load first video after setting translation
      isInitialized.value = true
      await loadVideo()
    }
  } catch (err: any) {
    error.value = err.response?.data?.message || 'Не удалось загрузить список озвучек'
    translations.value = []
  } finally {
    loadingTranslations.value = false
  }
}

const loadVideo = async () => {
  if (!selectedTranslation.value) return

  loadingVideo.value = true
  error.value = null

  try {
    const response = await kodikApi.getVideo(props.animeId, selectedEpisode.value, selectedTranslation.value)
    const data = response.data?.data || response.data
    if (data?.embed_link) {
      embedUrl.value = data.embed_link
    } else {
      error.value = 'Видео не найдено'
    }
  } catch (err: any) {
    error.value = err.response?.data?.message || 'Не удалось загрузить видео'
  } finally {
    loadingVideo.value = false
  }
}

// Auto-load video when translation changes
watch(selectedTranslation, (newVal, oldVal) => {
  if (isInitialized.value && newVal && newVal !== oldVal) {
    loadVideo()
  }
})

// Auto-load video when episode changes
watch(selectedEpisode, (newVal, oldVal) => {
  if (isInitialized.value && selectedTranslation.value && newVal !== oldVal) {
    loadVideo()
  }
})

// Reset when anime changes
watch(() => props.animeId, () => {
  embedUrl.value = null
  selectedEpisode.value = 1
  fetchTranslations()
})

onMounted(() => {
  if (props.initialEpisode) {
    selectedEpisode.value = props.initialEpisode
  }
  fetchTranslations()
})
</script>

<style scoped>
.kodik-player {
  width: 100%;
}

select option {
  background-color: #1a1a2e;
  color: white;
}
</style>
