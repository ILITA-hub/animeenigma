# aePlayer Notification Coverage — Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** aePlayer watchers on the `english` (sub), `ae`, and `raw` source families receive new-episode notifications, by resolving "latest available episode" at the anime level (no `translation_id`) and admitting those combos into the detector.

**Architecture:** A new self-contained `anime_level_episodes.go` unit (narrow interfaces → unit-testable) resolves latest-episode for empty-`translation_id` combos: `english` via the scraper episode list (sub = merged max), `ae`/`raw` via `RawResolver`. `EpisodesLookupService.LatestAvailable` delegates to it when the player is anime-level; the legacy translation-specific path is untouched. The catalog internal handler stops requiring `translation_id` for these players, and the notifications `hotcombos` filter admits them.

**Tech Stack:** Go (catalog, notifications services), `go test`, in-memory SQLite for the notifications DB test.

**Spec:** `docs/superpowers/specs/2026-06-18-aeplayer-notification-coverage-design.md`

## Global Constraints

- **Work in a clean `origin/main` git worktree**, NOT the shared `/data/animeenigma` tree (it is stale — pre-`unifiedPlayer→aePlayer` rename — and dirty with other agents' work). The controller provides the worktree path. Commit there; the controller pushes.
- **Commits:** path-scoped (`git commit <pathspec>`), never `git add -A` / bare commit. `git show --stat HEAD` after each. Do NOT push (controller pushes).
- **Co-authors on every commit:**
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Anime-level players this phase:** `english`, `ae`, `raw`. `english` + `watch_type == "dub"` is deliberately NOT resolved yet → returns a `NotFound`-class error (silent detector skip); Phase 2 implements dub. `kodik`/`animelib`/`hanime` are NOT anime-level in this phase.
- **Error-code contract:** "no episodes / not on this source" → an error whose message contains one of `no episode`, `not found`, `no episodes returned` (so the existing `isNotFoundLike` maps it to `CodeNotFound` → detector skips silently). Infra failures → any other message → `CodeInternal`/`CodeUnavailable` → detector logs a parser-failure + retries.
- **Snapshot/dedup:** no schema change — empty `translation_id` collapses teams into one snapshot row per `(anime, player, language, watch_type)`.

---

### Task 1: Catalog — anime-level latest-episode resolver + integration

**Files:**
- Create: `services/catalog/internal/service/anime_level_episodes.go`
- Create: `services/catalog/internal/service/anime_level_episodes_test.go`
- Modify: `services/catalog/internal/service/episodes_lookup.go` (constructor 59-75; `LatestAvailable` 90-152)
- Modify: `services/catalog/cmd/catalog-api/main.go` (DI at 291-298)

**Interfaces:**
- Produces: `isAnimeLevelPlayer(player string) bool`; `animeLevelResolver.Latest(ctx, shikimoriID, player, watchType string) (latest int, translationTitle string, err error)`.
- Consumes: `*repo.AnimeRepository.GetByShikimoriID`, `*CatalogService.GetScraperEpisodes`, `*RawResolver.GetEpisodes`/`GetLibraryEpisodes`, `EpisodesResponse`/`RawEpisode` (same `service` package).

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/anime_level_episodes_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

type fakeFinder struct {
	anime *domain.Anime
	err   error
}

func (f fakeFinder) GetByShikimoriID(_ context.Context, _ string) (*domain.Anime, error) {
	return f.anime, f.err
}

type fakeScraper struct {
	status int
	body   []byte
	err    error
}

func (f fakeScraper) GetScraperEpisodes(_ context.Context, _, _ string) (int, []byte, error) {
	return f.status, f.body, f.err
}

type fakeRaw struct {
	lib  *EpisodesResponse
	raw  *EpisodesResponse
	err  error
}

func (f fakeRaw) GetLibraryEpisodes(_ context.Context, _ string) (*EpisodesResponse, error) {
	return f.lib, f.err
}
func (f fakeRaw) GetEpisodes(_ context.Context, _ string) (*EpisodesResponse, error) {
	return f.raw, f.err
}

func newResolver(fnd animeFinder, scr scraperEpisodeLister, raw rawEpisodeLister) *animeLevelResolver {
	return &animeLevelResolver{finder: fnd, scraper: scr, raw: raw}
}

func TestIsAnimeLevelPlayer(t *testing.T) {
	for _, p := range []string{"english", "ae", "raw"} {
		if !isAnimeLevelPlayer(p) {
			t.Errorf("isAnimeLevelPlayer(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"kodik", "animelib", "hanime", ""} {
		if isAnimeLevelPlayer(p) {
			t.Errorf("isAnimeLevelPlayer(%q) = true, want false", p)
		}
	}
}

func TestAnimeLevel_EnglishSub_MaxEpisode(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[{"number":1},{"number":12},{"number":7}]}}`)},
		fakeRaw{},
	)
	latest, _, err := r.Latest(context.Background(), "57466", "english", "sub")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if latest != 12 {
		t.Errorf("latest = %d, want 12", latest)
	}
}

func TestAnimeLevel_EnglishDub_NotSupportedYet(t *testing.T) {
	r := newResolver(fakeFinder{anime: &domain.Anime{ID: "uuid-1"}}, fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[{"number":1}]}}`)}, fakeRaw{})
	_, _, err := r.Latest(context.Background(), "57466", "english", "dub")
	if err == nil || !isNotFoundLike(err) {
		t.Fatalf("english dub err = %v, want a NotFound-like error", err)
	}
}

