<template>
  <div class="pl-controls">
    <div class="pl-btns">

      <!-- Play / Pause -->
      <button
        class="pl-icon"
        :aria-label="playing ? 'Pause' : 'Play'"
        data-test="play-pause"
        @click="emit('toggle-play')"
      >
        <svg v-if="playing" width="20" height="20" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M10 9v6m4-6v6" />
        </svg>
        <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
          <path d="M5 3l14 9-14 9V3z" />
        </svg>
      </button>

      <!-- −10s (hidden on mobile via CSS) -->
      <button
        class="pl-icon pl-skip-back"
        aria-label="Back 10 seconds"
        data-test="seek-back"
        @click="emit('seek-rel', -10)"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M12.066 11.2a1 1 0 000 1.6l5.334 4A1 1 0 0019 16V8a1 1 0 00-1.6-.8l-5.334 4z" fill="currentColor" stroke="none" />
          <path d="M4.066 11.2a1 1 0 000 1.6l5.334 4A1 1 0 0011 16V8a1 1 0 00-1.6-.8l-5.334 4z" fill="currentColor" stroke="none" />
          <text x="7.5" y="23.5" font-size="5.5" font-weight="700" font-family="var(--font-mono,monospace)" fill="currentColor" text-anchor="middle">10</text>
        </svg>
      </button>

      <!-- +10s (hidden on mobile via CSS) -->
      <button
        class="pl-icon pl-skip-fwd"
        aria-label="Forward 10 seconds"
        data-test="seek-fwd"
        @click="emit('seek-rel', 10)"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M11.934 11.2a1 1 0 010 1.6l-5.334 4A1 1 0 015 16V8a1 1 0 011.6-.8l5.334 4z" fill="currentColor" stroke="none" />
          <path d="M19.934 11.2a1 1 0 010 1.6l-5.334 4A1 1 0 0113 16V8a1 1 0 011.6-.8l5.334 4z" fill="currentColor" stroke="none" />
          <text x="16.5" y="23.5" font-size="5.5" font-weight="700" font-family="var(--font-mono,monospace)" fill="currentColor" text-anchor="middle">10</text>
        </svg>
      </button>

      <!-- Volume cluster (hover to expand) -->
      <div class="pl-vol">
        <button
          class="pl-icon"
          :aria-label="muted || volume === 0 ? 'Unmute' : 'Mute'"
          data-test="mute"
          @click="emit('toggle-mute')"
        >
          <!-- Muted -->
          <svg v-if="muted || volume === 0" width="20" height="20" viewBox="0 0 24 24" fill="none"
            stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M11 5L6 9H2v6h4l5 4V5z" />
            <path d="M23 9l-6 6m0-6l6 6" />
          </svg>
          <!-- Volume medium -->
          <svg v-else-if="volume < 0.5" width="20" height="20" viewBox="0 0 24 24" fill="none"
            stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M11 5L6 9H2v6h4l5 4V5z" />
            <path d="M15.54 8.46a5 5 0 010 7.07" />
          </svg>
          <!-- Volume high -->
          <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none"
            stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M11 5L6 9H2v6h4l5 4V5zM15.54 8.46a5 5 0 010 7.07M19.07 4.93a10 10 0 010 14.14" />
          </svg>
        </button>
        <input
          type="range"
          class="pl-vol-range"
          min="0"
          max="1"
          step="0.05"
          :value="muted ? 0 : volume"
          aria-label="Volume"
          data-test="volume-slider"
          @input="onVolumeInput"
        />
      </div>

      <!-- Time display -->
      <span class="pl-time" data-test="time-current">{{ fmt(currentTime) }}</span>
      <span class="pl-time-sep" aria-hidden="true">/</span>
      <span class="pl-time pl-time-dur" data-test="time-duration">{{ fmt(duration) }}</span>

      <!-- Spacer -->
      <span class="pl-spacer" aria-hidden="true" />

      <!-- Source pill -->
      <button
        class="pl-srcbtn"
        :class="{ 'is-open': false }"
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
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      <!-- Subtitles (CC) -->
      <button
        class="pl-icon"
        aria-label="Subtitles"
        data-test="toggle-subs"
        @click="emit('toggle-subs')"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M4 5h16a1 1 0 011 1v12a1 1 0 01-1 1H4a1 1 0 01-1-1V6a1 1 0 011-1zm5.5 5.2a1.8 1.8 0 100 3.6m7 0a1.8 1.8 0 110-3.6" />
        </svg>
      </button>

      <!-- Settings gear -->
      <button
        class="pl-icon"
        aria-label="Settings"
        data-test="toggle-settings"
        @click="emit('toggle-settings')"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065zM15 12a3 3 0 11-6 0 3 3 0 016 0z" />
        </svg>
      </button>

      <!-- PiP (hidden on mobile via CSS) -->
      <button
        class="pl-icon pl-pip-btn"
        aria-label="Picture in Picture"
        data-test="toggle-pip"
        @click="emit('toggle-pip')"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M19 11h-6a1 1 0 00-1 1v4a1 1 0 001 1h6a1 1 0 001-1v-4a1 1 0 00-1-1zM4 5h16a1 1 0 011 1v3M4 5a1 1 0 00-1 1v12a1 1 0 001 1h5" />
        </svg>
      </button>

      <!-- Theater mode -->
      <button
        class="pl-icon"
        :class="{ 'is-open': theater }"
        :aria-label="theater ? 'Exit theater mode' : 'Theater mode'"
        :aria-pressed="theater"
        data-test="toggle-theater"
        @click="emit('toggle-theater')"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M4 8V5a1 1 0 011-1h3M16 4h3a1 1 0 011 1v3M20 16v3a1 1 0 01-1 1h-3M8 20H5a1 1 0 01-1-1v-3" />
        </svg>
      </button>

      <!-- Fullscreen (hidden on mobile via CSS) -->
      <button
        class="pl-icon pl-fs-btn"
        aria-label="Fullscreen"
        data-test="toggle-fullscreen"
        @click="emit('toggle-fullscreen')"
      >
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M4 8V4h4M16 4h4v4M20 16v4h-4M8 20H4v-4" />
        </svg>
      </button>

    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  playing: boolean
  currentTime: number
  duration: number
  volume: number
  muted: boolean
  providerName: string
  providerHue: string
  audioLabel: string
  theater: boolean
}>()

