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

          <!-- Video.js Player -->
          <div v-if="streamUrl && streamType === 'hls' && playerType === 'videojs'" class="absolute inset-0">
            <video ref="videoRef" class="video-js vjs-default-skin vjs-big-play-centered"></video>
          </div>

          <!-- Native HLS Player -->
          <video
            v-else-if="streamUrl && streamType === 'hls' && playerType === 'native'"
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

          <!-- Japanese Subtitle Overlay (outside v-if chain) -->
          <SubtitleOverlay
            :video-element="activeVideoElement"
            :subtitle-url="activeSubtitleUrl"
            :format="activeSubtitleFormat"
            :visible="showJpOverlay"
            @loading="(v: boolean) => loadingSubOverlay = v"
            @error="(msg: string) => jimakuError = msg"
          />
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
              ? 'bg-purple-500/20 text-purple-400 border border-purple-500/50'
              : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
          >
            Video.js
          </button>
          <button
            @click="switchPlayerType('native')"
            class="flex-1 px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
            :class="playerType === 'native'
              ? 'bg-purple-500/20 text-purple-400 border border-purple-500/50'
              : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
          >
            Native
          </button>
        </div>

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

        <!-- Subtitles section -->
        <div v-if="subtitles.length > 0 || selectedEpisode" class="mt-4 p-3 rounded-lg bg-white/5">
          <h4 class="text-white/60 text-sm mb-2">Субтитры</h4>
          <!-- Stream subtitles -->
          <div v-if="subtitles.length > 0" class="flex flex-wrap gap-2 mb-3">
            <span
              v-for="sub in subtitles"
              :key="sub.url"
              class="px-2 py-1 text-xs rounded bg-white/10 text-white/70"
            >
              {{ sub.label }}
            </span>
          </div>
          <!-- Japanese Subtitles (Jimaku) -->
          <div v-if="selectedEpisode">
            <button
              @click="fetchJimakuSubtitles"
              :disabled="loadingJimaku"
              class="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-all"
              :class="jimakuLoaded
                ? 'bg-red-500/20 text-red-400 border border-red-500/50'
                : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
            >
              <svg v-if="loadingJimaku" class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
              <span v-else>JP</span>
              {{ jimakuLoaded ? `JP Subs (${jimakuSubtitles.length})` : 'Load JP Subs' }}
            </button>
            <div v-if="jimakuError" class="mt-1 text-xs text-red-400/70">{{ jimakuError }}</div>
            <div v-if="jimakuLoaded && jimakuSubtitles.length > 0" class="mt-2 space-y-1 max-h-32 overflow-y-auto">
              <button
                v-for="(sub, index) in jimakuSubtitles"
                :key="index"
                @click="activateJimakuSubtitle(sub)"
                class="w-full text-left px-2 py-1 rounded text-xs transition-all"
                :class="activeJimakuSub === sub.url
                  ? 'bg-red-500/20 text-red-400'
                  : 'text-white/50 hover:bg-white/5 hover:text-white/70'"
              >
                {{ sub.file_name }} ({{ sub.format }})
              </button>
            </div>
            <!-- Toggle overlay visibility when active -->
            <button
              v-if="activeJimakuSub"
              @click="showJpOverlay = !showJpOverlay"
              class="mt-2 w-full flex items-center justify-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium transition-all"
              :class="showJpOverlay
                ? 'bg-red-500/20 text-red-400 border border-red-500/50'
                : 'bg-white/5 text-white/40 border border-transparent hover:bg-white/10'"
            >
              {{ showJpOverlay ? 'Hide JP Subs' : 'Show JP Subs' }}
            </button>
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
import videojs from 'video.js'
import Hls from 'hls.js'
import { hiAnimeApi, jimakuApi, userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import SubtitleOverlay from './SubtitleOverlay.vue'

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
const episodes = ref<HiAnimeEpisode[]>([])
const servers = ref<HiAnimeServer[]>([])
const selectedEpisode = ref<HiAnimeEpisode | null>(null)
const selectedServer = ref<HiAnimeServer | null>(null)
const selectedCategory = ref<'sub' | 'dub'>('sub')
const streamUrl = ref<string | null>(null)
const streamType = ref<'hls' | 'mp4' | 'iframe'>('hls')
const subtitles = ref<HiAnimeSubtitle[]>([])
const streamReferer = ref('')

const loadingEpisodes = ref(false)
const loadingServers = ref(false)
const loadingStream = ref(false)
const error = ref<string | null>(null)
const serverLoadWarning = ref<string | null>(null)

const videoRef = ref<HTMLVideoElement | null>(null)
const nativeVideoRef = ref<HTMLVideoElement | null>(null)
let vjsPlayer: ReturnType<typeof videojs> | null = null
const vjsPlayerReady = ref(false)
let hls: Hls | null = null

// Progress tracking
const currentTime = ref(0)
const maxTime = ref(0)
const lastSaveTime = ref(0)
const SAVE_INTERVAL = 30
const AUTO_MARK_THRESHOLD = 20 * 60 // 20 minutes

// Jimaku Japanese subtitles
interface JimakuSubtitle {
  url: string
  file_name: string
  lang: string
  format: string
}
const jimakuSubtitles = ref<JimakuSubtitle[]>([])
const loadingJimaku = ref(false)
const jimakuLoaded = ref(false)
const jimakuError = ref<string | null>(null)
const activeJimakuSub = ref<string | null>(null)

// Subtitle overlay state
const activeSubtitleUrl = ref<string | null>(null)
const activeSubtitleFormat = ref<'ass' | 'srt' | 'vtt' | null>(null)
const showJpOverlay = ref(true)
const loadingSubOverlay = ref(false)

// Computed: get the active video element for subtitle sync
const activeVideoElement = computed<HTMLVideoElement | null>(() => {
  if (playerType.value === 'videojs' && vjsPlayerReady.value && vjsPlayer) {
    return vjsPlayer.el()?.querySelector('video') || null
  }
  return nativeVideoRef.value
})

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

const disposeCurrentPlayer = () => {
  if (vjsPlayer) {
    vjsPlayer.dispose()
    vjsPlayer = null
    vjsPlayerReady.value = false
  }
  if (hls) {
    hls.destroy()
    hls = null
  }
}

const fetchStream = async () => {
  if (!selectedEpisode.value || !selectedServer.value) return

  // Dispose existing player BEFORE reactive state changes remove the DOM element
  disposeCurrentPlayer()

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

    const headers = data.headers || {}
    const referer = headers['Referer'] || headers['referer'] || ''
    streamReferer.value = referer

    // Initialize player if HLS
    if (data.type === 'hls' && data.url) {
      // Wait for Vue to render the video element
      await nextTick()

      const targetRef = playerType.value === 'videojs' ? videoRef : nativeVideoRef
      let retries = 0
      while (!targetRef.value && retries < 5) {
        await new Promise(resolve => setTimeout(resolve, 50))
        retries++
      }

      if (!targetRef.value) {
        console.error('[HiAnime] Video element not found')
        error.value = 'Ошибка инициализации плеера'
        return
      }

      initPlayer(data.url, referer)
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
  const params = new URLSearchParams()
  params.set('url', url)
  return `/api/streaming/hls-proxy?${params.toString()}`
}

// Jimaku Japanese subtitles
const fetchJimakuSubtitles = async () => {
  if (!selectedEpisode.value || loadingJimaku.value) return

  loadingJimaku.value = true
  jimakuError.value = null

  try {
    const response = await jimakuApi.getSubtitles(props.animeId, selectedEpisode.value.number)
    const data = response.data?.data || response.data
    jimakuSubtitles.value = data.subtitles || []
    jimakuLoaded.value = true

    if (jimakuSubtitles.value.length === 0) {
      jimakuError.value = 'No Japanese subtitles found for this episode'
    }
  } catch (err: any) {
    const msg = err.response?.data?.error || err.response?.data?.message || err.message
    jimakuError.value = msg || 'Failed to load Japanese subtitles'
    jimakuLoaded.value = false
  } finally {
    loadingJimaku.value = false
  }
}

const activateJimakuSubtitle = (sub: JimakuSubtitle) => {
  activeJimakuSub.value = sub.url
  // Use the custom subtitle overlay for selectable text rendering
  activeSubtitleUrl.value = sub.url
  activeSubtitleFormat.value = (sub.format as 'ass' | 'srt' | 'vtt') || 'ass'
  showJpOverlay.value = true
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

const initPlayer = (url: string, referer: string) => {
  if (playerType.value === 'videojs') {
    initVideoJsPlayer(url, referer)
  } else {
    initHlsPlayer(url, referer)
  }
}

const initVideoJsPlayer = (url: string, referer: string = '') => {
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
      console.error('[HiAnime Video.js Error]', err.code, err.message)
      error.value = 'Ошибка воспроизведения видео'
    }
  })

  // Set source, then add subtitles and play
  vjsPlayer.src({ src: proxyUrl, type: 'application/x-mpegURL' })
  vjsPlayer.ready(() => {
    vjsPlayerReady.value = true
    // Add subtitle tracks after source is set
    for (let i = 0; i < subtitles.value.length; i++) {
      const sub = subtitles.value[i]
      vjsPlayer?.addRemoteTextTrack({
        kind: 'subtitles',
        label: sub.label,
        srclang: getLanguageCode(sub.lang),
        src: buildSubtitleProxyUrl(sub.url),
        default: sub.default || i === 0,
      }, false)
    }
    vjsPlayer?.play()?.catch(() => {})
  })
}

