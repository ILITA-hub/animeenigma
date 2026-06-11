<template>
  <div :class="[posterVariants({ glow }), widthClass]">
    <img
      :src="cardPosterUrl(posterUrl, proxyWidth)"
      :alt="alt"
      class="w-full h-full object-cover"
      :class="imgClass"
      loading="lazy"
      decoding="async"
    />
  </div>
</template>

<script setup lang="ts">
/**
 * Spotlight UI primitive (v4 lock, 2026-06-11) — proxied lazy 2:3 poster
 * with the brand-triad glow shadows. Decorative by design: no link, no
 * badges, no context menu (that's PosterCard's catalog territory —
 * see the v4 PS decision in 2026-06-11 spec).
 */
import { cva } from 'class-variance-authority'
import { cardPosterUrl } from '@/composables/useImageProxy'

withDefaults(
  defineProps<{
    posterUrl?: string
    alt?: string
    /** Tailwind width class, e.g. 'w-24 md:w-32 lg:w-40'. */
    widthClass?: string
    /** Image-proxy width bucket (CSS px × 2 for retina). */
    proxyWidth?: number
    /** Brand-triad glow shadow. */
    glow?: 'none' | 'cyan' | 'pink' | 'violet'
    /** Extra classes on the inner <img> (e.g. grayscale effects). */
    imgClass?: string
  }>(),
  { posterUrl: '', alt: '', widthClass: 'w-24', proxyWidth: 256, glow: 'none', imgClass: '' },
)

const posterVariants = cva(
  'relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] flex-shrink-0',
  {
    variants: {
      glow: {
        none: '',
        cyan: 'shadow-2xl shadow-cyan-500/20',
        pink: 'shadow-2xl shadow-pink-500/30',
        violet: 'shadow-2xl shadow-brand-violet/20',
      },
    },
    defaultVariants: { glow: 'none' },
  },
)
</script>
