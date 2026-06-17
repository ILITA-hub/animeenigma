<template>
  <div class="kodik-adfree-player">
    <!-- Loading state for translations -->
    <div v-if="loadingTranslations" class="flex items-center justify-center py-20">
      <Spinner size="lg" />
    </div>

    <!-- No translations available -->
    <EmptyState v-else-if="translations.length === 0 && !loadingTranslations" size="lg">
      <template #icon><Video class="size-12 opacity-50" /></template>
      {{ $t('player.noTranslations') || 'Нет доступных озвучек' }}
    </EmptyState>

    <!-- Main content when translations available -->
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
              <p class="text-white/60 text-sm">{{ $t('player.loadingEpisode', { n: selectedEpisode }) }}</p>
            </div>
          </div>

          <!-- Stream extract error overlay -->
          <div
            v-if="streamError && !loadingStream"
            class="absolute inset-0 z-10 flex items-center justify-center bg-black/80 p-6"
          >
            <div class="text-center space-y-4 max-w-sm">
              <TriangleAlert class="size-12 mx-auto text-destructive" aria-hidden="true" />
              <p class="text-destructive text-sm font-medium">{{ $t('player.kodikAdfree.extractError') }}</p>
              <button
                data-testid="report-button"
                class="inline-flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-destructive/20 hover:bg-destructive/30 text-destructive border border-destructive/40 transition-colors"
                @click="reportStreamError"
              >
                <Flag class="size-4" aria-hidden="true" />
                {{ $t('player.report') || 'Сообщить об ошибке' }}
              </button>
            </div>
          </div>

          <!-- Native HLS video element -->
          <video
            ref="videoRef"
            class="absolute inset-0 w-full h-full"
            controls
            playsinline
            v-show="!streamError"
            @timeupdate="handleTimeUpdate"
          />

          <!-- Skip intro button -->
          <button
            v-if="introPlaying && showSkip"
            class="absolute bottom-6 right-6 z-20 rounded-md bg-background/80 px-4 py-2 text-sm font-medium text-foreground hover:bg-accent"
            @click="skipIntro"
          >
            {{ $t('player.kodikAdfree.skipIntro') }}
          </button>

          <!-- Placeholder when no video loaded yet -->
          <div
            v-if="!selectedTranslation && !loadingStream && !streamError"
            class="absolute inset-0 flex items-center justify-center"
          >
            <div class="text-center text-white/40">
              <Play class="size-16 mx-auto mb-3" aria-hidden="true" />
              <p>{{ $t('player.selectVoice') }}</p>
            </div>
          </div>
        </div>

        <!-- Episodes below player -->
        <div class="mt-4">
          <div class="flex items-center gap-3 mb-3 flex-wrap">
            <h3 class="text-white/60 text-sm flex items-center gap-2">
              <List class="size-4" aria-hidden="true" />
              {{ $t('player.episodesCount', { count: episodeRange.length }) }}
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
            :selected-key="selectedEpisode"
            :watched-up-to="watchedUpTo"
            @select="(k) => selectEpisode(Number(k))"
          />
        </div>
      </div>

      <!-- Right: Translations list -->
      <div class="lg:w-72 flex-shrink-0">
        <!-- Tab buttons -->
        <div class="flex gap-2 mb-3">
          <button
            @click="setTranslationType('voice')"
            class="flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-all"
            :class="translationType === 'voice'
              ? 'bg-success/20 text-success border border-success/50'
              : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
          >
            <Mic2 class="size-4" aria-hidden="true" />
            {{ $t('player.dub') }}
            <span class="text-xs opacity-70">({{ voiceTranslations.length }})</span>
          </button>
          <button
            @click="setTranslationType('subtitles')"
            class="flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-all"
            :class="translationType === 'subtitles'
              ? 'bg-info/20 text-info border border-info/50'
              : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
          >
            <MessageSquare class="size-4" aria-hidden="true" />
            {{ $t('player.sub') }}
            <span class="text-xs opacity-70">({{ subtitleTranslations.length }})</span>
          </button>
        </div>

        <!-- Translations list -->
        <div class="space-y-2 max-h-[350px] lg:max-h-[450px] overflow-y-auto custom-scrollbar pr-1">
          <template v-if="filteredTranslations.length > 0">
            <div
              v-for="tr in filteredTranslations"
              :key="tr.id"
              class="relative group"
            >
              <button
                @click="selectTranslation(tr.id)"
                class="w-full text-left p-3 rounded-lg transition-all"
                :class="[
                  selectedTranslation === tr.id
                    ? (translationType === 'voice' ? 'bg-success/20 border border-success/50' : 'bg-info/20 border border-info/50')
                    : 'bg-white/5 border border-transparent hover:bg-white/10',
                  tr.pinned ? 'ring-1 ring-warning/30' : ''
                ]"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <!-- Pinned badge -->
                      <span
                        v-if="tr.pinned"
                        class="inline-flex items-center gap-1 text-xs px-1.5 py-0.5 rounded bg-warning/20 text-warning"
                        :title="$t('player.recommendedVoice')"
                      >
                        <Star class="w-3 h-3" fill="currentColor" aria-hidden="true" />
                      </span>
                      <p class="text-white font-medium truncate" :title="tr.title">{{ tr.title }}</p>
                    </div>
                    <span class="text-white/40 text-xs">{{ tr.episodes_count || 1 }} {{ $t('player.episodeShort') }}</span>
                  </div>
                  <div
                    v-if="selectedTranslation === tr.id"
                    class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                    :class="translationType === 'voice' ? 'bg-success' : 'bg-info'"
                  >
                    <Check class="size-4 text-black" aria-hidden="true" />
                  </div>
                </div>
              </button>

              <!-- Pin/Unpin button -->
              <button
                @click.stop="togglePin(tr)"
                class="absolute top-2 right-2 p-1.5 rounded-lg transition-all opacity-0 group-hover:opacity-100"
                :class="tr.pinned
                  ? 'bg-warning/20 text-warning hover:bg-warning/30'
                  : 'bg-white/10 text-white/40 hover:bg-white/20 hover:text-white'"
                :title="tr.pinned ? $t('player.unpin') : $t('player.pin')"
              >
                <Pin class="size-4" :fill="tr.pinned ? 'currentColor' : 'none'" aria-hidden="true" />
              </button>
            </div>
          </template>
          <div v-else class="text-center py-8 text-white/40">
            <p>{{ translationType === 'voice' ? $t('player.noVoiceActing') : $t('player.noSubtitlesAvailable') }}</p>
          </div>
        </div>

        <!-- Quality selector -->
        <div v-if="availableQualities.length > 1" class="mt-4">
          <h3 class="text-white/60 text-sm mb-2 flex items-center gap-2">
            <MonitorPlay class="size-4" aria-hidden="true" />
            {{ $t('player.quality') }}
          </h3>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="q in availableQualities"
              :key="q"
              :data-testid="`quality-${q}`"
              @click="selectQuality(q)"
              class="px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
              :class="selectedQuality === q
                ? 'accent-bg-muted accent-text border accent-border'
                : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
            >
              {{ q }}p
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- General error message -->
    <div v-if="error" class="mt-4 p-4 bg-destructive/20 border border-destructive/30 rounded-lg text-destructive">
      {{ error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Video, TriangleAlert, Flag, Play, List, Check, Mic2, MessageSquare, MonitorPlay, Star, Pin } from 'lucide-vue-next'
import { Spinner, EmptyState } from '@/components/ui'
import { useI18n } from 'vue-i18n'
import Hls from 'hls.js'
import { kodikApi, userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useWatchSession } from '@/composables/useWatchSession'
import { usePlayerSyncBridge } from '@/composables/usePlayerSyncBridge'
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
import { emitRecWatchedIfRecent } from '@/utils/recsAnalytics'
import type { WatchCombo } from '@/types/preference'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'
const { t } = useI18n()

// ── Watch progress tracking ─────────────────────────────────────────────────
const currentTime = ref(0)
const maxTime = ref(0)
const lastSaveTime = ref(0)
const SAVE_INTERVAL = 30
const AUTO_MARK_FALLBACK = 20 * 60
// Duration-aware threshold (seconds): 90% of the known episode duration,
// else the legacy 20-min rule. The `nearEnd` check below (real HTML5
// duration) usually fires first; this covers metadata-less edge cases.
const autoMarkThreshold = computed(() => {
  const durMin = props.episodeDurationMin ?? 0
  if (durMin > 0) return Math.min(AUTO_MARK_FALLBACK, Math.round(0.9 * durMin * 60))
  return AUTO_MARK_FALLBACK
})
const authStore = useAuthStore()
const { sessionId } = useWatchSession()

// Mark episode as watched
const markingWatched = ref(false)
const episodeMarkedWatched = ref(false)

// ── Props (mirror KodikPlayer.vue exactly) ──────────────────────────────────
const props = defineProps<{
  animeId: string
  animeName?: string
  totalEpisodes?: number
  initialEpisode?: number
  preferredCombo?: WatchCombo | null
  /** Per-episode duration in MINUTES (Shikimori metadata) — see KodikPlayer.
   *  This player also has the real video duration (HTML5), so this prop is a
   *  secondary signal for the pre-metadata window. */
  episodeDurationMin?: number
  // When set, the player sync bridge mirrors play/pause/seek to the room.
  // When null/undefined the bridge is never instantiated and the player
  // behaves exactly as it did pre-WT wiring (zero regression).
  room?: WatchTogetherRoomHandle | null
}>()

// ── Emits (mirror KodikPlayer.vue) ──────────────────────────────────────────
const emit = defineEmits<{
  (e: 'progress', data: { episode: number; time: number; maxTime: number }): void
  (e: 'episodeWatched', data: { episode: number }): void
  (e: 'availableTranslations', combos: WatchCombo[]): void
}>()

// ── Translation types ────────────────────────────────────────────────────────
interface KodikTranslation {
  id: number
  title: string
  type: string
  episodes_count: number
  pinned?: boolean
}

interface PinnedTranslation {
  anime_id: string
  translation_id: number
  translation_title: string
  translation_type: string
}

// ── Watched-episode state (mirrored from KodikPlayer) ───────────────────────
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

// ── Translation state (mirrored from KodikPlayer) ───────────────────────────
const translations = ref<KodikTranslation[]>([])
const pinnedIds = ref<Set<number>>(new Set())
const selectedTranslation = ref<number | null>(null)
const selectedEpisode = ref(1)
const loadingTranslations = ref(false)
const loadingStream = ref(false)
const error = ref<string | null>(null)
const translationType = ref<'voice' | 'subtitles'>('voice')

const voiceTranslations = computed(() => sortByPinned(translations.value.filter(t => t.type === 'voice')))
const subtitleTranslations = computed(() => sortByPinned(translations.value.filter(t => t.type !== 'voice')))
const filteredTranslations = computed(() =>
  translationType.value === 'voice' ? voiceTranslations.value : subtitleTranslations.value
)

function sortByPinned(list: KodikTranslation[]): KodikTranslation[] {
  return [...list].sort((a, b) => {
    if (a.pinned && !b.pinned) return -1
    if (!a.pinned && b.pinned) return 1
    return a.title.localeCompare(b.title)
  })
}

const episodeRange = computed(() => {
  const selectedTrans = translations.value.find(t => t.id === selectedTranslation.value)
  const count = selectedTrans?.episodes_count || props.totalEpisodes || 12
  return Array.from({ length: count }, (_, i) => i + 1)
})

const episodeOptions = computed<EpisodeOption[]>(() =>
  episodeRange.value.map((n) => ({ key: n, label: n, number: n })),
)

// ── currentCombo (player:'kodik' so progress merges with iframe player's) ───
const currentCombo = computed((): WatchCombo | null => {
  if (!selectedTranslation.value) return null
  const tr = translations.value.find(t => t.id === selectedTranslation.value)
  if (!tr) return null
  return {
    player: 'kodik',
    language: 'ru',
    watch_type: tr.type === 'voice' ? 'dub' : 'sub',
    translation_id: String(tr.id),
    translation_title: tr.title,
  }
})

// Save progress to localStorage (works without auth)
const saveProgressLocal = (animeId: string, episode: number, time: number) => {
  const key = `watch_progress:${animeId}`
  const data = JSON.parse(localStorage.getItem(key) || '{}')
  data[episode] = { time, maxTime: maxTime.value, updatedAt: Date.now() }
  localStorage.setItem(key, JSON.stringify(data))
}

// Save progress to server (requires auth)
const saveProgressServer = async (animeId: string, episode: number, time: number) => {
  if (!authStore.isAuthenticated) return
  try {
    await userApi.updateProgress({
      anime_id: animeId,
      episode_number: episode,
      progress: Math.floor(time),
      duration: Math.floor(maxTime.value) || null,
      session_id: sessionId.value,
      ...currentCombo.value,
    })
  } catch (err) {
    console.warn('Failed to save progress to server:', err)
  }
}

// Mark current episode as watched (manual button)
const markCurrentEpisodeWatched = async () => {
  if (!authStore.isAuthenticated || markingWatched.value) return
  markingWatched.value = true
  try {
    await userApi.markEpisodeWatched(props.animeId, selectedEpisode.value, currentCombo.value ?? undefined, sessionId.value)
    episodeMarkedWatched.value = true
    await refreshWatched()
    emit('episodeWatched', { episode: selectedEpisode.value })
    void emitRecWatchedIfRecent(props.animeId, 'player')
  } catch (err: unknown) {
    const e = err as { response?: { data?: { message?: string } } }
    error.value = e.response?.data?.message || t('player.error.markWatched')
  } finally {
    markingWatched.value = false
  }
}

// Auto-mark episode as watched (silent, no UI feedback on error)
const autoMarkEpisodeWatched = async () => {
  if (!authStore.isAuthenticated || episodeMarkedWatched.value) return
  try {
    await userApi.markEpisodeWatched(props.animeId, selectedEpisode.value, currentCombo.value ?? undefined, sessionId.value)
    episodeMarkedWatched.value = true
    await refreshWatched()
    emit('episodeWatched', { episode: selectedEpisode.value })
    void emitRecWatchedIfRecent(props.animeId, 'player')
  } catch {
    // Silent fail for auto-mark
  }
}

// ── HLS + stream state (mirrored from RawPlayer) ────────────────────────────
const videoRef = ref<HTMLVideoElement | null>(null)
let hls: Hls | null = null

// ── Watch Together sync bridge ───────────────────────────────────────────────
// Mirror AnimeLibPlayer's wiring: when a room is provided, the bridge handles
// play/pause/seek/time-tick/drift automatically for the real <video> element.
// When room is null/undefined the bridge is never instantiated (zero regression).
if (props.room) {
  usePlayerSyncBridge(videoRef, props.room)
}

// Remember the active selection so we can re-extract once if the signed CDN
// URL expires mid-session (spec §5).
let current: { episode: number; translationID: number } | null = null
let reloadedOnce = false

const streamError = ref(false)

// ── Quality selection ────────────────────────────────────────────────────────
// Kodik's /ftor "default" quality is 360p, so quality is requested explicitly:
// the user's persisted choice, else QUALITY_MAX (backend PickQuality clamps to
// the highest available). selectedQuality mirrors what the backend actually
// served; only an explicit user click persists a preference.
const QUALITY_PREF_KEY = 'kodik_adfree_quality'
const QUALITY_MAX = 2160
const availableQualities = ref<number[]>([])
const selectedQuality = ref<number | null>(null)
// Position to restore after the next stream attach (quality switch / re-extract).
let pendingSeek = 0

function requestedQuality(): number {
  const saved = Number(localStorage.getItem(QUALITY_PREF_KEY) || '')
  return Number.isFinite(saved) && saved > 0 ? saved : QUALITY_MAX
}

function selectQuality(q: number) {
  if (q === selectedQuality.value) return
  localStorage.setItem(QUALITY_PREF_KEY, String(q))
  selectedQuality.value = q
  const v = videoRef.value
  pendingSeek = v && !introPlaying.value ? v.currentTime : 0
  if (current) void loadStream(current.episode, current.translationID)
}

// ── Pre-roll intro state ─────────────────────────────────────────────────────
// A branded 5s video plays before the real Kodik stream, replacing Kodik's own
// ad pre-roll. The intro is shown ONCE per (translation:episode) key, tracked
// by introShownFor. After the first view, subsequent selections skip straight
// to attachStream. If the asset is missing or autoplay is blocked, onerror /
// play().catch proceed immediately to the real stream (no dead-end).
const INTRO_SRC = '/branding/intro.mp4'
const showSkip = ref(false)
const introPlaying = ref(false)
const introShownFor = new Set<string>()
let skipTimer: ReturnType<typeof setTimeout> | null = null
let proceedFn: (() => void) | null = null

// ── buildProxyUrl (from plan, verbatim) ─────────────────────────────────────
function buildProxyUrl(url: string, referer: string): string {
  const params = new URLSearchParams()
  params.set('url', url)
  if (referer) params.set('referer', referer)
  return `/api/streaming/hls-proxy?${params.toString()}`
}

// ── disposePlayer (from RawPlayer pattern) ───────────────────────────────────
function disposePlayer() {
  if (hls) { hls.destroy(); hls = null }
  const v = videoRef.value
  if (v) { v.removeAttribute('src'); try { v.load() } catch { /* ignore */ } }
}

// ── attachStream (from plan, verbatim) ───────────────────────────────────────
function attachStream(streamUrl: string, referer: string) {
  const v = videoRef.value
  if (!v) return
  disposePlayer()
  const proxyUrl = buildProxyUrl(streamUrl, referer)
  if (Hls.isSupported()) {
    hls = new Hls({ enableWorker: true, lowLatencyMode: false, backBufferLength: 90 })
    hls.loadSource(proxyUrl)
    hls.attachMedia(v)
    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      if (pendingSeek > 0) { v.currentTime = pendingSeek; pendingSeek = 0 }
      v.play().catch(() => {})
    })
    hls.on(Hls.Events.ERROR, (_e, data) => {
      if (!data.fatal) return
      // Expired signed CDN URL -> re-extract a fresh stream once, then give up.
      if (!reloadedOnce && current) {
        reloadedOnce = true
        pendingSeek = v.currentTime || pendingSeek
        void loadStream(current.episode, current.translationID)
      } else {
        streamError.value = true
      }
    })
  } else if (v.canPlayType('application/vnd.apple.mpegurl')) {
    v.src = proxyUrl
    v.addEventListener('loadedmetadata', () => {
      if (pendingSeek > 0) { v.currentTime = pendingSeek; pendingSeek = 0 }
      v.play().catch(() => {})
    }, { once: true })
  }
}

