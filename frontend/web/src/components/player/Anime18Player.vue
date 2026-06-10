<template>
  <div class="anime18-player anime18-player-wrapper">
    <!-- Loading state for episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- No episodes available -->
    <div v-else-if="episodes.length === 0 && !loadingEpisodes" class="text-center py-20 text-white/60">
      <Video class="size-12 mx-auto mb-3 opacity-50" aria-hidden="true" />
      {{ $t('player.noEpisodes', { source: '18anime' }) }}
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
              <p class="text-white/60 text-sm">{{ $t('player.loadingEpisode', { n: (selectedEpisodeIndex ?? 0) + 1 }) }}</p>
            </div>
          </div>

          <!-- Error message -->
          <div
            v-if="error && !loadingStream"
            class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
          >
            <div class="text-center text-rose-400 px-4">
              <TriangleAlert class="size-12 mx-auto mb-3" aria-hidden="true" />
              <p>{{ error }}</p>
            </div>
          </div>

          <!-- Video player -->
          <video
            v-if="streamUrl && !error"
            ref="videoRef"
            class="absolute inset-0 w-full h-full"
            controls
            playsinline
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
              <Play class="size-16 mx-auto mb-3" aria-hidden="true" />
              <p>{{ $t('player.selectEpisode') }}</p>
            </div>
          </div>
        </div>

        <!-- Episode selector below player -->
        <div class="mt-4">
          <div class="flex items-center gap-3 mb-3 flex-wrap">
            <h3 class="text-white/60 text-sm flex items-center gap-2">
              <List class="size-4" aria-hidden="true" />
              {{ $t('player.episodesCount', { count: episodes.length }) }}
            </h3>
            <slot name="header-middle" />
          </div>
          <EpisodeSelector
            :episodes="episodeOptions"
            :selected-key="selectedEpisode?.slug ?? null"
            :watched-up-to="watchedUpTo"
            @select="onEpisodePicked"
          />
        </div>
      </div>

      <!-- Right: Settings panel -->
      <div class="lg:w-72 flex-shrink-0">
        <!-- Quality (18anime resolves a single best mirror per episode) -->
        <div v-if="currentSource" class="mt-0">
          <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
            <MonitorPlay class="size-4" aria-hidden="true" />
            {{ $t('player.quality') }}
          </h3>
          <div class="flex flex-wrap gap-2">
            <span class="px-3 py-1.5 rounded-lg text-sm font-medium accent-bg-muted accent-text border accent-border">
              {{ currentSource.quality || '—' }}
            </span>
          </div>
        </div>

        <!-- Episode info -->
        <div v-if="selectedEpisode" class="mt-4">
          <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
            <Info class="size-4" aria-hidden="true" />
            {{ $t('player.anime18.label') }}
          </h3>
          <p class="text-white text-sm font-medium truncate">{{ $t('player.anime18.label') }} {{ selectedEpisode.number }}</p>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { Video, TriangleAlert, Play, List, MonitorPlay, Info } from 'lucide-vue-next'
import Hls from 'hls.js'
import { anime18Api, userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { usePlayerSyncBridge } from '@/composables/usePlayerSyncBridge'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'

interface Anime18Episode {
  slug: string
  url: string
  number: number
}

interface Anime18Source {
  url: string
  referer?: string
  is_hls: boolean
  quality: string
}

const props = defineProps<{
  animeId: string
  animeName?: string
  totalEpisodes?: number
  initialEpisode?: number
  // room prop drives the WatchTogether sync bridge (wired below once videoRef exists).
  room?: WatchTogetherRoomHandle | null
}>()

const authStore = useAuthStore()

// State
const episodes = ref<Anime18Episode[]>([])

// Watched-episode state
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

// Normalized episode list for EpisodeSelector
const episodeOptions = computed<EpisodeOption[]>(() =>
  episodes.value.map((ep) => ({
    key: ep.slug,
    label: ep.number,
    number: ep.number,
  })),
)

// Bridge: resolve the slug key back to episode + index, then delegate to
// the existing selectEpisode(ep, idx) which owns the room-sync logic.
const onEpisodePicked = async (key: string | number) => {
  const idx = episodes.value.findIndex((e) => e.slug === String(key))
  if (idx === -1) return
  await selectEpisode(episodes.value[idx], idx)
  await refreshWatched()
}
// (episodes declared above with composables)
const selectedEpisode = ref<Anime18Episode | null>(null)
const selectedEpisodeIndex = ref<number | null>(null)
const currentSource = ref<Anime18Source | null>(null)
const streamUrl = ref<string | null>(null)

const loadingEpisodes = ref(false)
const loadingStream = ref(false)
const error = ref<string | null>(null)

const videoRef = ref<HTMLVideoElement | null>(null)
let hls: Hls | null = null

if (props.room) {
  usePlayerSyncBridge(videoRef, props.room)
}

// Progress tracking
const currentTime = ref(0)
const maxTime = ref(0)
const lastSaveTime = ref(0)
const SAVE_INTERVAL = 15

// Build proxy URL. mp4upload MP4 requires a Referer (carried on the source);
// turbovid HLS needs none, so only attach the param when present.
const buildProxyUrl = (url: string, referer?: string): string => {
  const params = new URLSearchParams()
  params.set('url', url)
  if (referer) params.set('referer', referer)
  return `/api/streaming/hls-proxy?${params.toString()}`
}

const destroyHls = () => {
  if (hls) {
    hls.destroy()
    hls = null
  }
}

// initPlayer dispatches on source type: HLS (turbovid) via hls.js, progressive
// MP4 (mp4upload) via native <video src>. Both route through the HLS proxy.
const initPlayer = (source: Anime18Source) => {
  const video = videoRef.value
  if (!video) return

  destroyHls()
  const proxyUrl = buildProxyUrl(source.url, source.referer)

  if (!source.is_hls) {
    // Progressive MP4 (mp4upload) — native playback, proxy injects Referer.
    video.src = proxyUrl
    video.play().catch(() => {})
    return
  }

  if (Hls.isSupported()) {
    hls = new Hls({
      enableWorker: true,
      lowLatencyMode: false,
      backBufferLength: 90,
      maxBufferLength: 30,
      maxMaxBufferLength: 60,
      maxBufferSize: 60 * 1000 * 1000,
      startLevel: -1,
      defaultAudioCodec: 'mp4a.40.2',
    })

    hls.loadSource(proxyUrl)
    hls.attachMedia(video)

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      video.play().catch(() => {})
    })

    hls.on(Hls.Events.ERROR, (_event, data) => {
      if (data.fatal) {
        console.error('[18anime HLS Error]', data.type, data.details)
        if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
          hls?.startLoad()
        } else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
          hls?.recoverMediaError()
        } else {
          error.value = `HLS error: ${data.details}`
        }
      }
    })
  } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
    video.src = proxyUrl
    video.play().catch(() => {})
  } else {
    error.value = 'HLS playback is not supported in this browser'
  }
}

