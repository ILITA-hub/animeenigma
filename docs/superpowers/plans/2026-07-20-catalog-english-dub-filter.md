# Catalog English-Dub Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a browse filter for titles that have an English dub, backed by a real per-title flag, and relabel the existing chip that claims to be that filter but reads Kodik's Russian voiceover.

**Architecture:** A new `animes.has_english_dub` column, paired with `english_dub_checked_at` so a background loop can tell "probed, no dub" from "never probed". Two writers feed it: a lazy hook on the existing scraper-episodes path (which already backfills `has_english`), and a slow backfiller goroutine inside catalog that walks `has_english = true` titles one per minute. The browse filter gains a fourth provider key, `endub`.

**Tech Stack:** Go 1.x + GORM + PostgreSQL (catalog service), Vue 3 + TypeScript + vue-i18n (frontend), Prometheus client_golang for metrics.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-20-catalog-english-dub-filter-design.md`.
- Worktree: `/data/animeenigma/.claude/worktrees/english-dub-filter`, branch `feat/catalog-english-dub-filter`. **Never edit `/data/animeenigma/...` paths directly** — absolute paths ignore the worktree and silently write the base tree.
- Every commit carries all three co-author trailers:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- Commit with an explicit pathspec (`git commit -m ... -- path1 path2`). Never `git add -A`: the repo has untracked non-ignored artifacts (`.claude/worktrees/`, `tmp/`, a 33 MB `services/scraper/scraper-api` binary).
- Never run `gofmt -w` or `make fmt` on this repo.
- Effort/impact are scored as UXΔ / CDI / MVQ. Never use days, hours, or sprints.
- Go tests run from the service directory: `cd services/catalog && go test ./...`.
- Frontend uses `bun`, never npm/pnpm. CLI tools via `bunx`, never `npx`.
- Existing browse URL keys (`kodik`, `dub`, `ae`) must keep working — bookmarks depend on them.

---

### Task 1: Schema and repository setter

**Files:**
- Modify: `services/catalog/internal/domain/anime.go` (after the `HasEnglish` field, ~line 81)
- Modify: `services/catalog/internal/repo/anime.go` (doc comment above `animeMetadataColumns` ~line 69; new setter after `SetHasEnglish` ~line 372)
- Modify: `services/catalog/internal/repo/browse_filter_test.go` (DDL ~line 22)
- Modify: `services/catalog/internal/repo/anime_studios_test.go` (DDL ~line 58)
- Modify: `services/catalog/internal/repo/anime_update_test.go` (DDL ~line 61)
- Modify: `services/catalog/internal/service/raw_resolver_test.go` (DDL ~line 113)
- Test: `services/catalog/internal/repo/anime_english_dub_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `domain.Anime.HasEnglishDub bool`, `domain.Anime.EnglishDubCheckedAt *time.Time`, and `(*repo.AnimeRepository).SetEnglishDub(ctx context.Context, animeID string, has bool) error`.

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/repo/anime_english_dub_test.go`:

```go
package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SetEnglishDub must move BOTH columns together. A verdict without a
// timestamp would be re-probed by the backfiller forever.
func TestSetEnglishDub_WritesVerdictAndStamp(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "a1", "Cowboy Bebop")

	require.NoError(t, r.SetEnglishDub(context.Background(), "a1", true))

	var got struct {
		HasEnglishDub       bool
		EnglishDubCheckedAt *string
	}
	require.NoError(t, db.Raw(
		`SELECT has_english_dub, english_dub_checked_at FROM animes WHERE id = 'a1'`,
	).Scan(&got).Error)

	assert.True(t, got.HasEnglishDub, "verdict not written")
	assert.NotNil(t, got.EnglishDubCheckedAt, "checked_at not stamped")
}

// A false verdict is still a verdict: it must stamp too, or the title is
// re-probed on every tick.
func TestSetEnglishDub_FalseVerdictAlsoStamps(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "a1", "Cowboy Bebop")

	require.NoError(t, r.SetEnglishDub(context.Background(), "a1", false))

	var got struct {
		HasEnglishDub       bool
		EnglishDubCheckedAt *string
	}
	require.NoError(t, db.Raw(
		`SELECT has_english_dub, english_dub_checked_at FROM animes WHERE id = 'a1'`,
	).Scan(&got).Error)

	assert.False(t, got.HasEnglishDub)
	assert.NotNil(t, got.EnglishDubCheckedAt, "checked_at not stamped on a false verdict")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/repo/ -run TestSetEnglishDub -v`
Expected: FAIL — compile error `r.SetEnglishDub undefined`.

- [ ] **Step 3: Add the domain fields**

In `services/catalog/internal/domain/anime.go`, immediately after the `HasEnglish` field:

```go
	// HasEnglishDub — the EN scraper chain reported at least one episode
	// carrying a dub track for this anime. Distinct from HasDub, which is
	// Kodik RU voiceover: the browse chip labelled "English (Dub)" read
	// has_dub until 2026-07-20 and was simply mislabelled.
	HasEnglishDub bool `gorm:"default:false;index;column:has_english_dub" json:"has_english_dub"`
	// EnglishDubCheckedAt — when the EN-dub verdict was last established.
	// NULL = never probed. Drives the background backfiller's re-check
	// cadence; internal bookkeeping, so not serialized.
	EnglishDubCheckedAt *time.Time `gorm:"index;column:english_dub_checked_at" json:"-"`
```

- [ ] **Step 4: Add the repository setter**

In `services/catalog/internal/repo/anime.go`, after `SetHasEnglish` (`time` is already imported):

```go
// SetEnglishDub writes the EN-dub verdict for one anime and stamps
// english_dub_checked_at, so the background backfiller can tell "probed, no
// dub" apart from "never probed". Both columns move together — a verdict
// without a timestamp would be re-probed forever. Best-effort at every caller.
func (r *AnimeRepository) SetEnglishDub(ctx context.Context, animeID string, has bool) error {
	return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Updates(map[string]any{
			"has_english_dub":        has,
			"english_dub_checked_at": time.Now().UTC(),
		}).Error
}
```

- [ ] **Step 5: Note the new flag in the metadata-columns doc comment**

In `services/catalog/internal/repo/anime.go`, in the comment above `animeMetadataColumns`, change the phrase:

```
// flags (has_dub/has_kodik/has_animelib/has_raw/has_english), the local
```

to:

```
// flags (has_dub/has_kodik/has_animelib/has_raw/has_english/has_english_dub
// and its english_dub_checked_at stamp), the local
```

The allowlist slice itself is **not** changed — provider flags are deliberately excluded so a Shikimori refresh never clobbers them.

- [ ] **Step 6: Add both columns to the four hand-written test DDLs**

Every one of these tests builds `animes` by hand, so a new domain field breaks them. In each file, extend the `CREATE TABLE animes` statement with:

```sql
		has_english_dub INTEGER DEFAULT 0, english_dub_checked_at DATETIME,
