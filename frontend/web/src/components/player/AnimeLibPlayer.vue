<template>
  <div class="animelib-player animelib-player-wrapper">
    <!-- Loading state for episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <Spinner size="lg" />
    </div>

    <!-- No episodes available -->
    <div v-else-if="episodes.length === 0 && !loadingEpisodes" class="text-center py-20 text-white/60">
      <Video class="size-12 mx-auto mb-3 opacity-50" aria-hidden="true" />
      {{ $t('player.noEpisodes', { source: 'AniLib' }) }}
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
              <p class="text-white/60 text-sm">{{ $t('player.loadingEpisode', { n: selectedEpisode?.number }) }}</p>
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
              <Play class="size-16 mx-auto mb-3" aria-hidden="true" />
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
          <div class="flex items-center gap-3 mb-3 flex-wrap">
            <h3 class="text-white/60 text-sm flex items-center gap-2">
              <List class="size-4" aria-hidden="true" />
              {{ $t('player.episodesCount', { count: episodes.length }) }}
            </h3>
            <slot name="header-middle" />
            <!-- Mark as watched button -->
            <button
              v-if="authStore.isAuthenticated"
              @click="markCurrentEpisodeWatched"
              :disabled="markingWatched"
              class="ml-auto flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
              :class="episodeMarkedWatched
                ? 'accent-bg-muted accent-text border accent-border'
                : 'bg-white/10 text-white hover:bg-white/20'"
            >
              <Check class="size-4" aria-hidden="true" />
              <span class="hidden sm:inline">{{ episodeMarkedWatched ? $t('player.watched') : $t('player.markWatched') }}</span>
            </button>
          </div>
          <EpisodeSelector
            :episodes="episodeOptions"
            :selected-key="selectedEpisode?.id ?? null"
            :watched-up-to="watchedUpTo"
            @select="onEpisodePicked"
          />
        </div>
      </div>

      <!-- Right: Settings panel -->
      <div class="lg:w-72 flex-shrink-0">
        <!-- Translation selector -->
        <h3 class="text-white/60 text-sm mb-3 flex items-center gap-2">
          <Languages class="size-4" aria-hidden="true" />
          {{ $t('player.voiceActing') }}
        </h3>

        <!-- Voice / Subtitles tabs -->
        <div v-if="translations.length > 0" class="mb-3">
          <div class="flex gap-1 bg-white/5 rounded-lg p-1 mb-3">
            <button
              @click="setTranslationFilter('all')"
              class="flex-1 px-2 py-1 rounded-md text-xs font-medium transition-all"
              :class="translationFilter === 'all'
                ? 'bg-white/15 text-white'
                : 'text-white/50 hover:text-white/70'"
            >
              {{ $t('player.allCount', { count: translations.length }) }}
            </button>
            <button
              @click="setTranslationFilter('voice')"
              class="flex-1 px-2 py-1 rounded-md text-xs font-medium transition-all"
              :class="translationFilter === 'voice'
                ? 'bg-white/15 text-white'
                : 'text-white/50 hover:text-white/70'"
            >
              {{ $t('player.voiceActingCount', { count: voiceTranslations.length }) }}
            </button>
            <button
              @click="setTranslationFilter('subtitles')"
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
                ? 'accent-bg-muted border accent-border'
                : 'bg-white/5 border border-transparent hover:bg-white/10'"
            >
              <div class="flex items-center justify-between gap-2">
                <div class="min-w-0">
                  <p class="text-white font-medium text-sm truncate">{{ tr.team_name }}</p>
                  <p class="text-white/40 text-xs">
                    {{ tr.type === 'voice' ? $t('player.dub') : $t('player.sub') }}
                    <span v-if="tr.player === 'Animelib'" class="accent-text ml-1">HD</span>
                  </p>
                </div>
                <div
                  v-if="selectedTranslation?.id === tr.id"
                  class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0 accent-bg"
                >
                  <Check class="size-4 text-black" aria-hidden="true" />
                </div>
              </div>
            </button>
          </div>
        </div>

        <!-- Loading translations -->
        <div v-else-if="loadingTranslations" class="flex items-center justify-center py-6">
          <Spinner size="md" tone="mono" />
        </div>

        <!-- No translations -->
        <div v-else-if="selectedEpisode && !loadingTranslations" class="text-center py-4 text-white/40 text-sm">
          {{ $t('player.noVoiceActing') }}
        </div>

        <!-- Quality selector (only for direct video) -->
        <div v-if="availableSources.length > 1" class="mt-4">
          <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
            <MonitorPlay class="size-4" aria-hidden="true" />
            {{ $t('player.quality') }}
          </h3>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="source in availableSources"
              :key="source.quality"
              @click="selectQuality(source)"
              class="px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
              :class="selectedQuality === source.quality
                ? 'accent-bg-muted accent-text border accent-border'
                : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
            >
              {{ source.quality }}p
            </button>
          </div>
        </div>

        <!-- Subtitle controls (only for direct video with external subtitles) -->
        <div v-if="streamSubtitles.length > 0" class="mt-4">
          <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
            <MessageSquare class="size-4" aria-hidden="true" />
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
                ? 'accent-bg-muted accent-text border accent-border'
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
              ? 'accent-bg-muted accent-text border accent-border'
              : 'bg-white/5 text-white/40 border border-transparent hover:bg-white/10'"
          >
            {{ showSubtitleOverlay ? $t('player.hideSubtitles') : $t('player.showSubtitles') }}
          </button>

          <div v-if="subtitleError" class="mt-1 text-xs text-destructive/70">{{ subtitleError }}</div>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, toRef, onMounted, watch } from 'vue'
