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
    <img
      v-if="variant === 'poster-blur' && posterUrl"
      :src="blurSrc"
      alt=""
      aria-hidden="true"
      loading="lazy"
      decoding="async"
      class="absolute inset-0 w-full h-full object-cover scale-110"
      :style="POSTER_BLUR_STYLE"
    />
    <div
      v-else
      aria-hidden="true"
      class="absolute inset-0"
      :class="meshClass"
      data-testid="spotlight-backdrop-mesh"
    />

    <!-- Shared right-edge vignette so foreground text remains AA-readable
         regardless of variant. -->
    <div
      aria-hidden="true"
      class="absolute inset-0 bg-gradient-to-r from-transparent via-black/30 to-black/60"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { cardPosterUrl } from '@/composables/useImageProxy'
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
// 1.2 saturation, 0.4 alpha as specified in the Phase 01 plan).
const POSTER_BLUR_STYLE = 'filter: blur(40px) saturate(1.2); opacity: 0.4;'

// The backdrop is blurred 40px — full-res source is pure waste, so route
// proxyable posters through the resizing image-proxy at the smallest bucket.
const blurSrc = computed(() => cardPosterUrl(props.posterUrl, 128))

// Per-accent radial-gradient mesh. Tailwind 4 evaluates these utility
// strings at build time, so we use a static lookup table rather than
// dynamic class composition (which would need a safelist).
const MESH_CLASSES: Record<SpotlightAccent, string> = {
  cyan:   'bg-[radial-gradient(at_25%_30%,rgba(34,211,238,0.30),transparent_55%),radial-gradient(at_75%_70%,rgba(14,165,233,0.20),transparent_60%)]',
  purple: 'bg-[radial-gradient(at_25%_30%,rgba(168,85,247,0.30),transparent_55%),radial-gradient(at_75%_70%,rgba(217,70,239,0.20),transparent_60%)]',
  sky:    'bg-[radial-gradient(at_25%_30%,rgba(56,189,248,0.30),transparent_55%),radial-gradient(at_75%_70%,rgba(2,132,199,0.20),transparent_60%)]',
  amber:  'bg-[radial-gradient(at_25%_30%,rgba(251,191,36,0.30),transparent_55%),radial-gradient(at_75%_70%,rgba(245,158,11,0.20),transparent_60%)]',
  teal:   'bg-[radial-gradient(at_25%_30%,rgba(45,212,191,0.30),transparent_55%),radial-gradient(at_75%_70%,rgba(20,184,166,0.20),transparent_60%)]',
  green:  'bg-[radial-gradient(at_25%_30%,rgba(74,222,128,0.30),transparent_55%),radial-gradient(at_75%_70%,rgba(34,197,94,0.20),transparent_60%)]',
}

const meshClass = computed<string>(() => MESH_CLASSES[props.accent])
</script>
