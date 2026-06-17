<template>
  <div class="pl-controls">
    <!-- Scrub row: full-width horizontal line (YouTube-style) -->
    <div class="pl-scrub-row">
      <PlayerScrubBar
        :progress="progress"
        :buffered="buffered"
        :duration-sec="duration"
        :chapters="chapters"
        :still-url="stillUrl"
        :fragments="fragments"
        :preview-url="previewUrl"
        :preview-type="previewType"
        @seek="emit('seek', $event)"
      />
    </div>

    <div class="pl-btns">

      <!-- Play / Pause -->
      <PlayerIconButton
        :aria-label="playing ? 'Pause' : 'Play'"
        data-test="play-pause"
        @click="emit('toggle-play')"
      >
        <Pause v-if="playing" class="size-5" aria-hidden="true" />
        <Play v-else class="size-5" aria-hidden="true" />
      </PlayerIconButton>

      <!-- −5s (hidden on mobile via CSS) — circular replay arrow -->
      <PlayerIconButton
        class="pl-skip-back"
        aria-label="Back 5 seconds"
        data-test="seek-back"
        @click="emit('seek-rel', -5)"
      >
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M4 4v6h6M4 10a8 8 0 11-1 4" />
        </svg>
      </PlayerIconButton>

      <!-- +5s (hidden on mobile via CSS) — circular forward arrow -->
      <PlayerIconButton
        class="pl-skip-fwd"
        aria-label="Forward 5 seconds"
        data-test="seek-fwd"
        @click="emit('seek-rel', 5)"
      >
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <g style="transform: scaleX(-1); transform-origin: center">
            <path d="M4 4v6h6M4 10a8 8 0 11-1 4" />
          </g>
        </svg>
      </PlayerIconButton>

      <!-- Volume cluster (hover to expand) -->
      <div class="pl-vol">
        <PlayerIconButton
          :aria-label="muted || volume === 0 ? 'Unmute' : 'Mute'"
          data-test="mute"
          @click="emit('toggle-mute')"
        >
          <!-- Muted -->
          <VolumeX v-if="muted || volume === 0" class="size-5" aria-hidden="true" />
          <!-- Volume medium -->
          <Volume1 v-else-if="volume < 0.5" class="size-5" aria-hidden="true" />
          <!-- Volume high -->
          <Volume2 v-else class="size-5" aria-hidden="true" />
        </PlayerIconButton>
        <input
          type="range"
          class="pl-vol-range"
          min="0"
          max="100"
          step="1"
          :value="muted ? 0 : volume"
          aria-label="Volume"
          data-test="volume-slider"
          @input="onVolumeInput"
        />
      </div>

      <!-- Time pill: current / total (YouTube-style, left of the spacer) -->
      <span class="pl-timepill" data-test="time-pill">
        <span data-test="time-current">{{ fmt(currentTime) }}</span>
        <span class="pl-timepill-sep" aria-hidden="true">/</span>
        <span data-test="time-duration">{{ fmt(duration) }}</span>
      </span>

      <!-- Episodes pill (left cluster; second access path — the top-left EP eyebrow is the other) -->
      <button
        class="pl-epbtn"
        :class="{ 'is-open': openMenu === 'episodes' }"
        data-test="episodes-pill"
        :aria-expanded="openMenu === 'episodes'"
        aria-label="Episodes"
        @click="emit('toggle-episodes')"
      >
        <ListVideo class="size-4" aria-hidden="true" />
        <span class="pl-epbtn-text">EP {{ episodeLabel }}</span>
        <ChevronDown class="size-3" aria-hidden="true" />
      </button>

      <!-- Spacer -->
      <span class="pl-spacer" aria-hidden="true" />

      <!-- Source pill -->
      <button
        class="pl-srcbtn"
        :class="{ 'is-open': openMenu === 'source' }"
        data-test="source-pill"
        aria-label="`Source: ${providerName} · ${audioLabel}`"
        @click="emit('toggle-source')"
      >
        <!-- Provider identity-hue dot -->
        <span
          class="pl-prov-dot"
          :style="{ background: providerHue, boxShadow: `0 0 8px ${providerHue}` }"
          aria-hidden="true"
        />
        <span class="pl-srcbtn-text">{{ providerName }} · {{ audioLabel }}</span>
        <!-- Chevron down -->
        <ChevronDown class="size-3" aria-hidden="true" />
      </button>

      <!-- Subtitles (CC) -->
      <PlayerIconButton
        :active="openMenu === 'subs'"
        aria-label="Subtitles"
        data-test="toggle-subs"
        @click="emit('toggle-subs')"
      >
        <Captions class="size-5" aria-hidden="true" />
      </PlayerIconButton>

      <!-- Settings gear -->
      <PlayerIconButton
        :active="openMenu === 'settings'"
        aria-label="Settings"
        data-test="toggle-settings"
        @click="emit('toggle-settings')"
      >
        <Settings class="size-5" aria-hidden="true" />
      </PlayerIconButton>

      <!-- PiP (hidden on mobile via CSS) -->
      <PlayerIconButton
        class="pl-pip-btn"
        aria-label="Picture in Picture"
        data-test="toggle-pip"
        @click="emit('toggle-pip')"
      >
        <PictureInPicture2 class="size-5" aria-hidden="true" />
      </PlayerIconButton>

      <!-- Fullscreen (hidden on mobile via CSS) -->
      <PlayerIconButton
        class="pl-fs-btn"
        aria-label="Fullscreen"
        data-test="toggle-fullscreen"
        @click="emit('toggle-fullscreen')"
      >
        <Maximize class="size-5" aria-hidden="true" />
      </PlayerIconButton>

    </div>
  </div>
</template>

