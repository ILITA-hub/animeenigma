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
      <MediaTile
        v-for="item in items"
        :key="item.anime.id + ':' + item.episode_number"
        :model="fromContinueWatching(item)"
        :progress-pct="progressPct(item)"
        class="cw-card-tile"
      />
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
import MediaTile from '@/components/anime/MediaTile.vue'
import { fromContinueWatching } from '@/utils/toCardModel'
import { useContinueWatching, type ContinueWatchingItem } from '@/composables/useContinueWatching'

const { items, isLoading } = useContinueWatching(10)

function progressPct(item: ContinueWatchingItem): number {
  if (!item.duration || item.duration <= 0) return 0
  const pct = (item.progress / item.duration) * 100
  // Cap at 100 in case progress > duration (clock skew between heartbeats).
  return Math.max(0, Math.min(100, pct))
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
  color: var(--foreground, #fff);
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
  color: var(--muted-foreground, rgba(255, 255, 255, 0.56));
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: color 0.15s ease;
}
.cw-see-all:hover {
  color: var(--brand-cyan, #00d4ff);
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
   MediaTile grid cell — snap alignment for the horizontal scroller
   ----------------------------------------------------------------------- */
.cw-card-tile { scroll-snap-align: start; }

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
