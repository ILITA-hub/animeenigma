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

    Neon Tokyo restyle (feat/homepage-neon-tokyo-redesign):
      36×36 round pill, rgba(8,8,15,0.7) bg + blur(8px) backdrop filter,
      1px solid var(--line) border, var(--ink-2) icon, hover raises to
      rgba(255,255,255,.08) + var(--line-strong) border + var(--foreground).
      Transcribed from .arrow-btn / .arrow-l / .arrow-r in the design
      handoff styles.css.
  -->
  <div class="contents">
    <!-- Chevron PREV — 44×44 touch target wraps the 36×36 visual pill -->
    <button
      type="button"
      class="arrow-prev"
      :aria-label="t('spotlight.prevSlide')"
      @click="$emit('prev')"
    >
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
      </svg>
    </button>

    <!-- Chevron NEXT — 44×44 touch target wraps the 36×36 visual pill -->
    <button
      type="button"
      class="arrow-next"
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

<style scoped>
/* Neon Tokyo arrow buttons — transcribed from .arrow-btn, .arrow-l, .arrow-r
   in design_handoff_homepage_redesign/styles.css.
   36×36 visual pill inside a 44×44 min-touch-target container. */
.arrow-prev,
.arrow-next {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  z-index: 10;
  /* 44×44 touch target; the inner pill is visually 36×36 via padding */
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  min-height: 44px;
  /* Visual pill */
  background: rgba(8, 8, 15, 0.7);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  border: 1px solid var(--line);
  border-radius: 999px;
  color: var(--ink-2);
  /* Hidden by default so the chevrons don't overlap card content (e.g. the
     RandomTail poster on the left). Revealed on hover/focus by the global
     `.spotlight-frame:hover .arrow-prev` rule in main.css (scoped CSS can't
     match an ancestor in a sibling component). Touch devices that can't
     hover get them back via @media (hover: none) in main.css. */
  opacity: 0;
  transition: opacity 0.2s ease, background 0.15s ease, border-color 0.15s ease, color 0.15s ease;
  cursor: pointer;
}
.arrow-prev { left: 20px; }
.arrow-next { right: 20px; }

.arrow-prev:hover,
.arrow-next:hover {
  background: rgba(255, 255, 255, 0.08);
  border-color: var(--line-strong);
  color: var(--foreground);
}

.arrow-prev:focus-visible,
.arrow-next:focus-visible {
  outline: 2px solid var(--accent-line);
  outline-offset: 2px;
}
</style>
