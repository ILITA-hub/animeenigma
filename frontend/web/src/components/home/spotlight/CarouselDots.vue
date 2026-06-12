<template>
  <!--
    Workstream hero-spotlight — v4 A-1 lock (2026-06-11, spec:
    2026-06-11-spotlight-v3-controls-and-cards.md). Icon menu replacing
    the anonymous 26×4 pills: every card renders a 32px (28px on mobile)
    round glass icon button; the ACTIVE card expands into an icon+label
    pill in its brand-triad accent — you always see where you are and
    what's around. Inactive buttons keep the label as title= tooltip +
    aria-label.

    ARR-1 lock (2026-06-11): the prev/next chevrons moved INTO this row,
    flanking the icon anchors — the in-frame edge overlays (old
    CarouselControls) collided with card content (terminal, deck) that
    deliberately runs to the frame edges. One control cluster, always
    visible, zero overlap on any card or viewport.

    Smooth-pill (same lock): the label is ALWAYS rendered inside a
    grid-template-columns 0fr→1fr wrapper, so activation ANIMATES the
    pill open instead of instantly reflowing the whole row («менюшка
    пролагивает из-за смещения текста»).

    Centered below the frame. The skeleton state in HeroSpotlightBlock
    reserves this exact row height (mt-3 + h-8) so the menu appearing
    after load causes ZERO layout shift.

    A-2 (in-frame progress segments) is reserved as a future variety
    option — see the v4 spec.

    Accessibility: real <button>s, aria-label from the kicker i18n key,
    aria-current on the active item; data-testid="spotlight-dots" kept
    for e2e selectors. Tab order: prev → anchors → next.
  -->
  <div
    class="relative mt-3 flex items-center justify-center gap-2 px-12 flex-wrap"
    data-testid="spotlight-dots"
    :style="{ '--menu-edge': `${edgeOffset + 32}px` }"
  >
    <!-- Chevrons are PINNED (absolute), not part of the centered flex
         flow: the active pill changes width per card, and a flow-positioned
         arrow would drift under the cursor on every flip («зафиксируй
         стрелочки», 2026-06-11). They sit at a FIXED offset from center —
         just past the widest possible menu width for THIS card set (edge
         pinning was «не удобно», 2026-06-12) — so they're close to the
         cluster yet never move. DOM order stays prev → anchors → next. -->
    <button
      type="button"
      class="menu-item menu-nav menu-nav-prev absolute top-1/2 -translate-y-1/2 inline-flex items-center justify-center rounded-full border w-8 h-8 bg-white/[0.06] border-white/10 text-white/50 hover:text-white hover:bg-white/10 hover:border-white/20 transition-colors duration-200"
      :aria-label="t('spotlight.prevSlide')"
      data-testid="menu-prev"
      @click="$emit('prev')"
    >
      <ChevronLeft class="w-4 h-4" aria-hidden="true" />
    </button>

    <button
      v-for="(card, i) in cards"
      :key="`${card.type}:${i}`"
      type="button"
      class="menu-item inline-flex items-center rounded-full border h-8 px-2 transition-[background-color,border-color,color,padding] duration-300"
      :class="
        i === currentIndex
          ? `${accentMenuPill[tokenFor(card.type).accent]} px-3 menu-active`
          : 'bg-white/[0.06] border-white/10 text-white/50 hover:text-white hover:bg-white/10 hover:border-white/20'
      "
      :aria-label="t(tokenFor(card.type).kickerKey)"
      :aria-current="i === currentIndex ? 'true' : 'false'"
      :title="t(tokenFor(card.type).kickerKey)"
      @click="$emit('goto', i)"
    >
      <SpotlightIcon :name="tokenFor(card.type).icon" class="w-4 h-4 flex-shrink-0" />
      <!-- Label always in the DOM inside a 0fr→1fr grid track: the pill
           WIDENS smoothly on activation instead of jolting the row. -->
      <span class="label-wrap" :class="{ 'label-open': i === currentIndex }" aria-hidden="true">
        <span
          class="label-inner font-mono text-[10px] uppercase tracking-[0.1em] font-medium whitespace-nowrap"
          :data-testid="i === currentIndex ? 'active-menu-label' : undefined"
        >
          {{ t(tokenFor(card.type).kickerKey) }}
        </span>
      </span>
    </button>

    <button
      type="button"
      class="menu-item menu-nav menu-nav-next absolute top-1/2 -translate-y-1/2 inline-flex items-center justify-center rounded-full border w-8 h-8 bg-white/[0.06] border-white/10 text-white/50 hover:text-white hover:bg-white/10 hover:border-white/20 transition-colors duration-200"
      :aria-label="t('spotlight.nextSlide')"
      data-testid="menu-next"
      @click="$emit('next')"
    >
      <ChevronRight class="w-4 h-4" aria-hidden="true" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import type { SpotlightCard } from '@/types/spotlight'