<script setup lang="ts">
import PlayerScrubBar from './PlayerScrubBar.vue'
import PlayerIconButton from './PlayerIconButton.vue'
import { Play, Pause, Volume1, Volume2, VolumeX, ChevronDown, Captions, Settings, PictureInPicture2, Maximize, ListVideo } from 'lucide-vue-next'

interface Chapter {
  kind: 'intro' | 'outro'
  startPct: number
  widthPct: number
}

withDefaults(
  defineProps<{
    playing: boolean
    currentTime: number
    duration: number
    volume: number
    muted: boolean
    providerName: string
    providerHue: string
    audioLabel: string
    /** current episode number/label, shown on the bottom episodes pill */
    episodeLabel?: string | number
    /** 0..100 playback progress for the scrub fill */
    progress?: number
    /** 0..100 buffered for the scrub bar */
    buffered?: number
    chapters?: Chapter[]
    stillUrl?: string
    /** which floating menu is open, for trigger-button is-open highlight */
    openMenu?: 'source' | 'settings' | 'subs' | 'episodes' | null
    /** hacker-mode fragment heatmap, forwarded to the scrub bar */
    fragments?: { startPct: number; widthPct: number; tone: 'ok' | 'warn' | 'bad'; label: string }[]
    /** current stream URL/type for real hover frame previews */
    previewUrl?: string | null
    previewType?: 'hls' | 'mp4' | null
  }>(),
  { progress: 0, buffered: 0, chapters: () => [], stillUrl: undefined, openMenu: null, fragments: () => [], previewUrl: null, previewType: null, episodeLabel: '' },
)

const emit = defineEmits<{
  (e: 'toggle-play'): void
  (e: 'seek-rel', delta: number): void
  (e: 'seek', pct: number): void
  (e: 'set-volume', v: number): void
  (e: 'toggle-mute'): void
  (e: 'toggle-source'): void
  (e: 'toggle-episodes'): void
  (e: 'toggle-subs'): void
  (e: 'toggle-settings'): void
  (e: 'toggle-pip'): void
  (e: 'toggle-fullscreen'): void
}>()

function fmt(sec: number): string {
  const s = Math.floor(Math.max(0, sec))
  const m = Math.floor(s / 60)
  const ss = s % 60
  return `${m}:${ss.toString().padStart(2, '0')}`
}

function onVolumeInput(event: Event) {
  const v = parseFloat((event.target as HTMLInputElement).value)
  emit('set-volume', v)
}
</script>

<style scoped>
.pl-controls {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 7;
  padding: 30px 0 12px;
  background: linear-gradient(transparent, var(--black-a80));
  transition: opacity 0.2s;
}

/* Scrub row — full-width horizontal line, edge to edge */
.pl-scrub-row {
  display: flex;
  align-items: center;
  margin-bottom: 4px;
}

.pl-btns {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 12px;
}

.pl-spacer {
  flex: 1;
}

/* Icon control buttons now live in the <PlayerIconButton> primitive
   (was `.pl-icon` / `.pl-icon:hover` / `.pl-icon.is-open`). The marker
   classes below (pl-skip-back/fwd, pl-pip-btn, pl-fs-btn) are kept only for
   the mobile-trim media query and are passed through via PlayerIconButton's
   `class` prop. */

/* Volume cluster */
.pl-vol {
  display: flex;
  align-items: center;
}

.pl-vol-range {
  width: 0;
  opacity: 0;
  accent-color: var(--brand-cyan);
  transition: width 0.2s, opacity 0.2s;
  cursor: pointer;
}

.pl-vol:hover .pl-vol-range {
  width: 72px;
  opacity: 1;
  margin-right: 6px;
}

/* Time pill — same geometry as the source pill (.pl-srcbtn) */
.pl-timepill {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  padding: 0 12px;
  margin-left: 4px;
  border-radius: 999px;
  background: var(--white-a8);
  border: 1px solid var(--border);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  flex-shrink: 0;
  user-select: none;
}

.pl-timepill-sep {
  color: var(--muted-foreground);
}

.pl-timepill [data-test='time-duration'] {
  color: var(--ink-2);
}

/* Episodes pill — same geometry/affordance as the source pill */
.pl-epbtn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 36px;
  padding: 0 12px;
  margin-right: 4px;
  border-radius: 999px;
  background: var(--white-a8);
  border: 1px solid var(--border);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
  flex-shrink: 0;
}

.pl-epbtn:hover {
  background: var(--line-strong);
  border-color: var(--accent-line);
}

.pl-epbtn.is-open {
  background: var(--accent-soft);
  border-color: var(--accent-line);
  color: var(--brand-cyan);
}

.pl-epbtn-text {
  white-space: nowrap;
}

/* Source pill */
.pl-srcbtn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  height: 36px;
  padding: 0 12px;
  margin-right: 4px;
  border-radius: 999px;
  background: var(--white-a8);
  border: 1px solid var(--border);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
  max-width: 200px;
}

.pl-srcbtn:hover {
  background: var(--line-strong);
  border-color: var(--accent-line);
}

.pl-srcbtn.is-open {
  background: var(--accent-soft);
  border-color: var(--accent-line);
  color: var(--brand-cyan);
}

.pl-srcbtn-text {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pl-prov-dot {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  flex-shrink: 0;
}

/* Mobile trim: hide skip ±10s, PiP, fullscreen at ≤680px */
@media (max-width: 680px) {
  .pl-skip-back,
  .pl-skip-fwd,
  .pl-pip-btn,
  .pl-fs-btn {
    display: none;
  }

  .pl-vol:hover .pl-vol-range {
    width: 52px;
  }

  /* Episodes pill stays (it's the second chooser) but trims to icon + chevron */
  .pl-epbtn-text {
    display: none;
  }
}
</style>
