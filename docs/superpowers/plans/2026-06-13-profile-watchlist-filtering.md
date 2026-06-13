# Profile Watchlist Filtering (genre / type / year) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add server-side genre (AND), type/`kind` (OR), and year-range filters to the profile watchlist tab on both own and public profiles, with a facets endpoint that drives counted filter options.

**Architecture:** The watchlist is paginated server-side; the frontend renders only the returned page, so filters are added as new query params on the existing list endpoints and applied in the GORM repo layer. A new facets endpoint aggregates the genres/kinds/year-range present in the user's whole list (with counts) to populate stable filter options. A new extracted `WatchlistFilters.vue` component renders the filter UI and is wired into `Profile.vue`.

**Tech Stack:** Go (chi, GORM), Postgres (shared `animeenigma` DB), Vue 3 + TypeScript, reka-ui primitives, Vitest, vue-i18n. Backend repo tests use the existing sqlite-in-memory + testify harness.

**Spec:** `docs/superpowers/specs/2026-06-13-profile-watchlist-filtering-design.md`

---

## File Structure

**Backend (player service):**
- `services/player/internal/domain/watch.go` — MODIFY: add `ListFilters`, `ListFacets`, `FacetGenre`, `FacetKind`, `FacetYearRange`, `KnownKinds`.
- `services/player/internal/repo/list.go` — MODIFY: apply filters in the two paginated queries; add `GetListFacets`.
- `services/player/internal/repo/list_filter_test.go` — CREATE: filter + facets repo tests.
- `services/player/internal/service/list.go` — MODIFY: thread filters; add facets service methods.
- `services/player/internal/handler/list.go` — MODIFY: parse filter params; add facets handlers.
- `services/player/internal/transport/router.go` — MODIFY: register 2 facets routes.

**Frontend:**
- `frontend/web/src/types/watchlist-facets.ts` — CREATE: facet types.
- `frontend/web/src/api/client.ts` — MODIFY: extend list param types; add facets methods.
- `frontend/web/src/components/profile/WatchlistFilters.vue` — CREATE: filter UI.
- `frontend/web/src/components/profile/WatchlistFilters.spec.ts` — CREATE: component tests.
- `frontend/web/src/views/Profile.vue` — MODIFY: filter state, facets fetch, fetch-key + params, page reset, render component.
- `frontend/web/src/locales/en.json` + `ru.json` — MODIFY: `profile.filters.*` keys.

---

## Task 1: Backend domain types

**Files:**
- Modify: `services/player/internal/domain/watch.go`

- [ ] **Step 1: Add the filter + facet types**

Append to `services/player/internal/domain/watch.go` (after the existing `AnimeInfo` / `GenreInfo` block):

```go
// ListFilters holds the optional watchlist filter dimensions added 2026-06-13.
// All fields are zero-value-safe: an empty ListFilters applies no filtering.
type ListFilters struct {
	GenreIDs []string // AND semantics — an anime must carry ALL listed genres
	Kinds    []string // OR semantics — animes.kind IN (Kinds)
	YearMin  *int     // nil = open lower bound
	YearMax  *int     // nil = open upper bound
}

// IsEmpty reports whether no filter dimension is set.
func (f ListFilters) IsEmpty() bool {
	return len(f.GenreIDs) == 0 && len(f.Kinds) == 0 && f.YearMin == nil && f.YearMax == nil
}

// KnownKinds is the validation whitelist for the `kind` filter param. Mirrors
// the distinct animes.kind values present in the catalog.
var KnownKinds = map[string]bool{
	"tv": true, "movie": true, "ova": true, "ona": true, "special": true,
	"tv_special": true, "music": true, "cm": true, "pv": true,
}

// FacetGenre is one genre option for the watchlist filter UI, with the count of
// the user's list entries carrying that genre.
type FacetGenre struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	NameRU string `json:"name_ru"`
	Count  int64  `json:"count"`
}

// FacetKind is one type/kind option with its count.
type FacetKind struct {
	Kind  string `json:"kind"`
	Count int64  `json:"count"`
}

// FacetYearRange is the min/max release year present in the user's list.
// Both nil when the list has no entries with a known (non-zero) year.
type FacetYearRange struct {
	Min *int `json:"min"`
	Max *int `json:"max"`
}

// ListFacets is the response of the watchlist facets endpoint.
type ListFacets struct {
	Genres []FacetGenre   `json:"genres"`
	Kinds  []FacetKind    `json:"kinds"`
	Years  FacetYearRange `json:"years"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd services/player && go build ./...`
Expected: builds clean (no usages yet, types are inert).

- [ ] **Step 3: Commit**

```bash
git add services/player/internal/domain/watch.go
git commit -m "feat(player): watchlist filter + facet domain types"
```

---

## Task 2: Repo — apply filters in the two paginated queries

**Files:**
- Modify: `services/player/internal/repo/list.go` (`GetByUserPaginated` ~line 242, `GetByUserAndStatusesPaginated` ~line 782)
- Test: `services/player/internal/repo/list_filter_test.go`

- [ ] **Step 1: Add a shared filter-application helper**

In `services/player/internal/repo/list.go`, after the `isGenreSort` helper (~line 70), add:

```go
// filtersNeedAnimesJoin reports whether the filter set references columns on
// the animes table (kind / year) and therefore requires the LEFT JOIN animes.
func filtersNeedAnimesJoin(f domain.ListFilters) bool {
	return len(f.Kinds) > 0 || f.YearMin != nil || f.YearMax != nil
}

