<template>
  <div class="pl-hud" :class="{ 'pl-hud--fading': fading }" data-test="debug-hud">
    <div class="pl-hud-row pl-hud-head">
      <span>{{ provider }} · {{ streamType }}<template v-if="levelLabel"> · {{ levelLabel }}</template></span>
      <span class="pl-hud-actions">
        <button
          class="pl-hud-btn"
          :class="{ 'is-on': pinned }"
          role="checkbox"
          :aria-checked="pinned"
          aria-label="Pin HUD during playback"
          title="Pin"
          data-test="hud-pin-toggle"
          @click="emit('update:pinned', !pinned)"
        >
          <Pin :size="11" aria-hidden="true" />
        </button>
        <button
          class="pl-hud-btn"
          :class="{ 'is-on': helpOpen }"
          aria-label="Seek pipeline reference"
          data-test="hud-help-toggle"
          @click="helpOpen = !helpOpen"
        >?</button>
      </span>
    </div>

    <!-- Selected combo + WHY it was chosen -->
    <template v-if="decision">
      <div class="pl-hud-row pl-hud-head" data-test="hud-decision-head">SELECTED COMBO</div>
      <div class="pl-hud-row" data-test="hud-decision-combo">
        {{ decision.provider || '—' }} · {{ decision.audio }} · {{ decision.lang
        }}<template v-if="decision.team"> · {{ decision.team }}</template>
      </div>
      <div class="pl-hud-row pl-hud-dim" data-test="hud-decision-why">why: {{ decision.reason }}</div>
    </template>

    <!-- Metadata — always shown in full -->
    <div class="pl-hud-row">BW   {{ bw }}</div>
    <div class="pl-hud-row">
      BUF  +{{ stats.bufferAheadSec.toFixed(1) }}s / −{{ stats.bufferBehindSec.toFixed(1) }}s · rs={{ stats.readyState }}
    </div>
    <div class="pl-hud-row">
      RES  {{ stats.resolution || '—' }} · drop {{ stats.droppedFrames }}/{{ stats.totalFrames }}
    </div>
    <div v-if="compat" class="pl-hud-row" data-test="hud-compat">WASM {{ compat }}</div>

    <!-- Seek pipeline trace — only the latest 3 steps -->
    <template v-if="seek">
      <div class="pl-hud-row pl-hud-head" data-test="hud-seek-head">
        SEEK →{{ fmtTime(seek.target) }} · buffer {{ seek.bufferHit ? 'HIT' : 'MISS' }}
      </div>
      <div
        v-for="(st, i) in visibleSteps"
        :key="seek.t0 + ':' + st.text"
        class="pl-hud-row pl-hud-step"
        data-test="hud-seek-step"
      >
        <Spinner v-if="!st.done && i === firstPendingIdx" size="xs" tone="mono" class="pl-hud-spin" label="working" />
        <span v-else class="pl-hud-mark">{{ st.done ? '✓' : '·' }}</span>
        <span>{{ st.text }}</span>
      </div>
    </template>

    <template v-if="frags.length">
      <div v-for="(f, i) in lastFrags" :key="i" class="pl-hud-row">
        FRAG {{ fmtSize(f.size) }} · {{ Math.round(f.loadMs) }}ms · {{ f.duration.toFixed(1) }}s @{{ Math.round(f.start) }}s
      </div>
    </template>
    <div v-else class="pl-hud-row pl-hud-dim">no fragments (mp4 / not loaded)</div>

    <!-- Scrub-preview thumbnail engine — frontend pump health vs provider cost -->
    <template v-if="scrub.engine !== 'idle'">
      <div class="pl-hud-row pl-hud-head" data-test="hud-preview-head">
        PREVIEW {{ scrub.engine }} · cache {{ scrub.cacheSize }} · queue {{ scrub.queueLen }}
      </div>
      <div class="pl-hud-row" data-test="hud-preview-stats">
        PRV  seek {{ scrub.seeks }} → cap {{ scrub.captures }} · wd {{ scrub.watchdogs }} · hover {{ scrub.hoverHits }}✓/{{ scrub.hoverMisses }}✗
      </div>
      <div class="pl-hud-row">
        PRV  cap {{ scrub.lastCaptureMs ?? '—' }}ms (avg {{ scrub.avgCaptureMs || '—' }}) · frag {{ scrub.lastFragKb ?? '—' }}KB/{{ scrub.lastFragMs ?? '—' }}ms
      </div>
      <div v-if="scrub.lastError" class="pl-hud-row" data-test="hud-preview-error">
        PRV  ERR×{{ scrub.errors }} {{ scrub.lastError }}
      </div>
      <div
        v-for="(ev, i) in lastScrubEvents"
        :key="scrub.seeks + ':' + i"
        class="pl-hud-row pl-hud-dim"
        data-test="hud-preview-event"
      >{{ ev }}</div>
    </template>

    <!-- Source auto-fallback ledger — what the resolver switched to (or, in
         hacker mode, what it WOULD switch to without acting). -->
    <template v-if="intents && intents.length">
      <div class="pl-hud-row pl-hud-head" data-test="hud-fallback-head">SOURCE FALLBACK</div>
      <div
        v-for="(it, i) in lastIntents"
        :key="it.at + ':' + i"
        class="pl-hud-row"
        :class="{ 'pl-hud-dim': it.acted }"
        data-test="hud-fallback-intent"
      >{{ it.acted ? '→ switched' : '✗ intent' }} {{ it.from }} → {{ it.to ?? '—' }} · {{ it.reason }}</div>
    </template>

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
import { Pin } from 'lucide-vue-next'
import Spinner from '@/components/ui/Spinner.vue'
import type { PlaybackStats } from '@/composables/aePlayer/usePlaybackStats'
import type { FragStat } from '@/composables/aePlayer/useVideoEngine'
import { scrubDebug as scrub } from '@/composables/aePlayer/scrubPreviewDebug'
import type { FallbackIntent } from '@/composables/aePlayer/sourceFallbackDebug'

