<template>
  <!-- Phase 8 (UX-15 / UA-061). Hidden when no items so logged-in users
       with zero in-progress rows see no degraded affordance — the
       trending row below remains the top-of-home anchor in that case.
       Anonymous users get `items.length === 0` from the composable's
       early-return path.
       Neon Tokyo redesign (Task 6): 16:9 cinematic cards with horizontal
       grid-scroll, play-on-hover, progress bar, and section header. -->
  <section v-if="items.length > 0" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <!-- Section header: .section-head pattern from design handoff -->
    <div class="cw-section-head">
      <h2 class="cw-title">
        {{ $t('home.continueWatching') }}
        <span class="cw-count">{{ items.length }}</span>
      </h2>
      <router-link to="/profile" class="cw-see-all">
        {{ $t('home.continueWatchingAll') }}
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M9 18l6-6-6-6" />
        </svg>
      </router-link>
    </div>

    <!-- Horizontal cinematic grid -->
    <div class="cw-row">
      <router-link
        v-for="item in items"
        :key="item.anime.id + ':' + item.episode_number"
        :to="`/anime/${item.anime.id}?episode=${item.episode_number}`"
        class="cw-card"
      >
        <!-- Cover image with bottom scrim -->
        <div
          class="cw-img"
          :style="item.anime.poster_url ? { backgroundImage: `url(${item.anime.poster_url})` } : {}"
        />

        <!-- Centered play button, revealed on hover -->
        <div class="cw-play" aria-hidden="true">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
            <path d="M5 3l14 9-14 9V3z" />
          </svg>
        </div>

        <!-- Info overlay (bottom) -->
        <div class="cw-info">
          <div class="cw-ep">
            {{ $t('home.continueWatchingEpisode', { n: item.episode_number }) }}
            <template v-if="item.anime.episodes_count"> · {{ item.anime.episodes_count }}</template>
          </div>
          <div class="cw-title-line">
            {{ getLocalizedTitle(item.anime.name, item.anime.name_ru, item.anime.name_jp) }}
          </div>
        </div>

        <!-- Progress bar (3px, cyan with glow) -->
        <div v-if="progressPct(item) > 0" class="cw-progress">
          <div
            class="cw-progress-fill"
            :style="progressBarStyle(item)"
          />
        </div>
      </router-link>
    </div>
  </section>

  <!-- Loading skeleton — horizontal cinematic cards to match loaded state -->
  <div v-else-if="isLoading" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <div class="h-8 w-52 bg-white/10 rounded animate-pulse mb-4" />
    <div class="cw-row">
      <div
        v-for="i in 5"
        :key="i"
        class="cw-card-skeleton"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { getLocalizedTitle } from '@/utils/title'
import { useContinueWatching, type ContinueWatchingItem } from '@/composables/useContinueWatching'

const { items, isLoading } = useContinueWatching(10)

function progressPct(item: ContinueWatchingItem): number {
  if (!item.duration || item.duration <= 0) return 0
  const pct = (item.progress / item.duration) * 100
  // Cap at 100 in case progress > duration (clock skew between heartbeats).
  return Math.max(0, Math.min(100, pct))
}

// IN-02 (Phase 8): when the user is genuinely past 0 but at a tiny
// percentage of the episode, render the cyan bar with a min-width of 4px
// so it's visible.
function progressBarStyle(item: ContinueWatchingItem): Record<string, string> {
  const pct = progressPct(item)
  return {
    width: pct + '%',
    minWidth: '4px',
  }
}
</script>

<style scoped>
/* -----------------------------------------------------------------------
   Section header — mirrors .section-head from design handoff styles.css
   ----------------------------------------------------------------------- */
.cw-section-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  margin-bottom: 18px;
  gap: 16px;
}

.cw-title {
  font-family: var(--f-display, "Manrope", "Inter", system-ui, sans-serif);
  font-size: 22px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--ink, #fff);
  display: flex;
  align-items: center;
  gap: 12px;
}

.cw-count {
  font-family: var(--f-mono, "JetBrains Mono", ui-monospace, monospace);
  font-size: 11px;
  letter-spacing: 0.1em;
  color: var(--ink-4, rgba(255, 255, 255, 0.36));
  text-transform: uppercase;
}

