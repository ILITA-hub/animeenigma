<template>
  <div class="relative" ref="containerRef">
    <Input
      :model-value="modelValue"
      type="search"
      :placeholder="placeholder ?? $t('search.placeholder')"
      :size="size"
      clearable
      role="combobox"
      aria-autocomplete="list"
      :aria-controls="listboxId"
      :aria-expanded="results.length > 0"
      :aria-activedescendant="highlightedIndex >= 0 ? `${listboxId}-opt-${highlightedIndex}` : undefined"
      @update:model-value="onInput"
      @keydown.enter.prevent="onEnter"
      @keydown.down.prevent="highlightNext"
      @keydown.up.prevent="highlightPrev"
      @keydown.escape="closeDropdown"
    >
      <template #prefix>
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
      </template>
    </Input>

    <Transition name="dropdown">
      <div
        v-if="results.length > 0"
        :id="listboxId"
        role="listbox"
        class="absolute top-full left-0 right-0 mt-2 glass-elevated rounded-xl overflow-hidden z-50 max-h-96 overflow-y-auto"
      >
        <router-link
          v-for="(result, index) in results"
          :id="`${listboxId}-opt-${index}`"
          :key="result.id"
          :to="`/anime/${result.id}`"
          role="option"
          :aria-selected="highlightedIndex === index"
          class="flex items-center gap-3 px-3 py-2.5 hover:bg-white/10 transition-colors"
          :class="{ 'bg-white/10': highlightedIndex === index }"
          @click="closeDropdown"
          @mouseenter="highlightedIndex = index"
        >
          <img
            :src="result.coverImage"
            :alt="result.title"
            class="w-10 h-14 rounded-md object-cover flex-shrink-0"
          />
          <div class="flex-1 min-w-0">
            <p class="text-white text-sm font-medium truncate">{{ result.title }}</p>
            <p class="text-white/60 text-xs">
              {{ result.releaseYear || '' }}{{ result.releaseYear && result.totalEpisodes ? ' \u00b7 ' : '' }}{{ result.totalEpisodes ? result.totalEpisodes + ' ' + $t('anime.episodesShort') : '' }}
            </p>
          </div>
          <span v-if="result.rating" class="text-cyan-400 text-xs font-medium flex-shrink-0">
            {{ result.rating.toFixed(1) }}
          </span>
        </router-link>
        <router-link
          :to="{ path: '/browse', query: { q: modelValue } }"
          class="block px-3 py-2.5 text-center text-sm text-cyan-400 hover:bg-white/10 transition-colors border-t border-white/10"
          @click="closeDropdown"
        >
          {{ $t('search.viewAll') }}
        </router-link>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { onClickOutside, useDebounceFn } from '@vueuse/core'
import { animeApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import Input from './Input.vue'

interface SearchResult {
  id: string
  title: string
  coverImage: string
  releaseYear?: number
  totalEpisodes?: number
  rating?: number
}

const props = withDefaults(defineProps<{
  modelValue: string
  placeholder?: string
  size?: 'sm' | 'md' | 'lg'
  listboxId?: string
}>(), {
  size: 'lg',
  listboxId: 'search-autocomplete-listbox',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  submit: []
}>()

const router = useRouter()

const results = ref<SearchResult[]>([])
const highlightedIndex = ref(-1)
const containerRef = ref<HTMLElement | null>(null)
let abortController: AbortController | null = null

const debouncedSearch = useDebounceFn(async (query: string) => {
  if (query.length < 2) {
    results.value = []
    highlightedIndex.value = -1
    return
  }
  abortController?.abort()
  abortController = new AbortController()
  try {
    const response = await animeApi.search(query, undefined, 5, abortController.signal)
    const data = response.data?.data || response.data
    const list = Array.isArray(data) ? data : []
    results.value = list.map((a: Record<string, unknown>) => ({
      id: a.id as string,
      title: getLocalizedTitle(a.name as string, a.name_ru as string, a.name_jp as string),
      coverImage: getImageUrl(a.poster_url as string | undefined),
      releaseYear: a.year as number | undefined,
      totalEpisodes: a.episodes_count as number | undefined,
      rating: a.score as number | undefined,
    }))
    highlightedIndex.value = -1
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') return
    results.value = []
    highlightedIndex.value = -1
  }
}, 300)

function onInput(value: string) {
  emit('update:modelValue', value)
  highlightedIndex.value = -1
  debouncedSearch(value)
}

function closeDropdown() {
  results.value = []
  highlightedIndex.value = -1
}

function highlightNext() {
  if (results.value.length === 0) return
  highlightedIndex.value = (highlightedIndex.value + 1) % results.value.length
}

function highlightPrev() {
  if (results.value.length === 0) return
  highlightedIndex.value = highlightedIndex.value <= 0
    ? results.value.length - 1
    : highlightedIndex.value - 1
}

function onEnter() {
  if (highlightedIndex.value >= 0 && results.value[highlightedIndex.value]) {
    router.push(`/anime/${results.value[highlightedIndex.value].id}`)
    closeDropdown()
    return
  }
  emit('submit')
  closeDropdown()
}

watch(() => props.modelValue, (next) => {
  // External programmatic clears (e.g. parent clears searchQuery after submit)
  // should collapse the dropdown too.
  if (!next) closeDropdown()
})

onClickOutside(containerRef, closeDropdown)
</script>
