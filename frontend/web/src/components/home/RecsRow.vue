<template>
  <!-- Notebook 12:04:05 — recommendations rail. Per user request (2026-06-10)
       the personalized "Подобрано для вас" / "Up Next for you" rail is hidden:
       we only render the anonymous "Trending now" discovery rail. Since the
       backend returns the personalized payload exactly when authenticated, we
       gate the whole rail on !isAuthenticated. -->
  <section v-if="!auth.isAuthenticated && recs.length > 0" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <div class="rr-section-head">
      <h2 class="rr-title">
        {{ $t(rowLabelKey) }}
        <span class="rr-count">{{ recs.length }}</span>
      </h2>
    </div>

    <div class="rr-row">
      <div
        v-for="(item, i) in recs"
        :key="item.anime.id"
        class="rr-card-tile"
        @click="onClick(item)"
      >
        <span v-if="item.pinned" class="rr-pin" :title="pinTitle(item)">{{ $t('recs.pinBadge') }}</span>
        <MediaTile :model="fromHomeAnime(item.anime)" />
        <span class="rr-rank" aria-hidden="true">{{ i + 1 }}</span>
      </div>
    </div>
  </section>

  <!-- Loading skeleton — matches the loaded horizontal scroller. -->
  <div v-else-if="!auth.isAuthenticated && isLoading" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <div class="h-8 w-52 bg-white/10 rounded animate-pulse mb-4" />
    <div class="rr-row">
      <div v-for="i in 6" :key="i" class="rr-card-skeleton" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import MediaTile from '@/components/anime/MediaTile.vue'
import { fromHomeAnime } from '@/utils/toCardModel'
import { useRecs, type RecItem } from '@/composables/useRecs'
import { emitRecClick } from '@/utils/recsAnalytics'
import { useAuthStore } from '@/stores/auth'

const { recs, isLoading, rowLabelKey } = useRecs()
const { t } = useI18n()
// Hide the personalized rail for logged-in users (user request 2026-06-10);
// the anonymous "Trending now" rail still renders.
const auth = useAuthStore()

// Pinned items carry the literal signal "s6_pin"; organic items carry the
// backend-surfaced top_contributor (empty string if none). Mirrors the
// contract in recsAnalytics.ts so rec_click → rec_watched correlation works.
function onClick(item: RecItem): void {
  void emitRecClick({
    event_type: 'rec_click',
    anime_id: String(item.anime.id),
    signal_id: item.pinned ? 's6_pin' : item.top_contributor || '',
    pinned: item.pinned,
    pin_source: item.pin_source,
    pin_seed_anime_id: item.pin_seed_anime_id,
    source_route: '/',
    rank: item.rank,
  })
}

// Locale-aware pin reason when the backend provides the key path; fall back to
// the raw English pin_reason, then a generic label.
function pinTitle(item: RecItem): string {
  if (item.pin_reason_key) return t(item.pin_reason_key, (item.pin_reason_data ?? {}) as Record<string, unknown>)
  return item.pin_reason || t('recs.pinBadge')
}
</script>

<style scoped>
/* Mirrors ContinueWatchingRow's section header + horizontal scroller. */
.rr-section-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  margin-bottom: 18px;
  gap: 16px;
}
.rr-title {
  font-family: var(--f-display, "Manrope", "Inter", system-ui, sans-serif);
  font-size: 22px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--foreground);
  display: flex;
  align-items: center;
  gap: 12px;
}
.rr-count {
  font-family: var(--f-mono, "JetBrains Mono", ui-monospace, monospace);
  font-size: 11px;
  letter-spacing: 0.1em;
  color: var(--ink-4, rgba(255, 255, 255, 0.36));
  text-transform: uppercase;
}
.rr-row {
  display: grid;
  grid-auto-flow: column;
  grid-auto-columns: minmax(280px, 360px);
  gap: 14px;
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  padding-bottom: 8px;
  scrollbar-width: thin;
  scrollbar-color: rgba(255, 255, 255, 0.08) transparent;
}
.rr-row::-webkit-scrollbar { height: 6px; }
.rr-row::-webkit-scrollbar-track { background: transparent; }
.rr-row::-webkit-scrollbar-thumb { background: rgba(255, 255, 255, 0.08); border-radius: 999px; }
.rr-row::-webkit-scrollbar-thumb:hover { background: rgba(255, 255, 255, 0.16); }

.rr-card-tile { position: relative; scroll-snap-align: start; }

/* PINNED badge — top-left, brand cyan. */
.rr-pin {
  position: absolute;
  top: 8px;
  left: 8px;
  z-index: 2;
  padding: 2px 7px;
  border-radius: 999px;
  font-family: var(--f-mono, "JetBrains Mono", ui-monospace, monospace);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.08em;
  background: var(--brand-cyan);
  color: rgb(10, 10, 15);
}
/* Rank chip — bottom-right corner. */
.rr-rank {
  position: absolute;
  bottom: 8px;
  right: 8px;
  z-index: 2;
  min-width: 20px;
  height: 20px;
  padding: 0 5px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  font-family: var(--f-mono, "JetBrains Mono", ui-monospace, monospace);
  font-size: 11px;
  color: rgba(255, 255, 255, 0.85);
  background: rgba(0, 0, 0, 0.55);
}

.rr-card-skeleton {
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
