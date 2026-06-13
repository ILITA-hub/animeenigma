# Watchlist Bulk Edit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let profile owners select multiple anime on the current watchlist page (table or grid) and bulk-change their status or bulk-remove them in one action.

**Architecture:** One authenticated `POST /users/watchlist/bulk` endpoint whose service loops over the EXISTING per-item `UpdateListEntry`/`DeleteListEntry` (preserving all side effects: completed_at, episodes, gacha, activity, rewatch). Frontend adds an owner-only selection mode (checkboxes in table + grid), a floating bulk-action bar, and Profile.vue wiring. Selection is current-page-scoped and cleared on page/filter change.

**Tech Stack:** Go (chi, GORM), Vue 3 + TS, reka-ui primitives, Vitest, vue-i18n. Backend tests use sqlite-in-memory + testify (matching existing repo/service tests).

**Spec:** `docs/superpowers/specs/2026-06-13-watchlist-bulk-edit-design.md`

---

## File Structure

**Backend (player service):**
- `services/player/internal/domain/watch.go` — MODIFY: `BulkUpdateRequest` type + `ValidListStatuses` map.
- `services/player/internal/service/list.go` — MODIFY: `BulkUpdate` method.
- `services/player/internal/service/list_bulk_test.go` — CREATE: bulk service tests.
- `services/player/internal/handler/list.go` — MODIFY: `BulkUpdateList` handler + parse.
- `services/player/internal/transport/router.go` — MODIFY: register route.

**Frontend:**
- `frontend/web/src/api/client.ts` — MODIFY: `bulkWatchlist`.
- `frontend/web/src/components/profile/WatchlistBulkBar.vue` — CREATE.
- `frontend/web/src/components/profile/WatchlistBulkBar.spec.ts` — CREATE.
- `frontend/web/src/components/profile/WatchlistRow.vue` — MODIFY: selectable checkbox.
- `frontend/web/src/views/Profile.vue` — MODIFY: selection state, toggle, table/grid checkboxes, bulk handlers, clear-on-change, render bar.
- `frontend/web/src/locales/en.json` + `ru.json` — MODIFY: `profile.bulk.*`.

---

## Task 1: Backend domain — bulk request type + valid statuses

**Files:**
- Modify: `services/player/internal/domain/watch.go`

- [ ] **Step 1: Append the type + status set**

Add to the end of `services/player/internal/domain/watch.go`:

```go
// ValidListStatuses is the whitelist of watchlist status values accepted by
// bulk operations. Mirrors the per-status filter pills in the profile UI.
var ValidListStatuses = map[string]bool{
	"watching": true, "plan_to_watch": true, "completed": true,
	"on_hold": true, "dropped": true,
}

// BulkUpdateRequest is the body of POST /users/watchlist/bulk.
// Action is "set_status" (requires Status) or "remove".
type BulkUpdateRequest struct {
	AnimeIDs []string `json:"anime_ids"`
	Action   string   `json:"action"`
	Status   string   `json:"status,omitempty"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /data/animeenigma/services/player && go build ./...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
cd /data/animeenigma
git add services/player/internal/domain/watch.go
git commit -m "feat(player): bulk watchlist request type + valid-status set"
git push
```
Verify `main -> main`.

---

## Task 2: Backend service — `BulkUpdate`

**Files:**
- Modify: `services/player/internal/service/list.go`
- Test: `services/player/internal/service/list_bulk_test.go`

- [ ] **Step 1: Add the method**

Append to `services/player/internal/service/list.go`:

```go
// MaxBulkAnimeIDs bounds a single bulk operation (defensive; the UI selects
// at most one page of entries).
const MaxBulkAnimeIDs = 200