/** The active source combo + WHY the player landed on it. */
export interface SourceDecision {
  provider: string
  audio: string
  lang: string
  team: string | null
  reason: string
}

// SeekTrace moved to @/types/aePlayer so .ts composables can import it
// (named type imports from .vue files throw TS2614 in a clean build).
import type { SeekTrace } from '@/types/aePlayer'
export type { SeekTrace }

const props = defineProps<{
  stats: PlaybackStats
  frags: FragStat[]
  /** hls.js bandwidthEstimate, bits/s; 0 = unknown */
  bandwidth: number
  provider: string
  streamType: string
  levelLabel: string
  seek?: SeekTrace | null
  /** the active source combo + the reason it was chosen (smart default,
   *  deep-link, failover, manual, room) */
  decision?: SourceDecision | null
  /** source auto-fallback ledger (newest last) */
  intents?: FallbackIntent[]
  /** keep the HUD on screen during playback */
  pinned?: boolean
  /** linger fade-out in progress */
  fading?: boolean
  /** Hi10P wasm compat engine live stats — null when the native pipeline
   *  is playing (metrics + the decision, per the hacker-mode contract) */
  compat?: string | null
}>()

const emit = defineEmits<{
  (e: 'update:pinned', value: boolean): void
}>()

const helpOpen = ref(false)

const lastFrags = computed(() => props.frags.slice(-5).reverse())

/** Newest 4 source-fallback decisions, newest first. */
const lastIntents = computed(() => (props.intents ?? []).slice(-4).reverse())

/** Newest 4 thumbnail-engine events, newest first. */
const lastScrubEvents = computed(() => scrub.events.slice(-4).reverse())

const bw = computed(() =>
  props.bandwidth > 0 ? `${(props.bandwidth / 1_000_000).toFixed(1)} Mbit/s` : '—',
)

