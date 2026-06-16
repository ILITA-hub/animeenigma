<template>
  <div
    ref="rootRef"
    class="pl"
    :class="{ 'pl--theater': theater, 'pl--ui-hidden': !uiVisible }"
    :style="{ '--prov': activeProviderHue }"
    tabindex="0"
    role="region"
    aria-label="Video player. Space to play or pause, arrow keys to seek and adjust volume."
    @click.self="closeMenus"
    @mouseenter="onPointerEnter"
    @mouseleave="onPointerLeave"
    @mousemove="wakeUi"
    @touchstart.passive="wakeUi"
    data-test="unified-player"
  >
    <!-- Poster / still background — only until playback first starts; a
         mid-episode pause must NOT bring the poster back (disruptive in
         fullscreen where object-contain letterboxing exposes it). -->
    <div
      v-if="anime.still && !hasStarted"
      class="pl-scene"
      :style="{ backgroundImage: `url(${anime.still})` }"
      aria-hidden="true"
    />
    <div class="pl-grain" aria-hidden="true" />

    <!-- Video element -->
    <video
      ref="videoRef"
      class="absolute inset-0 w-full h-full object-contain z-[1]"
      playsinline
      preload="auto"
      @play="onVideoPlay"
      @pause="onVideoPause"
      @ended="onEnded"
      @click="onVideoClick"
      @volumechange="onVolumeChange"
      @waiting="onBufferStart"
      @canplay="onBufferEnd"
      @playing="onBufferEnd"
      @seeked="onSeeked"
      @timeupdate="onTimeUpdate"
      @progress="onVideoProgress"
      @error="onVideoError"
    />

    <!-- Subtitle overlay -->
    <SubtitleOverlay
      :video-element="videoRef"
      :subtitle-url="chosenSubUrl"
      :format="chosenSubFormat"
      :visible="state.subLang.value !== 'off' && !!chosenSubUrl"
      :fullscreen-container="rootRef"
      :windowed-container="rootRef"
      :offset="state.subOffset.value"
    />

    <!-- Source error overlay -->
    <div
      v-if="sourceError"
      class="absolute inset-0 z-[2] flex items-center justify-center"
      style="background: rgba(0,0,0,0.72);"
    >
      <div class="flex flex-col items-center gap-3 text-center px-8">
        <CircleAlert :size="48" :stroke-width="1.5" class="text-muted-foreground" aria-hidden="true" />
        <p class="text-sm font-medium text-foreground">{{ sourceError }}</p>
        <button
          class="px-4 py-2 rounded-md text-sm font-semibold text-foreground"
          style="background: rgba(255,255,255,0.1);"
          @click="retryResolution"
        >
          Retry
        </button>
      </div>
    </div>

    <!-- Top bar -->
    <div class="pl-top" @click.stop>
      <!-- Title block -->
      <div class="pl-title-block">
        <span class="pl-eyebrow">
          <span class="pl-eyebrow-src">
            <!-- V2b: the EP block IS the episodes-sheet trigger -->
            <button
              type="button"
              class="pl-ep-trigger"
              :aria-expanded="openMenu === 'episodes'"
              aria-label="Episode list"
              title="Episodes"
              data-test="ep-trigger"
              @click="toggleMenu('episodes')"
            >
              EP {{ selectedEpisode?.number ?? anime.ep }}
              <span v-if="selectedEpisode?.title" class="pl-ep-title">· {{ selectedEpisode.title }}</span>
              <ChevronDown
                class="pl-ep-chev"
                :class="{ 'pl-ep-chev--open': openMenu === 'episodes' }"
                :size="12"
                :stroke-width="2.5"
                aria-hidden="true"
              />
            </button>
            <span v-if="activeProviderName" class="inline-flex items-center gap-1 ml-1">
              <span class="pl-prov-dot" :style="{ background: activeProviderHue, boxShadow: `0 0 8px ${activeProviderHue}` }" aria-hidden="true" />
              {{ activeProviderName }}
            </span>
            <span v-if="audioLabel" class="ml-1 opacity-70">· {{ audioLabel }}</span>
          </span>
        </span>
        <h1 class="pl-title">{{ anime.title }}</h1>
      </div>

      <!-- Top-right actions -->
      <div class="pl-top-right">
        <WatchTogetherButton />
      </div>
    </div>

    <!-- Overlays -->
    <ResumePill :kind="resumeKind" />

    <BigPlayButton
      :visible="!state.playing.value && !sourceError && !showBuffering && !isResolving"
      @play="togglePlay"
    />

    <BufferingOverlay :visible="(showBuffering || isResolving) && !sourceError" />

    <DebugHud
      v-if="hudVisible"
      :stats="playbackStats"
      :frags="engine.fragStats.value"
      :bandwidth="engine.bandwidthEstimate.value"
      :provider="activeProviderName"
      :stream-type="currentStream?.type ?? '—'"
      :level-label="engine.currentLevelLabel.value"
      :seek="lastSeek"
      :intents="sourceFallbackDebug.intents"
      :pinned="state.hudPinned.value"
      :fading="hudFading"
      @update:pinned="v => { state.hudPinned.value = v }"
    />

    <SkipIntroChip
      :visible="!!skipTarget"
      :label="skipTarget?.kind === 'outro' ? 'Skip Outro' : 'Skip Intro'"
      @skip="onSkipSegment"
    />

    <!-- Resume-from-saved-position chip (never auto-seeks) -->
    <div v-if="resumeChipVisible" class="pl-resume" data-test="resume-chip">
      <button class="pl-resume-go" type="button" @click="onResumeFromSaved">
        <Play :size="12" :stroke-width="2.5" aria-hidden="true" />
        <span>Resume from {{ fmtResume(resumePosSec) }}</span>
      </button>
      <button
        class="pl-resume-x"
        type="button"
        aria-label="Dismiss resume offer"
        data-test="resume-chip-dismiss"
        @click="resumeChipDismissed = true"
      >
        <X :size="12" aria-hidden="true" />
      </button>
    </div>

    <NextEpisodeCard
      v-if="showNextEpisode"
      :next-ep="anime.ep + 1"
      :title="anime.title"
      :still-url="anime.still"
      :countdown="nextEpCountdown"
      @play="goToNextEpisode"
      @cancel="showNextEpisode = false"
    />

    <!-- Control bar -->
    <PlayerControlBar
      :playing="state.playing.value"
      :current-time="currentTime"
      :duration="duration"
      :volume="state.volume.value"
      :muted="state.muted.value"
      :provider-name="activeProviderName"
      :provider-hue="activeProviderHue"
      :audio-label="audioLabel"
      :episode-label="selectedEpisode?.number ?? anime.ep"
      :progress="state.progress.value"
      :buffered="bufferedPct"
      :chapters="chapters"
      :still-url="anime.still"
      :open-menu="openMenu"
      :fragments="fragOverlay"
      :preview-url="currentStream?.url ?? null"
      :preview-type="currentStream?.type ?? null"
      @toggle-play="togglePlay"
      @seek-rel="onSeekRel"
      @seek="onSeek"
      @set-volume="onSetVolume"
      @toggle-mute="onToggleMute"
      @toggle-source="toggleMenu('source')"
      @toggle-episodes="toggleMenu('episodes')"
      @toggle-subs="toggleMenu('subs')"
      @toggle-settings="toggleMenu('settings')"
      @toggle-pip="onTogglePip"
      @toggle-fullscreen="onToggleFullscreen"
    />

    <!-- Source panel (floating, top-right) -->
    <div v-if="openMenu === 'source'" ref="sourceMenuEl" class="pl-floating pl-floating--source" @click.stop>
      <SourcePanel
        :rows="rows"
        :audio="state.combo.value.audio"
        :lang="state.combo.value.lang"
        :team="state.combo.value.team"
        :provider="state.combo.value.provider"
        :server="state.combo.value.server"
        :servers="resolvedServers"
        :teams="teams"
        :cap-map="capMap"
        :ranked-ids="orderedProviderIds"
        :hacker-mode="state.hackerMode.value"
        :playback-error="Boolean(sourceError)"
        @update:audio="state.setAudio"
        @update:lang="state.setLang"
        @update:team="state.setTeam"
        @select-provider="onSelectProvider"
        @select-server="state.setServer"
      />
    </div>

    <!-- Episodes sheet (V2b — bottom sheet above the control bar) -->
    <div v-if="openMenu === 'episodes'" ref="episodesMenuEl" class="pl-floating pl-floating--sheet" @click.stop>
      <EpisodesPanel
        :episodes="episodes"
        :selected-number="selectedEpisode?.number ?? null"
        :watched-up-to="watchedUpTo"
        :progress="epProgress"
        :can-mark="auth.isAuthenticated"
        :marking="tracking.marking.value"
        :marked="selectedEpisode ? isEpisodeWatched(selectedEpisode.number) : false"
        @select="onSelectEpisode"
        @mark-watched="onMarkWatched"
      />
    </div>

    <!-- Playback settings menu (floating, above control bar) -->
    <div v-if="openMenu === 'settings'" ref="settingsMenuEl" class="pl-floating pl-floating--btnmenu" @click.stop>
      <PlaybackSettingsMenu
        :quality="state.quality.value"
        :qualities="qualities"
        :quality-display="qualityDisplay"
        :speed="state.speed.value"
        :speeds="[0.75, 1, 1.25, 1.5, 2]"
        :auto-next="state.autoNext.value"
        :auto-skip="state.autoSkip.value"
        :hacker-mode="state.hackerMode.value"
        :debug-stats="debugStats"
        @update:quality="onSetQuality"
        @update:speed="onSetSpeed"
        @update:auto-next="v => { state.autoNext.value = v }"
        @update:auto-skip="v => { state.autoSkip.value = v }"
        @update:hacker-mode="v => { state.hackerMode.value = v }"
      />
    </div>

    <!-- Subtitles menu (floating, above control bar) -->
    <div v-if="openMenu === 'subs'" ref="subsMenuEl" class="pl-floating pl-floating--btnmenu" @click.stop>
      <SubtitlesMenu
        :sub-lang="state.subLang.value"
        :sub-langs="subLangsAvailable"
        :hardsub-note="hardsubNote"
        :sub-size="state.subSize.value"
        :sub-bg="state.subBg.value"
        :sub-offset="state.subOffset.value"
        @update:sub-lang="v => { state.subLang.value = v }"
        @update:sub-size="v => { state.subSize.value = v }"
        @update:sub-bg="v => { state.subBg.value = v }"
        @update:sub-offset="v => { state.subOffset.value = v }"
        @open-browse="browseOpen = true"
      />
    </div>

    <!-- Browse subtitles modal -->
    <BrowseSubsModal
      v-if="browseOpen"
      :tracks="[]"
      :selected-url="chosenSubUrl"
      @click.stop
      @select="onSelectSubTrack"
      @close="browseOpen = false"
    />
  </div>
