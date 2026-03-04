# Search Autocomplete Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a global autocomplete search bar to the navbar (desktop + mobile) and merge the `/search` route into `/browse`.

**Architecture:** Replace the navbar search icon button with an expandable input that calls the existing `GET /api/anime/search` endpoint with `page_size=5`. Show results in a dropdown. Remove `/search` route and merge its features (recent searches) into `/browse`. No backend changes needed — existing search API + Redis cache (15min TTL) handles everything.

**Tech Stack:** Vue 3, VueUse (`useDebounceFn`, `onClickOutside`), existing `animeApi.search`, Tailwind CSS

---

### Task 1: Extend `animeApi.search` to accept `page_size`

**Files:**
- Modify: `frontend/web/src/api/client.ts:154`

**Step 1: Update the search method signature**

Change line 154 from:
```typescript
search: (query: string, source?: string) => apiClient.get('/anime/search', { params: { q: query, ...(source && { source }) } }),
```
To:
```typescript
search: (query: string, source?: string, pageSize?: number) => apiClient.get('/anime/search', { params: { q: query, ...(source && { source }), ...(pageSize && { page_size: pageSize }) } }),
```

**Step 2: Verify no existing callers break**

Existing callers pass `(query)` or `(query, source)` — adding an optional 3rd param is backward-compatible. Callers:
- `frontend/web/src/composables/useAnime.ts:125` — `animeApi.search(query, source)` — still works
- `frontend/web/src/views/Browse.vue` — uses `searchAnime(query)` from composable — still works

**Step 3: Lint check**

Run: `cd frontend/web && bun lint`
Expected: clean

---

### Task 2: Add global search to Navbar (desktop)

**Files:**
- Modify: `frontend/web/src/components/layout/Navbar.vue`

**Step 1: Replace the search icon button with expandable search**

Replace the desktop search button (lines 36-45) with an expandable search input + autocomplete dropdown:

```vue
<!-- Search (replaces lines 36-45) -->
<div class="relative" ref="searchContainerRef">
  <button
    v-if="!searchOpen"
    class="p-2 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
    :aria-label="$t('nav.search')"
    @click="openSearch"
  >
    <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
    </svg>
  </button>
  <div v-else class="flex items-center gap-2">
    <div class="relative">
      <input
        ref="searchInputRef"
        v-model="searchQuery"
        type="text"
        :placeholder="$t('search.placeholder')"
        class="w-64 px-3 py-1.5 pl-9 bg-white/10 border border-white/20 rounded-lg text-white text-sm placeholder-white/40 focus:outline-none focus:border-cyan-400/50 focus:bg-white/15 transition-all"
        @input="onSearchInput"
        @keydown.enter="goToSearch"
        @keydown.escape="closeSearch"
        @keydown.down.prevent="highlightNext"
        @keydown.up.prevent="highlightPrev"
      />
      <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-white/40" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
      </svg>
      <!-- Autocomplete Dropdown -->
      <Transition name="dropdown">
        <div
          v-if="searchResults.length > 0"
          class="absolute top-full left-0 right-0 mt-2 glass-elevated rounded-xl overflow-hidden z-50"
          style="min-width: 320px"
        >
          <router-link
            v-for="(result, index) in searchResults"
            :key="result.id"
            :to="`/anime/${result.id}`"
            class="flex items-center gap-3 px-3 py-2.5 hover:bg-white/10 transition-colors"
            :class="{ 'bg-white/10': highlightedIndex === index }"
            @click="closeSearch"
          >
            <img
              :src="result.coverImage"
              :alt="result.title"
              class="w-10 h-14 rounded-md object-cover flex-shrink-0"
            />
            <div class="flex-1 min-w-0">
              <p class="text-white text-sm font-medium truncate">{{ result.title }}</p>
              <p class="text-white/40 text-xs">
                {{ result.releaseYear || '' }}{{ result.releaseYear && result.totalEpisodes ? ' · ' : '' }}{{ result.totalEpisodes ? result.totalEpisodes + ' эп.' : '' }}
              </p>
            </div>
            <span v-if="result.rating" class="text-cyan-400 text-xs font-medium flex-shrink-0">
              {{ result.rating.toFixed(1) }}
            </span>
          </router-link>
          <!-- "View all results" link -->
          <router-link
            :to="{ path: '/browse', query: { q: searchQuery } }"
            class="block px-3 py-2.5 text-center text-sm text-cyan-400 hover:bg-white/10 transition-colors border-t border-white/10"
            @click="closeSearch"
          >
            {{ $t('search.viewAll') }}
          </router-link>
        </div>
      </Transition>
    </div>
    <button
      class="p-1.5 text-white/40 hover:text-white transition-colors"
      @click="closeSearch"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
      </svg>
    </button>
  </div>
</div>
```

