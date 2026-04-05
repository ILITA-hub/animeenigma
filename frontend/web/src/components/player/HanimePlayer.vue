<template>
  <div class="hanime-player hanime-player-wrapper">
    <!-- Loading state for episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- No episodes available -->
    <div v-else-if="episodes.length === 0 && !loadingEpisodes" class="text-center py-20 text-white/60">
      <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
      </svg>
      Эпизоды не найдены на Hanime
    </div>

    <!-- Main content when episodes available -->
    <div v-else class="flex flex-col lg:flex-row gap-4">
      <!-- Left: Video Player -->
      <div class="flex-1 min-w-0">
        <div class="relative aspect-video bg-black rounded-xl overflow-hidden">
          <!-- Loading overlay -->
          <div
            v-if="loadingStream"
            class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
          >
            <div class="text-center">
              <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin mx-auto mb-3" />
              <p class="text-white/60 text-sm">Загрузка эпизода...</p>
            </div>
          </div>

          <!-- Error message -->
          <div
            v-if="error && !loadingStream"
            class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
          >
            <div class="text-center text-pink-400 px-4">
              <svg class="w-12 h-12 mx-auto mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
              <p>{{ error }}</p>
            </div>
          </div>

          <!-- Video player -->
          <video
            v-if="streamUrl && !error"
            ref="videoRef"
            :src="streamUrl"
            class="absolute inset-0 w-full h-full"
            controls
            playsinline
            autoplay
            @timeupdate="handleTimeUpdate"
            @pause="handlePause"
            @ended="handleEnded"
            @error="handleVideoError"
          />

          <!-- Placeholder when no video loaded -->
          <div
            v-else-if="!loadingStream && !error"
            class="absolute inset-0 flex items-center justify-center"
          >
            <div class="text-center text-white/40">
              <svg class="w-16 h-16 mx-auto mb-3" fill="currentColor" viewBox="0 0 24 24">
                <path d="M8 5v14l11-7z" />
              </svg>
              <p>Выберите эпизод</p>
            </div>
          </div>
        </div>

        <!-- Episode selector below player -->
        <div class="mt-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-white/60 text-sm flex items-center gap-2">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
              </svg>
              Эпизоды ({{ episodes.length }})
            </h3>
          </div>
          <div class="flex flex-wrap gap-2 max-h-32 overflow-y-auto custom-scrollbar p-1">
            <button
              v-for="(ep, idx) in episodes"
              :key="ep.slug"
              @click="selectEpisode(ep, idx)"
              class="relative px-3 h-10 rounded-lg text-sm font-medium transition-all"
              :class="selectedEpisode?.slug === ep.slug
                ? 'accent-bg text-white'
                : 'bg-white/10 text-white hover:bg-white/20'"
              :title="ep.name"
            >
              {{ idx + 1 }}
            </button>
          </div>
        </div>
      </div>

      <!-- Right: Settings panel -->
      <div class="lg:w-72 flex-shrink-0">
        <!-- Quality selector -->
        <div v-if="availableSources.length > 0" class="mt-0">
          <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
            </svg>
            Качество
          </h3>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="source in availableSources"
              :key="source.url"
              @click="selectQuality(source)"
              class="px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
              :class="selectedSource?.url === source.url
                ? 'accent-bg-muted accent-text border accent-border'
                : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
            >
              {{ source.height }}p
            </button>
          </div>
        </div>

        <!-- Episode info -->
        <div v-if="selectedEpisode" class="mt-4">
          <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            Эпизод
          </h3>
          <p class="text-white text-sm font-medium truncate">{{ selectedEpisode.name }}</p>
        </div>
      </div>
    </div>

    <!-- Report button -->
    <ReportButton
      player-type="hanime"
      :anime-id="animeId"
      :anime-name="animeName || animeId"
      :episode-number="selectedEpisodeIndex !== null ? selectedEpisodeIndex + 1 : undefined"
      :stream-url="streamUrl"
      :error-message="error"
      :accent-color="ACCENT_COLOR"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { hanimeApi, userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import ReportButton from './ReportButton.vue'

const ACCENT_COLOR = '#ec4899'

interface HanimeEpisode {
  name: string
  slug: string
}

interface HanimeSource {
  url: string
  height: string
  width: number
  size_mb: number
}

const props = defineProps<{
  animeId: string
  animeName?: string
  totalEpisodes?: number
  initialEpisode?: number
}>()

const authStore = useAuthStore()

// State
const episodes = ref<HanimeEpisode[]>([])
const selectedEpisode = ref<HanimeEpisode | null>(null)
const selectedEpisodeIndex = ref<number | null>(null)
const availableSources = ref<HanimeSource[]>([])
const selectedSource = ref<HanimeSource | null>(null)
const streamUrl = ref<string | null>(null)

const loadingEpisodes = ref(false)
const loadingStream = ref(false)
const error = ref<string | null>(null)

const videoRef = ref<HTMLVideoElement | null>(null)

// Progress tracking
const currentTime = ref(0)
const maxTime = ref(0)
const lastSaveTime = ref(0)
const SAVE_INTERVAL = 15

// Build proxy URL for MP4
const buildProxyUrl = (url: string): string => {
  const params = new URLSearchParams()
  params.set('url', url)
  params.set('referer', 'https://hanime.tv/')
  return `/api/streaming/hls-proxy?${params.toString()}`
}

const fetchEpisodes = async () => {
  loadingEpisodes.value = true
  error.value = null

  try {
    const response = await hanimeApi.getEpisodes(props.animeId)
    const data = response.data?.data || response.data || []
    episodes.value = Array.isArray(data) ? data : []
    loadingEpisodes.value = false

    if (episodes.value.length > 0) {
      const initialIdx = props.initialEpisode
        ? Math.min(props.initialEpisode - 1, episodes.value.length - 1)
        : 0
      await selectEpisode(episodes.value[initialIdx], initialIdx)
    }
  } catch (err: unknown) {
    const e = err as { response?: { data?: { message?: string } } }
    error.value = e.response?.data?.message || 'Не удалось загрузить эпизоды'
    episodes.value = []
    loadingEpisodes.value = false
  }
}

const selectEpisode = async (ep: HanimeEpisode, idx: number) => {
  selectedEpisode.value = ep
  selectedEpisodeIndex.value = idx
  streamUrl.value = null
  availableSources.value = []
  selectedSource.value = null
  error.value = null
  await fetchStream(ep)
}

const fetchStream = async (ep: HanimeEpisode) => {
  loadingStream.value = true
  error.value = null

  try {
    const response = await hanimeApi.getStream(props.animeId, ep.slug)
    const data = response.data?.data || response.data
    const sources: HanimeSource[] = data?.sources || []

    if (sources.length === 0) {
      error.value = 'Источники видео не найдены'
      return
    }

    // Sort by quality descending
    availableSources.value = [...sources].sort((a, b) => {
      return parseInt(b.height) - parseInt(a.height)
    })

    // Select best quality
    const best = availableSources.value[0]
    selectedSource.value = best
    streamUrl.value = buildProxyUrl(best.url)
  } catch (err: unknown) {
    const e = err as { response?: { data?: { message?: string; error?: { message?: string } } } }
    const message = e.response?.data?.error?.message
      || e.response?.data?.message
      || 'Не удалось загрузить видео'
    error.value = message
  } finally {
    loadingStream.value = false
  }
}

const selectQuality = (source: HanimeSource) => {
  if (source.url === selectedSource.value?.url) return
  selectedSource.value = source
  const newUrl = buildProxyUrl(source.url)
  if (videoRef.value) {
    const pos = videoRef.value.currentTime
    streamUrl.value = newUrl
    videoRef.value.src = newUrl
    videoRef.value.currentTime = pos
    videoRef.value.play().catch(() => {})
  } else {
    streamUrl.value = newUrl
  }
}

// Progress tracking
const saveProgress = () => {
  if (!selectedEpisode.value || currentTime.value <= 0) return

  const key = `watch_progress:${props.animeId}`
  const existing = JSON.parse(localStorage.getItem(key) || '{}')
  const epKey = selectedEpisodeIndex.value !== null ? String(selectedEpisodeIndex.value + 1) : selectedEpisode.value.slug
  existing[epKey] = {
    time: currentTime.value,
    maxTime: maxTime.value,
    updatedAt: Date.now()
  }
  localStorage.setItem(key, JSON.stringify(existing))
}

const handleTimeUpdate = () => {
  if (!videoRef.value) return
  currentTime.value = videoRef.value.currentTime
  maxTime.value = Math.max(maxTime.value, currentTime.value)

  if (currentTime.value - lastSaveTime.value >= SAVE_INTERVAL) {
    lastSaveTime.value = currentTime.value
    saveProgress()
  }
}

const handlePause = () => {
  saveProgress()
}

const handleEnded = () => {
  if (!selectedEpisode.value) return
  saveProgress()
  markEpisodeWatched()
  // Auto-advance to next episode
  if (selectedEpisodeIndex.value !== null && selectedEpisodeIndex.value < episodes.value.length - 1) {
    const nextIdx = selectedEpisodeIndex.value + 1
    selectEpisode(episodes.value[nextIdx], nextIdx)
  }
}

const handleVideoError = () => {
  const video = videoRef.value
  if (!video?.error) return
  const mediaError = video.error
  const codes: Record<number, string> = {
    1: 'MEDIA_ERR_ABORTED',
    2: 'MEDIA_ERR_NETWORK',
    3: 'MEDIA_ERR_DECODE',
    4: 'MEDIA_ERR_SRC_NOT_SUPPORTED',
  }
  const codeName = codes[mediaError.code] || `Unknown (${mediaError.code})`
  console.error('[Hanime] Video error:', codeName, mediaError.message, 'src:', streamUrl.value)
  error.value = `Video error: ${codeName}${mediaError.message ? ` — ${mediaError.message}` : ''}`
}

const markEpisodeWatched = async () => {
  if (!authStore.isAuthenticated || selectedEpisodeIndex.value === null) return
  const epNum = selectedEpisodeIndex.value + 1
  try {
    await userApi.markEpisodeWatched(props.animeId, epNum)
  } catch (err) {
    console.error('[Hanime] Failed to mark episode as watched:', err)
  }
}

// Reset when anime changes
watch(() => props.animeId, () => {
  saveProgress()
  streamUrl.value = null
  episodes.value = []
  selectedEpisode.value = null
  selectedEpisodeIndex.value = null
  availableSources.value = []
  selectedSource.value = null
  currentTime.value = 0
  maxTime.value = 0
  lastSaveTime.value = 0
  fetchEpisodes()
})

onMounted(async () => {
  await fetchEpisodes()
})
</script>

<style scoped>
.hanime-player-wrapper {
  --player-accent: #ec4899;
  --player-accent-rgb: 236, 72, 153;
}

.accent-bg { background-color: var(--player-accent); }
.accent-text { color: var(--player-accent); }
.accent-border { border-color: var(--player-accent); }
.accent-bg-muted { background-color: rgba(var(--player-accent-rgb), 0.2); }

.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: rgba(255, 255, 255, 0.05);
  border-radius: 3px;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
  border-radius: 3px;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.3);
}
</style>