func TestAnimeLevel_EnglishSub_EmptyIsNotFound(t *testing.T) {
	r := newResolver(fakeFinder{anime: &domain.Anime{ID: "uuid-1"}}, fakeScraper{status: 200, body: []byte(`{"data":{"episodes":[]}}`)}, fakeRaw{})
	_, _, err := r.Latest(context.Background(), "57466", "english", "sub")
	if err == nil || !isNotFoundLike(err) {
		t.Fatalf("empty english err = %v, want NotFound-like", err)
	}
}

func TestAnimeLevel_AE_MaxFromLibrary(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{},
		fakeRaw{lib: &EpisodesResponse{Available: true, Episodes: []RawEpisode{{Number: 3}, {Number: 9}, {Number: 5}}}},
	)
	latest, _, err := r.Latest(context.Background(), "57466", "ae", "sub")
	if err != nil || latest != 9 {
		t.Fatalf("ae latest = %d, err = %v, want 9, nil", latest, err)
	}
}

func TestAnimeLevel_Raw_MaxFromAllAnime(t *testing.T) {
	r := newResolver(
		fakeFinder{anime: &domain.Anime{ID: "uuid-1"}},
		fakeScraper{},
		fakeRaw{raw: &EpisodesResponse{Available: true, Episodes: []RawEpisode{{Number: 24}}}},
	)
	latest, _, err := r.Latest(context.Background(), "57466", "raw", "sub")
	if err != nil || latest != 24 {
		t.Fatalf("raw latest = %d, err = %v, want 24, nil", latest, err)
	}
}

func TestAnimeLevel_Raw_UnavailableIsNotFound(t *testing.T) {
	r := newResolver(fakeFinder{anime: &domain.Anime{ID: "uuid-1"}}, fakeScraper{}, fakeRaw{raw: &EpisodesResponse{Available: false}})
	_, _, err := r.Latest(context.Background(), "57466", "raw", "sub")
	if err == nil || !isNotFoundLike(err) {
		t.Fatalf("unavailable raw err = %v, want NotFound-like", err)
	}
}