import { cardTokens, accentMenuPill, type CardToken, type SpotlightCardType } from './tokens'
import SpotlightIcon from './SpotlightIcon.vue'

const props = defineProps<{
  currentIndex: number
  cards: SpotlightCard[]
}>()

defineEmits<{
  (e: 'goto', index: number): void
  (e: 'prev'): void
  (e: 'next'): void
}>()

const { t } = useI18n()

// Fixed chevron offset from the row's CENTER: half of the widest possible
// menu width for this card set (the pill open on the longest label), plus
// a breathing gap. Constant within a mount → the arrows never move, while
// sitting right next to the cluster instead of at the frame edges
// (2026-06-12: «по краям — не удобно»). Char width ≈7.2px is JetBrains
// Mono 10px uppercase + 0.1em tracking; the +12 safety pad absorbs the
// estimate error.
const edgeOffset = computed<number>(() => {
  const n = props.cards.length
  if (n === 0) return 0
  const maxChars = Math.max(
    ...props.cards.map((c) => t(tokenFor(c.type).kickerKey).length),
  )
  const pillMax = 16 + 8 + Math.ceil(maxChars * 7.2) + 24 // icon + gap + text + px-3
  const rowMax = (n - 1) * (32 + 8) + pillMax // (n-1) circles with gaps + pill
  return Math.ceil(rowMax / 2) + 12 + 12 // half row + gap + safety pad
})

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
/* Pinned chevrons: this scoped block is UNLAYERED and would beat the
   `absolute` utility on the nav buttons (Tailwind v4 cascade), so the
   absolute positioning must be (re)declared here. */
.menu-item.menu-nav {
  position: absolute;
}
/* Fixed offset from center (--menu-edge = half the widest possible menu
   width + arrow, computed per card set) — near the cluster, never moving.
   max(0px, …) clamps to the row edge on narrow viewports. */
.menu-nav-prev {
  left: max(0px, calc(50% - var(--menu-edge)));
}
.menu-nav-next {
  right: max(0px, calc(50% - var(--menu-edge)));
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

/* Smooth pill expansion: the label sits in a grid track that animates
   0fr → 1fr (the canonical "transition to auto width" trick), so the
   button's width — and the whole row's layout — changes CONTINUOUSLY
   instead of jumping when the active card switches. */
.label-wrap {
  display: grid;
  grid-template-columns: 0fr;
  padding-left: 0;
  transition: grid-template-columns 0.3s ease, padding-left 0.3s ease;
}
.label-open {
  grid-template-columns: 1fr;
  /* The icon→label gap lives HERE (outside the collapsing track) so it
     animates with the expansion instead of leaving an 8px stub when
     collapsed (border-box clamps an inner padding to min 8px width). */
  padding-left: 8px;
}
.label-inner {
  overflow: hidden;
  min-width: 0;
  opacity: 0;
  transition: opacity 0.2s ease 0.05s;
}
.label-open .label-inner {
  opacity: 1;
}

@media (max-width: 480px) {
  .menu-item {
    height: 28px;
  }
  .menu-item.menu-nav {
    width: 28px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .label-wrap,
  .label-inner,
  .menu-item {
    transition: none;
  }
}
</style>
