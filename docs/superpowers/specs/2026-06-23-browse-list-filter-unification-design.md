# Browse + List Filter Unification — Design Spec

**Date:** 2026-06-23
**Status:** Approved (design); pending spec review → plan
**Scope:** Frontend filter components (Browse + Profile watchlist) + catalog backend filter plumbing

## 1. Context & Problem

Two pages expose anime filters with divergent design and functionality:

- **Browse** — `frontend/web/src/views/Browse.vue` + `components/browse/BrowseSidebar.vue` + `composables/useBrowseFilters.ts`. Server-side via catalog `GET /api/anime` (`animeApi.getAnimeList`).
- **List (Profile watchlist tab)** — `frontend/web/src/views/Profile.vue` + `components/profile/WatchlistFilters.vue` + `types/watchlist-facets.ts`. Server-side via player `GET /users/watchlist` (+ `/users/watchlist/facets`).

Both are server-side filtered and share ~80% of their controls, but diverge:

| Axis | Browse | List | Divergence |
|---|---|---|---|
| Genres | Checkbox list, **no search** | Checkbox list **with search** | Browse behind |
| **Anime type** | RadioGroup **single**, hardcoded 5 + "Any" | Checkbox **multi**, server facets, 9 values | **Real mismatch** |
| Year range | two numeric `Input`s | two `Select` dropdowns | Cosmetic |
| Section chrome | `FilterSection` collapsibles | flat 3-col grid | Cosmetic |
| Clear button | pink full-width | ghost | Cosmetic |
| i18n | `browse.filters.*` | `profile.filters.*` | Duplicated |

Additionally, three Browse-specific asks:
1. **Add a Studio filter** (studio data already exists; see §6).
2. **Fix the "Available on" filter** — currently hardcoded `kodik / animelib / english`; AniLib's player is hidden, Raw is missing, no first-party option.
3. **Add a search box to the Genres filter** — same component as the studio search.

## 2. Goals / Non-Goals

**Goals**
- Extract a shared filter-component library both pages consume, so they look and behave the same and cannot silently drift.
- Make the anime-type control correspond across pages: same values, same labels, same control (multi-select), same backend semantics.
- Ship the three Browse asks (studio filter, available-on fix, genre search) **through** the shared library.

**Non-Goals**
- Do NOT unify genuinely page-specific filters: Browse-only `provider/available-on`, `season`, site-`score_min`; List-only watch-status pills, view-mode, bulk-select, personal-score sort. Watchlist items lack season/provider/site-score data, so sharing those would require backend changes not worth the cost (YAGNI).
- Do NOT expose individual EN scraper providers (gogoanime/animepahe/…) as availability options — that needs new `has_{provider}` columns + a population pipeline and exposes internal failover detail.
- No DB schema changes (studio m2m + all required `has_*` columns already exist).

## 3. Approach — shared `components/filters/` library

