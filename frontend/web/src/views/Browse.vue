<template>
  <div class="min-h-screen pt-20 pb-20 md:pb-8">
    <div class="max-w-7xl mx-auto px-4">
      <!-- Search Header -->
      <div class="mb-6">
        <h1 class="text-2xl md:text-3xl font-bold text-white mb-6">
          {{ $t('nav.catalog') }}
        </h1>
      </div>

      <!-- Mobile filter toggle (visible below md) -->
      <div class="md:hidden mb-4">
        <button
          ref="toggleButtonRef"
          type="button"
          class="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-white/5 border border-white/10 text-white/80 hover:text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/40"
          :aria-expanded="drawerOpen"
          aria-controls="browse-filter-drawer"
          @click="drawerOpen = true"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2a1 1 0 01-.293.707L15 12.414V19a1 1 0 01-1.447.894l-4-2A1 1 0 019 17v-4.586L3.293 6.707A1 1 0 013 6V4z" />
          </svg>
          {{ $t('browse.filters.mobileToggle') }}
          <span
            v-if="filters.activeCount.value"
            class="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 rounded-full bg-cyan-500/20 text-cyan-300 text-[10px] font-semibold"
            :aria-label="$t('browse.filters.activeCount', { count: filters.activeCount.value })"
          >{{ filters.activeCount.value }}</span>
        </button>
      </div>

      <!-- Desktop two-column grid: sidebar (280) + results -->
      <div class="grid grid-cols-1 md:grid-cols-[280px_1fr] gap-6">
        <!-- Desktop sidebar -->
        <div class="hidden md:block">
          <BrowseSidebar :genres="browseGenres" :filters="filters" />
        </div>

        <!-- Results column -->
        <div>
          <!-- Search Input (moved into results column above the grid) -->
          <div class="mb-4 relative z-40">
            <SearchAutocomplete
              v-model="searchQuery"
              listbox-id="browse-search"
              @submit="handleSearch"
            />
          </div>

          <!-- Recent Searches (when no query) -->
          <div v-if="!searchQuery && recentSearches.length > 0" class="mb-6">
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
            <!-- UA-048 (UX-11 Phase 4): sr-only h2 to satisfy heading-order. -->
            <h2 class="sr-only">{{ $t('browse.resultsHeading') }}</h2>
            <div class="grid grid-cols-2 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
              <AnimeCardNew
                v-for="anime in animeList"
                :key="anime.id"
                :anime="anime"
                :list-status="getListStatus(anime.id)"
                :site-rating="siteRatings[String(anime.id)]"
                :progress="browseProgress.get(String(anime.id)) ?? null"
                :menu-open="contextMenu.visible && String(contextMenu.anime?.id) === String(anime.id)"
                @open-menu="(el: HTMLElement) => openContextMenuAt(el, anime, { listStatus: getListStatus(anime.id), siteRating: siteRatings[String(anime.id)] })"
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
    </div>
  </div>

  <!-- Mobile drawer (hidden on md+) -->
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="-translate-x-full opacity-0"
      enter-to-class="translate-x-0 opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="translate-x-0 opacity-100"
      leave-to-class="-translate-x-full opacity-0"
    >
      <div
        v-if="drawerOpen"
        id="browse-filter-drawer"
        ref="drawerRef"
        role="dialog"
        aria-modal="true"
        :aria-label="$t('browse.filters.title')"
        class="fixed inset-0 z-50 md:hidden flex"
      >
        <!-- Backdrop -->
        <div class="absolute inset-0 bg-black/60" @click="drawerOpen = false" />
        <!-- Panel -->
        <div class="relative w-[85%] max-w-[320px] h-full bg-slate-950 border-r border-white/10 overflow-y-auto p-4">
          <div class="flex items-center justify-between mb-3">
            <h2 class="text-lg font-semibold text-white">{{ $t('browse.filters.title') }}</h2>
            <button
              type="button"
              class="p-1 rounded text-white/60 hover:text-white focus:outline-none focus:ring-2 focus:ring-cyan-500/40"
              :aria-label="$t('common.close')"
              @click="drawerOpen = false"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <BrowseSidebar :genres="browseGenres" :filters="filters" />
        </div>
      </div>
    </Transition>
  </Teleport>

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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAnime } from '@/composables/useAnime'
import { useAuthStore } from '@/stores/auth'
import { Button, PaginationBar, SearchAutocomplete } from '@/components/ui'
import { AnimeCardNew, AnimeContextMenu } from '@/components/anime'
import BrowseSidebar from '@/components/browse/BrowseSidebar.vue'
import { useBrowseFilters } from '@/composables/useBrowseFilters'
import { useFocusTrap } from '@/composables/useFocusTrap'
import { animeApi } from '@/api/client'
import { useWatchlistStore } from '@/stores/watchlist'
import { getLocalizedGenre } from '@/utils/title'
import { useSiteRatings } from '@/composables/useSiteRatings'
import { useContextMenu } from '@/composables/useContextMenu'
import { useAnimeProgress } from '@/composables/useAnimeProgress'

