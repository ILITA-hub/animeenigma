# Season Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Season" filter to the catalog Browse UI and fix the derived-season data so the filter is accurate (no separate page).

**Architecture:** Backend already filters by `season` — only the frontend filter state + sidebar control + i18n are new. Separately, `detectSeason` is corrected so a missing aired-month no longer mislabels anime as "fall", and the 49 already-mislabeled rows are cleared.

**Tech Stack:** Go (catalog service, Shikimori parser), Vue 3 + TypeScript (composable + SFC), Vitest, vue-i18n, PostgreSQL.

**Spec:** `docs/superpowers/specs/2026-06-15-season-filter-design.md`

**Commit convention (this repo):** path-scoped `git add <paths>` only (shared tree with concurrent committers — never `git add -A`). Every commit appends:
```
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
```

---

## File Structure

- **Modify** `services/catalog/internal/parser/shikimori/client.go` — fix `detectSeason` (lines 523-534).
- **Create** `services/catalog/internal/parser/shikimori/season_test.go` — white-box `detectSeason` table test.
- **Modify** `frontend/web/src/composables/useBrowseFilters.ts` — add `season` state + URL/apiParams wiring.
- **Create** `frontend/web/src/composables/__tests__/useBrowseFilters.spec.ts` — season behavior test.
- **Modify** `frontend/web/src/components/browse/BrowseSidebar.vue` — add Season `FilterSection`.
- **Modify** `frontend/web/src/locales/en.json` + `frontend/web/src/locales/ru.json` — season i18n keys.
- **One-off** PostgreSQL `UPDATE` (no file) — clear year-less seasons.

---

## Task 1: Backend — fix `detectSeason` (TDD)

**Files:**
- Create: `services/catalog/internal/parser/shikimori/season_test.go`
- Modify: `services/catalog/internal/parser/shikimori/client.go:523-534`

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/parser/shikimori/season_test.go`:

```go
package shikimori

import "testing"