func TestAnimeLevel_ScraperError_Propagates(t *testing.T) {
	r := newResolver(fakeFinder{anime: &domain.Anime{ID: "uuid-1"}}, fakeScraper{err: errors.New("connection refused")}, fakeRaw{})
	_, _, err := r.Latest(context.Background(), "57466", "english", "sub")
	if err == nil || isNotFoundLike(err) {
		t.Fatalf("scraper-down err = %v, want a non-NotFound (infra) error", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run from the worktree: `cd services/catalog && go test ./internal/service/ -run 'TestAnimeLevel|TestIsAnimeLevelPlayer' 2>&1 | tail -15`
Expected: FAIL — `anime_level_episodes.go` / `animeLevelResolver` undefined.

- [ ] **Step 3: Create `anime_level_episodes.go`**

```go
package service

import (
	"context"
	"encoding/json"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// Anime-level (team-agnostic) players: aePlayer persists translation_id='' for
// these, so "latest available episode" is resolved at the anime level — no
// translation/team id. See spec 2026-06-18-aeplayer-notification-coverage.
func isAnimeLevelPlayer(player string) bool {
	switch player {
	case "english", "ae", "raw":
		return true
	default:
		return false
	}
}

// Narrow dependency seams so the resolver is unit-testable without a DB or
// real HTTP. *repo.AnimeRepository, *CatalogService and *RawResolver satisfy
// these respectively.
type animeFinder interface {
	GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error)
}
type scraperEpisodeLister interface {
	GetScraperEpisodes(ctx context.Context, animeID, prefer string) (int, []byte, error)
}
type rawEpisodeLister interface {
	GetEpisodes(ctx context.Context, animeID string) (*EpisodesResponse, error)
	GetLibraryEpisodes(ctx context.Context, animeID string) (*EpisodesResponse, error)
}

// animeLevelResolver answers "latest available episode" for an empty-
// translation_id combo, dispatched by player family.
type animeLevelResolver struct {
	finder  animeFinder
	scraper scraperEpisodeLister
	raw     rawEpisodeLister
}

// Latest returns (latest episode number, translation title "", error). NotFound-
// like errors (message contains "no episode"/"not found") tell the caller to
// skip the combo silently; other errors are infra failures.
func (r *animeLevelResolver) Latest(ctx context.Context, shikimoriID, player, watchType string) (int, string, error) {
	anime, err := r.finder.GetByShikimoriID(ctx, shikimoriID)
	if err != nil {
		return 0, "", err
	}
	if anime == nil {
		return 0, "", apperrors.NotFound("anime not found")
	}

	switch player {
	case "english":
		if watchType == "dub" {
			// Phase 2 resolves dub via the scraper has_dub flags. Until then,
			// report "no episode" so the detector skips dub combos silently
			// rather than claiming the sub count for dub.
			return 0, "", apperrors.NotFound("no english dub episode lookup yet")
		}
		return r.latestEnglishSub(ctx, anime.ID)
	case "ae":
		return maxRawEpisode(r.raw.GetLibraryEpisodes(ctx, anime.ID))
	case "raw":
		return maxRawEpisode(r.raw.GetEpisodes(ctx, anime.ID))
	default:
		return 0, "", apperrors.InvalidInput("player is not anime-level")
	}
}

// latestEnglishSub takes the max episode number from the scraper's merged
// episode list (sub-complete; sub is the reliable floor for an ongoing title).
func (r *animeLevelResolver) latestEnglishSub(ctx context.Context, animeID string) (int, string, error) {
	status, body, err := r.scraper.GetScraperEpisodes(ctx, animeID, "")
	if err != nil {
		return 0, "", apperrors.Wrap(err, apperrors.CodeUnavailable, "scraper episodes lookup failed")
	}
	if status != 200 {
		return 0, "", apperrors.NotFound("no english episodes for anime")
	}
	var resp struct {
		Data struct {
			Episodes []struct {
				Number int `json:"number"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, "", apperrors.Wrap(err, apperrors.CodeInternal, "decode scraper episodes")
	}
	max := maxEpisodeNum(len(resp.Data.Episodes), func(i int) int { return resp.Data.Episodes[i].Number })
	if max == 0 {
		return 0, "", apperrors.NotFound("no english episodes returned")
	}
	return max, "", nil
}

// maxRawEpisode adapts a RawResolver call result. Empty / unavailable → NotFound.
func maxRawEpisode(resp *EpisodesResponse, err error) (int, string, error) {
	if err != nil {
		return 0, "", err
	}
	if resp == nil || !resp.Available || len(resp.Episodes) == 0 {
		return 0, "", apperrors.NotFound("no episodes available for anime")
	}
	max := maxEpisodeNum(len(resp.Episodes), func(i int) int { return resp.Episodes[i].Number })
	if max == 0 {
		return 0, "", apperrors.NotFound("no numbered episodes returned")
	}
	return max, "", nil
}

// maxEpisodeNum returns the largest episode number over n items via getter.
func maxEpisodeNum(n int, get func(i int) int) int {
	max := 0
	for i := 0; i < n; i++ {
		if v := get(i); v > max {
			max = v
		}
	}
	return max
}
```

- [ ] **Step 4: Run the resolver tests — verify they pass**

Run: `cd services/catalog && go test ./internal/service/ -run 'TestAnimeLevel|TestIsAnimeLevelPlayer' -v 2>&1 | tail -25`
Expected: PASS (8 tests).

- [ ] **Step 5: Wire the resolver into `EpisodesLookupService`**

In `services/catalog/internal/service/episodes_lookup.go`:

(a) Add the field to the struct (next to `animeRepo`):
```go
	animeLevel *animeLevelResolver
```

(b) Add a `rawResolver *RawResolver` parameter to `NewEpisodesLookupService` (after `catalogService`) and build the resolver. The constructor becomes:
```go
func NewEpisodesLookupService(
	c *cache.RedisCache,
	kodikClient *kodik.Client,
	animelibClient *animelib.Client,
	animeRepo *repo.AnimeRepository,
	catalogService *CatalogService,
	rawResolver *RawResolver,
	log *logger.Logger,
) *EpisodesLookupService {
	return &EpisodesLookupService{
		cache:          c,
		kodikClient:    kodikClient,
		animelibClient: animelibClient,
		animeRepo:      animeRepo,
		catalogService: catalogService,
		animeLevel:     &animeLevelResolver{finder: animeRepo, scraper: catalogService, raw: rawResolver},
		log:            log,
	}
}
```

(c) In `LatestAvailable`, replace the early `translation_id required` guard (currently lines ~97-99) and the `switch player` so anime-level players bypass the id requirement and route to the resolver. The head becomes:
```go
	if shikimoriID == "" {
		return EpisodesLookupResult{}, apperrors.InvalidInput("shikimori_id required")
	}
	animeLevel := isAnimeLevelPlayer(player)
	if !animeLevel && translationID == "" {
		return EpisodesLookupResult{}, apperrors.InvalidInput("translation_id required")
	}
```
and add a leading case to the existing `switch player` (keep `kodik`/`animelib`/`default` exactly as they are):
```go
	switch {
	case animeLevel:
		latest, translationTitle, err = s.animeLevel.Latest(ctx, shikimoriID, player, watchType)
	case player == "kodik":
		// ... unchanged kodik body ...
	case player == "animelib":
		// ... unchanged animelib body ...
	default:
		return EpisodesLookupResult{}, apperrors.InvalidInput("player not supported by detector in v1.0")
	}
```
(Convert the `switch player {` to `switch {` with `case player == "kodik":` etc. — the case bodies are unchanged. The shared cache-get/set and `isNotFoundLike` error handling below the switch stay exactly as-is.)

- [ ] **Step 6: Wire DI in `main.go`**

In `services/catalog/cmd/catalog-api/main.go`, the `NewEpisodesLookupService(...)` call (line ~291) gains `rawResolver` (constructed at line 275) before `log`:
```go
	episodesLookupService := service.NewEpisodesLookupService(
		redisCache,
		catalogService.KodikClient(),
		catalogService.AnimeLibClient(),
		animeRepo,
		catalogService,
		rawResolver,
		log,
	)
```

- [ ] **Step 7: Build + full service test (no regressions)**

Run: `cd services/catalog && go build ./... && go test ./internal/service/ 2>&1 | tail -15`
Expected: build OK; tests PASS (resolver tests + any existing service tests).

- [ ] **Step 8: Commit**

```bash
git commit services/catalog/internal/service/anime_level_episodes.go services/catalog/internal/service/anime_level_episodes_test.go services/catalog/internal/service/episodes_lookup.go services/catalog/cmd/catalog-api/main.go \
  -m "feat(catalog): anime-level latest-episode resolver (english sub/ae/raw)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 2: Catalog — internal handler allows anime-level players without `translation_id`

**Files:**
- Modify: `services/catalog/internal/handler/internal_episodes.go` (validation at 75-83)
- Test: `services/catalog/internal/handler/internal_episodes_test.go` (create if absent, else extend)

**Interfaces:**
- Consumes: `isAnimeLevelPlayer` (Task 1, same module — handler is a different package; duplicate the tiny allow-set locally to avoid a service→handler import, OR call through. Use a local set in the handler — see Step 2).

- [ ] **Step 1: Write the failing test**

Create/extend `services/catalog/internal/handler/internal_episodes_test.go`. The handler calls `h.svc.LatestAvailable(...)`; use a fake svc that records args and returns a canned result. Assert:
- `?player=english&watch_type=sub` (NO `translation_id`) → 200 (not 400).
- `?player=ae` (no id) → 200.
- `?player=kodik` (no id) → 400 (`translation_id` still required).
- `?player=hanime` → 400 (`player not supported`).

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

type fakeLookup struct{}

func (fakeLookup) LatestAvailable(_ context.Context, _, _, _, _, _ string) (service.EpisodesLookupResult, error) {
	return service.EpisodesLookupResult{LatestAvailableEpisode: 12}, nil
}

func doReq(t *testing.T, h *InternalEpisodesHandler, target string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("shikimoriId", "57466")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.GetLatestEpisode(rec, req)
	return rec.Code
}

func TestInternalEpisodes_AnimeLevelPlayersNoTranslationID(t *testing.T) {
	h := NewInternalEpisodesHandler(fakeLookup{}, nil)
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=english&watch_type=sub&language=en"); c != 200 {
		t.Errorf("english sub no-id = %d, want 200", c)
	}
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=ae"); c != 200 {
		t.Errorf("ae no-id = %d, want 200", c)
	}
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=kodik"); c != 400 {
		t.Errorf("kodik no-id = %d, want 400", c)
	}
	if c := doReq(t, h, "/internal/anime/57466/episodes?player=hanime&translation_id=x"); c != 400 {
		t.Errorf("hanime = %d, want 400", c)
	}
}
```

> If `InternalEpisodesHandler.svc` is a concrete `*service.EpisodesLookupService` (not an interface), first introduce a one-method interface `episodesLookup { LatestAvailable(ctx, shikimoriID, player, translationID, watchType, language string) (service.EpisodesLookupResult, error) }` as the handler's `svc` field type (the concrete service satisfies it) so the fake compiles. Verify the field type at `internal_episodes.go:44-51` and adjust the struct/constructor signature accordingly (keep `NewInternalEpisodesHandler`'s call site in main.go working — the concrete service still satisfies the interface).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/handler/ -run TestInternalEpisodes_AnimeLevel 2>&1 | tail -15`
Expected: FAIL — english/ae currently 400 (`player not supported by detector in v1.0`).

