# Catalog Pagination Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace "Load More" button in Browse page with numbered page navigation that preserves page in URL for back-button support.

**Architecture:** Frontend-only change. Backend already returns `{ meta: { page, page_size, total_count, total_pages } }`. We modify `useAnime` composable to return meta, then replace the Load More button in Browse.vue with a pagination component that syncs `page` to URL query params.

**Tech Stack:** Vue 3, TypeScript, Tailwind CSS, vue-router

---

### Task 1: Update useAnime composable to return pagination meta

**Files:**
- Modify: `frontend/web/src/composables/useAnime.ts`

**Step 1: Add meta ref and update fetchAnimeList to capture meta**

In `useAnime.ts`, add a `paginationMeta` ref and extract `meta` from the API response in `fetchAnimeList`.

The API response shape is: `{ success: true, data: [...], meta: { page, page_size, total_count, total_pages } }`

Axios wraps this in `response.data`, so `response.data.meta` gives us the meta object.

```typescript
// Add near top of useAnime():
interface PaginationMeta {
  page: number
  page_size: number
  total_count: number
  total_pages: number
}

// Add ref:
const paginationMeta = ref<PaginationMeta | null>(null)
```

Update `fetchAnimeList` (lines 105-119) to capture meta:

```typescript
const fetchAnimeList = async (params?: Record<string, unknown>) => {
  loading.value = true
  error.value = null
  try {
    const response = await animeApi.getAll(params)
    animeList.value = transformAnimeList(response.data)
    paginationMeta.value = response.data?.meta || null
    return animeList.value
  } catch (err: unknown) {
    const e = err as { response?: { data?: { message?: string } } }
    error.value = e.response?.data?.message || 'Failed to fetch anime list'
    throw err
  } finally {
    loading.value = false
  }
}
```

Add `paginationMeta` to the return object (line 252-269):

```typescript
return {
  // ... existing
  paginationMeta,
  // ... rest
}
```

**Step 2: Verify no type errors**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: No errors related to useAnime

**Step 3: Commit**

```bash
git add frontend/web/src/composables/useAnime.ts
git commit -m "feat: expose pagination meta from useAnime composable"
```

---

### Task 2: Create PaginationBar component

**Files:**
- Create: `frontend/web/src/components/ui/PaginationBar.vue`

**Step 1: Create the pagination component**

Create `frontend/web/src/components/ui/PaginationBar.vue`:

```vue
<template>
  <nav v-if="totalPages > 1" class="flex items-center justify-center gap-1 mt-8" aria-label="Pagination">
    <!-- Previous -->
    <button
      :disabled="currentPage <= 1"
      class="pagination-btn"
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
        class="pagination-btn"
        :class="page === currentPage ? 'bg-pink-500/80 text-white' : 'hover:bg-white/10'"
        @click="$emit('update:currentPage', page)"
      >
        {{ page }}
      </button>
    </template>

    <!-- Next -->
    <button
      :disabled="currentPage >= totalPages"
      class="pagination-btn"
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

<style scoped>
.pagination-btn {
  @apply w-9 h-9 flex items-center justify-center rounded-lg text-sm text-white/70 transition-colors;
}
</style>
```

**Step 2: Export from UI index**

Add PaginationBar to the UI barrel export. Find the export file at `frontend/web/src/components/ui/index.ts` and add:

```typescript
export { default as PaginationBar } from './PaginationBar.vue'
```

**Step 3: Verify no type errors**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/web/src/components/ui/PaginationBar.vue frontend/web/src/components/ui/index.ts
git commit -m "feat: add PaginationBar component"
```

---

### Task 3: Replace Load More with pagination in Browse.vue

**Files:**
- Modify: `frontend/web/src/views/Browse.vue`

**Step 1: Update imports and state**

1. Add `PaginationBar` to imports (line 212):
```typescript
import { Input, Badge, Button, Select, GenreFilterPopup, PaginationBar } from '@/components/ui'
```

2. Destructure `paginationMeta` from `useAnime()` (line 224):
```typescript
const { animeList, loading, error, fetchAnimeList, searchAnime, paginationMeta } = useAnime()
```

3. Remove `hasMore` and `loadingMore` refs (lines 232-233) — no longer needed.

4. Add `totalPages` computed (after line 239):
```typescript
const totalPages = computed(() => paginationMeta.value?.total_pages ?? 0)
```

**Step 2: Replace Load More template with PaginationBar**

Replace lines 196-201 (the Load More section):
```html
        <!-- Load More -->
        <div v-if="hasMore && animeList.length > 0" class="flex justify-center mt-8">
          <Button variant="ghost" :loading="loadingMore" @click="loadMore">
            Load More
          </Button>
        </div>