// ── playWithIntro (Task 9, from plan verbatim) ───────────────────────────────
function playWithIntro(streamUrl: string, referer: string, episodeKey: string) {
  // Skip the branded intro in a Watch Together room — synced members must not
  // each play a 5s pre-roll (it desyncs them). Proceed straight to attachStream.
  if (props.room) { attachStream(streamUrl, referer); return }
  if (skipTimer) { clearTimeout(skipTimer); skipTimer = null }
  const v = videoRef.value
  if (!v) return
  if (introShownFor.has(episodeKey)) { attachStream(streamUrl, referer); return }
  introShownFor.add(episodeKey)

  disposePlayer()
  introPlaying.value = true
  showSkip.value = false

  const proceed = () => {
    if (!introPlaying.value) return
    introPlaying.value = false
    showSkip.value = false
    proceedFn = null
    if (skipTimer) { clearTimeout(skipTimer); skipTimer = null }
    v.onended = null; v.onerror = null
    attachStream(streamUrl, referer)
  }
  proceedFn = proceed

  v.src = INTRO_SRC
  v.onended = proceed
  v.onerror = proceed   // missing/unplayable asset -> straight to stream
  skipTimer = setTimeout(() => { showSkip.value = true }, 3000)
  v.play().catch(() => { proceed() }) // autoplay blocked -> don't trap the user on the intro
}