- [ ] **Step 3: Update the handler validation (lines 75-83)**

Replace:
```go
	if player != "kodik" && player != "animelib" {
		// D-DET-03: any other player value returns 400 in v1.0.
		httputil.BadRequest(w, "player not supported by detector in v1.0")
		return
	}
	if translationID == "" {
		httputil.BadRequest(w, "translation_id is required")
		return
	}
```
with:
```go
	// Anime-level players (aePlayer, empty translation_id): english/ae/raw.
	// Legacy translation-specific players: kodik/animelib (require an id).
	animeLevel := player == "english" || player == "ae" || player == "raw"
	legacy := player == "kodik" || player == "animelib"
	if !animeLevel && !legacy {
		httputil.BadRequest(w, "player not supported by detector")
		return
	}
	if legacy && translationID == "" {
		httputil.BadRequest(w, "translation_id is required")
		return
	}
```

- [ ] **Step 4: Run test — verify it passes**

Run: `cd services/catalog && go test ./internal/handler/ -run TestInternalEpisodes_AnimeLevel -v 2>&1 | tail -15`
Expected: PASS.

- [ ] **Step 5: Build + handler package tests**

Run: `cd services/catalog && go build ./... && go test ./internal/handler/ 2>&1 | tail -10`
Expected: OK / PASS.