// applyListFilters appends the genre (AND) / kind (OR) / year-range WHERE
// clauses to a query that already has (or will have) the animes join when
// filtersNeedAnimesJoin is true. Genre AND is expressed as an anime_id IN
// (subquery) with a HAVING count so it composes with the Count + Find sessions.
func applyListFilters(q *gorm.DB, f domain.ListFilters) *gorm.DB {
	if len(f.Kinds) > 0 {
		q = q.Where("animes.kind IN ?", f.Kinds)
	}
	switch {
	case f.YearMin != nil && f.YearMax != nil:
		q = q.Where("animes.year BETWEEN ? AND ?", *f.YearMin, *f.YearMax)
	case f.YearMin != nil:
		q = q.Where("animes.year >= ?", *f.YearMin)
	case f.YearMax != nil:
		q = q.Where("animes.year <= ?", *f.YearMax)
	}
	if len(f.GenreIDs) > 0 {
		q = q.Where(
			"anime_list.anime_id IN (SELECT ag.anime_id FROM anime_genres ag WHERE ag.genre_id IN ? GROUP BY ag.anime_id HAVING COUNT(DISTINCT ag.genre_id) = ?)",
			f.GenreIDs, len(f.GenreIDs),
		)
	}
	return q
}
```

- [ ] **Step 2: Thread `filters` into `GetByUserPaginated`**

Change the signature and join/where logic in `GetByUserPaginated` (~line 242):

```go
func (r *ListRepository) GetByUserPaginated(ctx context.Context, userID, status, search string, excludeHentai bool, filters domain.ListFilters, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate() // defense in depth

	var entries []*domain.AnimeListEntry
	var total int64

	base := r.db.WithContext(ctx).Where("anime_list.user_id = ?", userID)
	if status != "" {
		base = base.Where("anime_list.status = ?", status)
	}
	if excludeHentai {
		base = base.Where("NOT " + fmt.Sprintf(hentaiAnimeExistsFmt, "anime_list.anime_id"))
	}

	needsAnimesJoin := isTitleSort(params.Sort) || search != "" || filtersNeedAnimesJoin(filters)
	if needsAnimesJoin {
		base = base.Joins("LEFT JOIN animes ON animes.id = anime_list.anime_id")
	}
	if isGenreSort(params.Sort) {
		base = base.Joins(genreSortJoin)
	}
	if search != "" {
		like := "%" + search + "%"
		base = base.Where(
			"animes.name ILIKE ? OR animes.name_ru ILIKE ? OR animes.name_jp ILIKE ?",
			like, like, like,
		)
	}
	base = applyListFilters(base, filters)

	if err := base.Session(&gorm.Session{}).Model(&domain.AnimeListEntry{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base.Session(&gorm.Session{}).
		Preload("Anime").Preload("Anime.Genres").
		Order(sanitizedOrderClause(params.Sort, params.Order)).
		Offset(params.Offset()).
		Limit(params.PerPage).
		Find(&entries).Error

	return entries, total, err
}
```

- [ ] **Step 3: Thread `filters` into `GetByUserAndStatusesPaginated`**

Apply the same pattern to `GetByUserAndStatusesPaginated` (~line 782): add `filters domain.ListFilters` before `params *domain.PaginationParams`, change `needsAnimesJoin` to include `|| filtersNeedAnimesJoin(filters)`, and add `base = applyListFilters(base, filters)` immediately before the Count call.

```go
func (r *ListRepository) GetByUserAndStatusesPaginated(ctx context.Context, userID string, statuses []string, search string, excludeHentai bool, filters domain.ListFilters, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate() // defense in depth

	var entries []*domain.AnimeListEntry
	var total int64

	base := r.db.WithContext(ctx).Where("anime_list.user_id = ? AND anime_list.status IN ?", userID, statuses)
	if excludeHentai {
		base = base.Where("NOT " + fmt.Sprintf(hentaiAnimeExistsFmt, "anime_list.anime_id"))
	}

	needsAnimesJoin := isTitleSort(params.Sort) || search != "" || filtersNeedAnimesJoin(filters)
	if needsAnimesJoin {
		base = base.Joins("LEFT JOIN animes ON animes.id = anime_list.anime_id")
	}
	if isGenreSort(params.Sort) {
		base = base.Joins(genreSortJoin)
	}
	if search != "" {
		like := "%" + search + "%"
		base = base.Where(
			"animes.name ILIKE ? OR animes.name_ru ILIKE ? OR animes.name_jp ILIKE ?",
			like, like, like,
		)
	}
	base = applyListFilters(base, filters)

	if err := base.Session(&gorm.Session{}).Model(&domain.AnimeListEntry{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base.Session(&gorm.Session{}).
		Preload("Anime").Preload("Anime.Genres").
		Order(sanitizedOrderClause(params.Sort, params.Order)).
		Offset(params.Offset()).
		Limit(params.PerPage).
		Find(&entries).Error

	return entries, total, err
}
```

- [ ] **Step 4: Update the two service call sites so the package compiles**

In `services/player/internal/service/list.go`:
- Line ~60: `return s.listRepo.GetByUserPaginated(ctx, userID, status, search, false, domain.ListFilters{}, params)`
- Line ~109: `return s.listRepo.GetByUserPaginated(ctx, userID, "", search, excludeHentai, domain.ListFilters{}, params)`
- Line ~111: `return s.listRepo.GetByUserAndStatusesPaginated(ctx, userID, statuses, search, excludeHentai, domain.ListFilters{}, params)`

(These get real filter values in Task 4 — for now pass an empty struct so the build stays green.)

- [ ] **Step 5: Write the failing filter test**

Create `services/player/internal/repo/list_filter_test.go`. The animes test table must include `kind` and `year` columns (the existing `setupListVisibilityTestDB` omits them), so this file builds its own DB helper:

```go
package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFilterTestDB(t *testing.T) *ListRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	stmts := []string{
		`CREATE TABLE animes (
			id TEXT PRIMARY KEY, name TEXT, name_ru TEXT, name_jp TEXT,
			poster_url TEXT, episodes_count INTEGER DEFAULT 0,
			episodes_aired INTEGER DEFAULT 0, kind TEXT, year INTEGER DEFAULT 0
		)`,
		`CREATE TABLE genres (id TEXT PRIMARY KEY, name TEXT, name_ru TEXT)`,
		`CREATE TABLE anime_genres (anime_id TEXT, genre_id TEXT)`,
		`CREATE TABLE anime_list (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, anime_id TEXT NOT NULL,
			status TEXT, score INTEGER, episodes INTEGER, rewatch_count INTEGER DEFAULT 0,
			created_at DATETIME, updated_at DATETIME
		)`,
		// animes
		`INSERT INTO animes (id, name, kind, year) VALUES
			('a-tv-2020', 'TV 2020', 'tv', 2020),
			('a-movie-2010', 'Movie 2010', 'movie', 2010),
			('a-ova-2024', 'OVA 2024', 'ova', 2024)`,
		// genres
		`INSERT INTO genres (id, name, name_ru) VALUES
			('g-action', 'Action', 'Экшен'),
			('g-comedy', 'Comedy', 'Комедия')`,
		// a-tv-2020 = Action+Comedy, a-movie-2010 = Action, a-ova-2024 = Comedy
		`INSERT INTO anime_genres (anime_id, genre_id) VALUES
			('a-tv-2020','g-action'),('a-tv-2020','g-comedy'),
			('a-movie-2010','g-action'),
			('a-ova-2024','g-comedy')`,
		`INSERT INTO anime_list (id, user_id, anime_id, status, score) VALUES
			('l1','u1','a-tv-2020','completed',8),
			('l2','u1','a-movie-2010','completed',7),
			('l3','u1','a-ova-2024','watching',9)`,
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error)
	}
	return NewListRepository(db)
}

func idsOf(entries []*domain.AnimeListEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.AnimeID)
	}
	return out
}