```

Insert it right after the existing `has_english` column in all four:
- `services/catalog/internal/repo/browse_filter_test.go`
- `services/catalog/internal/repo/anime_studios_test.go`
- `services/catalog/internal/repo/anime_update_test.go`
- `services/catalog/internal/service/raw_resolver_test.go`

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/repo/ ./internal/service/ -run 'TestSetEnglishDub|TestAnime|TestBrowse|TestRaw' -v`
Expected: PASS, no compile errors.

- [ ] **Step 8: Commit**

```bash
git commit -m "feat(catalog): add has_english_dub + english_dub_checked_at

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- \
  services/catalog/internal/domain/anime.go \
  services/catalog/internal/repo/anime.go \
  services/catalog/internal/repo/anime_english_dub_test.go \
  services/catalog/internal/repo/browse_filter_test.go \
  services/catalog/internal/repo/anime_studios_test.go \
  services/catalog/internal/repo/anime_update_test.go \
  services/catalog/internal/service/raw_resolver_test.go
```

---

### Task 2: Wire the `endub` filter key

**Files:**
- Modify: `services/catalog/internal/repo/anime.go` (`colsByKey` map, ~line 235)
- Modify: `services/catalog/internal/handler/catalog.go` (providers whitelist switch, ~line 732)
- Test: `services/catalog/internal/repo/browse_filter_test.go` (add a test function)

**Interfaces:**
- Consumes: `domain.Anime.HasEnglishDub` from Task 1.
- Produces: the wire-level provider key `endub`, accepted by `GET /api/anime?providers=endub` and mapped to `has_english_dub`.

- [ ] **Step 1: Write the failing test**

Append to `services/catalog/internal/repo/browse_filter_test.go`:

```go
// The endub provider key selects has_english_dub and nothing else — in
// particular it must NOT alias has_dub, which is Kodik RU voiceover.
func TestSearch_ProviderEndubSelectsEnglishDubOnly(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "endub", "Has English Dub")
	seedBrowseAnime(t, db, "rudub", "Has Kodik RU Dub")
	require.NoError(t, db.Exec(`UPDATE animes SET has_english_dub=1 WHERE id='endub'`).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET has_dub=1 WHERE id='rudub'`).Error)

	got, total, err := r.Search(context.Background(), &domain.SearchFilters{
		Providers: []string{"endub"}, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, got, 1)
	assert.Equal(t, "endub", got[0].ID)
}
```

If `assert` is not yet imported in that file, add `"github.com/stretchr/testify/assert"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/repo/ -run TestSearch_ProviderEndub -v`
Expected: FAIL — `total` is 0, because `endub` is dropped as an unknown key.

- [ ] **Step 3: Map the key in the repository**

In `services/catalog/internal/repo/anime.go`, extend `colsByKey`:

```go
		colsByKey := map[string]string{
			"kodik": "has_kodik",
			"dub":   "has_dub",
			"ae":    "has_video",
			"endub": "has_english_dub",
		}
```

- [ ] **Step 4: Whitelist the key in the handler**

In `services/catalog/internal/handler/catalog.go`, extend the providers switch:

```go
			case "kodik", "dub", "ae", "endub":
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/repo/ ./internal/handler/ ./internal/domain/ -v`
Expected: PASS. `SearchFilters.CacheKey` already sorts and folds in `Providers`, so no cache change is needed and `internal/domain` stays green.

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(catalog): accept endub provider filter key

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- \
  services/catalog/internal/repo/anime.go \
  services/catalog/internal/repo/browse_filter_test.go \
  services/catalog/internal/handler/catalog.go
```

---

### Task 3: Lazy backfill hook on the scraper-episodes path

**Files:**
- Modify: `services/catalog/internal/service/scraper.go` (`animeFetcher` interface ~line 28; `GetScraperEpisodes` body ~line 134)
- Modify: `services/catalog/internal/service/scraper_test.go` (`fakeAnimeFetcher`, ~line 33)
- Test: `services/catalog/internal/service/scraper_english_dub_test.go` (create)

