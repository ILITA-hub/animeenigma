<template>
  <div class="kodik-player">
    <!-- Loading state for translations -->
    <div v-if="loadingTranslations" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- No translations available -->
    <div v-else-if="translations.length === 0 && !loadingTranslations" class="text-center py-20 text-white/60">
      <Video class="size-12 mx-auto mb-3 opacity-50" aria-hidden="true" />
      {{ $t('player.noTranslations') || 'Нет доступных озвучек' }}
    </div>

    <!-- Main content when translations available -->
    <div v-else class="flex flex-col lg:flex-row gap-4">
      <!-- Left: Video Player -->
      <div class="flex-1 min-w-0">
        <div class="relative aspect-video bg-black rounded-xl overflow-hidden">
          <!-- Loading overlay -->
          <div
            v-if="loadingVideo"
            class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
          >
            <div class="text-center">
              <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin mx-auto mb-3" />
              <p class="text-white/60 text-sm">{{ $t('player.loadingEpisode', { n: selectedEpisode }) }}</p>
            </div>
          </div>

          <!-- Phase 3 (03.4): Watch Together fallback banner. Shown when the
               boot-time RPC probe timed out — outbound sync is disabled but
               inbound progress save still works (banner is informational). -->
          <div
            v-if="props.room && kodikSyncAvailable === false"
            class="absolute top-2 left-2 right-2 z-20 rounded-md bg-warning/90 text-black px-3 py-2 text-sm font-medium pointer-events-auto"
            role="status"
            aria-live="polite"
          >
            {{ t('watch_together.kodik_sync_unavailable') }}
          </div>

          <!-- Iframe player -->
          <iframe
            v-if="embedUrl"
            ref="playerFrame"
            :src="embedUrl"
            class="absolute inset-0 w-full h-full"
            frameborder="0"
            allowfullscreen
            allow="autoplay; fullscreen; encrypted-media"
          />

          <!-- Placeholder when no video loaded yet -->
          <div
            v-else-if="!loadingVideo"
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
              v-for="t in filteredTranslations"
              :key="t.id"
              class="relative group"
            >
              <button
                @click="selectTranslation(t.id)"
                class="w-full text-left p-3 rounded-lg transition-all"
                :class="[
                  selectedTranslation === t.id
                    ? (translationType === 'voice' ? 'bg-success/20 border border-success/50' : 'bg-info/20 border border-info/50')
                    : 'bg-white/5 border border-transparent hover:bg-white/10',
                  t.pinned ? 'ring-1 ring-warning/30' : ''
                ]"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <!-- Pinned badge -->
                      <span
                        v-if="t.pinned"
                        class="inline-flex items-center gap-1 text-xs px-1.5 py-0.5 rounded bg-warning/20 text-warning"
                        :title="$t('player.recommendedVoice')"
                      >
                        <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                          <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                        </svg>
                      </span>
                      <p class="text-white font-medium truncate" :title="t.title">{{ t.title }}</p>
                    </div>
                    <span class="text-white/40 text-xs">{{ t.episodes_count || 1 }} {{ $t('player.episodeShort') }}</span>
                  </div>
                  <div
                    v-if="selectedTranslation === t.id"
                    class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                    :class="translationType === 'voice' ? 'bg-success' : 'bg-info'"
                  >
                    <Check class="size-4 text-black" aria-hidden="true" />
                  </div>
                </div>
              </button>

              <!-- Pin/Unpin button -->
              <button
                @click.stop="togglePin(t)"
                class="absolute top-2 right-2 p-1.5 rounded-lg transition-all opacity-0 group-hover:opacity-100"
                :class="t.pinned
                  ? 'bg-warning/20 text-warning hover:bg-warning/30'
                  : 'bg-white/10 text-white/40 hover:bg-white/20 hover:text-white'"
                :title="t.pinned ? $t('player.unpin') : $t('player.pin')"
              >
                <svg class="w-4 h-4" :fill="t.pinned ? 'currentColor' : 'none'" stroke="currentColor" viewBox="0 0 24 24">
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

    <!-- Error message -->
    <div v-if="error" class="mt-4 p-4 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400">
      {{ error }}
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, toRef, onMounted, onUnmounted } from 'vue'
import { Video, Play, List, Check, Mic2, MessageSquare } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { kodikApi, userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useOverrideTracker } from '@/composables/useOverrideTracker'
import { useWatchSession } from '@/composables/useWatchSession'
import { setPreferredWatchType, getPreferredWatchType } from '@/composables/useWatchPreferences'
import { findRecentClick, emitRecWatched } from '@/utils/recsAnalytics'
import type { WatchCombo } from '@/types/preference'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'