</template>

<script setup lang="ts">
import {
  ref,
  computed,
  watch,
  onMounted,
  onUnmounted,
} from 'vue'
import { onClickOutside } from '@vueuse/core'
import { CircleAlert, ChevronDown, Play, X } from 'lucide-vue-next'

import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useViewerContextStore } from '@/stores/viewerContext'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
import SubtitleOverlay from '@/components/player/SubtitleOverlay.vue'
import ResumePill from '@/components/player/ResumePill.vue'
import PlayerControlBar from './PlayerControlBar.vue'
import SourcePanel from './SourcePanel.vue'
import EpisodesPanel from './EpisodesPanel.vue'
import PlaybackSettingsMenu from './PlaybackSettingsMenu.vue'
import SubtitlesMenu from './SubtitlesMenu.vue'
import BrowseSubsModal from './BrowseSubsModal.vue'
import BigPlayButton from './overlays/BigPlayButton.vue'
import BufferingOverlay from './overlays/BufferingOverlay.vue'
import DebugHud, { type SeekTrace } from './overlays/DebugHud.vue'
import SkipIntroChip from './overlays/SkipIntroChip.vue'
import NextEpisodeCard from './overlays/NextEpisodeCard.vue'
import WatchTogetherButton from './overlays/WatchTogetherButton.vue'

import { useSkipTimes } from '@/composables/useSkipTimes'
import { usePlayerState } from '@/composables/unifiedPlayer/usePlayerState'
import { usePlaybackStats } from '@/composables/unifiedPlayer/usePlaybackStats'
import { scrubDebug } from '@/composables/unifiedPlayer/scrubPreviewDebug'
import {
  sourceFallbackDebug,
  recordFallbackIntent,
  resetFallbackIntents,
} from '@/composables/unifiedPlayer/sourceFallbackDebug'
import { segmentsToChapters, activeSkipSegment } from '@/composables/unifiedPlayer/skipSegments'
import { useVideoEngine } from '@/composables/unifiedPlayer/useVideoEngine'
import { useProviderResolver, KODIK_QUALITY_PREF_KEY } from '@/composables/unifiedPlayer/useProviderResolver'
import { useProviderHealth } from '@/composables/unifiedPlayer/useProviderHealth'
import { useWatchTracking } from '@/composables/unifiedPlayer/useWatchTracking'
import { mapKeyToAction } from '@/composables/unifiedPlayer/playerHotkeys'
import { providerById, CURATED_TIER } from './providerRegistry'
import { pickSmartDefault } from '@/composables/unifiedPlayer/smartDefault'
import { useCapabilities } from '@/composables/unifiedPlayer/useCapabilities'
import { rankedProviderIds } from '@/composables/unifiedPlayer/rankedProviderIds'
import { pickEpisodeForProvider } from '@/composables/unifiedPlayer/episodeSelection'
import { aeApi } from '@/api/client'
import { useWatchPreferences } from '@/composables/useWatchPreferences'
import { comboToWatchCombo, watchComboToPartialCombo, providerToLegacyPlayer } from '@/composables/unifiedPlayer/comboMapping'
import { useToast } from '@/composables/useToast'
import { recordPlayerEvent } from '@/utils/playerTelemetry'

import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { StreamResult } from '@/types/unifiedPlayer'
import type { WatchCombo } from '@/types/preference'

// ─── Types ───────────────────────────────────────────────────────────────────

interface SubTrack {
  url: string
  provider: string
  lang: string
  label: string
  format: string
}

// ─── Props / Emits ───────────────────────────────────────────────────────────

const props = defineProps<{
  animeId: string
  anime: { title: string; ep: number; eps: number; still?: string }
  theater: boolean
  isHentai?: boolean
  initialEpisode?: number
  /** Shikimori id (= MAL id) for AniSkip skip-times. Absent ⇒ no skip UI. */
  malId?: string | number
}>()

defineEmits<{
  (e: 'toggle-theater'): void
}>()

// ─── Core state ──────────────────────────────────────────────────────────────

const videoRef = ref<HTMLVideoElement | null>(null)
const rootRef = ref<HTMLElement | null>(null)
const isPointerInside = ref(false)
const state = usePlayerState()
const engine = useVideoEngine(videoRef)
const resolver = useProviderResolver()
const toast = useToast()

// ─── Provider health filter ───────────────────────────────────────────────────

const filter = computed(() => ({
  audio: state.combo.value.audio,
  lang: state.combo.value.lang,
  content: (props.isHentai ? 'hentai' : 'common') as 'hentai' | 'common',
}))

const { rows, start } = useProviderHealth(filter)

// Capability report → server ranking + sub/dub/quality/team labels for the
// Source panel. orderedProviderIds merges the capability ranking with the
// registry rows (ae/raw/18anime fall back to CURATED_TIER). Degrades to
// CURATED_TIER order when /capabilities is absent.
const animeIdRef = computed(() => props.animeId)
const { capMap, rankedIds: capRankedIds } = useCapabilities(animeIdRef)
const orderedProviderIds = computed(() =>
  rankedProviderIds(rows.value, capRankedIds.value, CURATED_TIER),
)

// ─── Provider defaults ────────────────────────────────────────────────────────

// First-party (ae) availability — cached single probe per mount. The library
// only has a subset of titles encoded, so `ae` (top of CURATED_TIER) must be
// skipped when this anime isn't on-prem. aeApi.getEpisodes returns
// { episodes, available }; treat available=false OR an empty list as "no".
let aeAvailableCache: Promise<boolean> | null = null
function isProviderAvailable(id: string): Promise<boolean> {
  if (id !== 'ae') return Promise.resolve(true)
  if (!aeAvailableCache) {
    aeAvailableCache = aeApi
      .getEpisodes(props.animeId)
      .then((resp) => {
        const data = resp.data?.data ?? resp.data
        return Boolean(data?.available) && (data?.episodes?.length ?? 0) > 0
      })
      .catch(() => false)
  }
  return aeAvailableCache
}

// props.animeId can change without a remount (no :key on the player), so the
// per-anime ae availability probe must be invalidated when the title changes.
// Also reset saved-combo fallback state so the new title gets a fresh attempt.
let providerWasFromSavedCombo = false
let savedSourceFallbackDone = false
watch(() => props.animeId, () => {
  aeAvailableCache = null
  providerWasFromSavedCombo = false
  savedSourceFallbackDone = false
  resetFallbackIntents()
})

// Providers whose default-selection eligibility needs a runtime availability
// probe (see isProviderAvailable). Only first-party `ae` today.
const AE_NEEDS_CHECK = new Set(['ae'])