- [ ] **Step 6: Commit**

```bash
git commit services/catalog/internal/handler/internal_episodes.go services/catalog/internal/handler/internal_episodes_test.go \
  -m "feat(catalog): internal episodes handler accepts anime-level players (english/ae/raw)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 3: Notifications — `hotcombos` filter admits anime-level players

**Files:**
- Modify: `services/notifications/internal/job/hotcombos.go` (query WHERE at 57-59)
- Test: `services/notifications/internal/job/hotcombos_test.go` (create)

**Interfaces:**
- Consumes: `testDB(t)` and `seedWatch(...)` from `detector_test.go` (same package), plus the `anime_list`/`animes` seeding those tests use.

- [ ] **Step 1: Write the failing test**

Create `services/notifications/internal/job/hotcombos_test.go`. Reuse `testDB(t)` (creates `watch_history`, `anime_list`, `animes`) and `seedWatch`. Seed `anime_list` (status='watching') and `animes` (status='ongoing') rows for the anime ids, then assert `Collect` returns the english/ae/raw empty-`translation_id` combos AND still excludes a `hanime` empty-`translation_id` combo and still includes a legacy `kodik` row with a non-empty id.

```go
package job

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

func TestHotCombos_AdmitsAnimeLevelPlayers(t *testing.T) {
	db := testDB(t)
	// anime rows (ongoing) + list rows (watching)
	mustExec(t, db, `INSERT INTO animes (id, shikimori_id, status) VALUES ('a-en','111','ongoing'),('a-ae','222','ongoing'),('a-raw','333','ongoing'),('a-h','444','ongoing'),('a-k','555','ongoing')`)
	mustExec(t, db, `INSERT INTO anime_list (user_id, anime_id, status) VALUES ('u1','a-en','watching'),('u1','a-ae','watching'),('u1','a-raw','watching'),('u1','a-h','watching'),('u1','a-k','watching')`)

	seedWatch(t, db, "u1", "a-en", "english", "en", "sub", "", 5)   // empty id, anime-level
	seedWatch(t, db, "u1", "a-ae", "ae", "ja", "sub", "", 3)        // empty id, anime-level
	seedWatch(t, db, "u1", "a-raw", "raw", "ja", "sub", "", 2)      // empty id, anime-level
	seedWatch(t, db, "u1", "a-h", "hanime", "ru", "dub", "", 1)     // empty id, NOT admitted
	seedWatch(t, db, "u1", "a-k", "kodik", "ru", "sub", "1291", 7)  // legacy, admitted

	combos, err := NewHotCombosCollector(db, logger.Default()).Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	got := map[string]bool{}
	for _, c := range combos {
		got[c.Player] = true
	}
	for _, p := range []string{"english", "ae", "raw", "kodik"} {
		if !got[p] {
			t.Errorf("expected player %q in hot combos, missing", p)
		}
	}
	if got["hanime"] {
		t.Errorf("hanime (empty translation_id) must NOT be admitted")
	}
}
```

> Implementation note for the implementer: match `testDB`'s actual `*gorm.DB` type and add a small `mustExec(t, db, sql)` helper if one is not already present in the package's test files (check `detector_test.go` / `invalidation_test.go` first and reuse). Drop the empty `seedListAndAnime` stub if you inline the INSERTs as shown. Confirm the `animes`/`anime_list` columns created by `testDB` (`status`, `shikimori_id`, `id`) — adjust the INSERT column lists to match the actual DDL.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/notifications && go test ./internal/job/ -run TestHotCombos_AdmitsAnimeLevel 2>&1 | tail -15`
Expected: FAIL — english/ae/raw dropped by `translation_id != ''`.

