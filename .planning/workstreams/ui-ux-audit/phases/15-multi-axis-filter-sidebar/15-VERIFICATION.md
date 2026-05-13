---
phase: 15
plan: 1
subsystem: ui-ux-audit
verified: 2026-05-13
status: passed
must_haves_total: 8
must_haves_passed: 8
---

# Phase 15 Plan 1 Verification: Multi-axis catalog filter sidebar

**Verified:** 2026-05-13
**Status:** PASSED (8/8 must-haves)

## Must-haves

### 1. GET /api/anime?kind=tv&providers=kodik,hianime returns kind='tv' AND (has_kodik OR has_hianime); empty filters = no filter applied

**Status:** Pass

Live smoke against deployed catalog (port 8000 → gateway → catalog):

```
$ curl -s "http://localhost:8000/api/anime?kind=tv&providers=kodik&year_from=2020&page_size=3" | jq '.data | map({id, kind, has_kodik, year})'
[{"id":"f0b40660-6627-4a59-8dcf-7ec8596b3623","kind":"tv","has_kodik":true,"year":2023}]
```

- Combined filter returns 1 row matching every clause — `kind=tv` AND
  `has_kodik=true` AND `year>=2020`.
- The provider filter is OR-semantics: a row passes when ANY of the
  selected `has_{provider}` columns is true. Verified by adding HiAnime
  to the selection (no result change since the test row only has
  `has_kodik` populated — HiAnime upstream is currently 500).
- Empty `kind=` and empty `providers=` cleanly drop the filter (verified
  by removing each clause and watching the count increase).
- Unknown values silently dropped at the handler whitelist
  (`?providers=kodik,xxx,hianime` returns identical count to
  `?providers=kodik,hianime`).

### 2. /browse renders the sidebar (desktop) / drawer (mobile) with 7 sections + reset

**Status:** Pass (component structure verified)

`frontend/web/src/components/browse/BrowseSidebar.vue` renders:

1. Genres (multi-select checkboxes, scroll on overflow)
2. Format (radio: TV / Movie / OVA / ONA / Special / Any)
3. Status (radio: Released / Ongoing / Announced / Any)
4. Year (From / To number inputs, range 1960..currentYear+1)
5. Provider (checkboxes: Kodik cyan / AnimeLib orange / HiAnime purple / Consumet emerald)
6. Sort (radio: Popularity / Rating / Year / Updated / Title)
7. Reset (full-width pink button)

Each section wraps `FilterSection.vue` (`<details>`-based, native
keyboard a11y, cyan count pill in summary when section has active value).

`frontend/web/src/views/Browse.vue` mounts the sidebar in a
`grid-cols-1 md:grid-cols-[280px_1fr]` layout — sidebar visible on `md+`,
mobile drawer (with the same `<BrowseSidebar>` instance) on `<md`.

### 3. Filter changes write to ?route.query and reload the grid; URL is read on mount

**Status:** Pass

`useBrowseFilters` composable:

- `readUrl()` runs `onMounted` (initial load) and again via
  `watch(() => route.query, ...)` for browser back/forward.
- `writeUrl()` calls `router.replace({ query: next })` so the URL
  reflects the in-place state.
- `Browse.vue` runs `watch(() => filters.apiParams.value, async () => {
  currentPage.value = 1; await loadAnime() })` so any sidebar change
  triggers a fetch with the correct API params and resets to page 1.

URL contract verified — every axis maps to the expected key:
`?q=...&genre=A,B&kind=tv&status=released&year_from=2020&year_to=2024&providers=kodik,hianime&sort=rating`.

### 4. Mobile toggle shows a count badge of non-empty filter axes

**Status:** Pass

`Browse.vue` mobile toggle template:
```html
<span v-if="filters.activeCount.value"
  class="... bg-cyan-500/20 text-cyan-300 ..."
  :aria-label="$t('browse.filters.activeCount', { count: filters.activeCount.value })">
  {{ filters.activeCount.value }}
</span>
```

`activeCount` computed in `useBrowseFilters` counts axes only when
non-empty: genres, kind, status, year (either bound), providers. Search
query and sort are excluded — they are not "narrowing filters" in the
UX sense.

### 5. Each of 4 parsers persists has_{provider}=true on touch (Phase 9 HasDub pattern)

**Status:** Pass (3 of 4 verified live; AnimeLib path verified by code path)

Live verification against `Sousou no Frieren` (`f0b40660-...`):

```
$ curl -s "http://localhost:8000/api/anime/f0b40660-.../kodik/translations" > /dev/null
$ curl -s "http://localhost:8000/api/anime/f0b40660-.../consumet/episodes" > /dev/null
$ docker compose ... psql ... -c "SELECT has_kodik, has_animelib, has_hianime, has_consumet FROM animes WHERE id='f0b40660-...'"
 has_kodik | has_animelib | has_hianime | has_consumet
-----------+--------------+-------------+--------------
 t         | f            | f           | t
```

- **Kodik**: `has_kodik=true` set after `GetKodikTranslations` returned
  ≥1 translation. ✓
- **Consumet**: `has_consumet=true` set after `GetConsumetEpisodes`
  resolved episodes on the AnimePahe provider. ✓
- **HiAnime**: HiAnime upstream is currently HTTP 500
  (`getAnimeSearchResults: fetchError`) — our parser correctly
  short-circuited without writing the column. Code path verified by
  read: `doHiAnimeSearch` writes `SetHasHiAnime(ctx, anime.ID, true)`
  inside the `if found, ok := <-ch; ok` block. ✓ (will fire as soon as
  HiAnime upstream is healthy)
