<template>
  <div
    ref="rootRef"
    class="pl"
    :class="{ 'pl--theater': theater }"
    :style="{ '--prov': activeProviderHue }"
    tabindex="0"
    role="region"
    aria-label="Video player. Space to play or pause, arrow keys to seek and adjust volume."
    @click.self="closeMenus"
    @mouseenter="isPointerInside = true"
    @mouseleave="isPointerInside = false"
    data-test="unified-player"
  >
    <!-- Poster / still background -->
    <div
      v-if="anime.still && !state.playing.value"
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
      @click="togglePlay"
      @volumechange="onVolumeChange"
      @waiting="onBufferStart"
      @seeking="onBufferStart"
      @canplay="onBufferEnd"
      @playing="onBufferEnd"
      @seeked="onSeeked"
      @timeupdate="onTimeUpdate"
    />

    <!-- Subtitle overlay -->
    <SubtitleOverlay
      :video-element="videoRef"
      :subtitle-url="chosenSubUrl"
      :format="chosenSubFormat"
      :visible="state.subLang.value !== 'off' && !!chosenSubUrl"
      :offset="state.subOffset.value"
    />

    <!-- Source error overlay -->
    <div
      v-if="sourceError"
      class="absolute inset-0 z-[2] flex items-center justify-center"
      style="background: rgba(0,0,0,0.72);"
    >
      <div class="flex flex-col items-center gap-3 text-center px-8">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="text-muted-foreground" aria-hidden="true">
          <circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
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
      <!-- Episodes (left chevron — opens the episode drawer) -->
      <button class="pl-icon" aria-label="Episodes" @click="toggleMenu('episodes')">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <polyline points="15 18 9 12 15 6" />
        </svg>
      </button>

      <!-- Title block -->
      <div class="pl-title-block">
        <span class="pl-eyebrow">
          <span class="pl-eyebrow-src">
            EP {{ anime.ep }}
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
        <button
          class="pl-icon"
          aria-label="Episode list"
          title="Episodes"
          @click="toggleMenu('episodes')"
        >
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <line x1="8" y1="6" x2="21" y2="6" /><line x1="8" y1="12" x2="21" y2="12" /><line x1="8" y1="18" x2="21" y2="18" /><line x1="3" y1="6" x2="3.01" y2="6" /><line x1="3" y1="12" x2="3.01" y2="12" /><line x1="3" y1="18" x2="3.01" y2="18" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Overlays -->
    <ResumePill :kind="resumeKind" />

    <BigPlayButton
      :visible="!state.playing.value && !sourceError && !showBuffering && !isResolving"
      @play="togglePlay"
    />

    <BufferingOverlay :visible="(showBuffering || isResolving) && !sourceError" />

    <!-- TODO: real skip-timings backend (later stage) -->
    <SkipIntroChip :visible="false" @skip="() => {}" />

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
      :progress="state.progress.value"
      :buffered="bufferedPct"
      :chapters="[]"
      :still-url="anime.still"
      :open-menu="openMenu"
      @toggle-play="togglePlay"
      @seek-rel="onSeekRel"
      @seek="onSeek"
      @set-volume="onSetVolume"
      @toggle-mute="onToggleMute"
      @toggle-source="toggleMenu('source')"
      @toggle-subs="toggleMenu('subs')"
      @toggle-settings="toggleMenu('settings')"
      @toggle-pip="onTogglePip"
      @toggle-fullscreen="onToggleFullscreen"
    />

    <!-- Source panel (floating, top-right) -->
    <div v-if="openMenu === 'source'" class="pl-floating pl-floating--source" @click.stop>
      <SourcePanel
        :rows="rows"
        :audio="state.combo.value.audio"
        :lang="state.combo.value.lang"
        :team="state.combo.value.team"
        :provider="state.combo.value.provider"
        :server="state.combo.value.server"
        :servers="resolvedServers"
        :teams="[]"
        @update:audio="state.setAudio"
        @update:lang="state.setLang"
        @update:team="state.setTeam"
        @select-provider="onSelectProvider"
        @select-server="state.setServer"
      />
    </div>

    <!-- Episodes drawer (floating, top-right — reuses source-panel geometry) -->
    <div v-if="openMenu === 'episodes'" class="pl-floating pl-floating--source" @click.stop>
      <EpisodesPanel
        :episodes="episodes"
        :selected-number="selectedEpisode?.number ?? null"
        @select="onSelectEpisode"
      />
    </div>

    <!-- Playback settings menu (floating, above control bar) -->
    <div v-if="openMenu === 'settings'" class="pl-floating pl-floating--btnmenu" @click.stop>
      <PlaybackSettingsMenu
        :quality="state.quality.value"
        :qualities="qualities"
        :quality-display="qualityDisplay"
        :speed="state.speed.value"
        :speeds="[0.75, 1, 1.25, 1.5, 2]"
        :auto-next="state.autoNext.value"
        :auto-skip="state.autoSkip.value"
        @update:quality="onSetQuality"
        @update:speed="onSetSpeed"
        @update:auto-next="v => { state.autoNext.value = v }"
        @update:auto-skip="v => { state.autoSkip.value = v }"
      />
    </div>

    <!-- Subtitles menu (floating, above control bar) -->
    <div v-if="openMenu === 'subs'" class="pl-floating pl-floating--btnmenu" @click.stop>
      <SubtitlesMenu
        :sub-lang="state.subLang.value"
        :sub-langs="subLangsAvailable"
        :sub-size="state.subSize.value"
        :sub-bg="state.subBg.value"
        :sub-offset="state.subOffset.value"
        @update:sub-lang="v => { state.subLang.value = v as 'off' | 'en' | 'ru' | 'ja' }"
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
import SkipIntroChip from './overlays/SkipIntroChip.vue'
import NextEpisodeCard from './overlays/NextEpisodeCard.vue'
import WatchTogetherButton from './overlays/WatchTogetherButton.vue'