**Interfaces:**
- Consumes: `SetEnglishDub` from Task 1.
- Produces: `parseScraperEpisodes(body []byte) (count int, hasDub bool, ok bool)` — a package-level helper in `service`, reused by Task 5's backfiller for its outcome metric. `animeFetcher` gains `SetEnglishDub(ctx context.Context, animeID string, has bool) error`.

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/scraper_english_dub_test.go`:

```go
package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseScraperEpisodes(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCount int
		wantDub   bool
		wantOK    bool
	}{
		{
			name:      "dub present on one episode",
			body:      `{"data":{"episodes":[{"number":1,"has_dub":false},{"number":2,"has_dub":true}]}}`,
			wantCount: 2, wantDub: true, wantOK: true,
		},
		{
			name:      "sub only",
			body:      `{"data":{"episodes":[{"number":1,"has_dub":false},{"number":2}]}}`,
			wantCount: 2, wantDub: false, wantOK: true,
		},
		{
			name:      "empty episode list is not a verdict",
			body:      `{"data":{"episodes":[]}}`,
			wantCount: 0, wantDub: false, wantOK: false,
		},
		{
			name:      "garbage is not a verdict",
			body:      `not json`,
			wantCount: 0, wantDub: false, wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, dub, ok := parseScraperEpisodes([]byte(tt.body))
			assert.Equal(t, tt.wantCount, count)
			assert.Equal(t, tt.wantDub, dub)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

// Honesty rule: a provider-pinned call may only PROMOTE to true. A sub-only
// answer from one provider must not erase a true verdict established by
// another — miruro in particular is DUB-only.
func TestBackfillEnglishFlags_PinnedCallNeverWritesFalse(t *testing.T) {
	f := &fakeAnimeFetcher{}
	o := &scraperOps{animeRepo: f}

	o.backfillEnglishFlags(t.Context(), "a1", "gogoanime",
		[]byte(`{"data":{"episodes":[{"number":1,"has_dub":false}]}}`))

	assert.True(t, f.hasEnglish, "has_english should still be promoted")
	assert.Nil(t, f.englishDub, "pinned sub-only call must not write a dub verdict")
}

func TestBackfillEnglishFlags_PinnedCallStillPromotesTrue(t *testing.T) {
	f := &fakeAnimeFetcher{}
	o := &scraperOps{animeRepo: f}

	o.backfillEnglishFlags(t.Context(), "a1", "miruro",
		[]byte(`{"data":{"episodes":[{"number":1,"has_dub":true}]}}`))

	if assert.NotNil(t, f.englishDub) {
		assert.True(t, *f.englishDub)
	}
}

func TestBackfillEnglishFlags_UnpinnedCallWritesFalse(t *testing.T) {
	f := &fakeAnimeFetcher{}
	o := &scraperOps{animeRepo: f}

	o.backfillEnglishFlags(t.Context(), "a1", "",
		[]byte(`{"data":{"episodes":[{"number":1,"has_dub":false}]}}`))

	if assert.NotNil(t, f.englishDub) {
		assert.False(t, *f.englishDub, "an unpinned chain-wide call is a real negative verdict")
	}
}
```

- [ ] **Step 2: Extend the test fake**

In `services/catalog/internal/service/scraper_test.go`, add the recording fields and method to `fakeAnimeFetcher` (it already has `SetHasEnglish`; mirror that style). Add these struct fields:

```go
	hasEnglish bool
	englishDub *bool
```

Have the existing `SetHasEnglish` record `f.hasEnglish = has` before returning nil, and add:

```go
func (f *fakeAnimeFetcher) SetEnglishDub(ctx context.Context, animeID string, has bool) error {
	f.englishDub = &has
	return nil
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run 'TestParseScraperEpisodes|TestBackfillEnglishFlags' -v`
Expected: FAIL — `parseScraperEpisodes` and `backfillEnglishFlags` undefined.

- [ ] **Step 4: Add the parser and the hook**

In `services/catalog/internal/service/scraper.go`, add `"encoding/json"` to the imports, and add `SetEnglishDub` to the `animeFetcher` interface:

```go
type animeFetcher interface {
	GetByID(ctx context.Context, id string) (*domain.Anime, error)
	SetHasEnglish(ctx context.Context, animeID string, has bool) error
	SetEnglishDub(ctx context.Context, animeID string, has bool) error
}
```

Then add, near `GetScraperEpisodes`:

```go
// parseScraperEpisodes reads the episode list out of a scraper-episodes
// response. Returns the episode count, whether ANY episode carries a dub
// track, and ok=false when the body is undecodable or the list is empty —
// neither is a verdict. Mirrors the envelope parsed by
// animeLevelResolver.latestEnglish; shared with the EN-dub backfiller.
func parseScraperEpisodes(body []byte) (int, bool, bool) {
	var env struct {
		Data struct {
			Episodes []struct {
				Number int  `json:"number"`
				HasDub bool `json:"has_dub"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return 0, false, false
	}
	eps := env.Data.Episodes
	if len(eps) == 0 {
		return 0, false, false
	}
	for _, e := range eps {
		if e.HasDub {
			return len(eps), true, true
		}
	}
	return len(eps), false, true
}

// backfillEnglishFlags opportunistically flips animes.has_english and
// animes.has_english_dub from a successful scraper-episodes response.
// Best-effort throughout: the caller already has its episodes, so decode and
// write failures are swallowed.
//
// Honesty rule: a NEGATIVE dub verdict is written only for an unpinned call
// (prefer == ""), which consulted the whole failover chain. A call pinned to
// one provider may only promote to true — otherwise a sub-only gogoanime
// answer would erase a true verdict from miruro, which is DUB-only.
func (o *scraperOps) backfillEnglishFlags(ctx context.Context, animeID, prefer string, body []byte) {
	_, hasDub, ok := parseScraperEpisodes(body)
	if !ok {
		return
	}
	_ = o.animeRepo.SetHasEnglish(ctx, animeID, true)
	if !hasDub && prefer != "" {
		return
	}
	_ = o.animeRepo.SetEnglishDub(ctx, animeID, hasDub)
}
```

- [ ] **Step 5: Replace the substring scan in `GetScraperEpisodes`**

Replace this block:

```go
	if gErr == nil && status == 200 && len(body) > 0 &&
		strings.Contains(string(body), `"episodes":[{`) {
		if uerr := o.animeRepo.SetHasEnglish(ctx, animeID, true); uerr != nil {
			// Log only — never propagate. The user got their episodes;
			// the column will backfill on the next hit if this one
			// transiently failed.
			_ = uerr
		}
	}
```

with:

```go
	if gErr == nil && status == 200 && len(body) > 0 {
		o.backfillEnglishFlags(ctx, animeID, prefer, body)
	}
```

Also delete the now-stale comment above `GetScraperEpisodes` that describes the substring scan, replacing the "Phase 26" paragraph's last sentence with: `Both has_english and has_english_dub are set from the decoded episode list; see backfillEnglishFlags.` Run `go build ./...` and drop the `strings` import only if the compiler reports it unused.

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/ -run 'TestParseScraperEpisodes|TestBackfillEnglishFlags|TestGetScraperEpisodes' -v`
Expected: PASS.

Then the full package: `cd services/catalog && go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git commit -m "feat(catalog): set has_english_dub from scraper episode list

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- \
  services/catalog/internal/service/scraper.go \
  services/catalog/internal/service/scraper_test.go \
  services/catalog/internal/service/scraper_english_dub_test.go
```

---

### Task 4: Repository queries for the backfiller

**Files:**
- Modify: `services/catalog/internal/domain/anime.go` (new candidate struct, after `SearchFilters`)
- Modify: `services/catalog/internal/repo/anime.go` (three methods after `SetEnglishDub`)
- Test: `services/catalog/internal/repo/anime_english_dub_test.go` (extend)

**Interfaces:**
- Consumes: `SetEnglishDub` and the schema from Task 1.
- Produces, all on `*repo.AnimeRepository`:
  - `ListEnglishDubCandidates(ctx context.Context, limit int, ongoingAge, staleAge time.Duration) ([]domain.EnglishDubCandidate, error)`
  - `TouchEnglishDubChecked(ctx context.Context, animeID string) error`
  - `CountEnglishDubUnchecked(ctx context.Context) (int64, error)`
  - `PromoteVerifiedEnglishDubs(ctx context.Context) (int64, error)`
  - `domain.EnglishDubCandidate{ID, Name, Status string}`

- [ ] **Step 1: Write the failing test**

Append to `services/catalog/internal/repo/anime_english_dub_test.go`:

```go
// The candidate query must never return a title without an EN source: no EN
// source means no EN dub, and probing them would put ~4800 pointless calls on
// the wire.
func TestListEnglishDubCandidates_SkipsNonEnglishTitles(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "en", "Has EN source")
	seedBrowseAnime(t, db, "noen", "No EN source")
	require.NoError(t, db.Exec(`UPDATE animes SET has_english=1 WHERE id='en'`).Error)

	got, err := r.ListEnglishDubCandidates(context.Background(), 10, 7*24*time.Hour, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "en", got[0].ID)
}

// Never-probed titles outrank stale ones.
func TestListEnglishDubCandidates_NeverProbedFirst(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "stale", "Probed long ago")
	seedBrowseAnime(t, db, "fresh", "Never probed")
	require.NoError(t, db.Exec(
		`UPDATE animes SET has_english=1, english_dub_checked_at=? WHERE id='stale'`,
		time.Now().UTC().Add(-90*24*time.Hour),
	).Error)
	require.NoError(t, db.Exec(`UPDATE animes SET has_english=1 WHERE id='fresh'`).Error)

	got, err := r.ListEnglishDubCandidates(context.Background(), 10, 7*24*time.Hour, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "fresh", got[0].ID, "never-probed must sort first")
}

// A recently probed released title is not a candidate.
func TestListEnglishDubCandidates_ExcludesFreshlyProbed(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "done", "Probed yesterday")
	require.NoError(t, db.Exec(
		`UPDATE animes SET has_english=1, status='released', english_dub_checked_at=? WHERE id='done'`,
		time.Now().UTC().Add(-24*time.Hour),
	).Error)

	got, err := r.ListEnglishDubCandidates(context.Background(), 10, 7*24*time.Hour, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// Ongoing titles come back sooner than released ones — dubs ship after subs.
func TestListEnglishDubCandidates_OngoingRecheckedSooner(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "ong", "Ongoing probed 10 days ago")
	require.NoError(t, db.Exec(
		`UPDATE animes SET has_english=1, status='ongoing', english_dub_checked_at=? WHERE id='ong'`,
		time.Now().UTC().Add(-10*24*time.Hour),
	).Error)

	got, err := r.ListEnglishDubCandidates(context.Background(), 10, 7*24*time.Hour, 30*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "ong", got[0].ID)
}

// An unreachable probe must still stamp, or the same title is retried on
// every tick and the loop never rotates.
func TestTouchEnglishDubChecked_StampsWithoutChangingVerdict(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "a1", "Unreachable")
	require.NoError(t, db.Exec(`UPDATE animes SET has_english=1, has_english_dub=1 WHERE id='a1'`).Error)

	require.NoError(t, r.TouchEnglishDubChecked(context.Background(), "a1"))

	var got struct {
		HasEnglishDub       bool
		EnglishDubCheckedAt *string
	}
	require.NoError(t, db.Raw(
		`SELECT has_english_dub, english_dub_checked_at FROM animes WHERE id = 'a1'`,
	).Scan(&got).Error)
	assert.True(t, got.HasEnglishDub, "verdict must be preserved")
	assert.NotNil(t, got.EnglishDubCheckedAt)
}

func TestCountEnglishDubUnchecked(t *testing.T) {
	db := setupBrowseFilterTestDB(t)
	r := &AnimeRepository{db: db}
	seedBrowseAnime(t, db, "a", "unchecked")
	seedBrowseAnime(t, db, "b", "checked")
	seedBrowseAnime(t, db, "c", "no en source")
	require.NoError(t, db.Exec(`UPDATE animes SET has_english=1 WHERE id IN ('a','b')`).Error)
	require.NoError(t, db.Exec(
		`UPDATE animes SET english_dub_checked_at=? WHERE id='b'`, time.Now().UTC(),
	).Error)

	n, err := r.CountEnglishDubUnchecked(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
}
```

Add `"time"` to that file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/repo/ -run 'TestListEnglishDubCandidates|TestTouchEnglishDub|TestCountEnglishDub' -v`
Expected: FAIL — the methods are undefined.

- [ ] **Step 3: Add the candidate struct**

In `services/catalog/internal/domain/anime.go`, after the `SearchFilters` struct:

```go
// EnglishDubCandidate is one title the EN-dub backfiller may probe. A
// projection, not a full Anime — the loop only needs an identity and the
// status that decides the re-check cadence.
type EnglishDubCandidate struct {
	ID     string
	Name   string
	Status string
}
```

- [ ] **Step 4: Add the repository methods**

In `services/catalog/internal/repo/anime.go`, after `SetEnglishDub`:

```go
// ListEnglishDubCandidates returns up to limit titles whose EN-dub verdict is
// missing or stale, most-deserving first:
//
//	1. never probed (english_dub_checked_at IS NULL)
//	2. ongoing and last probed more than ongoingAge ago — dubs ship after subs
//	3. anything last probed more than staleAge ago
//
// Only has_english = true rows are ever returned: no EN source means no EN
// dub, and the restriction keeps thousands of pointless provider calls off
// the wire.
func (r *AnimeRepository) ListEnglishDubCandidates(ctx context.Context, limit int, ongoingAge, staleAge time.Duration) ([]domain.EnglishDubCandidate, error) {
	now := time.Now().UTC()
	var out []domain.EnglishDubCandidate
	err := r.db.WithContext(ctx).Model(&domain.Anime{}).
		Select("id, name, status").
		Where("has_english = ?", true).
		Where(`english_dub_checked_at IS NULL
			OR (status = ? AND english_dub_checked_at < ?)
			OR english_dub_checked_at < ?`,
			"ongoing", now.Add(-ongoingAge), now.Add(-staleAge)).
		// Portable NULLS FIRST: `IS NULL` is 1/true for unprobed rows on both
		// sqlite (tests) and postgres (production), so DESC floats them up.
		Order("english_dub_checked_at IS NULL DESC, english_dub_checked_at ASC").
		Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("list english dub candidates: %w", err)
	}
	return out, nil
}

// TouchEnglishDubChecked stamps english_dub_checked_at without touching the
// verdict. The backfiller calls it when a probe was inconclusive (provider
// unreachable, non-200): without the stamp the same title would be re-picked
// on every tick and the loop would never rotate.
func (r *AnimeRepository) TouchEnglishDubChecked(ctx context.Context, animeID string) error {
	return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("english_dub_checked_at", time.Now().UTC()).Error
}

// CountEnglishDubUnchecked reports how many EN-sourced titles have never had
// an EN-dub verdict established. Exported as a gauge so the backfill's
// catch-up progress is visible.
func (r *AnimeRepository) CountEnglishDubUnchecked(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&domain.Anime{}).
		Where("has_english = ? AND english_dub_checked_at IS NULL", true).
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("count english dub unchecked: %w", err)
	}
	return n, nil
}

// PromoteVerifiedEnglishDubs flips has_english_dub for every anime with an
// audio-verified English unit in content_verifications. That table belongs to
// the content-verify service; this is a read-only join into it. Verified audio
// outranks a provider's has_dub metadata claim, so this may promote a title
// the scraper pass concluded false on. Postgres-only (jsonb) — callers treat
// an error as non-fatal so the sqlite-backed tests and any pre-content-verify
// deployment keep working. Returns the number of rows promoted.
func (r *AnimeRepository) PromoteVerifiedEnglishDubs(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE animes SET has_english_dub = true, english_dub_checked_at = NOW()
		WHERE has_english_dub = false
		  AND id IN (
			SELECT cv.anime_id
			FROM content_verifications cv,
			     LATERAL jsonb_array_elements(cv.units) u
			WHERE u->>'status' = 'verified'
			  AND u->'audio'->>'lang' = 'en'
			  AND (u->'audio'->>'verified')::boolean
		  )`)
	if res.Error != nil {
		return 0, fmt.Errorf("promote verified english dubs: %w", res.Error)
	}
	return res.RowsAffected, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/repo/ -v`
Expected: PASS. `PromoteVerifiedEnglishDubs` has no unit test — it is Postgres-only jsonb SQL and is verified against production in Task 7.

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(catalog): repo queries for the EN-dub backfiller

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- \
  services/catalog/internal/domain/anime.go \
  services/catalog/internal/repo/anime.go \
  services/catalog/internal/repo/anime_english_dub_test.go
```

---

### Task 5: Backfiller goroutine

**Files:**
- Create: `services/catalog/internal/service/english_dub_backfill.go`
- Create: `services/catalog/internal/service/english_dub_metrics.go`
- Test: `services/catalog/internal/service/english_dub_backfill_test.go` (create)
- Modify: `services/catalog/internal/config/config.go` (new config struct ~line 87; field on `Config` ~line 25; loader entry ~line 210)
- Modify: `services/catalog/cmd/catalog-api/main.go` (after the `healthChecker` block, ~line 536)

**Interfaces:**
- Consumes: `parseScraperEpisodes` (Task 3); `ListEnglishDubCandidates`, `TouchEnglishDubChecked`, `CountEnglishDubUnchecked`, `PromoteVerifiedEnglishDubs` (Task 4).
- Produces: `NewEnglishDubBackfiller(repo englishDubRepo, probe englishDubProbe, shed shedChecker, cfg EnglishDubBackfillConfig, log *logger.Logger) *EnglishDubBackfiller` with a `Start(ctx context.Context)` loop, and `config.EnglishDubConfig`.

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/english_dub_backfill_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEnglishDubRepo struct {
	candidates []domain.EnglishDubCandidate
	touched    []string
	promoted   int64
	promoteErr error
}

func (f *fakeEnglishDubRepo) ListEnglishDubCandidates(ctx context.Context, limit int, ongoingAge, staleAge time.Duration) ([]domain.EnglishDubCandidate, error) {
	return f.candidates, nil
}
func (f *fakeEnglishDubRepo) TouchEnglishDubChecked(ctx context.Context, animeID string) error {
	f.touched = append(f.touched, animeID)
	return nil
}
func (f *fakeEnglishDubRepo) CountEnglishDubUnchecked(ctx context.Context) (int64, error) {
	return int64(len(f.candidates)), nil
}
func (f *fakeEnglishDubRepo) PromoteVerifiedEnglishDubs(ctx context.Context) (int64, error) {
	return f.promoted, f.promoteErr
}

type fakeEnglishDubProbe struct {
	calls  []string
	prefer []string
	status int
	body   string
	err    error
}

func (f *fakeEnglishDubProbe) GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error) {
	f.calls = append(f.calls, animeID)
	f.prefer = append(f.prefer, prefer)
	return f.status, []byte(f.body), f.err
}

type fakeShed struct{ level int }

func (f *fakeShed) ShouldShed(min int) bool { return f.level >= min }

func newTestBackfiller(r englishDubRepo, p englishDubProbe, s shedChecker) *EnglishDubBackfiller {
	return NewEnglishDubBackfiller(r, p, s, EnglishDubBackfillConfig{
		Interval:   time.Minute,
		OngoingAge: 7 * 24 * time.Hour,
		StaleAge:   30 * 24 * time.Hour,
	}, logger.Default())
}

// The probe must run unpinned so the hook is allowed to write a negative
// verdict — a pinned call can only ever promote.
func TestBackfiller_ProbesUnpinned(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1,"has_dub":true}]}}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	require.Equal(t, []string{"a1"}, p.calls)
	assert.Equal(t, []string{""}, p.prefer, "backfill probe must not be pinned to a provider")
}

// An unreachable provider must still stamp, or the loop re-picks the same
// title on every tick forever.
func TestBackfiller_StampsOnFailedProbe(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{err: errors.New("scraper unreachable")}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Equal(t, []string{"a1"}, r.touched)
}

func TestBackfiller_StampsOnNon200(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 503, body: `{}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Equal(t, []string{"a1"}, r.touched)
}

// A 200 with an empty list is not a verdict either — stamp so we rotate.
func TestBackfiller_StampsOnEmptyEpisodeList(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[]}}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Equal(t, []string{"a1"}, r.touched)
}

// On a good verdict the hook inside GetScraperEpisodes owns the write, so the
// backfiller must NOT stamp on top of it.
func TestBackfiller_DoesNotTouchOnGoodVerdict(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1,"has_dub":false}]}}`}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Empty(t, r.touched, "the lazy hook already wrote the verdict")
}

