<template>
  <article class="relative w-full h-full overflow-hidden">
    <!--
      Workstream hero-spotlight — DS alignment 2026-06-10 (spec:
      2026-06-10-spotlight-ds-alignment-design.md, block C, user-approved).

      Shared anatomy frame for all 9 spotlight cards:
        · kicker row (SpotlightIcon + uppercase label) in the card's
          brand-triad accent — single place, single style;
        · default slot = the card's body;
        · cta slot = action row PINNED to the bottom-left corner;
        · padding on the DS scale (p-4 md:p-6 lg:p-8);
        · background = SpotlightBackdrop by default, replaceable via the
          #background slot (FeaturedCard's sharp poster + scrim), extendable
          via #background-extra (RandomTail's violet tint).

      SINGLE-ROOT <article> (comment kept inside the root so the component
      stays a non-fragment), NO top-level v-if — required by the parent
      <Transition mode="out-in"> in HeroSpotlightBlock.
    -->
    <slot name="background">
      <SpotlightBackdrop
        v-if="backdrop !== 'none'"
        :variant="backdrop"
        :poster-url="posterUrl"
        :accent="accent"
      />
    </slot>
    <slot name="background-extra" />

    <div
      class="relative z-10 h-full flex flex-col gap-3 p-4 md:p-6 lg:p-8"
      :class="[justify === 'end' ? 'justify-end' : '', contentClass]"
    >
      <p
        v-if="kicker"
        class="inline-flex items-center gap-2 text-[11px] uppercase tracking-[0.14em] font-semibold font-mono"
        :class="accentText[accent]"
      >
        <slot name="kicker-lead">
          <SpotlightIcon v-if="icon" :name="icon" class="w-4 h-4" />
        </slot>
        {{ kicker }}
        <slot name="kicker-extra" />
      </p>

      <slot />

      <div
        v-if="$slots.cta"
        class="flex flex-wrap items-center gap-3"
        :class="justify === 'end' ? 'pt-1' : 'mt-auto pt-2'"
      >
        <slot name="cta" />
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import SpotlightBackdrop from './SpotlightBackdrop.vue'
import SpotlightIcon from './SpotlightIcon.vue'
import { accentText, type SpotlightAccent, type SpotlightIconName } from './tokens'

interface Props {
  /** Brand-triad accent (A-1): colors the kicker + default mesh. */
  accent: SpotlightAccent
  /** Kicker icon (SpotlightIcon name). Omit to render label-only. */
  icon?: SpotlightIconName
  /** Kicker label (already-translated string). Omit to hide the row. */
  kicker?: string
  /** Backdrop mode; 'none' expects the #background slot to provide one. */
  backdrop?: 'poster-blur' | 'gradient-mesh' | 'none'
  /** Poster URL for the poster-blur backdrop. */
  posterUrl?: string
  /** 'end' bottom-anchors the whole content stack (hero cards). */
  justify?: 'start' | 'end'
  /** Extra classes on the content column (e.g. max-w constraints). */
  contentClass?: string
}

withDefaults(defineProps<Props>(), {
  icon: undefined,
  kicker: '',
  backdrop: 'gradient-mesh',
  posterUrl: '',
  justify: 'start',
  contentClass: '',
})
</script>
