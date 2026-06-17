<template>
  <div class="ourenglish-player">
    <!-- Loading episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <Spinner size="lg" />
    </div>

    <!-- Provider chain exhausted -->
    <EmptyState v-else-if="!available" size="lg">
      <template #icon><Video class="size-12 opacity-50" /></template>
      {{ $t('player.ourenglish.unavailable') }}
    </EmptyState>

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
            <Spinner size="lg" class="mx-auto mb-3" />
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

        <!-- Stream failed (unrecoverable hls.js error / no playable source) -->
        <div
          v-if="streamFailed && !loadingStream"
          class="absolute inset-0 flex items-center justify-center text-white/70 px-6"
        >
          <div class="text-center max-w-sm">
            <CircleAlert class="size-14 mx-auto mb-3 text-warning/80" aria-hidden="true" />
            <p>{{ $t('player.sourceUnavailable') }}</p>
          </div>
        </div>

        <!-- Placeholder when nothing loaded -->
        <div
          v-if="!streamUrl && !loadingStream && !streamFailed"
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

      <!-- Source + Server + Subtitle toolbar -->
      <div class="flex flex-col gap-3 bg-white/5 rounded-lg p-3">
        <div class="flex flex-col sm:flex-row gap-3 sm:items-center sm:flex-wrap">
          <!-- Source provider dropdown (pins a specific scraper provider) -->
          <label class="flex items-center gap-2 text-white/70 text-sm" data-test="source-dropdown">
            <SlidersHorizontal class="size-4" aria-hidden="true" />
            {{ $t('player.ourenglish.sourceLabel') }}
          </label>
          <select
            v-model="preferredProvider"
            class="bg-white/10 hover:bg-white/15 text-white text-sm rounded-md px-3 py-1.5 border border-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50"
          >
            <option value="">{{ $t('player.ourenglish.sourceAuto') }}</option>
            <option v-for="p in availableProviders" :key="p" :value="p">
              {{ capitalizeProvider(p) }}
            </option>
          </select>

          <!-- Server picker (only shown when multiple servers for current episode) -->
          <template v-if="servers.length > 1">
            <label class="flex items-center gap-2 text-white/70 text-sm">
              {{ $t('player.ourenglish.serverLabel') }}
            </label>
            <select
              v-model="selectedServerId"
              class="bg-white/10 hover:bg-white/15 text-white text-sm rounded-md px-3 py-1.5 border border-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50"
            >
              <option v-for="s in servers" :key="s.id" :value="s.id">
                {{ s.name }}{{ s.type ? ` (${s.type})` : '' }}
              </option>
            </select>
          </template>

          <!-- Subtitle picker -->
          <label class="flex items-center gap-2 text-white/70 text-sm">
            <Captions class="size-4" aria-hidden="true" />
            {{ $t('player.subtitlePicker.label') }}
          </label>
          <select
            v-model="selectedSubKey"
            class="bg-white/10 hover:bg-white/15 text-white text-sm rounded-md px-3 py-1.5 border border-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50"
            :disabled="availableSubChoices.length === 0"
          >
            <option value="">{{ $t('player.subtitlePicker.none') }}</option>
            <option v-for="choice in availableSubChoices" :key="choice.key" :value="choice.key">
              {{ choice.label }}
            </option>
          </select>

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

        <!-- Active provider chip -->
        <div v-if="activeProvider" class="text-xs text-white/40">
          {{ $t('player.ourenglish.activeProvider', { name: capitalizeProvider(activeProvider) }) }}
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
import { Video, Play, List, SlidersHorizontal, Captions, CircleAlert } from 'lucide-vue-next'
import { Spinner, EmptyState } from '@/components/ui'
import Hls from 'hls.js'
import SubtitleOverlay from './SubtitleOverlay.vue'
import SubtitleSettingsMenu from './SubtitleSettingsMenu.vue'
import OtherSubsPanel from './OtherSubsPanel.vue'
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import type { SubtitleTrack } from '@/types/raw'
import { useSubtitleTimingOffset } from '@/composables/useSubtitleTimingOffset'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
import { scraperApi } from '@/api/client'
import { usePlayerSyncBridge } from '@/composables/usePlayerSyncBridge'
import type { WatchTogetherRoomHandle } from '@/composables/useWatchTogetherRoom'

