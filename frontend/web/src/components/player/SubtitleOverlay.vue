<template>
  <Teleport :to="teleportTarget || 'body'" :disabled="!teleportTarget">
    <div
      v-if="visible && activeCues.length > 0"
      class="absolute inset-0 pointer-events-none overflow-hidden"
      :class="teleportTarget ? 'z-[2147483647]' : 'z-20'"
    >
      <div
        v-for="(cue, index) in activeCues"
        :key="index"
        class="absolute pointer-events-auto select-text cursor-text px-3 py-1"
        :style="cuePositionStyle(cue, getCueStackIndex(cue, index))"
      >
        <span
          class="inline-block px-2 py-0.5 rounded subtitle-text"
          :style="cueTextStyle(cue)"
          v-html="cue.html"
        />
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, shallowRef, watch, onMounted, onUnmounted, computed } from 'vue'
import type { SubtitleCue } from '@/utils/subtitle-parser'
import { fetchAndParseCues } from '@/utils/subtitle-parser'
import { subtitleBgColor } from '@/utils/subtitleStyle'

const props = withDefaults(defineProps<{
  videoElement: HTMLVideoElement | null
  subtitleUrl: string | null
  format: 'ass' | 'srt' | 'vtt' | null
  visible: boolean
  fullscreenContainer?: HTMLElement | null
  /**
   * When set, the overlay is teleported into this element in WINDOWED mode too
   * (not just fullscreen), rendered as the container's last child at max
   * z-index. Opt-in for players whose in-place render is defeated by a child
   * component's stacking context (AePlayer): the in-place `z-20` overlay
   * is invisible there even though it out-numbers every player layer, but
   * teleporting into the player root — the exact path that already works in
   * fullscreen — puts it reliably on top. Legacy players omit this prop and
   * keep their unchanged in-place windowed behavior.
   */
  windowedContainer?: HTMLElement | null
  /** Timing offset in seconds. Positive = show subtitles later, negative = earlier. */
  offset?: number
  /**
   * User "Text size" preference as a percentage of the auto-computed base size
   * (100 = auto/unchanged). Scales every cue, including ASS cues with an explicit
   * font size, so the appearance slider always has an effect.
   */
  sizeScale?: number
  /**
   * User "Background" opacity preference, 0–100 (%). Mapped to a black-box alpha
   * via ×0.85 — the exact formula the appearance-panel live preview uses — so the
   * preview matches what renders over the video.
   */
  bgOpacity?: number
}>(), {
  fullscreenContainer: null,
  windowedContainer: null,
  offset: 0,
  sizeScale: 100,
  bgOpacity: 45,
})

const emit = defineEmits<{
  (e: 'loading', loading: boolean): void
  (e: 'error', msg: string): void
}>()

// shallowRef: the cue array is immutable after parse and replaced wholesale on
// every load, so per-object deep reactivity over 1000+ cues is pure overhead —
// activeCues only needs to re-run when the array reference changes.
const cues = shallowRef<SubtitleCue[]>([])
const currentTime = ref(0)
let animFrameId: number | null = null

// Dynamic font size based on video dimensions
const baseFontSize = ref(19)

function updateBaseFontSize() {
  const height = props.videoElement?.clientHeight || 400
  baseFontSize.value = Math.max(16, Math.min(48, height * 0.035))
}

// Fullscreen detection for Teleport
const fullscreenEl = ref<Element | null>(null)

// Where the overlay's DOM is teleported. Fullscreen element wins; otherwise the
// opt-in windowedContainer (AePlayer) so the overlay renders as that
// container's last child at max z-index in windowed mode too. Null → Teleport
// disabled → in-place render at z-20 (legacy players' unchanged behavior).
const teleportTarget = computed<Element | null>(() => fullscreenEl.value || props.windowedContainer || null)