import { Video, TriangleAlert, Play, List, Check, Languages, MonitorPlay, MessageSquare } from 'lucide-vue-next'
import { Spinner } from '@/components/ui'
import { useI18n } from 'vue-i18n'
import { animeLibApi, userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useOverrideTracker } from '@/composables/useOverrideTracker'
import { useWatchSession } from '@/composables/useWatchSession'
import { setPreferredWatchType, getPreferredWatchType } from '@/composables/useWatchPreferences'
import { findRecentClick, emitRecWatched } from '@/utils/recsAnalytics'
import SubtitleOverlay from './SubtitleOverlay.vue'
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
import { usePlayerSyncBridge } from '@/composables/usePlayerSyncBridge'
import type { WatchCombo } from '@/types/preference'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'

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
  subtitles?: AnimeLibSubtitle[]
}

const props = defineProps<{
  animeId: string
  animeName?: string
  totalEpisodes?: number
  initialEpisode?: number
  preferredCombo?: WatchCombo | null
  // Phase 3 (03.2) — when set, the player sync bridge mirrors play/pause/seek
  // to the room. When null/undefined the bridge is never instantiated and the
  // player behaves exactly as it did pre-Phase-3 (zero regression).
  room?: WatchTogetherRoomHandle | null
}>()

const emit = defineEmits<{
  (e: 'episodeWatched', data: { episode: number }): void
  (e: 'availableTranslations', combos: WatchCombo[]): void
}>()

const authStore = useAuthStore()
const { t } = useI18n()

// State
const episodes = ref<AnimeLibEpisode[]>([])
const translations = ref<AnimeLibTranslation[]>([])
const selectedEpisode = ref<AnimeLibEpisode | null>(null)
const selectedTranslation = ref<AnimeLibTranslation | null>(null)
const streamUrl = ref<string | null>(null)
const availableSources = ref<AnimeLibSource[]>([])
const selectedQuality = ref<number | null>(null)

const loadingEpisodes = ref(false)
const loadingTranslations = ref(false)
const loadingStream = ref(false)
const error = ref<string | null>(null)
// Match Kodik's late-combo discipline: freeze the player out of late-arriving
// preferredCombo once the user has explicitly clicked something, and skip
// re-selection when the initial auto-pick already used preferredCombo.
const userHasOverridden = ref(false)
const usedPreferredCombo = ref(false)

const translationFilter = ref<'all' | 'voice' | 'subtitles'>('all')
const videoRef = ref<HTMLVideoElement | null>(null)

// Phase 3 (03.2): wire real WatchTogether sync when a room is provided. When
// room is null/undefined the bridge is never instantiated and the player
// behaves exactly as it did pre-Phase-3 (zero regression).
if (props.room) {
  usePlayerSyncBridge(videoRef, props.room)
}

// Subtitle state
const streamSubtitles = ref<AnimeLibSubtitle[]>([])
const activeSubtitleUrl = ref<string | null>(null)
const activeSubtitleFormat = ref<'ass' | 'srt' | 'vtt' | null>(null)
const showSubtitleOverlay = ref(false)
const subtitleError = ref<string | null>(null)

// Progress tracking
const currentTime = ref(0)
const maxTime = ref(0)
const lastSaveTime = ref(0)
const SAVE_INTERVAL = 30
const AUTO_MARK_THRESHOLD = 20 * 60