interface ScraperEpisode {
  id: string
  number: number
  title?: string
  is_filler?: boolean
}

interface ScraperServer {
  id: string
  name: string
  type?: string // "sub" | "dub" | "raw"
}

interface ScraperSource {
  url: string
  type: string // "hls" | "mp4"
  quality?: string
}

interface ScraperTrack {
  file: string
  label?: string
  kind: string // "captions" | "subtitles"
  default?: boolean
}

interface ScraperStream {
  sources: ScraperSource[]
  tracks?: ScraperTrack[]
  headers?: Record<string, string>
}

interface ScraperEnvelope {
  episodes?: ScraperEpisode[]
  servers?: ScraperServer[]
  stream?: ScraperStream
  meta?: { tried?: string[]; provider?: string }
}

const props = defineProps<{
  animeId: string
  initialEpisode?: number
  // Phase 3 (03.2) — when set, the player sync bridge mirrors play/pause/seek
  // to the room. When null/undefined the bridge is never instantiated and the
  // player behaves exactly as it did pre-Phase-3 (zero regression).
  room?: WatchTogetherRoomHandle | null
}>()

const { offset: subtitleOffset } = useSubtitleTimingOffset()
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const playerContainer = ref<HTMLElement | null>(null)
const videoRef = ref<HTMLVideoElement | null>(null)

// Phase 3 (03.2): wire real WatchTogether sync when a room is provided. When
// room is null/undefined the bridge is never instantiated and the player
// behaves exactly as it did pre-Phase-3 (zero regression).
if (props.room) {
  usePlayerSyncBridge(videoRef, props.room)
}

const loadingEpisodes = ref(false)
const loadingStream = ref(false)
const available = ref(true)

const episodes = ref<ScraperEpisode[]>([])
const selectedEpisode = ref<ScraperEpisode | null>(null)

const episodeOptions = computed<EpisodeOption[]>(() =>
  episodes.value.map((ep) => ({
    key: ep.id,
    label: ep.number,
    number: Number(ep.number),
    isFiller: ep.is_filler,
  })),
)

function onEpisodePicked(key: string | number) {
  const ep = episodes.value.find((e) => e.id === key)
  if (ep) selectEpisode(ep)
}

const servers = ref<ScraperServer[]>([])
const selectedServerId = ref<string>('')

const streamUrl = ref<string | null>(null)
const activeTracks = ref<ScraperTrack[]>([])
// True when hls.js hit an unrecoverable fatal error. Without this the player
// swallowed every hls.js error and sat frozen at 0:00 with no feedback (part of
// the "only allanime works" symptom). Surfaces player.sourceUnavailable so the
// user knows to pick another Source instead of staring at a dead 0:00.
const streamFailed = ref(false)

// Empty string = auto (orchestrator picks); else pin to a specific provider
const preferredProvider = ref<string>('')
const activeProvider = ref<string>('')

const AVAILABLE_PROVIDERS = [
  'gogoanime',
  'animepahe',
  'allanime',
  'animefever',
  'miruro',
  'nineanime',
] as const
const availableProviders = computed(() => AVAILABLE_PROVIDERS as readonly string[])

const selectedSubKey = ref<string>('')
const otherSubsOpen = ref(false)

interface SubChoice {
  key: string
  label: string
  url: string
  format: 'ass' | 'srt' | 'vtt' | null
}

function detectFormat(format: string | undefined, url: string): 'ass' | 'srt' | 'vtt' | null {
  const ext = (format || url.split('?')[0].split('.').pop() || '').toLowerCase()
  return ext === 'ass' || ext === 'srt' || ext === 'vtt' ? ext : null
}