function onFullscreenChange() {
  const el = document.fullscreenElement || (document as Document & { webkitFullscreenElement?: Element }).webkitFullscreenElement || null
  if (el && el.tagName === 'VIDEO') {
    // Native <video> went fullscreen — subtitles can't render inside it.
    // Redirect fullscreen to the parent container so the overlay stays visible.
    // Call requestFullscreen on parent directly (replaces current fullscreen element).
    const container = el.parentElement
    if (container) {
      container.requestFullscreen().catch(() => {
        // Redirect failed (no user activation) — use the prop fallback
        // so the Teleport can still attach subtitles over the fullscreen video.
        if (props.fullscreenContainer) {
          fullscreenEl.value = props.fullscreenContainer
          setTimeout(updateBaseFontSize, 100)
        }
      })
    } else if (props.fullscreenContainer) {
      fullscreenEl.value = props.fullscreenContainer
      setTimeout(updateBaseFontSize, 100)
    }
    return
  }
  if (el) {
    fullscreenEl.value = el
  } else {
    fullscreenEl.value = null
  }
  // Recalculate font size after fullscreen change
  setTimeout(updateBaseFontSize, 100)
}

// Find active cues for current time, sorted by layer.
// Apply timing offset: positive offset = show subtitles later (shift the
// visibility window forward in time), so we subtract it from currentTime.
const activeCues = computed(() => {
  if (!cues.value.length) return []
  const t = currentTime.value - props.offset
  return cues.value
    .filter(c => t >= c.start && t <= c.end)
    .sort((a, b) => (a.style?.layer || 0) - (b.style?.layer || 0))
})

// Count stacking offset for overlapping cues at the same alignment.
// For bottom-aligned cues (1-3): count cues AFTER this one so the first cue
// in the array appears highest and the last sits at the bottom margin —
// this preserves top-to-bottom reading order.
// For top/middle cues: count cues BEFORE so they stack downward/outward.
function getCueStackIndex(cue: SubtitleCue, index: number): number {
  if (cue.style?.pos) return 0 // absolute positioned cues don't stack
  const alignment = cue.style?.alignment || 2
  const isBottom = alignment <= 3
  let stackIndex = 0
  if (isBottom) {
    for (let i = index + 1; i < activeCues.value.length; i++) {
      const other = activeCues.value[i]
      if (!other.style?.pos && (other.style?.alignment || 2) === alignment) {
        stackIndex++
      }
    }
  } else {
    for (let i = 0; i < index; i++) {
      const other = activeCues.value[i]
      if (!other.style?.pos && (other.style?.alignment || 2) === alignment) {
        stackIndex++
      }
    }
  }
  return stackIndex
}

// Position style based on ASS alignment, with vertical stacking for overlapping cues
function cuePositionStyle(cue: SubtitleCue, stackIndex: number = 0): Record<string, string> {
  // Handle \pos() — absolute positioning
  if (cue.style?.pos) {
    const [x, y] = cue.style.pos
    // Convert ASS pixel coords to percentages (standard PlayRes is ~640x480)
    const videoEl = props.videoElement
    const w = videoEl?.videoWidth || 640
    const h = videoEl?.videoHeight || 480
    return {
      left: `${(x / w) * 100}%`,
      top: `${(y / h) * 100}%`,
      maxWidth: '90%',
    }
  }

  const alignment = cue.style?.alignment || 2 // Default: bottom center

  // ASS numpad alignment:
  // 7=TL 8=TC 9=TR
  // 4=ML 5=MC 6=MR
  // 1=BL 2=BC 3=BR
  const style: Record<string, string> = {}

  // Stack offset for overlapping cues at same alignment
  const lineHeight = baseFontSize.value * 1.8
  const stackOffset = stackIndex * lineHeight

  // Vertical position with stacking
  if (alignment >= 7) {
    // Top-aligned: stack downward
    const baseTop = cue.style?.marginV || 5
    style.top = stackOffset > 0 ? `calc(${baseTop}% + ${stackOffset}px)` : `${baseTop}%`
  } else if (alignment >= 4) {
    style.top = '50%'
    style.transform = stackOffset > 0
      ? `translateY(calc(-50% + ${stackOffset}px))`
      : 'translateY(-50%)'
  } else {
    // Bottom-aligned: stack upward
    const baseBottom = cue.style?.marginV || 8
    style.bottom = stackOffset > 0 ? `calc(${baseBottom}% + ${stackOffset}px)` : `${baseBottom}%`
  }

  // Horizontal position
  const hPos = alignment % 3 // 0=right, 1=left, 2=center
  if (hPos === 1) {
    style.left = '5%'
    style.textAlign = 'left'
  } else if (hPos === 0) {
    style.right = '5%'
    style.textAlign = 'right'
  } else {
    style.left = '50%'
    if (!style.transform) {
      style.transform = 'translateX(-50%)'
    } else {
      style.transform += ' translateX(-50%)'
    }
    style.textAlign = 'center'
  }

  style.maxWidth = '90%'
  return style
}

