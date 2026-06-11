<template>
  <!--
    Workstream hero-spotlight — v1.1-polish Phase 01 (HSB-V11-CC-01).

    Two-variant decorative backdrop that sits behind a spotlight card's
    foreground content. ALL elements are aria-hidden + pointer-events-none
    so they're invisible to assistive tech and inert to the cursor.

    Variants:
      poster-blur    — Renders a blurred + tinted <img> from `posterUrl`.
                       Reuses the card's existing poster URL — browser
                       cache hit. (Threat T-V11-01 in the plan threat
                       register.)
      gradient-mesh  — Renders an `accent`-tinted radial-gradient mesh.
                       No HTTP request.

    Both variants are overlaid with a shared right-side vignette so the
    foreground text (which sits on the left/center) stays readable.

    If `variant === 'poster-blur'` but `posterUrl` is missing/empty, we
    fall back to the gradient-mesh of the same accent so the card never
    renders against a fully transparent background.
  -->
  <div class="absolute inset-0 overflow-hidden pointer-events-none">
    <!-- DS shimmer while a COLD blur poster loads (2026-06-11 feedback:
         «скелетона нет на фоне») — warm URLs (session registry) skip it so
         carousel re-mounts never replay the loading state. -->
    <div
      v-if="variant === 'poster-blur' && posterUrl && !bgLoaded"
      aria-hidden="true"
      class="absolute inset-0 skeleton-shimmer opacity-50"
      data-testid="backdrop-skeleton"
    />
    <!-- Keyed wrapper + non-out-in Transition = CROSSFADE when the poster
         URL changes in place (RandomTail reroll, 2026-06-11): old blur
         fades out while the new (preloaded by the card) fades in — no
         re-fetch flash. The 0.4 opacity lives on the wrapper so the
         transition classes can animate it (an inline opacity on the <img>
         would beat the enter/leave classes). -->
    <Transition name="backdrop-x">
      <div
        v-if="variant === 'poster-blur' && posterUrl"
        :key="blurSrc"
        aria-hidden="true"
        class="absolute inset-0 opacity-40"
      >
        <img
          :src="blurSrc"
          alt=""
          aria-hidden="true"
          decoding="async"
          class="absolute inset-0 w-full h-full object-cover scale-110"
          :style="POSTER_BLUR_STYLE"
          @load="onBgLoad"
          @error="onBgLoad"
        />
      </div>
      <div
        v-else
        aria-hidden="true"
        class="absolute inset-0"
        :class="meshClass"
        data-testid="spotlight-backdrop-mesh"
      />
    </Transition>

    <!-- Shared right-edge vignette so foreground text remains AA-readable
         regardless of variant. -->
    <div
      aria-hidden="true"
      class="absolute inset-0 bg-gradient-to-r from-transparent via-black/30 to-black/60"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { cardPosterUrl } from '@/composables/useImageProxy'
import { isImageWarm, markImageWarm } from '@/utils/preload-image'
import type { SpotlightAccent } from './tokens'

interface Props {
  variant: 'poster-blur' | 'gradient-mesh'
  posterUrl?: string
  accent?: SpotlightAccent
}
const props = withDefaults(defineProps<Props>(), {
  posterUrl: '',
  accent: 'cyan',
})

// Inline style for the blurred poster — kept here so the Tailwind class
// string stays cacheable and the blur values are explicit (40px blur,
// 1.2 saturation as specified in the Phase 01 plan). The 0.4 alpha moved
// to the keyed wrapper (opacity-40) so the reroll crossfade can animate it.
const POSTER_BLUR_STYLE = 'filter: blur(40px) saturate(1.2);'

// The backdrop is blurred 40px — full-res source is pure waste, so route
// proxyable posters through the resizing image-proxy at the smallest bucket.
const blurSrc = computed(() => cardPosterUrl(props.posterUrl, 128))

// Skeleton gate: cold URLs shimmer until @load; warm URLs (already loaded
// this session — slide prefetch, reroll preload, prior mount) never do.
const bgLoaded = ref(isImageWarm(blurSrc.value))
function onBgLoad(): void {
  bgLoaded.value = true
  markImageWarm(blurSrc.value)
}
watch(blurSrc, (v) => {
  bgLoaded.value = isImageWarm(v)
})

// Per-accent radial-gradient mesh — brand triad only (DS alignment A-1,
// 2026-06-10): the rgba stops are the brand-cyan-400/500, brand-pink-400/500
// and brand-violet/violet-500 primitives. Tailwind 4 evaluates these utility
// strings at build time, so we use a static lookup table rather than
// dynamic class composition (which would need a safelist).
const MESH_CLASSES: Record<SpotlightAccent, string> = {
  cyan:   'bg-[radial-gradient(at_25%_30%,rgba(0,212,255,0.25),transparent_55%),radial-gradient(at_75%_70%,rgba(0,184,230,0.18),transparent_60%)]',
  pink:   'bg-[radial-gradient(at_25%_30%,rgba(255,77,141,0.25),transparent_55%),radial-gradient(at_75%_70%,rgba(255,45,124,0.18),transparent_60%)]',
  violet: 'bg-[radial-gradient(at_25%_30%,rgba(167,139,250,0.28),transparent_55%),radial-gradient(at_75%_70%,rgba(139,92,246,0.18),transparent_60%)]',
}

const meshClass = computed<string>(() => MESH_CLASSES[props.accent])
</script>

<style scoped>
/* Reroll crossfade: both keyed wrappers are absolute inset-0, so the old
   and new backdrop overlap while one fades out and the other fades in. */
.backdrop-x-enter-active,
.backdrop-x-leave-active {
  transition: opacity 0.6s ease;
}
.backdrop-x-enter-from,
.backdrop-x-leave-to {
  opacity: 0;
}
</style>
