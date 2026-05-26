<template>
  <!--
    Workstream hero-spotlight — carousel prev/next chevrons.

    Stateless arrows overlaid on the card's left/right edges (absolute,
    positioned relative to the .spotlight-frame). The labeled-pill dot
    indicators were split out to CarouselDots.vue (v1.1-polish crop fix) and
    now render BELOW the frame, so the card slide can use the full frame
    height instead of being cropped by an in-frame dot strip.

    `display:contents` on the wrapper lets the two absolute buttons become
    direct children of the frame for positioning. Parent (HeroSpotlightBlock)
    owns carousel state and handles the prev / next events.
  -->
  <div class="contents">
    <!-- Chevron PREV — 44×44 touch target -->
    <button
      type="button"
      class="absolute left-2 top-1/2 -translate-y-1/2 z-10 w-11 h-11 min-h-[44px] touch-target flex items-center justify-center rounded-full bg-white/10 backdrop-blur-sm text-white hover:bg-cyan-500/20 hover:text-cyan-400 transition-colors focus:outline-none"
      :aria-label="t('spotlight.prevSlide')"
      @click="$emit('prev')"
    >
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
      </svg>
    </button>

    <!-- Chevron NEXT — 44×44 touch target -->
    <button
      type="button"
      class="absolute right-2 top-1/2 -translate-y-1/2 z-10 w-11 h-11 min-h-[44px] touch-target flex items-center justify-center rounded-full bg-white/10 backdrop-blur-sm text-white hover:bg-cyan-500/20 hover:text-cyan-400 transition-colors focus:outline-none"
      :aria-label="t('spotlight.nextSlide')"
      @click="$emit('next')"
    >
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineEmits<{
  (e: 'prev'): void
  (e: 'next'): void
}>()

const { t } = useI18n()
</script>