func TestGetByUserPaginated_KindFilter(t *testing.T) {
	repo := setupFilterTestDB(t)
	f := domain.ListFilters{Kinds: []string{"tv", "ova"}}
	entries, total, err := repo.GetByUserPaginated(context.Background(), "u1", "", "", false, f, &domain.PaginationParams{Page: 1, PerPage: 50})
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.ElementsMatch(t, []string{"a-tv-2020", "a-ova-2024"}, idsOf(entries))
}

func TestGetByUserPaginated_YearRange(t *testing.T) {
	repo := setupFilterTestDB(t)
	min, max := 2015, 2022
	f := domain.ListFilters{YearMin: &min, YearMax: &max}
	entries, total, err := repo.GetByUserPaginated(context.Background(), "u1", "", "", false, f, &domain.PaginationParams{Page: 1, PerPage: 50})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, []string{"a-tv-2020"}, idsOf(entries))
}

func TestGetByUserPaginated_GenreAND(t *testing.T) {
	repo := setupFilterTestDB(t)
	// Action AND Comedy → only a-tv-2020 has both.
	f := domain.ListFilters{GenreIDs: []string{"g-action", "g-comedy"}}
	entries, total, err := repo.GetByUserPaginated(context.Background(), "u1", "", "", false, f, &domain.PaginationParams{Page: 1, PerPage: 50})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, []string{"a-tv-2020"}, idsOf(entries))
}