// ─── Saved-combo restore (Stage 1b) ──────────────────────────────────────────
// Resolve the user's saved watch combo first; the Stage 1a smart default is
// gated on `preferenceSettled` so the saved pick always wins when present.

const preferenceSettled = ref(false)
const { resolve: resolvePreference, resolvedCombo, preferredScraperProvider } = useWatchPreferences(props.animeId)

function applyResolvedCombo() {
  const rc = resolvedCombo.value
  if (!rc || state.combo.value.provider) return
  const { audio, lang, team } = watchComboToPartialCombo(rc)
  let providerId: string | null = null
  if (rc.player === 'english') {
    providerId = preferredScraperProvider.value && rows.value.some(r => r.def.id === preferredScraperProvider.value && r.state === 'active')
      ? preferredScraperProvider.value
      : null
  } else {
    const match = rows.value.find(r => r.state === 'active' && providerToLegacyPlayer(r.def.id) === rc.player)
    providerId = match?.def.id ?? null
  }
  // setAudio/setLang each reset team → null, so setTeam must come AFTER them.
  state.setAudio(audio)
  state.setLang(lang)
  if (team) state.setTeam(team)
  if (providerId) {
    providerWasFromSavedCombo = true
    state.setProvider(providerId, '')
  }
}

// evaluated exactly once at first-active rows (resolveAttempted guards re-run)
const buildAvailable = (): WatchCombo[] => {
  const combos: WatchCombo[] = []
  const seen = new Set<string>()
  for (const r of rows.value) {
    if (r.state !== 'active') continue
    const player = providerToLegacyPlayer(r.def.id)
    if (!player) continue
    for (const audio of r.def.audios) {
      const key = `${player}:${audio}`
      if (seen.has(key)) continue
      seen.add(key)
      combos.push({
        player,
        language: (r.def.langs.includes(state.combo.value.lang) ? state.combo.value.lang : r.def.langs[0]) as WatchCombo['language'],
        watch_type: audio,
        translation_id: '',
        translation_title: '',
      })
    }
  }
  return combos
}

// one-shot latch (non-reactive on purpose — read/written only inside the watcher)
let resolveAttempted = false
watch(rows, () => {
  if (resolveAttempted) return
  const available = buildAvailable()
  if (available.length === 0) return
  resolveAttempted = true
  resolvePreference(available).finally(() => {
    applyResolvedCombo()
    preferenceSettled.value = true
  })
}, { immediate: true })

watch(
  [rows, preferenceSettled],
  () => {
    if (state.combo.value.provider) return
    if (!preferenceSettled.value) return // let the saved-combo restore go first
    void pickSmartDefault(rows.value, orderedProviderIds.value, {
      needsCheck: AE_NEEDS_CHECK,
      isAvailable: isProviderAvailable,
    }).then((id) => {
      // Guard against a race: only apply if still unset and the chosen row is
      // still active in the latest rows (filter may have changed mid-probe).
      if (id && !state.combo.value.provider &&
          rows.value.some((r) => r.def.id === id && r.state === 'active')) {
        state.setProvider(id, '')
      }
    })
  },
  { immediate: true },
)

// ─── Active provider display info ────────────────────────────────────────────

const activeProviderDef = computed(() =>
  providerById(state.combo.value.provider),
)

const activeProviderName = computed(
  () => activeProviderDef.value?.name ?? state.combo.value.provider ?? '',
)

const activeProviderHue = computed(
  () => activeProviderDef.value?.hue ?? '#00d4ff',
)

const audioLabel = computed(() => {
  const a = state.combo.value.audio
  return a === 'dub' ? 'DUB' : 'SUB'
})

// ─── Episode list + stream resolution ────────────────────────────────────────

const episodes = ref<EpisodeOption[]>([])
const selectedEpisode = ref<EpisodeOption | null>(null)

// ─── User watch data (read-only): watched marks + per-episode progress ───────

const auth = useAuthStore()
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const epProgress = ref<Record<number, { pct: number; sec: number; completed: boolean }>>({})

type ProgressRow = {
  episode_number?: number
  progress?: number
  duration?: number
  completed?: boolean
}

function progressRowsToMap(rows: ProgressRow[]) {
  const map: Record<number, { pct: number; sec: number; completed: boolean }> = {}
  for (const r of rows) {
    if (!r.episode_number) continue
    map[r.episode_number] = {
      pct: r.duration ? Math.min(1, (r.progress ?? 0) / r.duration) : 0,
      sec: r.progress ?? 0,
      completed: !!r.completed,
    }
  }
  return map
}

// Page-fetch optimization (2026-06-11): the FIRST load per anime consumes the
// viewer-context aggregate Anime.vue already fetched, killing the duplicate
// GET /users/progress/{id} on mount. Post-mutation reloads go to the network.
let progressPrefetchConsumedFor: string | null = null

async function loadEpisodeProgress() {
  if (!auth.isAuthenticated) {
    epProgress.value = {}
    return
  }
  if (progressPrefetchConsumedFor !== props.animeId) {
    progressPrefetchConsumedFor = props.animeId
    // whenLoaded (not forAnime): on deep-link mounts the aggregate request is
    // often still in flight — await it instead of duplicating the fetch.
    const ctx = await useViewerContextStore().whenLoaded(props.animeId)
    if (ctx) {
      epProgress.value = progressRowsToMap(ctx.progress ?? [])
      return
    }
  }
  try {
    const res = await userApi.getProgress(props.animeId)
    const rows = (res.data?.data ?? res.data ?? []) as ProgressRow[]
    epProgress.value = progressRowsToMap(rows)
  } catch {
    // 404 / anonymous / network — no user data, panel renders plain numbers
    epProgress.value = {}
  }
}

// ─── Watch-progress tracking (server + localStorage + auto-complete) ─────────

const tracking = useWatchTracking(
  () => props.animeId,
  () => selectedEpisode.value?.number ?? null,
  {
    onMarked: () => {
      void refreshWatched()
      void loadEpisodeProgress()
    },
  },
  () => comboToWatchCombo(state.combo.value),
)

/** Whether the user already has this episode marked watched (drawer data). */
function isEpisodeWatched(n: number): boolean {
  return n <= watchedUpTo.value || !!epProgress.value[n]?.completed
}

/** Manual mark from the episodes drawer (Kodik-parity button). */
function onMarkWatched() {
  void tracking.markWatched()
}

// ─── Resume-from-saved-position chip ─────────────────────────────────────────
// Saved position for the current episode: server watch_progress first (logged
// in), localStorage fallback (anonymous parity with KodikPlayer). The chip
// offers the jump — it never auto-seeks.

const resumeChipDismissed = ref(false)
const resumeChipUsed = ref(false)

function localResumeSec(ep: number): number {
  try {
    const data = JSON.parse(localStorage.getItem(`watch_progress:${props.animeId}`) || '{}')
    return Number(data[ep]?.time) || 0
  } catch {
    return 0
  }
}

const resumePosSec = computed(() => {
  const ep = selectedEpisode.value?.number
  if (!ep) return 0
  const server = epProgress.value[ep]
  if (server && !server.completed && server.sec > 0) return server.sec
  if (!auth.isAuthenticated) return localResumeSec(ep)
  return 0
})

const resumeChipVisible = computed(() => {
  if (resumeChipDismissed.value || resumeChipUsed.value) return false
  if (sourceError.value || isResolving.value) return false
  const pos = resumePosSec.value
  if (pos < 30) return false // too little progress to bother
  // Once near the end the next-episode flow takes over instead.
  if (duration.value > 0 && pos >= 0.95 * duration.value) return false
  // The offer expires once the user has clearly chosen to watch from here.
  if (hasStarted.value && currentTime.value > 5) return false
  return true
})

function fmtResume(s: number): string {
  const m = Math.floor(s / 60)
  const sec = Math.floor(s % 60)
  return `${m}:${sec.toString().padStart(2, '0')}`
}

function onResumeFromSaved() {
  const v = videoRef.value
  if (!v) return
  resumeChipUsed.value = true
  v.currentTime = resumePosSec.value
  if (v.paused) void v.play().catch(() => {})
  writeProgress()
}
const sourceError = ref<string | null>(null)
const resolvedServers = ref<{ id: string; label: string }[]>([])
const teams = ref<string[]>([])
const currentStream = ref<StreamResult | null>(null)
const isResolving = ref(false)

// Monotonically-increasing request token — only the latest resolve applies.
// Prevents a stale audio/lang/server re-resolve from clobbering a concurrent
// provider-change full re-list+resolve that started after it.
let resolveToken = 0

// ─── Telemetry timing state ───────────────────────────────────────────────────
// Best-effort; never influences playback logic.
let resolveStartedAt = 0
let reachedReported = false
let stallStartedAt = 0