// Watch progress tracking
const currentTime = ref(0)
const maxTime = ref(0) // Track max reached time as approximate duration
const lastSaveTime = ref(0)
const SAVE_INTERVAL = 30 // Save every 30 seconds of playback
const AUTO_MARK_THRESHOLD = 20 * 60 // Auto-mark as watched after 20 minutes (1200 seconds)
const authStore = useAuthStore()
const { t } = useI18n()

// Mark episode as watched
const markingWatched = ref(false)
const episodeMarkedWatched = ref(false)

const emit = defineEmits<{
  (e: 'progress', data: { episode: number; time: number; maxTime: number }): void
  (e: 'episodeWatched', data: { episode: number }): void
  (e: 'availableTranslations', combos: WatchCombo[]): void
}>()

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

// Template ref for the Kodik iframe. Phase 3 (03.4) — used by postCommand
// to drive the undocumented kodik_player_api RPC.
const playerFrame = ref<HTMLIFrameElement | null>(null)

// ── Phase 3 (03.4): Watch Together sync state ──
// kodikSyncAvailable is null while we're determining readiness, true once the
// kodik_player_api RPC is confirmed live, false only after we've exhausted the
// readiness window. When false: outbound sync from this client is suppressed
// and a fallback banner renders. Inbound time_update/pause are still consumed
// for the pre-Phase-3 progress-save path regardless.
//
// 2026-06-02 fix (Kodik kodikplayer.com bundle): the RPC is intact, but the
// `get_time` reply (kodik_player_time) only fires once window.player.api +
// flowApi.video exist — i.e. AFTER the pre-roll ad finishes and real playback
// begins. The original probe (get_time at +500ms, give up at 2s) raced the ad
// and produced a FALSE "sync unavailable" banner. Fix: treat ANY genuine
// playback signal the bundle emits autonomously (time_update / video_started /
// play / pause) as proof the RPC channel is alive, and keep the get_time probe
// as a fast-path only. See bundle handler `app.player_single.*.js`
// (`if(window.player && window.player.api){...get_time...}`).
const kodikSyncAvailable = ref<boolean | null>(null)
let bootProbeTimer: ReturnType<typeof setTimeout> | null = null

// Flip sync to available exactly once, cancelling the pending probe-timeout so
// it can't later stomp us back to false. Safe to call from any inbound signal.
function markKodikSyncReady() {
  if (bootProbeTimer !== null) {
    clearTimeout(bootProbeTimer)
    bootProbeTimer = null
  }
  if (kodikSyncAvailable.value !== true) {
    kodikSyncAvailable.value = true
  }
}
// Re-emission guard: when we post a command to the iframe, Kodik echoes
// the corresponding outbound event. Without this guard we'd loop the same
// play/pause/seek back to the room.
let applyingRemote = false

/**
 * Send a command to the Kodik iframe via the undocumented `kodik_player_api`
 * postMessage RPC. Discovered 2026-05-25 — see memory reference
 * `reference_kodik_inbound_postmessage_api.md`.
 *
 * @param method  One of 'play', 'pause', 'seek', 'volume', 'speed', 'mute',
 *                'unmute', 'enter_pip', 'exit_pip', 'get_time'.
 * @param payload Method-specific args merged into the value envelope. For
 *                seek: { seconds }. For volume: { volume }. For speed: { speed }.
 */
