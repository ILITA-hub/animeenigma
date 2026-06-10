<template>
  <div class="raw-player" style="--player-accent: #22d3ee; --player-accent-rgb: 34, 211, 238;">
    <!-- Loading episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- Provider not available -->
    <div
      v-else-if="!available"
      class="text-center py-16 text-white/60"
    >
      <Video class="size-12 mx-auto mb-3 opacity-50" aria-hidden="true" />
      {{ $t('player.raw.unavailable') }}
    </div>

    <!-- Main content -->
    <div v-else class="flex flex-col gap-4">
      <!-- Video container -->
      <div ref="playerContainer" class="relative aspect-video bg-black rounded-xl overflow-hidden">
        <!-- Loading overlay -->
        <div
          v-if="loadingStream"
          class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
        >
          <div class="text-center">
            <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin mx-auto mb-3" />
            <p class="text-white/60 text-sm">
              {{ $t('player.loadingEpisode', { n: selectedEpisode?.number }) }}
            </p>
          </div>
        </div>

        <!-- Native HLS video element -->
        <video
          v-show="streamUrl"
          ref="videoRef"
          class="absolute inset-0 w-full h-full"
          controls
          playsinline
          @timeupdate="handleTimeUpdate"
        />

        <!-- Placeholder when nothing loaded -->
        <div
          v-if="!streamUrl && !loadingStream"
          class="absolute inset-0 flex items-center justify-center text-white/40"
        >
          <div class="text-center">
            <Play class="size-16 mx-auto mb-3" aria-hidden="true" />
            <p>{{ $t('player.selectEpisode') }}</p>
          </div>
        </div>

        <!-- Subtitle overlay -->
        <SubtitleOverlay
          :video-element="videoRef"
          :subtitle-url="activeSubUrl"
          :format="activeSubFormat"
          :visible="!!activeSubUrl"
          :fullscreen-container="playerContainer"
          :offset="subtitleOffset"
        />
      </div>

      <!-- Primary toolbar — subtitle picker + Other Subs + provider note -->
      <div class="flex flex-col sm:flex-row gap-3 items-start sm:items-center justify-between bg-white/5 rounded-lg p-3">
        <div class="flex flex-col sm:flex-row gap-3 items-start sm:items-center w-full sm:w-auto">
          <label class="flex items-center gap-2 text-white/70 text-sm">
            <Captions class="size-4" aria-hidden="true" />
            {{ $t('player.subtitlePicker.label') }}
          </label>
          <select
            v-model="selectedSubKey"
            class="bg-white/10 hover:bg-white/15 text-white text-sm rounded-md px-3 py-1.5 border border-white/10 focus:outline-none focus:ring-2 focus:ring-cyan-400/40"
            :disabled="availableSubChoices.length === 0"
          >
            <option value="">{{ $t('player.subtitlePicker.none') }}</option>
            <option
              v-for="choice in availableSubChoices"
              :key="choice.key"
              :value="choice.key"
            >
              {{ choice.label }}
            </option>
          </select>
        </div>

        <div class="flex items-center gap-2">
          <SubtitleSettingsMenu :has-active-sub="!!activeSubUrl" />
          <button
            type="button"
            class="inline-flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-cyan-500/15 hover:bg-cyan-500/25 text-cyan-100 border border-cyan-400/30 transition-colors"
            @click="otherSubsOpen = true"
          >
            <List class="size-4" aria-hidden="true" />
            {{ $t('player.otherSubs.openButton') }}
          </button>
        </div>
      </div>

      <!-- Episode list -->
      <div>
        <div class="flex items-center gap-3 mb-3 flex-wrap">
          <h3 class="text-white/60 text-sm flex items-center gap-2">
            <List class="size-4" aria-hidden="true" />
            {{ $t('player.episodesCount', { count: episodes.length }) }}
          </h3>
          <slot name="header-middle" />
        </div>
        <EpisodeSelector
          :episodes="episodeOptions"
          :selected-key="selectedEpisode?.id ?? null"
          :watched-up-to="watchedUpTo"
          @select="onEpisodePicked"
        />
      </div>
    </div>

    <OtherSubsPanel
      v-model="otherSubsOpen"
      :anime-id="props.animeId"
      :episode="selectedEpisode?.number ?? 1"
      :current-track-url="activeSubUrl"
      @select-track="onOtherSubSelected"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Video, Play, Captions, List } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import Hls from 'hls.js'
import SubtitleOverlay from './SubtitleOverlay.vue'
import OtherSubsPanel from './OtherSubsPanel.vue'
import SubtitleSettingsMenu from './SubtitleSettingsMenu.vue'
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
import { useSubtitleTimingOffset } from '@/composables/useSubtitleTimingOffset'
import { rawApi, subtitlesApi } from '@/api/client'
import type {
  GroupedSubs,
  RawEpisode,
  RawEpisodesResponse,
  RawStream,
  SubtitleTrack,
} from '@/types/raw'
import { usePlayerSyncBridge } from '@/composables/usePlayerSyncBridge'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'