// ─── Quality ladder ──────────────────────────────────────────────────────────
// HLS: data-driven from hls.js levels. MP4: only when the provider returned
// multiple URL-valued qualities. Per-URL HLS (Kodik: one manifest per quality,
// numeric values): switching re-resolves the stream instead of changing an
// hls.js level. Single-variant streams stay Auto-only (D-05).
// NOTE: declared after currentStream — watch() below evaluates its computed
// source eagerly during setup.

const mp4Qualities = computed(() => {
  const s = currentStream.value
  if (!s || s.type !== 'mp4' || !s.qualities) return []
  return s.qualities.filter(
    (q) => typeof q.value === 'string' && /^(https?:|\/)/.test(q.value as string),
  )
})

const perUrlHlsQualities = computed(() => {
  const s = currentStream.value
  if (!s || s.type !== 'hls' || !s.qualities) return []
  return s.qualities.filter((q) => typeof q.value === 'number')
})

const qualities = computed(() => {
  if (mp4Qualities.value.length > 1) return ['Auto', ...mp4Qualities.value.map((q) => q.label)]
  if (perUrlHlsQualities.value.length > 1) {
    return ['Auto', ...perUrlHlsQualities.value.map((q) => q.label)]
  }
  return ['Auto', ...engine.levels.value.map((l) => l.label)]
})

// While auto-switching, show what is actually playing: hls.js's current level,
// or for per-URL ladders the quality the provider reported serving.
const qualityDisplay = computed(() => {
  const served = engine.currentLevelLabel.value || currentStream.value?.qualityLabel
  return state.quality.value === 'Auto' && served
    ? `Auto · ${served}`
    : state.quality.value
})

// New stream may not offer the previously-chosen quality — snap back to Auto.
// If it DOES offer it, re-apply: each load() creates a fresh hls instance
// that starts at auto, so a pinned level must be re-pinned. (Per-URL ladders
// need no re-apply — the resolved URL already carries the pinned quality.)
watch(qualities, (qs) => {
  if (!qs.includes(state.quality.value)) {
    state.quality.value = 'Auto'
  } else if (
    state.quality.value !== 'Auto' &&
    mp4Qualities.value.length === 0 &&
    perUrlHlsQualities.value.length === 0
  ) {
    engine.setLevel(state.quality.value)
  }
})

function swapMp4Source(url: string) {
  const v = videoRef.value
  if (!v) return
  const t = v.currentTime
  const wasPlaying = !v.paused
  v.addEventListener(
    'loadedmetadata',
    () => {
      v.currentTime = t
      if (wasPlaying) void v.play()
    },
    { once: true },
  )
  v.src = url
}

function onSetQuality(q: string) {
  state.quality.value = q
  const mq = mp4Qualities.value.find((x) => x.label === q)
  if (mq) {
    swapMp4Source(mq.value as string)
    return
  }
  if (q === 'Auto' && currentStream.value?.type === 'mp4') {
    // mp4 has no auto ladder — Auto = the originally-resolved URL
    swapMp4Source(currentStream.value.url)
    return
  }
  if (perUrlHlsQualities.value.length > 0) {
    // Per-URL ladder (Kodik): persist the choice (the adapter reads it on the
    // next resolve), then re-resolve the stream at the new quality in place.
    const pq = perUrlHlsQualities.value.find((x) => x.label === q)
    if (pq) localStorage.setItem(KODIK_QUALITY_PREF_KEY, String(pq.value))
    else if (q === 'Auto') localStorage.removeItem(KODIK_QUALITY_PREF_KEY)
    void reResolveAtPosition()
    return
  }
  engine.setLevel(q)
}

// Re-resolve the current episode's stream, restoring playback position and
// play state — used for per-URL quality switches where the new quality lives
// at a different manifest URL.
async function reResolveAtPosition() {
  const v = videoRef.value
  const t = v?.currentTime ?? 0
  const wasPlaying = v ? !v.paused : false
  await resolveStreamForCurrentEpisode()
  const v2 = videoRef.value
  if (!v2 || t <= 0) return
  const restore = () => {
    v2.currentTime = t
    if (wasPlaying) void v2.play().catch(() => {})
  }
  if (v2.readyState >= 1) restore()
  else v2.addEventListener('loadedmetadata', restore, { once: true })
}

// Initialize selectedEpisode from initialEpisode or anime.ep
function initSelectedEpisode() {
  const targetEp = props.initialEpisode ?? props.anime.ep ?? 1
  // Look up in existing list or create a synthetic placeholder
  const found = episodes.value.find((e) => e.number === targetEp)
  if (found) {
    selectedEpisode.value = found
  } else if (episodes.value.length > 0) {
    selectedEpisode.value = episodes.value[0]
  } else {
    // Synthetic placeholder for when episode list isn't loaded yet
    selectedEpisode.value = { key: targetEp, label: targetEp, number: targetEp }
  }
}

async function loadEpisodesAndStream() {
  const provider = state.combo.value.provider
  if (!provider) return

  sourceError.value = null
  isResolving.value = true
  hasStarted.value = false
  const token = ++resolveToken
  resolveStartedAt = performance.now()
  reachedReported = false
  stallStartedAt = 0

  try {
    // Load episode list
    const eps = await resolver.listEpisodes(provider, props.animeId)
    if (token !== resolveToken) return // superseded by a later request

    episodes.value = eps

    // Provider-native teams (e.g. Kodik translation titles) for the Source
    // panel. Best-effort — never blocks the stream resolve.
    teams.value = [] // clear stale chips immediately on provider switch
    resolver
      .listTeams(provider, props.animeId)
      .then((t) => { if (token === resolveToken) teams.value = t })
      .catch(() => { if (token === resolveToken) teams.value = [] })

    // Preserve the selected episode across provider changes: keep the same
    // episode NUMBER when the new source has it, and never snap back to EP 1
    // when it doesn't (pickEpisodeForProvider handles the nearest-fallback).
    const targetNum =
      selectedEpisode.value?.number ?? props.initialEpisode ?? props.anime.ep ?? 1
    const ep = pickEpisodeForProvider(eps, targetNum, selectedEpisode.value)

    if (!ep) {
      sourceError.value = 'No episodes available from this source'
      return
    }

    selectedEpisode.value = ep

    // Resolve stream
    const stream = await resolver.resolveStream(
      provider,
      props.animeId,
      ep,
      state.combo.value,
    )

    if (token !== resolveToken) return // superseded

    resolvedServers.value = stream.servers ?? []
    // Set BEFORE the await: a superseded resolve must never clobber the
    // winner's stream descriptor after resuming from engine.load.
    currentStream.value = stream
    await engine.load(stream)
  } catch (err: unknown) {
    if (token !== resolveToken) return // superseded
    const isNotAvailable =
      err instanceof Error && err.name === 'NotAvailableError'
    // Telemetry: resolve failure (best-effort, never throws)
    recordPlayerEvent({
      kind: 'resolve',
      provider: state.combo.value.provider,
      anime_id: props.animeId,
      episode: selectedEpisode.value?.number,
      outcome: 'fail',
      reached_playback: false,
      error_kind: isNotAvailable ? 'not_available' : 'stream_error',
      latency_ms: resolveStartedAt ? Math.round(performance.now() - resolveStartedAt) : undefined,
      audio: state.combo.value.audio,
      lang: state.combo.value.lang,
    })
    if (isNotAvailable) {
      if (!savedSourceFallbackDone && providerWasFromSavedCombo) {
        savedSourceFallbackDone = true
        const failed = state.combo.value.provider
        const next = await pickSmartDefault(
          rows.value.filter((r) => r.def.id !== failed),
          orderedProviderIds.value,
          { needsCheck: AE_NEEDS_CHECK, isAvailable: isProviderAvailable },
        )
        // Hacker mode: DON'T auto-switch. Record what the resolver would have
        // done so the fallback choice can be inspected manually before it's
        // trusted to act. With hacker mode off, the switch is performed and the
        // same record is written with acted=true.
        const willSwitch = !state.hackerMode.value && !!next
        recordFallbackIntent({
          from: failed,
          to: next,
          reason: 'saved source unavailable',
          acted: willSwitch,
        })
        if (willSwitch) {
          toast.push("The source you watched last time isn't available right now — switching.", 'info', 5000)
          providerWasFromSavedCombo = false
          state.setProvider(next as string, '') // provider watcher re-runs loadEpisodesAndStream
          return
        }
        if (state.hackerMode.value) {
          sourceError.value = next
            ? `Hacker mode: auto-switch suppressed — would switch ${failed} → ${next} (saved source unavailable). Pick a source manually.`
            : `Hacker mode: auto-switch suppressed — saved source "${failed}" unavailable, no fallback candidate.`
        }
      }
      if (!sourceError.value) sourceError.value = "This source isn't available yet"
    } else {
      sourceError.value = 'Stream unavailable'
    }
  } finally {
    if (token === resolveToken) {
      isResolving.value = false
    }
  }
}

