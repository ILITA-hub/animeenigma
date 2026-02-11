<template>
  <div class="hianime-player">
    <!-- Loading state for episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 border-purple-400 border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- No episodes available -->
    <div v-else-if="episodes.length === 0 && !loadingEpisodes" class="text-center py-20 text-white/60">
      <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
      </svg>
      {{ $t('player.noEpisodes') || 'Серии не найдены на HiAnime' }}
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
              <div class="w-10 h-10 border-2 border-purple-400 border-t-transparent rounded-full animate-spin mx-auto mb-3" />
              <p class="text-white/60 text-sm">Загрузка серии {{ selectedEpisode?.number }}...</p>
            </div>
          </div>

          <!-- HLS Video Player -->
          <video
            v-if="streamUrl && streamType === 'hls'"
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
              :label="sub.label"
              :srclang="getLanguageCode(sub.lang)"
              :src="buildSubtitleProxyUrl(sub.url)"
              :default="sub.default || index === 0"
            />
          </video>

          <!-- Iframe fallback -->
          <iframe
            v-else-if="streamUrl && streamType === 'iframe'"
            :src="streamUrl"
            class="absolute inset-0 w-full h-full"
            frameborder="0"
            allowfullscreen
            allow="autoplay; fullscreen; encrypted-media"
          />

          <!-- Placeholder when no video loaded -->
          <div
            v-else-if="!loadingStream"
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
              <span class="hidden sm:inline">{{ episodeMarkedWatched ? 'Просмотрено' : 'Отметить просмотренным' }}</span>
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
                  ? 'bg-purple-500 text-white'
                  : isEpisodeWatched(ep.number)
                    ? 'bg-green-500/20 text-green-400 border border-green-500/30 hover:bg-green-500/30'
                    : ep.is_filler
                      ? 'bg-amber-500/20 text-amber-400 border border-amber-500/30 hover:bg-amber-500/30'
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
        <!-- Category tabs (Sub/Dub) -->
        <div class="flex gap-2 mb-3">
          <button
            @click="selectedCategory = 'sub'"
            class="flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-all"
            :class="selectedCategory === 'sub'
              ? 'bg-purple-500/20 text-purple-400 border border-purple-500/50'
              : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
            </svg>
            Sub
            <span class="text-xs opacity-70">({{ subServers.length }})</span>
          </button>
          <button
            @click="selectedCategory = 'dub'"
            class="flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-all"
            :class="selectedCategory === 'dub'
              ? 'bg-blue-500/20 text-blue-400 border border-blue-500/50'
              : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
            </svg>
            Dub
            <span class="text-xs opacity-70">({{ dubServers.length }})</span>
          </button>
        </div>

        <!-- Loading servers -->
        <div v-if="loadingServers" class="flex items-center justify-center py-8">
          <div class="w-8 h-8 border-2 border-purple-400 border-t-transparent rounded-full animate-spin" />
        </div>

        <!-- Server list -->
        <div v-else class="space-y-2 max-h-[350px] lg:max-h-[450px] overflow-y-auto custom-scrollbar pr-1">
          <template v-if="filteredServers.length > 0">
            <button
              v-for="server in filteredServers"
              :key="server.id"
              @click="selectServer(server)"
              class="w-full text-left p-3 rounded-lg transition-all"
              :class="selectedServer?.id === server.id
                ? (selectedCategory === 'sub' ? 'bg-purple-500/20 border border-purple-500/50' : 'bg-blue-500/20 border border-blue-500/50')
                : 'bg-white/5 border border-transparent hover:bg-white/10'"
            >
              <div class="flex items-center justify-between gap-2">
                <div class="flex-1 min-w-0">
                  <p class="text-white font-medium truncate">{{ server.name }}</p>
                </div>
                <div
                  v-if="selectedServer?.id === server.id"
                  class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                  :class="selectedCategory === 'sub' ? 'bg-purple-500' : 'bg-blue-500'"
                >
                  <svg class="w-4 h-4 text-black" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                  </svg>
                </div>
              </div>
            </button>
          </template>
          <div v-else class="text-center py-8 text-white/40">
            <p>{{ selectedCategory === 'sub' ? 'Нет доступных субтитров' : 'Нет доступного дубляжа' }}</p>
          </div>
        </div>

        <!-- Subtitles info -->
        <div v-if="subtitles.length > 0" class="mt-4 p-3 rounded-lg bg-white/5">
          <h4 class="text-white/60 text-sm mb-2">Субтитры</h4>
          <div class="flex flex-wrap gap-2">
            <span
              v-for="sub in subtitles"
              :key="sub.url"
              class="px-2 py-1 text-xs rounded bg-white/10 text-white/70"
            >
              {{ sub.label }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- Server load warning -->
    <div v-if="serverLoadWarning" class="mt-4 p-4 bg-amber-500/20 border border-amber-500/30 rounded-lg text-amber-400">
      <div class="flex items-center gap-2">
        <svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
        </svg>
        {{ serverLoadWarning }}
      </div>
    </div>

    <!-- Error message -->
    <div v-if="error" class="mt-4 p-4 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400">
      {{ error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import Hls from 'hls.js'
import { hiAnimeApi, userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

interface HiAnimeEpisode {
  id: string
  number: number
  title: string
  is_filler: boolean
}

interface HiAnimeServer {
  id: string
  name: string
  type: string // sub, dub, raw
}

interface HiAnimeSubtitle {
  url: string
  lang: string
  label: string
  default: boolean
}

interface HiAnimeStream {
  url: string
  type: string // hls, mp4, iframe
  subtitles?: HiAnimeSubtitle[]
  headers?: Record<string, string>
  intro?: { start: number; end: number }
  outro?: { start: number; end: number }
}

interface ProxyStatus {
  active_connections: number
  max_connections: number
  load_percent: number
  available: boolean
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
const episodes = ref<HiAnimeEpisode[]>([])
const servers = ref<HiAnimeServer[]>([])
const selectedEpisode = ref<HiAnimeEpisode | null>(null)
const selectedServer = ref<HiAnimeServer | null>(null)
const selectedCategory = ref<'sub' | 'dub'>('sub')
const streamUrl = ref<string | null>(null)
const streamType = ref<'hls' | 'mp4' | 'iframe'>('hls')
const subtitles = ref<HiAnimeSubtitle[]>([])

const loadingEpisodes = ref(false)
const loadingServers = ref(false)
const loadingStream = ref(false)
const error = ref<string | null>(null)
const serverLoadWarning = ref<string | null>(null)

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

// Computed
const subServers = computed(() => servers.value.filter(s => s.type === 'sub'))
const dubServers = computed(() => servers.value.filter(s => s.type === 'dub'))
const filteredServers = computed(() =>
  selectedCategory.value === 'sub' ? subServers.value : dubServers.value
)

// Methods
const fetchEpisodes = async () => {
  loadingEpisodes.value = true
  error.value = null

  try {
    const response = await hiAnimeApi.getEpisodes(props.animeId)
    const data = response.data?.data || response.data || []
    episodes.value = Array.isArray(data) ? data : []

    // Mark episodes as loaded BEFORE selecting first episode
    // This allows the video player container to render so videoRef is available
    loadingEpisodes.value = false

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

const fetchServers = async (episodeId: string) => {
  loadingServers.value = true

  try {
    const response = await hiAnimeApi.getServers(props.animeId, episodeId)
    const data = response.data?.data || response.data || []
    servers.value = Array.isArray(data) ? data : []

    // Auto-select first server of preferred category
    const preferredServers = selectedCategory.value === 'sub' ? subServers.value : dubServers.value
    if (preferredServers.length > 0) {
      await selectServer(preferredServers[0])
    } else if (servers.value.length > 0) {
      // Fall back to any available server
      selectedCategory.value = servers.value[0].type as 'sub' | 'dub'
      await selectServer(servers.value[0])
    }
  } catch (err: any) {
    error.value = err.response?.data?.message || 'Не удалось загрузить серверы'
    servers.value = []
  } finally {
    loadingServers.value = false
  }
}

const checkServerLoad = async (): Promise<boolean> => {
  try {
    const response = await fetch('/api/streaming/proxy-status')
    if (!response.ok) return true // Assume available if status check fails

    const status: ProxyStatus = await response.json()
    if (status.load_percent > 80) {
      serverLoadWarning.value = 'Сервер загружен. Если видео тормозит, попробуйте Kodik.'
    } else {
      serverLoadWarning.value = null
    }
    return status.available
  } catch {
    return true // Assume available if status check fails
  }
}

const fetchStream = async () => {
  if (!selectedEpisode.value || !selectedServer.value) return

  loadingStream.value = true
  error.value = null
  serverLoadWarning.value = null

  try {
    // Check server load before fetching stream
    await checkServerLoad()

    // Send server NAME (not ID) - Aniwatch API expects server name like "hd-1"
    const serverName = selectedServer.value.name.toLowerCase()
    const response = await hiAnimeApi.getStream(
      props.animeId,
      selectedEpisode.value.id,
      serverName,
      selectedCategory.value
    )
    const data: HiAnimeStream = response.data?.data || response.data

    streamUrl.value = data.url
    streamType.value = data.type as 'hls' | 'mp4' | 'iframe'
    subtitles.value = data.subtitles || []

    // Initialize HLS player if needed
    if (data.type === 'hls' && data.url) {
      // Wait for Vue to render the video element
      await nextTick()

      // Additional wait if needed (should be fast now that loadingEpisodes is false)
      let retries = 0
      while (!videoRef.value && retries < 5) {
        await new Promise(resolve => setTimeout(resolve, 50))
        retries++
      }

      if (!videoRef.value) {
        console.error('[HiAnime] Video element not found')
        error.value = 'Ошибка инициализации плеера'
        return
      }

      // Get referer from headers returned by API
      const headers = data.headers || {}
      const referer = headers['Referer'] || headers['referer'] || ''
      initHlsPlayer(data.url, referer)
    }
  } catch (err: any) {
    // Show detailed error from backend
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
  // Route HLS streams through our backend proxy which can set the Referer header
  const params = new URLSearchParams()
  params.set('url', url)
  if (referer) {
    params.set('referer', referer)
  }
  return `/api/streaming/hls-proxy?${params.toString()}`
}

const buildSubtitleProxyUrl = (url: string): string => {
  // Route subtitles through proxy to handle CORS
  // Subtitles typically don't need a referer but may have CORS restrictions
  const params = new URLSearchParams()
  params.set('url', url)
  return `/api/streaming/hls-proxy?${params.toString()}`
}

const getLanguageCode = (lang: string): string => {
  // Convert full language names to ISO 639-1 codes
  const langMap: Record<string, string> = {
    'english': 'en',
    'japanese': 'ja',
    'russian': 'ru',
    'chinese': 'zh',
    'korean': 'ko',
    'spanish': 'es',
    'french': 'fr',
    'german': 'de',
    'italian': 'it',
    'portuguese': 'pt',
    'arabic': 'ar',
    'thai': 'th',
    'vietnamese': 'vi',
    'indonesian': 'id',
    'malay': 'ms',
    'hindi': 'hi',
  }
  const lower = lang?.toLowerCase() || ''
  return langMap[lower] || lower.substring(0, 2) || 'en'
}

const enableDefaultSubtitles = async () => {
  if (!videoRef.value) return

  // Wait for text tracks to be loaded
  await new Promise(resolve => setTimeout(resolve, 500))

  const textTracks = videoRef.value.textTracks
  if (textTracks.length === 0) return

  // Find and enable the default subtitle track
  for (let i = 0; i < textTracks.length; i++) {
    const track = textTracks[i]
    if (track.kind === 'subtitles') {
      // Find the default one, or enable the first subtitle track
      const subInfo = subtitles.value[i]
      if (subInfo?.default || i === 0) {
        track.mode = 'showing'
      } else {
        track.mode = 'hidden'
      }
    }
  }
}

const initHlsPlayer = (url: string, referer: string = '') => {
  if (!videoRef.value) {
    return
  }

  // Destroy existing HLS instance
  if (hls) {
    hls.destroy()
    hls = null
  }

  // Build proxy URL for the initial manifest
  const proxyUrl = buildProxyUrl(url, referer)

  if (Hls.isSupported()) {
    try {
      hls = new Hls()

      // Intercept fragment loading to route through proxy (fallback if M3U8 rewriting missed something)
      hls.on(Hls.Events.FRAG_LOADING, (_event, data) => {
        if (!data.frag.url.startsWith('/api/streaming/hls-proxy')) {
          data.frag.url = buildProxyUrl(data.frag.url, referer)
        }
      })

      hls.loadSource(proxyUrl)
      hls.attachMedia(videoRef.value)

      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        videoRef.value?.play().catch(() => {})
        // Enable default subtitle track
        enableDefaultSubtitles()
      })

      hls.on(Hls.Events.ERROR, (_event, data) => {
        if (data.fatal) {
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          if ((data as any).response?.code === 503) {
            error.value = 'Сервер занят. Попробуйте позже или используйте Kodik.'
          } else {
            error.value = 'Ошибка воспроизведения видео'
          }
        }
      })
    } catch (e) {
      console.error('[HiAnime] Error creating HLS instance:', e)
      error.value = 'Ошибка инициализации плеера'
    }
  } else if (videoRef.value.canPlayType('application/vnd.apple.mpegurl')) {
    // Native HLS support (Safari) - still needs proxy
    videoRef.value.src = proxyUrl
    videoRef.value.play().catch(() => {})
  } else {
    error.value = 'Ваш браузер не поддерживает HLS'
  }
}

const selectEpisode = async (episode: HiAnimeEpisode) => {
  if (selectedEpisode.value?.id === episode.id) return

  // Save progress before switching
  saveProgress()

  selectedEpisode.value = episode
  episodeMarkedWatched.value = isEpisodeWatched(episode.number)
  currentTime.value = 0
  maxTime.value = 0
  lastSaveTime.value = 0

  // Reset stream
  streamUrl.value = null
  servers.value = []
  selectedServer.value = null

  // Fetch servers for this episode
  await fetchServers(episode.id)
}

const selectServer = async (server: HiAnimeServer) => {
  if (selectedServer.value?.id === server.id) return

  selectedServer.value = server
  await fetchStream()
}

// Progress tracking
const handleTimeUpdate = () => {
  if (!videoRef.value) return

  currentTime.value = videoRef.value.currentTime

  if (currentTime.value > maxTime.value) {
    maxTime.value = currentTime.value
  }

  // Emit progress
  emit('progress', {
    episode: selectedEpisode.value?.number || 0,
    time: currentTime.value,
    maxTime: maxTime.value
  })

  // Save progress every 30 seconds
  if (currentTime.value - lastSaveTime.value >= SAVE_INTERVAL) {
    lastSaveTime.value = currentTime.value
    saveProgress()
  }

  // Auto-mark as watched after 20 minutes
  if (authStore.isAuthenticated && !episodeMarkedWatched.value && currentTime.value >= AUTO_MARK_THRESHOLD) {
    autoMarkEpisodeWatched()
  }
}

const handlePause = () => {
  saveProgress()
}

const handleEnded = () => {
  saveProgress()
  if (!episodeMarkedWatched.value) {
    autoMarkEpisodeWatched()
  }
}

const saveProgress = () => {
  if (!selectedEpisode.value || currentTime.value <= 0) return

  // Save to localStorage
  const key = `watch_progress:${props.animeId}`
  const data = JSON.parse(localStorage.getItem(key) || '{}')
  data[selectedEpisode.value.number] = {
    time: currentTime.value,
    maxTime: maxTime.value,
    updatedAt: Date.now()
  }
  localStorage.setItem(key, JSON.stringify(data))

  // Save to server if authenticated
  if (authStore.isAuthenticated) {
    userApi.updateProgress({
      anime_id: props.animeId,
      episode_number: selectedEpisode.value.number,
      progress: Math.floor(currentTime.value),
      duration: Math.floor(maxTime.value) || null
    }).catch(() => {})
  }
}

// Watch tracking
const fetchWatchedEpisodes = async () => {
  if (!authStore.isAuthenticated) {
    watchedEpisodes.value = 0
    return
  }

  try {
    const response = await userApi.getWatchlistEntry(props.animeId)
    const entry = response.data?.data || response.data
    watchedEpisodes.value = entry?.episodes || 0
  } catch {
    watchedEpisodes.value = 0
  }
}

const isEpisodeWatched = (episodeNum: number): boolean => {
  return episodeNum <= watchedEpisodes.value
}

const markCurrentEpisodeWatched = async () => {
  if (!authStore.isAuthenticated || !selectedEpisode.value || markingWatched.value) return

  markingWatched.value = true
  try {
    await userApi.markEpisodeWatched(props.animeId, selectedEpisode.value.number)
    episodeMarkedWatched.value = true
    if (selectedEpisode.value.number > watchedEpisodes.value) {
      watchedEpisodes.value = selectedEpisode.value.number
    }
    emit('episodeWatched', { episode: selectedEpisode.value.number })
  } catch (err: any) {
    error.value = err.response?.data?.message || 'Не удалось отметить серию'
  } finally {
    markingWatched.value = false
  }
}

const autoMarkEpisodeWatched = async () => {
  if (!authStore.isAuthenticated || !selectedEpisode.value || episodeMarkedWatched.value) return

  try {
    await userApi.markEpisodeWatched(props.animeId, selectedEpisode.value.number)
    episodeMarkedWatched.value = true
    if (selectedEpisode.value.number > watchedEpisodes.value) {
      watchedEpisodes.value = selectedEpisode.value.number
    }
    emit('episodeWatched', { episode: selectedEpisode.value.number })
  } catch {
    // Silent fail
  }
}

// Watchers
watch(selectedCategory, () => {
  // When category changes, select first server of new category
  const newServers = selectedCategory.value === 'sub' ? subServers.value : dubServers.value
  if (newServers.length > 0 && selectedServer.value?.type !== selectedCategory.value) {
    selectServer(newServers[0])
  }
})

watch(() => props.animeId, () => {
  saveProgress()
  streamUrl.value = null
  episodes.value = []
  servers.value = []
  selectedEpisode.value = null
  selectedServer.value = null
  currentTime.value = 0
  maxTime.value = 0
  watchedEpisodes.value = 0
  episodeMarkedWatched.value = false
  fetchEpisodes()
  fetchWatchedEpisodes()
})

// Lifecycle
onMounted(() => {
  fetchEpisodes()
  fetchWatchedEpisodes()
})

onUnmounted(() => {
  saveProgress()
  if (hls) {
    hls.destroy()
    hls = null
  }
})
</script>

<style scoped>
.hianime-player {
  width: 100%;
}

.custom-scrollbar::-webkit-scrollbar {
  width: 4px;
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
  border-radius: 2px;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.3);
}
</style>