const props = defineProps<{
  animeId: string
  // Phase 3 (03.3) — room prop drives the WatchTogether sync bridge (wired below
  // once `videoRef` is declared).
  room?: WatchTogetherRoomHandle | null
}>()

const { locale } = useI18n()

const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const playerContainer = ref<HTMLElement | null>(null)
const { offset: subtitleOffset } = useSubtitleTimingOffset()
const videoRef = ref<HTMLVideoElement | null>(null)

// Phase 3 (03.3): wire real sync when a room is provided. Zero behavior
// change when room is null/undefined.
if (props.room) {
  usePlayerSyncBridge(videoRef, props.room)
}

const loadingEpisodes = ref(false)
const loadingStream = ref(false)
const available = ref(true)
const episodes = ref<RawEpisode[]>([])
const selectedEpisode = ref<RawEpisode | null>(null)
const streamUrl = ref<string | null>(null)
const otherSubsOpen = ref(false)

const episodeOptions = computed<EpisodeOption[]>(() =>
  episodes.value.map((ep) => ({ key: ep.id, label: ep.number, number: Number(ep.number) })),
)

function onEpisodePicked(key: string | number) {
  const ep = episodes.value.find((e) => e.id === key)
  if (ep) void selectEpisode(ep)
}

const subsByLang = ref<GroupedSubs | null>(null)
// Composite key: "lang:url" so duplicate-language tracks remain selectable.
const selectedSubKey = ref<string>('')

let hls: Hls | null = null

interface SubChoice {
  key: string
  label: string
  url: string
  lang: string
  format: 'ass' | 'srt' | 'vtt' | null
}

const availableSubChoices = computed<SubChoice[]>(() => {
  if (!subsByLang.value) return []
  const out: SubChoice[] = []
  for (const [lang, tracks] of Object.entries(subsByLang.value.languages)) {
    for (const t of tracks) {
      out.push({
        key: `${lang}:${t.url}`,
        label: `${displayLangLabel(lang)} — ${t.label || t.release || 'subtitle'}`,
        url: t.url,
        lang,
        format: detectFormat(t.format, t.url),
      })
    }
  }
  return out
})

const activeChoice = computed<SubChoice | null>(() => {
  if (!selectedSubKey.value) return null
  return availableSubChoices.value.find((c) => c.key === selectedSubKey.value) ?? null
})

const activeSubUrl = computed(() => activeChoice.value?.url ?? null)
const activeSubFormat = computed(() => activeChoice.value?.format ?? null)

function detectFormat(format: string | undefined, url: string): 'ass' | 'srt' | 'vtt' | null {
  const candidate = (format || url.split('?')[0].split('.').pop() || '').toLowerCase()
  if (candidate === 'ass' || candidate === 'srt' || candidate === 'vtt') {
    return candidate
  }
  return null
}

function displayLangLabel(lang: string): string {
  switch (lang) {
    case 'ja': return '日本語'
    case 'en': return 'English'
    case 'ru': return 'Русский'
    default: return lang.toUpperCase()
  }
}

function preferredLanguage(): string {
  return (
    localStorage.getItem('preferred_subtitle_language')
    || locale.value
    || 'ja'
  )
}

function autoSelectSubtitle() {
  if (availableSubChoices.value.length === 0) {
    selectedSubKey.value = ''
    return
  }
  const pref = preferredLanguage()
  const matchPref = availableSubChoices.value.find((c) => c.lang === pref)
  if (matchPref) {
    selectedSubKey.value = matchPref.key
    return
  }
  // Fallback to first available track regardless of language.
  selectedSubKey.value = availableSubChoices.value[0].key
}

function onOtherSubSelected(track: SubtitleTrack) {
  const key = `${track.lang}:${track.url}`
  // If the track isn't in the byLang response (e.g. exotic language), inject
  // a synthetic choice so the picker can display it.
  if (!availableSubChoices.value.some((c) => c.key === key)) {
    const synthetic: SubChoice = {
      key,
      label: `${displayLangLabel(track.lang)} — ${track.label || track.release || 'subtitle'}`,
      url: track.url,
      lang: track.lang,
      format: detectFormat(track.format, track.url),
    }
    if (subsByLang.value) {
      const langArr = subsByLang.value.languages[track.lang] ?? []
      subsByLang.value = {
        ...subsByLang.value,
        languages: {
          ...subsByLang.value.languages,
          [track.lang]: [...langArr, track],
        },
      }
    }
    // Recompute via re-trigger of computed dependency.
    void synthetic
  }
  selectedSubKey.value = key
}

function buildProxyUrl(url: string, referer: string, streamType?: 'hls' | 'mp4'): string {
  const params = new URLSearchParams()
  params.set('url', url)
  if (referer) params.set('referer', referer)
  if (streamType === 'mp4') params.set('type', 'mp4')
  return `/api/streaming/hls-proxy?${params.toString()}`
}

