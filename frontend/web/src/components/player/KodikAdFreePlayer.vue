<template>
  <div class="kodik-adfree-player">
    <!-- Loading state for translations -->
    <div v-if="loadingTranslations" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- No translations available -->
    <div v-else-if="translations.length === 0 && !loadingTranslations" class="text-center py-20 text-white/60">
      <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
      </svg>
      {{ $t('player.noTranslations') || 'Нет доступных озвучек' }}
    </div>

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
              <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin mx-auto mb-3" />
              <p class="text-white/60 text-sm">{{ $t('player.loadingEpisode', { n: selectedEpisode }) }}</p>
            </div>
          </div>

          <!-- Stream extract error overlay -->
          <div
            v-if="streamError && !loadingStream"
            class="absolute inset-0 z-10 flex items-center justify-center bg-black/80 p-6"
          >
            <div class="text-center space-y-4 max-w-sm">
              <svg class="w-12 h-12 mx-auto text-destructive" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
              <p class="text-destructive text-sm font-medium">{{ $t('player.kodikAdfree.extractError') }}</p>
              <button
                data-testid="report-button"
                class="inline-flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-destructive/20 hover:bg-destructive/30 text-destructive border border-destructive/40 transition-colors"
                @click="reportStreamError"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 21v-4m0 0V5a2 2 0 012-2h6.5l1 1H21l-3 6 3 6h-8.5l-1-1H5a2 2 0 00-2 2zm9-13.5V9" />
                </svg>
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
              <svg class="w-16 h-16 mx-auto mb-3" fill="currentColor" viewBox="0 0 24 24">
                <path d="M8 5v14l11-7z" />
              </svg>
              <p>{{ $t('player.selectVoice') }}</p>
            </div>
          </div>
        </div>

        <!-- Episodes below player -->
        <div class="mt-4">
          <div class="flex items-center gap-3 mb-3 flex-wrap">
            <h3 class="text-white/60 text-sm flex items-center gap-2">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
              </svg>
              {{ $t('player.episodesCount', { count: episodeRange.length }) }}
            </h3>
            <slot name="header-middle" />
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
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
            </svg>
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
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
            </svg>
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
                        <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                          <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                        </svg>
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
                    <svg class="w-4 h-4 text-black" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
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
                <svg class="w-4 h-4" :fill="tr.pinned ? 'currentColor' : 'none'" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" />
                </svg>
              </button>
            </div>
          </template>
          <div v-else class="text-center py-8 text-white/40">
            <p>{{ translationType === 'voice' ? $t('player.noVoiceActing') : $t('player.noSubtitlesAvailable') }}</p>
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
import { useI18n } from 'vue-i18n'
import Hls from 'hls.js'
import { kodikApi, userApi } from '@/api/client'
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
import type { WatchCombo } from '@/types/preference'

const { t } = useI18n()

// ── Props (mirror KodikPlayer.vue exactly) ──────────────────────────────────
const props = defineProps<{
  animeId: string
  animeName?: string
  totalEpisodes?: number
  initialEpisode?: number
  preferredCombo?: WatchCombo | null
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

// ── HLS + stream state (mirrored from RawPlayer) ────────────────────────────
const videoRef = ref<HTMLVideoElement | null>(null)
let hls: Hls | null = null

// Remember the active selection so we can re-extract once if the signed CDN
// URL expires mid-session (spec §5).
let current: { episode: number; translationID: number } | null = null
let reloadedOnce = false

const streamError = ref(false)

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
    hls.on(Hls.Events.MANIFEST_PARSED, () => { v.play().catch(() => {}) })
    hls.on(Hls.Events.ERROR, (_e, data) => {
      if (!data.fatal) return
      // Expired signed CDN URL -> re-extract a fresh stream once, then give up.
      if (!reloadedOnce && current) {
        reloadedOnce = true
        void loadStream(current.episode, current.translationID)
      } else {
        streamError.value = true
      }
    })
  } else if (v.canPlayType('application/vnd.apple.mpegurl')) {
    v.src = proxyUrl
    v.addEventListener('loadedmetadata', () => { v.play().catch(() => {}) }, { once: true })
  }
}

// ── playWithIntro (Task 9, from plan verbatim) ───────────────────────────────
function playWithIntro(streamUrl: string, referer: string, episodeKey: string) {
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

// ── loadStream (from plan, verbatim) ─────────────────────────────────────────
async function loadStream(episode: number, translationID: number) {
  streamError.value = false
  // Reset the one-shot retry budget only on a NEW selection, never on the
  // retry itself (which re-calls loadStream with the same ep/translation).
  const changed = !current || current.episode !== episode || current.translationID !== translationID
  if (changed) reloadedOnce = false
  current = { episode, translationID }
  try {
    const resp = await kodikApi.getStream(props.animeId, episode, translationID)
    const data = resp.data?.data ?? resp.data
    playWithIntro(data.stream_url, data.referer, `${translationID}:${episode}`)
  } catch {
    streamError.value = true
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
        translation_title: tr.title
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

// ── Translation UI handlers ──────────────────────────────────────────────────
function setTranslationType(type: 'voice' | 'subtitles') {
  if (translationType.value === type) return
  translationType.value = type
}

function selectTranslation(translationId: number) {
  if (selectedTranslation.value === translationId) return

  selectedTranslation.value = translationId
  const trans = translations.value.find(t => t.id === translationId)
  if (trans?.episodes_count && selectedEpisode.value > trans.episodes_count) {
    selectedEpisode.value = 1
  }
  void loadStream(selectedEpisode.value, translationId)
}

function selectEpisode(episode: number) {
  if (selectedEpisode.value === episode) return
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
  void fetchTranslations()
  void refreshWatched()
})

onBeforeUnmount(() => {
  disposePlayer()
  if (skipTimer) { clearTimeout(skipTimer); skipTimer = null }
})

watch(() => props.animeId, () => {
  selectedEpisode.value = 1
  streamError.value = false
  error.value = null
  introPlaying.value = false
  showSkip.value = false
  if (skipTimer) { clearTimeout(skipTimer); skipTimer = null }
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
  background: rgba(255, 255, 255, 0.2);
  border-radius: 2px;
}
.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.3);
}
</style>
