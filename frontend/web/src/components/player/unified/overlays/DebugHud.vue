<template>
  <div class="pl-hud" data-test="debug-hud">
    <div class="pl-hud-row pl-hud-head">
      <span>{{ provider }} · {{ streamType }}<template v-if="levelLabel"> · {{ levelLabel }}</template></span>
      <button
        class="pl-hud-help"
        :class="{ 'is-open': helpOpen }"
        aria-label="Seek pipeline reference"
        data-test="hud-help-toggle"
        @click="helpOpen = !helpOpen"
      >?</button>
    </div>
    <div class="pl-hud-row">BW   {{ bw }}</div>
    <div class="pl-hud-row">
      BUF  +{{ stats.bufferAheadSec.toFixed(1) }}s / −{{ stats.bufferBehindSec.toFixed(1) }}s · rs={{ stats.readyState }}
    </div>
    <div class="pl-hud-row">
      RES  {{ stats.resolution || '—' }} · drop {{ stats.droppedFrames }}/{{ stats.totalFrames }}
    </div>

    <!-- Seek pipeline trace: what the spinner covers after a scrub/arrow seek -->
    <template v-if="seek">
      <div class="pl-hud-row pl-hud-head" data-test="hud-seek-head">
        SEEK →{{ fmtTime(seek.target) }} · buffer {{ seek.bufferHit ? 'HIT' : 'MISS' }}
      </div>
      <template v-if="seek.bufferHit">
        <div class="pl-hud-row" data-test="hud-seek-step">
          ✓ check  in buffer — no network
        </div>
      </template>
      <template v-else>
        <div class="pl-hud-row" data-test="hud-seek-step">✓ check  not buffered</div>
        <div class="pl-hud-row" data-test="hud-seek-step">✓ flush  abort loads · reset decoder</div>
        <div class="pl-hud-row" data-test="hud-seek-step">
          {{ seek.frags > 0 ? '✓' : seek.done ? '✓' : '…' }} fetch  {{ fetchLabel }}
        </div>
      </template>
      <div class="pl-hud-row" data-test="hud-seek-step">
        {{ seek.seekedMs !== null ? '✓' : '…' }} decode keyframe→target{{ seek.seekedMs !== null ? ` ${seek.seekedMs}ms` : ' …' }}
      </div>
      <div class="pl-hud-row" data-test="hud-seek-step">
        {{ seek.resumeMs !== null ? '✓' : '…' }} resume rs≥3{{ seek.resumeMs !== null ? ` ${seek.resumeMs}ms` : ' — buffering…' }}
      </div>
    </template>

    <template v-if="frags.length">
      <div v-for="(f, i) in lastFrags" :key="i" class="pl-hud-row">
        FRAG {{ fmtSize(f.size) }} · {{ Math.round(f.loadMs) }}ms · {{ f.duration.toFixed(1) }}s @{{ Math.round(f.start) }}s
      </div>
    </template>
    <div v-else class="pl-hud-row pl-hud-dim">no fragments (mp4 / not loaded)</div>

    <!-- "?" — condensed tech reference of the seek pipeline -->
    <div v-if="helpOpen" class="pl-hud-ref" data-test="hud-reference">
      <div class="pl-hud-ref-title">WHAT A SEEK ACTUALLY DOES</div>
      <p><b>1 check</b> — is the target inside the buffered ranges (the lighter bar)? If yes, playback resumes instantly with zero network — that's why ±5s is instant but a far jump isn't.</p>
      <p><b>2 flush</b> — abort in-flight segment downloads, drop queued frames from the decoder, reset the A/V sync clock.</p>
      <p><b>3 locate</b> — HLS: the manifest maps time→segment URL. MP4: the moov index maps time→byte offset (then an HTTP range request).</p>
      <p><b>4 fetch</b> — segments come from the CDN edge; a cache hit is tens of ms, a miss goes to origin. ABR players often grab the first post-seek segment at low quality (brief blur).</p>
      <p><b>5 decode</b> — compressed video only has full pictures at keyframes (every 2–10s); P/B-frames are diffs. The decoder rewinds to the keyframe before your target and decodes forward, discarding frames until it reaches it.</p>
      <p><b>6 resume</b> — re-align audio+video clocks, refill a safety buffer (rs≥3), hide the spinner, keep downloading ahead.</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { PlaybackStats } from '@/composables/unifiedPlayer/usePlaybackStats'
import type { FragStat } from '@/composables/unifiedPlayer/useVideoEngine'

/** One user seek, traced through the pipeline. Mutated in place as events land. */
export interface SeekTrace {
  /** target position, seconds */
  target: number
  /** target was inside a buffered range at seek time (instant, no network) */
  bufferHit: boolean
  /** performance.now() at seek start */
  t0: number
  /** ms until the `seeked` event (decoder positioned on the target frame) */
  seekedMs: number | null
  /** ms until readyState ≥ 3 (safety buffer refilled, playback resumes) */
  resumeMs: number | null
  /** fragments fetched while the seek was in flight (hls only) */
  frags: number
  /** bytes fetched while the seek was in flight (hls only) */
  bytes: number
  done: boolean
}

const props = defineProps<{
  stats: PlaybackStats
  frags: FragStat[]
  /** hls.js bandwidthEstimate, bits/s; 0 = unknown */
  bandwidth: number
  provider: string
  streamType: string
  levelLabel: string
  seek?: SeekTrace | null
}>()

const helpOpen = ref(false)

const lastFrags = computed(() => props.frags.slice(-5).reverse())

const bw = computed(() =>
  props.bandwidth > 0 ? `${(props.bandwidth / 1_000_000).toFixed(1)} Mbit/s` : '—',
)

const fetchLabel = computed(() => {
  const s = props.seek
  if (!s) return ''
  if (props.streamType === 'mp4') return 'range request (moov index)'
  if (s.frags > 0) return `${s.frags} frags · ${fmtSize(s.bytes)}`
  return s.done ? 'served from hls buffer' : 'requesting segments…'
})

function fmtSize(bytes: number): string {
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)}MB`
  return `${Math.round(bytes / 1024)}KB`
}

function fmtTime(sec: number): string {
  const s = Math.floor(Math.max(0, sec))
  const m = Math.floor(s / 60)
  return `${m}:${(s % 60).toString().padStart(2, '0')}`
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.pl-hud-dim {
  opacity: 0.6;
}

/* "?" is the only interactive element — re-enable pointer events on it */
.pl-hud-help {
  pointer-events: auto;
  width: 18px;
  height: 18px;
  flex-shrink: 0;
  border: 1px solid var(--border);
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.06);
  color: var(--brand-cyan);
  font-family: inherit;
  font-size: 11px;
  line-height: 1;
  cursor: pointer;
}

.pl-hud-help:hover,
.pl-hud-help.is-open {
  background: rgba(0, 212, 255, 0.18);
}

.pl-hud-ref {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--border);
  white-space: normal;
  max-width: 380px;
  max-height: 300px;
  overflow-y: auto;
  pointer-events: auto;
}

.pl-hud-ref-title {
  color: var(--brand-cyan);
  margin-bottom: 4px;
}

.pl-hud-ref p {
  margin: 0 0 6px;
  line-height: 1.5;
  opacity: 0.9;
}

.pl-hud-ref b {
  color: var(--warning);
  font-weight: 600;
}
</style>
