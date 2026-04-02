<template>
  <Teleport :to="fullscreenEl || 'body'" :disabled="!fullscreenEl">
    <div
      v-if="visible && activeCues.length > 0"
      class="absolute inset-0 pointer-events-none overflow-hidden"
      :class="fullscreenEl ? 'z-[2147483647]' : 'z-20'"
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
import { ref, watch, onMounted, onUnmounted, computed } from 'vue'
import type { SubtitleCue } from '@/utils/subtitle-parser'
import { parseASS, parseSRT, parseVTT } from '@/utils/subtitle-parser'

const props = defineProps<{
  videoElement: HTMLVideoElement | null
  subtitleUrl: string | null
  format: 'ass' | 'srt' | 'vtt' | null
  visible: boolean
}>()

const emit = defineEmits<{
  (e: 'loading', loading: boolean): void
  (e: 'error', msg: string): void
}>()

const cues = ref<SubtitleCue[]>([])
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

function onFullscreenChange() {
  const el = document.fullscreenElement || (document as Document & { webkitFullscreenElement?: Element }).webkitFullscreenElement || null
  if (el && el.tagName === 'VIDEO') {
    // Native <video> went fullscreen — subtitles can't render inside it.
    // Redirect fullscreen to the parent container so the overlay stays visible.
    // Call requestFullscreen on parent directly (replaces current fullscreen element).
    const container = el.parentElement
    if (container) {
      container.requestFullscreen().catch(() => {
        // If redirect fails (no user activation), keep video fullscreen
        // but subtitles won't be visible in native-player fullscreen mode.
      })
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

// Find active cues for current time, sorted by layer
const activeCues = computed(() => {
  if (!cues.value.length) return []
  const t = currentTime.value
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

// Text style from ASS cue styles
function cueTextStyle(cue: SubtitleCue): Record<string, string> {
  const style: Record<string, string> = {
    backgroundColor: 'rgba(0, 0, 0, 0.7)',
    color: '#ffffff',
    fontSize: `${baseFontSize.value}px`,
    lineHeight: '1.5',
    textShadow: '1px 1px 2px rgba(0,0,0,0.8)',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-word',
  }

  if (cue.style?.color) style.color = cue.style.color
  if (cue.style?.fontSize) style.fontSize = `${cue.style.fontSize}px`
  if (cue.style?.bold) style.fontWeight = 'bold'
  if (cue.style?.italic) style.fontStyle = 'italic'

  return style
}

// Time sync loop
function startTimeSync() {
  stopTimeSync()
  function tick() {
    if (props.videoElement) {
      currentTime.value = props.videoElement.currentTime
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
  // Cancel any in-flight subtitle fetch
  subtitleAbortController?.abort()
  subtitleAbortController = new AbortController()

  emit('loading', true)
  cues.value = []

  try {
    // Proxy through streaming service for CORS
    const proxyUrl = `/api/streaming/hls-proxy?url=${encodeURIComponent(url)}`
    const resp = await fetch(proxyUrl, { signal: subtitleAbortController.signal })
    if (!resp.ok) throw new Error(`Failed to fetch subtitle file: ${resp.status}`)

    const content = await resp.text()

    switch (format) {
      case 'ass':
        cues.value = await parseASS(content)
        break
      case 'srt':
        cues.value = parseSRT(content)
        break
      case 'vtt':
        cues.value = parseVTT(content)
        break
      default:
        // Try to detect from content
        if (content.includes('[Script Info]') || content.includes('[V4+ Styles]')) {
          cues.value = await parseASS(content)
        } else if (content.startsWith('WEBVTT')) {
          cues.value = parseVTT(content)
        } else {
          cues.value = parseSRT(content)
        }
    }
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