function postCommand(method: string, payload?: Record<string, unknown>): void {
  const frame = playerFrame.value
  if (!frame || !frame.contentWindow) return
  frame.contentWindow.postMessage(
    { key: 'kodik_player_api', value: { method, ...payload } },
    '*',
  )
}

// Handle Kodik postMessage events
const handleKodikMessage = (event: MessageEvent) => {
  // Filter only Kodik messages
  if (!event.origin.includes('kodik') && !event.origin.includes('aniqit')) return

  try {
    const data = typeof event.data === 'string' ? JSON.parse(event.data) : event.data

    // Handle time update: { key: "kodik_player_time_update", value: 1331 }
    if (data.key === 'kodik_player_time_update' && typeof data.value === 'number') {
      // The bundle only emits time_update once real playback is running, which
      // means window.player.api is live — i.e. the RPC channel works. This is
      // the reliable readiness signal the early get_time probe was racing.
      if (props.room) markKodikSyncReady()
      currentTime.value = data.value

      // Track max time reached (approximate duration)
      if (data.value > maxTime.value) {
        maxTime.value = data.value
      }

      // Emit progress event
      emit('progress', {
        episode: selectedEpisode.value,
        time: data.value,
        maxTime: maxTime.value
      })

      // Save progress every SAVE_INTERVAL seconds
      if (data.value - lastSaveTime.value >= SAVE_INTERVAL) {
        lastSaveTime.value = data.value
        saveProgressLocal(props.animeId, selectedEpisode.value, data.value)
        saveProgressServer(props.animeId, selectedEpisode.value, data.value)
      }

      // Auto-mark as watched after 20 minutes
      if (authStore.isAuthenticated &&
          !episodeMarkedWatched.value &&
          data.value >= AUTO_MARK_THRESHOLD) {
        autoMarkEpisodeWatched()
      }

      // Phase 3 (03.4): feed Watch Together's time_tick channel so the room
      // can drive drift correction. Suppressed if we're applying a remote
      // event right now (Kodik often emits a time_update immediately after
      // a seek echo).
      if (props.room && kodikSyncAvailable.value === true && !applyingRemote) {
        props.room.emitTimeTick(data.value)
      }
    }

    // Handle pause event (save immediately)
    if (data.key === 'kodik_player_pause') {
      saveProgressLocal(props.animeId, selectedEpisode.value, currentTime.value)
      saveProgressServer(props.animeId, selectedEpisode.value, currentTime.value)

      // Phase 3 (03.4): also surface pause to the room so the other members
      // pause. Guarded by applyingRemote so we don't echo an inbound pause.
      if (props.room && kodikSyncAvailable.value === true && !applyingRemote) {
        props.room.emitPause(currentTime.value)
      }
    }

    // ── Phase 3 (03.4): Watch Together outbound consumption ──
    // Fast-path readiness: reply to the boot-time get_time probe. (Kept as an
    // optimization; time_update above is the dependable signal once the ad ends.)
    if (data.key === 'kodik_player_time' && typeof data.value === 'number') {
      if (props.room) markKodikSyncReady()
      return
    }

    // video_started is the bundle's "real playback began" signal — another
    // proof the RPC channel is live. Mark ready BEFORE the guard below so a
    // first user action right after the ad isn't dropped.
    if (data.key === 'kodik_player_video_started') {
      if (props.room) markKodikSyncReady()
      // No outbound emit in v1 (diagnostic); fall through to the guard.
    }

    // The rest of the Phase 3 branches only emit when in a room with
    // outbound sync enabled and we're not currently applying a remote event.
    if (!props.room || kodikSyncAvailable.value !== true || applyingRemote) return

    if (data.key === 'kodik_player_play') {
      props.room.emitPlay(currentTime.value)
      return
    }
    if (data.key === 'kodik_player_seek') {
      const t = typeof data.value === 'number' ? data.value : currentTime.value
      props.room.emitSeek(t)
      return
    }
    if (data.key === 'kodik_player_video_ended') {
      // Diagnostic only. v1: no emit. Future: could feed a playback-session overlay.
      return
    }

  } catch {
    // Ignore parse errors
  }
}

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

