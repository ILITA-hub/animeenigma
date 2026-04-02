<template>
  <div class="min-h-screen pt-20 pb-20 md:pb-8">
    <div class="max-w-7xl mx-auto px-4">
      <!-- Search Header -->
      <div class="mb-8">
        <h1 class="text-2xl md:text-3xl font-bold text-white mb-6">
          {{ $t('nav.catalog') }}
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
          <div class="w-full sm:w-44">
            <GenreFilterPopup
              v-model="selectedGenre"
              :genres="browseGenres"
              :placeholder="$t('search.genre')"
              :all-label="$t('profile.genreFilter.all')"
              :search-placeholder="$t('profile.genreFilter.search')"
              size="sm"
              show-emoji
              @change="handleFilter"
            />
          </div>

          <!-- Status Filter -->
          <div class="w-full sm:w-36">
            <Select
              v-model="selectedStatus"
              :options="statusOptions"
              size="sm"
              @change="handleFilter"
            />
          </div>

          <!-- Year Filter -->
          <div class="w-full sm:w-32">
            <Select
              v-model="selectedYear"
              :options="yearOptions"
              size="sm"
              @change="handleFilter"
            />
          </div>

          <!-- Sort -->
          <div class="w-full sm:w-40">
            <Select
              v-model="sortBy"
              :options="sortOptions"
              size="sm"
              @change="handleFilter"
            />
          </div>

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
      <div v-if="!searchQuery && recentSearches.length > 0" class="mb-8">
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
        <Button
          v-if="searchQuery"
          variant="outline"
          class="mt-4"
          :loading="loadingShikimori"
          @click="searchOnShikimori"
        >
          <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          {{ $t('browse.searchShikimori') }}
        </Button>
      </div>

      <!-- Results section -->
      <template v-else>
        <!-- Search on Shikimori (when results exist but user wants fresh data) -->
        <div v-if="searchQuery && animeList.length > 0" class="mb-4 flex justify-end">
          <button
            class="text-sm text-cyan-400 hover:text-cyan-300 flex items-center gap-1 transition-colors"
            :disabled="loadingShikimori"
            @click="searchOnShikimori"
          >
            <svg v-if="loadingShikimori" class="w-4 h-4 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            {{ $t('browse.refreshShikimori') }}
          </button>
        </div>

        <!-- Results Grid -->
        <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
        <AnimeCardNew
          v-for="anime in animeList"
          :key="anime.id"
          :anime="anime"
          :list-status="getListStatus(anime.id)"
          :site-rating="siteRatings[String(anime.id)]"
          @contextmenu="openContextMenu($event, anime, { listStatus: getListStatus(anime.id), siteRating: siteRatings[String(anime.id)] })"
          @touchstart="onTouchstart($event, anime, { listStatus: getListStatus(anime.id), siteRating: siteRatings[String(anime.id)] })"
          @touchmove="onTouchmove"
          @touchend="onTouchend"
        />
      </div>

        <!-- Pagination -->
        <PaginationBar
          :current-page="currentPage"
          :total-pages="totalPages"
          @update:current-page="goToPage"
        />
      </template>
    </div>
  </div>

  <!-- Context Menu -->
  <AnimeContextMenu
    :visible="contextMenu.visible"
    :x="contextMenu.x"
    :y="contextMenu.y"
    :anime="contextMenu.anime"
    :list-status="contextMenu.listStatus"
    :site-rating="contextMenu.siteRating"
    @update:visible="contextMenu.visible = $event"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDebounceFn } from '@vueuse/core'
import { useAnime } from '@/composables/useAnime'
import { useAuthStore } from '@/stores/auth'
import { Input, Badge, Button, Select, GenreFilterPopup, PaginationBar } from '@/components/ui'
import { AnimeCardNew, AnimeContextMenu } from '@/components/anime'
import { animeApi } from '@/api/client'
import { useWatchlistStore } from '@/stores/watchlist'
import { useI18n } from 'vue-i18n'
import { getLocalizedTitle, getLocalizedGenre } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import { useSiteRatings } from '@/composables/useSiteRatings'
import { useContextMenu } from '@/composables/useContextMenu'

const { t } = useI18n()

interface Genre {
  id: string
  name: string
  name_ru?: string
}

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const { animeList, loading, error, fetchAnimeList, searchAnime, paginationMeta } = useAnime()

// Watchlist status map via shared store
const watchlistStore = useWatchlistStore()
const watchlistMap = computed(() => watchlistStore.watchlistMap)

const fetchWatchlistMap = async () => {
  if (!authStore.isAuthenticated) return
  await watchlistStore.fetchWatchlist()
}

const getListStatus = (animeId: string | number): string | null => {
  return watchlistMap.value.get(String(animeId)) || null
}

// Site ratings for anime cards
const { ratings: siteRatings, fetchRatings: fetchSiteRatings } = useSiteRatings()

// Context menu
const { contextMenu, open: openContextMenu, onTouchstart, onTouchmove, onTouchend } = useContextMenu()

const searchQuery = ref('')
const selectedGenre = ref('')
const selectedYear = ref('')
const selectedStatus = ref('')
const sortBy = ref('popularity')
const currentPage = ref(1)
const showLiveResults = ref(false)
const liveResults = ref<Array<{ id: string; title: string; name?: string; nameRu?: string; nameJp?: string; coverImage: string; releaseYear?: number; episodes?: number; rating?: number }>>([])
const recentSearches = ref<string[]>([])
const loadingShikimori = ref(false)

const totalPages = computed(() => paginationMeta.value?.total_pages ?? 0)

const hasActiveFilters = computed(() => !!selectedGenre.value || !!selectedYear.value || !!selectedStatus.value || sortBy.value !== 'popularity')

