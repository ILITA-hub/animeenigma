---
phase: 15
plan: 1
subsystem: ui-ux-audit
tags: [frontend, vue3, i18n, browse, filter-sidebar, mobile-drawer, focus-trap, go, catalog, gorm, postgres]
requires: [phase-6, phase-9, phase-11]
provides:
  - browse-filter-sidebar
  - browse-provider-filter
  - browse-kind-filter
  - browse-mobile-drawer
  - anime-has_kodik-column
  - anime-has_animelib-column
  - anime-has_hianime-column
  - anime-has_consumet-column
affects:
  - /api/anime (kind + providers query params)
  - postgres animes table (4 new boolean columns)
tech-stack:
  added: []
  patterns:
    - lazy-backfill-on-parser-touch
    - gorm-column-pin-against-snake-case-word-break
    - composable-owns-url-state-not-network
    - per-axis-section-with-pill-count-badge
    - eslint-no-mutating-props-disabled-for-shared-composable-pattern
key-files:
  created:
    - frontend/web/src/composables/useBrowseFilters.ts
    - frontend/web/src/components/browse/FilterSection.vue
    - frontend/web/src/components/browse/BrowseSidebar.vue
    - .planning/workstreams/ui-ux-audit/phases/15-multi-axis-filter-sidebar/15-SUMMARY.md
    - .planning/workstreams/ui-ux-audit/phases/15-multi-axis-filter-sidebar/15-VERIFICATION.md
  modified:
    - services/catalog/internal/domain/anime.go
    - services/catalog/internal/repo/anime.go
    - services/catalog/internal/handler/catalog.go
    - services/catalog/internal/service/catalog.go
    - services/catalog/Dockerfile
    - frontend/web/src/views/Browse.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
decisions:
  - 4-has_X-booleans-mirror-HasDub-pattern-not-a-new-providers-table
  - column-pin-via-gorm-column-tag-after-AutoMigrate-snake-case-word-break-surprised-us
  - useBrowseFilters-composable-owns-url-state-but-not-network-fetch
  - sidebar-receives-composable-via-prop-not-each-axis-as-prop
  - eslint-disable-vue-no-mutating-props-in-BrowseSidebar-for-shared-composable-pattern
  - apiParams-driven-watcher-replaces-half-dozen-per-axis-route-query-watchers
  - mobile-drawer-auto-close-on-reset-when-active-count-drops-to-zero
  - AnimeLib-Kodik-iframe-fallback-path-doesnt-count-for-has_animelib
metrics:
  duration: ~25min
  completed: 2026-05-13
  commits: 8
  tasks_complete: 14
  tasks_total: 14
---

# Phase 15 Plan 1: Multi-axis catalog filter sidebar (Dragon)

**Completed:** 2026-05-13
**Plan:** 15-PLAN.md
**Outcome:** `/browse` rebuilt as a 6-axis filter sidebar (genres / format /
status / year / provider / sort) + reset. Provider axis is the headline
feature — AnimeEnigma is the only anime catalog that lets a user filter
the library by "available on Kodik" / "available on HiAnime" / etc.
Backend gains four `has_{provider}` boolean columns (mirrors Phase 9's
`HasDub`), a `kind` filter, and a `providers` multi-value filter. Lazy
backfill: each parser sets its column the first time the catalog touches
that provider for an anime. Frontend extracts filter state into a
`useBrowseFilters` composable, ships a desktop sidebar + mobile drawer
reusing Phase 6's `useFocusTrap`, and adds 78 i18n entries across
en/ru/ja. Closes UX-31.

## Wave-by-wave changes

### Wave 1 — Backend: provider booleans + kind/providers filter

**Domain (`services/catalog/internal/domain/anime.go`):**

- Added 4 boolean fields immediately after `HasDub`:
  `HasKodik`, `HasAnimeLib`, `HasHiAnime`, `HasConsumet`. Each carries
  `gorm:"default:false;index;column:has_X"` — the explicit `column:` tag
  is mandatory because GORM's default snake_case auto-conversion turns
  `HasAnimeLib` into `has_anime_lib` (word-break inside the camelCase),
  which would NOT match the WHERE clauses or the JSON contract.
