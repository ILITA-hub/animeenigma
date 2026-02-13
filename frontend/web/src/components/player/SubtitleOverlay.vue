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
        :style="cuePositionStyle(cue)"
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

// Fullscreen detection for Teleport
const fullscreenEl = ref<Element | null>(null)

function onFullscreenChange() {
  const el = document.fullscreenElement || (document as any).webkitFullscreenElement || null
  // Only teleport to non-video elements (can't add children to <video>)
  if (el && el.tagName !== 'VIDEO') {
    fullscreenEl.value = el
  } else {
    fullscreenEl.value = null
  }
}

// Find active cues for current time
const activeCues = computed(() => {
  if (!cues.value.length) return []
  const t = currentTime.value
  return cues.value.filter(c => t >= c.start && t <= c.end)
})

// Position style based on ASS alignment
function cuePositionStyle(cue: SubtitleCue): Record<string, string> {
  const alignment = cue.style?.alignment || 2 // Default: bottom center

  // ASS numpad alignment:
  // 7=TL 8=TC 9=TR
  // 4=ML 5=MC 6=MR
  // 1=BL 2=BC 3=BR
  const style: Record<string, string> = {}

  // Vertical position
  if (alignment >= 7) {
    style.top = '5%'
  } else if (alignment >= 4) {
    style.top = '50%'
    style.transform = 'translateY(-50%)'
  } else {
    style.bottom = '8%'
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
    fontSize: '1.2em',
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
async function loadSubtitles(url: string, format: string) {
  emit('loading', true)
  cues.value = []

  try {
    // Proxy through streaming service for CORS
    const proxyUrl = `/api/streaming/hls-proxy?url=${encodeURIComponent(url)}`
    const resp = await fetch(proxyUrl)
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
  } catch (err: any) {
    emit('error', err.message || 'Failed to load subtitles')
  } finally {
    emit('loading', false)
  }
}

watch(() => props.subtitleUrl, (url) => {
  if (url && props.format) {
    loadSubtitles(url, props.format)
  } else {
    cues.value = []
  }
})

watch(() => props.videoElement, (el) => {
  if (el) startTimeSync()
  else stopTimeSync()
}, { immediate: true })

watch(() => props.visible, (v) => {
  if (v && props.videoElement) startTimeSync()
  else if (!v) stopTimeSync()
})

onMounted(() => {
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('webkitfullscreenchange', onFullscreenChange)
  if (props.videoElement && props.visible) startTimeSync()
  if (props.subtitleUrl && props.format) {
    loadSubtitles(props.subtitleUrl, props.format)
  }
})

onUnmounted(() => {
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('webkitfullscreenchange', onFullscreenChange)
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
