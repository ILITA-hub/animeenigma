<template>
  <div
    class="relative overflow-hidden"
    :class="[ratioClass, roundedClass]"
    :style="{ backgroundColor: 'var(--color-surface)' }"
  >
    <img
      v-if="src"
      :src="resolvedSrc"
      :alt="alt"
      loading="lazy"
      class="absolute inset-0 w-full h-full object-cover"
      @load="loaded = true"
      @error="onError"
    />

    <!-- Drift skeleton — translucent glass OVERLAY above the img, so the
         browser's native progressive (top-down) poster render stays visible
         underneath as loading feedback; fades out on @load. Its OWN element,
         so it never shares a `background` declaration with the container
         (the cascade bug). -->
    <Transition name="sk-fade">
      <div
        v-if="!loaded"
        class="absolute inset-0 sk-drift pointer-events-none"
        :class="roundedClass"
        aria-hidden="true"
      />
    </Transition>

    <!-- Optional scrims for legible overlay content on bright posters -->
    <div v-if="scrim" class="pointer-events-none absolute inset-x-0 top-0 h-16 bg-gradient-to-b from-black/55 to-transparent" />
    <div v-if="scrim" class="pointer-events-none absolute inset-x-0 bottom-0 h-20 bg-gradient-to-t from-black/75 to-transparent" />

    <slot />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { cardPosterUrl, getImageFallbackUrl } from '@/composables/useImageProxy'

const props = withDefaults(
  defineProps<{
    src: string
    alt: string
    ratio?: '2/3' | '16/9'
    rounded?: 'none' | 'sm' | 'md' | 'lg' | 'xl'
    scrim?: boolean
    /** When set, proxyable posters are served resized via the backend
     *  image-proxy (`w` param). Pass max CSS width × 2 for retina. */
    proxyWidth?: number
  }>(),
  { ratio: '2/3', rounded: 'none', scrim: false, proxyWidth: undefined }
)

const loaded = ref(false)

// Fallback chain when serving a resized proxy URL: proxied → original → done.
// Without proxyWidth the legacy chain applies: original → full-size proxy.
const fallbackStage = ref(0)

const resolvedSrc = computed(() => {
  if (!props.proxyWidth) return props.src
  const proxied = cardPosterUrl(props.src, props.proxyWidth)
  if (proxied === props.src) return props.src
  return fallbackStage.value === 0 ? proxied : props.src
})

const ratioClass = computed(() => (props.ratio === '16/9' ? 'aspect-[16/9]' : 'aspect-[2/3]'))
const roundedClass = computed(() => {
  const map = { none: '', sm: 'rounded-sm', md: 'rounded-md', lg: 'rounded-lg', xl: 'rounded-xl' }
  return map[props.rounded]
})

function onError(e: Event) {
  // Stage 1: resized proxy URL failed → retry with the original upstream URL
  if (props.proxyWidth && fallbackStage.value === 0 && resolvedSrc.value !== props.src) {
    fallbackStage.value = 1
    return
  }
  // Stage 2: original URL failed → full-size proxy (has server-side MAL fallback)
  const img = e.target as HTMLImageElement
  if (!img.dataset.fallback) {
    img.dataset.fallback = '1'
    img.src = getImageFallbackUrl(props.src)
  }
}
</script>