const props = defineProps<{
  animeId: string
  animeName?: string
  totalEpisodes?: number
  initialEpisode?: number
  preferredCombo?: WatchCombo | null
  // Phase 2 (02.7) — room prop accepted, sync wiring lands in Phase 3.
  room?: WatchTogetherRoomHandle | null
}>()
// Phase 3 (03.4): props.room is now consumed by handleKodikMessage + the
// onMounted boot probe + the onPlaybackEvent/onCorrection subscriptions
// below. The Phase-2 no-op shim is no longer needed.

const translations = ref<KodikTranslation[]>([])
const pinnedIds = ref<Set<number>>(new Set())
const selectedTranslation = ref<number | null>(null)
const selectedEpisode = ref(1)
const embedUrl = ref<string | null>(null)
const loadingTranslations = ref(false)
const loadingVideo = ref(false)
const error = ref<string | null>(null)
const isInitialized = ref(false)
const translationType = ref<'voice' | 'subtitles'>('voice')
// userHasOverridden flips true once the user explicitly clicks a translation
// or language toggle, freezing out any late-arriving preferredCombo. Without
// this, a slow resolve could undo the user's explicit pick.
const userHasOverridden = ref(false)
// usedPreferredCombo is true when fetchTranslations matched the props prop on
// the initial auto-pick. When false, the late-combo watcher is allowed to
// re-select once preferredCombo arrives from the parent's async resolve().
const usedPreferredCombo = ref(false)

// Filtered and sorted translations by type (pinned first)
const voiceTranslations = computed(() => {
  const voices = translations.value.filter(t => t.type === 'voice')
  return sortByPinned(voices)
})

const subtitleTranslations = computed(() => {
  const subs = translations.value.filter(t => t.type !== 'voice')
  return sortByPinned(subs)
})

const filteredTranslations = computed(() =>
  translationType.value === 'voice' ? voiceTranslations.value : subtitleTranslations.value
)

const currentCombo = computed((): WatchCombo | null => {
  if (!selectedTranslation.value) return null
  const tr = translations.value.find(t => t.id === selectedTranslation.value)
  if (!tr) return null
  return {
    player: 'kodik',
    language: 'ru',
    watch_type: translationType.value === 'voice' ? 'dub' : 'sub',
    translation_id: String(tr.id),
    translation_title: tr.title
  }
})

// Sort translations: pinned first, then by title
function sortByPinned(list: KodikTranslation[]): KodikTranslation[] {
  return [...list].sort((a, b) => {
    if (a.pinned && !b.pinned) return -1
    if (!a.pinned && b.pinned) return 1
    return a.title.localeCompare(b.title)
  })
}

const episodeRange = computed(() => {
  // Use episode count from selected translation if available
  const selectedTrans = translations.value.find(t => t.id === selectedTranslation.value)
  const count = selectedTrans?.episodes_count || props.totalEpisodes || 12
  return Array.from({ length: count }, (_, i) => i + 1)
})

const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const episodeOptions = computed<EpisodeOption[]>(() =>
  episodeRange.value.map((n) => ({ key: n, label: n, number: n })),
)

const fetchPinnedTranslations = async () => {
  try {
    const response = await kodikApi.getPinnedTranslations(props.animeId)
    const data = response.data?.data || response.data || []
    pinnedIds.value = new Set(data.map((p: PinnedTranslation) => p.translation_id))
  } catch {
    // Ignore errors, pinned translations are optional
    pinnedIds.value = new Set()
  }
}

