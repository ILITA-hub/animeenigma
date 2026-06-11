<template>
  <div :class="[posterVariants({ glow }), widthClass]">
    <!-- DS shimmer placeholder until the image decodes (2026-06-11 lock:
         «скелетоны под загрузку любого картиночного элемента»). Skipped
         entirely for warm URLs (already loaded this session) — a carousel
         re-mount over an HTTP-cache hit must NOT replay the loading
         choreography (reads as "the image reloads every time"). -->
    <div v-if="!loaded" class="absolute inset-0 skeleton-shimmer" aria-hidden="true" />
    <img
      ref="imgRef"
      :src="src"
      :alt="alt"
      class="w-full h-full object-cover"
      :class="[imgClass, loaded ? 'opacity-100' : 'opacity-0 img-pending']"
      decoding="async"
      @load="onLoad"
      @error="onLoad"
    />
  </div>
</template>

<script setup lang="ts">
/**
 * Spotlight UI primitive (v4 lock, 2026-06-11) — proxied 2:3 poster with
 * the brand-triad glow shadows. Decorative by design: no link, no badges,
 * no context menu (that's PosterCard's catalog territory — see the v4 PS
 * decision in 2026-06-11 spec).
 *
 * Loading is EAGER (no loading="lazy"): the carousel mounts only the
 * active slide, so every mounted poster is on screen — lazy only delayed
 * cached decodes and made slide flips look like reloads.
 */
import { computed, onMounted, ref, watch } from 'vue'
import { cva } from 'class-variance-authority'
import { cardPosterUrl } from '@/composables/useImageProxy'
import { isImageWarm, markImageWarm } from '@/utils/preload-image'

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

const src = computed(() => cardPosterUrl(props.posterUrl, props.proxyWidth))

const imgRef = ref<HTMLImageElement | null>(null)
// Warm URLs (preloaded by HeroSpotlightBlock's prefetch, the reroll
// preloader, or a previous mount) render instantly — no shimmer, no fade.
const loaded = ref(isImageWarm(src.value))

function onLoad(): void {
  loaded.value = true
  markImageWarm(src.value)
}

// Belt-and-braces for images that complete before the listener attaches.
onMounted(() => {
  if (imgRef.value?.complete && imgRef.value.naturalWidth > 0) onLoad()
})

// A reactive src swap (RandomTail reroll) restarts the cycle — but only
// shows the shimmer again if the NEW url is cold.
watch(src, (v) => {
  loaded.value = isImageWarm(v)
})

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

<style scoped>
/* Fade-in runs only for cold loads (.img-pending applied while waiting);
   warm renders mount directly at opacity-100 with no transition. */
img {
  transition: opacity 0.3s ease;
}
</style>
