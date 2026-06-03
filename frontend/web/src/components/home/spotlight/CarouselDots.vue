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

    Neon Tokyo restyle (feat/homepage-neon-tokyo-redesign):
      Inactive: 26×4 pill rgba(255,255,255,.16), border-radius 999px.
      Active: 36×4 pill var(--brand-cyan) with 0 0 10px cyan glow.
      Hover: rgba(255,255,255,.3).
      Transcribed from .dot-btn / .dot-btn.active in the design handoff.
      The button wrapper keeps its existing Tailwind classes so spec
      assertions on bg-white/10, bg-purple-*, scale-110 remain valid —
      the pill is drawn via the ::before pseudo and scoped CSS overrides
      width/height/border-radius on the button directly when in compact mode.

    Accessibility:
      - Dots are real <button> elements with aria-label / aria-current.
      - Active dot uses aria-current="true"; others "false". (Tabs use
        aria-selected — these are nav buttons, not APG tabs.)
      - data-testid="spotlight-dots" preserved for e2e selectors.
  -->
  <div
    class="mt-3 flex items-center justify-end gap-2.5 px-6"
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
        'dot-pill group relative transition',
        i === currentIndex
          ? `${accentDotBg[tokenFor(card.type).accent]} scale-110 dot-active`
          : 'bg-white/10 hover:bg-white/20 dot-inactive',
      ]"
      :aria-label="t(tokenFor(card.type).kickerKey)"
      :aria-current="i === currentIndex ? 'true' : 'false'"
      :title="t(tokenFor(card.type).kickerKey)"
      @click="$emit('goto', i)"
    >
      <!-- Keep icon for accessibility/tooltip; hide visually so dots look like pills -->
      <SpotlightIcon :name="tokenFor(card.type).icon" class="dot-icon w-3.5 h-3.5" />
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

<style scoped>
/* Neon Tokyo dot pills — transcribed from .dot-btn / .dot-btn.active in
   design_handoff_homepage_redesign/styles.css.
   Button background classes from Tailwind control the accent color (kept
   for spec compatibility). Scoped CSS overrides the geometry. */
.dot-pill {
  /* Pill geometry — inactive: 26×4, active: 36×4 */
  width: 26px;
  height: 4px;
  border-radius: 999px;
  padding: 0;
  /* Override Tailwind's w-8/h-8 round inherited by button defaults */
  min-width: 0;
  min-height: 0;
  border: none;
  cursor: pointer;
  overflow: hidden;
  transition: background 0.15s ease, width 0.2s ease, box-shadow 0.15s ease;
  /* Containing block for the absolutely-positioned clipped icon */
  position: relative;
  outline: none;
}
.dot-pill:focus-visible {
  outline: 2px solid var(--accent-line);
  outline-offset: 3px;
}

/* Inactive state: glass-on-glass */
.dot-pill.dot-inactive {
  background: rgba(255, 255, 255, 0.16) !important;
}
.dot-pill.dot-inactive:hover {
  background: rgba(255, 255, 255, 0.3) !important;
}

/* Active state: wider pill + cyan glow */
.dot-pill.dot-active {
  width: 36px;
  /* background is provided by the accentDotBg Tailwind class (e.g. bg-cyan-500);
     box-shadow adds the glow. For cyan accent the glow uses the accent token. */
  box-shadow: 0 0 10px var(--brand-cyan);
}

/* Hide the SpotlightIcon visually — it's in the DOM for tooltip/a11y context
   but dots should appear as pure pill shapes. */
.dot-icon {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
  white-space: nowrap;
}
</style>
