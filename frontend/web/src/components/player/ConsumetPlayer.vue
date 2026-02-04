<template>
  <div class="consumet-player">
    <!-- Loading state for episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 border-green-400 border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- No episodes available -->
    <div v-else-if="episodes.length === 0 && !loadingEpisodes" class="text-center py-20 text-white/60">
      <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
      </svg>
      {{ $t('player.noEpisodes') || 'Серии не найдены на Consumet' }}
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
              <div class="w-10 h-10 border-2 border-green-400 border-t-transparent rounded-full animate-spin mx-auto mb-3" />
              <p class="text-white/60 text-sm">Загрузка серии {{ selectedEpisode?.number }}...</p>
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

          <!-- HLS Video Player -->
          <video
            v-if="streamUrl && !error"
            ref="videoRef"
            class="absolute inset-0 w-full h-full"
            controls
            playsinline
            crossorigin="anonymous"
            @timeupdate="handleTimeUpdate"
            @pause="handlePause"
            @ended="handleEnded"
          >
            <!-- Subtitle tracks -->
            <track
              v-for="(sub, index) in subtitles"
              :key="sub.url"
              kind="subtitles"
              :label="sub.lang"
              :srclang="getLanguageCode(sub.lang)"
              :src="buildSubtitleProxyUrl(sub.url)"
              :default="index === 0"
            />
          </video>

          <!-- Placeholder when no video loaded -->
          <div
            v-else-if="!loadingStream && !error"
            class="absolute inset-0 flex items-center justify-center"
          >
            <div class="text-center text-white/40">
              <svg class="w-16 h-16 mx-auto mb-3" fill="currentColor" viewBox="0 0 24 24">
                <path d="M8 5v14l11-7z" />
              </svg>
              <p>Выберите серию для начала просмотра</p>
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
              Серии ({{ episodes.length }})
            </h3>
            <!-- Mark as watched button -->
            <button
              v-if="authStore.isAuthenticated"
              @click="markCurrentEpisodeWatched"
              :disabled="markingWatched"
              class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
              :class="episodeMarkedWatched
                ? 'bg-green-500/20 text-green-400 border border-green-500/50'
                : 'bg-white/10 text-white hover:bg-white/20'"
            >
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
              </svg>
              {{ episodeMarkedWatched ? 'Просмотрено' : 'Отметить просмотренным' }}
            </button>
          </div>
          <div class="flex flex-wrap gap-2 max-h-32 overflow-y-auto custom-scrollbar p-1">
            <button
              v-for="ep in episodes"
              :key="ep.id"
              @click="selectEpisode(ep)"
              class="relative w-12 h-10 rounded-lg text-sm font-medium transition-all"
              :class="[
                selectedEpisode?.id === ep.id
                  ? 'bg-green-500 text-white'
                  : isEpisodeWatched(ep.number)
                    ? 'bg-green-500/20 text-green-400 border border-green-500/30 hover:bg-green-500/30'
                    : 'bg-white/10 text-white hover:bg-white/20'
              ]"
              :title="ep.title || `Episode ${ep.number}`"
            >
              {{ ep.number }}
              <!-- Watched indicator -->
              <span
                v-if="isEpisodeWatched(ep.number) && selectedEpisode?.id !== ep.id"
                class="absolute -top-1 -right-1 w-3 h-3 bg-green-500 rounded-full flex items-center justify-center"
              >
                <svg class="w-2 h-2 text-black" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                </svg>
              </span>
            </button>
          </div>
        </div>
      </div>

      <!-- Right: Server selector -->
      <div class="lg:w-72 flex-shrink-0">
        <h3 class="text-white/60 text-sm mb-3 flex items-center gap-2">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
          </svg>
          Серверы
        </h3>

        <!-- Server list -->
        <div class="space-y-2">
          <button
            v-for="server in servers"
            :key="server.name"
            @click="selectServer(server)"
            class="w-full text-left p-3 rounded-lg transition-all"
            :class="selectedServer?.name === server.name
              ? 'bg-green-500/20 border border-green-500/50'
              : 'bg-white/5 border border-transparent hover:bg-white/10'"
          >
            <div class="flex items-center justify-between gap-2">
              <p class="text-white font-medium">{{ server.name }}</p>
              <div
                v-if="selectedServer?.name === server.name"
                class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0 bg-green-500"
              >
                <svg class="w-4 h-4 text-black" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                </svg>
              </div>
            </div>
          </button>
        </div>

        <!-- Info -->
        <div class="mt-4 p-3 bg-green-500/10 rounded-lg border border-green-500/20">
          <p class="text-green-400 text-sm">
            Consumet использует несколько источников для лучшего качества и стабильности.
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import Hls from 'hls.js'
import { consumetApi, userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

interface ConsumetEpisode {
  id: string
  number: number
  title: string
  is_filler: boolean
}

interface ConsumetServer {
  name: string
}

interface ConsumetSubtitle {
  url: string
  lang: string
}

interface ConsumetStream {
  url: string
  isM3U8: boolean
  quality: string
  headers?: Record<string, string>
  subtitles?: ConsumetSubtitle[]
}

const props = defineProps<{
  animeId: string
  totalEpisodes?: number
  initialEpisode?: number
}>()

const emit = defineEmits<{
  (e: 'progress', data: { episode: number; time: number; maxTime: number }): void
  (e: 'episodeWatched', data: { episode: number }): void
}>()

const authStore = useAuthStore()

// State
const episodes = ref<ConsumetEpisode[]>([])
const servers = ref<ConsumetServer[]>([])
const selectedEpisode = ref<ConsumetEpisode | null>(null)
const selectedServer = ref<ConsumetServer | null>(null)
const streamUrl = ref<string | null>(null)
const subtitles = ref<ConsumetSubtitle[]>([])

const loadingEpisodes = ref(false)
const loadingStream = ref(false)
const error = ref<string | null>(null)

const videoRef = ref<HTMLVideoElement | null>(null)
let hls: Hls | null = null

// Progress tracking
const currentTime = ref(0)
const maxTime = ref(0)
const lastSaveTime = ref(0)
const SAVE_INTERVAL = 30
const AUTO_MARK_THRESHOLD = 20 * 60 // 20 minutes

// Watch tracking
const markingWatched = ref(false)
const episodeMarkedWatched = ref(false)
const watchedEpisodes = ref(0)

// Methods
const fetchEpisodes = async () => {
  loadingEpisodes.value = true
  error.value = null

  try {
    const response = await consumetApi.getEpisodes(props.animeId)
    const data = response.data?.data || response.data || []
    episodes.value = Array.isArray(data) ? data : []

    loadingEpisodes.value = false

    // Fetch servers
    await fetchServers()

    // Auto-select first episode
    if (episodes.value.length > 0) {
      const initialEp = props.initialEpisode
        ? episodes.value.find(e => e.number === props.initialEpisode) || episodes.value[0]
        : episodes.value[0]
      await selectEpisode(initialEp)
    }
  } catch (err: any) {
    error.value = err.response?.data?.message || 'Не удалось загрузить список серий'
    episodes.value = []
    loadingEpisodes.value = false
  }
}

const fetchServers = async () => {
  try {
    const response = await consumetApi.getServers(props.animeId)
    const data = response.data?.data || response.data || []
    servers.value = Array.isArray(data) ? data : []

    // Auto-select first server
    if (servers.value.length > 0) {
      selectedServer.value = servers.value[0]
    }
  } catch (err: any) {
    console.error('Failed to fetch servers:', err)
    // Use default servers if API fails
    servers.value = [
      { name: 'vidcloud' },
      { name: 'streamsb' },
      { name: 'vidstreaming' }
    ]
    selectedServer.value = servers.value[0]
  }
}

const selectEpisode = async (ep: ConsumetEpisode) => {
  selectedEpisode.value = ep
  episodeMarkedWatched.value = false
  await fetchStream()
}

const selectServer = async (server: ConsumetServer) => {
  selectedServer.value = server
  if (selectedEpisode.value) {
    await fetchStream()
  }
}

const fetchStream = async () => {
  if (!selectedEpisode.value) return

  loadingStream.value = true
  error.value = null

  try {
    const response = await consumetApi.getStream(
      props.animeId,
      selectedEpisode.value.id,
      selectedServer.value?.name
    )
    const data: ConsumetStream = response.data?.data || response.data

    if (!data.url) {
      error.value = 'Не удалось получить ссылку на видео'
      return
    }

    streamUrl.value = data.url
    subtitles.value = data.subtitles || []

    // Initialize HLS player
    await nextTick()

    let retries = 0
    while (!videoRef.value && retries < 5) {
      await new Promise(resolve => setTimeout(resolve, 50))
      retries++
    }

    if (!videoRef.value) {
      console.error('[Consumet] Video element not found')
      error.value = 'Ошибка инициализации плеера'
      return
    }

    const headers = data.headers || {}
    const referer = headers['Referer'] || headers['referer'] || ''
    initHlsPlayer(data.url, referer)
  } catch (err: any) {
    const message = err.response?.data?.error?.message
      || err.response?.data?.message
      || 'Не удалось загрузить видео'
    error.value = message
    streamUrl.value = null
  } finally {
    loadingStream.value = false
  }
}

const buildProxyUrl = (url: string, referer: string): string => {
  const params = new URLSearchParams()
  params.set('url', url)
  if (referer) {
    params.set('referer', referer)
  }
  return `/api/streaming/hls-proxy?${params.toString()}`
}

const buildSubtitleProxyUrl = (url: string): string => {
  const params = new URLSearchParams()
  params.set('url', url)
  return `/api/streaming/hls-proxy?${params.toString()}`
}

const getLanguageCode = (lang: string): string => {
  const langMap: Record<string, string> = {
    'english': 'en',
    'japanese': 'ja',
    'russian': 'ru',
    'spanish': 'es',
    'french': 'fr',
    'german': 'de',
    'italian': 'it',
    'portuguese': 'pt',
    'arabic': 'ar',
  }
  return langMap[lang.toLowerCase()] || lang.substring(0, 2).toLowerCase()
}

const initHlsPlayer = (url: string, referer: string) => {
  if (hls) {
    hls.destroy()
    hls = null
  }

  const video = videoRef.value
  if (!video) return

  const proxyUrl = buildProxyUrl(url, referer)

  if (Hls.isSupported()) {
    hls = new Hls({
      xhrSetup: (xhr) => {
        xhr.withCredentials = false
      },
      enableWorker: true,
      lowLatencyMode: false,
      backBufferLength: 90,
    })

    hls.loadSource(proxyUrl)
    hls.attachMedia(video)

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      video.play().catch(() => {})
    })

    hls.on(Hls.Events.ERROR, (_event, data) => {
      console.error('[Consumet HLS Error]', data)
      if (data.fatal) {
        switch (data.type) {
          case Hls.ErrorTypes.NETWORK_ERROR:
            hls?.startLoad()
            break
          case Hls.ErrorTypes.MEDIA_ERROR:
            hls?.recoverMediaError()
            break
          default:
            error.value = 'Ошибка воспроизведения видео'
            break
        }
      }
    })
  } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
    video.src = proxyUrl
    video.addEventListener('loadedmetadata', () => {
      video.play().catch(() => {})
    })
  }
}