func TestGetByUserPaginated_GenreSingle(t *testing.T) {
	repo := setupFilterTestDB(t)
	// Action only → a-tv-2020 + a-movie-2010.
	f := domain.ListFilters{GenreIDs: []string{"g-action"}}
	entries, total, err := repo.GetByUserPaginated(context.Background(), "u1", "", "", false, f, &domain.PaginationParams{Page: 1, PerPage: 50})
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.ElementsMatch(t, []string{"a-tv-2020", "a-movie-2010"}, idsOf(entries))
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd services/player && go test ./internal/repo/ -run 'TestGetByUserPaginated_(KindFilter|YearRange|GenreAND|GenreSingle)' -count=1 -v`
Expected: 4 PASS.

- [ ] **Step 7: Commit**

```bash
git add services/player/internal/repo/list.go services/player/internal/repo/list_filter_test.go services/player/internal/service/list.go
git commit -m "feat(player): genre/kind/year filters in paginated watchlist queries"
```

---

## Task 3: Repo — `GetListFacets`

**Files:**
- Modify: `services/player/internal/repo/list.go`
- Test: `services/player/internal/repo/list_filter_test.go`

- [ ] **Step 1: Add the facets query method**

Append to `services/player/internal/repo/list.go`:

```go
// GetListFacets aggregates the distinct genres / kinds / release-year-range
// present in a user's entire anime_list, each with a count. Used to populate
// the profile watchlist filter UI with only-relevant, counted options.
// Computed over the WHOLE list (ignores status/search/other filters) so the
// option set stays stable as the user toggles filters. excludeHentai mirrors
// the public-watchlist activity-visibility rule.
func (r *ListRepository) GetListFacets(ctx context.Context, userID string, excludeHentai bool) (*domain.ListFacets, error) {
	facets := &domain.ListFacets{Genres: []domain.FacetGenre{}, Kinds: []domain.FacetKind{}}

	hentaiClause := ""
	if excludeHentai {
		hentaiClause = " AND NOT " + fmt.Sprintf(hentaiAnimeExistsFmt, "al.anime_id")
	}

	// Genres present, with counts, most-used first.
	genreSQL := `
SELECT g.id AS id, g.name AS name, g.name_ru AS name_ru, COUNT(DISTINCT al.anime_id) AS count
FROM anime_list al
JOIN anime_genres ag ON ag.anime_id = al.anime_id
JOIN genres g ON g.id = ag.genre_id
WHERE al.user_id = ?` + hentaiClause + `
GROUP BY g.id, g.name, g.name_ru
ORDER BY count DESC, g.name ASC`
	if err := r.db.WithContext(ctx).Raw(genreSQL, userID).Scan(&facets.Genres).Error; err != nil {
		return nil, err
	}

	// Kinds present, with counts. Skip blank kind.
	kindSQL := `
SELECT a.kind AS kind, COUNT(*) AS count
FROM anime_list al
JOIN animes a ON a.id = al.anime_id
WHERE al.user_id = ? AND a.kind IS NOT NULL AND a.kind <> ''` + hentaiClause + `
GROUP BY a.kind
ORDER BY count DESC, a.kind ASC`
	if err := r.db.WithContext(ctx).Raw(kindSQL, userID).Scan(&facets.Kinds).Error; err != nil {
		return nil, err
	}

	// Year range (ignore 0/unknown).
	yearSQL := `
SELECT MIN(NULLIF(a.year, 0)) AS min, MAX(NULLIF(a.year, 0)) AS max
FROM anime_list al
JOIN animes a ON a.id = al.anime_id
WHERE al.user_id = ?` + hentaiClause
	var yr struct {
		Min *int
		Max *int
	}
	if err := r.db.WithContext(ctx).Raw(yearSQL, userID).Scan(&yr).Error; err != nil {
		return nil, err
	}
	facets.Years = domain.FacetYearRange{Min: yr.Min, Max: yr.Max}

	if facets.Genres == nil {
		facets.Genres = []domain.FacetGenre{}
	}
	if facets.Kinds == nil {
		facets.Kinds = []domain.FacetKind{}
	}
	return facets, nil
}
```

- [ ] **Step 2: Write the failing facets test**

Append to `services/player/internal/repo/list_filter_test.go`:

```go
func TestGetListFacets(t *testing.T) {
	repo := setupFilterTestDB(t)
	facets, err := repo.GetListFacets(context.Background(), "u1", false)
	require.NoError(t, err)

	// Genres: Action (2 anime) and Comedy (2 anime).
	gotGenre := map[string]int64{}
	for _, g := range facets.Genres {
		gotGenre[g.ID] = g.Count
	}
	assert.EqualValues(t, 2, gotGenre["g-action"])
	assert.EqualValues(t, 2, gotGenre["g-comedy"])

	// Kinds: tv(1), movie(1), ova(1).
	gotKind := map[string]int64{}
	for _, k := range facets.Kinds {
		gotKind[k.Kind] = k.Count
	}
	assert.EqualValues(t, 1, gotKind["tv"])
	assert.EqualValues(t, 1, gotKind["movie"])
	assert.EqualValues(t, 1, gotKind["ova"])

	// Year range 2010..2024.
	require.NotNil(t, facets.Years.Min)
	require.NotNil(t, facets.Years.Max)
	assert.Equal(t, 2010, *facets.Years.Min)
	assert.Equal(t, 2024, *facets.Years.Max)
}

func TestGetListFacets_EmptyList(t *testing.T) {
	repo := setupFilterTestDB(t)
	facets, err := repo.GetListFacets(context.Background(), "nobody", false)
	require.NoError(t, err)
	assert.Empty(t, facets.Genres)
	assert.Empty(t, facets.Kinds)
	assert.Nil(t, facets.Years.Min)
	assert.Nil(t, facets.Years.Max)
}
```

- [ ] **Step 3: Run the tests**

Run: `cd services/player && go test ./internal/repo/ -run 'TestGetListFacets' -count=1 -v`
Expected: 2 PASS.

- [ ] **Step 4: Commit**

```bash
git add services/player/internal/repo/list.go services/player/internal/repo/list_filter_test.go
git commit -m "feat(player): GetListFacets watchlist facet aggregation"
```

---

## Task 4: Service — thread filters + add facets methods

**Files:**
- Modify: `services/player/internal/service/list.go`

- [ ] **Step 1: Add `filters` to the two paginated service methods**

Replace `GetUserListPaginated` (~line 58):

```go
func (s *ListService) GetUserListPaginated(ctx context.Context, userID, status, search string, filters domain.ListFilters, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate()
	return s.listRepo.GetByUserPaginated(ctx, userID, status, search, false, filters, params)
}
```

Replace `GetPublicWatchlistPaginated` (~line 101) — add `filters domain.ListFilters` and pass it to both repo calls:

```go
func (s *ListService) GetPublicWatchlistPaginated(ctx context.Context, userID string, statuses []string, search string, filters domain.ListFilters, params *domain.PaginationParams) ([]*domain.AnimeListEntry, int64, error) {
	params.Validate()
	visibility := s.listRepo.GetUserActivityVisibility(ctx, userID)
	if visibility == repo.ActivityVisibilityNone {
		return []*domain.AnimeListEntry{}, 0, nil
	}
	excludeHentai := visibility == repo.ActivityVisibilityNonHentai
	if len(statuses) == 0 {
		return s.listRepo.GetByUserPaginated(ctx, userID, "", search, excludeHentai, filters, params)
	}
	return s.listRepo.GetByUserAndStatusesPaginated(ctx, userID, statuses, search, excludeHentai, filters, params)
}
```

- [ ] **Step 2: Add the facets service methods**

Append to `services/player/internal/service/list.go`:

```go
// GetListFacets returns the filter facets for the caller's own list.
func (s *ListService) GetListFacets(ctx context.Context, userID string) (*domain.ListFacets, error) {
	facets, err := s.listRepo.GetListFacets(ctx, userID, false)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "GetListFacets: query")
	}
	return facets, nil
}

// GetPublicListFacets returns filter facets for a public profile, honoring the
// target user's activity_visibility (none → empty; non_hentai → 18+ excluded).
func (s *ListService) GetPublicListFacets(ctx context.Context, userID string) (*domain.ListFacets, error) {
	visibility := s.listRepo.GetUserActivityVisibility(ctx, userID)
	if visibility == repo.ActivityVisibilityNone {
		return &domain.ListFacets{Genres: []domain.FacetGenre{}, Kinds: []domain.FacetKind{}}, nil
	}
	excludeHentai := visibility == repo.ActivityVisibilityNonHentai
	facets, err := s.listRepo.GetListFacets(ctx, userID, excludeHentai)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "GetPublicListFacets: query")
	}
	return facets, nil
}
```

- [ ] **Step 3: Verify package compiles (handlers updated in Task 5 — build will fail until then)**

Run: `cd services/player && go build ./internal/service/...`
Expected: `internal/service` builds clean. (The `handler` package still calls the old signatures — fixed in Task 5.)

- [ ] **Step 4: Commit**

```bash
git add services/player/internal/service/list.go
git commit -m "feat(player): thread filters through list service + facets service methods"
```

---

## Task 5: Handler — parse filter params + facets endpoints + routes

**Files:**
- Modify: `services/player/internal/handler/list.go`
- Modify: `services/player/internal/transport/router.go`

- [ ] **Step 1: Add the filter-param parser**

In `services/player/internal/handler/list.go`, add near `parsePaginationParams` (~line 358):

```go
// parseListFilters reads the optional genre/kind/year filter query params.
// genres + kind are comma-separated; invalid kinds are dropped (validated
// against domain.KnownKinds); year_min/year_max parse to *int (nil if absent
// or non-numeric).
func parseListFilters(r *http.Request) domain.ListFilters {
	var f domain.ListFilters

	if g := r.URL.Query().Get("genres"); g != "" {
		for _, id := range splitAndTrim(g, ",") {
			if id != "" {
				f.GenreIDs = append(f.GenreIDs, id)
			}
		}
	}
	if k := r.URL.Query().Get("kind"); k != "" {
		for _, kind := range splitAndTrim(k, ",") {
			if domain.KnownKinds[kind] {
				f.Kinds = append(f.Kinds, kind)
			}
		}
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("year_min")); err == nil {
		f.YearMin = &v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("year_max")); err == nil {
		f.YearMax = &v
	}
	return f
}
```

- [ ] **Step 2: Pass filters at the two call sites**

In `GetUserList` (~line 42):

```go
	filters := parseListFilters(r)
	entries, total, err := h.listService.GetUserListPaginated(r.Context(), claims.UserID, status, search, filters, params)