// ── skipIntro (Task 9, from plan verbatim) ───────────────────────────────────
function skipIntro() { proceedFn?.() }

// ── handleTimeUpdate — drives progress saving and auto-mark ─────────────────
function handleTimeUpdate() {
  const v = videoRef.value
  if (!v || introPlaying.value) return  // never track during branded intro
  const time = v.currentTime
  currentTime.value = time
  if (Number.isFinite(v.duration) && v.duration > 0) maxTime.value = v.duration
  else if (time > maxTime.value) maxTime.value = time
  emit('progress', { episode: selectedEpisode.value, time, maxTime: maxTime.value })
  if (time - lastSaveTime.value >= SAVE_INTERVAL) {
    lastSaveTime.value = time
    saveProgressLocal(props.animeId, selectedEpisode.value, time)
    saveProgressServer(props.animeId, selectedEpisode.value, time)
  }
  // Auto-mark: near the end (real duration) OR the 20-min rule (covers short eps)
  const nearEnd = Number.isFinite(v.duration) && v.duration > 0 && time >= 0.9 * v.duration
  if (authStore.isAuthenticated && !episodeMarkedWatched.value && (nearEnd || time >= autoMarkThreshold.value)) {
    autoMarkEpisodeWatched()
  }
}

// ── loadStream (from plan, verbatim) ─────────────────────────────────────────
async function loadStream(episode: number, translationID: number) {
  streamError.value = false
  loadingStream.value = true
  // Reset the one-shot retry budget only on a NEW selection, never on the
  // retry itself (which re-calls loadStream with the same ep/translation).
  const changed = !current || current.episode !== episode || current.translationID !== translationID
  if (changed) reloadedOnce = false
  current = { episode, translationID }
  try {
    const resp = await kodikApi.getStream(props.animeId, episode, translationID, requestedQuality())
    const data = resp.data?.data ?? resp.data
    selectedQuality.value = data.quality ?? null
    availableQualities.value = Array.isArray(data.qualities)
      ? [...data.qualities].sort((a: number, b: number) => b - a)
      : []
    playWithIntro(data.stream_url, data.referer, `${translationID}:${episode}`)
  } catch {
    streamError.value = true
  } finally {
    loadingStream.value = false
  }
}

