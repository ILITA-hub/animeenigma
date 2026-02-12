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

          <!-- Video.js Player -->
          <div v-if="streamUrl && !error && playerType === 'videojs'" class="absolute inset-0">
            <video ref="videoRef" class="video-js vjs-default-skin vjs-big-play-centered"></video>
          </div>

          <!-- Native HLS Player -->
          <video
            v-else-if="streamUrl && !error && playerType === 'native'"
            ref="nativeVideoRef"
            class="absolute inset-0 w-full h-full"
            controls
            playsinline
            crossorigin="anonymous"
            @timeupdate="handleTimeUpdate"
            @pause="handlePause"
            @ended="handleEnded"
          >
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
        <!-- Player type toggle -->
        <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          Плеер
        </h3>
        <div class="flex gap-2 mb-4">
          <button
            @click="switchPlayerType('videojs')"
            class="flex-1 px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
            :class="playerType === 'videojs'
              ? 'bg-green-500/20 text-green-400 border border-green-500/50'
              : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
          >
            Video.js
          </button>
          <button
            @click="switchPlayerType('native')"
            class="flex-1 px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
            :class="playerType === 'native'
              ? 'bg-green-500/20 text-green-400 border border-green-500/50'
              : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
          >
            Native
          </button>
        </div>

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

      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import videojs from 'video.js'
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

type PlayerType = 'videojs' | 'native'

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

// Player type (persisted)
const playerType = ref<PlayerType>(
  (localStorage.getItem('preferred_player') as PlayerType) || 'videojs'
)

// State
const episodes = ref<ConsumetEpisode[]>([])
const servers = ref<ConsumetServer[]>([])
const selectedEpisode = ref<ConsumetEpisode | null>(null)
const selectedServer = ref<ConsumetServer | null>(null)
const streamUrl = ref<string | null>(null)
const subtitles = ref<ConsumetSubtitle[]>([])
const streamReferer = ref('')

const loadingEpisodes = ref(false)
const loadingStream = ref(false)
const error = ref<string | null>(null)

const videoRef = ref<HTMLVideoElement | null>(null)
const nativeVideoRef = ref<HTMLVideoElement | null>(null)
let vjsPlayer: ReturnType<typeof videojs> | null = null
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

const disposeCurrentPlayer = () => {
  if (vjsPlayer) {
    vjsPlayer.dispose()
    vjsPlayer = null
  }
  if (hls) {
    hls.destroy()
    hls = null
  }
}

