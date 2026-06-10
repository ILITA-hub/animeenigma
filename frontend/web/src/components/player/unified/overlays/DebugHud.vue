<template>
  <div class="pl-hud" data-test="debug-hud" aria-hidden="true">
    <div class="pl-hud-row pl-hud-head">
      {{ provider }} · {{ streamType }}<template v-if="levelLabel"> · {{ levelLabel }}</template>
    </div>
    <div class="pl-hud-row">BW   {{ bw }}</div>
    <div class="pl-hud-row">
      BUF  +{{ stats.bufferAheadSec.toFixed(1) }}s / −{{ stats.bufferBehindSec.toFixed(1) }}s · rs={{ stats.readyState }}
    </div>
    <div class="pl-hud-row">
      RES  {{ stats.resolution || '—' }} · drop {{ stats.droppedFrames }}/{{ stats.totalFrames }}
    </div>
    <template v-if="frags.length">
      <div v-for="(f, i) in lastFrags" :key="i" class="pl-hud-row">
        FRAG {{ fmtSize(f.size) }} · {{ Math.round(f.loadMs) }}ms · {{ f.duration.toFixed(1) }}s @{{ Math.round(f.start) }}s
      </div>
    </template>
    <div v-else class="pl-hud-row pl-hud-dim">no fragments (mp4 / not loaded)</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { PlaybackStats } from '@/composables/unifiedPlayer/usePlaybackStats'
import type { FragStat } from '@/composables/unifiedPlayer/useVideoEngine'

const props = defineProps<{
  stats: PlaybackStats
  frags: FragStat[]
  /** hls.js bandwidthEstimate, bits/s; 0 = unknown */
  bandwidth: number
  provider: string
  streamType: string
  levelLabel: string
}>()

const lastFrags = computed(() => props.frags.slice(-5).reverse())

const bw = computed(() =>
  props.bandwidth > 0 ? `${(props.bandwidth / 1_000_000).toFixed(1)} Mbit/s` : '—',
)

function fmtSize(bytes: number): string {
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)}MB`
  return `${Math.round(bytes / 1024)}KB`
}
</script>

<style scoped>
.pl-hud {
  position: absolute;
  top: 76px;
  left: 14px;
  z-index: 5;
  padding: 10px 12px;
  border-radius: var(--r-md, 8px);
  background: rgba(0, 0, 0, 0.78);
  border: 1px solid var(--border);
  font-family: var(--font-mono, monospace);
  font-size: 11px;
  line-height: 1.7;
  color: var(--success);
  pointer-events: none;
  white-space: pre;
  max-width: min(420px, calc(100% - 28px));
  overflow: hidden;
}

.pl-hud-head {
  color: var(--brand-cyan);
}

.pl-hud-dim {
  opacity: 0.6;
}
</style>