func TestBackfiller_ShedsUnderPressure(t *testing.T) {
	r := &fakeEnglishDubRepo{candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}}}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1}]}}`}

	newTestBackfiller(r, p, &fakeShed{level: 1}).tick(context.Background())

	assert.Empty(t, p.calls, "no provider calls while the governor reports Elevated+")
}

func TestBackfiller_NoCandidatesIsQuiet(t *testing.T) {
	r := &fakeEnglishDubRepo{}
	p := &fakeEnglishDubProbe{}

	newTestBackfiller(r, p, &fakeShed{}).tick(context.Background())

	assert.Empty(t, p.calls)
	assert.Empty(t, r.touched)
}

// A missing content_verifications table (pre-content-verify deploy) must not
// kill the loop.
func TestBackfiller_PromoteErrorIsNonFatal(t *testing.T) {
	r := &fakeEnglishDubRepo{
		candidates: []domain.EnglishDubCandidate{{ID: "a1", Name: "X"}},
		promoteErr: errors.New("relation content_verifications does not exist"),
	}
	p := &fakeEnglishDubProbe{status: 200, body: `{"data":{"episodes":[{"number":1,"has_dub":true}]}}`}
	b := newTestBackfiller(r, p, &fakeShed{})

	b.promote(context.Background())
	b.tick(context.Background())

	assert.Equal(t, []string{"a1"}, p.calls, "probe must still run after a failed promote")
}
```