**Step 2: Add script logic**

Add these imports and refs to the `<script setup>` section:

```typescript
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { onClickOutside, useDebounceFn } from '@vueuse/core'
import { animeApi } from '@/api/client'
import Button from '@/components/ui/Button.vue'

// ... existing code ...

// Search state
const searchOpen = ref(false)
const searchQuery = ref('')
const searchResults = ref<Array<{ id: string; title: string; coverImage: string; releaseYear?: number; totalEpisodes?: number; rating?: number }>>([])
const searchLoading = ref(false)
const highlightedIndex = ref(-1)
const searchInputRef = ref<HTMLInputElement | null>(null)
const searchContainerRef = ref<HTMLElement | null>(null)

const openSearch = async () => {
  searchOpen.value = true
  await nextTick()
  searchInputRef.value?.focus()
}

const closeSearch = () => {
  searchOpen.value = false
  searchQuery.value = ''
  searchResults.value = []
  highlightedIndex.value = -1
}

const debouncedSearch = useDebounceFn(async (query: string) => {
  if (query.length < 2) {
    searchResults.value = []
    return
  }
  try {
    searchLoading.value = true
    const response = await animeApi.search(query, undefined, 5)
    const data = response.data?.data || response.data
    const list = Array.isArray(data) ? data : []
    searchResults.value = list.map((a: Record<string, unknown>) => ({
      id: a.id as string,
      title: (a.name_ru || a.name || a.name_jp || '') as string,
      coverImage: (a.poster_url || '') as string,
      releaseYear: a.year as number | undefined,
      totalEpisodes: a.episodes_count as number | undefined,
      rating: a.score as number | undefined,
    }))
  } catch {
    searchResults.value = []
  } finally {
    searchLoading.value = false
  }
}, 300)

const onSearchInput = () => {
  highlightedIndex.value = -1
  debouncedSearch(searchQuery.value)
}

const goToSearch = () => {
  if (highlightedIndex.value >= 0 && highlightedIndex.value < searchResults.value.length) {
    const result = searchResults.value[highlightedIndex.value]
    router.push(`/anime/${result.id}`)
  } else if (searchQuery.value.trim()) {
    router.push({ path: '/browse', query: { q: searchQuery.value } })
  }
  closeSearch()
}

const highlightNext = () => {
  if (searchResults.value.length === 0) return
  highlightedIndex.value = (highlightedIndex.value + 1) % searchResults.value.length
}

const highlightPrev = () => {
  if (searchResults.value.length === 0) return
  highlightedIndex.value = highlightedIndex.value <= 0
    ? searchResults.value.length - 1
    : highlightedIndex.value - 1
}

onClickOutside(searchContainerRef, () => {
  if (searchOpen.value) closeSearch()
})
```

**Step 3: Lint check**

Run: `cd frontend/web && bun lint`
Expected: clean

---

### Task 3: Update mobile nav to point to `/browse`

**Files:**
- Modify: `frontend/web/src/components/layout/MobileNav.vue:99`
- Modify: `frontend/web/src/components/layout/Navbar.vue:122-129` (mobile menu search link)

**Step 1: Update MobileNav.vue**

Change line 99:
```typescript
{ to: '/search', label: 'nav.search', icon: SearchIcon },
```
To:
```typescript
{ to: '/browse', label: 'nav.search', icon: SearchIcon },
```

**Step 2: Update Navbar.vue mobile menu**

Replace lines 122-129:
```vue
<router-link
  to="/search"
  class="px-4 py-3 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
  active-class="text-cyan-400 bg-cyan-500/10"
  @click="mobileMenuOpen = false"
>
  {{ $t('nav.search') }}
</router-link>
```
With:
```vue
<router-link
  to="/browse"
  class="px-4 py-3 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
  active-class="text-cyan-400 bg-cyan-500/10"
  @click="mobileMenuOpen = false"
>
  {{ $t('nav.search') }}
</router-link>
```

Actually, this will be a duplicate of the Catalog link. Instead, just **remove** these lines entirely (lines 122-129). The mobile nav already has a "Catalog" link that goes to `/browse`.