// Subtitle tracks pulled in via the "Other Subs" panel (Jimaku / OpenSubtitles).
// Kept separate from the embed's own `activeTracks` so an episode switch (which
// clears activeTracks) also clears these, and so each carries its explicit
// upstream `format` instead of relying on URL-extension guessing alone.
const extraSubChoices = ref<SubChoice[]>([])

const availableSubChoices = computed<SubChoice[]>(() => {
  const fromTracks = activeTracks.value
    .filter(t => t.kind === 'captions' || t.kind === 'subtitles')
    .map<SubChoice>(t => {
      const url = t.file
      const format = detectFormat(undefined, url)
      return {
        key: url,
        label: t.label || (format ? format.toUpperCase() : 'subtitle'),
        url,
        format,
      }
    })
  const seen = new Set(fromTracks.map(c => c.key))
  const extras = extraSubChoices.value.filter(c => !seen.has(c.key))
  return [...fromTracks, ...extras]
})

// "Other Subs" panel → pick a Jimaku/OpenSubtitles track. Inject a synthetic
// choice (carrying its explicit format) so the picker can display it, then pin
// it so SubtitleOverlay (via activeSubUrl/activeSubFormat) renders it.
function onOtherSubSelected(track: SubtitleTrack) {
  const url = track.url
  const format = detectFormat(track.format, url)
  if (!availableSubChoices.value.some(c => c.key === url)) {
    extraSubChoices.value = [
      ...extraSubChoices.value,
      {
        key: url,
        label: track.label || track.release || (format ? format.toUpperCase() : 'subtitle'),
        url,
        format,
      },
    ]
  }
  selectedSubKey.value = url
}

const activeSubUrl = computed(() => {
  const c = availableSubChoices.value.find(x => x.key === selectedSubKey.value)
  return c?.url ?? null
})
const activeSubFormat = computed(() => {
  const c = availableSubChoices.value.find(x => x.key === selectedSubKey.value)
  return c?.format ?? null
})

let hls: Hls | null = null
// Per-attach single-shot recovery guards so a flapping stream can't spin
// recoverMediaError()/startLoad() into an OOM loop (observed in testing).
let netRecoverDone = false
let mediaRecoverDone = false

function capitalizeProvider(name: string): string {
  switch (name) {
    case 'animepahe': return 'AnimePahe'
    case 'gogoanime': return 'Gogoanime'
    case 'allanime': return 'AllAnime'
    case 'animefever': return 'AnimeFever'
    case 'miruro': return 'Miruro'
    case 'nineanime': return '9anime'
    case 'animekai': return 'AnimeKai'
    default: return name.charAt(0).toUpperCase() + name.slice(1)
  }
}

function buildProxyUrl(url: string, referer: string, streamType: 'hls' | 'mp4'): string {
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

async function attachStream(url: string, type: 'hls' | 'mp4', referer: string) {
  // Defense-in-depth: the <video> may not be mounted yet on the first load
  // (it lives behind v-else / v-show). Wait one render tick for the ref before
  // giving up — a null ref here means the stream silently never attaches and
  // the player freezes at 0:00.
  if (!videoRef.value) await nextTick()
  const v = videoRef.value
  if (!v) return
  disposePlayer()
  streamFailed.value = false
  netRecoverDone = false
  mediaRecoverDone = false

  if (type === 'mp4') {
    v.src = buildProxyUrl(url, referer, 'mp4')
    // Surface a hard MP4 failure (e.g. upstream 502) instead of a silent 0:00.
    v.addEventListener('error', () => { streamFailed.value = true }, { once: true })
    v.play().catch(() => { /* user-gesture required */ })
    return
  }

  const proxyUrl = buildProxyUrl(url, referer, 'hls')
  if (Hls.isSupported()) {
    hls = new Hls({ enableWorker: true, backBufferLength: 90 })
    hls.loadSource(proxyUrl)
    hls.attachMedia(v)
    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      v.play().catch(() => { /* ignore */ })
    })
    // Without this handler hls.js fatal errors are swallowed and the player
    // freezes at 0:00 with no feedback. Attempt ONE network/media recovery,
    // then give up and surface the failure so the user can switch Source.
    hls.on(Hls.Events.ERROR, (_evt, data) => {
      if (!data.fatal) return
      if (data.type === Hls.ErrorTypes.NETWORK_ERROR && !netRecoverDone) {
        netRecoverDone = true
        hls?.startLoad()
        return
      }
      if (data.type === Hls.ErrorTypes.MEDIA_ERROR && !mediaRecoverDone) {
        mediaRecoverDone = true
        hls?.recoverMediaError()
        return
      }
      streamFailed.value = true
      disposePlayer()
    })
  } else if (v.canPlayType('application/vnd.apple.mpegurl')) {
    v.src = proxyUrl
    v.addEventListener('loadedmetadata', () => {
      v.play().catch(() => { /* ignore */ })
    }, { once: true })
  }
}