// ── Error reporting ──────────────────────────────────────────────────────────
async function reportStreamError() {
  try {
    await userApi.reportError({
      player: 'kodik-adfree',
      anime_id: props.animeId,
      episode: selectedEpisode.value,
      translation_id: selectedTranslation.value,
    })
  } catch {
    // Silent fail — report is best-effort
  }
}

// ── Translation fetch (mirrored from KodikPlayer) ───────────────────────────
const fetchPinnedTranslations = async () => {
  try {
    const response = await kodikApi.getPinnedTranslations(props.animeId)
    const data = response.data?.data || response.data || []
    pinnedIds.value = new Set(data.map((p: PinnedTranslation) => p.translation_id))
  } catch {
    pinnedIds.value = new Set()
  }
}

const fetchTranslations = async () => {
  loadingTranslations.value = true
  error.value = null

  try {
    await fetchPinnedTranslations()

    const response = await kodikApi.getTranslations(props.animeId)
    const data = response.data?.data || response.data
    const rawTranslations: KodikTranslation[] = Array.isArray(data) ? data : []

    translations.value = rawTranslations.map(t => ({
      ...t,
      pinned: pinnedIds.value.has(t.id)
    }))

    if (translations.value.length > 0) {
      // Emit available translations as WatchCombo[]
      const combos: WatchCombo[] = translations.value.map(tr => ({
        player: 'kodik' as const,
        language: 'ru' as const,
        watch_type: tr.type === 'voice' ? 'dub' as const : 'sub' as const,
        translation_id: String(tr.id),
        translation_title: tr.title,
        episodes_count: tr.episodes_count
      }))
      emit('availableTranslations', combos)

      // Auto-select from preferredCombo if it matches this player
      let autoSelected = false
      if (props.preferredCombo?.player === 'kodik') {
        const match = translations.value.find(
          t => String(t.id) === props.preferredCombo!.translation_id
            || t.title === props.preferredCombo!.translation_title
        )
        if (match) {
          translationType.value = match.type === 'voice' ? 'voice' : 'subtitles'
          selectedTranslation.value = match.id
          autoSelected = true
        }
      }

      if (!autoSelected) {
        const voices = translations.value.filter(t => t.type === 'voice')
        const pinnedVoice = voices.find(t => t.pinned)
        if (pinnedVoice) {
          translationType.value = 'voice'
          selectedTranslation.value = pinnedVoice.id
        } else if (voices.length > 0) {
          translationType.value = 'voice'
          selectedTranslation.value = voices[0].id
        } else if (translations.value.length > 0) {
          translationType.value = 'subtitles'
          selectedTranslation.value = translations.value[0].id
        }
      }

      // Auto-load first video after setting translation.
      // Must flush loadingTranslations first so the <video> element mounts and
      // videoRef binds before attachStream runs (same fix as OurEnglishPlayer).
      if (selectedTranslation.value) {
        loadingTranslations.value = false
        await nextTick()
        await loadStream(selectedEpisode.value, selectedTranslation.value)
        return // loadingTranslations already cleared above
      }
    }
  } catch (err: unknown) {
    const e = err as { response?: { data?: { message?: string } } }
    error.value = e.response?.data?.message || t('player.error.loadTranslations')
    translations.value = []
  } finally {
    loadingTranslations.value = false
  }
}

