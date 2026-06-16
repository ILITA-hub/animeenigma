<template>
  <div class="pl-scrub-preview" data-test="scrub-preview">
    <!-- Hidden shadow video — never displayed; it only decodes frames that get
         captured into the thumbnail cache below. -->
    <video
      ref="shadowRef"
      class="pl-scrub-preview-shadow"
      muted
      playsinline
      preload="metadata"
      data-test="preview-video"
      aria-hidden="true"
    />
    <!-- Thumbnail canvas — shows the nearest CACHED frame for the hovered
         time instantly (no network), refined to the exact frame once the
         settle-seek decodes it. -->
    <canvas
      v-show="hasFrame"
      ref="canvasRef"
      width="192"
      height="108"
      class="pl-scrub-preview-canvas"
      data-test="preview-canvas"
      aria-hidden="true"
    />
    <!-- Static still fallback only until the very first frame is cached. -->
    <div
      v-if="!hasFrame && stillUrl"
      class="pl-scrub-preview-still"
      :style="{ backgroundImage: `url(${stillUrl})` }"
      data-test="preview-still"
      aria-hidden="true"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'
import { scrubDebug, slog, srecordCapture, sreset } from '@/composables/unifiedPlayer/scrubPreviewDebug'

/**
 * Real frame previews for the scrub-bar hover bubble — thumbnail-cache design.
 *
 * v1 of this component live-seeked a shadow <video> on every hover move. That
 * was network-bound end to end: each position needed a fragment/byte-range
 * fetch through the HLS proxy (0.5–3s), the next throttled seek aborted the
 * previous fetch while the pointer moved, and `backBufferLength: 0` evicted
 * every decoded frame immediately — so no frame ever survived and the bubble
 * showed one fallback image everywhere ("laggy, single frame" bug).
 *
 * v2 decouples display from the network:
 *  - every frame the shadow video decodes is CAPTURED into a tiny canvas,
 *    keyed by a 5-second time bucket (LRU, ~150 entries ≈ 12 min of video);
 *  - hovering renders the nearest cached thumbnail instantly — zero network;
 *  - the shadow video seeks only when the pointer SETTLES (180ms), so moving
 *    across the bar no longer abort-storms the loader;
 *  - after init, ~9 evenly-spaced timeline points are prefetched in the
 *    background (lowest HLS level, ~100-300KB each), so the whole bar has
 *    distinct frames within seconds of the first hover.
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

const THUMB_W = 192
const THUMB_H = 108
/** thumbnail granularity — one cached frame per 5s of video */
const BUCKET_SEC = 5
/** LRU cap — 150 × 192×108×4B ≈ 12 MB worst case */
const CACHE_MAX = 150
/** pointer-rest debounce before issuing a real (network) seek */
const SETTLE_MS = 180
/** evenly-spaced timeline points prefetched in the background */
const PREFETCH_POINTS = 50
/** a stuck seek (failed fragment) must not wedge the prefetch pump */
const SEEK_WATCHDOG_MS = 8000
/** eager-init delay after a stream loads — the MAIN player wins startup
 *  bandwidth, then the preview warms its 50 thumbnails in the background */
const EAGER_INIT_DELAY_MS = 3500
/** pump retry cadence while the user's hover blocks background prefetch */
const PUMP_RETRY_MS = 500

const shadowRef = ref<HTMLVideoElement | null>(null)
const canvasRef = ref<HTMLCanvasElement | null>(null)
const hasFrame = ref(false)

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let hls: any = null
let initializedFor: string | null = null
let initToken = 0

/** bucket → captured thumbnail. Map iteration order doubles as LRU order. */
let cache = new Map<number, HTMLCanvasElement>()
let currentBucket = 0
/** bucket a seek was issued FOR — decoded frames alias-cache under it, since
 *  HLS keyframe snapping can land currentTime in a neighbouring bucket and a
 *  miss there would refetch the same spot forever. */