Create `frontend/web/src/components/filters/` with reusable, page-agnostic controls. Both Browse and the Profile watchlist import them, parameterized by props. Rejected alternatives: (a) List importing `BrowseSidebar` directly — couples catalog-only filters into the watchlist; (b) re-skinning List to match Browse cosmetically — drifts again (today's problem).

### Shared components

**`FilterCheckboxList.vue`** — the workhorse. Multi-select checkbox list with an optional built-in search box.
- Props: `items: { id: string; label: string; count?: number }[]`, `selected: string[]` (v-model), `searchable?: boolean` (default true), `searchPlaceholder?: string`, `maxHeightClass?: string` (default the existing `max-h-48` scroll).
- Emits: `update:selected`.
- Behavior: client-side substring filter over `label` (case/locale-insensitive); preserves selection across searches; shows `count` when provided (List facets).
- Consumers: **genres (both pages), studios (Browse), anime-type (both pages)**. This single component satisfies "add genre search, same component as studio" — they are the same component instance.

**`FilterSection.vue`** — promote the existing `components/browse/FilterSection.vue` into `components/filters/` (label + optional count + slot; collapsible). List adopts it so section chrome matches.

**`FilterYearRange.vue`** — unify the year control on the two-`Select` form (DS-preferred over bare numeric `Input`; Rule 5 favors primitives).
- Props: `min: number|null`, `max: number|null` (v-model:min / v-model:max), `floorYear`, `ceilYear`.
- Builds descending year option lists; enforces min ≤ max.

All three live under one namespace; co-located `*.spec.ts` each (≥5 assertions, per repo convention).

## 4. Anime-type unification (the mismatch fix)

- **Canonical constant** `frontend/web/src/constants/animeKinds.ts`:
  ```ts
  export const ANIME_KINDS = ['tv','movie','ova','ona','special','tv_special','music','cm','pv'] as const
  export type AnimeKind = typeof ANIME_KINDS[number]
  ```
  Single source of truth for values + order. Both pages import it. i18n keys live under `common.filters.kind.*` (migrated from `browse.filters.format.*` + `profile.filters.kind.*`).
- **Browse** switches anime-type from single-select RadioGroup → **multi-select `FilterCheckboxList`** over `ANIME_KINDS`. Drop the "Any" sentinel (none-selected = any, like genres).
- **List** keeps server facets for **counts/presence** but renders labels from `common.filters.kind.*` via the same `FilterCheckboxList` (already multi-select).
- **Backend (catalog)**: `SearchFilters.Kind string` → `Kinds []string`. Handler parses comma-separated `kind` (mirror `GenreIDs`/`Providers`); repo applies `kind IN (?)`. The List backend already accepts comma-separated `kind`, so after this both correspond exactly.

## 5. Available-on filter fix (Browse)

Replace the 3 hardcoded options with **4**, all backed by existing populated columns. Drop AniLib (player hidden).

| Option (label) | Filter value | Column | Notes |
|---|---|---|---|
| Kodik (RU) | `kodik` | `has_kodik` | unchanged |
| English (Dub) | `dub` | `has_dub` | **dub-only** (per owner); replaces vague `english`/`has_english` |
| RAW (JP) | `raw` | `has_raw` | newly exposed |
| AnimeEnigma | `ae` | `has_video` | first-party self-hosted (set by `SetHasVideo`) |

- **Frontend** (`BrowseSidebar.vue` + `useBrowseFilters.ts`): `Provider` type → `'kodik' | 'dub' | 'raw' | 'ae'`; option list with brand-exempt accent hues (Kodik cyan, Raw rose, AnimeEnigma `#00d4ff`-family cyan, Dub a semantic/brand token). Remains a multi-select OR-set (existing semantics).
- **Backend** (`handler/catalog.go` + `repo/anime.go`): whitelist `{kodik, dub, raw, ae}`; `colsByKey = {kodik:has_kodik, dub:has_dub, raw:has_raw, ae:has_video}`; drop `animelib`/`english`. (Leave `has_animelib`/`has_english` columns + data intact — just unexposed.)
- i18n: `browse.filters.provider.*` → `{kodik, dub, raw, ae}` across en/ru/ja.

## 6. Studio filter (Browse)

Studio data already exists — `domain.Studio` + `Anime.Studios` (`many2many:anime_studios`), hydrated from Shikimori `studios { id name }`. No schema change.

- **Options endpoint**: `GET /api/studios` (mirror `/api/genres`). Catalog handler → service → repo: return studios with **≥1 anime**, ordered by anime count desc, then name. Add gateway route `/api/studios → catalog` (mirror `/api/genres`).
- **Backend filter**: `SearchFilters.StudioIDs []string`; handler parses comma-separated `studio`; repo filters via `anime_studios` (EXISTS subquery / join, mirroring the existing genre m2m filter in `repo/anime.go`).
- **Frontend**: a `FilterCheckboxList` instance (searchable) in `BrowseSidebar.vue`; `studios: string[]` in `useBrowseFilters.ts` (URL param `studio`, api param `studio`); fetch options on mount like genres. i18n `browse.filters.section.studios` + `common.filters.searchStudios` (×3 locales).

## 7. Genre search (Browse)

Browse genres become a `FilterCheckboxList` with `searchable: true` — no bespoke work beyond adopting the shared component. List genres already have search; both now use the same instance.

## 8. List page migration

`WatchlistFilters.vue` adopts `FilterCheckboxList` (genres, kind), `FilterSection` (section chrome), `FilterYearRange` (replacing its two ad-hoc `Select`s). Watch-status pills, view-mode, bulk-select, sort-direction toggle, and personal-score sort stay as-is (page-specific). Net effect: identical filter controls/visuals to Browse for the shared axes.

## 9. i18n plan

- New shared namespace `common.filters.*`: `genres, studios, type, year, clear, searchGenres, searchStudios, searchPlaceholder`, plus `common.filters.kind.{tv,movie,ova,ona,special,tv_special,music,cm,pv}`.
- Migrate Browse `format.*` and List `kind.*` usages to `common.filters.kind.*`.
- Keep page-specific keys (`browse.filters.provider/season/score`, `profile.filters.*` non-shared) where they are.
- All keys added to **en.json, ru.json, ja.json** (parity specs + `i18n-lint.sh` enforce this).

## 10. Backend API change summary (catalog)

| Change | File(s) |
|---|---|
| `SearchFilters.Kind string` → `Kinds []string` | `domain/anime.go`, `handler/catalog.go`, `repo/anime.go` |
| `SearchFilters.StudioIDs []string` + `studio` param + m2m filter | `domain/anime.go`, `handler/catalog.go`, `repo/anime.go` |
| Available-on whitelist/colsByKey → `{kodik,dub,raw,ae→has_video}` | `handler/catalog.go`, `repo/anime.go` |
| `GET /api/studios` options endpoint | `handler/catalog.go`, `service/*`, `repo/anime.go`, transport router |
| Gateway route `/api/studios → catalog` | `services/gateway/...` |

(Remove the dead `SearchFilters.Source` field while here — parsed but never applied.)

## 11. Testing strategy

- **Shared components**: `FilterCheckboxList.spec.ts` (search filters list, selection toggles, count render), `FilterYearRange.spec.ts`, plus updated `BrowseSidebar`/`WatchlistFilters` specs.
- **Catalog**: go tests for multi-kind `IN`, studio m2m filter, `/studios` endpoint, available-on column mapping (extend `repo/anime_update_test.go` style).
- **Gate**: `/frontend-verify` (DS-lint, i18n en/ru/ja parity, real `bun run build`, vitest) before commit. The DS-lint `PostToolUse` hook fires on every `.vue` edit during implementation (this is its first real-edit exercise).
- **Backend**: `cd services/catalog && go test ./... -race`.

## 12. Phasing

- **Phase 1** — shared `components/filters/` lib + apply to **Browse**: studio filter (+ `/api/studios`), available-on fix, genre search. Ships visible value. (`Effort_Fib ≈ 13`)
- **Phase 2** — anime-type unification (canonical const, Browse multi-kind FE+BE, `common.filters.kind.*`) + migrate **List** onto the shared components. (`Effort_Fib ≈ 21`)

## 13. Metrics

`UXΔ = +3 (Better)` — consistent filters across pages, new studio filter, accurate availability, searchable lists.
`CDI = 0.12 * 34` — Spread×Shift = 0.12 (FE Browse + FE List + catalog BE + gateway + i18n×3, moderate per-area shift); Effort_Fib = 34 total (P1 ≈ 13, P2 ≈ 21).
`MVQ = Griffin 88%/82%` — balanced, well-bounded consolidation; high slop-resistance (single source of truth resists drift).

## 14. Risks / Open questions

- **`has_dub` semantics**: assumed = English dub availability (the only dub pipeline is the EN scraper). If a non-EN dub flag ever shares the column, the "English (Dub)" label would need scoping. Acceptable today.
- **List kind via facets vs canonical set**: List shows only kinds present in the user's list (counts), Browse shows the full canonical set — values/labels/control match, list membership differs by design.
- **Studio list size**: handled by the search box; `/api/studios` returns only studios with ≥1 anime, ordered by count, bounding the list to the catalog.
