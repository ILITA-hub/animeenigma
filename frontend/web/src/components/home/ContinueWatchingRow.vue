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
        <ChevronRight class="size-3.5" aria-hidden="true" />
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
import { ChevronRight } from 'lucide-vue-next'
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
  font-family: var(--font-display);
  font-size: 22px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--foreground, #fff);
  display: flex;
  align-items: center;
  gap: 12px;
}

.cw-count {
  font-family: var(--font-mono);
  font-size: 11px;
  letter-spacing: 0.1em;
  color: var(--ink-4, var(--ink-4));
  text-transform: uppercase;
}

.cw-see-all {
  font-size: 13px;
  color: var(--muted-foreground, var(--muted-foreground));
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
  grid-auto-columns: minmax(280px, 360px);
  gap: 14px;
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  padding-bottom: 8px;
  /* Hide scrollbar cross-browser while keeping scroll functionality */
  scrollbar-width: thin;
  scrollbar-color: var(--white-a8) transparent;
}
.cw-row::-webkit-scrollbar {
  height: 6px;
}
.cw-row::-webkit-scrollbar-track {
  background: transparent;
}
.cw-row::-webkit-scrollbar-thumb {
  background: var(--white-a8);
  border-radius: 999px;
}
.cw-row::-webkit-scrollbar-thumb:hover {
  background: var(--white-a20);
}

/* -----------------------------------------------------------------------
   MediaTile grid cell — snap alignment for the horizontal scroller
   ----------------------------------------------------------------------- */
/* content-visibility culls off-viewport tiles from style/paint entirely —
   the 2026-07-04 trace showed rail cards 3 viewport-widths away repainting
   every frame (skeleton overlays). Intrinsic size ≈ the 16:9 grid column. */
.cw-card-tile {
  scroll-snap-align: start;
  content-visibility: auto;
  contain-intrinsic-size: auto 320px auto 180px;
}

/* -----------------------------------------------------------------------
   Loading skeleton card
   ----------------------------------------------------------------------- */
.cw-card-skeleton {
  scroll-snap-align: start;
  border-radius: var(--r-lg, 16px);
  aspect-ratio: 16 / 9;
  background: var(--line);
  animation: pulse 1.5s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
