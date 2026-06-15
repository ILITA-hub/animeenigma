<template>
  <div class="relative" ref="containerRef">
    <div class="relative">
      <Input
        ref="inputRef"
        v-model="query"
        :placeholder="$t('anidle.search_placeholder')"
        :disabled="disabled"
        class="w-full"
        autocomplete="off"
        role="combobox"
        :aria-expanded="isOpen"
        aria-haspopup="listbox"
        aria-autocomplete="list"
        @keydown="onKeydown"
        @focus="onFocus"
        @blur="onBlur"
      />
      <!-- Loading spinner inside the field -->
      <span
        v-if="isSearching"
        class="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground pointer-events-none"
      >
        <Spinner size="sm" />
      </span>
    </div>

    <!-- Dropdown listbox -->
    <Transition
      enter-active-class="transition-all duration-150 ease-out"
      enter-from-class="opacity-0 translate-y-1"
      enter-to-class="opacity-100 translate-y-0"
      leave-active-class="transition-all duration-100 ease-in"
      leave-from-class="opacity-100 translate-y-0"
      leave-to-class="opacity-0 translate-y-1"
    >
      <div
        v-if="isOpen"
        class="absolute top-full mt-2 left-0 right-0 z-50 bg-background/95 backdrop-blur-xl border border-white/10 rounded-xl shadow-2xl overflow-hidden"
        role="listbox"
        :aria-label="$t('anidle.search_placeholder')"
      >
        <!-- Loading state -->
        <div
          v-if="isSearching"
          class="px-4 py-3 text-sm text-muted-foreground"
        >
          {{ $t('anidle.search_loading') }}
        </div>

        <!-- No results -->
        <div
          v-else-if="results.length === 0 && query.length >= 2"
          class="px-4 py-3 text-sm text-muted-foreground"
        >
          {{ $t('anidle.search_no_results') }}
        </div>

        <!-- Results list -->
        <ul v-else class="max-h-72 overflow-y-auto py-1">
          <li
            v-for="(item, index) in results"
            :key="item.id"
            role="option"
            :aria-selected="index === activeIndex"
            :class="[
              'flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors',
              index === activeIndex ? 'bg-white/10' : 'hover:bg-white/10',
            ]"
            @mousedown.prevent="selectItem(item)"
            @mouseover="activeIndex = index"
          >
            <img
              :src="posterSrc(item.poster_url)"
              :alt="item.name_ru"
              class="w-12 h-16 rounded-lg object-cover flex-shrink-0 bg-white/10"
            />
            <div class="min-w-0 flex-1">
              <p class="font-medium text-white truncate">{{ item.name_ru }}</p>
              <p class="text-sm text-muted-foreground truncate">{{ item.name_en }}</p>
              <span class="text-xs text-muted-foreground">{{ item.year }}</span>
            </div>
          </li>
        </ul>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { cardPosterUrl } from '@/composables/useImageProxy'
import { anidleApi } from '@/api/anidle'
import type { SearchResultItem } from '@/api/anidle'
import Input from '@/components/ui/Input.vue'
import Spinner from '@/components/ui/Spinner.vue'

const props = defineProps<{
  disabled?: boolean
}>()

const emit = defineEmits<{
  select: [id: string]
}>()

const query = ref('')
const results = ref<SearchResultItem[]>([])
const isSearching = ref(false)
const isOpen = ref(false)
const activeIndex = ref(-1)

const containerRef = ref<HTMLElement | null>(null)
const inputRef = ref<InstanceType<typeof Input> | null>(null)

// ── Debounced search ─────────────────────────────────────────────────────────

let debounceTimer: ReturnType<typeof setTimeout> | null = null

watch(query, (val) => {
  if (debounceTimer) clearTimeout(debounceTimer)
  if (val.length < 2) {
    results.value = []
    isOpen.value = false
    return
  }
  debounceTimer = setTimeout(() => {
    void doSearch(val)
  }, 300)
})

async function doSearch(q: string) {
  isSearching.value = true
  isOpen.value = true
  try {
    const res = await anidleApi.search(q)
    if (query.value !== q) return // a newer query superseded this one — drop the stale result
    const data = res.data?.data ?? res.data
    results.value = (data as SearchResultItem[]) ?? []
    activeIndex.value = -1
  } catch {
    if (query.value === q) results.value = []
  } finally {
    if (query.value === q) isSearching.value = false
  }
}

// ── Keyboard navigation ───────────────────────────────────────────────────────

function onKeydown(e: KeyboardEvent) {
  if (!isOpen.value) return
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    activeIndex.value = Math.min(activeIndex.value + 1, results.value.length - 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    activeIndex.value = Math.max(activeIndex.value - 1, 0)
  } else if (e.key === 'Enter' && activeIndex.value >= 0) {
    e.preventDefault()
    const item = results.value[activeIndex.value]
    if (item) selectItem(item)
  } else if (e.key === 'Escape') {
    isOpen.value = false
    activeIndex.value = -1
  }
}

function onFocus() {
  if (results.value.length > 0) isOpen.value = true
}

function onBlur() {
  // Small delay so mousedown on item fires first
  setTimeout(() => {
    isOpen.value = false
  }, 150)
}

function selectItem(item: SearchResultItem) {
  emit('select', item.id)
  query.value = ''
  results.value = []
  isOpen.value = false
  activeIndex.value = -1
}

function posterSrc(url: string) {
  return cardPosterUrl(url, 128)
}
</script>