`logger.Default()` is what the neighbouring `health_checker_test.go` uses; import `"github.com/ILITA-hub/animeenigma/libs/logger"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run TestBackfiller -v`
Expected: FAIL — `NewEnglishDubBackfiller`, `EnglishDubBackfillConfig`, `englishDubRepo`, `englishDubProbe`, `shedChecker` undefined.

- [ ] **Step 3: Write the metrics**

Create `services/catalog/internal/service/english_dub_metrics.go`:

```go
package service

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// These live in the catalog service, NOT in libs/metrics. A plain (non-Vec)
// promauto metric placed in libs/metrics registers at import time in every
// service that imports the package and exports a permanent 0 from each,
// creating impostor series. catalog is the only emitter here, so the metrics
// belong to catalog.
var (
	// englishDubBackfillTotal counts probe outcomes:
	//	dub      — the title has an EN dub
	//	nodub    — probed, no dub
	//	stamped  — inconclusive (unreachable / non-200 / empty list); only the
	//	           timestamp moved, so the loop rotates
	//	error    — the candidate query itself failed
	//	shed     — skipped, governor reported Elevated+
	englishDubBackfillTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "catalog_english_dub_backfill_total",
		Help: "EN-dub backfill probe outcomes by result.",
	}, []string{"result"})

	// englishDubPromotedTotal counts titles promoted from an audio-verified
	// content-verify verdict, which outranks provider metadata.
	englishDubPromotedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "catalog_english_dub_promoted_total",
		Help: "Titles promoted to has_english_dub from a verified content-verify audio verdict.",
	})

	// englishDubUnchecked is the catch-up gauge: EN-sourced titles that have
	// never had an EN-dub verdict established.
	englishDubUnchecked = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "catalog_english_dub_unchecked",
		Help: "Titles with has_english=true whose EN-dub verdict has never been established.",
	})
)
```