const fetchTranslations = async () => {
  loadingTranslations.value = true
  error.value = null
  isInitialized.value = false

  try {
    // Fetch pinned translations first
    await fetchPinnedTranslations()

    const response = await kodikApi.getTranslations(props.animeId)
    const data = response.data?.data || response.data
    const rawTranslations: KodikTranslation[] = Array.isArray(data) ? data : []

    // Mark pinned translations
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
          usedPreferredCombo.value = true
        }
      }

      if (!autoSelected) {
        // Default fallback. Honor the user's last-used watch_type from
        // localStorage so the very first pick on a fresh anime doesn't force
        // dub on a sub viewer (and vice-versa). The late-combo watcher below
        // will still correct the pick once /api/preferences/resolve returns.
        const voices = translations.value.filter(t => t.type === 'voice')
        const subs = translations.value.filter(t => t.type !== 'voice')
        const pinnedVoice = voices.find(t => t.pinned)
        const pinnedSub = subs.find(t => t.pinned)
        const preferSubs = getPreferredWatchType() === 'sub'

        const pickVoice = () => {
          if (pinnedVoice) {
            translationType.value = 'voice'
            selectedTranslation.value = pinnedVoice.id
            return true
          }
          if (voices.length > 0) {
            translationType.value = 'voice'
            selectedTranslation.value = voices[0].id
            return true
          }
          return false
        }
        const pickSub = () => {
          if (pinnedSub) {
            translationType.value = 'subtitles'
            selectedTranslation.value = pinnedSub.id
            return true
          }
          if (subs.length > 0) {
            translationType.value = 'subtitles'
            selectedTranslation.value = subs[0].id
            return true
          }
          return false
        }

        if (preferSubs) {
          if (!pickSub()) pickVoice()
        } else {
          if (!pickVoice()) pickSub()
        }
      }

      // Persist the chosen watch_type so future fresh-anime loads honor it
      // before the server-side resolve returns. Cheap localStorage write.
      setPreferredWatchType(translationType.value === 'voice' ? 'dub' : 'sub')

      // Auto-load first video after setting translation
      isInitialized.value = true
      await loadVideo()
    }
  } catch (err: unknown) {
    const e = err as { response?: { data?: { message?: string } } }
    error.value = e.response?.data?.message || t('player.error.loadTranslations')
    translations.value = []
  } finally {
    loadingTranslations.value = false
  }
}

const loadVideo = async () => {
  if (!selectedTranslation.value) return

  loadingVideo.value = true
  error.value = null

  try {
    const response = await kodikApi.getVideo(props.animeId, selectedEpisode.value, selectedTranslation.value)
    const data = response.data?.data || response.data
    if (data?.embed_link) {
      // Add parameters to hide Kodik's built-in controls
      let url = data.embed_link
      const separator = url.includes('?') ? '&' : '?'
      // Hide translation selector and episode list in Kodik player
      url += `${separator}hide_selectors=true&only_season=true`
      embedUrl.value = url
    } else {
      error.value = t('player.error.videoNotFound')
    }
  } catch (err: unknown) {
    const e = err as { response?: { data?: { message?: string } } }
    error.value = e.response?.data?.message || t('player.error.loadVideo')
  } finally {
    loadingVideo.value = false
  }
}

// Override tracker: emits POST /api/preferences/override on first user-initiated
// pick within 30s of resolvedCombo apply. Per (load_session_id, dimension).
// Auto-advance and programmatic picks must NOT route through these wrapped
// handlers — Kodik has none today (selectTranslation/selectEpisode are only
// called from template @click), but a `setTranslationType` helper is added
// below to wrap the language toggle template clicks.
const tracker = useOverrideTracker({
  animeId: props.animeId,
  player: 'kodik',
  resolvedCombo: toRef(props, 'preferredCombo'),
  currentEpisode: selectedEpisode,
})

// Phase 5 (G-04-lite): playback session correlation. Kodik is an iframe — we
// can't read currentTime, so the drop-off beacon is unwired here (no useful
// position to report). session_id still flows so backend can group heartbeats
// from the same Kodik playback even though we don't get accurate seconds.
const { sessionId, newSession } = useWatchSession()
watch(selectedEpisode, () => newSession())