```

In `GetPublicWatchlist` (~line 290):

```go
	filters := parseListFilters(r)
	entries, total, err := h.listService.GetPublicWatchlistPaginated(r.Context(), userID, statuses, search, filters, params)
```

- [ ] **Step 3: Add the two facets handlers**

Append to `services/player/internal/handler/list.go`:

```go
// GetWatchlistFacets returns filter facets (genres/kinds/year-range with
// counts) for the authenticated user's own list.
func (h *ListHandler) GetWatchlistFacets(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	facets, err := h.listService.GetListFacets(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, facets)
}

// GetPublicWatchlistFacets returns filter facets for a public profile.
func (h *ListHandler) GetPublicWatchlistFacets(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		httputil.BadRequest(w, "user ID is required")
		return
	}
	facets, err := h.listService.GetPublicListFacets(r.Context(), userID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, facets)
}
```

- [ ] **Step 4: Register the routes**

In `services/player/internal/transport/router.go`:
- After line 91 (`r.Get("/watchlist/statuses", ...)`), in the same authenticated `/watchlist` group, add:

```go
			r.Get("/watchlist/facets", listHandler.GetWatchlistFacets)
```

- After line 189 (`r.Get("/users/{userId}/watchlist/public/stats", ...)`), in the public group, add:

```go
		r.Get("/users/{userId}/watchlist/facets", listHandler.GetPublicWatchlistFacets)
```

> Route order note: `/watchlist/facets` must be registered before the `/watchlist/{animeId}` wildcard (line 92) or chi will treat "facets" as an animeId. The Step-4 insertion point (after line 91) satisfies this.

- [ ] **Step 5: Build the whole service**

Run: `cd services/player && go build ./...`
Expected: builds clean.

- [ ] **Step 6: Run all player repo + service tests**

Run: `cd services/player && go test ./internal/repo/ ./internal/service/ ./internal/handler/ -count=1`
Expected: PASS (no regressions).

- [ ] **Step 7: Commit**

```bash
git add services/player/internal/handler/list.go services/player/internal/transport/router.go
git commit -m "feat(player): watchlist filter params + facets endpoints + routes"
```

---

## Task 6: Frontend — types + API client

**Files:**
- Create: `frontend/web/src/types/watchlist-facets.ts`
- Modify: `frontend/web/src/api/client.ts`

- [ ] **Step 1: Create the facet types**

`frontend/web/src/types/watchlist-facets.ts`:

```ts
export interface FacetGenre {
  id: string
  name: string
  name_ru: string
  count: number
}

export interface FacetKind {
  kind: string
  count: number
}

export interface FacetYearRange {
  min: number | null
  max: number | null
}

export interface WatchlistFacets {
  genres: FacetGenre[]
  kinds: FacetKind[]
  years: FacetYearRange
}

/** Active filter selections, v-modeled by WatchlistFilters.vue. */
export interface WatchlistFilterState {
  genreIds: string[]
  kinds: string[]
  yearMin: number | null
  yearMax: number | null
}

export const EMPTY_FILTER_STATE: WatchlistFilterState = {
  genreIds: [],
  kinds: [],
  yearMin: null,
  yearMax: null,
}

/** True when no filter dimension is active. */
export function isFilterStateEmpty(s: WatchlistFilterState): boolean {
  return s.genreIds.length === 0 && s.kinds.length === 0 && s.yearMin === null && s.yearMax === null
}

/** Count of active filter dimensions (for the trigger badge). genres/kinds
 *  each count their selected entries; an active year range counts as 1. */
export function activeFilterCount(s: WatchlistFilterState): number {
  return s.genreIds.length + s.kinds.length + (s.yearMin !== null || s.yearMax !== null ? 1 : 0)
}

/** Serialize to query params for the watchlist list endpoints. */
export function filterParams(s: WatchlistFilterState): Record<string, string> {
  const p: Record<string, string> = {}
  if (s.genreIds.length) p.genres = s.genreIds.join(',')
  if (s.kinds.length) p.kind = s.kinds.join(',')
  if (s.yearMin !== null) p.year_min = String(s.yearMin)
  if (s.yearMax !== null) p.year_max = String(s.yearMax)
  return p
}

/** Stable string for the page-cache key (order-independent). */
export function filterKey(s: WatchlistFilterState): string {
  return [
    [...s.genreIds].sort().join('+'),
    [...s.kinds].sort().join('+'),
    s.yearMin ?? '',
    s.yearMax ?? '',
  ].join('|')
}
```

- [ ] **Step 2: Extend the API client param types + add facets methods**

In `frontend/web/src/api/client.ts`:

Extend `getWatchlist` (line 424) param type and `getPublicWatchlist` (line 555) param type by adding `genres?: string; kind?: string; year_min?: string; year_max?: string` to each params object type.

Add inside the `userApi` object (near `getWatchlistStatuses`, line 426):

```ts
  getWatchlistFacets: () => apiClient.get('/users/watchlist/facets'),
```

Add inside the public API object (near `getPublicWatchlistStats`, line 558):

```ts
  getPublicWatchlistFacets: (userId: string) =>
    apiClient.get(`/users/${userId}/watchlist/facets`),
