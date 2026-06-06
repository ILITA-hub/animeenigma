<template>
  <div
    class="pl"
    :class="{ 'pl--theater': theater }"
    :style="{ '--prov': activeProviderHue }"
    @click.self="closeMenus"
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
      preload="metadata"
      @play="state.playing.value = true"
      @pause="state.playing.value = false"
      @ended="onEnded"
      @click="togglePlay"
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
      <!-- Back button -->
      <button class="pl-icon" aria-label="Back" @click="$emit('open-episodes')">
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
          @click="$emit('open-episodes')"
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
      :visible="!state.playing.value && !sourceError"
      @play="togglePlay"
    />

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
      :theater="theater"
      @toggle-play="togglePlay"
      @seek-rel="onSeekRel"
      @set-volume="onSetVolume"
      @toggle-mute="onToggleMute"
      @toggle-source="toggleMenu('source')"
      @toggle-subs="toggleMenu('subs')"
      @toggle-settings="toggleMenu('settings')"
      @toggle-pip="onTogglePip"
      @toggle-theater="$emit('toggle-theater')"
      @toggle-fullscreen="onToggleFullscreen"
    />

    <!-- PlayerScrubBar (inside control bar area via slot or directly above it) -->
    <div class="pl-scrub-overlay" @click.stop>
      <PlayerScrubBar
        :progress="state.progress.value"
        :buffered="bufferedPct"
        :duration-sec="duration"
        :chapters="[]"
        :still-url="anime.still"
        @seek="onSeek"
      />
    </div>

    <!-- Source panel -->
    <SourcePanel
      v-if="openMenu === 'source'"
      :rows="rows"
      :audio="state.combo.value.audio"
      :lang="state.combo.value.lang"
      :team="state.combo.value.team"
      :provider="state.combo.value.provider"
      :server="state.combo.value.server"
      :servers="resolvedServers"
      :teams="[]"
      @click.stop
      @update:audio="state.setAudio"
      @update:lang="state.setLang"
      @update:team="state.setTeam"
      @select-provider="onSelectProvider"
      @select-server="state.setServer"
    />

    <!-- Playback settings menu -->
    <PlaybackSettingsMenu
      v-if="openMenu === 'settings'"
      :quality="state.quality.value"
      :qualities="['Auto']"
      :speed="state.speed.value"
      :speeds="[0.25, 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2]"
      :auto-next="state.autoNext.value"
      :auto-skip="state.autoSkip.value"
      @click.stop
      @update:quality="v => { state.quality.value = v }"
      @update:speed="onSetSpeed"
      @update:auto-next="v => { state.autoNext.value = v }"
      @update:auto-skip="v => { state.autoSkip.value = v }"
    />

    <!-- Subtitles menu -->
    <SubtitlesMenu
      v-if="openMenu === 'subs'"
      :sub-lang="state.subLang.value"
      :sub-langs="subLangsAvailable"
      :sub-size="state.subSize.value"
      :sub-bg="state.subBg.value"
      :sub-offset="state.subOffset.value"
      @click.stop
      @update:sub-lang="v => { state.subLang.value = v as 'off' | 'en' | 'ru' | 'ja' }"
      @update:sub-size="v => { state.subSize.value = v }"
      @update:sub-bg="v => { state.subBg.value = v }"
      @update:sub-offset="v => { state.subOffset.value = v }"
      @open-browse="browseOpen = true"
    />

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
import PlayerScrubBar from './PlayerScrubBar.vue'
import SourcePanel from './SourcePanel.vue'
import PlaybackSettingsMenu from './PlaybackSettingsMenu.vue'
import SubtitlesMenu from './SubtitlesMenu.vue'
import BrowseSubsModal from './BrowseSubsModal.vue'
import BigPlayButton from './overlays/BigPlayButton.vue'
import SkipIntroChip from './overlays/SkipIntroChip.vue'
import NextEpisodeCard from './overlays/NextEpisodeCard.vue'
import WatchTogetherButton from './overlays/WatchTogetherButton.vue'

import { usePlayerState } from '@/composables/unifiedPlayer/usePlayerState'
import { useVideoEngine } from '@/composables/unifiedPlayer/useVideoEngine'
import { useProviderResolver } from '@/composables/unifiedPlayer/useProviderResolver'
import { useProviderHealth } from '@/composables/unifiedPlayer/useProviderHealth'
import { providerById } from './providerRegistry'

import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

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

const emit = defineEmits<{
  (e: 'toggle-theater'): void
  (e: 'open-episodes'): void
}>()

// ─── Core state ──────────────────────────────────────────────────────────────

const videoRef = ref<HTMLVideoElement | null>(null)
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
const isResolving = ref(false)

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

  try {
    // Load episode list
    const eps = await resolver.listEpisodes(provider, props.animeId)
    episodes.value = eps

    // Select the right episode
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

    resolvedServers.value = stream.servers ?? []
    await engine.load(stream)
  } catch (err: unknown) {
    const isNotAvailable =
      err instanceof Error && err.name === 'NotAvailableError'
    if (isNotAvailable) {
      sourceError.value = "This source isn't available yet"
    } else {
      sourceError.value = 'Stream unavailable'
    }
  } finally {
    isResolving.value = false
  }
}

// Watch provider or selected episode changes → re-resolve
watch(
  () => state.combo.value.provider,
  (newProvider) => {
    if (newProvider) {
      void loadEpisodesAndStream()
    }
  },
)

// ─── Provider selection helper ────────────────────────────────────────────────

function onSelectProvider(id: string) {
  state.setProvider(id, '')
  // loadEpisodesAndStream fires via the provider watcher above
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

function tick() {
  const v = videoRef.value
  if (v) {
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
  rafId = requestAnimationFrame(tick)
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
  try {
    const stream = await resolver.resolveStream(
      provider,
      props.animeId,
      ep,
      state.combo.value,
    )
    resolvedServers.value = stream.servers ?? []
    await engine.load(stream)
  } catch (err: unknown) {
    const isNotAvailable =
      err instanceof Error && err.name === 'NotAvailableError'
    sourceError.value = isNotAvailable
      ? "This source isn't available yet"
      : 'Stream unavailable'
  }
}

// ─── Menu state ───────────────────────────────────────────────────────────────

type MenuKind = 'source' | 'settings' | 'subs' | null
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

const subLangsAvailable = computed(() =>
  state.subLang.value === 'off' ? ['off'] : ['off', 'en', 'ru', 'ja'],
)

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
  const el = videoRef.value?.parentElement
  if (!el) return
  if (document.fullscreenElement) {
    void document.exitFullscreen()
  } else {
    void el.requestFullscreen()
  }
}

// ─── Lifecycle ────────────────────────────────────────────────────────────────

onMounted(() => {
  start()
  rafId = requestAnimationFrame(tick)
  // Apply initial volume
  const v = videoRef.value
  if (v) {
    v.volume = state.volume.value / 100
    v.muted = state.muted.value
  }
  // Bootstrap episode selection so it's ready before provider resolves
  initSelectedEpisode()
})

onUnmounted(() => {
  if (rafId !== null) {
    cancelAnimationFrame(rafId)
    rafId = null
  }
  clearNextEpTimer()
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

/* Scrub bar override position — sits inside control bar area */
.pl-scrub-overlay {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 44px; /* sits just above the button row */
  z-index: 7;
  padding: 0 16px;
}
</style>