// React to room episode broadcasts. props.initialEpisode is the 1-based ordinal.
watch(
  () => props.initialEpisode,
  (epNum) => {
    if (!props.room || epNum == null || episodes.value.length === 0) return
    const idx = Math.min(epNum - 1, episodes.value.length - 1)
    if (idx < 0 || selectedEpisodeIndex.value === idx) return
    selectEpisode(episodes.value[idx], idx, true)
  },
)

const fetchEpisodes = async () => {
  loadingEpisodes.value = true
  error.value = null

  try {
    const response = await anime18Api.getEpisodes(props.animeId)
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

const selectEpisode = async (ep: Anime18Episode, idx: number, fromRoomSync = false) => {
  if (props.room && !fromRoomSync) {
    props.room.emitChangeEpisode(String(idx + 1))
    return
  }
  selectedEpisode.value = ep
  selectedEpisodeIndex.value = idx
  streamUrl.value = null
  currentSource.value = null
  error.value = null
  await fetchStream(ep)
}

const fetchStream = async (ep: Anime18Episode) => {
  loadingStream.value = true
  error.value = null

  try {
    const response = await anime18Api.getStream(props.animeId, ep.slug)
    const data: Anime18Source | undefined = response.data?.data || response.data

    if (!data || !data.url) {
      error.value = 'Источники видео не найдены'
      return
    }

    currentSource.value = data
    streamUrl.value = data.url
    // Player init happens after DOM update via watcher.
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

const handleEnded = async () => {
  if (!selectedEpisode.value) return
  saveProgress()
  await markEpisodeWatched()
  await refreshWatched()
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
  console.error('[18anime] Video error:', codeName, mediaError.message, 'src:', streamUrl.value)
  error.value = `Video error: ${codeName}${mediaError.message ? ` — ${mediaError.message}` : ''}`
}

const markEpisodeWatched = async () => {
  if (!authStore.isAuthenticated || selectedEpisodeIndex.value === null) return
  const epNum = selectedEpisodeIndex.value + 1
  try {
    await userApi.markEpisodeWatched(props.animeId, epNum)
  } catch (err) {
    console.error('[18anime] Failed to mark episode as watched:', err)
  }
}

// Init player when streamUrl is set and the video element exists.
watch(streamUrl, (url) => {
  if (url && videoRef.value && currentSource.value) {
    initPlayer(currentSource.value)
  }
}, { flush: 'post' })

watch(videoRef, (el) => {
  if (el && streamUrl.value && currentSource.value) {
    initPlayer(currentSource.value)
  }
})

// Reset when anime changes
watch(() => props.animeId, () => {
  saveProgress()
  destroyHls()
  streamUrl.value = null
  currentSource.value = null
  episodes.value = []
  selectedEpisode.value = null
  selectedEpisodeIndex.value = null
  currentTime.value = 0
  maxTime.value = 0
  lastSaveTime.value = 0
  fetchEpisodes()
})

onMounted(async () => {
  await fetchEpisodes()
  await refreshWatched()
})

onBeforeUnmount(() => {
  saveProgress()
  destroyHls()
})

// Exposed for unit tests.
defineExpose({ buildProxyUrl })
</script>

<style scoped>
.anime18-player-wrapper {
  --player-accent: #f43f5e;
  --player-accent-rgb: 244, 63, 94;
}

.accent-bg { background-color: var(--player-accent); }
.accent-text { color: color-mix(in srgb, var(--player-accent), white 40%); }
.accent-border { border-color: var(--player-accent); }
.accent-bg-muted { background-color: rgba(var(--player-accent-rgb), 0.28); }

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