.cw-see-all {
  font-size: 13px;
  color: var(--ink-3, rgba(255, 255, 255, 0.56));
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: color 0.15s ease;
}
.cw-see-all:hover {
  color: var(--accent, #00d4ff);
}

/* -----------------------------------------------------------------------
   Horizontal scroll grid — mirrors .cw-row from design handoff
   ----------------------------------------------------------------------- */
.cw-row {
  display: grid;
  grid-auto-flow: column;
  grid-auto-columns: minmax(280px, 1fr);
  gap: 14px;
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  padding-bottom: 8px;
  /* Hide scrollbar cross-browser while keeping scroll functionality */
  scrollbar-width: thin;
  scrollbar-color: rgba(255, 255, 255, 0.08) transparent;
}
.cw-row::-webkit-scrollbar {
  height: 6px;
}
.cw-row::-webkit-scrollbar-track {
  background: transparent;
}
.cw-row::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.08);
  border-radius: 999px;
}
.cw-row::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.16);
}

/* -----------------------------------------------------------------------
   16:9 cinematic card — mirrors .cw-card from design handoff
   ----------------------------------------------------------------------- */
.cw-card {
  position: relative;
  scroll-snap-align: start;
  border-radius: var(--r-lg, 16px);
  overflow: hidden;
  aspect-ratio: 16 / 9;
  background: var(--color-surface, #11111c);
  border: 1px solid var(--line, rgba(255, 255, 255, 0.06));
  cursor: pointer;
  display: block;
  transition: transform 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}
.cw-card:hover {
  transform: translateY(-2px);
  border-color: var(--accent-line, rgba(0, 212, 255, 0.28));
  box-shadow: var(--accent-glow, 0 0 30px rgba(0, 212, 255, 0.28));
}
.cw-card:focus-visible {
  outline: 2px solid var(--accent, #00d4ff);
  outline-offset: 2px;
}

/* Cover image with bottom gradient scrim */
.cw-img {
  position: absolute;
  inset: 0;
  background-size: cover;
  background-position: center;
  background-color: var(--color-surface, #11111c);
}
.cw-img::after {
  content: "";
  position: absolute;
  inset: 0;
  background: linear-gradient(
    180deg,
    transparent 30%,
    rgba(8, 8, 15, 0.55) 65%,
    rgba(8, 8, 15, 0.95) 100%
  );
}

/* Centered play button — hidden at rest, revealed on card hover */
.cw-play {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 52px;
  height: 52px;
  border-radius: 999px;
  background: rgba(0, 212, 255, 0.95);
  display: grid;
  place-items: center;
  color: #001218;
  opacity: 0;
  transition: opacity 0.2s ease, transform 0.2s ease;
  box-shadow: 0 0 24px rgba(0, 212, 255, 0.5);
  z-index: 1;
}
.cw-card:hover .cw-play {
  opacity: 1;
  transform: translate(-50%, -50%) scale(1.06);
}

/* Info overlay at the bottom of the card */
.cw-info {
  position: absolute;
  left: 14px;
  right: 14px;
  bottom: 12px;
  z-index: 1;
}

/* Episode label: mono, cyan, uppercase */
.cw-ep {
  font-family: var(--f-mono, "JetBrains Mono", ui-monospace, monospace);
  font-size: 10px;
  letter-spacing: 0.1em;
  color: var(--accent, #00d4ff);
  text-transform: uppercase;
  margin-bottom: 4px;
}

/* Title: single line, ellipsis */
.cw-title-line {
  font-weight: 600;
  font-size: 14px;
  color: var(--ink, #fff);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* -----------------------------------------------------------------------
   Progress bar — 3px, cyan with glow, absolute bottom
   ----------------------------------------------------------------------- */
.cw-progress {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 3px;
  background: rgba(255, 255, 255, 0.08);
  z-index: 2;
}
.cw-progress-fill {
  height: 100%;
  background: var(--accent, #00d4ff);
  box-shadow: 0 0 8px var(--accent, #00d4ff);
  transition: width 0.3s ease;
}

/* -----------------------------------------------------------------------
   Loading skeleton card
   ----------------------------------------------------------------------- */
.cw-card-skeleton {
  scroll-snap-align: start;
  border-radius: var(--r-lg, 16px);
  aspect-ratio: 16 / 9;
  background: rgba(255, 255, 255, 0.06);
  animation: pulse 1.5s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