- [ ] **Step 4: Write the backfiller**

Create `services/catalog/internal/service/english_dub_backfill.go`:

```go
package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// englishDubRepo is the minimal anime-repository surface the backfiller needs.
// Production wiring satisfies it via *repo.AnimeRepository.
type englishDubRepo interface {
	ListEnglishDubCandidates(ctx context.Context, limit int, ongoingAge, staleAge time.Duration) ([]domain.EnglishDubCandidate, error)
	TouchEnglishDubChecked(ctx context.Context, animeID string) error
	CountEnglishDubUnchecked(ctx context.Context) (int64, error)
	PromoteVerifiedEnglishDubs(ctx context.Context) (int64, error)
}

// englishDubProbe is the scraper-episodes surface the backfiller drives.
// Production wiring passes *scraperOps, whose GetScraperEpisodes performs the
// has_english / has_english_dub write as a side effect — the backfiller never
// writes a verdict itself, it only decides when to ask.
type englishDubProbe interface {
	GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error)
}

// shedChecker is satisfied by *cache.DegradationWatcher.
type shedChecker interface {
	ShouldShed(min int) bool
}

// EnglishDubBackfillConfig tunes the loop.
type EnglishDubBackfillConfig struct {
	Interval     time.Duration
	OngoingAge   time.Duration
	StaleAge     time.Duration
	PromoteEvery time.Duration
}

// EnglishDubBackfiller keeps animes.has_english_dub fresh. It probes exactly
// ONE title per tick: the lazy hook on the scraper-episodes path covers titles
// users actually open, and this loop exists only to reach the long tail
// without putting meaningful load on providers (each probe fans out to real
// upstreams, some through the Camoufox sidecar).
type EnglishDubBackfiller struct {
	repo  englishDubRepo
	probe englishDubProbe
	shed  shedChecker
	cfg   EnglishDubBackfillConfig
	log   *logger.Logger
}

func NewEnglishDubBackfiller(repo englishDubRepo, probe englishDubProbe, shed shedChecker, cfg EnglishDubBackfillConfig, log *logger.Logger) *EnglishDubBackfiller {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	if cfg.OngoingAge <= 0 {
		cfg.OngoingAge = 7 * 24 * time.Hour
	}
	if cfg.StaleAge <= 0 {
		cfg.StaleAge = 30 * 24 * time.Hour
	}
	if cfg.PromoteEvery <= 0 {
		cfg.PromoteEvery = time.Hour
	}
	return &EnglishDubBackfiller{repo: repo, probe: probe, shed: shed, cfg: cfg, log: log}
}

// Start runs until ctx is cancelled.
func (b *EnglishDubBackfiller) Start(ctx context.Context) {
	b.log.Infow("english dub backfiller started",
		"interval", b.cfg.Interval.String(),
		"ongoing_age", b.cfg.OngoingAge.String(),
		"stale_age", b.cfg.StaleAge.String(),
	)

	b.promote(ctx)

	ticker := time.NewTicker(b.cfg.Interval)
	defer ticker.Stop()
	promoteTicker := time.NewTicker(b.cfg.PromoteEvery)
	defer promoteTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.log.Info("english dub backfiller stopped")
			return
		case <-promoteTicker.C:
			b.promote(ctx)
		case <-ticker.C:
			b.tick(ctx)
		}
	}
}

// promote runs the network-free content-verify pass. Audio-verified English
// beats provider metadata, so this may flip a title the scraper pass
// concluded false on. Non-fatal: content-verify is a separate service and its
// table may not exist in every deployment.
func (b *EnglishDubBackfiller) promote(ctx context.Context) {
	n, err := b.repo.PromoteVerifiedEnglishDubs(ctx)
	if err != nil {
		b.log.Warnw("english dub promote from content-verify failed", "error", err)
		return
	}
	if n > 0 {
		englishDubPromotedTotal.Add(float64(n))
		b.log.Infow("english dub promoted from verified audio", "count", n)
	}
}

// tick probes at most one title.
func (b *EnglishDubBackfiller) tick(ctx context.Context) {
	if b.shed != nil && b.shed.ShouldShed(1) {
		englishDubBackfillTotal.WithLabelValues("shed").Inc()
		return
	}

	if n, err := b.repo.CountEnglishDubUnchecked(ctx); err == nil {
		englishDubUnchecked.Set(float64(n))
	}

	candidates, err := b.repo.ListEnglishDubCandidates(ctx, 1, b.cfg.OngoingAge, b.cfg.StaleAge)
	if err != nil {
		englishDubBackfillTotal.WithLabelValues("error").Inc()
		b.log.Warnw("english dub candidate query failed", "error", err)
		return
	}
	if len(candidates) == 0 {
		return
	}
	c := candidates[0]

	// prefer="" on purpose: only a chain-wide answer earns the right to write
	// a NEGATIVE verdict (see backfillEnglishFlags' honesty rule).
	status, body, err := b.probe.GetScraperEpisodes(ctx, c.ID, "", false)
	if err != nil || status != 200 {
		b.stamp(ctx, c, "probe unreachable", err, status)
		return
	}
	_, hasDub, ok := parseScraperEpisodes(body)
	if !ok {
		b.stamp(ctx, c, "no episodes in response", nil, status)
		return
	}

	// The verdict was already persisted by the hook inside GetScraperEpisodes.
	result := "nodub"
	if hasDub {
		result = "dub"
	}
	englishDubBackfillTotal.WithLabelValues(result).Inc()
	b.log.Infow("english dub verdict", "anime_id", c.ID, "name", c.Name, "has_dub", hasDub)
}

// stamp records an inconclusive probe so the loop rotates to the next title
// instead of retrying this one every tick.
func (b *EnglishDubBackfiller) stamp(ctx context.Context, c domain.EnglishDubCandidate, reason string, err error, status int) {
	englishDubBackfillTotal.WithLabelValues("stamped").Inc()
	if terr := b.repo.TouchEnglishDubChecked(ctx, c.ID); terr != nil {
		b.log.Warnw("english dub stamp failed", "anime_id", c.ID, "error", terr)
	}
	b.log.Debugw("english dub probe inconclusive",
		"anime_id", c.ID, "name", c.Name, "reason", reason, "status", status, "error", err)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/ -run TestBackfiller -v`