```

With:
```html
        <!-- Pagination -->
        <PaginationBar
          :current-page="currentPage"
          :total-pages="totalPages"
          @update:current-page="goToPage"
        />
```

**Step 3: Replace loadMore with goToPage**

Remove the `loadMore` function (lines 365-370) and add:

```typescript
const goToPage = async (page: number) => {
  currentPage.value = page
  // Sync page to URL
  const query = { ...route.query, page: page > 1 ? String(page) : undefined }
  router.replace({ query })
  await loadAnime()
  // Scroll to top of results
  window.scrollTo({ top: 0, behavior: 'smooth' })
}
```

**Step 4: Remove hasMore calculation from loadAnime**

In `loadAnime()` (line 362), remove:
```typescript
hasMore.value = (results?.length ?? 0) >= 20
```

The function becomes simply:
```typescript
const loadAnime = async () => {
  const params: Record<string, string | number | boolean> = {
    page: currentPage.value,
    sort: sortBy.value,
  }
  if (selectedGenre.value) params.genre = selectedGenre.value
  if (selectedYear.value) params.year = selectedYear.value
  if (selectedStatus.value) params.status = selectedStatus.value
  await fetchAnimeList(params)
}
```

**Step 5: Read page from URL on mount**

In `onMounted` (around line 393-406), add page reading from URL before the initial load:

```typescript
// Read page from URL
if (route.query.page) {
  currentPage.value = parseInt(route.query.page as string, 10) || 1
}
```

Add this before the existing `if (route.query.status)` block (line 393).

**Step 6: Update handleFilter to sync page reset to URL**

Update `handleFilter` (lines 342-345):

```typescript
const handleFilter = async () => {
  currentPage.value = 1
  const query = { ...route.query, page: undefined }
  router.replace({ query })
  await loadAnime()
}
```

**Step 7: Update handleSearch to reset page in URL**

In `handleSearch` (lines 313-330), make sure page is cleared from URL when searching. The existing `router.replace` calls on lines 325 and 328 should include `page: undefined`:

```typescript
router.replace({ query: { ...route.query, q: searchQuery.value, page: undefined } })
// and
router.replace({ query: { ...route.query, q: undefined, page: undefined } })
```

**Step 8: Add watcher for page query param**

Add a watcher for route.query.page changes (after the existing watchers at line 431):

```typescript
watch(() => route.query.page, (newPage) => {
  const page = parseInt(newPage as string, 10) || 1
  if (page !== currentPage.value) {
    currentPage.value = page
    loadAnime()
  }
})
```

**Step 9: Verify no type errors**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: No errors

**Step 10: Commit**

```bash
git add frontend/web/src/views/Browse.vue
git commit -m "feat: replace Load More with page navigation in Browse

Adds numbered pagination bar with URL sync. Page number is stored
in ?page= query param so browser back button preserves position.
Filters (genre, status, year, sort) continue to work alongside pagination."
```

---

### Task 4: Manual testing checklist

**No files to modify — verification only.**

**Step 1: Start dev environment**

Run: `make redeploy-web` (or `cd frontend/web && bun run dev` for local)

**Step 2: Test pagination basics**

1. Open `/browse` — should show page 1 with pagination bar at bottom
2. Click page 2 — URL updates to `?page=2`, grid shows different anime, scrolls to top
3. Click page 5 — URL updates to `?page=5`, ellipsis shows correctly

**Step 3: Test back button (the main feature request)**

1. Browse to page 3 → URL: `/browse?page=3`
2. Click on an anime → navigate to `/anime/xxx`
3. Press browser back → should return to `/browse?page=3` showing page 3

**Step 4: Test filters + pagination**

1. Select genre "Action" → page resets to 1, URL: `?genre=ID`
2. Go to page 2 → URL: `?genre=ID&page=2`
3. Change status filter → page resets to 1
4. Clear filters → page resets to 1

**Step 5: Test search + pagination**

1. Type search query and press Enter → page resets to 1
2. Pagination bar should hide during search results (search uses different API)

**Step 6: Test edge cases**

1. Navigate directly to `/browse?page=999` → should show empty or last page
2. Navigate to `/browse?page=1` → should not show `?page=1` in URL (clean)
3. Pagination bar should not appear if total_pages <= 1