func TestDetectSeason(t *testing.T) {
	cases := []struct {
		month int
		want  string
	}{
		{1, "winter"}, {2, "winter"}, {3, "winter"},
		{4, "spring"}, {5, "spring"}, {6, "spring"},
		{7, "summer"}, {8, "summer"}, {9, "summer"},
		{10, "fall"}, {11, "fall"}, {12, "fall"},
		{0, ""},   // missing month must NOT map to fall
		{13, ""},  // out of range
		{-1, ""},  // out of range
	}
	for _, c := range cases {
		if got := detectSeason(c.month); got != c.want {
			t.Errorf("detectSeason(%d) = %q, want %q", c.month, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/shikimori/ -run TestDetectSeason -v`
Expected: FAIL — `detectSeason(0) = "fall", want ""` (and same for 13, -1).

- [ ] **Step 3: Fix `detectSeason`**

In `services/catalog/internal/parser/shikimori/client.go`, replace the function body (currently lines 523-534) so the catch-all no longer returns "fall":

```go
func detectSeason(month int) string {
	switch {
	case month >= 1 && month <= 3:
		return "winter"
	case month >= 4 && month <= 6:
		return "spring"
	case month >= 7 && month <= 9:
		return "summer"
	case month >= 10 && month <= 12:
		return "fall"
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/parser/shikimori/ -run TestDetectSeason -v`
Expected: PASS.

- [ ] **Step 5: Build the whole package (no other call site broke)**

Run: `cd services/catalog && go build ./... && go vet ./internal/parser/shikimori/`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add services/catalog/internal/parser/shikimori/client.go services/catalog/internal/parser/shikimori/season_test.go
git commit -m "fix(catalog): detectSeason returns empty for missing/invalid month

Was defaulting month=0 (no aired date) to fall, mislabeling undated
announcements. Add explicit Oct-Dec=fall branch; default to empty.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Backend — clear year-less seasons (one-off data backfill)

**Files:** none (direct DB mutation). This corrects existing rows; the Task 1 fix keeps future re-ingests correct.

- [ ] **Step 1: Snapshot the bad rows (before)**

Run:
```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c \
"SELECT season, COUNT(*) FROM animes WHERE year = 0 AND season <> '' GROUP BY season;"
```
Expected: a `fall | 49` row (count may differ slightly if catalog changed).

- [ ] **Step 2: Run the backfill**

Run:
```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c \
"UPDATE animes SET season = '' WHERE year = 0 AND season <> '';"
```
Expected: `UPDATE 49` (or current count).

- [ ] **Step 3: Verify (after)**

Run:
```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c \
"SELECT COUNT(*) AS leftover FROM animes WHERE year = 0 AND season <> '';"
```
Expected: `leftover | 0`.

No commit (database state only). Record the statement in the changelog during ship.

---

## Task 3: Frontend — `useBrowseFilters` season state (TDD)

**Files:**
- Modify: `frontend/web/src/composables/useBrowseFilters.ts`
- Create: `frontend/web/src/composables/__tests__/useBrowseFilters.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/composables/__tests__/useBrowseFilters.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

// Mutable route query + spy router, controlled per test. Pattern mirrors
// src/views/Schedule.spec.ts:7.
const mockState = vi.hoisted(() => ({ query: {} as Record<string, string> }))
const mockReplace = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRoute: () => ({
    get query() {
      return mockState.query
    },
  }),
  useRouter: () => ({ replace: mockReplace }),
}))

import { useBrowseFilters } from '@/composables/useBrowseFilters'

function harness() {
  let api!: ReturnType<typeof useBrowseFilters>
  const Cmp = defineComponent({
    setup() {
      api = useBrowseFilters()
      return () => null
    },
  })
  const wrapper = mount(Cmp)
  return { api, wrapper }
}

beforeEach(() => {
  mockState.query = {}
  mockReplace.mockClear()
})

describe('useBrowseFilters — season', () => {
  it('reads a valid season from the URL into apiParams', () => {
    mockState.query = { season: 'summer' }
    const { api } = harness()
    api.readUrl()
    expect(api.season.value).toBe('summer')
    expect(api.apiParams.value.season).toBe('summer')
  })

  it('drops an invalid season value', () => {
    mockState.query = { season: 'bogus' }
    const { api } = harness()
    api.readUrl()
    expect(api.season.value).toBe('')
    expect(api.apiParams.value.season).toBeUndefined()
  })

  it('writes the season to the URL query', () => {
    const { api } = harness()
    api.season.value = 'fall'
    api.writeUrl()
    expect(mockReplace).toHaveBeenCalledWith({
      query: expect.objectContaining({ season: 'fall' }),
    })
  })

  it('counts season as an active filter', () => {
    const { api } = harness()
    expect(api.activeCount.value).toBe(0)
    api.season.value = 'winter'
    expect(api.activeCount.value).toBe(1)
  })

  it('reset clears the season', () => {
    const { api } = harness()
    api.season.value = 'spring'
    api.reset()
    expect(api.season.value).toBe('')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useBrowseFilters.spec.ts`
Expected: FAIL — `api.season` is undefined (property does not exist yet).

- [ ] **Step 3: Add season type + whitelist**

In `frontend/web/src/composables/useBrowseFilters.ts`, after the existing
`STATUS_VALUES` / `Status` declarations (near the top type block), add:

```ts
export type Season = '' | 'winter' | 'spring' | 'summer' | 'fall'
const SEASON_VALUES: Season[] = ['', 'winter', 'spring', 'summer', 'fall']
```

- [ ] **Step 4: Add the `season` ref**

In the same file, alongside the other refs (after `const status = ref<Status>('')`), add:

```ts
const season = ref<Season>('')
```

- [ ] **Step 5: Wire `readUrl()`**

In `readUrl()`, after the `status.value = ...` line, add:

```ts
const rawSeason = (typeof qry.season === 'string' ? qry.season : '') as Season
season.value = (SEASON_VALUES as readonly string[]).includes(rawSeason) ? rawSeason : ''
```

- [ ] **Step 6: Wire `writeUrl()`**

In `writeUrl()`, after the `next.status = status.value || undefined` line, add:

```ts
next.season = season.value || undefined
```

- [ ] **Step 7: Wire `apiParams`**

In the `apiParams` computed, after `if (status.value) p.status = status.value`, add:

```ts
if (season.value) p.season = season.value
```

- [ ] **Step 8: Wire `activeCount`**

In the `activeCount` computed, after `if (status.value) n++`, add:

```ts
if (season.value) n++
```

- [ ] **Step 9: Wire `reset()` and the return**

In `reset()`, after `status.value = ''`, add `season.value = ''`.
In the final `return { ... }` object, add `season,` next to `status,`.

- [ ] **Step 10: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useBrowseFilters.spec.ts`
Expected: PASS (5 tests).

- [ ] **Step 11: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: no errors.

- [ ] **Step 12: Commit**

```bash
git add frontend/web/src/composables/useBrowseFilters.ts frontend/web/src/composables/__tests__/useBrowseFilters.spec.ts
git commit -m "feat(browse): season filter state in useBrowseFilters

URL <-> state sync + apiParams + activeCount + reset for ?season=.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: Frontend — Season sidebar control + i18n

**Files:**
- Modify: `frontend/web/src/components/browse/BrowseSidebar.vue`
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`

- [ ] **Step 1: Add the Season `FilterSection` to the template**

In `frontend/web/src/components/browse/BrowseSidebar.vue`, insert this block
immediately **after** the closing `</FilterSection>` of the Status section
(the one with `:label="$t('browse.filters.section.status')"`, around line 41)
and **before** the `<!-- Year range -->` comment:

```html
    <!-- Season — single-select radio -->
    <FilterSection
      :label="$t('browse.filters.section.season')"
      :count="filters.season.value ? 1 : 0"
    >
      <RadioGroup :model-value="filters.season.value" :options="seasonOptions" @update:model-value="onSeasonChange" />
    </FilterSection>
```

- [ ] **Step 2: Add the `seasonOptions` computed**

In the `<script setup>` block, immediately after the `statusOptions` computed
(around line 182-187), add:

```ts
const seasonOptions = computed(() => [
  { value: '', label: t('browse.filters.season.any') },
  { value: 'winter', label: t('browse.filters.season.winter') },
  { value: 'spring', label: t('browse.filters.season.spring') },
  { value: 'summer', label: t('browse.filters.season.summer') },
  { value: 'fall', label: t('browse.filters.season.fall') },
])
```

- [ ] **Step 3: Add the `onSeasonChange` handler**

Immediately after the `onStatusChange` function (around line 243-246), add:

```ts
function onSeasonChange(v: string) {
  props.filters.season.value = v as typeof props.filters.season.value
  props.filters.writeUrl()
}
```

- [ ] **Step 4: Add EN i18n keys**

In `frontend/web/src/locales/en.json`:

(a) In `browse.filters.section` (line 958, after `"status": "Status",`) add:
```json
        "season": "Season",
```

(b) After the `browse.filters.status` block's closing `},` (line 977) insert a new block:
```json
      "season": {
        "any": "Any season",
        "winter": "Winter",
        "spring": "Spring",
        "summer": "Summer",
        "fall": "Fall"
      },
```

- [ ] **Step 5: Add RU i18n keys**

In `frontend/web/src/locales/ru.json`:

(a) In `browse.filters.section` (line 958, after `"status": "Статус",`) add:
```json
        "season": "Сезон",
```

(b) After the `browse.filters.status` block's closing `},` (line 977) insert:
```json
      "season": {
        "any": "Любой сезон",
        "winter": "Зима",
        "spring": "Весна",
        "summer": "Лето",
        "fall": "Осень"
      },
```

- [ ] **Step 6: Validate JSON + types**

Run:
```bash
cd frontend/web && node -e "JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8')); JSON.parse(require('fs').readFileSync('src/locales/ru.json','utf8')); console.log('json ok')"
bunx tsc --noEmit
```
Expected: `json ok`, no type errors.

- [ ] **Step 7: Lint (design-system gate + eslint)**

Run:
```bash
cd frontend/web && bash scripts/design-system-lint.sh && bunx eslint src/components/browse/BrowseSidebar.vue
```
Expected: design-system lint `ERRORS=0`; eslint clean. (The block adds no colors/hex — it reuses `FilterSection`/`RadioGroup`.)

- [ ] **Step 8: Run the full browse-related test suite**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useBrowseFilters.spec.ts`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add frontend/web/src/components/browse/BrowseSidebar.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json
git commit -m "feat(browse): season filter control + i18n (en/ru)

Single-select Season RadioGroup in the catalog sidebar, mirroring Status.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 5: Ship — redeploy, changelog, push

- [ ] **Step 1: Full backend test for the touched package**

Run: `cd services/catalog && go test ./internal/parser/shikimori/...`
Expected: PASS.

- [ ] **Step 2: Runtime smoke the season filter against the live API**

Run (after catalog redeploy in Step 3, re-run if needed):
```bash
curl -s "http://localhost:8000/api/anime?status=announced&season=summer&year_from=2026&year_to=2026&page_size=5" | head -c 400
```
Expected: a JSON list of summer-2026 announced anime (non-empty given ~69 such rows).

- [ ] **Step 3: Invoke the project ship skill**

Invoke `/animeenigma-after-update`. It will: lint + build the affected code,
`make redeploy-catalog` and `make redeploy-web`, run health checks, append a
Russian Trump-mode changelog entry to `frontend/web/public/changelog.json`
(mention: new Season filter in catalog + season-data fix), and commit + push.

- [ ] **Step 4: Confirm pushed to main**

Verify the after-update push reported `... -> main`. In this shared tree, if a
direct push is rejected (remote moved), land the commits on main via a detached
`git worktree` at `origin/main` + cherry-pick + push (NEVER `git stash` /
`--autostash` the shared tree).

---

## Self-Review

- **Spec coverage:** Part A (frontend filter) → Tasks 3-4. Part B (`detectSeason` fix) → Task 1; (data backfill) → Task 2. Behavior notes / out-of-scope → no tasks (correctly nothing to build). Verification section → Tasks 1-5. ✓
- **Placeholder scan:** none — every code/SQL/command step is concrete. ✓
- **Type consistency:** `Season` type, `SEASON_VALUES`, `season` ref, `seasonOptions`, `onSeasonChange` used consistently across Tasks 3-4; i18n keys `browse.filters.section.season` + `browse.filters.season.{any,winter,spring,summer,fall}` match between BrowseSidebar (Task 4 Step 2) and the locale files (Task 4 Steps 4-5). ✓
- **Note:** there is no automated en/ru parity test for the `browse` namespace (only spotlight/gacha/watch-together have key-parity specs), so Task 4 adds keys to both files by hand and Step 6 validates JSON.