import { usePlayerState } from '@/composables/unifiedPlayer/usePlayerState'
import { useVideoEngine } from '@/composables/unifiedPlayer/useVideoEngine'
import { useProviderResolver } from '@/composables/unifiedPlayer/useProviderResolver'
import { useProviderHealth } from '@/composables/unifiedPlayer/useProviderHealth'
import { mapKeyToAction } from '@/composables/unifiedPlayer/playerHotkeys'
import { providerById } from './providerRegistry'

import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { StreamResult } from '@/types/unifiedPlayer'

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

// ─── Provider health filter ───────────────────────────────────────────────────

const filter = computed(() => ({
  audio: state.combo.value.audio,
  lang: state.combo.value.lang,
  content: (props.isHentai ? 'hentai' : 'common') as 'hentai' | 'common',
}))

const { rows, start } = useProviderHealth(filter)

// ─── Provider defaults ────────────────────────────────────────────────────────

watch(
  rows,
  () => {
    if (!state.combo.value.provider) {
      const first = rows.value.find((r) => r.state === 'active')
      if (first) {
        state.setProvider(first.def.id, '')
      }
    }
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
const sourceError = ref<string | null>(null)
const resolvedServers = ref<{ id: string; label: string }[]>([])
const currentStream = ref<StreamResult | null>(null)
const isResolving = ref(false)

// Monotonically-increasing request token — only the latest resolve applies.
// Prevents a stale audio/lang/server re-resolve from clobbering a concurrent
// provider-change full re-list+resolve that started after it.
let resolveToken = 0

// ─── Quality ladder ──────────────────────────────────────────────────────────
// HLS: data-driven from hls.js levels. MP4: only when the provider returned
// multiple URL-valued qualities. Single-variant streams stay Auto-only (D-05).
// NOTE: declared after currentStream — watch() below evaluates its computed
// source eagerly during setup.

const mp4Qualities = computed(() => {
  const s = currentStream.value
  if (!s || s.type !== 'mp4' || !s.qualities) return []
  return s.qualities.filter(
    (q) => typeof q.value === 'string' && /^(https?:|\/)/.test(q.value as string),
  )
})

const qualities = computed(() => {
  if (mp4Qualities.value.length > 1) return ['Auto', ...mp4Qualities.value.map((q) => q.label)]
  return ['Auto', ...engine.levels.value.map((l) => l.label)]
})

const qualityDisplay = computed(() =>
  state.quality.value === 'Auto' && engine.currentLevelLabel.value
    ? `Auto · ${engine.currentLevelLabel.value}`
    : state.quality.value,
)

// New stream may not offer the previously-chosen quality — snap back to Auto.
// If it DOES offer it, re-apply: each load() creates a fresh hls instance
// that starts at auto, so a pinned level must be re-pinned.
watch(qualities, (qs) => {
  if (!qs.includes(state.quality.value)) {
    state.quality.value = 'Auto'
  } else if (state.quality.value !== 'Auto' && mp4Qualities.value.length === 0) {
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
  engine.setLevel(q)
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
  const token = ++resolveToken

  try {
    // Load episode list
    const eps = await resolver.listEpisodes(provider, props.animeId)
    if (token !== resolveToken) return // superseded by a later request

    episodes.value = eps

    // Preserve the selected episode number across provider changes
    const targetNum =
      selectedEpisode.value?.number ?? props.initialEpisode ?? props.anime.ep ?? 1
    const ep =
      eps.find((e) => e.number === targetNum) ??
      eps[0] ??
      selectedEpisode.value

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
    if (isNotAvailable) {
      sourceError.value = "This source isn't available yet"
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

// Audio / language / server change, or episode selection → re-resolve stream
// only (no need to re-list episodes). The token inside resolveStreamForEpisode
// ensures a concurrent provider-change full-resolve wins if they race.
// Skip when loadEpisodesAndStream is in-flight: it sets selectedEpisode itself
// and will call resolveStream at the end — we must not fire a duplicate.
watch(
  () => [
    state.combo.value.audio,
    state.combo.value.lang,
    state.combo.value.server,
    selectedEpisode.value,
  ] as const,
  (_newVal, oldVal) => {
    // Skip the very first run (oldVal is undefined on initial watch fire)
    if (oldVal === undefined) return
    // Skip if a full re-list is already in progress (provider changed)
    if (isResolving.value) return
    void resolveStreamForCurrentEpisode()
  },
)

// ─── Provider selection helper ────────────────────────────────────────────────

function onSelectProvider(id: string) {
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
  selectedEpisode.value = ep
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
}

function onVideoPause() {
  state.playing.value = false
  stopRaf()
}

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
}

function onBufferEnd() {
  setBuffering(false)
}

function onSeeked() {
  const v = videoRef.value
  if (v && v.readyState >= 3) setBuffering(false)
}

// Self-heal: if time is advancing with decodable data, we are NOT buffering.
function onTimeUpdate() {
  const v = videoRef.value
  if (isBuffering.value && v && v.readyState >= 3 && !v.seeking) {
    setBuffering(false)
  }
}

// ─── Next episode logic ───────────────────────────────────────────────────────

const showNextEpisode = ref(false)
const nextEpCountdown = ref(5)
let nextEpTimer: ReturnType<typeof setInterval> | null = null

function onEnded() {
  state.playing.value = false
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
    selectedEpisode.value = next
    void resolveStreamForEpisode(next)
  }
}

async function resolveStreamForEpisode(ep: EpisodeOption) {
  const provider = state.combo.value.provider
  if (!provider) return
  sourceError.value = null
  isResolving.value = true
  const token = ++resolveToken
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

function toggleMenu(menu: MenuKind) {
  openMenu.value = openMenu.value === menu ? null : menu
  if (openMenu.value !== null) browseOpen.value = false
}

function closeMenus() {
  openMenu.value = null
  browseOpen.value = false
}

// ─── Subtitles ────────────────────────────────────────────────────────────────

const chosenSub = ref<SubTrack | null>(null)

const chosenSubUrl = computed<string | null>(() => chosenSub.value?.url ?? null)
const chosenSubFormat = computed<'ass' | 'srt' | 'vtt' | null>(() => {
  const fmt = chosenSub.value?.format ?? null
  if (fmt === 'ass' || fmt === 'srt' || fmt === 'vtt') return fmt
  return null
})

// Real subtitle languages (the menu renders the "Off" option itself).
const subLangsAvailable = computed(() => ['en', 'ru', 'ja'])

function onSelectSubTrack(track: SubTrack) {
  chosenSub.value = track
  browseOpen.value = false
}

// ─── Resume pill ─────────────────────────────────────────────────────────────

// Stage 1: static "first-time" — no persistent watch progress wired yet
const resumeKind = computed<
  'first-time' | 'watching' | 'finished' | 'not-yet-aired' | 'episode-not-loaded-yet'
>(() => 'first-time')

// ─── Playback helpers ─────────────────────────────────────────────────────────

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
  v.currentTime = Math.max(0, Math.min(v.duration || 0, v.currentTime + delta))
}

function onSeek(pct: number) {
  const v = videoRef.value
  if (!v || !v.duration) return
  v.currentTime = (pct / 100) * v.duration
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
  window.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  stopRaf()
  clearNextEpTimer()
  if (bufferingTimer) clearTimeout(bufferingTimer)
  window.removeEventListener('keydown', onKeydown)
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

/* Keyboard focus ring on the player shell (tabindex=0 for hotkeys). */
.pl:focus {
  outline: none;
}

.pl:focus-visible {
  outline: 2px solid var(--brand-cyan);
  outline-offset: -2px;
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
