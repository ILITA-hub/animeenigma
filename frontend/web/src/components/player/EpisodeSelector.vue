<script setup lang="ts">
/**
 * Shared episode grid for every video player (Kodik, AnimeLib, OurEnglish,
 * Hanime, Raw, Anime18) in BOTH solo (Anime.vue) and Watch Together.
 * Presentational only — the host player owns episode + watched state and
 * passes a normalized list down.
 *
 * Watched episodes render in the HOST PLAYER'S accent: this component
 * redefines its own .accent-* classes against the inherited --player-accent
 * / --player-accent-rgb vars each player declares on its root. (Scoped class
 * definitions don't cross component boundaries, but CSS custom properties
 * inherit — so we redefine the classes here against the inherited vars.)
 */
import type { EpisodeOption } from './EpisodeSelector.types'

const props = withDefaults(defineProps<{
  episodes: EpisodeOption[]
  selectedKey: string | number | null
  watchedUpTo?: number
}>(), {
  watchedUpTo: 0,
})

const emit = defineEmits<{ (e: 'select', key: string | number): void }>()

/** Selection match is type-tolerant: data-* keys are stringified, so a host
 *  passing a number selectedKey against a string key (or vice-versa) must
 *  still match. */
function isSelected(ep: EpisodeOption): boolean {
  return props.selectedKey !== null && String(props.selectedKey) === String(ep.key)
}

// Episode numbers are 1-based across every provider, so the default
// watchedUpTo of 0 correctly marks nothing watched (no ep.number <= 0).
function isWatched(ep: EpisodeOption): boolean {
  return ep.number <= props.watchedUpTo
}
</script>

<template>
  <div class="flex flex-wrap gap-2 max-h-32 overflow-y-auto custom-scrollbar p-1">
    <button
      v-for="ep in episodes"
      :key="ep.key"
      :data-wt-id="`episode:${ep.key}`"
      @click="emit('select', ep.key)"
      class="relative w-12 h-10 rounded-lg text-sm font-medium transition-all"
      :class="[
        isSelected(ep)
          ? 'accent-bg text-white'
          : isWatched(ep)
            ? 'accent-bg-muted accent-text border accent-border hover:brightness-125'
            : 'bg-white/10 text-white hover:bg-white/20',
        ep.isFiller ? 'opacity-60' : '',
      ]"
    >
      {{ ep.label }}
      <span
        v-if="isWatched(ep) && !isSelected(ep)"
        class="absolute -top-1 -right-1 w-3 h-3 accent-bg rounded-full flex items-center justify-center"
      >
        <svg class="w-2 h-2 text-black" fill="currentColor" viewBox="0 0 20 20">
          <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
        </svg>
      </span>
    </button>
  </div>
</template>

<style scoped>
/* Redefine accent utilities against the INHERITED player vars so watched
   episodes glow in the host player's accent. Each player declares
   --player-accent + --player-accent-rgb on its root. */
.accent-bg { background-color: var(--player-accent); }
.accent-text { color: color-mix(in srgb, var(--player-accent), white 40%); }
.accent-border { border-color: var(--player-accent); }
.accent-bg-muted { background-color: rgba(var(--player-accent-rgb), 0.28); }
</style>