let pendingBucket: number | null = null
let seekBusy = false
let settleTimer: ReturnType<typeof setTimeout> | null = null
let watchdogTimer: ReturnType<typeof setTimeout> | null = null
let eagerTimer: ReturnType<typeof setTimeout> | null = null
let pumpTimer: ReturnType<typeof setTimeout> | null = null
let prefetchQueue: number[] = []
let prefetchArmed = false
/** performance.now() when the in-flight seek was issued — capture latency */
let seekIssuedAt: number | null = null
let prefetchCompleteLogged = false

function bucketOf(t: number): number {
  return Math.max(0, Math.round(t / BUCKET_SEC))
}

function bucketTime(b: number): number {
  return Math.max(0.1, b * BUCKET_SEC)
}

function nearestCached(b: number): HTMLCanvasElement | null {
  const exact = cache.get(b)
  if (exact) return exact
  let best: HTMLCanvasElement | null = null
  let bestDist = Infinity
  for (const [k, c] of cache) {
    const d = Math.abs(k - b)
    if (d < bestDist) {
      bestDist = d
      best = c
    }
  }
  return best
}

/** Draw the best available thumbnail for the hovered bucket. No network. */
function render() {
  const thumb = nearestCached(currentBucket)
  if (!thumb) {
    hasFrame.value = false
    return
  }
  const ctx = canvasRef.value?.getContext('2d')
  if (ctx) ctx.drawImage(thumb, 0, 0, THUMB_W, THUMB_H)
  hasFrame.value = true
}

/** Capture the shadow video's current frame into the bucket cache. */
function capture() {
  const v = shadowRef.value
  if (!v || v.readyState < 2) {
    slog(`capture skipped: rs=${v?.readyState ?? '∅'} (frame not decodable yet)`)
    return
  }
  const b = bucketOf(v.currentTime)
  let c = cache.get(b)
  if (c) {
    cache.delete(b) // refresh LRU position
  } else {
    c = document.createElement('canvas')
    c.width = THUMB_W
    c.height = THUMB_H
  }
  const ctx = c.getContext('2d')
  if (ctx && v.videoWidth > 0) ctx.drawImage(v, 0, 0, THUMB_W, THUMB_H)
  cache.set(b, c)
  if (pendingBucket !== null && pendingBucket !== b) {
    cache.set(pendingBucket, c) // keyframe-snap alias (same thumbnail)
  }
  pendingBucket = null
  while (cache.size > CACHE_MAX) {
    const oldest = cache.keys().next().value
    if (oldest === undefined) break
    cache.delete(oldest)
  }
  seekBusy = false
  if (watchdogTimer) {
    clearTimeout(watchdogTimer)
    watchdogTimer = null
  }
  const ms = seekIssuedAt !== null ? performance.now() - seekIssuedAt : null
  seekIssuedAt = null
  srecordCapture(ms)
  scrubDebug.cacheSize = cache.size
  scrubDebug.engine = 'ready'
  slog(
    `captured b${b}${ms !== null ? ` in ${Math.round(ms)}ms` : ' (initial)'}` +
      ` · cache=${cache.size} queue=${prefetchQueue.length}`,
  )
  render()
  armPrefetch()
  pumpPrefetch()
}

/** Issue a real seek on the shadow video (network-bound). */
function seekTo(t: number, reason: 'hover' | 'prefetch' = 'hover') {
  const v = shadowRef.value
  if (!v || !initializedFor) {
    slog(`seek →${Math.round(t)}s dropped: engine not initialized`)
    return
  }
  pendingBucket = bucketOf(t)
  seekBusy = true
  seekIssuedAt = performance.now()
  scrubDebug.seeks++
  slog(`seek →${Math.round(t)}s b${pendingBucket} (${reason}) rs=${v.readyState}`)
  if (watchdogTimer) clearTimeout(watchdogTimer)
  watchdogTimer = setTimeout(() => {
    // Failed/stalled fragment — unblock the pump rather than wedge forever.
    scrubDebug.watchdogs++
    slog(
      `WATCHDOG: seek b${pendingBucket} got no frame in ${SEEK_WATCHDOG_MS / 1000}s — ` +
        `provider stall or dead engine (errors=${scrubDebug.errors})`,
    )
    seekBusy = false
    seekIssuedAt = null
    watchdogTimer = null
    pumpPrefetch()
  }, SEEK_WATCHDOG_MS)
  v.currentTime = t
}

