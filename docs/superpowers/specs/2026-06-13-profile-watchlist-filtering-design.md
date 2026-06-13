# Profile watchlist filtering (genre / type / year)

**Date:** 2026-06-13
**Status:** Approved (design)
**Author:** AI pair-session with project owner

## Summary

Add three new **server-side** filters to the profile watchlist tab (own +
public profiles) that combine with the existing status / free-text search /
sort controls:

- **Genre** — multi-select, **AND** semantics (anime must carry *all* selected genres)
- **Type (`kind`)** — multi-select, **OR** semantics
- **Year** — `min`–`max` range

Filter option lists come from a new **facets endpoint** that returns only the
genres / kinds / year-range present in *that user's* list, each with a count —
mirroring the existing status-pill `count` pattern.

## Motivation

The watchlist already filters by status, free-text search, and sorts by
title / score / progress / status / genre — all server-side and paginated.
Users with large lists want to narrow by genre, format, and era. Because the
list is paginated server-side (the frontend only renders the returned page),
the new filters MUST also be server-side; client-side filtering would only
touch the visible ~20-item page.

## Decisions (locked)

- **Option source:** Approach A — a dedicated facets endpoint with counts
  (not the global `/genres` list / static enums). Consistent with the counted
  status pills, avoids offering options the user has nothing in.
- **Genre logic:** AND (must have all selected genres).
- **Type logic:** OR (an anime can't be two kinds at once).
- **Year:** min–max range.
- **Facet computation:** over the user's *whole* list (ignores active
  status/search/other filters) so the option set stays stable. Counts are
  informational; no cross-filtering (counts don't shrink as you select).
- **Persistence:** filters are NOT persisted to localStorage — they reset on
  navigation, same as the status filter (only sort is persisted today).

## Data grounding

Single shared `animeenigma` Postgres DB. The catalog-owned `animes` table
(read by the player service via the `AnimeInfo` projection) has:

- `year bigint` — 4476/4476 populated (range 0–2027; `0` treated as unknown)
- `kind varchar(20)` — indexed (`idx_animes_kind`), 4476/4476 populated.
  Distinct values + counts: `tv` (1870), `movie` (653), `ova` (640),
  `ona` (471), `music` (301), `special` (296), `tv_special` (157), `cm` (48),
  `pv` (27), `''` (13).
- Genres are many-to-many: `anime_genres` (anime_id, genre_id) → `genres`
  (id, name, name_ru). Already joined for the existing genre *sort*
  (`genreSortJoin` in `repo/list.go`).

## Backend (player service)

### 1. Filters struct

New `domain.ListFilters`:

```go
type ListFilters struct {
    GenreIDs []string // AND — anime must have all
    Kinds    []string // OR  — animes.kind IN (...)
    YearMin  *int      // nil = open lower bound
    YearMax  *int      // nil = open upper bound
}

func (f ListFilters) IsEmpty() bool { ... }
```

Threaded as a single param into the two paginated repo methods:

- `GetByUserPaginated(ctx, userID, status, search, excludeHentai, filters, params)`
- `GetByUserAndStatusesPaginated(ctx, userID, statuses, search, excludeHentai, filters, params)`

Service layer (`GetUserListPaginated`, `GetPublicWatchlistPaginated`) gains a
`filters ListFilters` param passed straight through.

### 2. Query clauses (`repo/list.go`)

Applied to the shared `base` query in both paginated methods:

- **animes join trigger:** the existing conditional LEFT JOIN on `animes`
  (`needsAnimesJoin`) must also fire when `len(filters.Kinds) > 0` or a year
  bound is set.
- **kind:** `base.Where("animes.kind IN ?", filters.Kinds)` (when non-empty).
- **year:**
  - both bounds → `animes.year BETWEEN ? AND ?`
  - only min → `animes.year >= ?`
  - only max → `animes.year <= ?`
- **genre (AND):**
  ```sql
  anime_list.anime_id IN (
    SELECT ag.anime_id FROM anime_genres ag
    WHERE ag.genre_id IN (?)
    GROUP BY ag.anime_id
    HAVING COUNT(DISTINCT ag.genre_id) = ?   -- = len(GenreIDs)
  )
  ```
  Passed as a subquery so it composes with the existing count + find sessions.

These compose with the existing status / search / hentai-exclusion clauses
(all AND).

### 3. Facets endpoint

Routes (both → player via gateway `/api/users/*`):

- `GET /users/watchlist/facets` — own list (auth required, no hentai exclusion)
- `GET /users/{userId}/watchlist/facets` — public (hentai-excluded per the
  same `ActivityVisibilityNonHentai` rule used by the public watchlist)

Response shape:

```json
{
  "genres": [{ "id": "...", "name": "Action", "name_ru": "Экшен", "count": 42 }],
  "kinds":  [{ "kind": "tv", "count": 80 }],
  "years":  { "min": 2009, "max": 2024 }
}
```

Three grouped queries over the user's whole list (scoped by user_id, +
hentai-exclusion for public):