const setTranslationType = (type: 'voice' | 'subtitles') => {
  if (translationType.value === type) return
  // 'language' dimension is the sub-vs-dub axis here (D-07 enumerates the set).
  tracker.recordPickerEvent('language', { watch_type: type === 'voice' ? 'dub' : 'sub' })
  translationType.value = type
  userHasOverridden.value = true
  setPreferredWatchType(type === 'voice' ? 'dub' : 'sub')
}

const selectTranslation = (translationId: number) => {
  if (selectedTranslation.value === translationId) return

  // Phase 4 WT-STATE-04: when mounted inside a Watch Together room,
  // route the user click through the room handle so the backend can
  // validate and broadcast to all members. The room:state_changed
  // broadcast will reactively update room.translation_id, which flows
  // back through the existing programmatic path.
  if (props.room) {
    props.room.emitChangeTranslation(String(translationId))
    return
  }

  // Look up the translation title for the override new_combo before mutating state.
  const tr = translations.value.find(t => t.id === translationId)
  tracker.recordPickerEvent('team', {
    translation_title: tr?.title ?? String(translationId),
    player: 'kodik',
  })

  selectedTranslation.value = translationId
  userHasOverridden.value = true
  if (tr) {
    setPreferredWatchType(tr.type === 'voice' ? 'dub' : 'sub')
  }

  // Check if current episode exceeds available episodes for this translation
  const trans = translations.value.find(t => t.id === translationId)
  if (trans?.episodes_count && selectedEpisode.value > trans.episodes_count) {
    selectedEpisode.value = 1
  }

  loadVideo()
}

// WT-STATE-04: react to room episode broadcasts (own echo or another
// member's change). Apply locally WITHOUT re-emitting — selectEpisode's
// room guard returns early on click, so this watcher is what actually
// loads the new episode in a Watch Together room. Mirrors the AnimeLib/
// OurEnglish inbound watchers.
watch(
  () => props.initialEpisode,
  (epNum) => {
    if (!props.room || epNum == null || selectedEpisode.value === epNum) return
    selectedEpisode.value = epNum
    if (selectedTranslation.value) loadVideo()
  },
)

const selectEpisode = (episode: number) => {
  if (selectedEpisode.value === episode) return

  // Phase 4 WT-STATE-04: when mounted inside a Watch Together room,
  // route the user click through the room handle so the backend can
  // validate and broadcast to all members. The room:state_changed
  // broadcast will reactively update room.episode_id, which flows
  // back through the existing programmatic path.
  if (props.room) {
    props.room.emitChangeEpisode(String(episode))
    return
  }

  tracker.recordPickerEvent('episode', { episode })

  // Save progress of current episode before switching
  if (currentTime.value > 0) {
    saveProgressLocal(props.animeId, selectedEpisode.value, currentTime.value)
    saveProgressServer(props.animeId, selectedEpisode.value, currentTime.value)
  }

  // Reset progress tracking for new episode
  currentTime.value = 0
  maxTime.value = 0
  lastSaveTime.value = 0
  // Check if new episode is already watched
  episodeMarkedWatched.value = episode <= watchedUpTo.value

  selectedEpisode.value = episode
  if (selectedTranslation.value) {
    loadVideo()
  }
}

// Mark current episode as watched
const markCurrentEpisodeWatched = async () => {
  if (!authStore.isAuthenticated || markingWatched.value) return

  markingWatched.value = true
  try {
    await userApi.markEpisodeWatched(props.animeId, selectedEpisode.value, currentCombo.value ?? undefined, sessionId.value)
    episodeMarkedWatched.value = true
    await refreshWatched()
    emit('episodeWatched', { episode: selectedEpisode.value })
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
  } catch {
    // Silent fail for auto-mark
  }
}

const togglePin = async (translation: KodikTranslation) => {
  try {
    if (translation.pinned) {
      await kodikApi.unpinTranslation(props.animeId, translation.id)
      pinnedIds.value.delete(translation.id)
    } else {
      await kodikApi.pinTranslation(props.animeId, translation.id, translation.title, translation.type)
      pinnedIds.value.add(translation.id)
    }

    // Update the pinned status in the translations list
    translations.value = translations.value.map(t => ({
      ...t,
      pinned: pinnedIds.value.has(t.id)
    }))
  } catch (err: unknown) {
    const e = err as { response?: { data?: { message?: string } } }
    error.value = e.response?.data?.message || t('player.error.pinTranslation')
  }
}