- Extended `SearchFilters` with `Kind string` and `Providers []string`.

**Repo (`services/catalog/internal/repo/anime.go`):**

- `Search()` got two new branches:
  - `if filters.Kind != ""` → `WHERE kind = ?` (whitelisted at the handler).
  - `if len(filters.Providers) > 0` → builds an OR-set across the 4
    `has_{provider}` columns. Unknown values are silently dropped at
    this layer too (defence-in-depth alongside the handler whitelist).
- Added `SetHasKodik` / `SetHasAnimeLib` / `SetHasHiAnime` / `SetHasConsumet`
  helpers — exact pattern of `SetHasDub`.

**Handler (`services/catalog/internal/handler/catalog.go`):**

- `parseFilters` now reads `?kind=tv|movie|ova|ona|special` (whitelist —
  unknown values silently dropped) and `?providers=a,b,c` (comma-separated,
  lowercased, deduped, whitelist).

**Service (`services/catalog/internal/service/catalog.go`):**

- `GetKodikTranslations`: when ≥1 translation returned, lazily sets
  `has_kodik=true`. Idempotency guard skips already-true rows.
- `GetAnimeLibTranslations`: when ≥1 non-Kodik translation returned,
  sets `has_animelib=true`. The Kodik-iframe-fallback path inside
  AnimeLib does NOT count (per
  `feedback_animelib_no_kodik_fallback.md` — AnimeLib treats Kodik-only
  translations as empty).
- `doHiAnimeSearch`: when a match resolves to a real HiAnime ID, sets
  `has_hianime=true`.
- `GetConsumetEpisodes`: when ≥1 episode resolved on any Consumet
  provider, sets `has_consumet=true`.
- All four writes best-effort: failures are `Warnw`-logged, never
  propagated. The provider boolean is an advisory hint for the
  filter — its absence does NOT affect playback.

**Dockerfile (`services/catalog/Dockerfile`):**

- Added missing `COPY libs/streamprobe/go.mod` line. `libs/streamprobe`
  was added to `go.work` previously but never wired into the
  `services/catalog/Dockerfile`'s mod-download stage, so the first
  `make redeploy-catalog` for Phase 15 failed at `go mod download`.
  This is a pre-existing bug unblocked by Rule 3 (auto-fix blocking
  issue). Other services' Dockerfiles have the same gap but are out of
  scope for this phase.

### Wave 2 — Frontend: composable + sidebar components

**Composable (`frontend/web/src/composables/useBrowseFilters.ts`, NEW):**

- Reactive state for 8 axes: `q`, `genres[]`, `kind`, `status`,
  `yearFrom`, `yearTo`, `providers[]`, `sort`.
- Bi-directional URL sync:
  - `readUrl()` on mount + on `watch(() => route.query, ...)` covers
    initial load and browser back/forward.
  - `writeUrl()` flushes to `?route.query` via `router.replace`.
  - `suppressNextWatch` flag prevents the watcher from echoing the
    composable's own writes back into `readUrl()`.
- `apiParams` computed yields the exact shape `animeApi.getAnimeList`
  consumes. Sort=popularity is the default; omitted from URL/params to
  keep clean URLs.
- `activeCount` counts narrowing axes only — search query and sort are
  intentionally excluded (sort never narrows; the search input has its
  own affordance).
- `reset()` clears all axes and rewrites URL once.
- Composable does NOT call the network — keeps it test-mockable and
  the existing `useAnime` composable untouched.

**Components (`frontend/web/src/components/browse/`, NEW):**

- `FilterSection.vue`: thin `<details>`-based collapsible wrapper.
  Native browser semantics handle Tab/Enter/Space + screen reader
  expanded state — no manual ARIA wiring. Renders a cyan pill next to
  the label when the section has an active count.