// ── Watch Together inbound episode sync ──────────────────────────────────────
// React to room episode broadcasts (own echo or another member's change).
// Map the 1-based number to our selectedEpisode and apply via the programmatic
// selectEpisodeLocal path (no re-emit). Mirror AnimeLibPlayer ~572.
watch(
  () => props.initialEpisode,
  (epNum) => {
    if (!props.room || epNum == null || translations.value.length === 0) return
    if (selectedEpisode.value === epNum) return
    selectedEpisode.value = epNum
    episodeMarkedWatched.value = epNum <= watchedUpTo.value
    if (selectedTranslation.value) void loadStream(epNum, selectedTranslation.value)
  },
)

// ── Translation UI handlers ──────────────────────────────────────────────────
function setTranslationType(type: 'voice' | 'subtitles') {
  if (translationType.value === type) return
  translationType.value = type
}

function selectTranslation(translationId: number) {
  // Phase WT: when in a room, route through the room handle so the backend
  // validates and broadcasts to all members. The broadcast's room:state_changed
  // event will drive the local state mutation via the initialEpisode watcher.
  if (props.room) {
    props.room.emitChangeTranslation(String(translationId))
    return
  }
  if (selectedTranslation.value === translationId) return

  pendingSeek = 0
  selectedTranslation.value = translationId
  const trans = translations.value.find(t => t.id === translationId)
  if (trans?.episodes_count && selectedEpisode.value > trans.episodes_count) {
    selectedEpisode.value = 1
  }
  void loadStream(selectedEpisode.value, translationId)
}