// Save progress before leaving
const saveBeforeLeave = () => {
  if (currentTime.value > 0) {
    saveProgressLocal(props.animeId, selectedEpisode.value, currentTime.value)
    // Use sendBeacon for reliable save on page close
    if (authStore.isAuthenticated && navigator.sendBeacon) {
      const data = JSON.stringify({
        anime_id: props.animeId,
        episode_number: selectedEpisode.value,
        progress: Math.floor(currentTime.value),
        duration: Math.floor(maxTime.value) || null,
        ...currentCombo.value
      })
      navigator.sendBeacon('/api/users/progress', new Blob([data], { type: 'application/json' }))
    }
  }
}

// Reset when anime changes
watch(() => props.animeId, () => {
  // Save current progress before switching anime
  saveBeforeLeave()

  embedUrl.value = null
  selectedEpisode.value = 1
  currentTime.value = 0
  maxTime.value = 0
  lastSaveTime.value = 0
  episodeMarkedWatched.value = false
  userHasOverridden.value = false
  usedPreferredCombo.value = false
  fetchTranslations()
  void refreshWatched()
})

// Late-arriving preferredCombo. When /api/preferences/resolve returns after
// fetchTranslations already defaulted, this watcher re-selects the matching
// translation so the user sees their preferred sub/dub axis instead of the
// hardcoded default. Gated on: same player, no user override yet, no prior
// preferredCombo apply, translations already loaded.
watch(() => props.preferredCombo, async (newCombo) => {
  if (!newCombo || newCombo.player !== 'kodik') return
  if (userHasOverridden.value) return
  if (usedPreferredCombo.value) return
  if (!isInitialized.value) return
  if (translations.value.length === 0) return

  const match = translations.value.find(
    t => String(t.id) === newCombo.translation_id || t.title === newCombo.translation_title,
  )
  if (!match) return
  if (selectedTranslation.value === match.id) return

  translationType.value = match.type === 'voice' ? 'voice' : 'subtitles'
  selectedTranslation.value = match.id
  usedPreferredCombo.value = true
  setPreferredWatchType(match.type === 'voice' ? 'dub' : 'sub')
  await loadVideo()
})

// ── Phase 3 (03.4): Watch Together sync unsubscribers ──
// Held outside onMounted so onUnmounted can call them.
let unsubPlayback: (() => void) | null = null
let unsubCorrection: (() => void) | null = null
let probeKickoffTimer: ReturnType<typeof setTimeout> | null = null