const genres = ref<Genre[]>([])

const browseGenres = computed(() =>
  genres.value.map(g => ({ id: g.id, name: g.name, name_ru: g.name_ru }))
)

const statusOptions = computed(() => [
  { value: '', label: t('browse.filterStatus') },
  { value: 'ongoing', label: t('browse.filterOngoing') },
  { value: 'announced', label: t('browse.filterAnnounced') },
  { value: 'released', label: t('browse.filterReleased') },
])

const yearOptions = computed(() => [
  { value: '', label: t('browse.filterYear') },
  ...Array.from({ length: 30 }, (_, i) => {
    const year = new Date().getFullYear() - i
    return { value: String(year), label: String(year) }
  })
])

const sortOptions = computed(() => [
  { value: 'popularity', label: t('browse.sortPopular') },
  { value: 'rating', label: t('browse.sortRating') },
  { value: 'year', label: t('browse.sortYear') },
  { value: 'title', label: t('browse.sortTitle') },
])

const clearFilters = () => {
  selectedGenre.value = ''
  selectedYear.value = ''
  selectedStatus.value = ''
  sortBy.value = 'popularity'
  router.replace({ query: {} })
  handleFilter()
}

const clearRecentSearches = () => {
  recentSearches.value = []
  localStorage.removeItem('recentSearches')
}

// Debounced live search with AbortController to cancel stale requests
let searchAbortController: AbortController | null = null

const debouncedLiveSearch = useDebounceFn(async (query: string) => {
  if (query.length < 2) {
    liveResults.value = []
    showLiveResults.value = false
    return
  }

  // Cancel previous in-flight search request
  if (searchAbortController) {
    searchAbortController.abort()
  }
  searchAbortController = new AbortController()

  try {
    const response = await animeApi.search(query, undefined, 5, searchAbortController.signal)
    const data = response.data?.data || response.data
    const list = Array.isArray(data) ? data : []
    liveResults.value = list.map((a: Record<string, unknown>) => ({
      id: a.id as string,
      title: getLocalizedTitle(a.name as string, a.name_ru as string, a.name_jp as string),
      name: a.name as string | undefined,
      nameRu: a.name_ru as string | undefined,
      nameJp: a.name_jp as string | undefined,
      coverImage: getImageUrl(a.poster_url as string | undefined),
      releaseYear: a.year as number | undefined,
      episodes: a.episodes_count as number | undefined,
      rating: a.score as number | undefined,
    }))
    showLiveResults.value = true
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') return
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
    router.replace({ query: { ...route.query, q: searchQuery.value, page: undefined } })
  } else {
    await loadAnime()
    router.replace({ query: { ...route.query, q: undefined, page: undefined } })
  }
}

const searchOnShikimori = async () => {
  if (!searchQuery.value.trim()) return
  loadingShikimori.value = true
  try {
    await searchAnime(searchQuery.value, 'shikimori')
  } finally {
    loadingShikimori.value = false
  }
}

const handleFilter = async () => {
  currentPage.value = 1
  const query = { ...route.query, page: undefined }
  router.replace({ query })
  await loadAnime()
}

const loadAnime = async () => {
  const params: Record<string, string | number | boolean> = {
    page: currentPage.value,
    sort: sortBy.value,
  }
  if (selectedGenre.value) {
    params.genre = selectedGenre.value
  }
  if (selectedYear.value) {
    params.year = selectedYear.value
  }
  if (selectedStatus.value) {
    params.status = selectedStatus.value
  }
  await fetchAnimeList(params)
}

const goToPage = async (page: number) => {
  currentPage.value = page
  const query = { ...route.query, page: page > 1 ? String(page) : undefined }
  router.replace({ query })
  await loadAnime()
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

// Load recent searches from localStorage
onMounted(async () => {
  const stored = localStorage.getItem('recentSearches')
  if (stored) {
    recentSearches.value = JSON.parse(stored)
  }

  // Load genres and watchlist in parallel
  const genrePromise = animeApi.getGenres().then(response => {
    const data = response.data?.data || response.data || []
    genres.value = data.sort((a: Genre, b: Genre) => getLocalizedGenre(a.name, a.name_ru).localeCompare(getLocalizedGenre(b.name, b.name_ru)))
  }).catch(err => {
    console.error('Failed to load genres:', err)
  })

  const watchlistPromise = fetchWatchlistMap()

  await Promise.all([genrePromise, watchlistPromise])

  // Store pending MAL bind for later resolution on Anime page
  if (route.query.bind_mal) {
    sessionStorage.setItem('pending_mal_bind', route.query.bind_mal as string)
  }

  if (route.query.page) {
    currentPage.value = parseInt(route.query.page as string, 10) || 1
  }

  if (route.query.status) {
    selectedStatus.value = route.query.status as string
  }

  if (route.query.genre) {
    selectedGenre.value = route.query.genre as string
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

watch(() => route.query.status, (newStatus) => {
  const val = (newStatus as string) || ''
  if (val !== selectedStatus.value) {
    selectedStatus.value = val
    handleFilter()
  }
})

watch(() => route.query.genre, (newGenre) => {
  const val = (newGenre as string) || ''
  if (val !== selectedGenre.value) {
    selectedGenre.value = val
    handleFilter()
  }
})

watch(() => route.query.page, (newPage) => {
  const page = parseInt(newPage as string, 10) || 1
  if (page !== currentPage.value) {
    currentPage.value = page
    loadAnime()
  }
})

// Fetch site ratings whenever anime list changes
watch(animeList, (list) => {
  if (list.length > 0) {
    fetchSiteRatings(list.map(a => String(a.id)))
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
