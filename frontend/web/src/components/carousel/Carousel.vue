<template>
  <div class="relative group">
    <!-- Header -->
    <div v-if="title || $slots.header" class="flex items-center justify-between mb-4 px-4 lg:px-0">
      <slot name="header">
        <h2 class="text-xl md:text-2xl font-bold text-white">{{ title }}</h2>
      </slot>
      <router-link
        v-if="seeAllLink"
        :to="seeAllLink"
        class="text-sm text-cyan-400 hover:text-cyan-300 transition-colors flex items-center gap-1"
      >
        {{ $t('home.seeAll') }}
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
        </svg>
      </router-link>
    </div>

    <!-- Carousel Container -->
    <div class="relative">
      <!-- Left Arrow (Desktop only) -->
      <button
        v-if="canScrollLeft && !prefersReducedMotion"
        class="hidden md:flex absolute left-0 top-1/2 -translate-y-1/2 z-10 w-12 h-12 items-center justify-center bg-base/80 backdrop-blur-sm rounded-full border border-white/10 text-white/70 hover:text-white hover:bg-base/90 transition-all opacity-0 group-hover:opacity-100 -ml-6"
        @click="scrollLeft"
        :aria-label="$t('common.back')"
      >
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
        </svg>
      </button>

      <!-- Scrollable Container -->
      <div
        ref="scrollContainer"
        class="flex gap-4 overflow-x-auto scrollbar-hide scroll-smooth px-4 lg:px-0"
        :class="{ 'snap-x-mandatory': snapScroll }"
        @scroll="handleScroll"
      >
        <div
          v-for="(item, index) in items"
          :key="getItemKey(item, index)"
          class="flex-shrink-0"
          :class="[itemClass, { 'snap-start': snapScroll }]"
          :style="itemStyle"
        >
          <slot :item="item" :index="index" />
        </div>

        <!-- Peek space for next item -->
        <div v-if="peekNext" class="flex-shrink-0 w-4 lg:w-0" />
      </div>

      <!-- Right Arrow (Desktop only) -->
      <button
        v-if="canScrollRight && !prefersReducedMotion"
        class="hidden md:flex absolute right-0 top-1/2 -translate-y-1/2 z-10 w-12 h-12 items-center justify-center bg-base/80 backdrop-blur-sm rounded-full border border-white/10 text-white/70 hover:text-white hover:bg-base/90 transition-all opacity-0 group-hover:opacity-100 -mr-6"
        @click="scrollRight"
        :aria-label="$t('common.next')"
      >
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
        </svg>
      </button>
    </div>

    <!-- Pagination Dots (optional) -->
    <div
      v-if="showDots && totalPages > 1"
      class="flex justify-center gap-2 mt-4"
    >
      <button
        v-for="page in totalPages"
        :key="page"
        class="w-2 h-2 rounded-full transition-all"
        :class="currentPage === page ? 'bg-cyan-400 w-6' : 'bg-white/30 hover:bg-white/50'"
        @click="goToPage(page)"
        :aria-label="`Page ${page}`"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useMediaQuery } from '@vueuse/core'

interface Props {
  items: unknown[]
  title?: string
  seeAllLink?: string
  itemKey?: string
  itemClass?: string
  itemWidth?: {
    mobile?: number
    tablet?: number
    desktop?: number
    large?: number
  }
  snapScroll?: boolean
  peekNext?: boolean
  showDots?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  itemClass: '',
  itemWidth: () => ({
    mobile: 128,
    tablet: 160,
    desktop: 192,
    large: 224,
  }),
  snapScroll: true,
  peekNext: true,
  showDots: false,
})

const scrollContainer = ref<HTMLElement | null>(null)
const scrollPosition = ref(0)
const containerWidth = ref(0)
const scrollWidth = ref(0)

const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
const isTablet = useMediaQuery('(min-width: 768px)')
const isDesktop = useMediaQuery('(min-width: 1024px)')
const isLarge = useMediaQuery('(min-width: 1280px)')

const currentItemWidth = computed(() => {
  if (isLarge.value) return props.itemWidth.large ?? 224
  if (isDesktop.value) return props.itemWidth.desktop ?? 192
  if (isTablet.value) return props.itemWidth.tablet ?? 160
  return props.itemWidth.mobile ?? 128
})

const itemStyle = computed(() => ({
  width: `${currentItemWidth.value}px`,
}))

const canScrollLeft = computed(() => scrollPosition.value > 10)
const canScrollRight = computed(() => {
  return scrollPosition.value < scrollWidth.value - containerWidth.value - 10
})

const totalPages = computed(() => {
  if (!containerWidth.value) return 1
  const itemsPerPage = Math.floor(containerWidth.value / (currentItemWidth.value + 16))
  return Math.ceil(props.items.length / Math.max(itemsPerPage, 1))
})

const currentPage = computed(() => {
  if (!containerWidth.value) return 1
  return Math.floor(scrollPosition.value / containerWidth.value) + 1
})

const getItemKey = (item: unknown, index: number): string | number => {
  if (props.itemKey && typeof item === 'object' && item !== null) {
    return (item as Record<string, unknown>)[props.itemKey] as string | number
  }
  return index
}

const handleScroll = () => {
  if (scrollContainer.value) {
    scrollPosition.value = scrollContainer.value.scrollLeft
  }
}

const scrollLeft = () => {
  if (scrollContainer.value) {
    const scrollAmount = containerWidth.value * 0.8
    scrollContainer.value.scrollBy({ left: -scrollAmount, behavior: 'smooth' })
  }
}

const scrollRight = () => {
  if (scrollContainer.value) {
    const scrollAmount = containerWidth.value * 0.8
    scrollContainer.value.scrollBy({ left: scrollAmount, behavior: 'smooth' })
  }
}

const goToPage = (page: number) => {
  if (scrollContainer.value) {
    const targetScroll = (page - 1) * containerWidth.value
    scrollContainer.value.scrollTo({ left: targetScroll, behavior: 'smooth' })
  }
}

const updateDimensions = () => {
  if (scrollContainer.value) {
    containerWidth.value = scrollContainer.value.clientWidth
    scrollWidth.value = scrollContainer.value.scrollWidth
  }
}

onMounted(() => {
  updateDimensions()
  window.addEventListener('resize', updateDimensions)
})

onUnmounted(() => {
  window.removeEventListener('resize', updateDimensions)
})
</script>