// Text style from ASS cue styles + user appearance prefs (size / background).
function cueTextStyle(cue: SubtitleCue): Record<string, string> {
  // Size: scale the ASS-specified size, or the auto base, by the user's percent.
  const px = (cue.style?.fontSize ?? baseFontSize.value) * (props.sizeScale / 100)
  const style: Record<string, string> = {
    // Shared mapping — kept in lockstep with the appearance-panel preview.
    backgroundColor: subtitleBgColor(props.bgOpacity),
    color: '#ffffff',
    fontSize: `${px}px`,
    lineHeight: '1.5',
    textShadow: '1px 1px 2px var(--black-a80)',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
  }

  if (cue.style?.color) style.color = cue.style.color
  if (cue.style?.bold) style.fontWeight = 'bold'
  if (cue.style?.italic) style.fontStyle = 'italic'

  return style
}

// Time sync loop. The video clock is read every frame, but the reactive write
// (which drives the activeCues filter+sort over the whole cue list) is coarsened
// to ~8 Hz and change-gated: subtitle timing only needs ~⅛-second precision, so
// recomputing 60×/sec was ~7× wasted work on the main thread during playback.
// While paused the value never changes → no write → no recompute.
const TIME_SYNC_HZ = 8
function startTimeSync() {
  stopTimeSync()
  function tick() {
    if (props.videoElement) {
      const snapped = Math.round(props.videoElement.currentTime * TIME_SYNC_HZ) / TIME_SYNC_HZ
      if (snapped !== currentTime.value) currentTime.value = snapped
    }
    animFrameId = requestAnimationFrame(tick)
  }
  animFrameId = requestAnimationFrame(tick)
}

function stopTimeSync() {
  if (animFrameId !== null) {
    cancelAnimationFrame(animFrameId)
    animFrameId = null
  }
}

// Fetch and parse subtitles when URL changes
let subtitleAbortController: AbortController | null = null

async function loadSubtitles(url: string, format: string) {
  subtitleAbortController?.abort()
  subtitleAbortController = new AbortController()
  emit('loading', true)
  cues.value = []
  try {
    cues.value = await fetchAndParseCues(url, format, subtitleAbortController.signal)
  } catch (err: unknown) {
    if (err instanceof DOMException && err.name === 'AbortError') return
    const e = err as { message?: string }
    emit('error', e.message || 'Failed to load subtitles')
  } finally {
    emit('loading', false)
  }
}

watch(() => props.subtitleUrl, (url) => {
  if (url) {
    loadSubtitles(url, props.format || 'auto')
  } else {
    cues.value = []
  }
})

watch(() => props.videoElement, (el) => {
  if (el) {
    startTimeSync()
    updateBaseFontSize()
  } else {
    stopTimeSync()
  }
}, { immediate: true })

watch(() => props.visible, (v) => {
  if (v && props.videoElement) startTimeSync()
  else if (!v) stopTimeSync()
})

onMounted(() => {
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('webkitfullscreenchange', onFullscreenChange)
  window.addEventListener('resize', updateBaseFontSize)
  updateBaseFontSize()
  if (props.videoElement && props.visible) startTimeSync()
  if (props.subtitleUrl) {
    loadSubtitles(props.subtitleUrl, props.format || 'auto')
  }
})

onUnmounted(() => {
  subtitleAbortController?.abort()
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('webkitfullscreenchange', onFullscreenChange)
  window.removeEventListener('resize', updateBaseFontSize)
  stopTimeSync()
})
</script>

<style scoped>
.subtitle-text ::v-deep(ruby) {
  ruby-align: center;
}
.subtitle-text ::v-deep(rt) {
  font-size: 0.6em;
  color: #ffcccc;
}
.subtitle-text ::v-deep(b) {
  font-weight: bold;
}
.subtitle-text ::v-deep(i) {
  font-style: italic;
}
.subtitle-text ::v-deep(br) {
  line-height: 2;
}
</style>