// BulkUpdate applies a status change or removal to many of the user's own
// watchlist entries in one call. It loops over the existing per-item service
// methods so every side effect (completed_at/episodes auto-set, gacha credit,
// activity event, rewatch-finale bump) stays identical to single-item edits.
// Per-item failures are logged and counted into `failed`; only a request-level
// validation problem returns a non-nil error.
func (s *ListService) BulkUpdate(ctx context.Context, userID, username string, req *domain.BulkUpdateRequest) (updated int, failed int, err error) {
	if req == nil || len(req.AnimeIDs) == 0 {
		return 0, 0, errors.New(errors.CodeInvalidArgument, "anime_ids is required")
	}
	if len(req.AnimeIDs) > MaxBulkAnimeIDs {
		return 0, 0, errors.New(errors.CodeInvalidArgument, "too many anime_ids")
	}

	switch req.Action {
	case "set_status":
		if !domain.ValidListStatuses[req.Status] {
			return 0, 0, errors.New(errors.CodeInvalidArgument, "invalid status")
		}
		for _, id := range req.AnimeIDs {
			if _, e := s.UpdateListEntry(ctx, userID, username, &domain.UpdateListRequest{AnimeID: id, Status: req.Status}); e != nil {
				failed++
				s.log.Errorw("bulk set_status: item failed", "user_id", userID, "anime_id", id, "error", e)
				continue
			}
			updated++
		}
	case "remove":
		for _, id := range req.AnimeIDs {
			if e := s.DeleteListEntry(ctx, userID, id); e != nil {
				failed++
				s.log.Errorw("bulk remove: item failed", "user_id", userID, "anime_id", id, "error", e)
				continue
			}
			updated++
		}
	default:
		return 0, 0, errors.New(errors.CodeInvalidArgument, "invalid action")
	}
	return updated, failed, nil
}
```

IMPORTANT: confirm the `errors.New` constructor signature in this package before assuming — grep `errors.New(` in `services/player/internal/service/list.go`. The codebase uses `libs/errors`; match the EXACT shape used elsewhere (it may be `errors.New(errors.CodeInvalidArgument, "msg")` or `errors.BadRequest("msg")`). Adapt these four error returns to the existing convention and report what you found. Also confirm `s.log` exists on `ListService` (it is used by `UpdateListEntry`).

- [ ] **Step 2: Write the failing tests**

Create `services/player/internal/service/list_bulk_test.go`. First inspect an existing service test (e.g. `services/player/internal/service/list_rewatch_stats_test.go`) to copy the EXACT `ListService` construction/harness (constructor name, fake repos, sqlite setup). Then mirror it. The test must cover: set_status updates all + sets completed_at on completion; remove deletes all; validation (empty ids, bad action, set_status with bad status); per-item failure counted.

```go
package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBulkTestService builds a ListService backed by the same in-memory harness
// the other service tests use. ADAPT the body to match the real constructor and
// fakes found in the existing *_test.go files in this package.
func newBulkTestService(t *testing.T) (*ListService, string) {
	t.Helper()
	svc, userID := newListTestService(t) // reuse existing helper if present
	return svc, userID
}

func TestBulkUpdate_SetStatus(t *testing.T) {
	svc, userID := newBulkTestService(t)
	// seed two entries for userID with status "watching" (use the same seeding
	// path the sibling tests use, e.g. svc.UpdateListEntry add).
	_, err := svc.UpdateListEntry(context.Background(), userID, "tester", &domain.UpdateListRequest{AnimeID: "a1", Status: "watching"})
	require.NoError(t, err)
	_, err = svc.UpdateListEntry(context.Background(), userID, "tester", &domain.UpdateListRequest{AnimeID: "a2", Status: "watching"})
	require.NoError(t, err)

	updated, failed, err := svc.BulkUpdate(context.Background(), userID, "tester", &domain.BulkUpdateRequest{
		AnimeIDs: []string{"a1", "a2"}, Action: "set_status", Status: "completed",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, updated)
	assert.Equal(t, 0, failed)

	e1, _ := svc.GetUserAnimeEntry(context.Background(), userID, "a1")
	require.NotNil(t, e1)
	assert.Equal(t, "completed", e1.Status)
	assert.NotNil(t, e1.CompletedAt)
}

func TestBulkUpdate_Remove(t *testing.T) {
	svc, userID := newBulkTestService(t)
	_, err := svc.UpdateListEntry(context.Background(), userID, "tester", &domain.UpdateListRequest{AnimeID: "a1", Status: "watching"})
	require.NoError(t, err)

	updated, _, err := svc.BulkUpdate(context.Background(), userID, "tester", &domain.BulkUpdateRequest{
		AnimeIDs: []string{"a1"}, Action: "remove",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, updated)

	e1, _ := svc.GetUserAnimeEntry(context.Background(), userID, "a1")
	assert.Nil(t, e1)
}

func TestBulkUpdate_Validation(t *testing.T) {
	svc, userID := newBulkTestService(t)
	_, _, err := svc.BulkUpdate(context.Background(), userID, "tester", &domain.BulkUpdateRequest{AnimeIDs: []string{}, Action: "remove"})
	assert.Error(t, err)
	_, _, err = svc.BulkUpdate(context.Background(), userID, "tester", &domain.BulkUpdateRequest{AnimeIDs: []string{"a1"}, Action: "nope"})
	assert.Error(t, err)
	_, _, err = svc.BulkUpdate(context.Background(), userID, "tester", &domain.BulkUpdateRequest{AnimeIDs: []string{"a1"}, Action: "set_status", Status: "bogus"})
	assert.Error(t, err)
}
```

NOTE: the exact signature of `UpdateListEntry` returns `(*AnimeListEntry, error)` (two values). The placeholder line above is illustrative — when adapting, call it correctly: `_, err := svc.UpdateListEntry(...)`. Remove any placeholder lines. The KEY assertions that must hold: bulk set_status flips status + sets CompletedAt; remove deletes; validation errors on empty/bad-action/bad-status. If the existing harness has no `newListTestService` helper, build the `ListService` the same way the sibling test file does (same repos/fakes) inside `newBulkTestService`.

- [ ] **Step 3: Run the tests**

Run: `cd /data/animeenigma/services/player && go test ./internal/service/ -run 'TestBulkUpdate' -count=1 -v`
Expected: all PASS. (Iterate on the harness until green — do not change the assertions' intent.)

- [ ] **Step 4: Run the full service package (no regressions)**

Run: `go test ./internal/service/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/player/internal/service/list.go services/player/internal/service/list_bulk_test.go
git commit -m "feat(player): BulkUpdate watchlist service (status/remove, side-effect preserving)"
git push
```

---

## Task 3: Backend handler + route

**Files:**
- Modify: `services/player/internal/handler/list.go`
- Modify: `services/player/internal/transport/router.go`

- [ ] **Step 1: Add the handler**

Append to `services/player/internal/handler/list.go`:

```go
// BulkUpdateList applies a bulk status change or removal to the authenticated
// user's own watchlist entries. Returns {updated, failed}.
func (h *ListHandler) BulkUpdateList(w http.ResponseWriter, r *http.Request) {
	var req domain.BulkUpdateRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	updated, failed, err := h.listService.BulkUpdate(r.Context(), claims.UserID, claims.Username, &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]int{"updated": updated, "failed": failed})
}
```

IMPORTANT: confirm `httputil.Bind`, `httputil.OK`, `httputil.Unauthorized`, `httputil.Error` are the exact helpers used by the sibling handlers in this file (they are — `UpdateListEntry` uses `httputil.Bind` + `httputil.OK`). Match them.

- [ ] **Step 2: Register the route**

In `services/player/internal/transport/router.go`, in the authenticated watchlist group, after the line `r.Get("/watchlist/facets", listHandler.GetWatchlistFacets)` (added in the prior feature), add:

```go
			r.Post("/watchlist/bulk", listHandler.BulkUpdateList)
```
`"/watchlist/bulk"` is a static path registered before the `/watchlist/{animeId}` wildcard — no collision. Match surrounding indentation.

- [ ] **Step 3: Build + vet + tests**

Run:
```bash
cd /data/animeenigma/services/player
go build ./... && go vet ./... && go test ./internal/service/ ./internal/repo/ -count=1
```
Expected: clean build, vet clean, tests PASS.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add services/player/internal/handler/list.go services/player/internal/transport/router.go
git commit -m "feat(player): POST /users/watchlist/bulk handler + route"
git push
```

---

## Task 4: Frontend API client

**Files:**
- Modify: `frontend/web/src/api/client.ts`

- [ ] **Step 1: Add the method**

In `frontend/web/src/api/client.ts`, inside the `userApi` object near `getWatchlistFacets`, add:

```ts
  bulkWatchlist: (body: { anime_ids: string[]; action: 'set_status' | 'remove'; status?: string }) =>
    apiClient.post('/users/watchlist/bulk', body),
```
Match surrounding indentation + trailing-comma style.

- [ ] **Step 2: Type-check**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit`
Expected: no new errors.

- [ ] **Step 3: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/api/client.ts
git commit -m "feat(web): bulkWatchlist API client method"
git push
```

---

## Task 5: `WatchlistBulkBar.vue` + spec

**Files:**
- Create: `frontend/web/src/components/profile/WatchlistBulkBar.vue`
- Test: `frontend/web/src/components/profile/WatchlistBulkBar.spec.ts`

- [ ] **Step 1: Create the component**

`frontend/web/src/components/profile/WatchlistBulkBar.vue`:

```vue
<template>
  <div class="fixed inset-x-0 bottom-4 z-50 flex justify-center px-4 pointer-events-none">
    <div class="pointer-events-auto flex items-center gap-3 rounded-full border border-white/10 bg-popover/95 backdrop-blur-xl px-4 py-2 shadow-xl shadow-black/30">
      <span class="text-sm font-medium text-white whitespace-nowrap">{{ $t('profile.bulk.selected', { n: count }) }}</span>

      <div class="w-40">
        <Select
          :model-value="''"
          :options="statusSelectOptions"
          size="sm"
          :placeholder="$t('profile.bulk.setStatus')"
          @update:model-value="onStatus"
        />
      </div>

      <Button variant="ghost" size="sm" class="text-destructive hover:text-destructive" @click="emit('remove')">
        <Trash2 class="size-4" />
        <span>{{ $t('profile.bulk.remove') }}</span>
      </Button>

      <Button variant="ghost" size="sm" class="text-muted-foreground" @click="emit('clear')">
        {{ $t('profile.bulk.clear') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Trash2 } from 'lucide-vue-next'
import Button from '@/components/ui/Button.vue'
import Select from '@/components/ui/Select.vue'
import type { SelectOption } from '@/components/ui/Select.vue'

const props = defineProps<{
  count: number
  statusOptions: SelectOption[]
}>()

const emit = defineEmits<{
  'set-status': [status: string]
  remove: []
  clear: []
}>()

// reka-ui Select forbids empty-string item values; the placeholder handles the
// "no choice yet" state, so we just pass the real status options through.
const statusSelectOptions = computed(() => props.statusOptions)

function onStatus(v: string | number) {
  const s = String(v)
  if (s) emit('set-status', s)
}
</script>
```

IMPORTANT: verify `SelectOption` is exported from `@/components/ui/Select.vue` (Profile.vue imports `SelectOption` — confirm the exact import path/name and match it; if it lives elsewhere, e.g. a types file, import from there). Verify `Select` supports a `placeholder` prop; if not, drop the prop and rely on the options. Confirm `text-destructive` is an allowed semantic token (it is — design-system rule 1 lists it as a migration target, i.e. allowed).

- [ ] **Step 2: Create the spec**

`frontend/web/src/components/profile/WatchlistBulkBar.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import WatchlistBulkBar from './WatchlistBulkBar.vue'

const statusOptions = [
  { value: 'watching', label: 'Watching' },
  { value: 'completed', label: 'Completed' },
]

function mountBar(count = 3) {
  return mount(WatchlistBulkBar, {
    props: { count, statusOptions },
    global: { mocks: { $t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k) } },
  })
}

describe('WatchlistBulkBar', () => {
  it('renders the selected count', () => {
    const wrapper = mountBar(5)
    expect(wrapper.text()).toContain('profile.bulk.selected')
    expect(wrapper.text()).toContain('5')
  })

  it('emits remove when the remove button is clicked', async () => {
    const wrapper = mountBar()
    const btn = wrapper.findAll('button').find((b) => b.text().includes('profile.bulk.remove'))
    await btn!.trigger('click')
    expect(wrapper.emitted('remove')).toBeTruthy()
  })

  it('emits clear when the clear button is clicked', async () => {
    const wrapper = mountBar()
    const btn = wrapper.findAll('button').find((b) => b.text().includes('profile.bulk.clear'))
    await btn!.trigger('click')
    expect(wrapper.emitted('clear')).toBeTruthy()
  })

  it('emits set-status with the chosen status', async () => {
    const wrapper = mountBar()
    const select = wrapper.findComponent({ name: 'Select' })
    await select.vm.$emit('update:modelValue', 'completed')
    expect(wrapper.emitted('set-status')?.[0]).toEqual(['completed'])
  })

  it('does not emit set-status for an empty value', async () => {
    const wrapper = mountBar()
    const select = wrapper.findComponent({ name: 'Select' })
    await select.vm.$emit('update:modelValue', '')
    expect(wrapper.emitted('set-status')).toBeFalsy()
  })
})
```

If `findComponent({ name: 'Select' })` fails (component not named "Select"), target it via `wrapper.findComponent(Select)` by importing the component, and adapt. Keep the 5 assertions' intent.

- [ ] **Step 3: Run spec + lint**

Run:
```bash
cd /data/animeenigma/frontend/web
bunx vitest run src/components/profile/WatchlistBulkBar.spec.ts
bash scripts/design-system-lint.sh
```
Expected: 5 tests PASS; design-system-lint ERRORS=0.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/profile/WatchlistBulkBar.vue frontend/web/src/components/profile/WatchlistBulkBar.spec.ts
git commit -m "feat(web): WatchlistBulkBar floating bulk-action bar"
git push
```

---

## Task 6: `WatchlistRow.vue` — selectable checkbox

**Files:**
- Modify: `frontend/web/src/components/profile/WatchlistRow.vue`

- [ ] **Step 1: Add props + emit**

In `WatchlistRow.vue`, extend `defineProps` and `defineEmits`:

```ts
const props = defineProps<{
  entry: WatchlistRowEntry
  /** Absolute 1-based position (pagination offset applied by the parent). */
  index: number
  isOwn: boolean
  statusOptions: SelectOption[]
  selectable?: boolean
  selected?: boolean
}>()

const emit = defineEmits<{
  editScore: [value: string]
  updateEpisodes: [episodes: number]
  updateRewatchCount: [count: number]
  updateDate: [field: 'started_at' | 'completed_at', value: string]
  updateStatus: [status: string]
  remove: []
  toggleSelect: []
}>()
```

- [ ] **Step 2: Render the checkbox**

In the row's template, add a leading checkbox before the existing first cell (the index/title). Find the row's outermost content container and insert, as the first child of the row's flex layout:

```vue
        <label v-if="selectable" class="flex items-center pr-2 cursor-pointer self-center" @click.stop>
          <Checkbox :model-value="!!selected" @update:model-value="() => emit('toggleSelect')" />
        </label>
```

Add the import in the script if not present: `import Checkbox from '@/components/ui/Checkbox.vue'`. Inspect the current top-level template structure of the row first and place the checkbox so it sits at the start of the row on one line with the title (align with `self-center`/`items-center` to match the row's vertical alignment). Do not alter existing cells.

- [ ] **Step 3: Type-check + run existing row spec**

Run:
```bash
cd /data/animeenigma/frontend/web
bunx tsc --noEmit
bunx vitest run src/components/profile/WatchlistRow.spec.ts
```
Expected: no new type errors; existing row spec still PASSES (the new props are optional, so existing usage is unaffected).

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/components/profile/WatchlistRow.vue
git commit -m "feat(web): optional selection checkbox in WatchlistRow"
git push
```

---

## Task 7: Profile.vue — selection state, table/grid checkboxes, bulk wiring

**Files:**
- Modify: `frontend/web/src/views/Profile.vue`

- [ ] **Step 1: Imports + state**

Add to the lucide import line in `Profile.vue` (the one with `SlidersHorizontal, ChevronDown`): append `, CheckSquare`.
Add component + checkbox imports near the other component imports:
```ts
import WatchlistBulkBar from '@/components/profile/WatchlistBulkBar.vue'
```
(Checkbox is already available globally / or import `import Checkbox from '@/components/ui/Checkbox.vue'` if not.)

Add state near `filterState`:
```ts
const selectionMode = ref(false)
const selectedIds = ref<Set<string>>(new Set())

const allOnPageSelected = computed(() =>
  watchlist.value.length > 0 && watchlist.value.every((a) => selectedIds.value.has(a.anime_id)),
)

function toggleSelect(animeId: string) {
  const next = new Set(selectedIds.value)
  if (next.has(animeId)) next.delete(animeId)
  else next.add(animeId)
  selectedIds.value = next
}

function toggleSelectAllOnPage() {
  if (allOnPageSelected.value) {
    selectedIds.value = new Set()
  } else {
    selectedIds.value = new Set(watchlist.value.map((a) => a.anime_id))
  }
}

function exitSelectionMode() {
  selectionMode.value = false
  selectedIds.value = new Set()
}
```

- [ ] **Step 2: Clear selection on page/filter change**

Find the existing `watch(filterState, ...)` and the `watch(watchlistFilter, ...)` and the page-fetch path. Add a watcher that clears selection whenever the visible page identity changes:
```ts
watch([watchlistPage, watchlistFilter, searchQuery, filterState], () => {
  selectedIds.value = new Set()
}, { deep: true })
```
Place it near the other watchers.

- [ ] **Step 3: Selection-mode toggle button (controls row)**

In the controls row, next to the Filters trigger Button (added earlier), add (own profile only):
```vue
                <Button
                  v-if="isOwnProfile"
                  variant="ghost"
                  size="sm"
                  class="gap-1.5"
                  :class="selectionMode ? 'text-cyan-400' : 'text-white/70 hover:text-white'"
                  :aria-pressed="selectionMode"
                  @click="selectionMode ? exitSelectionMode() : (selectionMode = true)"
                >
                  <CheckSquare class="size-4" />
                  <span>{{ $t('profile.bulk.select') }}</span>
                </Button>
```

- [ ] **Step 4: Table — header select-all + per-row checkbox**

Above the `<div v-if="viewMode === 'table'" class="flex flex-col">` table block, add a select-all header (own profile + selection mode):
```vue
              <label v-if="isOwnProfile && selectionMode && viewMode === 'table'" class="flex items-center gap-2 px-1 py-2 cursor-pointer">
                <Checkbox :model-value="allOnPageSelected" @update:model-value="toggleSelectAllOnPage" />
                <span class="text-xs text-muted-foreground">{{ $t('profile.bulk.selectAllPage') }}</span>
              </label>
```
Then on the `<WatchlistRow>` element add:
```vue
                  :selectable="isOwnProfile && selectionMode"
                  :selected="selectedIds.has(anime.anime_id)"
                  @toggle-select="toggleSelect(anime.anime_id)"
```

- [ ] **Step 5: Grid — checkbox overlay + click interception**

In the grid item `<div ... class="relative" ...>` wrapper, add (after `<PosterCard .../>` and the score popover):
```vue
                  <!-- Selection overlay (own profile, selection mode) -->
                  <template v-if="isOwnProfile && selectionMode">
                    <div
                      class="absolute inset-0 z-30 rounded-xl cursor-pointer"
                      :class="selectedIds.has(anime.anime_id) ? 'ring-2 ring-cyan-400 bg-cyan-400/10' : ''"
                      @click.prevent.stop="toggleSelect(anime.anime_id)"
                    />
                    <label class="absolute top-2 left-2 z-40" @click.stop>
                      <Checkbox :model-value="selectedIds.has(anime.anime_id)" @update:model-value="() => toggleSelect(anime.anime_id)" />
                    </label>
                  </template>
```

- [ ] **Step 6: Bulk bar + handlers**

After the table/grid content block (before the pagination bar), render the bar:
```vue
              <WatchlistBulkBar
                v-if="isOwnProfile && selectionMode && selectedIds.size > 0"
                :count="selectedIds.size"
                :status-options="statusOptions"
                @set-status="bulkSetStatus"
                @remove="bulkRemove"
                @clear="exitSelectionMode"
              />
```

Add handlers in the script (near `updateAnimeStatus`/`removeFromWatchlist`):
```ts
async function bulkSetStatus(status: string) {
  const ids = [...selectedIds.value]
  if (!ids.length) return
  // Optimistic: flip visible rows.
  for (const a of watchlist.value) if (selectedIds.value.has(a.anime_id)) a.status = status
  clearPageCache()
  try {
    await userApi.bulkWatchlist({ anime_ids: ids, action: 'set_status', status })
    exitSelectionMode()
    await fetchWatchlistPage()
  } catch (err) {
    console.error('bulk set status failed', err)
    toast.push(t('profile.bulk.error'))
    await fetchWatchlistPage()
  }
}

async function bulkRemove() {
  const ids = [...selectedIds.value]
  if (!ids.length) return
  if (!(await confirm({
    title: t('common.confirmTitle'),
    description: t('profile.bulk.removeConfirm', { n: ids.length }),
    confirmText: t('common.confirm'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  }))) return
  watchlist.value = watchlist.value.filter((a) => !selectedIds.value.has(a.anime_id))
  clearPageCache()
  try {
    await userApi.bulkWatchlist({ anime_ids: ids, action: 'remove' })
    exitSelectionMode()
    await fetchWatchlistPage()
  } catch (err) {
    console.error('bulk remove failed', err)
    toast.push(t('profile.bulk.error'))
    await fetchWatchlistPage()
  }
}
```

IMPORTANT: confirm the exact names of `clearPageCache`, `fetchWatchlistPage`, `toast`, `confirm`, `t`, `userApi`, `statusOptions`, `watchlist` in Profile.vue (they all exist — `clearPageCache`/`fetchWatchlistPage` are used by `updateAnimeStatus`/`removeFromWatchlist`; `confirm` from `useConfirm`; `toast` from `useToast`). Match them exactly.

- [ ] **Step 7: Type-check + lint + tests**

Run:
```bash
cd /data/animeenigma/frontend/web
bunx tsc --noEmit
bunx eslint src/views/Profile.vue
bash scripts/design-system-lint.sh
bunx vitest run src/components/profile/
```
Expected: no type errors; eslint 0 errors; design-system-lint ERRORS=0; profile tests PASS.

- [ ] **Step 8: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/views/Profile.vue
git commit -m "feat(web): watchlist selection mode + bulk actions wiring in profile"
git push
```

---

## Task 8: i18n keys

**Files:**
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`

- [ ] **Step 1: Add `profile.bulk` to `en.json`**

Inside the `profile` object:
```json
"bulk": {
  "select": "Select",
  "selectAllPage": "Select all on this page",
  "selected": "{n} selected",
  "setStatus": "Change status",
  "remove": "Remove",
  "clear": "Clear",
  "removeConfirm": "Remove {n} anime from your list? This cannot be undone.",
  "error": "Bulk action failed. Please try again."
}
```

- [ ] **Step 2: Add `profile.bulk` to `ru.json`**

Inside the `profile` object:
```json
"bulk": {
  "select": "Выделить",
  "selectAllPage": "Выделить всё на странице",
  "selected": "Выбрано: {n}",
  "setStatus": "Сменить статус",
  "remove": "Удалить",
  "clear": "Снять",
  "removeConfirm": "Удалить {n} аниме из списка? Действие необратимо.",
  "error": "Массовое действие не удалось. Попробуйте ещё раз."
}
```

- [ ] **Step 3: Validate JSON + parity**

Run:
```bash
cd /data/animeenigma/frontend/web
node -e "const en=JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8'));const ru=JSON.parse(require('fs').readFileSync('src/locales/ru.json','utf8'));const ek=Object.keys(en.profile.bulk).sort();const rk=Object.keys(ru.profile.bulk).sort();console.log('parity:', JSON.stringify(ek)===JSON.stringify(rk), ek.join(','))"
bunx vitest run src/locales/ 2>&1 | tail -5
```
Expected: `parity: true ...`; locale tests PASS.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json
git commit -m "i18n(web): profile.bulk keys (en + ru)"
git push
```

---

## Task 9: Full verification + after-update

**Files:** none (verification + deploy)

- [ ] **Step 1: Backend**

Run: `cd /data/animeenigma/services/player && go test ./... -count=1 2>&1 | tail -20 && go vet ./...`
Expected: PASS (the pre-existing MAL-export handler test that needs a live scheduler may fail — confirm it is the ONLY failure and unrelated to bulk).

- [ ] **Step 2: Frontend**

Run:
```bash
cd /data/animeenigma/frontend/web
bash scripts/design-system-lint.sh
bunx tsc --noEmit
bunx vitest run src/components/profile/ src/locales/
```
Expected: design-system ERRORS=0; tsc clean; tests PASS.

- [ ] **Step 3: Update changelog BEFORE building web**

Edit `frontend/web/public/changelog.json` — merge a Russian Trump-mode `feature` entry into the top (today's) date group, e.g.:
`"🧹 МАССОВОЕ редактирование списка! Выделяешь пачку аниме — и одним махом меняешь статус всем или сносишь из списка. Больше никакого клик-клик-клик по одному. ЖОСКО сделали. Один КАЙФ."`
Validate JSON parses.

- [ ] **Step 4: Deploy**

Commit the changelog (path-scoped), push, then build + deploy web from a CLEAN `main` worktree (excludes other agents' uncommitted files — see `[[shared-tree-stash-deploy-hazards]]`):
```bash
cd /data/animeenigma && make redeploy-player
WT=/data/animeenigma/.claude/worktrees/deploy-web-clean
git worktree remove --force "$WT" 2>/dev/null || true
git worktree add --detach "$WT" HEAD
cp docker/.env "$WT/docker/.env"
cd "$WT" && docker compose -f docker/docker-compose.yml build web
docker stop animeenigma-web; docker rm animeenigma-web
docker compose -f docker/docker-compose.yml up -d --no-deps web
cd /data/animeenigma && git worktree remove --force "$WT"
make health
```
Verify `curl 127.0.0.1:3003/` returns 200 (NOT `:80`, which is the host nginx default).

- [ ] **Step 5: API smoke**

With the `ui_audit_bot` key (`UI_AUDIT_API_KEY` in `docker/.env`):
```bash
KEY=$(grep -E '^UI_AUDIT_API_KEY=' docker/.env | cut -d= -f2-)
curl -s -X POST http://localhost:8000/api/users/watchlist/bulk -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' -d '{"anime_ids":["<real-id-from-list>"],"action":"set_status","status":"on_hold"}'
```
Expected: `{"success":true,"data":{"updated":1,"failed":0}}` (or your envelope shape). Use a real anime_id from `GET /api/users/watchlist`.

- [ ] **Step 6: Commit any remaining + done**

Ensure all changes are committed + pushed (path-scoped). The `/animeenigma-after-update` changelog/commit/push is folded into Steps 3–4 here; do a final `git status --porcelain` to confirm only pre-existing unrelated files remain dirty.

---

## Self-Review Notes

- **Spec coverage:** endpoint (T1-T3), service loop-reuse preserving side effects (T2), validation + owner-scoping (T2/T3), API client (T4), bulk bar (T5), table selection (T6 + T7), grid selection overlay (T7), select-all-on-page + clear-on-change (T7), confirm on remove (T7), i18n (T8), tests (T2/T5), deploy (T9). YAGNI exclusions (no bulk score, no cross-page select-all, no undo) respected.
- **Type consistency:** `BulkUpdateRequest{AnimeIDs,Action,Status}` (T1) used verbatim in T2/T3; `bulkWatchlist({anime_ids,action,status?})` (T4) matches the Go JSON tags; `WatchlistBulkBar` emits `set-status`/`remove`/`clear` (T5) match Profile.vue handlers `bulkSetStatus`/`bulkRemove`/`exitSelectionMode` (T7); `WatchlistRow` props `selectable`/`selected` + emit `toggleSelect` (T6) match T7 bindings; `selectedIds`/`selectionMode`/`toggleSelect`/`allOnPageSelected`/`exitSelectionMode` consistent across T7 steps.
- **Placeholders:** none — the one illustrative line in the T2 test is explicitly flagged to be removed during harness adaptation.
