<template>
  <div class="ourenglish-player">
    <!-- Loading episodes -->
    <div v-if="loadingEpisodes" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 accent-border border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- Provider chain exhausted -->
    <div
      v-else-if="!available"
      class="text-center py-16 text-white/60"
    >
      <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
      </svg>
      {{ $t('player.ourenglish.unavailable') }}
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

        <!-- Stream failed (unrecoverable hls.js error / no playable source) -->
        <div
          v-if="streamFailed && !loadingStream"
          class="absolute inset-0 flex items-center justify-center text-white/70 px-6"
        >
          <div class="text-center max-w-sm">
            <svg class="w-14 h-14 mx-auto mb-3 text-warning/80" fill="none" stroke="currentColor" stroke-width="1.5" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
            </svg>
            <p>{{ $t('player.sourceUnavailable') }}</p>
          </div>
        </div>

        <!-- Placeholder when nothing loaded -->
        <div
          v-if="!streamUrl && !loadingStream && !streamFailed"
          class="absolute inset-0 flex items-center justify-center text-white/40"
        >
          <div class="text-center">
            <svg class="w-16 h-16 mx-auto mb-3" fill="currentColor" viewBox="0 0 24 24">
              <path d="M8 5v14l11-7z" />
            </svg>
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
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 17V7m0 10a2 2 0 01-2 2H5a2 2 0 01-2-2V7a2 2 0 012-2h2a2 2 0 012 2m0 10a2 2 0 002 2h2a2 2 0 002-2M9 7a2 2 0 012-2h2a2 2 0 012 2m0 10V7m0 10a2 2 0 002 2h2a2 2 0 002-2V7a2 2 0 00-2-2h-2a2 2 0 00-2 2" />
            </svg>
            {{ $t('player.ourenglish.sourceLabel') }}
          </label>
          <select
            v-model="preferredProvider"
            class="bg-white/10 hover:bg-white/15 text-white text-sm rounded-md px-3 py-1.5 border border-white/10 focus:outline-none focus:ring-2 focus:ring-cyan-400/40"
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
              class="bg-white/10 hover:bg-white/15 text-white text-sm rounded-md px-3 py-1.5 border border-white/10 focus:outline-none focus:ring-2 focus:ring-cyan-400/40"
            >
              <option v-for="s in servers" :key="s.id" :value="s.id">
                {{ s.name }}{{ s.type ? ` (${s.type})` : '' }}
              </option>
            </select>
          </template>

          <!-- Subtitle picker -->
          <label class="flex items-center gap-2 text-white/70 text-sm">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 15a2 2 0 01-2 2H5a2 2 0 01-2-2V5a2 2 0 012-2h14a2 2 0 012 2v10z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 12h2m4 0h4M7 8h4m4 0h2" />
            </svg>
            {{ $t('player.subtitlePicker.label') }}
          </label>
          <select
            v-model="selectedSubKey"
            class="bg-white/10 hover:bg-white/15 text-white text-sm rounded-md px-3 py-1.5 border border-white/10 focus:outline-none focus:ring-2 focus:ring-cyan-400/40"
            :disabled="availableSubChoices.length === 0"
          >
            <option value="">{{ $t('player.subtitlePicker.none') }}</option>
            <option v-for="choice in availableSubChoices" :key="choice.key" :value="choice.key">
              {{ choice.label }}
            </option>
          </select>

          <SubtitleSettingsMenu :has-active-sub="!!activeSubUrl" />
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
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
            </svg>
            {{ $t('player.episodesCount', { count: episodes.length }) }}
          </h3>
          <slot name="header-middle" />
        </div>
        <div class="flex flex-wrap gap-2 max-h-32 overflow-y-auto custom-scrollbar p-1">
          <button
            v-for="ep in episodes"
            :key="ep.id"
            class="w-12 h-10 rounded-lg text-sm font-medium transition-all"
            :class="selectedEpisode?.id === ep.id
              ? 'accent-bg text-white'
              : 'bg-white/10 text-white hover:bg-white/20'"
            :title="ep.title || `Episode ${ep.number}`"
            @click="selectEpisode(ep)"
          >
            {{ ep.number }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Hls from 'hls.js'
import SubtitleOverlay from './SubtitleOverlay.vue'
import SubtitleSettingsMenu from './SubtitleSettingsMenu.vue'
import { useSubtitleTimingOffset } from '@/composables/useSubtitleTimingOffset'
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

interface SubChoice {
  key: string
  label: string
  url: string
  format: 'ass' | 'srt' | 'vtt' | null
}

const availableSubChoices = computed<SubChoice[]>(() =>
  activeTracks.value
    .filter(t => t.kind === 'captions' || t.kind === 'subtitles')
    .map(t => {
      const url = t.file
      const ext = (url.split('?')[0].split('.').pop() || '').toLowerCase()
      const format: 'ass' | 'srt' | 'vtt' | null =
        ext === 'ass' || ext === 'srt' || ext === 'vtt' ? ext : null
      return {
        key: url,
        label: t.label || (format ? format.toUpperCase() : 'subtitle'),
        url,
        format,
      }
    }),
)

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
  loadingStream.value = true
  streamUrl.value = null
  streamFailed.value = false
  servers.value = []
  selectedServerId.value = ''
  selectedSubKey.value = ''
  activeTracks.value = []
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

onBeforeUnmount(() => {
  disposePlayer()
})
</script>

<style scoped>
.custom-scrollbar::-webkit-scrollbar { width: 6px; height: 6px; }
.custom-scrollbar::-webkit-scrollbar-thumb { background-color: rgba(255, 255, 255, 0.2); border-radius: 3px; }
.accent-bg { background-color: rgb(34, 211, 238); }
.accent-border { border-color: rgb(34, 211, 238); }
</style>