```

- [ ] **Step 3: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/types/watchlist-facets.ts frontend/web/src/api/client.ts
git commit -m "feat(web): watchlist facet types + API client methods"
```

---

## Task 7: Frontend — `WatchlistFilters.vue` component + tests

**Files:**
- Create: `frontend/web/src/components/profile/WatchlistFilters.vue`
- Test: `frontend/web/src/components/profile/WatchlistFilters.spec.ts`

- [ ] **Step 1: Create the component**

`frontend/web/src/components/profile/WatchlistFilters.vue`:

```vue
<template>
  <Popover v-model:open="open" align="end">
    <template #trigger>
      <Button variant="ghost" size="sm" class="gap-1.5 text-white/70 hover:text-white">
        <SlidersHorizontal class="size-4" />
        <span>{{ $t('profile.filters.button') }}</span>
        <Badge v-if="count > 0" variant="primary" size="sm">{{ count }}</Badge>
      </Button>
    </template>

    <div class="w-72 max-h-[70vh] overflow-y-auto p-1 space-y-4">
      <!-- Genres (AND) -->
      <section v-if="facets.genres.length">
        <header class="flex items-center justify-between px-1 mb-1.5">
          <span class="text-sm font-semibold text-white">{{ $t('profile.filters.genres') }}</span>
          <span class="text-xs text-muted-foreground">{{ $t('profile.filters.genresHint') }}</span>
        </header>
        <Input
          v-if="facets.genres.length > 8"
          v-model="genreSearch"
          size="sm"
          :placeholder="$t('profile.filters.searchGenres')"
          class="mb-1.5"
        />
        <ul class="space-y-0.5">
          <li v-for="g in filteredGenres" :key="g.id">
            <label class="flex items-center gap-2 px-1 py-1 rounded-md hover:bg-white/5 cursor-pointer">
              <Checkbox :model-value="genreIds.includes(g.id)" @update:model-value="() => toggleGenre(g.id)" />
              <span class="text-sm text-white/90 flex-1 truncate">{{ localizedGenre(g) }}</span>
              <span class="text-xs text-muted-foreground tabular-nums">{{ g.count }}</span>
            </label>
          </li>
        </ul>
      </section>

      <!-- Types (OR) -->
      <section v-if="facets.kinds.length">
        <header class="flex items-center justify-between px-1 mb-1.5">
          <span class="text-sm font-semibold text-white">{{ $t('profile.filters.types') }}</span>
          <span class="text-xs text-muted-foreground">{{ $t('profile.filters.typesHint') }}</span>
        </header>
        <ul class="space-y-0.5">
          <li v-for="k in facets.kinds" :key="k.kind">
            <label class="flex items-center gap-2 px-1 py-1 rounded-md hover:bg-white/5 cursor-pointer">
              <Checkbox :model-value="kinds.includes(k.kind)" @update:model-value="() => toggleKind(k.kind)" />
              <span class="text-sm text-white/90 flex-1">{{ $t('profile.filters.kind.' + k.kind) }}</span>
              <span class="text-xs text-muted-foreground tabular-nums">{{ k.count }}</span>
            </label>
          </li>
        </ul>
      </section>

      <!-- Year range -->
      <section v-if="facets.years.min !== null && facets.years.max !== null">
        <header class="px-1 mb-1.5">
          <span class="text-sm font-semibold text-white">{{ $t('profile.filters.year') }}</span>
        </header>
        <div class="flex items-center gap-2 px-1">
          <Select :model-value="yearMinStr" :options="yearMinOptions" size="sm" class="flex-1"
            @update:model-value="(v) => emitYear('min', v as string)" />
          <span class="text-white/40">—</span>
          <Select :model-value="yearMaxStr" :options="yearMaxOptions" size="sm" class="flex-1"
            @update:model-value="(v) => emitYear('max', v as string)" />
        </div>
      </section>

      <!-- Clear all -->
      <div class="pt-1 border-t border-white/10">
        <Button variant="ghost" size="sm" class="w-full text-muted-foreground" :disabled="count === 0" @click="clearAll">
          {{ $t('profile.filters.clear') }}
        </Button>
      </div>
    </div>
  </Popover>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { SlidersHorizontal } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import Button from '@/components/ui/Button.vue'
import Badge from '@/components/ui/Badge.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import Input from '@/components/ui/Input.vue'
import Select from '@/components/ui/Select.vue'
import type { WatchlistFacets, FacetGenre } from '@/types/watchlist-facets'
import { activeFilterCount } from '@/types/watchlist-facets'

const props = defineProps<{
  facets: WatchlistFacets
  genreIds: string[]
  kinds: string[]
  yearMin: number | null
  yearMax: number | null
}>()

const emit = defineEmits<{
  'update:genreIds': [string[]]
  'update:kinds': [string[]]
  'update:yearMin': [number | null]
  'update:yearMax': [number | null]
}>()

const { locale } = useI18n()
const open = ref(false)
const genreSearch = ref('')

const count = computed(() =>
  activeFilterCount({ genreIds: props.genreIds, kinds: props.kinds, yearMin: props.yearMin, yearMax: props.yearMax }),
)

function localizedGenre(g: FacetGenre): string {
  return locale.value.startsWith('ru') && g.name_ru ? g.name_ru : g.name
}

const filteredGenres = computed(() => {
  const q = genreSearch.value.trim().toLowerCase()
  if (!q) return props.facets.genres
  return props.facets.genres.filter((g) => localizedGenre(g).toLowerCase().includes(q))
})

function toggleGenre(id: string) {
  const next = props.genreIds.includes(id)
    ? props.genreIds.filter((x) => x !== id)
    : [...props.genreIds, id]
  emit('update:genreIds', next)
}

function toggleKind(kind: string) {
  const next = props.kinds.includes(kind)
    ? props.kinds.filter((x) => x !== kind)
    : [...props.kinds, kind]
  emit('update:kinds', next)
}

const years = computed(() => {
  const lo = props.facets.years.min
  const hi = props.facets.years.max
  if (lo === null || hi === null) return []
  const out: number[] = []
  for (let y = hi; y >= lo; y--) out.push(y)
  return out
})

const yearMinStr = computed(() => (props.yearMin === null ? '' : String(props.yearMin)))
const yearMaxStr = computed(() => (props.yearMax === null ? '' : String(props.yearMax)))

const yearMinOptions = computed(() => [
  { value: '', label: '—' },
  ...years.value.map((y) => ({ value: String(y), label: String(y) })),
])
const yearMaxOptions = computed(() => [
  { value: '', label: '—' },
  ...years.value.map((y) => ({ value: String(y), label: String(y) })),
])

function emitYear(which: 'min' | 'max', v: string) {
  const n = v === '' ? null : Number(v)
  if (which === 'min') emit('update:yearMin', n)
  else emit('update:yearMax', n)
}

function clearAll() {
  emit('update:genreIds', [])
  emit('update:kinds', [])
  emit('update:yearMin', null)
  emit('update:yearMax', null)
}
</script>
```