// Computed: filtered translations
const voiceTranslations = computed(() => translations.value.filter(t => t.type === 'voice'))
const subTranslations = computed(() => translations.value.filter(t => t.type === 'subtitles'))
const filteredTranslations = computed(() => {
  if (translationFilter.value === 'voice') return voiceTranslations.value
  if (translationFilter.value === 'subtitles') return subTranslations.value
  return translations.value
})

const currentCombo = computed((): WatchCombo | null => {
  if (!selectedTranslation.value) return null
  return {
    player: 'animelib',
    language: 'ru',
    watch_type: selectedTranslation.value.type === 'voice' ? 'dub' : 'sub',
    translation_id: String(selectedTranslation.value.id),
    translation_title: selectedTranslation.value.team_name
  }
})

// Watch tracking
const markingWatched = ref(false)
const episodeMarkedWatched = ref(false)

const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const episodeOptions = computed<EpisodeOption[]>(() =>
  episodes.value.map((ep) => ({ key: ep.id, label: ep.number, number: Number(ep.number) })),
)

// currentEpisodeNumber: numeric ref for the override tracker. AnimeLib stores
// the episode as the full AnimeLibEpisode object — extract the number lazily.
const currentEpisodeNumber = computed(() =>
  selectedEpisode.value ? parseInt(selectedEpisode.value.number) || 0 : 0,
)

// Override tracker. User-click handlers wrap the existing logic with
// recordPickerEvent BEFORE the work; programmatic call sites (initial
// auto-select inside fetchEpisodes/fetchTranslations and the post-episode-pick
// re-fetch) bypass via _selectEpisode/_selectTranslation siblings to avoid
// false-positive 'team' overrides when an episode change re-runs translation
// auto-selection.
const tracker = useOverrideTracker({
  animeId: props.animeId,
  player: 'animelib',
  resolvedCombo: toRef(props, 'preferredCombo'),
  currentEpisode: currentEpisodeNumber,
})

// Phase 5 (G-04-lite + G-01): playback session correlation + drop-off beacon.
// Session ID rotates whenever the selected episode changes — Tier 2 cares
// about per-episode sessions, not per-mount sessions.
const { sessionId, newSession, registerBeaconHooks } = useWatchSession()
watch(currentEpisodeNumber, (n, old) => { if (n && n !== old) newSession() })
registerBeaconHooks(() => {
  const ep = currentEpisodeNumber.value
  if (!ep || ep <= 0) return null
  return {
    animeId: props.animeId,
    episodeNumber: ep,
    progressSeconds: Math.floor(currentTime.value),
  }
})

const setTranslationFilter = (filter: 'all' | 'voice' | 'subtitles') => {
  if (translationFilter.value === filter) return
  // 'all' is just a UI filter — only emit when toggling between voice/subtitles.
  if (filter === 'voice' || filter === 'subtitles') {
    tracker.recordPickerEvent('language', { watch_type: filter === 'voice' ? 'dub' : 'sub' })
    userHasOverridden.value = true
    setPreferredWatchType(filter === 'voice' ? 'dub' : 'sub')
  }
  translationFilter.value = filter
}

const onEpisodePicked = (key: string | number) => {
  const ep = episodes.value.find((e) => e.id === key)
  if (ep) selectEpisode(ep)
}

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
      // Initial auto-pick — bypass the user-click wrapper so no false override fires.
      await _selectEpisode(initialEp)
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
      // Emit available translations as WatchCombo[]
      const combos: WatchCombo[] = translations.value.map(tr => ({
        player: 'animelib' as const,
        language: 'ru' as const,
        watch_type: tr.type === 'voice' ? 'dub' as const : 'sub' as const,
        translation_id: String(tr.id),
        translation_title: tr.team_name
      }))
      emit('availableTranslations', combos)

      // Auto-select from preferredCombo if it matches this player.
      // Bypass user-click wrapper — programmatic (no override emission).
      let autoSelected = false
      if (props.preferredCombo?.player === 'animelib') {
        const match = translations.value.find(
          tr => String(tr.id) === props.preferredCombo!.translation_id
            || tr.team_name === props.preferredCombo!.translation_title
        )
        if (match) {
          autoSelected = true
          usedPreferredCombo.value = true
          await _selectTranslation(match)
        }
      }

      if (!autoSelected) {
        // Default fallback honors the user's last-used watch_type from
        // localStorage. The API doesn't sort translations by sub-vs-dub, so
        // without this a sub viewer can land on a dub pick by chance.
        const preferSubs = getPreferredWatchType() === 'sub'
        const voiceTr = translations.value.find(tr => tr.type === 'voice')
        const subTr = translations.value.find(tr => tr.type !== 'voice')
        const picked = preferSubs
          ? (subTr ?? voiceTr ?? translations.value[0])
          : (voiceTr ?? subTr ?? translations.value[0])
        await _selectTranslation(picked)
      }

      // Persist the chosen watch_type so future fresh-anime loads honor it
      // before the server-side resolve returns.
      if (selectedTranslation.value) {
        setPreferredWatchType(selectedTranslation.value.type === 'voice' ? 'dub' : 'sub')
      }
    }
  } catch (err: unknown) {
    console.error('Failed to fetch translations:', err)
    translations.value = []
  } finally {
    loadingTranslations.value = false
  }
}