async function fetchEpisodes() {
  loadingEpisodes.value = true
  let startEp: ScraperEpisode | null = null
  let fromRoomSync = false
  try {
    const prefer = preferredProvider.value || undefined
    const resp = await scraperApi.getEpisodes(props.animeId, prefer)
    const env = resp.data?.data as ScraperEnvelope | undefined
    const eps = env?.episodes ?? []
    episodes.value = eps
    available.value = eps.length > 0
    // Pin the provider that ACTUALLY produced this episode list (meta.provider).
    // Episode/server IDs are opaque + provider-specific, so servers/stream MUST
    // hit the same provider. meta.tried is only the ordered candidate list — its
    // last entry is NOT the winner (it's the lowest-priority fallback), which is
    // why the previous `tried[tried.length - 1]` pin broke playback by forcing a
    // mismatched provider. Empty => fall back to auto (also correct).
    activeProvider.value = env?.meta?.provider ?? ''

    if (available.value) {
      startEp =
        props.initialEpisode != null
          ? eps.find(e => e.number === props.initialEpisode) ?? eps[0]
          : eps[0]
      // WT-STATE-04: a guest joining an existing room (or a host whose room
      // already has an episode set) must load the stream directly on mount —
      // there is no incoming room echo to react to. fromRoomSync=true bypasses
      // the emit-to-room guard so the stream loads immediately.
      fromRoomSync = !!(props.room && props.initialEpisode != null)
    }
  } catch {
    available.value = false
  } finally {
    loadingEpisodes.value = false
  }

  // CRITICAL: auto-select AFTER loadingEpisodes is false + a render tick.
  // The <video> element lives in the v-else branch that only renders once
  // loadingEpisodes is false. If we select (and attachStream) while
  // loadingEpisodes is still true, videoRef is null, attachStream early-returns,
  // and the stream never attaches — the player sits frozen at 0:00 even though
  // the stream resolved. Deferring past nextTick guarantees the element + ref
  // exist before attachStream runs.
  if (startEp) {
    await nextTick()
    await selectEpisode(startEp, fromRoomSync)
  }
}

