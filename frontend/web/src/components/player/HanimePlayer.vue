<template>
  <div class="hanime-player hanime-player-wrapper">
    <!-- Loading state for episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <Spinner size="lg" />
    </div>

    <!-- No episodes available -->
    <div v-else-if="episodes.length === 0 && !loadingEpisodes" class="text-center py-20 text-white/60">
      <Video class="size-12 mx-auto mb-3 opacity-50" aria-hidden="true" />
      {{ $t('player.noEpisodes', { source: 'Hanime' }) }}
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
              <Spinner size="lg" class="mx-auto mb-3" />
              <p class="text-white/60 text-sm">{{ $t('player.loadingEpisode', { n: (selectedEpisodeIndex ?? 0) + 1 }) }}</p>
            </div>
          </div>

          <!-- Error message -->
          <div
            v-if="error && !loadingStream"
            class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
          >
            <div class="text-center text-pink-400 px-4">
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
        <!-- Quality selector -->
        <div v-if="availableSources.length > 0" class="mt-0">
          <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
            <MonitorPlay class="size-4" aria-hidden="true" />
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
            <Info class="size-4" aria-hidden="true" />
            Эпизод
          </h3>
          <p class="text-white text-sm font-medium truncate">{{ selectedEpisode.name }}</p>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, watch } from 'vue'
import { Video, TriangleAlert, Play, List, MonitorPlay, Info } from 'lucide-vue-next'
import { Spinner } from '@/components/ui'
import Hls from 'hls.js'
import { hanimeApi, userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { usePlayerSyncBridge } from '@/composables/usePlayerSyncBridge'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'

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
  // Phase 3 (03.3) — room prop drives the WatchTogether sync bridge (wired below
  // once `videoRef` is declared).
  room?: WatchTogetherRoomHandle | null
}>()

const authStore = useAuthStore()

const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

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
let hls: Hls | null = null

// Normalized episode list for EpisodeSelector — ordinal-based (Hanime has no
// numeric episode field; the ordinal idx+1 doubles as both label and number).
const episodeOptions = computed<EpisodeOption[]>(() =>
  episodes.value.map((ep, idx) => ({ key: ep.slug, label: idx + 1, number: idx + 1 })),
)

function onEpisodePicked(key: string | number) {
  const idx = episodes.value.findIndex((e) => e.slug === key)
  if (idx >= 0) selectEpisode(episodes.value[idx], idx)
}

// Phase 3 (03.3): wire real sync when a room is provided. Zero behavior
// change when room is null/undefined.
if (props.room) {
  usePlayerSyncBridge(videoRef, props.room)
}

// Progress tracking
const currentTime = ref(0)
const maxTime = ref(0)
const lastSaveTime = ref(0)
const SAVE_INTERVAL = 15

// Build proxy URL
const buildProxyUrl = (url: string): string => {
  const params = new URLSearchParams()
  params.set('url', url)
  params.set('referer', 'https://hanime.tv/')
  return `/api/streaming/hls-proxy?${params.toString()}`
}

const destroyHls = () => {
  if (hls) {
    hls.destroy()
    hls = null
  }
}

const initHlsPlayer = (url: string) => {
  const video = videoRef.value
  if (!video) return

  destroyHls()

  const proxyUrl = buildProxyUrl(url)

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
        console.error('[Hanime HLS Error]', data.type, data.details)
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
    // Safari native HLS
    video.src = proxyUrl
    video.play().catch(() => {})
  } else {
    error.value = 'HLS playback is not supported in this browser'
  }
}

// WT-STATE-04: react to room episode broadcasts (own echo or another
// member's change). props.initialEpisode is the 1-based ordinal; map it
// back to an index and re-select with fromRoomSync=true to avoid re-emitting.
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

const selectEpisode = async (ep: HanimeEpisode, idx: number, fromRoomSync = false) => {
  // Phase 4 WT-STATE-04: when mounted inside a Watch Together room,
  // route the user click (or end-of-episode auto-advance, which calls the
  // same function) through the room handle so the backend can validate
  // and broadcast to all members. The room:state_changed broadcast will
  // reactively update room.episode_id, which flows back through the
  // existing :initial-episode prop -> programmatic re-select path.
  if (props.room && !fromRoomSync) {
    props.room.emitChangeEpisode(String(idx + 1))
    return
  }
  selectedEpisode.value = ep
  selectedEpisodeIndex.value = idx
  streamUrl.value = null
  availableSources.value = []
  selectedSource.value = null
  error.value = null
  void refreshWatched()
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

    // Select best quality (prefer 720p, then highest)
    const preferred = availableSources.value.find(s => s.height === '720') || availableSources.value[0]
    selectedSource.value = preferred
    streamUrl.value = preferred.url
    // HLS init happens after DOM update via watcher
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
  const pos = videoRef.value?.currentTime || 0
  selectedSource.value = source
  streamUrl.value = source.url
  initHlsPlayer(source.url)
  // Restore position after HLS loads
  if (hls && pos > 0) {
    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      if (videoRef.value) {
        videoRef.value.currentTime = pos
      }
    })
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
  void refreshWatched()
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

// Init HLS when streamUrl is set and video element exists
watch(streamUrl, (url) => {
  if (url && videoRef.value) {
    initHlsPlayer(url)
  }
}, { flush: 'post' })

// Also watch videoRef in case it mounts after streamUrl is set
watch(videoRef, (el) => {
  if (el && streamUrl.value) {
    initHlsPlayer(streamUrl.value)
  }
})

// Reset when anime changes
watch(() => props.animeId, () => {
  saveProgress()
  destroyHls()
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
  void refreshWatched()
  await fetchEpisodes()
})

onBeforeUnmount(() => {
  saveProgress()
  destroyHls()
})
</script>

<style scoped>
.hanime-player-wrapper {
  --player-accent: #ec4899;
  --player-accent-rgb: 236, 72, 153;
}

.accent-bg { background-color: var(--player-accent); }
/* UA-036: lightened text mix keeps contrast ≥4.5:1 over accent-bg-muted */
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
