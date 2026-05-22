<template>
  <!--
    Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-02,
    refactored in v1.1-polish Phase 01 Task 5 (HSB-V11-CC-06).

    Stateless carousel chrome:
      - Two chevron buttons (prev/next) with 44×44 tap targets.
      - One labeled-pill dot per card. Each pill renders the card-type
        icon from SpotlightIcon, an aria-label from the i18n kicker key
        in cardTokens, and a tooltip via title=. The active pill picks
        up the card's accent background; inactive pills stay glass-on-
        glass.

    Parent (HeroSpotlightBlock) owns the carousel state and emits handlers
    for prev / next / goto. The dots iterate the `cards` prop (NOT a bare
    cardCount integer) so each dot can read its card's type/icon/accent.

    Accessibility:
      - Dots are real <button> elements with aria-label / aria-current.
      - Active dot uses aria-current="true"; others "false". (Tabs use
        aria-selected — these are nav buttons, not APG tabs.)
      - data-testid="spotlight-dots" preserved for e2e selectors.
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

    <!-- Labeled-pill dot indicators (one per card).

         Each pill carries the card-type icon + the card's kicker label
         (i18n via cardTokens[card.type].kickerKey). Active pill picks up
         the accent background from accentDotBg[token.accent]; inactive
         stay glass-on-glass.

         data-testid is preserved verbatim for e2e selector compatibility
         (frontend/web/e2e/spotlight*.spec.ts). -->
    <div
      class="absolute bottom-3 left-1/2 -translate-x-1/2 z-10 flex items-center gap-1.5"
      data-testid="spotlight-dots"
    >
      <!-- Each dot reads from cardTokens by card.type. If the backend
           ships an unknown variant the frontend doesn't yet know about
           (forward-compat scenario), we fall back to FALLBACK_TOKEN so
           the dot still renders rather than throwing on an undefined
           property access. -->
      <button
        v-for="(card, i) in cards"
        :key="`${card.type}:${i}`"
        type="button"
        :class="[
          'group relative inline-flex items-center justify-center w-8 h-8 rounded-full transition',
          i === currentIndex
            ? `${accentDotBg[tokenFor(card.type).accent]} scale-110`
            : 'bg-white/10 hover:bg-white/20 text-white/70',
        ]"
        :aria-label="t(tokenFor(card.type).kickerKey)"
        :aria-current="i === currentIndex ? 'true' : 'false'"
        :title="t(tokenFor(card.type).kickerKey)"
        @click="$emit('goto', i)"
      >
        <SpotlightIcon :name="tokenFor(card.type).icon" class="w-3.5 h-3.5" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { SpotlightCard } from '@/types/spotlight'
import { cardTokens, accentDotBg, type CardToken, type SpotlightCardType } from './tokens'
import SpotlightIcon from './SpotlightIcon.vue'

defineProps<{
  currentIndex: number
  cards: SpotlightCard[]
}>()

defineEmits<{
  (e: 'prev'): void
  (e: 'next'): void
  (e: 'goto', index: number): void
}>()

const { t } = useI18n()

// Forward-compat fallback for the "backend ships unknown variant" case.
// Mirrors the HeroSpotlightBlock.spec.ts contract that an unknown type
// renders silently (no console.error, no thrown access on cardTokens).
const FALLBACK_TOKEN: CardToken = {
  accent: 'cyan',
  kickerKey: 'spotlight.regionLabel',
  icon: 'sparkles',
}
function tokenFor(type: string): CardToken {
  return cardTokens[type as SpotlightCardType] ?? FALLBACK_TOKEN
}
</script>
