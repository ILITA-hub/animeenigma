<template>
  <div class="min-h-screen pt-20 pb-20 md:pb-8">
    <div class="max-w-7xl mx-auto px-4">
      <!-- Search Header -->
      <div class="mb-8">
        <h1 class="text-2xl md:text-3xl font-bold text-white mb-6">
          {{ isSearchMode ? $t('nav.search') : $t('nav.catalog') }}
        </h1>

        <!-- Search Input -->
        <div class="relative mb-4">
          <Input
            v-model="searchQuery"
            type="search"
            :placeholder="$t('search.placeholder')"
            size="lg"
            clearable
            @input="handleSearchInput"
            @keyup.enter="handleSearch"
          >
            <template #prefix>
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
            </template>
          </Input>

          <!-- Live Search Results Dropdown -->
          <Transition name="dropdown">
            <div
              v-if="showLiveResults && liveResults.length > 0"
              class="absolute top-full left-0 right-0 mt-2 glass-elevated rounded-xl max-h-96 overflow-y-auto z-20"
            >
              <router-link
                v-for="result in liveResults"
                :key="result.id"
                :to="`/anime/${result.id}`"
                class="flex items-center gap-3 p-3 hover:bg-white/10 transition-colors"
                @click="showLiveResults = false"
              >
                <img
                  :src="result.coverImage"
                  :alt="result.title"
                  class="w-12 h-16 rounded-lg object-cover"
                />
                <div class="flex-1 min-w-0">
                  <p class="text-white font-medium truncate">{{ result.title }}</p>
                  <p class="text-white/50 text-sm">
                    {{ result.releaseYear }} • {{ result.episodes }} eps
                  </p>
                </div>
                <Badge v-if="result.rating" variant="rating" size="sm">
                  {{ result.rating.toFixed(1) }}
                </Badge>
              </router-link>
            </div>
          </Transition>
        </div>

        <!-- Filters -->
        <div class="flex flex-wrap gap-3">
          <!-- Genre Filter -->
          <div class="relative" ref="genreDropdownRef">
            <button
              class="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-white/5 border border-white/10 text-white/70 hover:text-white hover:bg-white/10 transition-colors"
              @click="genreDropdownOpen = !genreDropdownOpen"
            >
              <span>{{ selectedGenre || $t('search.genre') }}</span>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
              </svg>
            </button>
            <Transition name="dropdown">
              <div
                v-if="genreDropdownOpen"
                class="absolute top-full left-0 mt-2 py-2 w-48 glass-elevated rounded-xl z-10"
              >
                <button
                  v-for="genre in genres"
                  :key="genre.value"
                  class="w-full px-4 py-2 text-left text-sm hover:bg-white/10 transition-colors"
                  :class="selectedGenre === genre.value ? 'text-cyan-400' : 'text-white/70'"
                  @click="selectGenre(genre.value)"
                >
                  {{ genre.label }}
                </button>
              </div>
            </Transition>
          </div>

          <!-- Year Filter -->
          <select
            v-model="selectedYear"
            class="px-4 py-2.5 rounded-xl bg-white/5 border border-white/10 text-white/70 focus:outline-none focus:border-cyan-500 appearance-none cursor-pointer"
            @change="handleFilter"
          >
            <option value="">{{ $t('search.year') }}</option>
            <option v-for="year in years" :key="year" :value="year">{{ year }}</option>
          </select>

          <!-- Sort -->
          <select
            v-model="sortBy"
            class="px-4 py-2.5 rounded-xl bg-white/5 border border-white/10 text-white/70 focus:outline-none focus:border-cyan-500 appearance-none cursor-pointer"
            @change="handleFilter"
          >
            <option value="popularity">{{ $t('search.sort') }}: Popular</option>
            <option value="rating">{{ $t('search.sort') }}: Rating</option>
            <option value="year">{{ $t('search.sort') }}: Year</option>
            <option value="title">{{ $t('search.sort') }}: A-Z</option>
          </select>

          <!-- Clear Filters -->
          <button
            v-if="hasActiveFilters"
            class="px-4 py-2.5 text-pink-400 hover:text-pink-300 transition-colors"
            @click="clearFilters"
          >
            {{ $t('search.clearAll') }}
          </button>
        </div>
      </div>

      <!-- Recent Searches (when no query) -->
      <div v-if="isSearchMode && !searchQuery && recentSearches.length > 0" class="mb-8">
        <div class="flex items-center justify-between mb-3">
          <h2 class="text-lg font-semibold text-white">{{ $t('search.recent') }}</h2>
          <button class="text-sm text-pink-400 hover:text-pink-300" @click="clearRecentSearches">
            {{ $t('search.clearAll') }}
          </button>
        </div>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="search in recentSearches"
            :key="search"
            class="px-4 py-2 rounded-full bg-white/5 border border-white/10 text-white/70 hover:text-white hover:bg-white/10 transition-colors"
            @click="searchQuery = search; handleSearch()"
          >
            {{ search }}
          </button>
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-20">
        <div class="w-12 h-12 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin" />
      </div>

      <!-- Error State -->
      <div v-else-if="error" class="text-center py-20">
        <p class="text-pink-400 mb-4">{{ error }}</p>
        <Button variant="outline" @click="loadAnime">{{ $t('common.retry') }}</Button>
      </div>

      <!-- Empty State -->
      <div v-else-if="animeList.length === 0" class="text-center py-20">
        <svg class="w-16 h-16 mx-auto text-white/20 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.172 16.172a4 4 0 015.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <p class="text-white/50 text-lg">{{ $t('search.noResults') }}</p>
      </div>

      <!-- Results Grid -->
      <div v-else class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
        <AnimeCardNew
          v-for="anime in animeList"
          :key="anime.id"
          :anime="anime"
        />
      </div>

      <!-- Load More -->
      <div v-if="hasMore && animeList.length > 0" class="flex justify-center mt-8">
        <Button variant="ghost" :loading="loadingMore" @click="loadMore">
          Load More
        </Button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { onClickOutside, useDebounceFn } from '@vueuse/core'