// Programmatic (no-tracking) episode selector. Used by fetchEpisodes initial
// auto-pick and any other non-user code path. Keep behavior identical to
// selectEpisode minus the recordPickerEvent.
const _selectEpisode = async (ep: AnimeLibEpisode) => {
  selectedEpisode.value = ep
  episodeMarkedWatched.value = false
  selectedTranslation.value = null
  streamUrl.value = null
  availableSources.value = []
  selectedQuality.value = null
  streamSubtitles.value = []
  activeSubtitleUrl.value = null
  activeSubtitleFormat.value = null
  showSubtitleOverlay.value = false
  subtitleError.value = null
  await fetchTranslations()
}

// User-click episode selector — fires combo_override ('episode') BEFORE the work.
// WT-STATE-04: react to room episode broadcasts (own echo or another
// member's change). Map the 1-based number to our episode object and apply
// via the programmatic _selectEpisode path (no re-emit). Without this the
// click only emits and the player never actually switches in a room.
watch(
  () => props.initialEpisode,
  (epNum) => {
    if (!props.room || epNum == null || episodes.value.length === 0) return
    const ep = episodes.value.find(e => parseInt(e.number) === epNum)
    if (ep && ep.id !== selectedEpisode.value?.id) _selectEpisode(ep)
  },
)

const selectEpisode = async (ep: AnimeLibEpisode) => {
  // Phase 4 WT-STATE-04: when mounted inside a Watch Together room,
  // route the user click through the room handle so the backend can
  // validate and broadcast to all members. The room:state_changed
  // broadcast will reactively update room.episode_id, which flows
  // back through the existing :initial-episode prop -> _selectEpisode
  // programmatic path.
  if (props.room) {
    props.room.emitChangeEpisode(String(ep.number))
    return
  }
  tracker.recordPickerEvent('episode', { episode: parseInt(ep.number) || 0 })
  await _selectEpisode(ep)
}

// Programmatic (no-tracking) translation selector. Called from fetchTranslations
// (initial auto-pick OR post-episode-change re-pick — both of which would
// otherwise emit a false-positive 'team' override).
const _selectTranslation = async (tr: AnimeLibTranslation) => {
  selectedTranslation.value = tr
  await fetchStream()
}

