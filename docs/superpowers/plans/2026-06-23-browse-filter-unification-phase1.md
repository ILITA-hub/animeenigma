# Browse Filter Unification — Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the shared `FilterCheckboxList` (searchable multi-select) and apply it to Browse — add a Studio filter, give Genres a search box, and fix the "Available on" filter to Kodik / English(Dub) / RAW / AnimeEnigma.

**Architecture:** New reusable `components/filters/FilterCheckboxList.vue` (search box + checkbox list) replaces Browse's inline genre list and powers a new Studio section. Catalog gains a `GET /api/studios` options endpoint (`AnimeRepository.ListStudios`, no new DI wiring) and a `StudioIDs` OR-set filter on `Search`. The "Available on" filter is re-pointed to existing populated columns (`has_kodik`, `has_dub`, `has_raw`, `has_video`).

**Tech Stack:** Go 1.x + GORM + chi (catalog, gateway); Vue 3 `<script setup>` + TypeScript + Tailwind v4 + vue-i18n; Vitest; bun.

## Global Constraints

(Every task's requirements implicitly include these — copied from the spec + CLAUDE.md.)

- **Design System gate (build-enforced):** bind to semantic tokens; brand/provider hues `cyan pink orange rose indigo teal lime` are exempt. No off-palette color classes, no raw hex/rgba in `.vue`, no bare `<select>/<input type=checkbox|radio>` (use `Checkbox`/`Input`/`Select` from `@/components/ui`; `bespoke-keep` comment only with justification). Run `bash frontend/web/scripts/design-system-lint.sh` — `ERRORS>0` fails the build. A `PostToolUse` hook runs it automatically on every `.vue`/`.ts` edit.
- **i18n parity:** every new key MUST exist in `en.json`, `ru.json`, AND `ja.json` with matching ICU placeholders, or `i18n-lint.sh` + locale-parity specs fail the build.
- **Type truth:** verify types with a real `bun run build` (NOT `vue-tsc --noEmit`, which false-passes from cache). Import UI types from the `@/components/ui` barrel.
- **lucide:** named imports only (`import { X } from 'lucide-vue-next'`).
- **Pre-flight:** run `/frontend-verify` before finishing.
- **Effort metrics (no days):** this is Phase 1 of `UXΔ = +3 (Better)` · `CDI = 0.12 * 34` (P1 ≈ 13) · `MVQ = Griffin 88%/82%`.
- **Git:** all work in the worktree `/data/animeenigma/.claude/worktrees/browse-list-filters`; commit per task with co-authors:
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```

## File Structure

**Backend (`services/catalog`)**
- Modify `internal/domain/anime.go` — add `StudioIDs []string` to `SearchFilters`.
- Modify `internal/repo/anime.go` — `Search` studio OR-subquery; provider `colsByKey` fix; new `ListStudios` method.
- Modify `internal/handler/catalog.go` — `parseFilters` studio param + provider whitelist fix; new `GetStudios` handler.
- Modify `internal/service/catalog.go` — new `GetStudios` method (via `animeRepo`).
- Modify `internal/transport/router.go` — register `GET /studios`.
- Create `internal/repo/browse_filter_test.go` — repo tests (studio filter, provider columns, ListStudios).

**Backend (`services/gateway`)**
- Modify `internal/transport/router.go` — add `/studios` passthrough.

**Frontend (`frontend/web/src`)**
- Create `components/filters/FilterCheckboxList.vue` + `components/filters/FilterCheckboxList.spec.ts`.
- Modify `composables/useBrowseFilters.ts` — `studios` state; `Provider` type → `kodik|dub|raw|ae`.
- Modify `components/browse/BrowseSidebar.vue` — genres + studios via `FilterCheckboxList`; provider options.
- Modify `views/Browse.vue` — fetch studios; pass `:studios`.
- Modify `api/client.ts` — `getStudios`.
- Modify `locales/{en,ru,ja}.json` — `filters.section.studios`, `filters.searchGenres`, `filters.searchStudios`, provider keys.

---

## Task 1: Studio filter (backend query + param)

**Files:**
- Modify: `services/catalog/internal/domain/anime.go:329-353` (`SearchFilters`)
- Modify: `services/catalog/internal/repo/anime.go` (`Search`, after the genre block ~line 180)
- Modify: `services/catalog/internal/handler/catalog.go:686-696` (`parseFilters`, after the genre block)
- Test: `services/catalog/internal/repo/browse_filter_test.go` (new)

**Interfaces:**
- Produces: `domain.SearchFilters.StudioIDs []string`; `Search` applies `id IN (SELECT anime_id FROM anime_studios WHERE studio_id IN ?)` (OR-set — any selected studio).

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/repo/browse_filter_test.go`:

```go
package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupBrowseFilterTestDB builds an in-memory SQLite with the animes +
// studios + anime_studios tables Search/ListStudios touch. Mirrors
// setupAnimeUpdateTestDB (AutoMigrate can't emit Postgres uuid defaults).
func setupBrowseFilterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE animes (
		id TEXT PRIMARY KEY, name TEXT, name_ru TEXT, name_jp TEXT,
		year INTEGER, season TEXT, status TEXT, kind TEXT, score REAL,
		has_video INTEGER DEFAULT 0, has_dub INTEGER DEFAULT 0,
		has_kodik INTEGER DEFAULT 0, has_animelib INTEGER DEFAULT 0,
		has_raw INTEGER DEFAULT 0, has_english INTEGER DEFAULT 0,
		hidden INTEGER DEFAULT 0, sort_priority INTEGER DEFAULT 0,
		created_at DATETIME, updated_at DATETIME, deleted_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE studios (
		id TEXT PRIMARY KEY, name TEXT, created_at DATETIME, updated_at DATETIME
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE anime_studios (
		anime_id TEXT, studio_id TEXT
	)`).Error)
	return db
}

