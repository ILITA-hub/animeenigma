<template>
  <div
    data-test="track"
    class="pl-track"
    role="slider"
    tabindex="0"
    aria-label="Seek"
    :aria-valuemin="0"
    :aria-valuemax="100"
    :aria-valuenow="Math.round(progress)"
    :aria-valuetext="fmt((progress / 100) * durationSec)"
    @click="onTrackClick"
    @mousemove="onMouseMove"
    @mouseleave="onMouseLeave"
    @keydown.left.prevent.stop="onKeySeek(-1)"
    @keydown.right.prevent.stop="onKeySeek(1)"
    @keydown.down.prevent.stop="onKeySeek(-1)"
    @keydown.up.prevent.stop="onKeySeek(1)"
    @keydown.home.prevent.stop="onKeySeekTo(0)"
    @keydown.end.prevent.stop="onKeySeekTo(100)"
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

    <!-- Hacker-mode fragment heatmap (sits over the fill so it reads on top) -->
    <span
      v-for="(f, i) in fragments ?? []"
      :key="'frag' + i"
      data-test="frag"
      class="pl-frag"
      :data-tone="f.tone"
      :style="{ left: f.startPct + '%', width: f.widthPct + '%' }"
      :title="f.label"
    />

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

    <!-- Hover preview — v-show (not v-if) so the ScrubPreview shadow engine
         survives between hovers instead of re-initializing every time -->
    <div
      v-show="hoverVisible"
      class="pl-preview"
      :style="{ left: hoverPct + '%' }"
      aria-hidden="true"
    >
      <div v-if="previewUrl || stillUrl" class="pl-preview-thumb">
        <ScrubPreview
          :time-sec="hoverTimeSec"
          :visible="hoverVisible"
          :stream-url="previewUrl ?? null"
          :stream-type="previewType ?? null"
          :still-url="stillUrl"
        />
      </div>
      <span class="pl-preview-time">{{ hoverTime }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import ScrubPreview from './ScrubPreview.vue'

interface Chapter {
  kind: 'intro' | 'outro'
  startPct: number
  widthPct: number
}

export interface FragSegment {
  startPct: number
  widthPct: number
  tone: 'ok' | 'warn' | 'bad'
  label: string
}

const props = defineProps<{
  progress: number
  buffered: number
  durationSec: number
  chapters: Chapter[]
  stillUrl?: string
  /** hacker-mode per-fragment heatmap segments (empty/omitted = off) */
  fragments?: FragSegment[]
  /** current stream URL for real hover frame previews (null = still only) */
  previewUrl?: string | null
  previewType?: 'hls' | 'mp4' | null
}>()

const emit = defineEmits<{
  (e: 'seek', pct: number): void
  (e: 'hover', pct: number | null): void
}>()

const hoverPct = ref(0)
const hoverVisible = ref(false)

const hoverTimeSec = computed(() => (hoverPct.value / 100) * props.durationSec)
const hoverTime = computed(() => fmt(hoverTimeSec.value))

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

// Keyboard slider: ←/→ and ↓/↑ = ±5 s expressed in track percent. Up/Down are
// bound with .stop so they act on the focused slider instead of falling through
// to the global volume hotkey (WAI-ARIA slider pattern).
function onKeySeek(dir: 1 | -1) {
  if (!props.durationSec) return
  const stepPct = (5 / props.durationSec) * 100
  emit('seek', clamp(props.progress + dir * stepPct, 0, 100))
}

// Home/End jump to the start/end of the timeline (WAI-ARIA slider pattern).
function onKeySeekTo(pct: number) {
  emit('seek', clamp(pct, 0, 100))
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
  background: var(--white-a20);
  transition: height 0.1s;
}

.pl-track:hover::before {
  height: 6px;
}

.pl-buffered {
  position: absolute;
  height: 4px;
  border-radius: 999px;
  background: var(--ink-4);
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
  box-shadow: 0 0 0 4px var(--accent-line);
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

/* Hacker-mode fragment heatmap — size-tinted segments. Clicks still seek:
   the track's click handler reads currentTarget, so child spans don't break
   getPct; title attr gives the native size/time tooltip on hover. */
.pl-frag {
  position: absolute;
  height: 4px;
  opacity: 0.85;
  border-right: 1px solid var(--black-a60);
}

.pl-track:hover .pl-frag {
  height: 6px;
}

.pl-frag[data-tone='ok'] {
  background: var(--success);
}

.pl-frag[data-tone='warn'] {
  background: var(--warning);
}

.pl-frag[data-tone='bad'] {
  background: var(--destructive);
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
  border: 2px solid var(--muted-foreground);
  overflow: hidden;
}

.pl-preview-time {
  font-size: 12px;
  color: #fff;
  background: var(--black-a80);
  padding: 1px 6px;
  border-radius: 4px;
  font-weight: 500;
}

/* Touch: taller grab zone + always-visible thumb (there is no hover). */
@media (pointer: coarse) {
  .pl-track {
    height: 28px;
  }

  .pl-thumb {
    opacity: 1;
    width: 15px;
    height: 15px;
  }
}
</style>