function disposePlayer() {
  if (hls) {
    hls.destroy()
    hls = null
  }
  const v = videoRef.value
  if (v) {
    v.removeAttribute('src')
    try { v.load() } catch { /* ignore */ }
  }
}

function attachStream(url: string, type: 'hls' | 'mp4') {
  const v = videoRef.value
  if (!v) return

  disposePlayer()

  if (type !== 'hls') {
    // AllAnime's fast4speed.rsvp CDN requires Referer: https://allmanga.to/,
    // which a direct <video src> won't send. Route MP4 through the proxy so
    // the backend can inject the right Referer while passing range requests through.
    v.src = buildProxyUrl(url, 'https://allmanga.to/', 'mp4')
    v.play().catch(() => { /* user-gesture required */ })
    return
  }

  const proxyUrl = buildProxyUrl(url, 'https://allmanga.to/')

  if (Hls.isSupported()) {
    hls = new Hls({
      enableWorker: true,
      lowLatencyMode: false,
      backBufferLength: 90,
    })
    hls.loadSource(proxyUrl)
    hls.attachMedia(v)
    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      v.play().catch(() => { /* user-gesture required */ })
    })
  } else if (v.canPlayType('application/vnd.apple.mpegurl')) {
    v.src = proxyUrl
    v.addEventListener('loadedmetadata', () => {
      v.play().catch(() => { /* user-gesture required */ })
    }, { once: true })
  }
}

const fetchEpisodes = async () => {
  loadingEpisodes.value = true
  let shouldAutoSelect = false
  try {
    const resp = await rawApi.getEpisodes(props.animeId)
    const data: RawEpisodesResponse = resp.data?.data ?? resp.data
    episodes.value = data.episodes ?? []
    available.value = data.available && episodes.value.length > 0
    shouldAutoSelect = available.value && episodes.value.length > 0
  } catch {
    available.value = false
  } finally {
    loadingEpisodes.value = false
  }
  // Auto-select must happen AFTER loadingEpisodes flips so the v-if branch
  // renders the <video> and binds the template ref. Without the nextTick
  // the inner attachStream sees videoRef.value === null and early-returns,
  // leaving v.src empty (user sees a blank player and assumes broken).
  if (shouldAutoSelect) {
    await nextTick()
    await selectEpisode(episodes.value[0])
  }
}

const selectEpisode = async (ep: RawEpisode) => {
  // Phase 4 WT-STATE-04: when mounted inside a Watch Together room,
  // route the user click through the room handle so the backend can
  // validate and broadcast to all members. The room:state_changed
  // broadcast will reactively update room.episode_id, which flows back
  // through the existing programmatic re-select path. Jimaku subtitle
  // selection is NOT routed — that's a per-user UX choice, not room state.
  if (props.room) {
    props.room.emitChangeEpisode(String(ep.id))
    return
  }
  selectedEpisode.value = ep
  loadingStream.value = true
  streamUrl.value = null
  selectedSubKey.value = ''
  subsByLang.value = null
  disposePlayer()

  // Subtitles run in parallel but are awaited independently. Jimaku occasionally
  // stalls past the gateway's 15s timeout; bundling subs into the stream's
  // Promise.all would let that rejection nuke the already-resolved streamUrl
  // and trap the user on an infinite spinner with no video.
  void (async () => {
    try {
      const subsResp = await subtitlesApi.byLang(props.animeId, ep.number, ['ja', 'en', 'ru'])
      if (selectedEpisode.value?.id !== ep.id) return
      subsByLang.value = subsResp.data?.data ?? subsResp.data
      autoSelectSubtitle()
    } catch {
      if (selectedEpisode.value?.id !== ep.id) return
      subsByLang.value = null
    }
  })()

  try {
    const streamResp = await rawApi.getStream(props.animeId, ep.number)
    if (selectedEpisode.value?.id !== ep.id) return
    const stream: RawStream = streamResp.data?.data ?? streamResp.data
    streamUrl.value = stream.url
    attachStream(stream.url, stream.type)
  } catch {
    streamUrl.value = null
  } finally {
    loadingStream.value = false
  }
  void refreshWatched()
}

const handleTimeUpdate = () => { /* placeholder for future watch-progress tracking */ }

onMounted(() => {
  void refreshWatched()
})

watch(() => props.animeId, () => {
  episodes.value = []
  selectedEpisode.value = null
  streamUrl.value = null
  disposePlayer()
  fetchEpisodes()
}, { immediate: true })

onBeforeUnmount(() => {
  disposePlayer()
})
</script>

<style scoped>
.custom-scrollbar::-webkit-scrollbar { width: 6px; height: 6px; }
.custom-scrollbar::-webkit-scrollbar-thumb { background-color: rgba(255, 255, 255, 0.2); border-radius: 3px; }
.accent-bg { background-color: var(--player-accent); }
.accent-border { border-color: var(--player-accent); }
</style>
