<template>
  <div class="flex flex-col" data-test="episodes-panel">
    <div class="px-3 pt-3 pb-2 flex items-baseline justify-between">
      <span class="text-[13px] font-semibold text-white">Episodes</span>
      <span class="text-[11px] text-[var(--muted-foreground)]">{{ episodes.length }}</span>
    </div>

    <div v-if="episodes.length === 0" class="px-3 pb-3 text-[13px] text-[var(--muted-foreground)]">
      No episodes from this source
    </div>

    <div v-else class="grid grid-cols-5 gap-[6px] p-3 pt-1">
      <button
        v-for="ep in episodes"
        :key="ep.key"
        :class="[
          'ep-cell relative h-10 rounded-[var(--r-sm)] border-0 text-[13px] font-semibold transition-colors cursor-pointer overflow-hidden',
          ep.number === selectedNumber
            ? 'text-[var(--brand-cyan)]'
            : isWatched(ep)
              ? 'text-[var(--muted-foreground)] hover:bg-white/[0.14] hover:text-white'
              : 'text-[var(--ink-2)] hover:bg-white/[0.14] hover:text-white',
          ep.isFiller ? 'opacity-50' : '',
        ]"
        :style="ep.number === selectedNumber
          ? 'background: rgba(0,212,255,0.18)'
          : 'background: rgba(255,255,255,0.07)'"
        :title="ep.title ? `${ep.number}. ${ep.title}` : undefined"
        :data-test="`episode-${ep.number}`"
        @click="emit('select', ep)"
      >
        {{ ep.label }}

        <!-- Watched check (user data) -->
        <Check
          v-if="isWatched(ep)"
          class="ep-check"
          :size="10"
          :stroke-width="3"
          aria-hidden="true"
          data-test="ep-watched"
        />

        <!-- Partial watch progress (user data) — bottom underline -->
        <span
          v-if="partialPct(ep) > 0"
          class="ep-progress"
          :style="{ width: partialPct(ep) + '%' }"
          aria-hidden="true"
          data-test="ep-progress"
        />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Check } from 'lucide-vue-next'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

export interface EpisodeUserProgress {
  /** 0..1 fraction of the episode watched */
  pct: number
  completed: boolean
}

const props = withDefaults(
  defineProps<{
    episodes: EpisodeOption[]
    selectedNumber: number | null
    /** episodes with number <= this are watched (anime-list user data) */
    watchedUpTo?: number
    /** per-episode watch progress keyed by episode number (user data) */
    progress?: Record<number, EpisodeUserProgress>
  }>(),
  { watchedUpTo: 0, progress: () => ({}) },
)

const emit = defineEmits<{
  (e: 'select', ep: EpisodeOption): void
}>()

function isWatched(ep: EpisodeOption): boolean {
  return ep.number <= props.watchedUpTo || !!props.progress[ep.number]?.completed
}

/** Partial progress fraction (0..100); 0 for watched/untouched episodes. */
function partialPct(ep: EpisodeOption): number {
  if (isWatched(ep)) return 0
  const p = props.progress[ep.number]
  if (!p || p.pct <= 0) return 0
  return Math.min(100, Math.round(p.pct * 100))
}
</script>

<style scoped>
.ep-check {
  position: absolute;
  top: 3px;
  right: 3px;
  color: var(--brand-cyan);
  opacity: 0.85;
}

.ep-progress {
  position: absolute;
  left: 0;
  bottom: 0;
  height: 2px;
  background: var(--brand-cyan);
  box-shadow: 0 0 4px var(--brand-cyan);
}
</style>