- [ ] **Step 2: Write the component test**

`frontend/web/src/components/profile/WatchlistFilters.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import WatchlistFilters from './WatchlistFilters.vue'
import type { WatchlistFacets } from '@/types/watchlist-facets'

const facets: WatchlistFacets = {
  genres: [
    { id: 'g-action', name: 'Action', name_ru: 'Экшен', count: 5 },
    { id: 'g-comedy', name: 'Comedy', name_ru: 'Комедия', count: 3 },
  ],
  kinds: [
    { kind: 'tv', count: 7 },
    { kind: 'movie', count: 2 },
  ],
  years: { min: 2010, max: 2024 },
}

const i18nStub = {
  global: {
    mocks: { $t: (k: string) => k },
    plugins: [],
  },
}

function mountWith(props: Record<string, unknown> = {}) {
  return mount(WatchlistFilters, {
    props: {
      facets,
      genreIds: [],
      kinds: [],
      yearMin: null,
      yearMax: null,
      ...props,
    },
    global: {
      stubs: { Popover: { template: '<div><slot name="trigger" /><slot /></div>' } },
      mocks: { $t: (k: string) => k },
    },
  })
}

describe('WatchlistFilters', () => {
  it('renders a genre checkbox per facet genre with its count', () => {
    const wrapper = mountWith()
    expect(wrapper.text()).toContain('Action')
    expect(wrapper.text()).toContain('5')
    expect(wrapper.text()).toContain('Comedy')
  })

  it('emits update:genreIds when a genre is toggled on', async () => {
    const wrapper = mountWith()
    const checkbox = wrapper.findComponent({ name: 'Checkbox' })
    await checkbox.vm.$emit('update:modelValue', true)
    expect(wrapper.emitted('update:genreIds')?.[0]).toEqual([['g-action']])
  })

  it('removes a genre when toggled off', async () => {
    const wrapper = mountWith({ genreIds: ['g-action'] })
    const checkbox = wrapper.findComponent({ name: 'Checkbox' })
    await checkbox.vm.$emit('update:modelValue', false)
    expect(wrapper.emitted('update:genreIds')?.[0]).toEqual([[]])
  })

  it('shows the active-filter count badge', () => {
    const wrapper = mountWith({ genreIds: ['g-action'], kinds: ['tv'], yearMin: 2015 })
    // 1 genre + 1 kind + 1 year range = 3
    expect(wrapper.text()).toContain('3')
  })

  it('clear-all emits resets for every dimension', async () => {
    const wrapper = mountWith({ genreIds: ['g-action'], kinds: ['tv'], yearMin: 2015, yearMax: 2020 })
    const clearBtn = wrapper.findAll('button').find((b) => b.text().includes('profile.filters.clear'))
    await clearBtn!.trigger('click')
    expect(wrapper.emitted('update:genreIds')?.[0]).toEqual([[]])
    expect(wrapper.emitted('update:kinds')?.[0]).toEqual([[]])
    expect(wrapper.emitted('update:yearMin')?.[0]).toEqual([null])
    expect(wrapper.emitted('update:yearMax')?.[0]).toEqual([null])
  })

  it('renders AND hint for genres and OR hint for types', () => {
    const wrapper = mountWith()
    expect(wrapper.text()).toContain('profile.filters.genresHint')
    expect(wrapper.text()).toContain('profile.filters.typesHint')
  })
})
```

- [ ] **Step 3: Run the component tests**