// Provider change → full re-list + resolve (episodes are provider-specific)
watch(
  () => state.combo.value.provider,
  (newProvider) => {
    if (newProvider) {
      void loadEpisodesAndStream()
    }
  },
)

// Audio / language / server / team change, or episode selection → re-resolve stream
// only (no need to re-list episodes). The token inside resolveStreamForEpisode
// ensures a concurrent provider-change full-resolve wins if they race.
// Skip when loadEpisodesAndStream is in-flight: it sets selectedEpisode itself
// and will call resolveStream at the end — we must not fire a duplicate.
watch(
  () => [
    state.combo.value.audio,
    state.combo.value.lang,
    state.combo.value.server,
    state.combo.value.team,
    selectedEpisode.value,
  ] as const,
  (newVal, oldVal) => {
    // Skip the very first run (oldVal is undefined on initial watch fire)
    if (oldVal === undefined) return
    // Skip if a full re-list is already in progress (provider changed)
    if (isResolving.value) return
    // Skip an EPISODE change while the episode list hasn't loaded: that's the
    // mount-time null→synthetic-placeholder transition from
    // initSelectedEpisode, whose key is a bare episode number — scraper
    // episode ids are opaque, so resolving it fires a doomed
    // scraper/servers?episode=<number> request (seen in prod HARs on
    // ?episode=N deep links). loadEpisodesAndStream reconciles the selection
    // and resolves the stream itself once the real list arrives. Combo
    // (audio/lang/server) changes are NOT gated on the list.
    if (newVal[4] !== oldVal[4] && episodes.value.length === 0) return
    void resolveStreamForCurrentEpisode()
  },
)

// ─── Provider selection helper ────────────────────────────────────────────────

function onSelectProvider(id: string) {
  providerWasFromSavedCombo = false
  state.setProvider(id, '')
  // loadEpisodesAndStream fires via the provider watcher above
}

// ─── Episode selection (episodes drawer) ─────────────────────────────────────
// Resolve DIRECTLY (mirrors goToNextEpisode) — the combo/episode watcher
// early-returns while isResolving and would silently swallow a click made
// during an in-flight resolve. resolveStreamForEpisode sets isResolving
// synchronously, so the watcher's deferred fire is deduped, and resolveToken
// arbitrates any race with the in-flight request.

function onSelectEpisode(ep: EpisodeOption) {
  openMenu.value = null
  if (selectedEpisode.value?.number === ep.number) return
  tracking.saveNow() // persist the outgoing episode's position
  selectedEpisode.value = ep
  tracking.resetEpisode(isEpisodeWatched(ep.number))
  resumeChipDismissed.value = false
  resumeChipUsed.value = false
  void resolveStreamForEpisode(ep)
}

// ─── Retry ───────────────────────────────────────────────────────────────────

function retryResolution() {
  sourceError.value = null
  void loadEpisodesAndStream()
}

// ─── rAF progress loop ───────────────────────────────────────────────────────

const currentTime = ref(0)
const duration = ref(0)
const bufferedPct = ref(0)
/** true once playback has started for the current stream — gates the poster */
const hasStarted = ref(false)
let rafId: number | null = null

function writeProgress() {
  const v = videoRef.value
  if (!v) return
  currentTime.value = v.currentTime
  const dur = v.duration || 0
  duration.value = dur
  if (dur > 0) {
    state.progress.value = (v.currentTime / dur) * 100
  }
  // Buffered
  if (v.buffered.length > 0 && dur > 0) {
    bufferedPct.value = (v.buffered.end(v.buffered.length - 1) / dur) * 100
  }
  // Watch tracking: heartbeat saves + duration-aware auto-complete. Only feed
  // real playback positions — a paused pre-start frame (currentTime 0) or a
  // dead source must not write progress.
  if (v.currentTime > 0) {
    tracking.onTick(v.currentTime, dur)
  }
}

function tick() {
  writeProgress()
  rafId = requestAnimationFrame(tick)
}

function startRaf() {
  if (rafId === null) {
    rafId = requestAnimationFrame(tick)
  }
}

function stopRaf() {
  if (rafId !== null) {
    cancelAnimationFrame(rafId)
    rafId = null
  }
  // One final write so progress/time are up-to-date while paused
  writeProgress()
}

function onVideoPlay() {
  state.playing.value = true
  startRaf()
  armUiIdleTimer()
}

function onVideoPause() {
  state.playing.value = false
  stopRaf() // final writeProgress() inside keeps tracking's lastKnown fresh
  tracking.saveNow()
  clearUiIdleTimer()
  uiVisible.value = true
}

// ─── Intro/outro skip (AniSkip via catalog proxy) ────────────────────────────

const epNumber = computed(() => selectedEpisode.value?.number ?? null)
const malIdRef = computed(() => props.malId ?? null)
const { opening, ending } = useSkipTimes(malIdRef, epNumber)

const chapters = computed(() =>
  segmentsToChapters(opening.value, ending.value, duration.value),
)

const skipTarget = computed(() =>
  activeSkipSegment(currentTime.value, opening.value, ending.value),
)

function onSkipSegment() {
  const v = videoRef.value
  const target = skipTarget.value
  if (!v || !target) return
  v.currentTime = target.end
  writeProgress()
}

// Auto-skip intro (settings toggle) — once per episode view so a manual
// seek back into the OP isn't fought.
let autoSkippedEp: number | null = null
watch(epNumber, () => {
  autoSkippedEp = null
})
watch(currentTime, (t) => {
  if (!state.autoSkip.value) return
  const op = opening.value
  if (!op) return
  const ep = epNumber.value
  if (ep === null || autoSkippedEp === ep) return
  if (t >= op.start && t < op.end - 1) {
    autoSkippedEp = ep
    const v = videoRef.value
    if (v) {
      v.currentTime = op.end
      writeProgress()
    }
  }
})

// ─── Buffering indicator ──────────────────────────────────────────────────────
// waiting/seeking → on; playing/canplay → off. A 150ms grace window keeps
// instant in-buffer seeks from flashing the ring. `seeked` only clears when
// the element actually has decodable data (readyState ≥ 3) — otherwise the
// following `waiting` keeps the ring up. We deliberately do NOT bind
// `stalled`: browsers fire it spuriously when the download is throttled
// because the buffer is FULL, and nothing would clear the ring during healthy
// playback. `timeupdate` self-heals any false positive.

const isBuffering = ref(false)
const showBuffering = ref(false)
let bufferingTimer: ReturnType<typeof setTimeout> | null = null

function setBuffering(on: boolean) {
  if (on === isBuffering.value) return
  isBuffering.value = on
  if (on) {
    bufferingTimer = setTimeout(() => {
      showBuffering.value = true
    }, 150)
  } else {
    if (bufferingTimer) {
      clearTimeout(bufferingTimer)
      bufferingTimer = null
    }
    showBuffering.value = false
  }
}

function onBufferStart() {
  setBuffering(true)
  // Telemetry: mid-playback stall — capture start time (only after first frame)
  if (reachedReported && !stallStartedAt) {
    stallStartedAt = performance.now()
  }
}

function onBufferEnd() {
  setBuffering(false)
  markSeekResumed()
  // Telemetry: stall resolved — emit duration (best-effort, never throws)
  if (stallStartedAt) {
    const stallMs = Math.round(performance.now() - stallStartedAt)
    stallStartedAt = 0
    recordPlayerEvent({
      kind: 'stall',
      provider: state.combo.value.provider,
      anime_id: props.animeId,
      episode: selectedEpisode.value?.number,
      stall_ms: stallMs,
    })
  }
}

function onSeeked() {
  const v = videoRef.value
  if (v && v.readyState >= 3) setBuffering(false)
  // Seek trace: decoder positioned at the target frame
  const s = lastSeek.value
  if (s && !s.done && s.seekedMs === null) {
    s.seekedMs = Math.round(performance.now() - s.t0)
  }
}

