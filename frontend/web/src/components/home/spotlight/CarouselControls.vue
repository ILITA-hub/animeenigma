<template>
  <!--
    Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-02.

    Stateless carousel chrome: two chevron buttons (prev/next) and `cardCount`
    dot indicators. Parent component (HeroSpotlightBlock in Plan 02-04) owns
    the carousel state machine and consumes the `prev` / `next` / `goto`
    events emitted from this component.

    A11y per UI-SPEC §Accessibility Contract:
      - Dot buttons are real <button> elements with aria-label via i18n and
        aria-current="true"/"false" derived from props.
      - Chevron <button>s have aria-label via i18n; inner SVG marked
        aria-hidden="true" since the button itself carries the label.
      - All interactive elements opt into the project's .touch-target utility
        (44x44 WCAG 2.5.5 tap-target sizing) even when the visual dot is 8x8.

    No <style> block — all styling via Tailwind utilities + project tokens
    from main.css (.glass-card, .touch-target, global :focus-visible ring).

    Iteration approach: use Array.from({length: cardCount}, (_, idx) => idx)
    so the loop variable is 0-indexed directly. This keeps the emitted
    `goto` payload 0-indexed without an off-by-one conversion in the
    template; the human-facing aria-label uses `idx + 1` for the slide
    number.
  -->
  <div class="contents">
    <!-- Chevron PREV -->
    <button
      type="button"
      class="absolute left-2 top-1/2 -translate-y-1/2 z-10 w-10 h-10 touch-target flex items-center justify-center rounded-full bg-white/10 backdrop-blur-sm text-white hover:bg-cyan-500/20 hover:text-cyan-400 transition-colors focus:outline-none"
      :aria-label="t('spotlight.prevSlide')"
      @click="$emit('prev')"
    >
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
      </svg>
    </button>

    <!-- Chevron NEXT -->
    <button
      type="button"
      class="absolute right-2 top-1/2 -translate-y-1/2 z-10 w-10 h-10 touch-target flex items-center justify-center rounded-full bg-white/10 backdrop-blur-sm text-white hover:bg-cyan-500/20 hover:text-cyan-400 transition-colors focus:outline-none"
      :aria-label="t('spotlight.nextSlide')"
      @click="$emit('next')"
    >
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
      </svg>
    </button>

    <!-- Dot indicators (one per card) -->
    <div
      class="absolute bottom-3 left-1/2 -translate-x-1/2 z-10 flex items-center gap-2"
      role="tablist"
    >
      <button
        v-for="idx in dotIndices"
        :key="idx"
        type="button"
        :class="[
          'touch-target w-2 h-2 rounded-full transition-colors',
          idx === currentIndex ? 'bg-cyan-400' : 'bg-white/30 hover:bg-white/50',
        ]"
        :aria-label="t('spotlight.goToSlide', { n: idx + 1 })"
        :aria-current="idx === currentIndex ? 'true' : 'false'"
        @click="$emit('goto', idx)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  currentIndex: number
  cardCount: number
}>()

defineEmits<{
  (e: 'prev'): void
  (e: 'next'): void
  (e: 'goto', index: number): void
}>()

const { t } = useI18n()

// 0-indexed array of dot indices. Centralizing this here means the template
// can iterate over a plain number[] (no template helper needed) and the
// `goto` emit payload is always 0-indexed by construction.
const dotIndices = computed<number[]>(() =>
  Array.from({ length: Math.max(0, props.cardCount) }, (_, i) => i),
)
</script>