function selectEpisode(episode: number) {
  // Phase WT: when in a room, route the user click through the room handle so
  // the backend can validate and broadcast to all members. The room:state_changed
  // broadcast will reactively update room.episode_id, which flows back through
  // the existing :initial-episode prop -> inbound watcher programmatic path.
  if (props.room) {
    props.room.emitChangeEpisode(String(episode))
    return
  }
  if (selectedEpisode.value === episode) return
  // Save progress of current episode before switching
  if (currentTime.value > 0) {
    saveProgressLocal(props.animeId, selectedEpisode.value, currentTime.value)
    saveProgressServer(props.animeId, selectedEpisode.value, currentTime.value)
  }
  // Reset progress tracking for new episode
  currentTime.value = 0
  maxTime.value = 0
  lastSaveTime.value = 0
  pendingSeek = 0
  episodeMarkedWatched.value = episode <= watchedUpTo.value
  selectedEpisode.value = episode
  if (selectedTranslation.value) {
    void loadStream(episode, selectedTranslation.value)
  }
}

async function togglePin(translation: KodikTranslation) {
  try {
    if (translation.pinned) {
      await kodikApi.unpinTranslation(props.animeId, translation.id)
      pinnedIds.value.delete(translation.id)
    } else {
      await kodikApi.pinTranslation(props.animeId, translation.id, translation.title, translation.type)
      pinnedIds.value.add(translation.id)
    }
    translations.value = translations.value.map(t => ({
      ...t,
      pinned: pinnedIds.value.has(t.id)
    }))
  } catch (err: unknown) {
    const e = err as { response?: { data?: { message?: string } } }
    error.value = e.response?.data?.message || t('player.error.pinTranslation')
  }
}