// Self-heal: if time is advancing with decodable data, we are NOT buffering.
function onTimeUpdate() {
  const v = videoRef.value
  // Drop the poster only once playback actually progresses — the bare `play`
  // event fires even on a dead source (readyState 0), and `timeupdate` also
  // fires on a seek-while-paused before the first play; both would swap the
  // poster for a black frame. Require real (unpaused) progress.
  if (!hasStarted.value && v && v.currentTime > 0 && !v.paused) {
    hasStarted.value = true
    // Telemetry: first real frame — resolve ok + reached_playback (best-effort)
    if (!reachedReported) {
      reachedReported = true
      recordPlayerEvent({
        kind: 'resolve',
        provider: state.combo.value.provider,
        anime_id: props.animeId,
        episode: selectedEpisode.value?.number,
        outcome: 'ok',
        reached_playback: true,
        latency_ms: resolveStartedAt ? Math.round(performance.now() - resolveStartedAt) : undefined,
        audio: state.combo.value.audio,
        lang: state.combo.value.lang,
      })
    }
  }
  if (isBuffering.value && v && v.readyState >= 3 && !v.seeking) {
    setBuffering(false)
  }
  if (v && v.readyState >= 3 && !v.seeking) {
    markSeekResumed()
  }
}

// A dead source must surface the error overlay, not an endless spinner.
// Covers the native/mp4 path (e.g. upstream CDN 404 → MEDIA_ERR_SRC_NOT_SUPPORTED).
function onVideoError() {
  const v = videoRef.value
  if (!v?.error || isResolving.value) return
  setBuffering(false)
  sourceError.value = 'Stream unavailable'
}

// Same for the hls.js path: the engine flags unrecoverable fatals.
watch(engine.fatal, (f) => {
  if (f) {
    setBuffering(false)
    sourceError.value = 'Stream unavailable'
    // Telemetry: HLS fatal (best-effort, never throws)
    recordPlayerEvent({
      kind: 'resolve',
      provider: state.combo.value.provider,
      anime_id: props.animeId,
      episode: selectedEpisode.value?.number,
      outcome: 'fail',
      reached_playback: reachedReported,
      error_kind: 'media_fatal',
      latency_ms: resolveStartedAt ? Math.round(performance.now() - resolveStartedAt) : undefined,
      audio: state.combo.value.audio,
      lang: state.combo.value.lang,
    })
  }
})

// ─── Hacker mode (debug HUD) ──────────────────────────────────────────────────

const statsEnabled = computed(() => state.hackerMode.value)
const { stats: playbackStats } = usePlaybackStats(videoRef, statsEnabled)

// Hacker mode also mirrors the scrub-preview engine log to the console
// (prefix [ScrubPreview]) — frontend pump health vs provider fetch latency.
watch(
  () => state.hackerMode.value,
  (v) => {
    scrubDebug.console = v
  },
  { immediate: true },
)

// Seek pipeline trace — what actually happens between releasing the scrubber
// and playback resuming: buffer check (hit = instant, no network), pipeline
// flush + segment fetch, decode from the nearest keyframe to the target
// (the `seeked` event), then buffer refill to readyState ≥ 3 (resume).
const lastSeek = ref<SeekTrace | null>(null)

function traceSeekStart(target: number) {
  if (!state.hackerMode.value) return
  const v = videoRef.value
  if (!v) return
  let hit = false
  for (let i = 0; i < v.buffered.length; i++) {
    if (target >= v.buffered.start(i) && target <= v.buffered.end(i)) {
      hit = true
      break
    }
  }
  lastSeek.value = {
    target,
    bufferHit: hit,
    t0: performance.now(),
    fetchMs: null,
    fetchedRange: null,
    seekedMs: null,
    resumeMs: null,
    frags: 0,
    bytes: 0,
    done: false,
  }
}

// Fetch-phase depth: the `progress` event fires as bytes arrive — the moment
// a buffered range covers the seek target, the network part of the seek is
// done (mp4: the ranged bytes landed; hls: the segment(s) arrived).
function onVideoProgress() {
  const s = lastSeek.value
  const v = videoRef.value
  if (!s || s.done || s.fetchMs !== null || !v) return
  for (let i = 0; i < v.buffered.length; i++) {
    const start = v.buffered.start(i)
    const end = v.buffered.end(i)
    if (s.target >= start && s.target <= end) {
      s.fetchMs = Math.round(performance.now() - s.t0)
      s.fetchedRange = [start, end]
      return
    }
  }
}

function markSeekResumed() {
  const s = lastSeek.value
  if (!s || s.done) return
  if (s.seekedMs === null) return // resume can't precede the seeked event
  s.resumeMs = Math.round(performance.now() - s.t0)
  s.done = true
}

// Count fragments fetched while a seek is in flight (hls path; FRAG_LOADED
// appends exactly one entry per fragment to the rolling fragStats window).
watch(engine.fragStats, (arr) => {
  const s = lastSeek.value
  if (!s || s.done) return
  const last = arr[arr.length - 1]
  if (!last) return
  s.frags++
  s.bytes += last.size
})

// HUD shows while paused or while actively buffering/seeking (or always when
// pinned). When the show-condition drops (playback resumed), the panel
// LINGERS ~1s, fades 0.4s, then unmounts — so the final seek numbers are
// readable instead of vanishing the instant video continues.
const hudCondition = computed(
  () =>
    state.hackerMode.value &&
    (state.hudPinned.value || !state.playing.value || showBuffering.value),
)
const hudVisible = ref(false)
const hudFading = ref(false)
let hudLingerTimer: ReturnType<typeof setTimeout> | null = null
let hudFadeTimer: ReturnType<typeof setTimeout> | null = null

function clearHudTimers() {
  if (hudLingerTimer) { clearTimeout(hudLingerTimer); hudLingerTimer = null }
  if (hudFadeTimer) { clearTimeout(hudFadeTimer); hudFadeTimer = null }
}

watch(hudCondition, (on) => {
  clearHudTimers()
  if (on) {
    hudVisible.value = true
    hudFading.value = false
    return
  }
  // linger 1s with full opacity, then 0.4s CSS fade, then unmount
  hudLingerTimer = setTimeout(() => {
    hudFading.value = true
    hudFadeTimer = setTimeout(() => {
      hudVisible.value = false
      hudFading.value = false
    }, 450)
  }, 1000)
}, { immediate: true })

// Scrub-bar heatmap segments — size-tinted (green <300KB, amber <1MB, red ≥1MB).
const fragOverlay = computed(() => {
  if (!state.hackerMode.value) return []
  const dur = duration.value
  if (!dur) return []
  return engine.fragStats.value.map((f) => ({
    startPct: (f.start / dur) * 100,
    widthPct: (f.duration / dur) * 100,
    tone: (f.size < 300_000 ? 'ok' : f.size < 1_000_000 ? 'warn' : 'bad') as 'ok' | 'warn' | 'bad',
    label: `${Math.round(f.size / 1024)} KB · ${Math.round(f.loadMs)} ms`,
  }))
})

// Compact line set for the settings-menu mini-stats section.
const debugStats = computed(() => {
  if (!state.hackerMode.value) return null
  const bwv = engine.bandwidthEstimate.value
  const frs = engine.fragStats.value
  const last = frs[frs.length - 1]
  return {
    bw: bwv > 0 ? `${(bwv / 1_000_000).toFixed(1)} Mbit/s` : '—',
    buffer: `+${playbackStats.value.bufferAheadSec.toFixed(1)}s / −${playbackStats.value.bufferBehindSec.toFixed(1)}s`,
    level:
      engine.currentLevelLabel.value ||
      (currentStream.value?.type === 'mp4' ? 'mp4' : '—'),
    frag: last ? `${Math.round(last.size / 1024)} KB · ${Math.round(last.loadMs)} ms` : '—',
  }
})

// ─── Next episode logic ───────────────────────────────────────────────────────

const showNextEpisode = ref(false)
const nextEpCountdown = ref(5)
let nextEpTimer: ReturnType<typeof setInterval> | null = null

function onEnded() {
  state.playing.value = false
  // Reaching the end IS a completed watch — mark even if the 90% tick raced.
  tracking.saveNow()
  void tracking.markWatched()
  if (anime_hasNextEp.value && state.autoNext.value) {
    startNextEpCountdown()
  }
}

const anime_hasNextEp = computed(
  () => props.anime.ep < props.anime.eps,
)

function startNextEpCountdown() {
  showNextEpisode.value = true
  nextEpCountdown.value = 5
  nextEpTimer = setInterval(() => {
    nextEpCountdown.value--
    if (nextEpCountdown.value <= 0) {
      clearNextEpTimer()
      goToNextEpisode()
    }
  }, 1000)
}

function clearNextEpTimer() {
  if (nextEpTimer) {
    clearInterval(nextEpTimer)
    nextEpTimer = null
  }
}

function goToNextEpisode() {
  showNextEpisode.value = false
  clearNextEpTimer()
  // Find next episode in list
  const current = selectedEpisode.value
  if (!current) return
  const idx = episodes.value.findIndex((e) => e.number === current.number)
  const next = episodes.value[idx + 1]
  if (next) {
    tracking.saveNow()
    selectedEpisode.value = next
    tracking.resetEpisode(isEpisodeWatched(next.number))
    resumeChipDismissed.value = false
    resumeChipUsed.value = false
    void resolveStreamForEpisode(next)
  }
}