interface Genre {
  id: string
  name: string
  name_ru?: string
}

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const { animeList, loading, error, fetchAnimeList, searchAnime, paginationMeta } = useAnime()

// Phase 9 (UX-16): bulk per-card progress for the Browse grid.
const browseIds = computed(() => animeList.value.map((a: { id: string | number }) => String(a.id)))
const { progressMap: browseProgress } = useAnimeProgress(browseIds)

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
const { contextMenu, openAtElement: openContextMenuAt, onTouchstart, onTouchmove, onTouchend } = useContextMenu()

// Phase 15 (UX-31) — filter state is owned by the composable; the sidebar
// (desktop) and the mobile drawer share this same instance via prop.
const filters = useBrowseFilters()

// SearchAutocomplete uses v-model; keep it in sync with filters.q
// bidirectionally without echoing.
const searchQuery = ref(filters.q.value)
watch(
  () => filters.q.value,
  (v) => {
    if (searchQuery.value !== v) searchQuery.value = v
  },
  { immediate: true },
)
watch(searchQuery, (v) => {
  if (filters.q.value !== v) {
    filters.q.value = v
  }
})

const currentPage = ref(1)
const recentSearches = ref<string[]>([])
const loadingShikimori = ref(false)

const totalPages = computed(() => paginationMeta.value?.total_pages ?? 0)

const genres = ref<Genre[]>([])
const browseGenres = computed(() =>
  genres.value.map(g => ({ id: g.id, name: g.name, name_ru: g.name_ru })),
)

// ─── Mobile drawer state + a11y ────────────────────────────────────────
const drawerOpen = ref(false)
const drawerRef = ref<HTMLElement | null>(null)
const toggleButtonRef = ref<HTMLButtonElement | null>(null)

useFocusTrap({
  active: drawerOpen,
  container: drawerRef,
  returnFocusTo: toggleButtonRef as unknown as import('vue').Ref<HTMLElement | null>,
})

// ESC closes the drawer (focus trap handles Tab cycling; ESC is separate
// per Phase 6 navbar drawer pattern).
function onDrawerKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && drawerOpen.value) {
    drawerOpen.value = false
  }
}
onMounted(() => document.addEventListener('keydown', onDrawerKey))
onBeforeUnmount(() => document.removeEventListener('keydown', onDrawerKey))

// Auto-close drawer when filters reset to all-empty (mobile UX: reset
// should give the user a clean canvas, not leave them inside the drawer).
watch(
  () => filters.activeCount.value,
  (n, prev) => {
    if (drawerOpen.value && prev && prev > 0 && n === 0) {
      drawerOpen.value = false
    }
  },
)

// ─── Re-fetch on any filter change ─────────────────────────────────────
// The composable's apiParams covers all 6 axes + search + sort. Browse
// owns pagination, so we reset to page 1 on any filter change.
watch(
  () => filters.apiParams.value,
  async () => {
    currentPage.value = 1
    await loadAnime()
  },
  { deep: true },
)

const loadAnime = async () => {
  const params: Record<string, string | number | boolean> = {
    page: currentPage.value,
    ...filters.apiParams.value,
  }
  await fetchAnimeList(params)
}

const handleSearch = async () => {
  currentPage.value = 1
  if (searchQuery.value.trim()) {
    // Save to recent searches
    const searches = recentSearches.value.filter(s => s !== searchQuery.value)
    searches.unshift(searchQuery.value)
    recentSearches.value = searches.slice(0, 5)
    localStorage.setItem('recentSearches', JSON.stringify(recentSearches.value))
  }
  // Push the new q (or absence thereof) through the composable so the
  // URL stays canonical and the apiParams watcher re-fetches.
  filters.q.value = searchQuery.value
  filters.writeUrl()
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

const clearRecentSearches = () => {
  recentSearches.value = []
  localStorage.removeItem('recentSearches')
}

const goToPage = async (page: number) => {
  currentPage.value = page
  const query = { ...route.query, page: page > 1 ? String(page) : undefined }
  router.replace({ query })
  await loadAnime()
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

// Load recent searches from localStorage + genres + watchlist + initial fetch.
onMounted(async () => {
  const stored = localStorage.getItem('recentSearches')
  if (stored) {
    recentSearches.value = JSON.parse(stored)
  }

  // Load genres and watchlist in parallel.
  const genrePromise = animeApi
    .getGenres()
    .then(response => {
      const data = response.data?.data || response.data || []
      genres.value = data.sort((a: Genre, b: Genre) =>
        getLocalizedGenre(a.name, a.name_ru).localeCompare(getLocalizedGenre(b.name, b.name_ru)),
      )
    })
    .catch(err => {
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

  // useBrowseFilters runs readUrl() on mount, so all filter axes are
  // already populated by the time we get here. Trigger the initial fetch
  // — the apiParams watcher will handle subsequent changes.
  if (filters.q.value) {
    await searchAnime(filters.q.value)
  } else {
    await loadAnime()
  }
})

// Watch ?page changes (browser back/forward to a different page within
// the same filter set).
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