Expected: PASS, all nine cases.

- [ ] **Step 6: Add the config block**

In `services/catalog/internal/config/config.go`, add the struct next to `HealthCheckConfig`:

```go
type EnglishDubConfig struct {
	Interval     time.Duration
	OngoingAge   time.Duration
	StaleAge     time.Duration
	PromoteEvery time.Duration
}
```

Add the field to `Config`, next to `HealthCheck HealthCheckConfig`:

```go
	EnglishDub  EnglishDubConfig
```

And the loader entry, right after the `HealthCheck:` block:

```go
		EnglishDub: EnglishDubConfig{
			Interval:     getEnvDuration("CATALOG_ENDUB_BACKFILL_INTERVAL", time.Minute),
			OngoingAge:   getEnvDuration("CATALOG_ENDUB_ONGOING_AGE", 7*24*time.Hour),
			StaleAge:     getEnvDuration("CATALOG_ENDUB_STALE_AGE", 30*24*time.Hour),
			PromoteEvery: getEnvDuration("CATALOG_ENDUB_PROMOTE_INTERVAL", time.Hour),
		},
```

- [ ] **Step 7: Wire it in main**

In `services/catalog/cmd/catalog-api/main.go`, directly after `go healthChecker.Start(healthCtx)`:

```go
	// EN-dub backfill. The lazy hook on the scraper-episodes path covers
	// titles users open; this loop reaches the long tail at one title per
	// minute. It pauses entirely while the governor reports Elevated+ —
	// every probe fans out to real upstream providers.
	endubShed := cache.NewDegradationWatcher(redisCache, 5*time.Second)
	endubShed.Start(healthCtx)
	endubBackfiller := service.NewEnglishDubBackfiller(
		animeRepo,
		catalogService,
		endubShed,
		service.EnglishDubBackfillConfig{
			Interval:     cfg.EnglishDub.Interval,
			OngoingAge:   cfg.EnglishDub.OngoingAge,
			StaleAge:     cfg.EnglishDub.StaleAge,
			PromoteEvery: cfg.EnglishDub.PromoteEvery,
		},
		log,
	)
	go endubBackfiller.Start(healthCtx)
```

`*CatalogService` already exposes `GetScraperEpisodes(ctx, animeID, prefer string, exclusive bool) (int, []byte, error)` (`services/catalog/internal/service/scraper.go`, delegating to the private `scraperOps()`), so it satisfies `englishDubProbe` as-is. No accessor and no second scraper client.

- [ ] **Step 8: Build and run the full suite**

Run: `cd services/catalog && go build ./... && go test ./...`
Expected: build succeeds, all tests PASS.

- [ ] **Step 9: Commit**

```bash
git commit -m "feat(catalog): background EN-dub backfill goroutine

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- \
  services/catalog/internal/service/english_dub_backfill.go \
  services/catalog/internal/service/english_dub_backfill_test.go \
  services/catalog/internal/service/english_dub_metrics.go \
  services/catalog/internal/config/config.go \
  services/catalog/cmd/catalog-api/main.go
```

---

### Task 6: Frontend filter chip and locales

**Files:**
- Modify: `frontend/web/src/composables/useBrowseFilters.ts` (`Provider` type ~line 16, `PROVIDER_VALUES` ~line 18)
- Modify: `frontend/web/src/components/browse/BrowseSidebar.vue` (`providerOptions` ~line 220)
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`
- Test: `frontend/web/src/composables/useBrowseFilters.spec.ts` (extend — it already exists, alongside the composable rather than in a `__tests__/` directory)

**Interfaces:**
- Consumes: the `endub` wire key from Task 2.
- Produces: no downstream consumers — this is the last task in the chain.

- [ ] **Step 1: Write the failing test**

Append to the existing `frontend/web/src/composables/useBrowseFilters.spec.ts`:

```ts
it('round-trips the endub provider through the URL', async () => {
  const filters = useBrowseFilters()
  filters.providers.value = ['endub']
  filters.writeUrl()
  await nextTick()
  expect(router.currentRoute.value.query.providers).toBe('endub')
})