async function resolveStreamForEpisode(ep: EpisodeOption) {
  const provider = state.combo.value.provider
  if (!provider) return
  sourceError.value = null
  isResolving.value = true
  hasStarted.value = false
  const token = ++resolveToken
  resolveStartedAt = performance.now()
  reachedReported = false
  stallStartedAt = 0
  try {
    const stream = await resolver.resolveStream(
      provider,
      props.animeId,
      ep,
      state.combo.value,
    )
    if (token !== resolveToken) return // superseded
    resolvedServers.value = stream.servers ?? []
    // Set BEFORE the await — see loadEpisodesAndStream.
    currentStream.value = stream
    await engine.load(stream)
  } catch (err: unknown) {
    if (token !== resolveToken) return // superseded
    const isNotAvailable =
      err instanceof Error && err.name === 'NotAvailableError'
    // Telemetry: resolve failure (best-effort, never throws)
    recordPlayerEvent({
      kind: 'resolve',
      provider: state.combo.value.provider,
      anime_id: props.animeId,
      episode: ep.number,
      outcome: 'fail',
      reached_playback: false,
      error_kind: isNotAvailable ? 'not_available' : 'stream_error',
      latency_ms: resolveStartedAt ? Math.round(performance.now() - resolveStartedAt) : undefined,
      audio: state.combo.value.audio,
      lang: state.combo.value.lang,
    })
    sourceError.value = isNotAvailable
      ? "This source isn't available yet"
      : 'Stream unavailable'
  } finally {
    if (token === resolveToken) {
      isResolving.value = false
    }
  }
}

// Re-resolve stream (without re-listing episodes) for the currently-selected
// episode when audio, lang, server, or selectedEpisode changes.
// Provider changes are handled separately by the provider watcher above
// (which does a full re-list + resolve) — we skip here when it's active.
async function resolveStreamForCurrentEpisode() {
  const ep = selectedEpisode.value
  if (!ep) return
  await resolveStreamForEpisode(ep)
}

// ─── Menu state ───────────────────────────────────────────────────────────────

type MenuKind = 'source' | 'settings' | 'subs' | 'episodes' | null
const openMenu = ref<MenuKind>(null)
const browseOpen = ref(false)

// One floating dropdown is open at a time (mutually-exclusive v-if), so the
// active element resolves from whichever menu `openMenu` selects.
const sourceMenuEl = ref<HTMLElement | null>(null)
const episodesMenuEl = ref<HTMLElement | null>(null)
const settingsMenuEl = ref<HTMLElement | null>(null)
const subsMenuEl = ref<HTMLElement | null>(null)
const activeMenuEl = computed<HTMLElement | null>(() => {
  switch (openMenu.value) {
    case 'source': return sourceMenuEl.value
    case 'episodes': return episodesMenuEl.value
    case 'settings': return settingsMenuEl.value
    case 'subs': return subsMenuEl.value
    default: return null
  }
})

// Click-outside dismiss: a click anywhere outside the open dropdown closes it.
// Ignore the trigger regions — the control bar (source/subs/settings pills) and
// top bar (EP trigger) own their own toggle, so letting click-outside fire there
// too would race the trigger and reopen-then-close. The <video> is also ignored
// because onVideoClick handles it (and suppresses the play/pause side effect);
// without this the pointerdown-phase click-outside would close the menu first,
// leaving onVideoClick's click to fall through to togglePlay.
onClickOutside(
  activeMenuEl,
  () => { if (openMenu.value !== null) closeMenus() },
  { ignore: ['.pl-controls', '.pl-top', videoRef] },
)

function toggleMenu(menu: MenuKind) {
  openMenu.value = openMenu.value === menu ? null : menu
  if (openMenu.value !== null) browseOpen.value = false
}

function closeMenus() {
  openMenu.value = null
  browseOpen.value = false
}

// ─── Controls auto-hide (idle while playing) ─────────────────────────────────
// Top bar + control bar fade out after UI_IDLE_MS of pointer inactivity while
// playing (matters most in fullscreen). Any pointer/keyboard activity, a pause,
// or an open menu brings them back and keeps them visible.

const UI_IDLE_MS = 2500
const uiVisible = ref(true)
let uiIdleTimer: ReturnType<typeof setTimeout> | null = null

function clearUiIdleTimer() {
  if (uiIdleTimer !== null) {
    clearTimeout(uiIdleTimer)
    uiIdleTimer = null
  }
}

function armUiIdleTimer() {
  clearUiIdleTimer()
  if (!state.playing.value || openMenu.value !== null) return
  uiIdleTimer = setTimeout(() => {
    uiVisible.value = false
  }, UI_IDLE_MS)
}

function wakeUi() {
  uiVisible.value = true
  armUiIdleTimer()
}

function onPointerEnter() {
  isPointerInside.value = true
  wakeUi()
}

function onPointerLeave() {
  isPointerInside.value = false
  // Pointer left the player while playing — hide right away (menus pin it)
  if (state.playing.value && openMenu.value === null) {
    clearUiIdleTimer()
    uiVisible.value = false
  }
}

watch(openMenu, (menu) => {
  if (menu !== null) {
    clearUiIdleTimer()
    uiVisible.value = true
  } else {
    armUiIdleTimer()
  }
})

// ─── Subtitles ────────────────────────────────────────────────────────────────

const chosenSub = ref<SubTrack | null>(null)

const chosenSubUrl = computed<string | null>(() => chosenSub.value?.url ?? null)
const chosenSubFormat = computed<'ass' | 'srt' | 'vtt' | null>(() => {
  const fmt = chosenSub.value?.format ?? null
  if (fmt === 'ass' || fmt === 'srt' || fmt === 'vtt') return fmt
  return null
})

// Real subtitle languages — only what's actually loaded as a soft track
// (the menu renders the "Off" option itself). Provider hardsubs are burned
// into the video and are NOT a selectable track, so a fresh stream offers
// nothing here until the user browses one in.
const subLangsAvailable = computed(() =>
  chosenSub.value ? [chosenSub.value.lang] : [],
)

// Informational note for the subs menu: when there's no soft track but the
// stream is a SUB cut, the subs the user sees are hardsubbed by the provider.
const hardsubNote = computed(() => {
  if (chosenSub.value) return null
  if (state.combo.value.audio !== 'sub') return null
  const prov = activeProviderName.value
  if (!prov) return null
  const langName =
    state.combo.value.lang === 'ru' ? 'Russian'
    : state.combo.value.lang === 'ja' ? 'Japanese'
    : 'English'
  return `${langName} subtitles are burned into the video by ${prov}`
})

function onSelectSubTrack(track: SubTrack) {
  chosenSub.value = track
  // Selecting a track turns the overlay on for that language
  state.subLang.value = track.lang
  browseOpen.value = false
}

// ─── Resume pill ─────────────────────────────────────────────────────────────

// Stage 1: static "first-time" — no persistent watch progress wired yet
const resumeKind = computed<
  'first-time' | 'watching' | 'finished' | 'not-yet-aired' | 'episode-not-loaded-yet'
>(() => 'first-time')

// ─── Playback helpers ─────────────────────────────────────────────────────────

// A tap on the video acts as a backdrop dismiss while any settings menu /
// modal is open — close it WITHOUT also toggling play/pause (the dismiss must
// not have a side effect). With nothing open it's the normal play/pause tap.
function onVideoClick() {
  if (openMenu.value !== null || browseOpen.value) {
    closeMenus()
    return
  }
  togglePlay()
}

function togglePlay() {
  const v = videoRef.value
  if (!v) return
  if (v.paused) {
    void v.play()
  } else {
    v.pause()
  }
}

function onSeekRel(delta: number) {
  const v = videoRef.value
  if (!v) return
  const target = Math.max(0, Math.min(isFinite(v.duration) ? v.duration : Infinity, v.currentTime + delta))
  traceSeekStart(target)
  v.currentTime = target
}

function onSeek(pct: number) {
  const v = videoRef.value
  if (!v || !v.duration) return
  const target = (pct / 100) * v.duration
  traceSeekStart(target)
  v.currentTime = target
  // Write progress immediately so the scrub bar reflects the new position
  // even while paused (rAF loop is stopped when paused).
  writeProgress()
}

function onSetVolume(vol: number) {
  state.volume.value = vol
  const v = videoRef.value
  if (v) v.volume = vol / 100
}

function onToggleMute() {
  state.muted.value = !state.muted.value
  const v = videoRef.value
  if (v) v.muted = state.muted.value
}

function onSetSpeed(speed: number) {
  state.speed.value = speed
  const v = videoRef.value
  if (v) v.playbackRate = speed
}

