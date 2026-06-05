<template>
  <div
    class="relative overflow-hidden"
    :class="[ratioClass, roundedClass]"
    :style="{ backgroundColor: 'var(--color-surface)' }"
  >
    <!-- Drift skeleton placeholder — its OWN element, so it never shares a
         `background` declaration with the container (the cascade bug). -->
    <div
      v-if="!loaded"
      class="absolute inset-0 sk-drift"
      :class="roundedClass"
      aria-hidden="true"
    />

    <img
      v-if="src"
      :src="src"
      :alt="alt"
      loading="lazy"
      class="absolute inset-0 w-full h-full object-cover transition-opacity duration-300"
      :class="loaded ? 'opacity-100' : 'opacity-0'"
      @load="loaded = true"
      @error="onError"
    />

    <!-- Optional scrims for legible overlay content on bright posters -->
    <div v-if="scrim" class="pointer-events-none absolute inset-x-0 top-0 h-16 bg-gradient-to-b from-black/55 to-transparent" />
    <div v-if="scrim" class="pointer-events-none absolute inset-x-0 bottom-0 h-20 bg-gradient-to-t from-black/75 to-transparent" />

    <slot />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { getImageFallbackUrl } from '@/composables/useImageProxy'

const props = withDefaults(
  defineProps<{
    src: string
    alt: string
    ratio?: '2/3' | '16/9'
    rounded?: 'none' | 'sm' | 'md' | 'lg' | 'xl'
    scrim?: boolean
  }>(),
  { ratio: '2/3', rounded: 'none', scrim: false }
)

const loaded = ref(false)

const ratioClass = computed(() => (props.ratio === '16/9' ? 'aspect-[16/9]' : 'aspect-[2/3]'))
const roundedClass = computed(() => {
  const map = { none: '', sm: 'rounded-sm', md: 'rounded-md', lg: 'rounded-lg', xl: 'rounded-xl' }
  return map[props.rounded]
})

function onError(e: Event) {
  const img = e.target as HTMLImageElement
  if (!img.dataset.fallback) {
    img.dataset.fallback = '1'
    img.src = getImageFallbackUrl(props.src)
  }
}
</script>
