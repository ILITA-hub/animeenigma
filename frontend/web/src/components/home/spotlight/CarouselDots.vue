<template>
  <!--
    Workstream hero-spotlight — v4 A-1 lock (2026-06-11, spec:
    2026-06-11-spotlight-v3-controls-and-cards.md). Icon menu replacing
    the anonymous 26×4 pills: every card renders a 32px (28px on mobile)
    round glass icon button; the ACTIVE card expands into an icon+label
    pill in its brand-triad accent — you always see where you are and
    what's around. Inactive buttons keep the label as title= tooltip +
    aria-label.

    Centered below the frame. The skeleton state in HeroSpotlightBlock
    reserves this exact row height (mt-3 + h-8) so the menu appearing
    after load causes ZERO layout shift (the old dots row pushed the page
    ~28px).

    A-2 (in-frame progress segments) is reserved as a future variety
    option — see the v4 spec.

    Accessibility: real <button>s, aria-label from the kicker i18n key,
    aria-current on the active item; data-testid="spotlight-dots" kept
    for e2e selectors.
  -->
  <div
    class="mt-3 flex items-center justify-center gap-2 px-4 flex-wrap"
    data-testid="spotlight-dots"
  >
    <button
      v-for="(card, i) in cards"
      :key="`${card.type}:${i}`"
      type="button"
      class="menu-item inline-flex items-center justify-center rounded-full border transition-all duration-200"
      :class="
        i === currentIndex
          ? `${accentMenuPill[tokenFor(card.type).accent]} gap-2 px-3.5 h-8 menu-active`
          : 'w-8 h-8 bg-white/[0.06] border-white/10 text-white/50 hover:text-white hover:bg-white/10 hover:border-white/20'
      "
      :aria-label="t(tokenFor(card.type).kickerKey)"
      :aria-current="i === currentIndex ? 'true' : 'false'"
      :title="t(tokenFor(card.type).kickerKey)"
      @click="$emit('goto', i)"
    >
      <SpotlightIcon :name="tokenFor(card.type).icon" class="w-4 h-4 flex-shrink-0" />
      <span
        v-if="i === currentIndex"
        class="font-mono text-[10px] uppercase tracking-[0.1em] font-medium whitespace-nowrap"
        data-testid="active-menu-label"
      >
        {{ t(tokenFor(card.type).kickerKey) }}
      </span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { SpotlightCard } from '@/types/spotlight'
import { cardTokens, accentMenuPill, type CardToken, type SpotlightCardType } from './tokens'
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

<style scoped>
/* 44px effective touch target around the 32px visual button (28px on
   small screens) without changing layout — padding-box trick via a
   transparent ::after hit area. */
.menu-item {
  position: relative;
  cursor: pointer;
}
.menu-item::after {
  content: '';
  position: absolute;
  inset: -6px;
}
.menu-item:focus-visible {
  outline: 2px solid var(--accent-line);
  outline-offset: 2px;
}
@media (max-width: 480px) {
  .menu-item:not(.menu-active) {
    width: 28px;
    height: 28px;
  }
  .menu-item.menu-active {
    height: 28px;
  }
}
</style>
