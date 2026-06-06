<template>
  <div
    data-test="track"
    class="pl-track"
    @click="onTrackClick"
    @mousemove="onMouseMove"
    @mouseleave="onMouseLeave"
  >
    <!-- Rail background (via ::before in scoped CSS) -->

    <!-- Buffered range -->
    <div
      data-test="buffered"
      class="pl-buffered"
      :style="{ width: buffered + '%' }"
    />

    <!-- Playback fill -->
    <div
      data-test="fill"
      class="pl-fill"
      :style="{ width: progress + '%' }"
    >
      <!-- Draggable thumb -->
      <span class="pl-thumb" aria-hidden="true" />
    </div>

    <!-- Chapter markers (intro / outro) -->
    <span
      v-for="(c, i) in chapters"
      :key="i"
      data-test="chapter"
      class="pl-chapter"
      :data-kind="c.kind"
      :style="{ left: c.startPct + '%', width: c.widthPct + '%' }"
      aria-hidden="true"
    />

    <!-- Hover preview -->
    <div
      v-if="hoverVisible"
      class="pl-preview"
      :style="{ left: hoverPct + '%' }"
      aria-hidden="true"
    >
      <div
        v-if="stillUrl"
        class="pl-preview-thumb"
        :style="{ backgroundImage: `url(${stillUrl})` }"
      />
      <span class="pl-preview-time">{{ hoverTime }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

interface Chapter {
  kind: 'intro' | 'outro'
  startPct: number
  widthPct: number
}

const props = defineProps<{
  progress: number
  buffered: number
  durationSec: number
  chapters: Chapter[]
  stillUrl?: string
}>()

const emit = defineEmits<{
  (e: 'seek', pct: number): void
  (e: 'hover', pct: number | null): void
}>()

const hoverPct = ref(0)
const hoverVisible = ref(false)

const hoverTime = computed(() => fmt((hoverPct.value / 100) * props.durationSec))

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v))
}

function fmt(sec: number): string {
  const s = Math.floor(sec)
  const m = Math.floor(s / 60)
  const ss = s % 60
  return `${m}:${ss.toString().padStart(2, '0')}`
}

function getPct(event: MouseEvent): number {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  if (!rect.width) return 0
  return clamp(((event.clientX - rect.left) / rect.width) * 100, 0, 100)
}

function onTrackClick(event: MouseEvent) {
  const pct = getPct(event)
  emit('seek', pct)
}

function onMouseMove(event: MouseEvent) {
  const pct = getPct(event)
  hoverPct.value = pct
  hoverVisible.value = true
  emit('hover', pct)
}

function onMouseLeave() {
  hoverVisible.value = false
  emit('hover', null)
}
</script>

<style scoped>
.pl-track {
  position: relative;
  flex: 1;
  height: 16px;
  display: flex;
  align-items: center;
  cursor: pointer;
}

.pl-track::before {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  height: 4px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.25);
  transition: height 0.1s;
}

.pl-track:hover::before {
  height: 6px;
}

.pl-buffered {
  position: absolute;
  height: 4px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.4);
  transition: height 0.1s;
}

.pl-track:hover .pl-buffered {
  height: 6px;
}

.pl-fill {
  position: absolute;
  height: 4px;
  border-radius: 999px;
  background: var(--brand-cyan);
  box-shadow: 0 0 8px var(--brand-cyan);
  transition: height 0.1s;
}

.pl-track:hover .pl-fill {
  height: 6px;
}

.pl-thumb {
  position: absolute;
  right: -6px;
  top: 50%;
  transform: translateY(-50%);
  width: 13px;
  height: 13px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 0 0 4px rgba(0, 212, 255, 0.3);
  opacity: 0;
  transition: opacity 0.15s;
}

.pl-track:hover .pl-thumb {
  opacity: 1;
}

.pl-chapter {
  position: absolute;
  height: 4px;
  background: var(--warning);
  opacity: 0.65;
  border-radius: 999px;
  pointer-events: none;
}

.pl-preview {
  position: absolute;
  bottom: 22px;
  transform: translateX(-50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  pointer-events: none;
  z-index: 10;
}

.pl-preview-thumb {
  width: 120px;
  height: 68px;
  border-radius: 8px;
  border: 2px solid rgba(255, 255, 255, 0.6);
  background-size: cover;
  background-position: center;
}

.pl-preview-time {
  font-size: 12px;
  color: #fff;
  background: rgba(0, 0, 0, 0.7);
  padding: 1px 6px;
  border-radius: 4px;
  font-weight: 500;
}
</style>
