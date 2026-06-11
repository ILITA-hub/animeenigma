<template>
  <div class="flex flex-wrap items-center gap-2" data-testid="episode-stepper">
    <template v-for="(ep, i) in watchedChips" :key="`done-${ep}`">
      <Badge size="sm" overlay :class="i === watchedChips.length - 1 ? 'opacity-80' : 'opacity-55'">
        {{ t('spotlight.continueWatchingNew.epChip', { n: ep }) }} ✓
      </Badge>
      <span class="text-muted-foreground" aria-hidden="true">→</span>
    </template>
    <span
      class="new-chip inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold bg-pink-500/20 text-pink-400"
      data-testid="stepper-new"
    >
      {{ t('spotlight.continueWatchingNew.epChipNew', { n: newEpisode }) }}
    </span>
  </div>
</template>

<script setup lang="ts">
/**
 * Spotlight UI primitive (v4 H-4 lock, 2026-06-11) — the episode stepper
 * chain: up to two watched chips (dimmed, ✓) → the freshly aired episode
 * as a glowing pink NEW chip. Tells "where you are and what just dropped"
 * in one glance; replaces the poster ribbon that hid the artwork.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Badge from '@/components/ui/Badge.vue'

const props = defineProps<{
  lastWatched: number
  newEpisode: number
}>()

const { t } = useI18n()

// Up to two trailing watched episodes (e.g. last=5 → [4, 5]); a fresh
// viewer (last=0) gets no watched chips, just the NEW one.
const watchedChips = computed<number[]>(() => {
  const eps: number[] = []
  if (props.lastWatched >= 2) eps.push(props.lastWatched - 1)
  if (props.lastWatched >= 1) eps.push(props.lastWatched)
  return eps
})
</script>

<style scoped>
/* Pink ring + glow on the NEW chip — composed shadow no utility pair
   reproduces (inset ring + outer glow in one declaration). */
.new-chip {
  box-shadow:
    inset 0 0 0 1px rgba(255, 77, 141, 0.55),
    0 0 14px rgba(255, 45, 124, 0.35);
}
</style>