func seedAnime(t *testing.T, db *gorm.DB, id, name string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO animes (id, name, hidden) VALUES (?, ?, 0)`, id, name).Error)
}

func TestAnimeRepository_Search_StudioFilter_ORSet(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()

	seedAnime(t, db, "a1", "Ufotable Show")
	seedAnime(t, db, "a2", "Madhouse Show")
	seedAnime(t, db, "a3", "No Studio Show")
	require.NoError(t, db.Exec(`INSERT INTO studios (id, name) VALUES ('s-ufo','Ufotable'),('s-mad','Madhouse')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO anime_studios (anime_id, studio_id) VALUES ('a1','s-ufo'),('a2','s-mad')`).Error)

	// OR-set: selecting both studios returns BOTH anime (any match), not the
	// intersection (genre uses AND; studios deliberately use OR).
	got, total, err := r.Search(ctx, domain.SearchFilters{
		StudioIDs: []string{"s-ufo", "s-mad"}, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	ids := map[string]bool{}
	for _, a := range got {
		ids[a.ID] = true
	}
	require.True(t, ids["a1"] && ids["a2"])
	require.False(t, ids["a3"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/repo/ -run TestAnimeRepository_Search_StudioFilter_ORSet -v`
Expected: FAIL — `SearchFilters` has no `StudioIDs` field (compile error).

- [ ] **Step 3a: Add `StudioIDs` to `SearchFilters`**

In `services/catalog/internal/domain/anime.go`, inside `SearchFilters` (after `GenreIDs []string`):

```go
	GenreIDs  []string
	StudioIDs []string
```

- [ ] **Step 3b: Add the studio OR-subquery to `Search`**

In `services/catalog/internal/repo/anime.go`, immediately AFTER the genre block (`if len(filters.GenreIDs) > 0 { ... }`):

```go
	// Studio filter — OR-set (a row passes if it has ANY selected studio).
	// Unlike genres (AND-set via HAVING COUNT), an anime rarely has >1
	// studio, so AND would near-always return empty. Mirrors the anime_studios
	// m2m join (column studio_id).
	if len(filters.StudioIDs) > 0 {
		query = query.Where("id IN (SELECT anime_id FROM anime_studios WHERE studio_id IN ?)", filters.StudioIDs)
	}
```

- [ ] **Step 3c: Parse the `studio` param in `parseFilters`**

In `services/catalog/internal/handler/catalog.go`, after the `genre` parse block (`if genres := query["genre"]; ... }`):

```go
	// Studios — same comma-joined OR multi-value forms as genres (?studio=id1,id2).
	if studios := query["studio"]; len(studios) > 0 {
		for _, s := range studios {
			for _, id := range strings.Split(s, ",") {
				if id = strings.TrimSpace(id); id != "" {
					filters.StudioIDs = append(filters.StudioIDs, id)
				}
			}
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/repo/ -run TestAnimeRepository_Search_StudioFilter_ORSet -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/domain/anime.go services/catalog/internal/repo/anime.go services/catalog/internal/handler/catalog.go services/catalog/internal/repo/browse_filter_test.go
git commit -m "feat(catalog): studio OR-set filter on browse search"
```

---

## Task 2: Available-on provider fix (Kodik / Dub / RAW / AnimeEnigma)

**Files:**
- Modify: `services/catalog/internal/repo/anime.go` (`Search` provider `colsByKey` ~161-176)
- Modify: `services/catalog/internal/handler/catalog.go` (`parseFilters` provider whitelist ~671-684)
- Test: `services/catalog/internal/repo/browse_filter_test.go`

**Interfaces:**
- Produces: provider keys `{kodik, dub, raw, ae}` → columns `{has_kodik, has_dub, has_raw, has_video}`. `animelib`/`english` no longer accepted.

- [ ] **Step 1: Write the failing test**

Append to `services/catalog/internal/repo/browse_filter_test.go`:

```go
func TestAnimeRepository_Search_ProviderColumns(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()

	seedAnime(t, db, "k", "Kodik only")
	seedAnime(t, db, "d", "Dub only")
	seedAnime(t, db, "rw", "Raw only")
	seedAnime(t, db, "ae", "First-party only")
	require.NoError(t, db.Exec(`UPDATE animes SET has_kodik=1 WHERE id='k'`).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET has_dub=1 WHERE id='d'`).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET has_raw=1 WHERE id='rw'`).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET has_video=1 WHERE id='ae'`).Error)

	cases := []struct {
		key, wantID string
	}{
		{"kodik", "k"}, {"dub", "d"}, {"raw", "rw"}, {"ae", "ae"},
	}
	for _, c := range cases {
		got, total, err := r.Search(ctx, domain.SearchFilters{
			Providers: []string{c.key}, Page: 1, PageSize: 20,
		})
		require.NoError(t, err, c.key)
		require.Equal(t, int64(1), total, c.key)
		require.Equal(t, c.wantID, got[0].ID, c.key)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/repo/ -run TestAnimeRepository_Search_ProviderColumns -v`
Expected: FAIL — `dub`/`raw`/`ae` are unmapped (`colsByKey` only has kodik/animelib/english), so `total` is 0.

- [ ] **Step 3a: Fix `colsByKey` in `Search`**

In `services/catalog/internal/repo/anime.go`, replace the `colsByKey` map literal:

```go
		colsByKey := map[string]string{
			"kodik": "has_kodik",
			"dub":   "has_dub",
			"raw":   "has_raw",
			"ae":    "has_video",
		}
```

- [ ] **Step 3b: Fix the provider whitelist in `parseFilters`**

In `services/catalog/internal/handler/catalog.go`, replace the provider `switch` cases:

```go
			switch p {
			case "kodik", "dub", "raw", "ae":
				if !seen[p] {
					filters.Providers = append(filters.Providers, p)
					seen[p] = true
				}
			}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/repo/ -run TestAnimeRepository_Search_ProviderColumns -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/repo/anime.go services/catalog/internal/handler/catalog.go services/catalog/internal/repo/browse_filter_test.go
git commit -m "fix(catalog): available-on filter → kodik/dub/raw/ae(has_video)"
```

---

## Task 3: `GET /api/studios` options endpoint

**Files:**
- Modify: `services/catalog/internal/repo/anime.go` (new `ListStudios`)
- Modify: `services/catalog/internal/service/catalog.go` (new `GetStudios`)
- Modify: `services/catalog/internal/handler/catalog.go` (new `GetStudios` handler)
- Modify: `services/catalog/internal/transport/router.go:233` (register route)
- Test: `services/catalog/internal/repo/browse_filter_test.go`

**Interfaces:**
- Produces: `AnimeRepository.ListStudios(ctx) ([]domain.Studio, error)` — studios with ≥1 anime, ordered by anime count DESC then name ASC; `CatalogService.GetStudios(ctx) ([]domain.Studio, error)`; `GET /api/studios` → `[]domain.Studio` (JSON-wrapped by `httputil.OK`).

- [ ] **Step 1: Write the failing test**

Append to `services/catalog/internal/repo/browse_filter_test.go`:

```go
func TestAnimeRepository_ListStudios_OrderedByCount(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()

	seedAnime(t, db, "a1", "A1")
	seedAnime(t, db, "a2", "A2")
	require.NoError(t, db.Exec(`INSERT INTO studios (id, name) VALUES ('s-big','Big'),('s-small','Small'),('s-empty','Empty')`).Error)
	// Big has 2 anime, Small has 1, Empty has 0 (must be excluded).
	require.NoError(t, db.Exec(`INSERT INTO anime_studios (anime_id, studio_id) VALUES ('a1','s-big'),('a2','s-big'),('a1','s-small')`).Error)

	got, err := r.ListStudios(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2) // Empty excluded (no anime)
	require.Equal(t, "s-big", got[0].ID)   // count 2 first
	require.Equal(t, "s-small", got[1].ID) // count 1 second
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/repo/ -run TestAnimeRepository_ListStudios_OrderedByCount -v`
Expected: FAIL — `r.ListStudios` undefined (compile error).

- [ ] **Step 3a: Add `ListStudios` to `AnimeRepository`**

In `services/catalog/internal/repo/anime.go` (after `Search`):

```go
// ListStudios returns every studio that has at least one anime, ordered by
// anime count DESC then name ASC. The JOIN excludes zero-anime studios.
func (r *AnimeRepository) ListStudios(ctx context.Context) ([]domain.Studio, error) {
	var studios []domain.Studio
	err := r.db.WithContext(ctx).
		Model(&domain.Studio{}).
		Joins("JOIN anime_studios ON anime_studios.studio_id = studios.id").
		Group("studios.id").
		Order("COUNT(anime_studios.anime_id) DESC, studios.name ASC").
		Find(&studios).Error
	if err != nil {
		return nil, fmt.Errorf("list studios: %w", err)
	}
	return studios, nil
}
```

- [ ] **Step 3b: Add `GetStudios` to `CatalogService`**

In `services/catalog/internal/service/catalog.go` (next to `GetGenres`, mirroring its cache pattern; uses the existing `animeRepo` field):

```go
func (s *CatalogService) GetStudios(ctx context.Context) ([]domain.Studio, error) {
	cacheKey := "studios:all"
	var studios []domain.Studio
	if err := s.cache.Get(ctx, cacheKey, &studios); err == nil {
		return studios, nil
	}

	studios, err := s.animeRepo.ListStudios(ctx)
	if err != nil {
		return nil, err
	}

	_ = s.cache.Set(ctx, cacheKey, studios, cache.TTLGenreList)
	return studios, nil
}
```

- [ ] **Step 3c: Add the `GetStudios` handler**

In `services/catalog/internal/handler/catalog.go` (next to `GetGenres`):

```go
// GetStudios handles getting all studios that have anime (browse filter options)
func (h *CatalogHandler) GetStudios(w http.ResponseWriter, r *http.Request) {
	studios, err := h.catalogService.GetStudios(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, studios)
}
```

- [ ] **Step 3d: Register the route**

In `services/catalog/internal/transport/router.go`, immediately after the `/genres` line (`r.Get("/genres", catalogHandler.GetGenres)`):

```go
		r.Get("/studios", catalogHandler.GetStudios)
```

- [ ] **Step 4: Run tests + build**

Run: `cd services/catalog && go test ./internal/repo/ -run TestAnimeRepository_ListStudios -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/repo/anime.go services/catalog/internal/service/catalog.go services/catalog/internal/handler/catalog.go services/catalog/internal/transport/router.go services/catalog/internal/repo/browse_filter_test.go
git commit -m "feat(catalog): GET /api/studios options endpoint"
```

---

## Task 4: Gateway `/api/studios` passthrough

**Files:**
- Modify: `services/gateway/internal/transport/router.go:294` (after the `/genres` passthrough)

**Interfaces:**
- Consumes: catalog `GET /studios` (Task 3).
- Produces: public `GET /api/studios` proxied to catalog (no auth, mirroring `/genres`).

- [ ] **Step 1: Add the passthrough line**

In `services/gateway/internal/transport/router.go`, immediately after `r.HandleFunc("/genres", proxyHandler.ProxyToCatalog)`:

```go
		r.HandleFunc("/studios", proxyHandler.ProxyToCatalog)
```

- [ ] **Step 2: Build to verify**

Run: `cd services/gateway && go build ./...`
Expected: clean build. (Routing is integration-verified by curl in Task 9 — `/genres` has no unit test to mirror.)

- [ ] **Step 3: Commit**

```bash
git add services/gateway/internal/transport/router.go
git commit -m "feat(gateway): proxy /api/studios to catalog"
```

---

## Task 5: `FilterCheckboxList.vue` (shared, searchable)

**Files:**
- Create: `frontend/web/src/components/filters/FilterCheckboxList.vue`
- Test: `frontend/web/src/components/filters/FilterCheckboxList.spec.ts`

**Interfaces:**
- Produces: `<FilterCheckboxList :items="Item[]" :selected="string[]" :searchable :search-placeholder @update:selected>` where `Item = { id: string; label: string; count?: number }`. Toggling emits the new `selected` array. Search filters by `label` (case-insensitive), preserving selection.

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/components/filters/FilterCheckboxList.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FilterCheckboxList from './FilterCheckboxList.vue'

const items = [
  { id: 'a', label: 'Action' },
  { id: 'c', label: 'Comedy' },
  { id: 'd', label: 'Drama', count: 7 },
]

describe('FilterCheckboxList', () => {
  it('renders one row per item with labels', () => {
    const w = mount(FilterCheckboxList, { props: { items, selected: [] } })
    expect(w.text()).toContain('Action')
    expect(w.text()).toContain('Comedy')
    expect(w.text()).toContain('Drama')
  })

  it('shows the count when provided', () => {
    const w = mount(FilterCheckboxList, { props: { items, selected: [] } })
    expect(w.text()).toContain('7')
  })

  // NOTE: `Checkbox` and `Input` are reka-ui-based primitives (Checkbox renders
  // <button role="checkbox">, NOT a native <input>), so interact via the
  // component's update:modelValue event — matching the existing pattern in
  // WatchlistFilters.spec.ts (findAllComponents({ name: 'Checkbox' })).
  it('emits update:selected adding a toggled id', async () => {
    const w = mount(FilterCheckboxList, { props: { items, selected: [] } })
    await w.findAllComponents({ name: 'Checkbox' })[0].vm.$emit('update:modelValue', true)
    expect(w.emitted('update:selected')?.[0]).toEqual([['a']])
  })

  it('emits update:selected removing an already-selected id', async () => {
    const w = mount(FilterCheckboxList, { props: { items, selected: ['a'] } })
    await w.findAllComponents({ name: 'Checkbox' })[0].vm.$emit('update:modelValue', false)
    expect(w.emitted('update:selected')?.[0]).toEqual([[]])
  })

  it('filters visible rows by the search query (case-insensitive)', async () => {
    const w = mount(FilterCheckboxList, { props: { items, selected: [], searchable: true } })
    await w.findComponent({ name: 'Input' }).vm.$emit('update:modelValue', 'com')
    expect(w.text()).toContain('Comedy')
    expect(w.text()).not.toContain('Action')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/filters/FilterCheckboxList.spec.ts`
Expected: FAIL — file `FilterCheckboxList.vue` does not exist.

- [ ] **Step 3: Create the component**

Create `frontend/web/src/components/filters/FilterCheckboxList.vue`:

```vue
<template>
  <div class="space-y-2">
    <Input
      v-if="searchable"
      v-model="query"
      type="text"
      size="sm"
      :placeholder="searchPlaceholder"
      :aria-label="searchPlaceholder"
    />
    <div :class="['overflow-y-auto pr-1 space-y-1', maxHeightClass]">
      <label
        v-for="item in visibleItems"
        :key="item.id"
        class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5"
      >
        <Checkbox
          :model-value="selected.includes(item.id)"
          @update:model-value="(v) => onToggle(item.id, v === true)"
        />
        <span class="flex-1">{{ item.label }}</span>
        <span v-if="item.count != null" class="text-xs text-white/40">{{ item.count }}</span>
      </label>
      <p v-if="!visibleItems.length" class="text-xs text-white/40 py-1">{{ emptyText }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Input, Checkbox } from '@/components/ui'

export interface FilterItem {
  id: string
  label: string
  count?: number
}

const props = withDefaults(
  defineProps<{
    items: FilterItem[]
    selected: string[]
    searchable?: boolean
    searchPlaceholder?: string
    maxHeightClass?: string
    emptyText?: string
  }>(),
  { searchable: true, searchPlaceholder: '', maxHeightClass: 'max-h-48', emptyText: '—' },
)

const emit = defineEmits<{ (e: 'update:selected', value: string[]): void }>()

const query = ref('')

const visibleItems = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.items
  return props.items.filter(i => i.label.toLowerCase().includes(q))
})

function onToggle(id: string, checked: boolean) {
  const set = new Set(props.selected)
  if (checked) set.add(id)
  else set.delete(id)
  emit('update:selected', [...set])
}
</script>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/filters/FilterCheckboxList.spec.ts`
Expected: PASS (all 5)

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/filters/FilterCheckboxList.vue frontend/web/src/components/filters/FilterCheckboxList.spec.ts
git commit -m "feat(filters): shared searchable FilterCheckboxList"
```

---

## Task 6: Browse genres → `FilterCheckboxList` (adds search)

**Files:**
- Modify: `frontend/web/src/components/browse/BrowseSidebar.vue` (genre section template 7-25; script imports + add `genreItems`)
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (`browse.filters.searchGenres`)

**Interfaces:**
- Consumes: `FilterCheckboxList` (Task 5).
- Produces: genre section is searchable; `onGenreToggle` now receives the full id array.

- [ ] **Step 1: Add the i18n key (all three locales)**

In each of `en.json`, `ru.json`, `ja.json`, inside `browse.filters` (add a sibling to `section`):

en.json:
```json
      "searchGenres": "Search genres",
      "searchStudios": "Search studios",
```
ru.json:
```json
      "searchGenres": "Поиск жанров",
      "searchStudios": "Поиск студий",
```
ja.json:
```json
      "searchGenres": "ジャンルを検索",
      "searchStudios": "スタジオを検索",
```

- [ ] **Step 2: Replace the genre section template**

In `BrowseSidebar.vue`, replace lines 7-25 (the genre `FilterSection`) with:

```vue
    <!-- Genres — searchable multi-select -->
    <FilterSection
      :label="$t('browse.filters.section.genres')"
      :count="filters.genres.value.length"
    >
      <FilterCheckboxList
        :items="genreItems"
        :selected="filters.genres.value"
        :search-placeholder="$t('browse.filters.searchGenres')"
        @update:selected="onGenresChange"
      />
    </FilterSection>
```

- [ ] **Step 3: Update the script — import, items, handler**

In `BrowseSidebar.vue` script, add the import after `import FilterSection from './FilterSection.vue'`:

```ts
import FilterCheckboxList from '@/components/filters/FilterCheckboxList.vue'
```

Add a `genreItems` computed (after `localizedGenre`):

```ts
const genreItems = computed(() => props.genres.map(g => ({ id: g.id, label: localizedGenre(g) })))
```

Replace `onGenreToggle` with an array-based handler:

```ts
function onGenresChange(ids: string[]) {
  props.filters.genres.value = ids
  props.filters.writeUrl()
}
```

(`computed` is already imported at line 146.)

- [ ] **Step 4: Verify (DS-lint fires on save) + build + tests**

Run: `cd frontend/web && bash scripts/design-system-lint.sh && bunx vitest run src/components/browse/ && bun run build`
Expected: DS-lint clean; vitest pass; build OK.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/browse/BrowseSidebar.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(browse): genres use shared FilterCheckboxList with search"
```

---

## Task 7: Studio filter — frontend wiring

**Files:**
- Modify: `frontend/web/src/api/client.ts:389` (`getStudios`)
- Modify: `frontend/web/src/composables/useBrowseFilters.ts` (`studios` ref + URL + apiParams + reset + activeCount)
- Modify: `frontend/web/src/views/Browse.vue` (fetch studios; pass `:studios`)
- Modify: `frontend/web/src/components/browse/BrowseSidebar.vue` (studio section + prop)
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (`browse.filters.section.studios`)

**Interfaces:**
- Consumes: `GET /api/studios` (Task 3/4), `FilterCheckboxList` (Task 5).
- Produces: `filters.studios` ref (URL param `studio`, api param `studio`); `BrowseSidebar` accepts `studios: Studio[]` prop.

- [ ] **Step 1: Add `getStudios` to the API client**

In `frontend/web/src/api/client.ts`, after `getGenres: () => apiClient.get('/genres'),`:

```ts
  getStudios: () => apiClient.get('/studios'),
```

- [ ] **Step 2: Add `studios` to `useBrowseFilters`**

In `composables/useBrowseFilters.ts`:

(a) declare the ref (after `const genres = ref<string[]>([])` line 33):
```ts
  const studios = ref<string[]>([])
```

(b) read from URL — in `readUrl()`, after the genres block (line 60):
```ts
    const newStudios = ((qry.studio as string) || '')
      .split(',')
      .map(s => s.trim())
      .filter(Boolean)
    if (newStudios.join(',') !== studios.value.join(',')) {
      studios.value = newStudios
    }
```

(c) write to URL — in `writeUrl()`, after the `next.genre` line (line 90):
```ts
    next.studio = studios.value.length ? studios.value.join(',') : undefined
```

(d) apiParams — in the computed, after the `genre` line (line 109):
```ts
    if (studios.value.length) p.studio = studios.value.join(',')
```

(e) activeCount — after the `genres.value.length` line (line 126):
```ts
    if (studios.value.length) n++
```

(f) reset — after `genres.value = []` (line 138):
```ts
    studios.value = []
```

(g) return — add `studios,` to the returned object (after `genres,` line 168):
```ts
    studios,
```

- [ ] **Step 3: Add the studio `Studio` type + fetch in Browse.vue**

In `views/Browse.vue`, add a `studios` ref + `browseStudios` computed next to `genres` (after line 312):

```ts
const studios = ref<{ id: string; name: string }[]>([])
const browseStudios = computed(() => studios.value.map(s => ({ id: s.id, label: s.name })))
```

In the `onMounted` block, add a studio fetch alongside `genrePromise` and include it in `Promise.all`:

```ts
  const studioPromise = animeApi
    .getStudios()
    .then(response => {
      studios.value = response.data?.data || response.data || []
    })
    .catch(err => {
      console.error('Failed to load studios:', err)
    })

  const watchlistPromise = fetchWatchlistMap()
  await Promise.all([genrePromise, studioPromise, watchlistPromise])
```

Pass `:studios` to BOTH `<BrowseSidebar>` instances (desktop ~line 37 and mobile ~line 190):

```vue
          <BrowseSidebar :genres="browseGenres" :studios="browseStudios" :filters="filters" />
```

- [ ] **Step 4: Add the studio section + prop in BrowseSidebar.vue**

Add `studios` to the props block (after `genres: Genre[]`):

```ts
const props = defineProps<{
  genres: Genre[]
  studios: { id: string; label: string }[]
  filters: ReturnType<typeof useBrowseFilters>
}>()
```

Add the studio `FilterSection` in the template immediately after the genre section:

```vue
    <!-- Studios — searchable multi-select -->
    <FilterSection
      :label="$t('browse.filters.section.studios')"
      :count="filters.studios.value.length"
    >
      <FilterCheckboxList
        :items="studios"
        :selected="filters.studios.value"
        :search-placeholder="$t('browse.filters.searchStudios')"
        @update:selected="onStudiosChange"
      />
    </FilterSection>
```

Add the handler (next to `onGenresChange`):

```ts
function onStudiosChange(ids: string[]) {
  props.filters.studios.value = ids
  props.filters.writeUrl()
}
```

- [ ] **Step 5: Add the `studios` section label (all three locales)**

In each locale, inside `browse.filters.section`, add `studios` after `genres`:

en: `"studios": "Studios",` · ru: `"studios": "Студии",` · ja: `"studios": "スタジオ",`

- [ ] **Step 6: Verify + commit**

Run: `cd frontend/web && bash scripts/design-system-lint.sh && bash scripts/i18n-lint.sh && bunx vitest run src/components/browse/ src/locales/__tests__ && bun run build`
Expected: all pass.

```bash
git add frontend/web/src/api/client.ts frontend/web/src/composables/useBrowseFilters.ts frontend/web/src/views/Browse.vue frontend/web/src/components/browse/BrowseSidebar.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(browse): studio filter (searchable) wired end-to-end"
```

---

## Task 8: Available-on filter — frontend options

**Files:**
- Modify: `frontend/web/src/composables/useBrowseFilters.ts:16,20` (`Provider` type + values)
- Modify: `frontend/web/src/components/browse/BrowseSidebar.vue:206-224` (`providerOptions`)
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (`browse.filters.provider`)

**Interfaces:**
- Produces: `Provider = 'kodik' | 'dub' | 'raw' | 'ae'`; option labels Kodik (RU) / English (Dub) / RAW (JP) / AnimeEnigma.

- [ ] **Step 1: Update the `Provider` type + whitelist**

In `useBrowseFilters.ts`, replace line 16 and line 20:

```ts
export type Provider = 'kodik' | 'dub' | 'raw' | 'ae'
```
```ts
const PROVIDER_VALUES: Provider[] = ['kodik', 'dub', 'raw', 'ae']
```

- [ ] **Step 2: Update `providerOptions` in BrowseSidebar.vue**

Replace the `providerOptions` computed (lines 206-224) with:

```ts
// Per-provider brand/identity accents (DS brand-exempt hues + a semantic token).
const providerOptions = computed<{ value: Provider; label: string; accent: string }[]>(() => [
  {
    value: 'kodik',
    label: t('browse.filters.provider.kodik'),
    accent: 'text-cyan-500 focus:ring-cyan-500',
  },
  {
    value: 'dub',
    // English-dub availability (has_dub) — semantic success token (EN surface).
    label: t('browse.filters.provider.dub'),
    accent: 'text-success focus:ring-success',
  },
  {
    value: 'raw',
    // Raw (JP) provider-identity hue (rose) — brand-exempt decorative accent.
    label: t('browse.filters.provider.raw'),
    accent: 'text-rose-500 focus:ring-rose-500',
  },
  {
    value: 'ae',
    // First-party AnimeEnigma (has_video) — indigo brand-exempt accent.
    label: t('browse.filters.provider.ae'),
    accent: 'text-indigo-400 focus:ring-indigo-400',
  },
])
```

- [ ] **Step 3: Replace the `provider` i18n block (all three locales)**

Replace `browse.filters.provider` in each file:

en.json:
```json
      "provider": {
        "kodik": "Kodik (RU)",
        "dub": "English (Dub)",
        "raw": "RAW (JP)",
        "ae": "AnimeEnigma"
      },
```
ru.json:
```json
      "provider": {
        "kodik": "Kodik (RU)",
        "dub": "Английский (озвучка)",
        "raw": "RAW (JP)",
        "ae": "AnimeEnigma"
      },
```
ja.json:
```json
      "provider": {
        "kodik": "Kodik (RU)",
        "dub": "英語（吹替）",
        "raw": "RAW (JP)",
        "ae": "AnimeEnigma"
      },
```

- [ ] **Step 4: Verify + commit**

Run: `cd frontend/web && bash scripts/design-system-lint.sh && bash scripts/i18n-lint.sh && bunx vitest run src/components/browse/ src/locales/__tests__ && bun run build`
Expected: all pass. (DS-lint: `cyan/rose/indigo` are brand-exempt; `text-success` is semantic — all clean.)

```bash
git add frontend/web/src/composables/useBrowseFilters.ts frontend/web/src/components/browse/BrowseSidebar.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "fix(browse): available-on options → kodik/dub/raw/ae"
```

---

## Task 9: Full verification + deploy

**Files:** none (verification only)

- [ ] **Step 1: Backend test + build**

Run: `cd services/catalog && go test ./... -race && go build ./... && cd ../gateway && go build ./...`
Expected: all pass.

- [ ] **Step 2: Frontend pre-flight**

Run `/frontend-verify` (DS-lint, i18n en/ru/ja parity, `bun run build`, vitest). All gates green.

- [ ] **Step 3: Deploy + smoke**

Run `/animeenigma-after-update` (redeploys `catalog`, `gateway`, `web`; health checks; changelog; commit; push). Then smoke the new endpoint + filter against a real anime:

```bash
curl -s http://localhost:8000/api/studios | head -c 300        # expect JSON list of studios
curl -s "http://localhost:8000/api/anime?providers=ae&page_size=3" | head -c 300   # first-party only
curl -s "http://localhost:8000/api/anime?providers=dub&page_size=3" | head -c 300  # dub only
```
Expected: `/api/studios` returns studios ordered by count; provider filters return rows (or an empty list if no anime currently has that flag — a data state, not a bug).

- [ ] **Step 4: Cascade smoke (opt-in)**

The genre/studio search box + provider list are not cascade-sensitive (utility classes only), so a Chrome smoke is optional. Offer one only if the owner wants a visual check of the new Studios section.

---

## Self-Review

**Spec coverage:** §3 `FilterCheckboxList` → Task 5; §6 studio filter + `/api/studios` → Tasks 1,3,4,7; §5 available-on → Tasks 2,8; §7 genre search → Task 6; §9 i18n parity → Tasks 6,7,8 (en/ru/ja each). Phase-1-only: kind unification + `FilterYearRange` + List migration are **Phase 2** (out of scope here, per spec §12).

**Placeholder scan:** none — every step carries real code/commands.

**Type consistency:** `Provider = kodik|dub|raw|ae` matches backend `colsByKey` keys (Task 2) and whitelist (Task 2). `FilterItem {id,label,count?}` is consumed identically by genres (Task 6) and studios (Task 7). `studios` ref name is consistent across `useBrowseFilters`, `BrowseSidebar`, `Browse.vue`. `getStudios`/`ListStudios`/`GetStudios` chain aligns Tasks 3→7.

**Known data caveat:** provider filters return rows only where the `has_*` column is populated; `has_dub`/`has_video` are lazily/admin-set, so early results may be sparse — verified as a data state in Task 9, not a code bug.
