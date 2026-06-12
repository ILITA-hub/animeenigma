<template>
  <div class="pl-scrub-preview" data-test="scrub-preview">
    <!-- Shadow video — always mounted once initialized so seeks reuse the
         same decoder/buffer; shown only after the first frame decodes AND
         the frame is fresh (not awaiting a new HLS segment download). -->
    <video
      ref="shadowRef"
      v-show="frameReady && !frameStale"
      class="pl-scrub-preview-video"
      muted
      playsinline
      preload="metadata"
      data-test="preview-video"
      aria-hidden="true"
    />
    <!-- Static still fallback until a real fresh frame is available.
         Also shown while seeking on HLS (segment download in progress) so the
         user sees the anime thumbnail instead of a frozen stale frame. -->
    <div
      v-if="(!frameReady || frameStale) && stillUrl"
      class="pl-scrub-preview-still"
      :style="{ backgroundImage: `url(${stillUrl})` }"
      data-test="preview-still"
      aria-hidden="true"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'

/**
 * Real frame previews for the scrub-bar hover bubble.
 *
 * A muted shadow <video> plays nothing — it only seeks to the hovered time so
 * its current frame can be shown in the bubble. HLS streams get a dedicated
 * minimal hls.js instance pinned to the LOWEST level with a tiny buffer
 * (preview frames, not playback); MP4 uses the proxied URL directly (the
 * backend serves byte ranges). The engine is created lazily on the first
 * hover so users who never hover pay zero bandwidth.
 */

const props = defineProps<{
  /** hovered position in seconds */
  timeSec: number
  /** bubble visibility — gates lazy init + seeking */
  visible: boolean
  streamUrl: string | null
  streamType: 'hls' | 'mp4' | null
  /** static fallback image until the first frame decodes */
  stillUrl?: string
}>()

const shadowRef = ref<HTMLVideoElement | null>(null)
const frameReady = ref(false)
// True while an HLS seek is in-flight (segment not yet decoded). We show the
// still image during this window instead of the frozen stale frame.
const frameStale = ref(false)

const SEEK_THROTTLE_MS = 150

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let hls: any = null
let initializedFor: string | null = null
let initToken = 0

let seekTimer: ReturnType<typeof setTimeout> | null = null
let targetTime = 0
let lastSeekedTime = -1

function destroyEngine() {
  initToken++
  if (hls) {
    hls.destroy()
    hls = null
  }
  const v = shadowRef.value
  if (v) {
    v.removeAttribute('src')
    v.load()
  }
  initializedFor = null
  frameReady.value = false
  frameStale.value = false
  lastSeekedTime = -1
  if (seekTimer) {
    clearTimeout(seekTimer)
    seekTimer = null
  }
}

async function ensureEngine() {
  const { streamUrl, streamType } = props
  const v = shadowRef.value
  if (!v || !streamUrl || !streamType) return
  if (initializedFor === streamUrl) return

  destroyEngine()
  const token = initToken
  initializedFor = streamUrl

  v.addEventListener('loadeddata', onFrame)
  v.addEventListener('seeked', onFrame)

  if (streamType === 'mp4') {
    v.src = streamUrl
    return
  }

  const Hls = (await import('hls.js')).default
  if (token !== initToken) return // stream changed during import
  if (!Hls.isSupported()) {
    // Safari — native HLS handles seeking itself
    v.src = streamUrl
    return
  }
  hls = new Hls({
    enableWorker: true,
    // Preview frames only — keep the footprint tiny
    maxBufferLength: 4,
    maxMaxBufferLength: 6,
    backBufferLength: 0,
    startLevel: 0,
  })
  hls.loadSource(streamUrl)
  hls.attachMedia(v)
  hls.on(Hls.Events.MANIFEST_PARSED, () => {
    // Pin the LOWEST quality — disables ABR for the shadow instance
    if (hls) hls.currentLevel = 0
    hls?.startLoad(targetTime)
  })
}

function onFrame() {
  const v = shadowRef.value
  if (v && v.readyState >= 2) {
    frameReady.value = true
    frameStale.value = false
  }
}

function doSeek(t: number) {
  const v = shadowRef.value
  if (!v || !initializedFor) return
  if (Math.abs(t - lastSeekedTime) < 0.5) return
  lastSeekedTime = t
  // For HLS, segment download takes time — mark the current frame stale so the
  // still image shows instead of a frozen frame from a different timestamp.
  if (props.streamType === 'hls') frameStale.value = true
  v.currentTime = t
}

/** Trailing throttle — seek now, then at most once per SEEK_THROTTLE_MS. */
function requestSeek(t: number) {
  targetTime = t
  if (seekTimer) return
  doSeek(targetTime)
  seekTimer = setTimeout(() => {
    seekTimer = null
    if (Math.abs(targetTime - lastSeekedTime) >= 0.5) requestSeek(targetTime)
  }, SEEK_THROTTLE_MS)
}

watch(
  () => [props.visible, props.timeSec] as const,
  ([visible, t]) => {
    if (!visible) return
    void ensureEngine()
    requestSeek(t)
  },
)

// New stream — tear down; if the bubble is showing right now, re-arm against
// the new URL immediately (watcher order vs the hover watcher is not
// guaranteed, so this must not leave a destroyed engine behind).
watch(
  () => props.streamUrl,
  () => {
    destroyEngine()
    if (props.visible) void ensureEngine()
  },
)

onUnmounted(() => {
  const v = shadowRef.value
  if (v) {
    v.removeEventListener('loadeddata', onFrame)
    v.removeEventListener('seeked', onFrame)
  }
  destroyEngine()
})
</script>

<style scoped>
.pl-scrub-preview {
  width: 100%;
  height: 100%;
  background: black; /* video letterbox — theme-independent */
}

.pl-scrub-preview-video {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.pl-scrub-preview-still {
  width: 100%;
  height: 100%;
  background-size: cover;
  background-position: center;
}
</style>
