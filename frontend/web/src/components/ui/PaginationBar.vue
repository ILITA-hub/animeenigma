<template>
  <nav v-if="totalPages > 1" class="flex items-center justify-center gap-1 mt-8" aria-label="Pagination">
    <!-- Previous -->
    <button
      :disabled="currentPage <= 1"
      class="w-9 h-9 flex items-center justify-center rounded-lg text-sm text-white/70 transition-colors"
      :class="{ 'opacity-30 cursor-not-allowed': currentPage <= 1 }"
      @click="$emit('update:currentPage', currentPage - 1)"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
      </svg>
    </button>

    <!-- Page buttons -->
    <template v-for="page in visiblePages" :key="page">
      <span v-if="page === '...'" class="px-2 text-white/30">...</span>
      <button
        v-else
        class="w-9 h-9 flex items-center justify-center rounded-lg text-sm text-white/70 transition-colors"
        :class="page === currentPage ? 'bg-pink-500/80 text-white' : 'hover:bg-white/10'"
        @click="$emit('update:currentPage', page as number)"
      >
        {{ page }}
      </button>
    </template>

    <!-- Next -->
    <button
      :disabled="currentPage >= totalPages"
      class="w-9 h-9 flex items-center justify-center rounded-lg text-sm text-white/70 transition-colors"
      :class="{ 'opacity-30 cursor-not-allowed': currentPage >= totalPages }"
      @click="$emit('update:currentPage', currentPage + 1)"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
      </svg>
    </button>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  currentPage: number
  totalPages: number
}>()

defineEmits<{
  'update:currentPage': [page: number]
}>()

const visiblePages = computed(() => {
  const total = props.totalPages
  const current = props.currentPage
  const pages: (number | string)[] = []

  if (total <= 7) {
    for (let i = 1; i <= total; i++) pages.push(i)
    return pages
  }

  // Always show first page
  pages.push(1)

  if (current > 3) {
    pages.push('...')
  }

  // Window around current page
  const start = Math.max(2, current - 1)
  const end = Math.min(total - 1, current + 1)
  for (let i = start; i <= end; i++) {
    pages.push(i)
  }

  if (current < total - 2) {
    pages.push('...')
  }

  // Always show last page
  pages.push(total)

  return pages
})
</script>