async function selectEpisode(ep: ScraperEpisode, fromRoomSync = false) {
  // Phase 4 WT-STATE-04: when mounted inside a Watch Together room,
  // route the user click through the room handle so the backend can
  // validate and broadcast to all members. The room:state_changed
  // broadcast will reactively update room.episode_id, which flows
  // back through the existing :initial-episode prop -> the watcher below
  // -> selectEpisode(ep, true). The fromRoomSync flag marks that
  // programmatic re-entry so we load the stream instead of re-emitting
  // (which would loop forever).
  if (props.room && !fromRoomSync) {
    props.room.emitChangeEpisode(String(ep.number))
    return
  }
  selectedEpisode.value = ep
  void refreshWatched()
  loadingStream.value = true
  streamUrl.value = null
  streamFailed.value = false
  servers.value = []
  selectedServerId.value = ''
  selectedSubKey.value = ''
  activeTracks.value = []
  extraSubChoices.value = []
  disposePlayer()
  try {
    const prefer = activeProvider.value || preferredProvider.value || undefined
    const sResp = await scraperApi.getServers(props.animeId, ep.id, prefer)
    const sEnv = sResp.data?.data as ScraperEnvelope | undefined
    const srvs = sEnv?.servers ?? []
    servers.value = srvs
    if (srvs.length === 0) {
      streamUrl.value = null
      streamFailed.value = true
      return
    }
    // Prefer sub > raw > dub for initial pick
    const sub = srvs.find(s => s.type === 'sub')
    selectedServerId.value = (sub ?? srvs[0]).id
    await loadStream()
  } catch {
    streamUrl.value = null
    streamFailed.value = true
  } finally {
    loadingStream.value = false
  }
}

async function loadStream() {
  const ep = selectedEpisode.value
  const srv = servers.value.find(s => s.id === selectedServerId.value)
  if (!ep || !srv) return
  const prefer = activeProvider.value || preferredProvider.value || undefined
  const category: 'sub' | 'dub' = srv.type === 'dub' ? 'dub' : 'sub'
  try {
    const resp = await scraperApi.getStream(props.animeId, ep.id, srv.id, category, prefer)
    const env = resp.data?.data as ScraperEnvelope | undefined
    const stream = env?.stream
    if (!stream || !stream.sources?.length) {
      streamUrl.value = null
      streamFailed.value = true
      return
    }
    const source = stream.sources[0]
    streamUrl.value = source.url
    activeTracks.value = stream.tracks ?? []
    const referer = stream.headers?.Referer || stream.headers?.referer || ''
    const type: 'hls' | 'mp4' = source.type === 'mp4' ? 'mp4' : 'hls'
    await attachStream(source.url, type, referer)
    // Auto-pick a default subtitle track if upstream marked one
    const def = activeTracks.value.find(t => t.default)
    if (def) selectedSubKey.value = def.file
  } catch {
    streamUrl.value = null
    streamFailed.value = true
  }
}

function handleTimeUpdate() {
  /* placeholder for future watch-progress tracking */
}

watch(() => props.animeId, () => {
  episodes.value = []
  selectedEpisode.value = null
  streamUrl.value = null
  disposePlayer()
  fetchEpisodes()
}, { immediate: true })

// Re-fetch when user pins a different source provider
watch(preferredProvider, () => {
  fetchEpisodes()
})

// Switching server inside the same episode just re-loads stream
watch(selectedServerId, (next, prev) => {
  if (next && prev && next !== prev) {
    loadingStream.value = true
    loadStream().finally(() => { loadingStream.value = false })
  }
})

// WT-STATE-04: react to room state_changed broadcasts. When the room's current
// episode changes (this member's own click echo, or another member's change),
// the parent updates the :initial-episode prop. Load the matching stream with
// fromRoomSync=true so we don't re-emit to the room (which would loop forever).
watch(() => props.initialEpisode, (epNum) => {
  if (!props.room || !epNum || episodes.value.length === 0) return
  const ep = episodes.value.find(e => e.number === epNum)
  if (ep) selectEpisode(ep, true)
})

onMounted(() => {
  void refreshWatched()
})

onBeforeUnmount(() => {
  disposePlayer()
})
</script>

<style scoped>
.ourenglish-player {
  --player-accent: #22d3ee;
  --player-accent-rgb: 34, 211, 238;
}
.custom-scrollbar::-webkit-scrollbar { width: 6px; height: 6px; }
.custom-scrollbar::-webkit-scrollbar-thumb { background-color: var(--white-a20); border-radius: 3px; }
.accent-bg { background-color: var(--player-accent); }
.accent-border { border-color: var(--player-accent); }
</style>
