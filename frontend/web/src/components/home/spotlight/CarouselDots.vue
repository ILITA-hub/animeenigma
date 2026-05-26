<template>
  <!--
    Workstream hero-spotlight — labeled-pill dot indicators.

    Split out of CarouselControls.vue (v1.1-polish crop fix). Rendered BELOW
    the .spotlight-frame, OUTSIDE the card — so the card slide occupies the
    full frame height instead of being cropped by a reserved dot strip (the
    earlier `pb-10` / in-frame footer both ate ~40px out of every card).

    Each pill renders the card-type icon from SpotlightIcon, an aria-label
    from the i18n kicker key in cardTokens, and a tooltip via title=. The
    active pill picks up the card's accent background; inactive pills stay
    glass-on-glass.

    Accessibility:
      - Dots are real <button> elements with aria-label / aria-current.
      - Active dot uses aria-current="true"; others "false". (Tabs use
        aria-selected — these are nav buttons, not APG tabs.)
      - data-testid="spotlight-dots" preserved for e2e selectors.
  -->
  <div
    class="mt-3 flex items-center justify-center gap-1.5"
    data-testid="spotlight-dots"
  >
    <!-- Each dot reads from cardTokens by card.type. If the backend ships an
         unknown variant the frontend doesn't yet know about (forward-compat
         scenario), we fall back to FALLBACK_TOKEN so the dot still renders
         rather than throwing on an undefined property access. -->
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