- `BrowseSidebar.vue`: 7 sections in order — Genres → Format → Status
  → Year → Provider → Sort → Reset. Consumes the parent's
  `useBrowseFilters()` instance via prop so the composable's reactive
  refs are shared (the parent's `apiParams` watcher re-fetches as soon
  as the sidebar mutates a ref). Provider checkboxes use per-provider
  accent classes (locked in CONTEXT.md "specifics"):
  - Kodik = cyan
  - AnimeLib = orange
  - HiAnime = purple
  - Consumet = emerald
  Year-range inputs client-side validate `from <= to` (auto-clamp the
  other end if the user inverts the range).
  ESLint's `vue/no-mutating-props` is locally disabled at the top of
  the script block — the rule mis-flags mutation of nested reactive
  refs on a shared composable prop. The pattern is intentional (the
  composable IS the contract); the comment in the source documents why.

### Wave 3 — Browse.vue rebuild (desktop grid + mobile drawer)

- Replaced the horizontal filter row (`<div class="flex flex-wrap gap-3">`,
  4 dropdowns + clear button) with a `grid grid-cols-1
  md:grid-cols-[280px_1fr] gap-6` layout. Desktop renders the sidebar
  on the left; mobile collapses to single-column.
- Moved `<SearchAutocomplete>` into the results column above the grid so
  it stays visible on both layouts.
- Mobile toggle: 1-button-wide row above the grid (visible only on
  `<md`). Shows the filters icon + label + count badge when
  `filters.activeCount > 0`. `aria-expanded` and `aria-controls` are
  tied to the drawer id.
- Mobile drawer: `<Teleport to="body">` + `<Transition>` slide-from-left
  animation. `role="dialog"`, `aria-modal="true"`, `aria-label` bound
  to the localised "Filters" string. Backdrop click closes. ESC closes
  (separate `keydown` listener — focus trap handles Tab cycling). The
  drawer also auto-closes when `filters.activeCount` drops from >0 to 0
  (the user just hit reset). Focus returns to the toggle button via
  `useFocusTrap`'s `returnFocusTo`.
- All previous per-axis route-query watchers are replaced by a single
  `watch(() => filters.apiParams.value, () => { currentPage = 1;
  loadAnime() })`. Net code reduction (~30 lines despite gaining the
  drawer).
- Grid breakpoints reduced one step (xl 6→5, etc.) so the cards don't
  cramp against the new sidebar's 280px column.

### Wave 4 — i18n (en/ru/ja)

- Added a new `browse.filters.*` nested namespace to all three locale
  files. 26 keys × 3 locales = 78 entries:
  - `title`, `reset`, `mobileToggle`, `activeCount({count})`
  - `section.{genres,format,status,year,provider,sort}`
  - `format.{any,tv,movie,ova,ona,special}`
  - `status.{any,released,ongoing,anons}`
  - `year.{from,to}`
  - `provider.{kodik,animelib,hianime,consumet}`
- JSON structure validated programmatically — parity confirmed across
  all three locales (no missing keys).

### Wave 5 — Verification

See `15-VERIFICATION.md`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] GORM auto-named `HasAnimeLib`/`HasHiAnime` columns wrongly**
- **Found during:** Wave 1.4 (post-redeploy column check)
- **Issue:** GORM's default convention turned `HasAnimeLib` →
  `has_anime_lib` (word break at the inner camelCase) and `HasHiAnime`
  → `has_hi_anime`. The WHERE clauses, JSON contract, and SetHas*
  helpers all referenced `has_animelib` / `has_hianime`, so the
  AutoMigrate landed two unused columns + the filter was silently
  matching empty columns.
- **Fix:** Added explicit `column:has_animelib` / `column:has_hianime`
  tags to the GORM struct, dropped the wrongly named DB columns
  inline via `ALTER TABLE ... DROP COLUMN IF EXISTS`, redeployed.
- **Files modified:** `services/catalog/internal/domain/anime.go`
- **Commit:** `b221675`