const fetchStream = async () => {
  if (!selectedEpisode.value) return

  // Dispose existing player before reactive state changes remove the DOM element
  disposeCurrentPlayer()

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

    const headers = data.headers || {}
    const referer = headers['Referer'] || headers['referer'] || ''
    streamReferer.value = referer

    // Wait for Vue to render the video element
    await nextTick()

    const targetRef = playerType.value === 'videojs' ? videoRef : nativeVideoRef
    let retries = 0
    while (!targetRef.value && retries < 5) {
      await new Promise(resolve => setTimeout(resolve, 50))
      retries++
    }

    if (!targetRef.value) {
      console.error('[Consumet] Video element not found')
      error.value = 'Ошибка инициализации плеера'
      return
    }

    initPlayer(data.url, referer)
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

const initPlayer = (url: string, referer: string) => {
  if (playerType.value === 'videojs') {
    initVideoJsPlayer(url, referer)
  } else {
    initHlsPlayer(url, referer)
  }
}

const initVideoJsPlayer = (url: string, referer: string) => {
  if (vjsPlayer) {
    vjsPlayer.dispose()
    vjsPlayer = null
  }

  if (!videoRef.value) return

  const proxyUrl = buildProxyUrl(url, referer)

  vjsPlayer = videojs(videoRef.value, {
    controls: true,
    autoplay: false,
    preload: 'auto',
    fill: true,
    playsinline: true,
  })

  // Attach events
  vjsPlayer.on('timeupdate', handleTimeUpdate)
  vjsPlayer.on('pause', handlePause)
  vjsPlayer.on('ended', handleEnded)
  vjsPlayer.on('error', () => {
    const err = vjsPlayer?.error()
    if (err) {
      console.error('[Consumet Video.js Error]', err.code, err.message)
      error.value = 'Ошибка воспроизведения видео'
    }
  })

  // Set source, then add subtitles and play
  vjsPlayer.src({ src: proxyUrl, type: 'application/x-mpegURL' })
  vjsPlayer.ready(() => {
    // Add subtitle tracks after source is set
    for (let i = 0; i < subtitles.value.length; i++) {
      const sub = subtitles.value[i]
      vjsPlayer?.addRemoteTextTrack({
        kind: 'subtitles',
        label: sub.lang,
        srclang: getLanguageCode(sub.lang),
        src: buildSubtitleProxyUrl(sub.url),
        default: i === 0,
      }, false)
    }
    vjsPlayer?.play()?.catch(() => {})
  })
}

const initHlsPlayer = (url: string, referer: string) => {
  if (hls) {
    hls.destroy()
    hls = null
  }

  const video = nativeVideoRef.value
  if (!video) return

  const proxyUrl = buildProxyUrl(url, referer)

  if (Hls.isSupported()) {
    hls = new Hls({
      enableWorker: true,
      lowLatencyMode: false,
      backBufferLength: 90,
      maxBufferLength: 30,
      maxMaxBufferLength: 60,
      maxBufferSize: 60 * 1000 * 1000,
      startLevel: -1,
    })

    hls.loadSource(proxyUrl)
    hls.attachMedia(video)

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      video.play().catch(() => {})
    })

    hls.on(Hls.Events.ERROR, (_event, data) => {
      if (data.fatal) {
        console.error('[Consumet HLS Error]', data.type, data.details)
        switch (data.type) {
          case Hls.ErrorTypes.NETWORK_ERROR:
            hls?.startLoad()
            break
          case Hls.ErrorTypes.MEDIA_ERROR:
            hls?.recoverMediaError()
            break
          default:
            error.value = 'Ошибка воспроизведения видео'
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

const switchPlayerType = async (type: PlayerType) => {
  if (type === playerType.value) return

  disposeCurrentPlayer()

  const savedUrl = streamUrl.value
  const savedReferer = streamReferer.value

  // Clear stream to remove player DOM elements
  streamUrl.value = null
  playerType.value = type
  localStorage.setItem('preferred_player', type)

  if (savedUrl) {
    await nextTick()

    streamUrl.value = savedUrl
    await nextTick()

    const targetRef = type === 'videojs' ? videoRef : nativeVideoRef
    let retries = 0
    while (!targetRef.value && retries < 5) {
      await new Promise(resolve => setTimeout(resolve, 50))
      retries++
    }

    if (targetRef.value) {
      initPlayer(savedUrl, savedReferer)
    }
  }
}

// Progress tracking
const handleTimeUpdate = () => {
  if (!selectedEpisode.value) return

  if (vjsPlayer) {
    currentTime.value = vjsPlayer.currentTime() || 0
  } else if (nativeVideoRef.value) {
    currentTime.value = nativeVideoRef.value.currentTime
  } else {
    return
  }

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
  disposeCurrentPlayer()
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

/* Video.js overrides - green accent */
:deep(.video-js) {
  width: 100%;
  height: 100%;
  font-family: inherit;
}

:deep(.video-js .vjs-big-play-button) {
  background-color: rgba(74, 222, 128, 0.9);
  border: none;
  border-radius: 50%;
  width: 2em;
  height: 2em;
  line-height: 2em;
  font-size: 3em;
  transition: all 0.3s;
}

:deep(.video-js .vjs-big-play-button:hover) {
  background-color: #4ade80;
  transform: scale(1.1);
}

:deep(.video-js:hover .vjs-big-play-button),
:deep(.video-js .vjs-big-play-button:focus) {
  background-color: #4ade80;
}

:deep(.video-js .vjs-control-bar) {
  background-color: rgba(26, 26, 26, 0.9);
  backdrop-filter: blur(10px);
}

:deep(.video-js .vjs-play-progress) {
  background-color: #4ade80;
}

:deep(.video-js .vjs-volume-level) {
  background-color: #4ade80;
}

:deep(.video-js .vjs-slider-horizontal .vjs-volume-level:before) {
  color: #4ade80;
}

:deep(.video-js .vjs-load-progress) {
  background: rgba(255, 255, 255, 0.2);
}

:deep(.video-js .vjs-progress-holder) {
  height: 0.5em;
}

:deep(.video-js .vjs-play-progress:before) {
  font-size: 1em;
  top: -0.25em;
}
</style>