// User-click translation selector — fires combo_override ('team') BEFORE the work.
const selectTranslation = async (tr: AnimeLibTranslation) => {
  // Phase 4 WT-STATE-04: same routing as selectEpisode — emit to room and let
  // the inbound room:state_changed broadcast drive the local state mutation
  // (via the existing programmatic _selectTranslation re-pick path).
  if (props.room) {
    props.room.emitChangeTranslation(String(tr.id))
    return
  }
  tracker.recordPickerEvent('team', { translation_title: tr.team_name, player: 'animelib' })
  userHasOverridden.value = true
  setPreferredWatchType(tr.type === 'voice' ? 'dub' : 'sub')
  await _selectTranslation(tr)
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
const saveProgress = () => {
  if (!selectedEpisode.value || currentTime.value <= 0) return

  // Save to localStorage
  const key = `watch_progress:${props.animeId}`
  const existing = JSON.parse(localStorage.getItem(key) || '{}')
  existing[selectedEpisode.value.number] = {
    time: currentTime.value,
    maxTime: maxTime.value,
    updatedAt: Date.now()
  }
  localStorage.setItem(key, JSON.stringify(existing))

  // Save to server if authenticated
  if (authStore.isAuthenticated && currentCombo.value) {
    userApi.updateProgress({
      anime_id: props.animeId,
      episode_number: parseInt(selectedEpisode.value.number),
      progress: Math.floor(currentTime.value),
      duration: Math.floor(maxTime.value) || null,
      session_id: sessionId.value,
      ...currentCombo.value,
    }).catch(() => {})
  }
}

const handleTimeUpdate = () => {
  if (!selectedEpisode.value || !videoRef.value) return
  currentTime.value = videoRef.value.currentTime
  maxTime.value = Math.max(maxTime.value, currentTime.value)

  // Save progress every SAVE_INTERVAL seconds
  if (currentTime.value - lastSaveTime.value >= SAVE_INTERVAL) {
    lastSaveTime.value = currentTime.value
    saveProgress()
  }

  if (maxTime.value >= AUTO_MARK_THRESHOLD && !episodeMarkedWatched.value) {
    markCurrentEpisodeWatched()
  }
}

const handlePause = () => {
  saveProgress()
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
  console.error('[AnimeLib] Video error:', codeName, mediaError.message, 'src:', streamUrl.value)
  error.value = `Video error: ${codeName}${mediaError.message ? ` — ${mediaError.message}` : ''}`
}

const handleEnded = () => {
  if (!selectedEpisode.value) return
  saveProgress()
  markCurrentEpisodeWatched()
}

const markCurrentEpisodeWatched = async () => {
  if (!selectedEpisode.value || markingWatched.value || !authStore.isAuthenticated) return

  markingWatched.value = true
  try {
    const epNum = parseInt(selectedEpisode.value.number)
    await userApi.markEpisodeWatched(props.animeId, epNum, currentCombo.value ?? undefined, sessionId.value)
    episodeMarkedWatched.value = true
    void refreshWatched()
    emit('episodeWatched', { episode: epNum })
    // Phase 14 (REC-EVAL-01): emit rec_watched if a click for this anime
    // landed in the last hour. Strict click→watched correlation.
    const recent = findRecentClick(props.animeId)
    if (recent) {
      void emitRecWatched({
        event_type: 'rec_watched',
        anime_id: props.animeId,
        signal_id: recent.signal_id,
        pinned: recent.pinned,
        pin_source: recent.pin_source,
        pin_seed_anime_id: recent.pin_seed_anime_id,
        source_route: 'player',
        rank: recent.rank,
      })
    }
  } catch (err) {
    console.error('Failed to mark episode as watched:', err)
  } finally {
    markingWatched.value = false
  }
}

// Reset when anime changes
watch(() => props.animeId, () => {
  saveProgress()
  streamUrl.value = null
  episodes.value = []
  translations.value = []
  selectedEpisode.value = null
  selectedTranslation.value = null
  currentTime.value = 0
  maxTime.value = 0
  lastSaveTime.value = 0
  episodeMarkedWatched.value = false
  userHasOverridden.value = false
  usedPreferredCombo.value = false
  void refreshWatched()
  fetchEpisodes()
})

// Late-arriving preferredCombo. When /api/preferences/resolve returns after
// fetchTranslations already defaulted, re-select the matching translation so
// the user's preferred sub/dub axis wins. Same gates as KodikPlayer.
watch(() => props.preferredCombo, async (newCombo) => {
  if (!newCombo || newCombo.player !== 'animelib') return
  if (userHasOverridden.value) return
  if (usedPreferredCombo.value) return
  if (translations.value.length === 0) return

  const match = translations.value.find(
    tr => String(tr.id) === newCombo.translation_id || tr.team_name === newCombo.translation_title,
  )
  if (!match) return
  if (selectedTranslation.value?.id === match.id) return

  usedPreferredCombo.value = true
  setPreferredWatchType(match.type === 'voice' ? 'dub' : 'sub')
  await _selectTranslation(match)
})

// Lifecycle
onMounted(async () => {
  await fetchEpisodes()
  await refreshWatched()
})
</script>

<style scoped>
.animelib-player-wrapper {
  --player-accent: #f97316;
  --player-accent-rgb: 249, 115, 22;
}

.accent-bg { background-color: var(--player-accent); }
.accent-bg-hover:hover { background-color: color-mix(in srgb, var(--player-accent), black 15%); }
/* UA-036: lightened text mix keeps contrast ≥4.5:1 over accent-bg-muted */
.accent-text { color: color-mix(in srgb, var(--player-accent), white 40%); }
.accent-border { border-color: var(--player-accent); }
.accent-bg-muted { background-color: rgba(var(--player-accent-rgb), 0.28); }
.accent-ring { --tw-ring-color: rgba(var(--player-accent-rgb), 0.5); }

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