- [ ] **Step 3: Update the filter**

In `services/notifications/internal/job/hotcombos.go`, change the WHERE (and update the doc comment at 28-31 which says English rows "naturally drop out"):
```go
		WHERE al.status = 'watching'
		  AND a.status = 'ongoing'
		  AND (wh.translation_id != '' OR wh.player IN ('english', 'ae', 'raw'))
```
Update the comment block above the query to: legacy translation-specific players carry a non-empty `translation_id`; anime-level aePlayer players (`english`/`ae`/`raw`) are admitted with an empty `translation_id` and resolved at the anime level. `hanime`/`kodik`/`animelib` with an empty id stay excluded (no anime-level resolver yet).

- [ ] **Step 4: Run test — verify it passes**

Run: `cd services/notifications && go test ./internal/job/ -run TestHotCombos_AdmitsAnimeLevel -v 2>&1 | tail -15`
Expected: PASS.

- [ ] **Step 5: Full notifications job tests (no regressions)**

Run: `cd services/notifications && go test ./internal/job/ 2>&1 | tail -10`
Expected: PASS (existing detector/invalidation tests unaffected).

- [ ] **Step 6: Commit**

```bash
git commit services/notifications/internal/job/hotcombos.go services/notifications/internal/job/hotcombos_test.go \
  -m "feat(notifications): admit anime-level aePlayer combos (english/ae/raw) into detection

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD
```