interface SeekStep {
  done: boolean
  text: string
}

const allSteps = computed<SeekStep[]>(() => {
  const s = props.seek
  if (!s) return []
  const steps: SeekStep[] = []
  if (s.bufferHit) {
    steps.push({ done: true, text: 'check  in buffer — no network' })
  } else {
    steps.push({ done: true, text: 'check  not buffered' })
    steps.push({ done: true, text: 'flush  abort loads · reset decoder' })
    steps.push({ done: s.fetchMs !== null || s.done, text: `fetch  ${fetchLabel.value}` })
  }
  steps.push({
    done: s.seekedMs !== null,
    text: `decode keyframe→target${s.seekedMs !== null ? ` ${s.seekedMs}ms` : ''}`,
  })
  steps.push({
    done: s.resumeMs !== null,
    text: `resume rs≥3${s.resumeMs !== null ? ` ${s.resumeMs}ms` : ' — buffering'}`,
  })
  return steps
})

/** Only the latest 3 pipeline steps — metadata above stays complete. */
const visibleSteps = computed(() => allSteps.value.slice(-3))

const firstPendingIdx = computed(() => visibleSteps.value.findIndex((s) => !s.done))

const fetchLabel = computed(() => {
  const s = props.seek
  if (!s) return ''
  // The fetch step is usually the long one — show what it actually did:
  // mp4 = moov-index lookup + HTTP range request; hls = segment downloads.
  const ms = s.fetchMs !== null ? ` · ${s.fetchMs}ms` : ''
  const range = s.fetchedRange
    ? ` → ${Math.round(s.fetchedRange[0])}–${Math.round(s.fetchedRange[1])}s buffered`
    : ''
  if (props.streamType === 'mp4') {
    return s.fetchMs !== null
      ? `range req (moov→bytes)${ms}${range}`
      : 'range req — moov index → byte offset…'
  }
  if (s.frags > 0) return `${s.frags} frags · ${fmtSize(s.bytes)}${ms}`
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
  /* ABOVE the top bar (z-6): its transparent padding area extends past 76px
     and was swallowing clicks on the "?" button. */
  z-index: 8;
  padding: 10px 12px;
  border-radius: var(--r-md, 8px);
  background: var(--black-a80);
  border: 1px solid var(--border);
  font-family: var(--font-mono, monospace);
  font-size: 11px;
  line-height: 1.7;
  color: var(--success);
  pointer-events: none;
  white-space: pre;
  max-width: min(420px, calc(100% - 28px));
  overflow: hidden;
  opacity: 1;
  transition: opacity 0.4s ease;
}

/* Linger: playback resumed — hold for ~1s (handled by the parent timer),
   then this class fades the panel out before unmount. */
.pl-hud--fading {
  opacity: 0;
}

.pl-hud-head {
  color: var(--brand-cyan);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.pl-hud-actions {
  display: inline-flex;
  gap: 4px;
  flex-shrink: 0;
}

.pl-hud-dim {
  opacity: 0.6;
}

.pl-hud-step {
  display: flex;
  align-items: center;
  gap: 6px;
}

.pl-hud-mark {
  width: 14px;
  text-align: center;
  flex-shrink: 0;
}

.pl-hud-spin {
  flex-shrink: 0;
  margin: 0 1px;
}

/* The pin / "?" buttons are the only interactive elements */
.pl-hud-btn {
  pointer-events: auto;
  width: 18px;
  height: 18px;
  flex-shrink: 0;
  display: inline-grid;
  place-items: center;
  border: 1px solid var(--border);
  border-radius: 4px;
  background: var(--line);
  color: var(--brand-cyan);
  font-family: inherit;
  font-size: 11px;
  line-height: 1;
  cursor: pointer;
}

.pl-hud-btn:hover,
.pl-hud-btn.is-on {
  background: var(--cyan-a20);
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