Run: `cd frontend/web && bunx vitest run src/components/profile/WatchlistFilters.spec.ts`
Expected: 6 PASS. (If `useI18n()`'s `locale` is undefined under the stub, guard `localizedGenre` with `locale.value?.startsWith?.('ru')` — adjust and re-run.)

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/components/profile/WatchlistFilters.vue frontend/web/src/components/profile/WatchlistFilters.spec.ts
git commit -m "feat(web): WatchlistFilters component (genre AND / type OR / year range)"
```

---

## Task 8: Frontend — wire into `Profile.vue`

**Files:**
- Modify: `frontend/web/src/views/Profile.vue`

- [ ] **Step 1: Import the component, types, and icon**

In the `<script setup>` imports of `Profile.vue`, add:

```ts
import WatchlistFilters from '@/components/profile/WatchlistFilters.vue'
import type { WatchlistFacets, WatchlistFilterState } from '@/types/watchlist-facets'
import { EMPTY_FILTER_STATE, filterParams, filterKey } from '@/types/watchlist-facets'
```

- [ ] **Step 2: Add filter + facets state**

Near the existing `watchlistFilter`/`searchQuery` refs (~line 1013):

```ts
const facets = ref<WatchlistFacets>({ genres: [], kinds: [], years: { min: null, max: null } })
const filterState = ref<WatchlistFilterState>({ ...EMPTY_FILTER_STATE })
```

- [ ] **Step 3: Fold filters into the page-cache key**

Find `pageCacheKey` (the function that returns `${uid}:${status}:${page}:${sortKey.value}:${sortDirection.value}:${q}`, ~line 1042) and append the filter key:

```ts
const q = searchQuery.value.trim().toLowerCase()
return `${uid}:${status}:${page}:${sortKey.value}:${sortDirection.value}:${q}:${filterKey(filterState.value)}`
```

- [ ] **Step 4: Add filter params to both list requests**

In the fetch function (`fetchWatchlistPage`, ~line 1395), add `...filterParams(filterState.value)` to BOTH the `userApi.getWatchlist({...})` and `publicApi.getPublicWatchlist(userId, {...})` params objects, after the `q` spread:

```ts
        ...(trimmedQuery && { q: trimmedQuery }),
        ...filterParams(filterState.value),
```

- [ ] **Step 5: Add a facets loader and call it on init + profile change**

Add a function near the watchlist init logic:

```ts
async function loadFacets() {
  try {
    const resp = _isOwnProfile.value
      ? await userApi.getWatchlistFacets()
      : await publicApi.getPublicWatchlistFacets(profileUser.value!.id)
    facets.value = resp.data?.data || resp.data || { genres: [], kinds: [], years: { min: null, max: null } }
  } catch {
    facets.value = { genres: [], kinds: [], years: { min: null, max: null } }
  }
}
```

Call `loadFacets()` from the same place the watchlist is first initialized (where `_watchlistInitialized` is set), and reset `filterState.value = { ...EMPTY_FILTER_STATE }` there too so switching profiles starts clean.

- [ ] **Step 6: Refetch + reset to page 1 when filters change**

Near the existing `watch(watchlistFilter, ...)` (~line 1525), add:

```ts
watch(filterState, () => {
  watchlistPage.value = 1
  fetchWatchlistPage()
}, { deep: true })
```

- [ ] **Step 7: Render the component in the controls row**

In the template's "View Toggle + Sort" row (~line 119), add the filters control before the `SegmentedControl` (after the sort-direction `Button`):

```vue
                <WatchlistFilters
                  v-model:genre-ids="filterState.genreIds"
                  v-model:kinds="filterState.kinds"
                  v-model:year-min="filterState.yearMin"
                  v-model:year-max="filterState.yearMax"
                  :facets="facets"
                />
```

- [ ] **Step 8: Type-check + run Profile-adjacent tests**

Run: `cd frontend/web && bunx tsc --noEmit && bunx vitest run src/components/profile/`
Expected: no type errors; profile component tests PASS.

- [ ] **Step 9: Commit**

```bash
git add frontend/web/src/views/Profile.vue
git commit -m "feat(web): wire watchlist filters + facets into profile"
```

---

## Task 9: i18n keys

**Files:**
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`

- [ ] **Step 1: Add the `profile.filters` namespace to `en.json`**

Inside the existing `profile` object, add:

```json
"filters": {
  "button": "Filters",
  "genres": "Genres",
  "genresHint": "match all",
  "searchGenres": "Search genres…",
  "types": "Type",
  "typesHint": "match any",
  "year": "Year",
  "clear": "Clear all",
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

- [ ] **Step 2: Add the matching namespace to `ru.json`**

Inside the `profile` object:

```json
"filters": {
  "button": "Фильтры",
  "genres": "Жанры",
  "genresHint": "все сразу",
  "searchGenres": "Поиск жанров…",
  "types": "Тип",
  "typesHint": "любой из",
  "year": "Год",
  "clear": "Сбросить всё",
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

- [ ] **Step 3: Verify locale JSON parses + key parity**

Run: `cd frontend/web && node -e "JSON.parse(require('fs').readFileSync('src/locales/en.json')); JSON.parse(require('fs').readFileSync('src/locales/ru.json')); console.log('ok')"`
Expected: `ok`.
Then run the locale parity test if present: `bunx vitest run src/locales/__tests__/ 2>/dev/null || true`
Expected: PASS (or no such test).

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json
git commit -m "i18n(web): profile.filters keys (en + ru)"
```

---

## Task 10: Full verification + after-update

**Files:** none (verification + deploy)

- [ ] **Step 1: Backend full test + vet**

Run: `cd services/player && go test ./... -count=1 && go vet ./...`
Expected: all PASS, vet clean.

- [ ] **Step 2: Frontend lint + type-check + tests**

Run:
```bash
cd frontend/web
bash scripts/design-system-lint.sh
bunx tsc --noEmit
bunx vitest run src/components/profile/ src/types/
```
Expected: design-system lint ERRORS=0; no type errors; tests PASS.

- [ ] **Step 3: Manual API smoke (own list)**

With a valid `ak_` key (see memory `project_test_user_pattern`), after redeploy:
```bash
curl -s http://localhost:8000/api/users/watchlist/facets -H "Authorization: Bearer ak_xxx" | head
curl -s "http://localhost:8000/api/users/watchlist?kind=tv&year_min=2015" -H "Authorization: Bearer ak_xxx" | head
```
Expected: facets JSON with genres/kinds/years; filtered list returns only matching entries.

- [ ] **Step 4: Run the after-update skill**

Invoke `/animeenigma-after-update` to lint, redeploy `player` + `web`, health-check, write the Russian Trump-mode changelog entry, and commit+push. This is the mandated finishing step.

---

## Self-Review Notes

- **Spec coverage:** genre AND (Task 2 + 7), type OR (Task 2 + 7), year range (Task 2 + 7), facets endpoint with counts (Task 3 + 5), own + public paths (Task 4 + 5), facets over whole list / stable options (Task 3), not persisted (Task 8 resets on profile change, no localStorage), i18n parity (Task 9), tests (Tasks 2/3/7), YAGNI exclusions respected (no cross-filtering, no presets, no score/rewatch filter).
- **Type consistency:** `ListFilters`/`ListFacets` defined in Task 1 used verbatim in Tasks 2–5; `WatchlistFacets`/`WatchlistFilterState` + helpers defined in Task 6 used verbatim in Tasks 7–8; repo method names (`GetByUserPaginated`, `GetByUserAndStatusesPaginated`, `GetListFacets`) and service names (`GetUserListPaginated`, `GetPublicWatchlistPaginated`, `GetListFacets`, `GetPublicListFacets`) consistent across tasks; query param names (`genres`, `kind`, `year_min`, `year_max`) consistent between `filterParams` (FE) and `parseListFilters` (BE).
- **Placeholders:** none — all steps contain concrete code/commands.