onMounted(() => {
  // Listen for Kodik postMessage events
  window.addEventListener('message', handleKodikMessage)
  window.addEventListener('beforeunload', saveBeforeLeave)

  if (props.initialEpisode) {
    selectedEpisode.value = props.initialEpisode
  }
  fetchTranslations()
  void refreshWatched()

  // ── Phase 3 (03.4): Watch Together sync wiring (only when in a room). ──
  if (props.room) {
    // Readiness window — verify the kodik_player_api RPC is alive in the
    // currently-loaded Kodik bundle. The RPC dispatcher only services get_time
    // once `window.player.api` + `flowApi.video` exist, which is AFTER the
    // pre-roll ad finishes and real playback starts (can be 15-30s). So we
    // POLL get_time every 3s rather than give up after one 2s timeout, and we
    // ALSO flip ready on any autonomous playback signal (handled in
    // handleKodikMessage via markKodikSyncReady). Only after the full window
    // elapses with zero signal do we show the "sync unavailable" banner.
    //
    // This replaces the original single-shot probe that raced the ad and
    // produced a false-positive banner on the kodikplayer.com bundle
    // (diagnosed 2026-06-02 — RPC intact, probe just fired too early).
    const READINESS_WINDOW_MS = 30000
    const PROBE_INTERVAL_MS = 3000
    const readinessDeadline = Date.now() + READINESS_WINDOW_MS
    const pollProbe = () => {
      bootProbeTimer = null
      if (kodikSyncAvailable.value !== null) return // already resolved (ready or torn down)
      postCommand('get_time')
      if (Date.now() >= readinessDeadline) {
        // Window exhausted with no playback signal at all → genuinely no RPC.
        kodikSyncAvailable.value = false
        return
      }
      bootProbeTimer = setTimeout(pollProbe, PROBE_INTERVAL_MS)
    }
    // First probe shortly after mount; the iframe ignores commands pre-boot,
    // but the autonomous time_update/video_started signals will mark us ready
    // independently once playback begins.
    probeKickoffTimer = setTimeout(pollProbe, 800)

    // Subscribe to room playback events — translate to RPC commands.
    unsubPlayback = props.room.onPlaybackEvent((e) => {
      if (kodikSyncAvailable.value !== true) return
      applyingRemote = true
      try {
        if (e.kind === 'play') {
          // Realign before play to avoid an audible "starts behind" glitch.
          if (Math.abs(currentTime.value - e.time) > 1) {
            postCommand('seek', { seconds: e.time })
          }
          postCommand('play')
        } else if (e.kind === 'pause') {
          postCommand('pause')
          postCommand('seek', { seconds: e.time })
        } else if (e.kind === 'seek') {
          postCommand('seek', { seconds: e.time })
        }
      } finally {
        // Release the guard after Kodik's echo events have had time to fire.
        // 300ms is enough headroom for the iframe's event-loop hop without
        // trapping subsequent legitimate events from the local viewer.
        setTimeout(() => { applyingRemote = false }, 300)
      }
    })

    // Per-recipient drift correction (WT-SYNC-06). Soft = speed nudge,
    // hard = silent seek. No UI feedback per spec.
    unsubCorrection = props.room.onCorrection((c) => {
      if (kodikSyncAvailable.value !== true) return
      const target = c.time + (Date.now() - c.server_ts) / 1000
      const drift = Math.abs(currentTime.value - target)
      if (drift < 1.0) {
        postCommand('speed', { speed: currentTime.value < target ? 1.03 : 0.97 })
        setTimeout(() => postCommand('speed', { speed: 1.0 }), 5000)
      } else {
        applyingRemote = true
        postCommand('seek', { seconds: target })
        setTimeout(() => { applyingRemote = false }, 300)
      }
    })
  }
})

onUnmounted(() => {
  // Save progress when component unmounts
  saveBeforeLeave()

  window.removeEventListener('message', handleKodikMessage)
  window.removeEventListener('beforeunload', saveBeforeLeave)

  // Phase 3 (03.4): tear down Watch Together sync wiring.
  if (probeKickoffTimer !== null) {
    clearTimeout(probeKickoffTimer)
    probeKickoffTimer = null
  }
  if (bootProbeTimer !== null) {
    clearTimeout(bootProbeTimer)
    bootProbeTimer = null
  }
  if (unsubPlayback) {
    unsubPlayback()
    unsubPlayback = null
  }
  if (unsubCorrection) {
    unsubCorrection()
    unsubCorrection = null
  }
})
</script>

<style scoped>
.kodik-player {
  --player-accent: #06b6d4;
  --player-accent-rgb: 6, 182, 212;
  width: 100%;
}

.accent-bg { background-color: var(--player-accent); }
.accent-bg-hover:hover { background-color: color-mix(in srgb, var(--player-accent), black 15%); }
/* UA-036: lightened text mix keeps contrast ≥4.5:1 over accent-bg-muted */
.accent-text { color: color-mix(in srgb, var(--player-accent), white 40%); }
.accent-border { border-color: var(--player-accent); }
.accent-bg-muted { background-color: rgba(var(--player-accent-rgb), 0.28); }
.accent-ring { --tw-ring-color: rgba(var(--player-accent-rgb), 0.5); }

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