function onVolumeChange() {
  const v = videoRef.value
  if (!v) return
  // Sync state from element — covers PiP / media-session external changes.
  // Only write state here; the set-volume path writes to the element.
  state.volume.value = Math.round(v.volume * 100)
  state.muted.value = v.muted
}

function onTogglePip() {
  const v = videoRef.value
  if (!v) return
  if (document.pictureInPictureElement) {
    void document.exitPictureInPicture()
  } else {
    void v.requestPictureInPicture?.()
  }
}

function onToggleFullscreen() {
  const el = rootRef.value ?? videoRef.value?.parentElement
  if (!el) return
  if (document.fullscreenElement) {
    void document.exitFullscreen()
  } else {
    void el.requestFullscreen()
  }
}

// ─── Keyboard shortcuts ───────────────────────────────────────────────────────
// Listen on window but only act when the pointer is over the player or focus is
// inside it — so space/arrows control THIS player without hijacking the page.

function playerIsActive(): boolean {
  if (isPointerInside.value) return true
  const root = rootRef.value
  return !!(root && document.activeElement && root.contains(document.activeElement))
}

function onKeydown(e: KeyboardEvent) {
  if (!playerIsActive()) return
  wakeUi()

  if (e.key === 'Escape') {
    if (openMenu.value !== null || browseOpen.value) {
      closeMenus()
      e.preventDefault()
    }
    return
  }

  const action = mapKeyToAction(e)
  if (!action) return
  e.preventDefault()

  switch (action.type) {
    case 'play-pause':
      togglePlay()
      break
    case 'seek-rel':
      onSeekRel(action.value)
      break
    case 'vol-rel': {
      const next = Math.max(0, Math.min(100, state.volume.value + action.value))
      if (state.muted.value && action.value > 0) onToggleMute()
      onSetVolume(next)
      break
    }
    case 'seek-pct': {
      const v = videoRef.value
      if (v && v.duration) {
        v.currentTime = (action.value / 100) * v.duration
        writeProgress()
      }
      break
    }
    case 'sub-offset': {
      const next = Math.round((state.subOffset.value + action.value) * 10) / 10
      state.subOffset.value = next
      break
    }
    case 'mute':
      onToggleMute()
      break
    case 'fullscreen':
      onToggleFullscreen()
      break
    case 'subs':
      toggleMenu('subs')
      break
    case 'pip':
      onTogglePip()
      break
  }
}

// ─── Lifecycle ────────────────────────────────────────────────────────────────

// Graceful save on tab close / backgrounding. `pagehide` covers navigation
// and close (incl. mobile); visibilitychange→hidden covers app switches —
// both use sendBeacon, which survives the unload where XHR doesn't.
function onPageHide() {
  tracking.beaconSave()
}
function onVisibilityChange() {
  if (document.visibilityState === 'hidden') tracking.beaconSave()
}

onMounted(() => {
  start()
  // Apply initial volume
  const v = videoRef.value
  if (v) {
    v.volume = state.volume.value / 100
    v.muted = state.muted.value
  }
  // Bootstrap episode selection so it's ready before provider resolves
  initSelectedEpisode()
  // User watch data for the episodes drawer (best-effort, anonymous = empty)
  void refreshWatched()
  void loadEpisodeProgress()
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('pagehide', onPageHide)
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onUnmounted(() => {
  stopRaf()
  tracking.saveNow() // persist position when navigating away in-app
  clearNextEpTimer()
  clearUiIdleTimer()
  clearHudTimers()
  if (bufferingTimer) clearTimeout(bufferingTimer)
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('pagehide', onPageHide)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<style scoped>
.pl {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  border-radius: var(--r-xl, 16px);
  overflow: hidden;
  background: #000;
  border: 1px solid var(--border);
  user-select: none;
}

/* Resume-from-saved-position chip — bottom-left mirror of the skip chip. */
.pl-resume {
  position: absolute;
  left: 22px;
  bottom: 92px;
  z-index: 6;
  display: inline-flex;
  align-items: stretch;
  border-radius: var(--r-md);
  background: rgba(8, 8, 15, 0.75);
  border: 1px solid rgba(255, 255, 255, 0.35);
  backdrop-filter: blur(8px);
  overflow: hidden;
}

.pl-resume-go {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 11px 14px;
  border: 0;
  background: transparent;
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.pl-resume-go:hover {
  background: #fff;
  color: var(--color-base, #08080f);
}

.pl-resume-x {
  display: inline-flex;
  align-items: center;
  padding: 0 10px;
  border: 0;
  border-left: 1px solid rgba(255, 255, 255, 0.2);
  background: transparent;
  color: rgba(255, 255, 255, 0.6);
  cursor: pointer;
  transition: color 0.15s, background 0.15s;
}

.pl-resume-x:hover {
  color: #fff;
  background: rgba(255, 255, 255, 0.12);
}

.pl--theater {
  border-radius: 0;
  border: 0;
  aspect-ratio: auto;
  height: 100vh;
}

.pl-scene {
  position: absolute;
  inset: 0;
  background-size: cover;
  background-position: center;
  z-index: 0;
}

.pl-grain {
  position: absolute;
  inset: 0;
  background: radial-gradient(80% 60% at 50% 38%, transparent, rgba(0, 0, 0, 0.35));
  z-index: 1;
  pointer-events: none;
}

/* Episode title in the eyebrow — truncate long provider titles */
.pl-ep-title {
  display: inline-block;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
  opacity: 0.85;
}

/* V2b: the EP block doubles as the episodes-sheet trigger */
.pl-ep-trigger {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  padding: 2px 6px;
  margin: -2px -2px -2px -6px;
  border-radius: var(--r-sm, 6px);
  cursor: pointer;
  transition: background 0.15s;
}

.pl-ep-trigger:hover,
.pl-ep-trigger[aria-expanded='true'] {
  background: rgba(0, 212, 255, 0.12);
}

.pl-ep-chev {
  opacity: 0.7;
  transition: transform 0.15s;
}

.pl-ep-chev--open {
  transform: rotate(180deg);
}

/* Idle while playing — fade out the chrome (top bar lives here, the control
   bar is inside <PlayerControlBar>, hence :deep). */
.pl--ui-hidden {
  cursor: none;
}

.pl--ui-hidden .pl-top,
.pl--ui-hidden :deep(.pl-controls) {
  opacity: 0;
  pointer-events: none;
}

/* Top bar */
.pl-top {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 6;
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 16px 18px 40px;
  background: linear-gradient(180deg, rgba(0, 0, 0, 0.65), transparent);
  transition: opacity 0.2s;
}

.pl-icon {
  width: 40px;
  height: 40px;
  display: grid;
  place-items: center;
  border-radius: var(--r-md, 8px);
  background: transparent;
  border: 0;
  color: #fff;
  transition: background 0.15s;
  flex-shrink: 0;
  cursor: pointer;
}

.pl-icon:hover {
  background: rgba(255, 255, 255, 0.14);
}

.pl-title-block {
  flex: 1;
  min-width: 0;
  padding-top: 3px;
}

.pl-eyebrow {
  font-size: 12px;
  color: var(--brand-cyan);
  display: block;
}

.pl-eyebrow-src {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.pl-prov-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
  display: inline-block;
}

.pl-title {
  font-family: var(--font-display, inherit);
  font-weight: 800;
  font-size: 19px;
  margin: 2px 0 0;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pl-top-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* Player shell is tabindex=0 for hotkeys — suppress all focus rings;
   play/pause state and control bar visibility are sufficient feedback. */
.pl:focus,
.pl:focus-visible {
  outline: none;
}

/* Floating menu cards (source / settings / subtitles) — anchored over the
   video so they actually appear; without a positioned wrapper the bare menu
   components rendered in static flow and were invisible. */
.pl-floating {
  position: absolute;
  z-index: 12;
  border-radius: var(--r-lg, 12px);
  background: var(--card);
  border: 1px solid var(--border);
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
  animation: pl-pop 0.18s ease;
  overflow-y: auto;
  scrollbar-width: thin;
}

/* Source panel: larger, top-right under the header. */
.pl-floating--source {
  top: 64px;
  right: 14px;
  width: 320px;
  max-height: calc(100% - 130px);
}

/* Episodes sheet (V2b): full-width above the control bar. */
.pl-floating--sheet {
  left: 10px;
  right: 10px;
  bottom: 76px;
  max-height: calc(100% - 150px);
}

/* Settings / subtitles: compact card floating above the control-bar buttons. */
.pl-floating--btnmenu {
  right: 14px;
  bottom: 76px;
  padding: 6px;
  max-height: calc(100% - 130px);
}

@keyframes pl-pop {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 680px) {
  .pl-floating--source {
    width: calc(100% - 28px);
  }
}
</style>