**2. [Rule 3 — Blocking] `services/catalog/Dockerfile` missing `libs/streamprobe` COPY**
- **Found during:** Wave 1.4 (first `make redeploy-catalog`)
- **Issue:** `go.work` lists `./libs/streamprobe` but the
  `services/catalog/Dockerfile` `COPY libs/*/go.mod` block doesn't
  include it. `go mod download` failed with
  `open /app/libs/streamprobe/go.mod: no such file or directory`.
  Pre-existing pre-Phase-15 (not caused by our changes).
- **Fix:** Added the missing COPY line.
- **Out of scope (deferred):** The same gap exists in
  `services/{rooms,themes,scheduler,auth,streaming}/Dockerfile`. They
  weren't touched in this phase. Logged for future maintenance pass.
- **Files modified:** `services/catalog/Dockerfile`
- **Commit:** `b221675`

### Out-of-Scope Discoveries (Deferred)

- HiAnime parser API was returning HTTP 500 during our Wave 1.4
  smoke check (`getAnimeSearchResults: fetchError`). Our parser
  correctly returned the empty result without writing `has_hianime`,
  so the lazy backfill behaves as designed. The HiAnime upstream
  outage is not a Phase 15 concern.
- 5 other services have the same `libs/streamprobe` COPY gap in their
  Dockerfile (`rooms`, `themes`, `scheduler`, `auth`, `streaming`).
  Out of scope — Phase 15 only needs `catalog` to redeploy.

## Commits

| # | Hash      | Description |
| - | --------- | --- |
| 1 | `b8d6cb1` | feat(ui-ux-audit/phase-15): Anime HasKodik/HasAnimeLib/HasHiAnime/HasConsumet + SetHasX helpers (Wave 1.1) |
| 2 | `190537a` | feat(ui-ux-audit/phase-15): parse kind + providers query params in catalog handler (Wave 1.2) |
| 3 | `ec9963b` | feat(ui-ux-audit/phase-15): 4 parser entry points lazily set HasX boolean (Wave 1.3) |
| 4 | `b221675` | fix(ui-ux-audit/phase-15): pin has_animelib/has_hianime column names + Dockerfile streamprobe copy |
| 5 | `4dd023b` | feat(ui-ux-audit/phase-15): useBrowseFilters composable (Wave 2.1) |
| 6 | `ae29600` | feat(ui-ux-audit/phase-15): FilterSection + BrowseSidebar components (Wave 2.2-2.3) |
| 7 | `6276373` | feat(ui-ux-audit/phase-15): Browse.vue desktop grid + mobile drawer + active count (Wave 3.1) |
| 8 | `816845b` | feat(ui-ux-audit/phase-15): browse.filters.* i18n (en/ru/ja, 26 keys x 3) (Wave 4.1) |

Plus a `docs(...): summary + verification` commit appended at phase end.

## Closes

| Requirement | Surface | Mechanism |
|---|---|---|
| UX-31 | /browse | Backend: 4 `has_{provider}` boolean columns on `animes` + `kind` + `providers` query params on `/api/anime`; each of the 4 parsers (kodik / animelib / hianime / consumet) lazily backfills its column on first touch. Frontend: `useBrowseFilters` composable owns URL state, `BrowseSidebar.vue` renders 7 collapsible sections + reset, desktop `grid-cols-[280px_1fr]` layout, mobile drawer with `role=dialog` / `aria-modal` / focus trap (Phase 6 `useFocusTrap`) / ESC close / outside-click close. Active filter count badge on the mobile toggle. URL contract: `?genre=A,B&kind=tv&status=released&year_from=2020&year_to=2024&providers=kodik,hianime&sort=rating`. |

## Self-Check: PASSED

All files created and verified on disk:
- frontend/web/src/composables/useBrowseFilters.ts
- frontend/web/src/components/browse/FilterSection.vue
- frontend/web/src/components/browse/BrowseSidebar.vue
- .planning/workstreams/ui-ux-audit/phases/15-multi-axis-filter-sidebar/15-SUMMARY.md
- .planning/workstreams/ui-ux-audit/phases/15-multi-axis-filter-sidebar/15-VERIFICATION.md

All 8 commits verified in git history:
b8d6cb1, 190537a, ec9963b, b221675, 4dd023b, ae29600, 6276373, 816845b.