- genres: `JOIN anime_genres ag ON ag.anime_id = al.anime_id JOIN genres g ...
  GROUP BY g.id, g.name, g.name_ru ORDER BY count DESC`
- kinds: `JOIN animes a ON a.id = al.anime_id GROUP BY a.kind` (skip empty kind)
- years: `MIN(NULLIF(a.year,0)), MAX(NULLIF(a.year,0))` (null range when list empty)

New repo method `GetListFacets(ctx, userID string, excludeHentai bool) (*domain.ListFacets, error)`.

### 4. Handler parsing

`parseListFilters(r)` reads query params:

- `genres` — comma-separated genre UUIDs (validated as UUID; invalid entries dropped)
- `kind` — comma-separated; validated against the known kind set
- `year_min`, `year_max` — parsed ints; nil if absent/invalid

Wired into `GetUserList` and `GetPublicWatchlist`.

## Frontend

### 1. `WatchlistFilters.vue` (new component)

Extracted to keep `Profile.vue` (already 2159 lines) from growing.

- **Props:** `facets: WatchlistFacets`, plus v-model bindings for
  `genreIds: string[]`, `kinds: string[]`, `yearMin: number | null`,
  `yearMax: number | null`.
- **Trigger:** a `Filters` `Button` with a `Badge` showing the active-filter
  count, placed in the controls row (alongside search / sort / view toggle).
- **Panel:** a `Popover` containing three groups:
  - **Genres** — `Checkbox` list with per-genre counts; scrollable; a
    search-within input when the list is long. AND hint in the group label.
  - **Types** — `Checkbox` list with counts (OR).
  - **Year** — min / max `Select`s bounded by `facets.years`.
  - **Clear all** button (disabled when no active filters).
- Reuses `Popover`, `Checkbox`, `Badge`, `Button` primitives.
  `GenreFilterPopup.vue` is single-select, so it is referenced for its popover
  pattern only — multi-select is built here.
- Co-located `WatchlistFilters.spec.ts` (≥5 Vitest assertions).

### 2. `Profile.vue` wiring

- New refs: `genreFilter (string[])`, `kindFilter (string[])`,
  `yearMin/yearMax (number | null)`; `facets` ref.
- Fetch facets when the watchlist loads and when the profile user changes
  (own → `userApi.getWatchlistFacets()`, public →
  `publicApi.getPublicWatchlistFacets(userId)`).
- Extend the fetch key (currently `${uid}:${status}:${page}:${sortKey}:${sortDirection}:${q}`)
  with the serialized filters, and add the params to the `getWatchlist` /
  `getPublicWatchlist` calls.
- Any filter change resets `watchlistPage = 1` and refetches.
- Existing empty-state (`profile.empty.filter`) already covers "no matches".

### 3. API client

Extend `getWatchlist` / `getPublicWatchlist` param types with
`genres?`, `kind?`, `year_min?`, `year_max?`. Add `getWatchlistFacets()` and
`getPublicWatchlistFacets(userId)`.

### 4. i18n

`profile.filters.*` keys in both `frontend/web/src/locales/en.json` and
`ru.json` (button label, group titles, AND/OR hints, year min/max, clear-all,
empty-options). Locale parity is test-enforced.

## Testing

- **Backend** (`repo` + handler, handwritten fakes — no testify/mock):
  - kind filter (OR), year range (both/one bound), genre AND
    (single + multiple, HAVING-count correctness)
  - facets aggregation (genre counts, kind counts, year min/max, empty list)
  - hentai exclusion on the public facets path
- **Frontend**: `WatchlistFilters.spec.ts` — emits on change, active-count
  badge, clear-all resets, genre AND hint vs type OR hint rendering, counts shown.

## Out of scope (YAGNI)

- Cross-filtering facet counts (counts won't shrink as selections are made).
- Saved / persisted filter presets.
- Filtering by rewatch-count or personal score (not requested).

## Affected files (anticipated)

- `services/player/internal/domain/` — `ListFilters`, `ListFacets`
- `services/player/internal/repo/list.go` (+ tests)
- `services/player/internal/service/list.go`
- `services/player/internal/handler/list.go` (+ tests)
- `services/player/internal/transport/router.go` (2 facets routes)
- `frontend/web/src/api/client.ts`
- `frontend/web/src/components/profile/WatchlistFilters.vue` (+ `.spec.ts`)
- `frontend/web/src/views/Profile.vue`
- `frontend/web/src/types/` (facets type)
- `frontend/web/src/locales/{en,ru}.json`
