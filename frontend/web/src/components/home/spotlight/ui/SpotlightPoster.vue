<template>
  <div :class="[posterVariants({ glow }), widthClass]">
    <!-- DS shimmer placeholder until the image decodes (2026-06-11 lock:
         «скелетоны под загрузку любого картиночного элемента») — the img
         fades in over it so a slow proxy never shows a raw empty box. -->
    <div v-if="!loaded" class="absolute inset-0 skeleton-shimmer" aria-hidden="true" />
    <img
      ref="imgRef"
      :src="cardPosterUrl(posterUrl, proxyWidth)"
      :alt="alt"
      class="w-full h-full object-cover transition-opacity duration-300"
      :class="[imgClass, loaded ? 'opacity-100' : 'opacity-0']"
      loading="lazy"
      decoding="async"
      @load="loaded = true"
      @error="loaded = true"
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
import { onMounted, ref, watch } from 'vue'
import { cva } from 'class-variance-authority'
import { cardPosterUrl } from '@/composables/useImageProxy'

const props = withDefaults(
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

const imgRef = ref<HTMLImageElement | null>(null)
const loaded = ref(false)

// Cache-hit images may be `complete` before the @load listener attaches
// (e.g. prefetched by HeroSpotlightBlock) — check once on mount so the
// shimmer doesn't flash over an already-decoded poster.
onMounted(() => {
  if (imgRef.value?.complete && imgRef.value.naturalWidth > 0) loaded.value = true
})

// A reactive src swap (RandomTail reroll) restarts the load cycle.
watch(
  () => props.posterUrl,
  () => {
    loaded.value = false
  },
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
