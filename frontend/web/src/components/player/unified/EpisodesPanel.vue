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
          'h-9 rounded-[var(--r-sm)] border-0 text-[13px] font-semibold transition-colors cursor-pointer',
          ep.number === selectedNumber
            ? 'text-[var(--brand-cyan)]'
            : 'text-[var(--ink-2)] hover:bg-white/[0.14] hover:text-white',
          ep.isFiller ? 'opacity-50' : '',
        ]"
        :style="ep.number === selectedNumber
          ? 'background: rgba(0,212,255,0.18)'
          : 'background: rgba(255,255,255,0.07)'"
        :data-test="`episode-${ep.number}`"
        @click="emit('select', ep)"
      >
        {{ ep.label }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

defineProps<{
  episodes: EpisodeOption[]
  selectedNumber: number | null
}>()

const emit = defineEmits<{
  (e: 'select', ep: EpisodeOption): void
}>()
</script>
