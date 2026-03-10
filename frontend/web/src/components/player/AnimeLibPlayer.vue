<template>
  <div class="animelib-player">
    <!-- Loading state for episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 border-orange-400 border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- No episodes available -->
    <div v-else-if="episodes.length === 0 && !loadingEpisodes" class="text-center py-20 text-white/60">
      <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
      </svg>
      {{ $t('player.noEpisodes') || 'Серии не найдены на AniLib' }}
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
              <div class="w-10 h-10 border-2 border-orange-400 border-t-transparent rounded-full animate-spin mx-auto mb-3" />
              <p class="text-white/60 text-sm">{{ $t('player.loadingEpisode', { n: selectedEpisode?.number }) }}</p>
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

          <!-- Direct video player (Animelib native) -->
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
          >
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
              <p>{{ $t('player.selectEpisode') }}</p>
            </div>
          </div>

          <!-- Subtitle overlay for external subtitle files -->
          <SubtitleOverlay
            v-if="streamSubtitles.length > 0"
            :video-element="videoRef"
            :subtitle-url="activeSubtitleUrl"
            :format="activeSubtitleFormat"
            :visible="showSubtitleOverlay"
            @loading="() => {}"
            @error="(msg: string) => subtitleError = msg"
          />
        </div>

        <!-- Episode selector below player -->
        <div class="mt-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-white/60 text-sm flex items-center gap-2">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
              </svg>
              {{ $t('player.episodesCount', { count: episodes.length }) }}
            </h3>
            <!-- Mark as watched button -->
            <button
              v-if="authStore.isAuthenticated"
              @click="markCurrentEpisodeWatched"
              :disabled="markingWatched"
              class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
              :class="episodeMarkedWatched
                ? 'bg-orange-500/20 text-orange-400 border border-orange-500/50'
                : 'bg-white/10 text-white hover:bg-white/20'"
            >
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
              </svg>
              {{ episodeMarkedWatched ? $t('player.watched') : $t('player.markWatched') }}
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
                  ? 'bg-orange-500 text-white'
                  : isEpisodeWatched(parseInt(ep.number))
                    ? 'bg-orange-500/20 text-orange-400 border border-orange-500/30 hover:bg-orange-500/30'
                    : 'bg-white/10 text-white hover:bg-white/20'
              ]"
              :title="ep.name || `Episode ${ep.number}`"
            >
              {{ ep.number }}
              <span
                v-if="isEpisodeWatched(parseInt(ep.number)) && selectedEpisode?.id !== ep.id"
                class="absolute -top-1 -right-1 w-3 h-3 bg-orange-500 rounded-full flex items-center justify-center"
              >
                <svg class="w-2 h-2 text-black" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                </svg>
              </span>
            </button>
          </div>
        </div>
      </div>

      <!-- Right: Settings panel -->
      <div class="lg:w-72 flex-shrink-0">
        <!-- Translation selector -->
        <h3 class="text-white/60 text-sm mb-3 flex items-center gap-2">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129" />
          </svg>
          {{ $t('player.voiceActing') }}
        </h3>

        <!-- Voice / Subtitles tabs -->
        <div v-if="translations.length > 0" class="mb-3">
          <div class="flex gap-1 bg-white/5 rounded-lg p-1 mb-3">
            <button
              @click="translationFilter = 'all'"
              class="flex-1 px-2 py-1 rounded-md text-xs font-medium transition-all"
              :class="translationFilter === 'all'
                ? 'bg-white/15 text-white'
                : 'text-white/50 hover:text-white/70'"
            >
              {{ $t('player.allCount', { count: translations.length }) }}
            </button>
            <button
              @click="translationFilter = 'voice'"
              class="flex-1 px-2 py-1 rounded-md text-xs font-medium transition-all"
              :class="translationFilter === 'voice'
                ? 'bg-white/15 text-white'
                : 'text-white/50 hover:text-white/70'"
            >
              {{ $t('player.voiceActingCount', { count: voiceTranslations.length }) }}
            </button>
            <button
              @click="translationFilter = 'subtitles'"
              class="flex-1 px-2 py-1 rounded-md text-xs font-medium transition-all"
              :class="translationFilter === 'subtitles'
                ? 'bg-white/15 text-white'
                : 'text-white/50 hover:text-white/70'"
            >
              {{ $t('player.subtitlesCount', { count: subTranslations.length }) }}
            </button>
          </div>

          <!-- Translation team list -->
          <div class="space-y-2 max-h-48 overflow-y-auto custom-scrollbar">
            <button
              v-for="tr in filteredTranslations"
              :key="tr.id"
              @click="selectTranslation(tr)"
              class="w-full text-left p-3 rounded-lg transition-all"
              :class="selectedTranslation?.id === tr.id
                ? 'bg-orange-500/20 border border-orange-500/50'
                : 'bg-white/5 border border-transparent hover:bg-white/10'"
            >
              <div class="flex items-center justify-between gap-2">
                <div class="min-w-0">
                  <p class="text-white font-medium text-sm truncate">{{ tr.team_name }}</p>
                  <p class="text-white/40 text-xs">
                    {{ tr.type === 'voice' ? $t('player.dub') : $t('player.sub') }}
                    <span v-if="tr.player === 'Animelib'" class="text-orange-400 ml-1">HD</span>
                  </p>
                </div>
                <div
                  v-if="selectedTranslation?.id === tr.id"
                  class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0 bg-orange-500"
                >
                  <svg class="w-4 h-4 text-black" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                  </svg>
                </div>
              </div>
            </button>
          </div>
        </div>

        <!-- Loading translations -->
        <div v-else-if="loadingTranslations" class="flex items-center justify-center py-6">
          <div class="w-6 h-6 border-2 border-orange-400 border-t-transparent rounded-full animate-spin" />
        </div>

        <!-- No translations -->
        <div v-else-if="selectedEpisode && !loadingTranslations" class="text-center py-4 text-white/40 text-sm">
          {{ $t('player.noVoiceActing') }}
        </div>

        <!-- Quality selector (only for direct video) -->
        <div v-if="availableSources.length > 1" class="mt-4">
          <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01" />
            </svg>
            {{ $t('player.quality') }}
          </h3>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="source in availableSources"
              :key="source.quality"
              @click="selectQuality(source)"
              class="px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
              :class="selectedQuality === source.quality
                ? 'bg-orange-500/20 text-orange-400 border border-orange-500/50'
                : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
            >
              {{ source.quality }}p
            </button>
          </div>
        </div>

        <!-- Subtitle controls (only for direct video with external subtitles) -->
        <div v-if="streamSubtitles.length > 0" class="mt-4">
          <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
            </svg>
            {{ $t('player.subtitles') }}
          </h3>

          <!-- Format selector (if multiple formats available) -->
          <div v-if="streamSubtitles.length > 1" class="flex flex-wrap gap-2 mb-2">
            <button
              v-for="sub in streamSubtitles"
              :key="sub.url"
              @click="selectSubtitle(sub)"
              class="px-3 py-1.5 rounded-lg text-sm font-medium transition-all uppercase"
              :class="activeSubtitleUrl === buildProxyUrl(sub.url)
                ? 'bg-orange-500/20 text-orange-400 border border-orange-500/50'
                : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
            >
              {{ sub.format }}
            </button>
          </div>

          <!-- Toggle visibility -->
          <button
            @click="showSubtitleOverlay = !showSubtitleOverlay"
            class="w-full flex items-center justify-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
            :class="showSubtitleOverlay
              ? 'bg-orange-500/20 text-orange-400 border border-orange-500/50'
              : 'bg-white/5 text-white/40 border border-transparent hover:bg-white/10'"
          >
            {{ showSubtitleOverlay ? $t('player.hideSubtitles') : $t('player.showSubtitles') }}
          </button>

          <div v-if="subtitleError" class="mt-1 text-xs text-red-400/70">{{ subtitleError }}</div>
        </div>
      </div>
    </div>

    <!-- Report button -->
    <ReportButton
      player-type="animelib"
      :anime-id="animeId"
      :anime-name="animeName || animeId"
      :episode-number="selectedEpisode ? parseInt(selectedEpisode.number) : undefined"
      :server-name="selectedTranslation?.team_name"
      :stream-url="streamUrl"
      :error-message="error"
      accent-color="#f97316"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { animeLibApi, userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import SubtitleOverlay from './SubtitleOverlay.vue'
import ReportButton from './ReportButton.vue'

interface AnimeLibEpisode {
  id: number
  number: string
  name: string
}

interface AnimeLibTranslation {
  id: number
  team_name: string
  type: string
  player: string // "Animelib" or "Kodik"
  has_subtitles: boolean
}

interface AnimeLibSource {
  url: string
  quality: number
}

interface AnimeLibSubtitle {
  format: string // "ass", "vtt"
  url: string
}

interface AnimeLibStream {
  sources?: AnimeLibSource[]
  iframe_url?: string
  subtitles?: AnimeLibSubtitle[]
}

const props = defineProps<{
  animeId: string
  animeName?: string
  totalEpisodes?: number
  initialEpisode?: number
}>()

const emit = defineEmits<{
  (e: 'episodeWatched', data: { episode: number }): void
}>()

const authStore = useAuthStore()
const { t } = useI18n()

// State
const episodes = ref<AnimeLibEpisode[]>([])
const translations = ref<AnimeLibTranslation[]>([])
const selectedEpisode = ref<AnimeLibEpisode | null>(null)
const selectedTranslation = ref<AnimeLibTranslation | null>(null)
const streamUrl = ref<string | null>(null)
// iframeUrl removed — Kodik fallback disabled to expose MP4 errors
const availableSources = ref<AnimeLibSource[]>([])
const selectedQuality = ref<number | null>(null)

const loadingEpisodes = ref(false)
const loadingTranslations = ref(false)
const loadingStream = ref(false)
const error = ref<string | null>(null)

const translationFilter = ref<'all' | 'voice' | 'subtitles'>('all')
const videoRef = ref<HTMLVideoElement | null>(null)

// Subtitle state
const streamSubtitles = ref<AnimeLibSubtitle[]>([])
const activeSubtitleUrl = ref<string | null>(null)
const activeSubtitleFormat = ref<'ass' | 'srt' | 'vtt' | null>(null)
const showSubtitleOverlay = ref(false)
const subtitleError = ref<string | null>(null)

// Progress tracking
const currentTime = ref(0)
const maxTime = ref(0)
const AUTO_MARK_THRESHOLD = 20 * 60

// Computed: filtered translations
const voiceTranslations = computed(() => translations.value.filter(t => t.type === 'voice'))
const subTranslations = computed(() => translations.value.filter(t => t.type === 'subtitles'))
const filteredTranslations = computed(() => {
  if (translationFilter.value === 'voice') return voiceTranslations.value
  if (translationFilter.value === 'subtitles') return subTranslations.value
  return translations.value
})

// Watch tracking
const markingWatched = ref(false)
const episodeMarkedWatched = ref(false)
const watchedEpisodes = ref(0)

// Methods
const fetchEpisodes = async () => {
  loadingEpisodes.value = true
  error.value = null

  try {
    const response = await animeLibApi.getEpisodes(props.animeId)
    const data = response.data?.data || response.data || []
    episodes.value = Array.isArray(data) ? data : []
    loadingEpisodes.value = false

    if (episodes.value.length > 0) {
      const initialEp = props.initialEpisode
        ? episodes.value.find(e => parseInt(e.number) === props.initialEpisode) || episodes.value[0]
        : episodes.value[0]
      await selectEpisode(initialEp)
    }
  } catch (err: unknown) {
    const e = err as { response?: { data?: { message?: string } } }
    error.value = e.response?.data?.message || t('player.error.loadEpisodes')
    episodes.value = []
    loadingEpisodes.value = false
  }
}

const fetchTranslations = async () => {
  if (!selectedEpisode.value) return

  loadingTranslations.value = true

  try {
    const response = await animeLibApi.getTranslations(props.animeId, selectedEpisode.value.id)
    const data = response.data?.data || response.data || []
    translations.value = Array.isArray(data) ? data : []

    if (translations.value.length > 0) {
      await selectTranslation(translations.value[0])
    }
  } catch (err: unknown) {
    console.error('Failed to fetch translations:', err)
    translations.value = []
  } finally {
    loadingTranslations.value = false
  }
}

const selectEpisode = async (ep: AnimeLibEpisode) => {
  selectedEpisode.value = ep
  episodeMarkedWatched.value = false
  selectedTranslation.value = null
  streamUrl.value = null
  // iframe fallback removed
  availableSources.value = []
  selectedQuality.value = null
  streamSubtitles.value = []
  activeSubtitleUrl.value = null
  activeSubtitleFormat.value = null
  showSubtitleOverlay.value = false
  subtitleError.value = null
  await fetchTranslations()
}

const selectTranslation = async (tr: AnimeLibTranslation) => {
  selectedTranslation.value = tr
  await fetchStream()
}

const selectSubtitle = (sub: AnimeLibSubtitle) => {
  activeSubtitleUrl.value = buildProxyUrl(sub.url)
  activeSubtitleFormat.value = sub.format as 'ass' | 'vtt'
  showSubtitleOverlay.value = true
}

const selectQuality = (source: AnimeLibSource) => {
  if (source.url === streamUrl.value) return
  selectedQuality.value = source.quality
  streamUrl.value = source.url
  if (videoRef.value) {
    const currentPos = videoRef.value.currentTime
    videoRef.value.src = source.url
    videoRef.value.currentTime = currentPos
    videoRef.value.play().catch(() => {})
  }
}

const fetchStream = async () => {
  if (!selectedEpisode.value || !selectedTranslation.value) return

  streamUrl.value = null
  // iframe fallback removed
  availableSources.value = []
  selectedQuality.value = null
  streamSubtitles.value = []
  activeSubtitleUrl.value = null
  activeSubtitleFormat.value = null
  showSubtitleOverlay.value = false
  subtitleError.value = null
  loadingStream.value = true
  error.value = null

  try {
    const response = await animeLibApi.getStream(
      props.animeId,
      selectedEpisode.value.id,
      selectedTranslation.value.id
    )
    const data: AnimeLibStream = response.data?.data || response.data

    if (data.sources && data.sources.length > 0) {
      // Direct MP4 video (Animelib native player) — proxy through our backend for CORS
      availableSources.value = data.sources
        .map(s => ({ ...s, url: buildProxyUrl(s.url) }))
        .sort((a, b) => b.quality - a.quality)
      const best = availableSources.value[0]
      selectedQuality.value = best.quality
      streamUrl.value = best.url

      // Capture external subtitles if available
      if (data.subtitles && data.subtitles.length > 0) {
        streamSubtitles.value = data.subtitles
        // Prefer ASS for richer styling, fall back to VTT
        const assSub = data.subtitles.find(s => s.format === 'ass')
        const activeSub = assSub || data.subtitles[0]
        activeSubtitleUrl.value = buildProxyUrl(activeSub.url)
        activeSubtitleFormat.value = activeSub.format as 'ass' | 'vtt'
        // Auto-enable for subtitle-type translations
        showSubtitleOverlay.value = selectedTranslation.value?.type === 'subtitles'
      }
    } else {
      error.value = t('player.error.getVideoUrl')
    }
  } catch (err: unknown) {
    const e = err as { response?: { data?: { error?: { message?: string }; message?: string } } }
    const message = e.response?.data?.error?.message
      || e.response?.data?.message
      || t('player.error.loadVideo')
    error.value = message
  } finally {
    loadingStream.value = false
  }
}

const buildProxyUrl = (url: string): string => {
  const params = new URLSearchParams()
  params.set('url', url)
  params.set('referer', 'https://v3.animelib.org/')
  return `/api/streaming/hls-proxy?${params.toString()}`
}

// Progress tracking
const handleTimeUpdate = () => {
  if (!selectedEpisode.value || !videoRef.value) return
  currentTime.value = videoRef.value.currentTime
  maxTime.value = Math.max(maxTime.value, currentTime.value)

  if (maxTime.value >= AUTO_MARK_THRESHOLD && !episodeMarkedWatched.value) {
    markCurrentEpisodeWatched()
  }
}

const handlePause = () => {}

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
  console.error('[AnimeLib] Video error:', codeName, mediaError.message, 'src:', streamUrl.value)
  error.value = `Video error: ${codeName}${mediaError.message ? ` — ${mediaError.message}` : ''}`
}

const handleEnded = () => {
  if (!selectedEpisode.value) return
  markCurrentEpisodeWatched()
}

const markCurrentEpisodeWatched = async () => {
  if (!selectedEpisode.value || markingWatched.value || !authStore.isAuthenticated) return

  markingWatched.value = true
  try {
    const epNum = parseInt(selectedEpisode.value.number)
    await userApi.markEpisodeWatched(props.animeId, epNum)
    episodeMarkedWatched.value = true
    watchedEpisodes.value = Math.max(watchedEpisodes.value, epNum)
    emit('episodeWatched', { episode: epNum })
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

  if (authStore.isAuthenticated) {
    try {
      const response = await userApi.getWatchlistEntry(props.animeId)
      const entry = response.data?.data || response.data
      if (entry?.episodes_watched) {
        watchedEpisodes.value = entry.episodes_watched
      }
    } catch {
      // Ignore
    }
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