// ── Lifecycle ────────────────────────────────────────────────────────────────
onMounted(() => {
  if (props.initialEpisode) {
    selectedEpisode.value = props.initialEpisode
  }
  // episodeMarkedWatched seeded after refreshWatched resolves; reset for now
  episodeMarkedWatched.value = false
  void fetchTranslations()
  void refreshWatched()
})

onBeforeUnmount(() => {
  // Save progress when component unmounts
  if (currentTime.value > 0) {
    saveProgressLocal(props.animeId, selectedEpisode.value, currentTime.value)
  }
  disposePlayer()
  if (skipTimer) { clearTimeout(skipTimer); skipTimer = null }
})

watch(() => props.animeId, () => {
  // Save current progress before switching anime
  if (currentTime.value > 0) {
    saveProgressLocal(props.animeId, selectedEpisode.value, currentTime.value)
  }
  selectedEpisode.value = 1
  currentTime.value = 0
  maxTime.value = 0
  lastSaveTime.value = 0
  pendingSeek = 0
  availableQualities.value = []
  selectedQuality.value = null
  episodeMarkedWatched.value = false
  streamError.value = false
  error.value = null
  introPlaying.value = false
  showSkip.value = false
  proceedFn = null
  if (skipTimer) { clearTimeout(skipTimer); skipTimer = null }
  // Clear the per-session intro guard so the branded intro plays once per
  // anime across the session — not permanently suppressed on anime switch.
  introShownFor.clear()
  void fetchTranslations()
  void refreshWatched()
})
</script>

<style scoped>
.kodik-adfree-player {
  --player-accent: #06b6d4;
  --player-accent-rgb: 6, 182, 212;
  width: 100%;
}

.accent-bg { background-color: var(--player-accent); }
.accent-text { color: color-mix(in srgb, var(--player-accent), white 40%); }
.accent-border { border-color: var(--player-accent); }
.accent-bg-muted { background-color: rgba(var(--player-accent-rgb), 0.28); }

.custom-scrollbar::-webkit-scrollbar {
  width: 4px;
}
.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}
.custom-scrollbar::-webkit-scrollbar-thumb {
  background: var(--white-a20);
  border-radius: 2px;
}
.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: var(--white-a30);
}
</style>