---

### Task 4: Remove `/search` route and clean up Browse.vue

**Files:**
- Modify: `frontend/web/src/router/index.ts:28-33`
- Modify: `frontend/web/src/views/Browse.vue`

**Step 1: Remove `/search` route**

Delete lines 28-33 from `router/index.ts`:
```typescript
  {
    path: '/search',
    name: 'search',
    component: Browse,
    meta: { title: 'Search Anime' }
  },
```

**Step 2: Add a redirect for `/search` (backward compat)**

Add after the `/browse` route:
```typescript
  {
    path: '/search',
    redirect: (to) => ({ path: '/browse', query: to.query }),
  },
```

**Step 3: Clean up Browse.vue**

Remove the `isSearchMode` computed and its usage:
- Line 239: Delete `const isSearchMode = computed(() => route.name === 'search')`
- Line 7: Change `{{ isSearchMode ? $t('nav.search') : $t('nav.catalog') }}` to `{{ $t('nav.catalog') }}`
- Line 118: Change `v-if="isSearchMode && !searchQuery && recentSearches.length > 0"` to `v-if="!searchQuery && recentSearches.length > 0"`

This makes recent searches always visible on `/browse` when there's no active query.

---

### Task 5: Wire up real API call for Browse.vue live search dropdown

**Files:**
- Modify: `frontend/web/src/views/Browse.vue:285-301`

**Step 1: Replace mock debouncedLiveSearch with real API call**

Replace lines 285-301:
```typescript
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
```

With:
```typescript
const debouncedLiveSearch = useDebounceFn(async (query: string) => {
  if (query.length < 2) {
    liveResults.value = []
    showLiveResults.value = false
    return
  }

  try {
    const response = await animeApi.search(query, undefined, 5)
    const data = response.data?.data || response.data
    const list = Array.isArray(data) ? data : []
    liveResults.value = list.map((a: Record<string, unknown>) => ({
      id: a.id as string,
      title: (a.name_ru || a.name || a.name_jp || '') as string,
      coverImage: (a.poster_url || '') as string,
      releaseYear: a.year as number | undefined,
      episodes: a.episodes_count as number | undefined,
      rating: a.score as number | undefined,
    }))
    showLiveResults.value = true
  } catch (err) {
    console.error('Live search error:', err)
  }
}, 300)
```

---

### Task 6: Add i18n keys for "View all results"

**Files:**
- Modify: `frontend/web/src/locales/ru.json`
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ja.json`

**Step 1: Add `search.viewAll` key**

In each locale file, add inside the `"search"` section:

**ru.json:**
```json
"viewAll": "Все результаты"
```

**en.json:**
```json
"viewAll": "View all results"
```

**ja.json:**
```json
"viewAll": "すべての結果を見る"
```

---

### Task 7: Lint, build, and commit

**Step 1: Lint**

Run: `cd frontend/web && bun lint`
Expected: clean

**Step 2: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: no errors

**Step 3: Build**

Run: `cd frontend/web && bun run build`
Expected: builds successfully

**Step 4: Commit**

```bash
git add frontend/web/src/api/client.ts \
       frontend/web/src/components/layout/Navbar.vue \
       frontend/web/src/components/layout/MobileNav.vue \
       frontend/web/src/router/index.ts \
       frontend/web/src/views/Browse.vue \
       frontend/web/src/locales/ru.json \
       frontend/web/src/locales/en.json \
       frontend/web/src/locales/ja.json
```

---

### Task 8: Deploy and verify

**Step 1: Deploy**

Run: `make redeploy-web`

**Step 2: Manual verification checklist**

- [ ] Desktop: Click search icon in navbar → input expands, focused
- [ ] Desktop: Type 2+ chars → autocomplete dropdown appears with up to 5 results
- [ ] Desktop: Click a result → navigates to `/anime/:id`
- [ ] Desktop: Press Enter → navigates to `/browse?q=...`
- [ ] Desktop: Press Escape or click outside → search closes
- [ ] Desktop: Arrow keys navigate results, Enter selects
- [ ] Mobile: `/search` redirects to `/browse`
- [ ] Mobile: Bottom nav search icon goes to `/browse`
- [ ] Browse page: Recent searches shown when no query
- [ ] Browse page: Live dropdown calls API (not mock filter)
- [ ] Browse page: "View all results" link works
