<template>
  <div ref="wrapperRef" class="relative">
    <!-- Trigger Button -->
    <button
      type="button"
      :class="triggerClasses"
      @click="toggle"
      @keydown="handleTriggerKeydown"
    >
      <span v-if="showEmoji && selectedGenre" class="text-base leading-none">{{ selectedEmoji }}</span>
      <span :class="selectedGenre ? 'text-white' : 'text-white/30'" class="truncate">
        {{ selectedLabel || placeholder }}
      </span>
      <!-- Clear button -->
      <button
        v-if="modelValue"
        class="ml-auto p-0.5 rounded hover:bg-white/10 text-white/40 hover:text-white/70 transition-colors"
        @click.stop="clear"
      >
        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
      <!-- Chevron -->
      <svg
        v-else
        class="w-4 h-4 text-white/50 transition-transform duration-200 flex-shrink-0"
        :class="{ 'rotate-180': isOpen }"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
      </svg>
    </button>

    <!-- Dropdown Popup -->
    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="opacity-0 scale-95 -translate-y-1"
      enter-to-class="opacity-100 scale-100 translate-y-0"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="opacity-100 scale-100 translate-y-0"
      leave-to-class="opacity-0 scale-95 -translate-y-1"
    >
      <div
        v-if="isOpen"
        class="absolute z-50 mt-1 w-64 bg-slate-900/95 backdrop-blur-xl border border-white/10 rounded-xl shadow-xl shadow-black/20"
      >
        <!-- Search Input -->
        <div class="p-2 border-b border-white/10">
          <input
            ref="searchRef"
            v-model="search"
            type="text"
            :placeholder="searchPlaceholder"
            class="w-full px-3 py-1.5 text-sm bg-white/5 border border-white/10 rounded-lg text-white placeholder-white/30 focus:outline-none focus:border-cyan-500/30"
            @keydown="handleSearchKeydown"
          />
        </div>
        <!-- Genre List -->
        <ul class="max-h-60 overflow-y-auto py-1" role="listbox">
          <!-- "All" option -->
          <li
            role="option"
            :aria-selected="!modelValue"
            :class="optionClasses('', -1)"
            @click="select('')"
            @mouseenter="focusedIndex = -1"
          >
            <span>{{ allLabel }}</span>
            <svg
              v-if="!modelValue"
              class="w-4 h-4 text-cyan-400 flex-shrink-0"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
            </svg>
          </li>
          <!-- Genre items -->
          <li
            v-for="(g, index) in filtered"
            :key="g.id"
            role="option"
            :aria-selected="g.id === modelValue"
            :class="optionClasses(g.id, index)"
            @click="select(g.id)"
            @mouseenter="focusedIndex = index"
          >
            <span class="flex items-center gap-2 min-w-0">
              <span v-if="showEmoji" class="text-base leading-none flex-shrink-0">{{ getGenreEmoji(g.name) }}</span>
              <span class="truncate">{{ g.name_ru || g.name }}</span>
            </span>
            <span class="flex items-center gap-2 flex-shrink-0">
              <span v-if="g.count" class="text-xs text-white/30">{{ g.count }}</span>
              <svg
                v-if="g.id === modelValue"
                class="w-4 h-4 text-cyan-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
              </svg>
            </span>
          </li>
          <!-- Empty state -->
          <li v-if="filtered.length === 0 && search" class="px-4 py-3 text-sm text-white/30 text-center">
            No genres found
          </li>
        </ul>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { getGenreEmoji } from '@/utils/genre-emoji'

export interface GenreOption {
  id: string
  name: string
  name_ru?: string
  count?: number
}

interface Props {
  modelValue: string
  genres: GenreOption[]
  placeholder?: string
  allLabel?: string
  searchPlaceholder?: string
  size?: 'sm' | 'md'
  showEmoji?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  placeholder: 'Genre',
  allLabel: 'All genres',
  searchPlaceholder: 'Search genres...',
  size: 'md',
  showEmoji: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'change': [value: string]
}>()

const isOpen = ref(false)
const search = ref('')
const focusedIndex = ref(-1)
const wrapperRef = ref<HTMLElement | null>(null)
const searchRef = ref<HTMLInputElement | null>(null)

const selectedGenre = computed(() =>
  props.genres.find(g => g.id === props.modelValue)
)

const selectedLabel = computed(() => {
  const g = selectedGenre.value
  return g ? (g.name_ru || g.name) : ''
})

const selectedEmoji = computed(() => {
  const g = selectedGenre.value
  return g ? getGenreEmoji(g.name) : ''
})

const filtered = computed(() => {
  if (!search.value) return props.genres
  const q = search.value.toLowerCase()
  return props.genres.filter(g =>
    g.name.toLowerCase().includes(q) ||
    (g.name_ru && g.name_ru.toLowerCase().includes(q))
  )
})

const triggerClasses = computed(() => {
  const base = 'w-full flex items-center bg-white/5 border text-white transition-all duration-200 focus:outline-none cursor-pointer'
  const sizes = {
    sm: 'px-3 py-2 text-sm rounded-lg gap-2',
    md: 'px-4 py-3 text-base rounded-xl gap-2',
  }
  const states = isOpen.value
    ? 'border-cyan-400 ring-2 ring-cyan-400/20'
    : 'border-white/10 hover:border-white/20'
  return [base, sizes[props.size], states].join(' ')
})

const optionClasses = (id: string, index: number) => {
  const base = 'flex items-center justify-between px-4 py-2.5 cursor-pointer transition-colors text-sm'
  const isSelected = id === props.modelValue
  const isFocused = index === focusedIndex.value

  if (isSelected) return `${base} bg-cyan-500/20 text-cyan-300`
  if (isFocused) return `${base} bg-white/10 text-white`
  return `${base} text-white/70 hover:bg-white/5 hover:text-white`
}

const toggle = () => {
  isOpen.value = !isOpen.value
}

const select = (id: string) => {
  emit('update:modelValue', id)
  emit('change', id)
  isOpen.value = false
  search.value = ''
}

const clear = () => {
  emit('update:modelValue', '')
  emit('change', '')
  search.value = ''
}

const handleTriggerKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    toggle()
  } else if (e.key === 'Escape') {
    isOpen.value = false
  }
}

const handleSearchKeydown = (e: KeyboardEvent) => {
  const items = filtered.value
  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      focusedIndex.value = Math.min(focusedIndex.value + 1, items.length - 1)
      break
    case 'ArrowUp':
      e.preventDefault()
      focusedIndex.value = Math.max(focusedIndex.value - 1, -1)
      break
    case 'Enter':
      e.preventDefault()
      if (focusedIndex.value === -1) {
        select('')
      } else if (focusedIndex.value < items.length) {
        select(items[focusedIndex.value].id)
      }
      break
    case 'Escape':
      isOpen.value = false
      break
  }
}

const handleClickOutside = (event: MouseEvent) => {
  if (wrapperRef.value && !wrapperRef.value.contains(event.target as Node)) {
    isOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})

watch(isOpen, async (value) => {
  if (value) {
    search.value = ''
    focusedIndex.value = -1
    await nextTick()
    searchRef.value?.focus()
  }
})
</script>
