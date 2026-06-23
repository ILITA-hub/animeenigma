# Browse + List Filter Unification — Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the anime-type filter correspond across Browse and the Profile watchlist (same values, labels, control, backend semantics) and unify the two pages' interactive filter controls onto the shared `components/filters/` library.

**Architecture:** Backend `SearchFilters.Kind string` widens to `Kinds []string` (comma-separated `kind` param, `kind IN (?)`). A canonical `constants/animeKinds.ts` (9 Shikimori kinds) is the single source of truth for values+order. Browse's anime-type control switches from single-select RadioGroup to multi-select `FilterCheckboxList`; both pages render kind labels from a new shared `common.filters.kind.*` namespace. A new shared `FilterYearRange.vue` replaces both pages' ad-hoc year controls. The List page (`WatchlistFilters.vue`) adopts `FilterCheckboxList` (genres + kinds) and `FilterYearRange`.

**Tech Stack:** Go + GORM (catalog), Vue 3 + TypeScript + Tailwind v4 + reka-ui (frontend), vitest, vue-i18n (en/ru/ja).

## Global Constraints

- **No time-effort units.** Metrics use UXΔ / CDI / MVQ only.
- **Canonical kind set (exact values + order):** `tv, movie, ova, ona, special, tv_special, music, cm, pv`. The backend whitelist, the FE constant, and `common.filters.kind.*` MUST all carry exactly these 9.
- **DO NOT remove `SearchFilters.Source`.** (Design §10 called it dead; it is NOT — `service/catalog.go:188,250` use `filters.Source == "shikimori"` to force a live Shikimori search. Removing it breaks `?source=shikimori`.)
- **DS-lint is build-enforced** (`frontend/web/scripts/design-system-lint.sh`, 8 rules). Brand/provider hues `cyan/pink/orange/rose/indigo/teal/lime` are EXEMPT. The `PostToolUse` DS-lint hook fires on every `frontend/web/src/**/*.{vue,ts}` edit — fix any reported violation before moving on. Bind colors to semantic tokens.
- **i18n parity en/ru/ja is enforced.** Every key added/removed must be mirrored across all three locale files. Run the i18n gate after locale edits.
- **`vue-tsc --noEmit` false-passes from cache** — the authoritative FE gate is a real `cd frontend/web && bun run build` (or `/frontend-verify`).
- **reka primitives are NOT native inputs.** `Checkbox` renders `<button role="checkbox">`, `Input`/`Select` are components. Test interaction via `findAllComponents({ name: 'Checkbox' })[i].vm.$emit('update:modelValue', …)` / `findComponent({ name: 'Input'|'Select' }).vm.$emit('update:modelValue', …)`.
- **lucide icons:** named imports from `lucide-vue-next`.
- **Every commit** carries the three co-author trailers:
  ```
  Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Work in the existing worktree** `/data/animeenigma/.claude/worktrees/browse-list-filters` (base `a25dc4d3`, bun deps installed). Never edit the base tree `/data/animeenigma`.

### Scope refinements from the design spec (deliberate, surfaced to owner)

1. **`SearchFilters.Source` kept** (see above) — §10's removal item is dropped.
2. **Section chrome stays page-specific.** Browse keeps its `FilterSection` collapsibles (narrow sidebar); List keeps its always-visible panel sections (wide grid). Only the *interactive controls* (`FilterCheckboxList`, `FilterYearRange`) and the anime-type *values/labels* (`common.filters.kind/type`) are unified — that resolves the owner's stated pain ("different design and functionality", "anime type do not correspond") without degrading List's wide-panel layout by forcing collapsible chrome into a grid. `FilterSection` therefore stays in `components/browse/` (not moved to `components/filters/`).
3. **i18n consolidation scoped to the genuinely-shared, drift-prone labels:** `common.filters.type` (the anime-type section header, both pages) + `common.filters.kind.*` (the 9 kind labels, both pages). Genre/studio/year/search/clear labels stay in their page namespaces — they are passed as props to the now-identical shared components, so they carry no drift risk, and migrating them is pure churn.

---

### Task 1: Backend — multi-kind `IN` filter

**Files:**
- Modify: `services/catalog/internal/domain/anime.go` (SearchFilters: `Kind string` → `Kinds []string`)
- Modify: `services/catalog/internal/handler/catalog.go` (parse comma-separated `kind`, 9-value whitelist)
- Modify: `services/catalog/internal/repo/anime.go` (`kind IN (?)`)
- Test: `services/catalog/internal/repo/browse_filter_test.go` (add multi-kind OR test)

**Interfaces:**
- Consumes: existing `domain.SearchFilters`, `repo.AnimeRepository.Search`.
- Produces: `SearchFilters.Kinds []string`; the catalog `GET /api/anime?kind=tv,movie` accepts a comma-separated OR-set of the 9 canonical kinds.

- [ ] **Step 1: Write the failing test** — append to `services/catalog/internal/repo/browse_filter_test.go`:

```go
func TestAnimeRepository_Search_KindsFilter_ORSet(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := NewAnimeRepository(db)
	ctx := context.Background()

	seedBrowseAnime(t, db, "tv", "A TV Show")
	seedBrowseAnime(t, db, "mv", "A Movie")
	seedBrowseAnime(t, db, "ova", "An OVA")
	require.NoError(t, db.Exec(`UPDATE animes SET kind='tv' WHERE id='tv'`).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET kind='movie' WHERE id='mv'`).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET kind='ova' WHERE id='ova'`).Error)

	// OR-set: selecting tv+movie returns BOTH, not the (empty) intersection.
	got, total, err := r.Search(ctx, domain.SearchFilters{
		Kinds: []string{"tv", "movie"}, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	ids := map[string]bool{}
	for _, a := range got {
		ids[a.ID] = true
	}
	require.True(t, ids["tv"] && ids["mv"])
	require.False(t, ids["ova"])

	// Single kind still works.
	got, total, err = r.Search(ctx, domain.SearchFilters{
		Kinds: []string{"ova"}, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "ova", got[0].ID)
}
```

- [ ] **Step 2: Run it to confirm it fails to compile** (`Kinds` field does not exist yet)

Run: `cd services/catalog && go test ./internal/repo/ -run TestAnimeRepository_Search_KindsFilter_ORSet`
Expected: FAIL — `unknown field 'Kinds' in struct literal` / `domain.SearchFilters has no field or method Kinds`.

- [ ] **Step 3: Widen the domain field** — in `services/catalog/internal/domain/anime.go`, replace the `Kind` field and refresh its doc comment. Change the struct so:

```go
	// Phase 15 (UX-31) — multi-axis browse sidebar filters.
	// Kinds is the OR-set of Shikimori kinds: "tv" / "movie" / "ova" /
	// "ona" / "special" / "tv_special" / "music" / "cm" / "pv". A row
	// passes when its kind matches ANY selected value. Empty = no filter.
	// Providers is the OR-set of {"kodik","dub","raw","ae"} → columns
	// has_kodik/has_dub/has_raw/has_video — a row passes when ANY of the
	// selected columns is true. Empty = no filter. Unknown values dropped at
	// the handler. StudioIDs is an OR-set over the anime_studios join.
	Kinds     []string
	Providers []string
```

(i.e. delete the line `Kind      string` and the old "Kind matches the Shikimori-source enum…" comment lines; replace with the above. Keep `Source string` untouched.)

- [ ] **Step 4: Apply the filter in the repo** — in `services/catalog/internal/repo/anime.go`, replace the Kind block (currently `if filters.Kind != "" { query = query.Where("kind = ?", filters.Kind) }`) with:

```go
	// Phase 15 (UX-31) — Kinds is an OR-set over the Shikimori kind enum.
	// A row passes when its kind matches ANY selected value. Whitelisted at
	// the handler so unknown values never reach the SQL.
	if len(filters.Kinds) > 0 {
		query = query.Where("kind IN ?", filters.Kinds)
	}
```

- [ ] **Step 5: Parse the param in the handler** — in `services/catalog/internal/handler/catalog.go`, replace the kind block (currently `if kind := query.Get("kind"); kind != "" { switch kind { case "tv","movie","ova","ona","special": filters.Kind = kind } }`) with a comma-split, deduped, 9-value whitelist mirroring the Providers block:

```go
	// Phase 15 (UX-31) — Kinds filter (comma-separated, e.g. "tv,movie").
	// Whitelist matches the canonical Shikimori kind set and the frontend
	// FilterCheckboxList options. Unknown values silently drop so a malicious
	// value can't reach the SQL. Duplicates collapse via `seen`.
	if kinds := query.Get("kind"); kinds != "" {
		seen := map[string]bool{}
		for _, k := range strings.Split(kinds, ",") {
			k = strings.TrimSpace(strings.ToLower(k))
			switch k {
			case "tv", "movie", "ova", "ona", "special", "tv_special", "music", "cm", "pv":
				if !seen[k] {
					filters.Kinds = append(filters.Kinds, k)
					seen[k] = true
				}
			}
		}
	}
```

- [ ] **Step 6: Run the new test + full catalog suite**

Run: `cd services/catalog && go test ./... -race`
Expected: PASS (new test green; nothing else broken — no prior test referenced `filters.Kind`).

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/domain/anime.go services/catalog/internal/handler/catalog.go services/catalog/internal/repo/anime.go services/catalog/internal/repo/browse_filter_test.go
git commit -m "feat(catalog): multi-kind browse filter (Kind -> Kinds, kind IN)"
```

---

### Task 2: Frontend — canonical `constants/animeKinds.ts`

**Files:**
- Create: `frontend/web/src/constants/animeKinds.ts`
- Test: `frontend/web/src/constants/animeKinds.spec.ts`

**Interfaces:**
- Produces: `export const ANIME_KINDS` (readonly tuple of the 9 canonical kinds, in order) and `export type AnimeKind`. Consumed by `useBrowseFilters.ts` and `BrowseSidebar.vue` (Task 4).

- [ ] **Step 1: Write the failing test** — `frontend/web/src/constants/animeKinds.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { ANIME_KINDS } from './animeKinds'

describe('ANIME_KINDS', () => {
  it('carries exactly the 9 canonical Shikimori kinds in order', () => {
    expect([...ANIME_KINDS]).toEqual([
      'tv', 'movie', 'ova', 'ona', 'special', 'tv_special', 'music', 'cm', 'pv',
    ])
  })

  it('has no duplicates', () => {
    expect(new Set(ANIME_KINDS).size).toBe(ANIME_KINDS.length)
  })

  it('every value has a matching common.filters.kind locale key in en', async () => {
    const en = (await import('@/locales/en.json')).default as Record<string, unknown>
    const kind = ((en.common as Record<string, Record<string, Record<string, string>>>)?.filters?.kind) ?? {}
    for (const k of ANIME_KINDS) expect(kind[k], `en common.filters.kind.${k}`).toBeTruthy()
  })
})
```

> Note: the third assertion will only pass once Task 4 adds `common.filters.kind.*` to `en.json`. If running Task 2 strictly before Task 4, expect that one case red until Task 4; the first two must pass immediately. (Implementer: it is acceptable to land Task 2 with the first two assertions and the third following Task 4 — but the constant + first two MUST be green at commit.)

- [ ] **Step 2: Run it to verify failure** (`Cannot find module './animeKinds'`)

Run: `cd frontend/web && bunx vitest run src/constants/animeKinds.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Create the constant** — `frontend/web/src/constants/animeKinds.ts`:

```ts
// Canonical Shikimori anime "kind" enum — the single source of truth for
// both the Browse anime-type filter (FilterCheckboxList options) and the
// backend whitelist (services/catalog handler). Order drives display order.
// Labels live in i18n under common.filters.kind.* (shared by Browse + List).
export const ANIME_KINDS = [
  'tv', 'movie', 'ova', 'ona', 'special', 'tv_special', 'music', 'cm', 'pv',
] as const

export type AnimeKind = (typeof ANIME_KINDS)[number]
```

- [ ] **Step 4: Run the test** (first two cases green; third green after Task 4)

Run: `cd frontend/web && bunx vitest run src/constants/animeKinds.spec.ts`
Expected: the two structural cases PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/constants/animeKinds.ts frontend/web/src/constants/animeKinds.spec.ts
git commit -m "feat(filters): canonical ANIME_KINDS constant (9 Shikimori kinds)"
```

---

### Task 3: Frontend — shared `FilterYearRange.vue`

**Files:**
- Create: `frontend/web/src/components/filters/FilterYearRange.vue`
- Test: `frontend/web/src/components/filters/FilterYearRange.spec.ts`

**Interfaces:**
- Produces: `<FilterYearRange :min :max :floor-year :ceil-year @update:min @update:max />`. Props `min: number|null`, `max: number|null`, `floorYear: number`, `ceilYear: number`. Emits `update:min`/`update:max` with `number|null`. Enforces `min ≤ max` internally (a min above max bumps max, and vice-versa). Consumed by Browse (Task 4) and List (Task 5).

- [ ] **Step 1: Write the failing test** — `frontend/web/src/components/filters/FilterYearRange.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FilterYearRange from './FilterYearRange.vue'

// Select is a reka-ui primitive (portaled options); interact via the
// component's update:modelValue event and assert via its props.
function mountWith(props: Record<string, unknown> = {}) {
  return mount(FilterYearRange, {
    props: { min: null, max: null, floorYear: 2010, ceilYear: 2012, ...props },
  })
}

describe('FilterYearRange', () => {
  it('builds descending year options between ceil and floor plus an ANY sentinel', () => {
    const w = mountWith()
    const opts = w.findAllComponents({ name: 'Select' })[0].props('options') as { value: string; label: string }[]
    expect(opts.map((o) => o.value)).toEqual(['any', '2012', '2011', '2010'])
  })

  it('shows the ANY sentinel when a bound is null', () => {
    const w = mountWith({ min: null, max: 2011 })
    expect(w.findAllComponents({ name: 'Select' })[0].props('modelValue')).toBe('any')
    expect(w.findAllComponents({ name: 'Select' })[1].props('modelValue')).toBe('2011')
  })

  it('emits a numeric update:min when a year is chosen', async () => {
    const w = mountWith()
    await w.findAllComponents({ name: 'Select' })[0].vm.$emit('update:modelValue', '2011')
    expect(w.emitted('update:min')?.[0]).toEqual([2011])
  })

  it('emits null update:max when ANY is chosen', async () => {
    const w = mountWith({ max: 2012 })
    await w.findAllComponents({ name: 'Select' })[1].vm.$emit('update:modelValue', 'any')
    expect(w.emitted('update:max')?.[0]).toEqual([null])
  })

  it('bumps max up to keep min <= max', async () => {
    const w = mountWith({ min: null, max: 2010 })
    await w.findAllComponents({ name: 'Select' })[0].vm.$emit('update:modelValue', '2012')
    expect(w.emitted('update:min')?.[0]).toEqual([2012])
    expect(w.emitted('update:max')?.[0]).toEqual([2012])
  })
})
```

- [ ] **Step 2: Run it to verify failure** (`Cannot find module './FilterYearRange.vue'`)

Run: `cd frontend/web && bunx vitest run src/components/filters/FilterYearRange.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Create the component** — `frontend/web/src/components/filters/FilterYearRange.vue`:

```vue
<template>
  <div class="flex items-center gap-2">
    <Select
      :model-value="minStr"
      :options="options"
      size="sm"
      class="flex-1"
      @update:model-value="(v) => emitVal('min', v as string)"
    />
    <span class="text-white/40">—</span>
    <Select
      :model-value="maxStr"
      :options="options"
      size="sm"
      class="flex-1"
      @update:model-value="(v) => emitVal('max', v as string)"
    />
  </div>
</template>

<script setup lang="ts">
// Shared year-range control (Browse + Profile watchlist). Two DS Select
// dropdowns over a descending year list bounded by floorYear..ceilYear.
// reka-ui's SelectItem forbids an empty-string value, so the "any year"
// option uses a non-empty sentinel. min <= max is enforced here so both
// consumers inherit it.
import { computed } from 'vue'
import { Select } from '@/components/ui'

const props = defineProps<{
  min: number | null
  max: number | null
  floorYear: number
  ceilYear: number
}>()

const emit = defineEmits<{
  (e: 'update:min', v: number | null): void
  (e: 'update:max', v: number | null): void
}>()

const ANY = 'any'

const options = computed(() => {
  const out = [{ value: ANY, label: '—' }]
  for (let y = props.ceilYear; y >= props.floorYear; y--) {
    out.push({ value: String(y), label: String(y) })
  }
  return out
})

const minStr = computed(() => (props.min == null ? ANY : String(props.min)))
const maxStr = computed(() => (props.max == null ? ANY : String(props.max)))

function emitVal(which: 'min' | 'max', v: string) {
  const n = v === ANY || v === '' ? null : Number(v)
  if (which === 'min') {
    emit('update:min', n)
    if (n != null && props.max != null && n > props.max) emit('update:max', n)
  } else {
    emit('update:max', n)
    if (n != null && props.min != null && n < props.min) emit('update:min', n)
  }
}
</script>
```

- [ ] **Step 4: Run the test**

Run: `cd frontend/web && bunx vitest run src/components/filters/FilterYearRange.spec.ts`
Expected: PASS (5/5).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/filters/FilterYearRange.vue frontend/web/src/components/filters/FilterYearRange.spec.ts
git commit -m "feat(filters): shared FilterYearRange (two-Select year range, min<=max)"
```

---

### Task 4: Frontend — Browse adopts multi-kind + FilterYearRange

**Files:**
- Modify: `frontend/web/src/composables/useBrowseFilters.ts` (`kind` → `kinds[]`)
- Modify: `frontend/web/src/components/browse/BrowseSidebar.vue` (anime-type → FilterCheckboxList; year → FilterYearRange)
- Modify: `frontend/web/src/composables/__tests__/useBrowseFilters.spec.ts` (add kinds tests)
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json` (add `common.filters.{type,kind.*}`; remove `browse.filters.format` + `browse.filters.section.format`)

**Interfaces:**
- Consumes: `ANIME_KINDS` (Task 2), `FilterCheckboxList` (Phase 1), `FilterYearRange` (Task 3).
- Produces: `useBrowseFilters().kinds` (a `Ref<string[]>`) replacing `.kind`; `apiParams.kind` is now a comma-joined string. `common.filters.type` + `common.filters.kind.*` locale keys.

- [ ] **Step 1: Update the composable test first** — in `frontend/web/src/composables/__tests__/useBrowseFilters.spec.ts`, append a kinds describe block (the existing season block stays unchanged):

```ts
describe('useBrowseFilters — kinds', () => {
  it('reads a comma-separated kind list from the URL into apiParams', () => {
    mockState.query = { kind: 'tv,movie' }
    const { api } = harness()
    api.readUrl()
    expect(api.kinds.value).toEqual(['tv', 'movie'])
    expect(api.apiParams.value.kind).toBe('tv,movie')
  })

  it('drops invalid kind values', () => {
    mockState.query = { kind: 'tv,bogus' }
    const { api } = harness()
    api.readUrl()
    expect(api.kinds.value).toEqual(['tv'])
  })

  it('writes the kinds joined to the URL query', () => {
    const { api } = harness()
    api.kinds.value = ['ova', 'ona']
    api.writeUrl()
    expect(mockReplace).toHaveBeenCalledWith({
      query: expect.objectContaining({ kind: 'ova,ona' }),
    })
  })

  it('counts a non-empty kinds list as one active filter', () => {
    const { api } = harness()
    expect(api.activeCount.value).toBe(0)
    api.kinds.value = ['tv', 'movie']
    expect(api.activeCount.value).toBe(1)
  })

  it('reset clears the kinds', () => {
    const { api } = harness()
    api.kinds.value = ['special']
    api.reset()
    expect(api.kinds.value).toEqual([])
  })
})
```

- [ ] **Step 2: Run it to verify failure** (`api.kinds` is undefined — composable still exposes `kind`)

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useBrowseFilters.spec.ts`
Expected: FAIL on the kinds block.

- [ ] **Step 3: Widen the composable** — edit `frontend/web/src/composables/useBrowseFilters.ts`:

  (a) Replace the `Kind` type + `KIND_VALUES` lines with a canonical import. Delete:
  ```ts
  export type Kind = '' | 'tv' | 'movie' | 'ova' | 'ona' | 'special'
  ```
  and
  ```ts
  const KIND_VALUES: Kind[] = ['', 'tv', 'movie', 'ova', 'ona', 'special']
  ```
  Add at the top of the import section:
  ```ts
  import { ANIME_KINDS } from '@/constants/animeKinds'
  ```

  (b) Replace the state ref `const kind = ref<Kind>('')` with:
  ```ts
  const kinds = ref<string[]>([])
  ```

  (c) In `readUrl()`, replace the two kind lines
  ```ts
  const rawKind = typeof qry.kind === 'string' ? (qry.kind as Kind) : ''
  kind.value = KIND_VALUES.includes(rawKind) ? rawKind : ''
  ```
  with the guarded comma-split (mirrors the genres guard):
  ```ts
  const newKinds = ((qry.kind as string) || '')
    .split(',')
    .map(s => s.trim().toLowerCase())
    .filter(k => (ANIME_KINDS as readonly string[]).includes(k))
  if (newKinds.join(',') !== kinds.value.join(',')) {
    kinds.value = newKinds
  }
  ```

  (d) In `writeUrl()`, replace `next.kind = kind.value || undefined` with:
  ```ts
  next.kind = kinds.value.length ? kinds.value.join(',') : undefined
  ```

  (e) In `apiParams`, replace `if (kind.value) p.kind = kind.value` with:
  ```ts
  if (kinds.value.length) p.kind = kinds.value.join(',')
  ```

  (f) In `activeCount`, replace `if (kind.value) n++` with:
  ```ts
  if (kinds.value.length) n++
  ```

  (g) In `reset()`, replace `kind.value = ''` with:
  ```ts
  kinds.value = []
  ```

  (h) In the returned object, replace `kind,` with `kinds,`.

- [ ] **Step 4: Run the composable test**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useBrowseFilters.spec.ts`
Expected: PASS (season + kinds blocks).

- [ ] **Step 5: Add the shared locale keys** — in EACH of `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`, add a `filters` object inside the existing `common` object (alongside `loading`, `retry`, …). Use these exact values:

  en.json — `common.filters`:
  ```json
  "filters": {
    "type": "Type",
    "kind": {
      "tv": "TV series",
      "movie": "Movie",
      "ova": "OVA",
      "ona": "ONA",
      "special": "Special",
      "tv_special": "TV Special",
      "music": "Music",
      "cm": "Commercial",
      "pv": "Promo"
    }
  }
  ```
  ru.json — `common.filters`:
  ```json
  "filters": {
    "type": "Тип",
    "kind": {
      "tv": "ТВ-сериал",
      "movie": "Фильм",
      "ova": "OVA",
      "ona": "ONA",
      "special": "Спешл",
      "tv_special": "ТВ-спешл",
      "music": "Клип",
      "cm": "Реклама",
      "pv": "Промо"
    }
  }
  ```
  ja.json — `common.filters`:
  ```json
  "filters": {
    "type": "種別",
    "kind": {
      "tv": "TVシリーズ",
      "movie": "映画",
      "ova": "OVA",
      "ona": "ONA",
      "special": "スペシャル",
      "tv_special": "TVスペシャル",
      "music": "ミュージック",
      "cm": "CM",
      "pv": "PV"
    }
  }
  ```

  Then REMOVE the now-superseded Browse keys from all three locales:
  - `browse.filters.format` (the whole object: `any/tv/movie/ova/ona/special`)
  - `browse.filters.section.format`

- [ ] **Step 6: Rewire BrowseSidebar** — edit `frontend/web/src/components/browse/BrowseSidebar.vue`:

  (a) Imports: change
  ```ts
  import { useBrowseFilters, type Provider, type Kind, type Sort } from '@/composables/useBrowseFilters'
  ```
  to
  ```ts
  import { useBrowseFilters, type Provider, type Sort } from '@/composables/useBrowseFilters'
  import { ANIME_KINDS } from '@/constants/animeKinds'
  ```
  Add `FilterYearRange` import next to `FilterCheckboxList`:
  ```ts
  import FilterYearRange from '@/components/filters/FilterYearRange.vue'
  ```
  Change the ui-barrel import from `import { Input, RadioGroup } from '@/components/ui'` to:
  ```ts
  import { RadioGroup } from '@/components/ui'
  ```
  (Input is no longer used after the year swap.)

  (b) Replace the `kindOptions` computed (the whole `const kindOptions = computed…` block) with a `kindItems` computed:
  ```ts
  const kindItems = computed(() =>
    ANIME_KINDS.map(k => ({ id: k, label: t('common.filters.kind.' + k) })),
  )
  ```

  (c) Replace the Format (kind) `<FilterSection>` block in the template:
  ```vue
  <!-- Type (kind) — multi-select checkbox list (matches the List page) -->
  <FilterSection
    :label="$t('common.filters.type')"
    :count="filters.kinds.value.length"
  >
    <FilterCheckboxList
      :items="kindItems"
      :selected="filters.kinds.value"
      :searchable="false"
      @update:selected="onKindsChange"
    />
  </FilterSection>
  ```

  (d) Replace the Year range `<FilterSection>` inner content:
  ```vue
  <!-- Year range -->
  <FilterSection
    :label="$t('browse.filters.section.year')"
    :count="filters.yearFrom.value || filters.yearTo.value ? 1 : 0"
  >
    <FilterYearRange
      :min="filters.yearFrom.value"
      :max="filters.yearTo.value"
      :floor-year="MIN_YEAR"
      :ceil-year="MAX_YEAR"
      @update:min="onYearMin"
      @update:max="onYearMax"
    />
  </FilterSection>
  ```

  (e) In `<script setup>`, replace `onKindChange` with `onKindsChange`, and replace `onYearChange` with `onYearMin`/`onYearMax`:
  ```ts
  function onKindsChange(ids: string[]) {
    props.filters.kinds.value = ids
    props.filters.writeUrl()
  }
  ```
  ```ts
  function onYearMin(v: number | null) {
    props.filters.yearFrom.value = v
    props.filters.writeUrl()
  }

  function onYearMax(v: number | null) {
    props.filters.yearTo.value = v
    props.filters.writeUrl()
  }
  ```
  Delete the old `onKindChange` and `onYearChange` functions. `MIN_YEAR`/`MAX_YEAR` consts stay (now passed to FilterYearRange).

- [ ] **Step 7: Run FE gates** (DS-lint hook fires on each edit; then full check)

Run:
```bash
cd frontend/web
bunx vitest run src/components/filters/ src/composables/__tests__/useBrowseFilters.spec.ts src/constants/animeKinds.spec.ts
bash scripts/design-system-lint.sh
bun run build
```
Expected: vitest PASS (incl. animeKinds 3rd assertion now green); DS-lint `ERRORS: 0`; build succeeds with no type errors.

- [ ] **Step 8: i18n parity check**

Run: `cd frontend/web && bunx vitest run src/locales/` (locale parity specs)
Expected: PASS. If a dedicated `scripts/i18n-lint.sh` exists, run it too; it must report en/ru/ja parity for the added `common.filters.*` keys.

- [ ] **Step 9: Commit**

```bash
git add frontend/web/src/composables/useBrowseFilters.ts frontend/web/src/composables/__tests__/useBrowseFilters.spec.ts frontend/web/src/components/browse/BrowseSidebar.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(browse): multi-select anime-type + shared FilterYearRange"
```

---

### Task 5: Frontend — List (WatchlistFilters) adopts shared controls

**Files:**
- Modify: `frontend/web/src/components/profile/WatchlistFilters.vue`
- Modify: `frontend/web/src/components/profile/WatchlistFilters.spec.ts`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json` (repoint kind labels to `common.filters.*`; remove `profile.filters.kind` + `profile.filters.types`)

**Interfaces:**
- Consumes: `FilterCheckboxList` (Phase 1), `FilterYearRange` (Task 3), `common.filters.{type,kind.*}` (Task 4).
- Produces: List renders genres/kinds via `FilterCheckboxList` and year via `FilterYearRange` — visually/behaviorally identical controls to Browse for the shared axes. Emits unchanged (`update:genreIds/kinds/yearMin/yearMax`).

- [ ] **Step 1: Update the spec first** — in `frontend/web/src/components/profile/WatchlistFilters.spec.ts`, change the two kind-label assertions from the `profile.filters.kind.*` paths to `common.filters.kind.*`:
  - line ~52: `expect(wrapper.text()).toContain('profile.filters.kind.tv')` → `'common.filters.kind.tv'`
  - line ~54: `expect(wrapper.text()).toContain('profile.filters.kind.movie')` → `'common.filters.kind.movie'`

  All other assertions stay (genre toggle via `findAllComponents({ name: 'Checkbox' })[0]` still resolves to the first genre row — `FilterCheckboxList` renders `Checkbox` components; genres render before kinds; with 2 genres + 2 kinds and no search Inputs, index 0 is the first genre).

- [ ] **Step 2: Run it to verify failure** (the kind text is still rendered from `profile.filters.kind.*`)

Run: `cd frontend/web && bunx vitest run src/components/profile/WatchlistFilters.spec.ts`
Expected: FAIL on the two kind-label assertions.

- [ ] **Step 3: Rewire WatchlistFilters template** — in `frontend/web/src/components/profile/WatchlistFilters.vue`, replace the Genres `<section>` inner list, the Types `<section>` inner list, and the Year `<section>` inner control:

  Genres section — replace the `<Input v-if=…>` + `<ul>…</ul>` with:
  ```vue
  <FilterCheckboxList
    :items="genreItems"
    :selected="genreIds"
    :searchable="facets.genres.length > 8"
    :search-placeholder="$t('profile.filters.searchGenres')"
    max-height-class="max-h-64"
    @update:selected="(v) => emit('update:genreIds', v)"
  />
  ```

  Types section — replace the `<ul>…</ul>` with (and change the header label to the shared `common.filters.type`):
  ```vue
  <header class="flex items-center justify-between mb-2">
    <span class="text-sm font-semibold text-white">{{ $t('common.filters.type') }}</span>
    <span class="text-xs text-muted-foreground">{{ $t('profile.filters.typesHint') }}</span>
  </header>
  <FilterCheckboxList
    :items="kindItems"
    :selected="kinds"
    :searchable="false"
    @update:selected="(v) => emit('update:kinds', v)"
  />
  ```

  Year section — replace the two `<Select>`s with:
  ```vue
  <FilterYearRange
    :min="yearMin"
    :max="yearMax"
    :floor-year="facets.years.min as number"
    :ceil-year="facets.years.max as number"
    @update:min="(v) => emit('update:yearMin', v)"
    @update:max="(v) => emit('update:yearMax', v)"
  />
  ```

- [ ] **Step 4: Rewire WatchlistFilters script** — in the `<script setup>`:

  (a) Replace the imports of the now-unused primitives. Change:
  ```ts
  import Button from '@/components/ui/Button.vue'
  import Checkbox from '@/components/ui/Checkbox.vue'
  import Input from '@/components/ui/Input.vue'
  import Select from '@/components/ui/Select.vue'
  ```
  to:
  ```ts
  import Button from '@/components/ui/Button.vue'
  import FilterCheckboxList from '@/components/filters/FilterCheckboxList.vue'
  import FilterYearRange from '@/components/filters/FilterYearRange.vue'
  ```

  (b) Add `t` to the i18n destructure:
  ```ts
  const { locale, t } = useI18n()
  ```

  (c) Replace the genre-search state + `filteredGenres` + `toggleGenre` + `toggleKind` + the year machinery (`years`, `ANY_YEAR`, `yearMinStr`, `yearMaxStr`, `yearMinOptions`, `yearMaxOptions`, `emitYear`) with two computed item lists (delete all of those):
  ```ts
  const genreItems = computed(() =>
    props.facets.genres.map((g) => ({ id: g.id, label: localizedGenre(g), count: g.count })),
  )

  const kindItems = computed(() =>
    props.facets.kinds.map((k) => ({ id: k.kind, label: t('common.filters.kind.' + k.kind), count: k.count })),
  )
  ```
  Keep `localizedGenre`, the `count` computed, and `clearAll` unchanged. Remove the now-unused `genreSearch` ref.

- [ ] **Step 5: Repoint + prune locale keys** — in all three locales, REMOVE (now unreferenced after Step 3/4):
  - `profile.filters.kind` (the whole object)
  - `profile.filters.types`
  
  Keep `profile.filters.typesHint`, `profile.filters.genres`, `profile.filters.genresHint`, `profile.filters.searchGenres`, `profile.filters.year`, `profile.filters.clear`, `profile.filters.button`. (Before deleting, grep to confirm zero references: `grep -rn "profile.filters.types\b\|profile.filters.kind\." frontend/web/src` should return nothing after Step 3.)

- [ ] **Step 6: Run the spec + FE gates**

Run:
```bash
cd frontend/web
bunx vitest run src/components/profile/WatchlistFilters.spec.ts src/components/filters/
bash scripts/design-system-lint.sh
bun run build
```
Expected: vitest PASS; DS-lint `ERRORS: 0`; build clean.

- [ ] **Step 7: i18n parity check**

Run: `cd frontend/web && bunx vitest run src/locales/`
Expected: PASS (no orphaned/missing keys across en/ru/ja).

- [ ] **Step 8: Commit**

```bash
git add frontend/web/src/components/profile/WatchlistFilters.vue frontend/web/src/components/profile/WatchlistFilters.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -m "feat(profile): watchlist filters adopt shared FilterCheckboxList + FilterYearRange"
```

---

## Final Verification (after all tasks, before deploy)

- [ ] **Backend:** `cd services/catalog && go test ./... -race` → all pass.
- [ ] **Frontend full pre-flight:** `/frontend-verify` (DS-lint, i18n en/ru/ja parity, real `bun run build`, vitest). All green.
- [ ] **Whole-branch review:** dispatch the final code-reviewer over `merge-base(origin/main, HEAD)..HEAD` (the Phase 2 commits) with this plan's Global Constraints as the lens. Resolve Critical/Important before merge; record Minors.
- [ ] **End-to-end smoke** (after deploy): `curl -s "http://localhost:8000/api/anime?kind=tv,movie&page_size=3" | jq '.data[].kind'` returns only tv/movie; Browse anime-type shows 9 checkboxes; List + Browse anime-type controls look identical.

## Self-Review (plan author)

- **Spec coverage:** §4 anime-type unification → Tasks 1 (BE Kinds), 2 (constant), 4 (Browse multi-select), 5 (List labels). §3 FilterYearRange → Task 3 + adopted in 4 & 5. §8 List migration → Task 5 (controls; chrome intentionally page-specific, see Scope Refinements). §9 i18n → Tasks 4/5 (scoped to kind+type). §10 → Source kept (reality correction). §5/§6/§7 shipped in Phase 1.
- **Placeholder scan:** none — every step carries exact code/commands.
- **Type consistency:** `kinds: Ref<string[]>` used uniformly (composable return, sidebar `filters.kinds.value`, apiParams). `FilterYearRange` emits `number|null`; both consumers' handlers accept `number|null`. `FilterCheckboxList` item shape `{id,label,count?}` matches both `genreItems`/`kindItems` producers.
- **Each task leaves build green:** Task 4 adds `common.filters.*` before removing `browse.filters.format`; Task 5 removes `profile.filters.kind/types` only after repointing. Between Task 4 and 5, List still uses `profile.filters.kind` (present) → no missing key.

## Metrics

`UXΔ = +2 (Better)` — anime-type now corresponds across both pages (same 9 values, multi-select, shared labels); identical genre/year controls; one source of truth resists drift.
`CDI = 0.10 * 21` — Spread×Shift = 0.10 (catalog BE + Browse FE + List FE + i18n×3, modest per-area shift); Effort_Fib = 21.
`MVQ = Griffin 87%/85%` — well-bounded consolidation; high slop-resistance (canonical constant + shared components remove the drift vector).