---

### Task 4: Verify + deploy + changelog

**Files:** `frontend/web/changelog.full.json` + `frontend/web/public/changelog.json` (changelog only).

- [ ] **Step 1: Full suites for both services**

Run:
```bash
cd services/catalog && go test ./... 2>&1 | tail -15
cd ../notifications && go test ./... 2>&1 | tail -15
```
Expected: no FAIL.

- [ ] **Step 2: Add the changelog entry (Russian Trump-mode)**

Read `frontend/web/changelog.full.json`; merge into the top `2026-06-18` group a `feature` entry, e.g.:
`"🔔 УВЕДОМЛЕНИЯ ОЖИЛИ ДЛЯ ПЛЕЕРА ANIMEENIGMA! Раньше, если ты смотрел в нашем плеере (английская озвучка, первоисточник AnimeEnigma, RAW-японский) — о новой серии ты НЕ УЗНАВАЛ. КАТАСТРОФА. МЫ это нашли — никто другой не заметил. Теперь новая серия выходит — тебе ЛЕТИТ уведомление. ЖОСКО починили. Поверьте мне."`
Then regenerate the served file: `cd frontend/web && node scripts/changelog-trim.mjs`. Commit BOTH files (path-scoped, co-authors).

- [ ] **Step 3: Deploy (controller does this from the clean worktree)**

Redeploy `catalog` and `notifications` (Go services changed); `web` (changelog). `make redeploy-catalog`, `make redeploy-notifications`, `make redeploy-web` from the worktree (copy `docker/.env` first). Then `make health` (expect all services ✓).

- [ ] **Step 4: Runtime verification**

```bash
# anime-level lookup now accepted (no translation_id) — expect 200/JSON or a clean 404, NOT 400 "player not supported"
curl -s -o /dev/null -w "%{http_code}\n" "http://localhost:8081/internal/anime/57466/episodes?player=english&watch_type=sub&language=en"
```
Expected: `200` (or `404` if that title has no EN episodes) — NOT `400`.

## Notes / scope

- **`english` dub** is intentionally deferred to Phase 2 (returns NotFound now → silent skip). **`kodik`/`animelib` via aePlayer** (empty id) and **`hanime`** remain uncovered this phase by design.
- Deep-link behavior is already shipped: `provider=english` → aePlayer auto-picks best EN source; `provider=ae`/`raw` → pinned. `team` empty (anime-level).
- No DB migration (empty `translation_id` collapses into one snapshot row per family).