it('accepts endub when reading the URL', async () => {
  await router.push({ query: { providers: 'endub' } })
  const filters = useBrowseFilters()
  filters.readUrl()
  expect(filters.providers.value).toEqual(['endub'])
})
```

Match the surrounding file's router/test harness — read the neighbouring tests before writing, and reuse their setup verbatim rather than inventing one.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useBrowseFilters.spec.ts`
Expected: FAIL — `endub` is filtered out as an unknown provider value.

- [ ] **Step 3: Extend the Provider union**

In `frontend/web/src/composables/useBrowseFilters.ts`:

```ts
export type Provider = 'kodik' | 'dub' | 'ae' | 'endub'

const PROVIDER_VALUES: Provider[] = ['kodik', 'dub', 'ae', 'endub']
```

- [ ] **Step 4: Rebuild the chip list**

In `frontend/web/src/components/browse/BrowseSidebar.vue`, replace the whole `providerOptions` computed with:

```ts
// Per-provider brand/identity accents (DS brand-exempt hues + a semantic token).
// Ordered by what users actually filter on: dub language first, then source.
const providerOptions = computed<{ value: Provider; label: string; accent: string }[]>(() => [
  {
    value: 'dub',
    // Kodik RU voiceover (has_dub). Labelled "English (Dub)" until 2026-07-20,
    // which was simply wrong — has_dub is written only from Kodik translations
    // with type=="voice", and Kodik is the RU family.
    label: t('browse.filters.provider.dub'),
    accent: 'text-success focus:ring-success',
  },
  {
    value: 'endub',
    // Real English dub (has_english_dub), from the EN scraper chain's
    // per-episode has_dub plus verified content-verify audio.
    label: t('browse.filters.provider.endub'),
    accent: 'text-teal-400 focus:ring-teal-400',
  },
  {
    value: 'kodik',
    label: t('browse.filters.provider.kodik'),
    accent: 'text-cyan-500 focus:ring-cyan-500',
  },
  {
    value: 'ae',
    // First-party AnimeEnigma (has_video) — indigo brand-exempt accent.
    label: t('browse.filters.provider.ae'),
    accent: 'text-indigo-400 focus:ring-indigo-400',
  },
])
```

- [ ] **Step 5: Update all three locales**

All three files must change together — the i18n parity gate fails otherwise.

`frontend/web/src/locales/en.json`:
```json
        "provider": "Dub and sources",
```
```json
      "provider": {
        "kodik": "Kodik (RU)",
        "dub": "Russian dub (Kodik)",
        "endub": "English dub",
        "ae": "AnimeEnigma"
      },
```

`frontend/web/src/locales/ru.json`:
```json
        "provider": "Озвучка и источники",
```
```json
      "provider": {
        "kodik": "Kodik (RU)",
        "dub": "Русская озвучка (Kodik)",
        "endub": "Английская озвучка",
        "ae": "AnimeEnigma"
      },
```

`frontend/web/src/locales/ja.json`:
```json
        "provider": "吹替とソース",
```
```json
      "provider": {
        "kodik": "Kodik (RU)",
        "dub": "ロシア語吹替（Kodik）",
        "endub": "英語吹替",
        "ae": "AnimeEnigma"
      },
```

The section title lives at `browse.filters.section.provider`; the chip labels at `browse.filters.provider.*`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/__tests__/useBrowseFilters.spec.ts`
Expected: PASS.

- [ ] **Step 7: Run the frontend pre-flight gate**

Run: `/frontend-verify`

This is mandatory for any `frontend/web/` change: it runs the design-system lint (`teal` is on the brand-hue exemption list, so the new accent passes), the en/ru/ja parity check, and a real `bun run build`. Fix anything it reports before committing.

- [ ] **Step 8: Commit**

```bash
git commit -m "feat(browse): add English-dub filter, relabel the RU dub chip

The provider chip labelled 'English (Dub)' in all three locales actually
filtered has_dub — Kodik RU voiceover. It now says so, and a real English-dub
chip sits next to it backed by has_english_dub.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>" -- \
  frontend/web/src/composables/useBrowseFilters.ts \
  frontend/web/src/components/browse/BrowseSidebar.vue \
  frontend/web/src/locales/en.json \
  frontend/web/src/locales/ru.json \
  frontend/web/src/locales/ja.json \
  frontend/web/src/composables/useBrowseFilters.spec.ts
```

---

### Task 7: Verify against production data

**Files:** none — verification only.

**Interfaces:**
- Consumes: everything above.
- Produces: evidence that the column, the promote SQL, and the filter behave on real data.

- [ ] **Step 1: Confirm the columns exist after AutoMigrate**

Deploy per the normal flow, then:

```bash
docker exec animeenigma-postgres psql -U postgres -d animeenigma -c \
  "\d animes" | grep english_dub
```
Expected: both `has_english_dub` and `english_dub_checked_at` listed.

- [ ] **Step 2: Verify the promote SQL against real content_verifications**

```bash
docker exec animeenigma-postgres psql -U postgres -d animeenigma -tAc \
  "SELECT count(DISTINCT cv.anime_id) FROM content_verifications cv,
     LATERAL jsonb_array_elements(cv.units) u
   WHERE u->>'status'='verified' AND u->'audio'->>'lang'='en'
     AND (u->'audio'->>'verified')::boolean;"
```
Expected: a small non-negative integer (5 at the time of writing). This is the query embedded in `PromoteVerifiedEnglishDubs`; a syntax error here means the method is broken, since it is the one piece with no unit test.

- [ ] **Step 3: Watch the backfiller run**

```bash
make logs-catalog | grep -i "english dub"
```
Expected within a couple of minutes: `english dub backfiller started`, then per-title `english dub verdict` lines. If instead you see the same `anime_id` repeatedly, the stamp path is broken — that is the loop-forever bug Task 4's `TouchEnglishDubChecked` exists to prevent.

- [ ] **Step 4: Verify the filter end to end**

```bash
curl -s 'http://localhost:8000/api/anime?providers=endub&page=1&page_size=5' | head -c 400
docker exec animeenigma-postgres psql -U postgres -d animeenigma -tAc \
  "SELECT count(*) FROM animes WHERE has_english_dub;"
```
Expected: the API returns only titles with `has_english_dub = true`, and the count grows over successive checks as the backfiller works through the queue.

- [ ] **Step 5: Confirm the metrics are live**

```bash
curl -s http://localhost:8081/metrics | grep catalog_english_dub
```
Expected: `catalog_english_dub_backfill_total`, `catalog_english_dub_promoted_total`, and `catalog_english_dub_unchecked` present. The gauge should trend down over time.

---

## Metrics

UXΔ = +2 (Better) · CDI = 0.05 * 13 · MVQ = Griffin 80%/85%