/** Hover handler: instant cached render + settle-debounced refinement. */
function onHover(t: number) {
  currentBucket = bucketOf(t)
  render()
  if (settleTimer) {
    clearTimeout(settleTimer)
    settleTimer = null
  }
  if (cache.has(currentBucket)) {
    scrubDebug.hoverHits++
    return // exact frame already on screen
  }
  scrubDebug.hoverMisses++
  settleTimer = setTimeout(() => {
    settleTimer = null
    seekTo(bucketTime(currentBucket), 'hover')
  }, SETTLE_MS)
}

// ── Background prefetch: seed the timeline with PREFETCH_POINTS thumbnails so
//    hovering ANY position shows a frame from roughly the right part of the
//    video. The pump is TIMER-DRIVEN: if the user's hover blocks it (their
//    seek always wins), it retries on its own instead of waiting for the next
//    capture — a capture-driven pump stalls forever the moment one slot is
//    skipped, which is exactly the "only shows the last cached pic" bug.

function armPrefetch() {
  if (prefetchArmed) return
  const dur = shadowRef.value?.duration
  if (!dur || !Number.isFinite(dur) || dur < BUCKET_SEC * 4) return
  prefetchArmed = true
  for (let i = 1; i <= PREFETCH_POINTS; i++) {
    const b = bucketOf((dur * i) / (PREFETCH_POINTS + 1))
    if (!cache.has(b)) prefetchQueue.push(b)
  }
  scrubDebug.queueLen = prefetchQueue.length
  slog(`prefetch armed: ${prefetchQueue.length} points over ${Math.round(dur)}s`)
}

function schedulePump(delayMs: number = PUMP_RETRY_MS) {
  if (pumpTimer) return
  pumpTimer = setTimeout(() => {
    pumpTimer = null
    pumpPrefetch()
  }, delayMs)
}

function pumpPrefetch() {
  scrubDebug.queueLen = prefetchQueue.length
  if (prefetchQueue.length === 0) {
    if (prefetchArmed && !prefetchCompleteLogged) {
      prefetchCompleteLogged = true
      slog(`prefetch complete · cache=${cache.size}`)
    }
    return
  }
  if (seekBusy || settleTimer) {
    schedulePump() // busy with the user's hover — come back, don't stall
    return
  }
  let next: number | undefined
  while ((next = prefetchQueue.shift()) !== undefined) {
    if (!cache.has(next)) break
  }
  scrubDebug.queueLen = prefetchQueue.length
  if (next === undefined) return
  seekTo(bucketTime(next), 'prefetch')
}

/** loadedmetadata — duration is known; arm and start the background warm-up. */
function onMeta() {
  slog(`metadata: dur=${Math.round(shadowRef.value?.duration ?? 0)}s`)
  armPrefetch()
  pumpPrefetch()
}