import { useAnime } from '@/composables/useAnime'
import { Input, Badge, Button } from '@/components/ui'
import { AnimeCardNew } from '@/components/anime'

const route = useRoute()
const router = useRouter()
const { animeList, loading, error, fetchAnimeList, searchAnime } = useAnime()

const searchQuery = ref('')
const selectedGenre = ref('')
const selectedYear = ref('')
const sortBy = ref('popularity')
const currentPage = ref(1)
const hasMore = ref(true)
const loadingMore = ref(false)
const genreDropdownOpen = ref(false)
const genreDropdownRef = ref<HTMLElement | null>(null)
const showLiveResults = ref(false)
const liveResults = ref<Array<{ id: string; title: string; coverImage: string; releaseYear?: number; episodes?: number; rating?: number }>>([])
const recentSearches = ref<string[]>([])

const isSearchMode = computed(() => route.name === 'search')
const hasActiveFilters = computed(() => !!selectedGenre.value || !!selectedYear.value || sortBy.value !== 'popularity')

const genres = [
  { value: '', label: 'All Genres' },
  { value: 'action', label: 'Action' },
  { value: 'adventure', label: 'Adventure' },
  { value: 'comedy', label: 'Comedy' },
  { value: 'drama', label: 'Drama' },
  { value: 'fantasy', label: 'Fantasy' },
  { value: 'romance', label: 'Romance' },
  { value: 'sci-fi', label: 'Sci-Fi' },
  { value: 'slice-of-life', label: 'Slice of Life' },
  { value: 'sports', label: 'Sports' },
  { value: 'supernatural', label: 'Supernatural' },
]

const years = Array.from({ length: 30 }, (_, i) => new Date().getFullYear() - i)

const selectGenre = (genre: string) => {
  selectedGenre.value = genre
  genreDropdownOpen.value = false
  handleFilter()
}

const clearFilters = () => {
  selectedGenre.value = ''
  selectedYear.value = ''
  sortBy.value = 'popularity'
  handleFilter()
}

const clearRecentSearches = () => {
  recentSearches.value = []
  localStorage.removeItem('recentSearches')
}

// Debounced live search
const debouncedLiveSearch = useDebounceFn(async (query: string) => {
  if (query.length < 2) {
    liveResults.value = []
    showLiveResults.value = false
    return
  }

  try {
    // Mock live results - would call API
    liveResults.value = animeList.value.filter(a =>
      a.title.toLowerCase().includes(query.toLowerCase())
    ).slice(0, 5) as typeof liveResults.value
    showLiveResults.value = true
  } catch (err) {
    console.error('Live search error:', err)
  }
}, 300)

const handleSearchInput = () => {
  debouncedLiveSearch(searchQuery.value)
}

const handleSearch = async () => {
  showLiveResults.value = false
  currentPage.value = 1

  if (searchQuery.value.trim()) {
    // Save to recent searches
    const searches = recentSearches.value.filter(s => s !== searchQuery.value)
    searches.unshift(searchQuery.value)
    recentSearches.value = searches.slice(0, 5)
    localStorage.setItem('recentSearches', JSON.stringify(recentSearches.value))

    await searchAnime(searchQuery.value)
    router.replace({ query: { ...route.query, q: searchQuery.value } })
  } else {
    await loadAnime()
    router.replace({ query: { ...route.query, q: undefined } })
  }
}

const handleFilter = async () => {
  currentPage.value = 1
  await loadAnime()
}

const loadAnime = async () => {
  const params = {
    page: currentPage.value,
    genre: selectedGenre.value,
    year: selectedYear.value,
    sort: sortBy.value,
  }
  const results = await fetchAnimeList(params)
  hasMore.value = (results?.length ?? 0) >= 20
}

const loadMore = async () => {
  loadingMore.value = true
  currentPage.value++
  await loadAnime()
  loadingMore.value = false
}

onClickOutside(genreDropdownRef, () => {
  genreDropdownOpen.value = false
})

// Load recent searches from localStorage
onMounted(async () => {
  const stored = localStorage.getItem('recentSearches')
  if (stored) {
    recentSearches.value = JSON.parse(stored)
  }

  if (route.query.q) {
    searchQuery.value = route.query.q as string
    await searchAnime(searchQuery.value)
  } else {
    await loadAnime()
  }
})

// Watch for route query changes
watch(() => route.query.q, async (newQuery) => {
  if (newQuery && newQuery !== searchQuery.value) {
    searchQuery.value = newQuery as string
    await searchAnime(searchQuery.value)
  }
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