const initHlsPlayer = (url: string, referer: string = '') => {
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
        console.error('[HiAnime HLS Error]', data.type, data.details)
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        if ((data as any).response?.code === 503) {
          error.value = 'Сервер занят. Попробуйте позже или используйте Kodik.'
        } else {
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
      }
    })
  } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
    // Native HLS support (Safari) - still needs proxy
    video.src = proxyUrl
    video.addEventListener('loadedmetadata', () => {
      video.play().catch(() => {})
    })
  } else {
    error.value = 'Ваш браузер не поддерживает HLS'
  }
}

const switchPlayerType = async (type: PlayerType) => {
  if (type === playerType.value) return

  disposeCurrentPlayer()

  const savedUrl = streamUrl.value
  const savedReferer = streamReferer.value
  const savedType = streamType.value

  // Clear stream to remove player DOM elements
  streamUrl.value = null
  playerType.value = type
  localStorage.setItem('preferred_player', type)

  // Re-init player if HLS stream was active
  if (savedUrl && savedType === 'hls') {
    await nextTick()

    streamUrl.value = savedUrl
    streamType.value = savedType
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
  } else if (savedUrl) {
    // Non-HLS stream (iframe) - just restore URL
    streamUrl.value = savedUrl
    streamType.value = savedType
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

  // Reset stream and jimaku state
  streamUrl.value = null
  servers.value = []
  selectedServer.value = null
  jimakuSubtitles.value = []
  jimakuLoaded.value = false
  jimakuError.value = null
  activeJimakuSub.value = null
  activeSubtitleUrl.value = null
  activeSubtitleFormat.value = null

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
  if (!selectedEpisode.value) return

  if (vjsPlayer) {
    currentTime.value = vjsPlayer.currentTime() || 0
  } else if (nativeVideoRef.value) {
    currentTime.value = nativeVideoRef.value.currentTime
  } else {
    return
  }

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

// Keyboard shortcuts (document-level so they work regardless of focus)
const handleKeyDown = (e: KeyboardEvent) => {
  const target = e.target as HTMLElement
  if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return
  if (!streamUrl.value) return

  switch (e.code) {
    case 'Space':
      e.preventDefault()
      if (playerType.value === 'videojs' && vjsPlayer) {
        vjsPlayer.paused() ? vjsPlayer.play() : vjsPlayer.pause()
      } else if (nativeVideoRef.value) {
        nativeVideoRef.value.paused ? nativeVideoRef.value.play() : nativeVideoRef.value.pause()
      }
      break
    case 'ArrowLeft':
      e.preventDefault()
      if (playerType.value === 'videojs' && vjsPlayer) {
        vjsPlayer.currentTime(Math.max(0, (vjsPlayer.currentTime() || 0) - 5))
      } else if (nativeVideoRef.value) {
        nativeVideoRef.value.currentTime = Math.max(0, nativeVideoRef.value.currentTime - 5)
      }
      break
    case 'ArrowRight':
      e.preventDefault()
      if (playerType.value === 'videojs' && vjsPlayer) {
        vjsPlayer.currentTime((vjsPlayer.currentTime() || 0) + 5)
      } else if (nativeVideoRef.value) {
        nativeVideoRef.value.currentTime += 5
      }
      break
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
  disposeCurrentPlayer()
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
  document.addEventListener('keydown', handleKeyDown)
  fetchEpisodes()
  fetchWatchedEpisodes()
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeyDown)
  saveProgress()
  disposeCurrentPlayer()
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

/* Video.js overrides - purple accent */
:deep(.video-js) {
  width: 100%;
  height: 100%;
  font-family: inherit;
}

:deep(.video-js .vjs-big-play-button) {
  background-color: rgba(168, 85, 247, 0.9);
  border: none;
  border-radius: 50%;
  width: 2em;
  height: 2em;
  line-height: 2em;
  font-size: 3em;
  transition: all 0.3s;
}

:deep(.video-js .vjs-big-play-button:hover) {
  background-color: #a855f7;
  transform: scale(1.1);
}

:deep(.video-js:hover .vjs-big-play-button),
:deep(.video-js .vjs-big-play-button:focus) {
  background-color: #a855f7;
}

:deep(.video-js .vjs-control-bar) {
  background-color: rgba(26, 26, 26, 0.9);
  backdrop-filter: blur(10px);
}

:deep(.video-js .vjs-play-progress) {
  background-color: #a855f7;
}

:deep(.video-js .vjs-volume-level) {
  background-color: #a855f7;
}

:deep(.video-js .vjs-slider-horizontal .vjs-volume-level:before) {
  color: #a855f7;
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