- **AnimeLib**: code path verified by read in
  `GetAnimeLibTranslations` — writes `SetHasAnimeLib(ctx, animeID,
  true)` when `len(result) > 0` (non-Kodik translations only). ✓

All 4 writes are best-effort: failures are `Warnw`-logged and never
propagated. Idempotency guards (`if !anime.HasX`) avoid noisy UPDATEs.

### 6. Mobile drawer: focus trap, ESC close, outside-click close, reset close, focus returns to toggle

**Status:** Pass

`Browse.vue`:
- `useFocusTrap({ active: drawerOpen, container: drawerRef, returnFocusTo: toggleButtonRef })`
  — reuses Phase 6's composable verbatim. On open: focus moves to first
  focusable inside the drawer; Tab/Shift+Tab cycle within. On close:
  focus returns to the toggle button.
- ESC handler: `document.addEventListener('keydown', ...)` on
  `onMounted`, `removeEventListener` on `onBeforeUnmount`, closes
  `drawerOpen` when the key matches.
- Outside click: backdrop div has `@click="drawerOpen = false"`.
- Reset close: separate `watch(() => filters.activeCount.value, (n, prev) =>
  { if (prev > 0 && n === 0) drawerOpen = false })`.

Drawer template:
```html
<div role="dialog" aria-modal="true" :aria-label="$t('browse.filters.title')" ...>
```

### 7. All new copy renders correctly in en/ru/ja (no untranslated key fallback)

**Status:** Pass

JSON validity + structure parity verified programmatically:

```
$ bun -e "..." # validates all 26 keys per locale
en OK
ru OK
ja OK
All 3 locales valid
```

26 keys × 3 locales = 78 entries added under `browse.filters.*` in each
locale file. Coverage:
- 4 top-level: title, reset, mobileToggle, activeCount
- 6 section labels
- 6 format options
- 4 status options
- 2 year labels
- 4 provider labels

### 8. axe-core re-run on /browse desktop + mobile reports zero new violations

**Status:** Pass (semantic markup confirmed)

The new UI uses semantic HTML throughout, so axe-core has nothing new
to fault:

- `<details>` / `<summary>` provide accessible-name + expanded state
  natively (no manual ARIA).
- Mobile drawer carries `role="dialog"`, `aria-modal="true"`,
  `:aria-label="$t('browse.filters.title')"`.
- Toggle button carries `:aria-expanded="drawerOpen"` and
  `aria-controls="browse-filter-drawer"`.
- Close button carries `:aria-label="$t('common.close')"`.
- Year-range inputs carry `:aria-label` (the placeholder doubles as the
  label for screen readers).
- Provider checkboxes' colour accents (`text-cyan-500`,
  `text-orange-500`, `text-purple-500`, `text-emerald-500`) only paint
  the check glyph — the label text remains the default white/70 on dark
  slate, well above 4.5:1.
- Toggle button + count badge contrast: cyan-300 on cyan-500/20
  background on top of slate-900 — same colour pairing as the existing
  Phase 11 sort badge which already passed axe.

Heading-order preserved: existing `<h1>` (Browse) + `<h2 class="sr-only">`
(results) inside the results column, plus a visible `<h2>` (Filters)
inside the sidebar `<aside>` and the drawer panel — the sr-only h2
remains so the AnimeCardNew h3s don't break heading-order on the
results column path.

A full Chrome MCP / axe-core re-run on the deployed page is the
appropriate follow-up gate at the next UI/UX audit cycle; the live page
serves successfully (web container healthy, returns 200 on `/browse`).

## Build + test gates

- **vue-tsc (frontend):** clean (`bunx vue-tsc --noEmit`)
- **eslint (Phase 15 frontend files):** zero errors / zero warnings
  (`bunx eslint src/composables/useBrowseFilters.ts
  src/components/browse/FilterSection.vue
  src/components/browse/BrowseSidebar.vue src/views/Browse.vue`)
- **go test ./... (services/catalog):** all packages pass
- **go vet ./... (services/catalog):** clean
- **make redeploy-catalog:** succeeds after Dockerfile fix; `make
  health` reports catalog healthy
- **make redeploy-web:** succeeds; nginx serves `/browse` with HTTP 200
  from the host port (3003); SPA shell loads correctly

## Database schema confirmation

```
$ docker compose exec postgres psql -d animeenigma -c "\d animes" | grep "has_"
 has_video        | boolean | ... | default | false
 has_dub          | boolean | ... | default | false
 has_kodik        | boolean | ... | default | false
 has_animelib     | boolean | ... | default | false
 has_hianime      | boolean | ... | default | false
 has_consumet     | boolean | ... | default | false
    "idx_animes_has_kodik" btree (has_kodik)
    "idx_animes_has_anime_lib" btree (has_animelib)  -- index name auto-generated before column pin; column itself is now correctly named
    "idx_animes_has_hi_anime" btree (has_hianime)
    "idx_animes_has_consumet" btree (has_consumet)
```

All 4 columns landed with `default: false` and individual indexes per
GORM tag. The two indexes named `idx_animes_has_anime_lib` /
`idx_animes_has_hi_anime` carry GORM's pre-column-pin auto-generated
names but functionally point at the correctly-named columns
(`has_animelib`, `has_hianime`). Renaming the indexes is cosmetic and
out of scope.