// Progress tracking
const handleTimeUpdate = () => {
  if (!videoRef.value || !selectedEpisode.value) return

  currentTime.value = videoRef.value.currentTime
  maxTime.value = Math.max(maxTime.value, currentTime.value)

  // Save progress periodically
  if (currentTime.value - lastSaveTime.value >= SAVE_INTERVAL) {
    lastSaveTime.value = currentTime.value
    emit('progress', {
      episode: selectedEpisode.value.number,
      time: currentTime.value,
      maxTime: maxTime.value
    })
  }

  // Auto-mark as watched
  if (maxTime.value >= AUTO_MARK_THRESHOLD && !episodeMarkedWatched.value) {
    markCurrentEpisodeWatched()
  }
}

const handlePause = () => {
  if (!selectedEpisode.value) return
  emit('progress', {
    episode: selectedEpisode.value.number,
    time: currentTime.value,
    maxTime: maxTime.value
  })
}

const handleEnded = () => {
  if (!selectedEpisode.value) return
  markCurrentEpisodeWatched()
  emit('progress', {
    episode: selectedEpisode.value.number,
    time: currentTime.value,
    maxTime: maxTime.value
  })
}

const markCurrentEpisodeWatched = async () => {
  if (!selectedEpisode.value || markingWatched.value || !authStore.isAuthenticated) return

  markingWatched.value = true
  try {
    await userApi.markEpisodeWatched(props.animeId, selectedEpisode.value.number)
    episodeMarkedWatched.value = true
    watchedEpisodes.value = Math.max(watchedEpisodes.value, selectedEpisode.value.number)
    emit('episodeWatched', { episode: selectedEpisode.value.number })
  } catch (err) {
    console.error('Failed to mark episode as watched:', err)
  } finally {
    markingWatched.value = false
  }
}

const isEpisodeWatched = (episodeNumber: number): boolean => {
  return episodeNumber <= watchedEpisodes.value
}

// Lifecycle
onMounted(async () => {
  await fetchEpisodes()

  // Fetch watched episodes
  if (authStore.isAuthenticated) {
    try {
      const response = await userApi.getWatchlistEntry(props.animeId)
      const entry = response.data?.data || response.data
      if (entry?.episodes_watched) {
        watchedEpisodes.value = entry.episodes_watched
      }
    } catch {
      // Ignore - user might not have this in watchlist
    }
  }
})

onUnmounted(() => {
  if (hls) {
    hls.destroy()
    hls = null
  }
})
</script>

<style scoped>
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