// ── Engine lifecycle ─────────────────────────────────────────────────────────

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
  hasFrame.value = false
  seekBusy = false
  pendingBucket = null
  seekIssuedAt = null
  prefetchArmed = false
  prefetchCompleteLogged = false
  prefetchQueue = []
  cache = new Map()
  scrubDebug.engine = 'idle'
  if (settleTimer) {
    clearTimeout(settleTimer)
    settleTimer = null
  }
  if (watchdogTimer) {
    clearTimeout(watchdogTimer)
    watchdogTimer = null
  }
  if (pumpTimer) {
    clearTimeout(pumpTimer)
    pumpTimer = null
  }
  if (eagerTimer) {
    clearTimeout(eagerTimer)
    eagerTimer = null
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
  sreset()
  scrubDebug.engine = 'loading'
  scrubDebug.streamType = streamType
  slog(`init ${streamType} · ${streamUrl.slice(0, 96)}`)

  v.addEventListener('loadeddata', capture)
  v.addEventListener('seeked', capture)
  v.addEventListener('loadedmetadata', onMeta)
  v.addEventListener('error', onMediaError)

  if (streamType === 'mp4') {
    v.src = streamUrl
    return
  }

  const Hls = (await import('hls.js')).default
  if (token !== initToken) return // stream changed during import
  if (!Hls.isSupported()) {
    // Safari — native HLS handles seeking itself
    slog('MSE unsupported — native HLS path (Safari)')
    v.src = streamUrl
    return
  }
  hls = new Hls({
    enableWorker: true,
    // Tiny live buffer is fine — decoded frames persist in the canvas cache.
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
    hls?.startLoad(0)
    slog(`manifest: ${hls?.levels?.length ?? '?'} levels · pinned L0 (${hls?.levels?.[0]?.height ?? '?'}p)`)
  })
  // Fragment loads ARE the provider cost — one line each (only seeks trigger
  // them here, the 4s buffer cap stops continuous loading).
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  hls.on(Hls.Events.FRAG_LOADED, (_e: string, data: any) => {
    const ms = Math.round(data?.frag?.stats ? data.frag.stats.loading.end - data.frag.stats.loading.start : 0)
    const kb = Math.round((data?.frag?.stats?.total ?? 0) / 1024)
    scrubDebug.lastFragMs = ms
    scrubDebug.lastFragKb = kb
    slog(`frag ${kb}KB in ${ms}ms @${Math.round(data?.frag?.start ?? 0)}s`)
  })
  // The shadow engine had NO error listener — a fatal hls error silently
  // killed it and every later hover showed the last cached frame forever.
  // Log everything; on fatal, mark the engine dead so the HUD says WHY.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  hls.on(Hls.Events.ERROR, (_e: string, data: any) => {
    scrubDebug.errors++
    const desc = `${data?.type ?? '?'}/${data?.details ?? '?'}${data?.fatal ? ' FATAL' : ''}` +
      (data?.response?.code ? ` http=${data.response.code}` : '')
    scrubDebug.lastError = desc
    slog(`hls error: ${desc}`)
    if (data?.fatal) scrubDebug.engine = 'error'
  })
}

function onMediaError() {
  const err = shadowRef.value?.error
  scrubDebug.errors++
  scrubDebug.lastError = `media code=${err?.code ?? '?'} ${err?.message ?? ''}`.trim()
  scrubDebug.engine = 'error'
  slog(`media error: ${scrubDebug.lastError}`)
}

watch(
  () => [props.visible, props.timeSec] as const,
  ([visible, t]) => {
    if (!visible) return
    void ensureEngine()
    onHover(t)
  },
)

// New stream — tear down (cache frames belong to the old video). Re-arm
// immediately if the bubble is showing; otherwise EAGERLY after a short
// delay, so the 50-point thumbnail warm-up runs before the first hover
// instead of being gated on it. `immediate` covers the initial mount.
watch(
  () => props.streamUrl,
  () => {
    destroyEngine()
    if (!props.streamUrl) return
    if (props.visible) {
      void ensureEngine()
      return
    }
    eagerTimer = setTimeout(() => {
      eagerTimer = null
      void ensureEngine()
    }, EAGER_INIT_DELAY_MS)
  },
  { immediate: true },
)

onUnmounted(() => {
  const v = shadowRef.value
  if (v) {
    v.removeEventListener('loadeddata', capture)
    v.removeEventListener('seeked', capture)
    v.removeEventListener('loadedmetadata', onMeta)
    v.removeEventListener('error', onMediaError)
  }
  destroyEngine()
})
</script>

<style scoped>
.pl-scrub-preview {
  width: 100%;
  height: 100%;
  background: black; /* video letterbox — theme-independent */
  position: relative;
}

.pl-scrub-preview-shadow {
  display: none;
}

.pl-scrub-preview-canvas {
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