const emit = defineEmits<{
  (e: 'toggle-play'): void
  (e: 'seek-rel', delta: number): void
  (e: 'set-volume', v: number): void
  (e: 'toggle-mute'): void
  (e: 'toggle-source'): void
  (e: 'toggle-subs'): void
  (e: 'toggle-settings'): void
  (e: 'toggle-pip'): void
  (e: 'toggle-theater'): void
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
  padding: 30px 16px 12px;
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.82));
}

.pl-btns {
  display: flex;
  align-items: center;
  gap: 4px;
}

.pl-spacer {
  flex: 1;
}

.pl-icon {
  width: 40px;
  height: 40px;
  display: grid;
  place-items: center;
  border-radius: var(--r-md);
  background: transparent;
  border: 0;
  color: #fff;
  cursor: pointer;
  transition: background 0.15s;
  flex-shrink: 0;
}

.pl-icon:hover {
  background: rgba(255, 255, 255, 0.14);
}

.pl-icon.is-open {
  background: rgba(0, 212, 255, 0.2);
  color: var(--brand-cyan);
}

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

/* Time labels */
.pl-time {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.85);
  flex-shrink: 0;
  font-variant-numeric: tabular-nums;
  min-width: 38px;
}

.pl-time-sep {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.4);
  flex-shrink: 0;
  margin: 0 1px;
}

.pl-time-dur {
  color: rgba(255, 255, 255, 0.55);
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
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid var(--border);
  color: #fff;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
  max-width: 200px;
}

.pl-srcbtn:hover {
  background: rgba(255, 255, 255, 0.14);
  border-color: var(--accent-line);
}

.pl-srcbtn.is-open {
  background: rgba(0, 212, 255, 0.16);
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
}
</style>
