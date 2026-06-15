# Anidle Plan 1 — Catalog Foundation (franchise + guess-pool endpoint)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the catalog service a `franchise` field on the `Anime` model and an internal `GET /internal/guessgame/pool` endpoint that returns the franchise-collapsed, `score > 8` anime pool (with the 8 guessable attributes) for the `anidle` game service to consume.

**Architecture:** Add `Franchise` to the `Anime` GORM model (auto-migrated on startup). Source franchise from the Shikimori **REST** endpoint `/api/animes/{id}` (GraphQL does not expose it). A new `GuessPoolService` lists `score > 8` candidates, lazily backfills missing `franchise` values via REST (persisted once to the DB row, so it is a one-time cost), collapses each franchise to its earliest-aired entry, and maps to a JSON DTO. A Docker-network-only internal handler exposes it; the gateway does NOT proxy `/internal/*`.

**Tech Stack:** Go, chi router, GORM (PostgreSQL), `libs/httputil` response helpers, handwritten test fakes (no testify).

**Reference spec:** `docs/superpowers/specs/2026-06-15-anidle-anime-guessing-game-design.md` §2.1, §7.

**Caching note (YAGNI):** Catalog does NOT cache the pool. `anidle` owns the cache (`anidle:pool` Redis snapshot, per spec §4.2). Because franchise backfill persists to the DB row, repeated pool builds make zero REST calls after the first.

**Franchise-collapse simplification (v1):** Representative = earliest `aired_on` **among the `score > 8` pool members sharing a franchise**, not the franchise's true season-1 if that season scored ≤ 8. This avoids fetching the full franchise graph. Rare edge case (a franchise whose only >8 entry is a later season) yields a still-recognizable representative. Documented here as the intended v1 behavior.

---

## File Structure

- **Modify** `services/catalog/internal/domain/anime.go` — add `Franchise` field.
- **Create** `services/catalog/internal/domain/anime_franchise_test.go` — field serialization test.
- **Modify** `services/catalog/internal/parser/shikimori/client.go` — add `GetAnimeFranchise`.
- **Create** `services/catalog/internal/parser/shikimori/franchise_test.go` — REST fetch test.
- **Modify** `services/catalog/internal/repo/anime.go` — add `ListGuessPoolCandidates` + `SetFranchise`.
- **Create** `services/catalog/internal/service/guesspool.go` — DTO, collapse fn, `GuessPoolService`.
- **Create** `services/catalog/internal/service/guesspool_test.go` — collapse + BuildPool tests (fakes).
- **Create** `services/catalog/internal/handler/internal_guesspool.go` — internal handler.
- **Create** `services/catalog/internal/handler/internal_guesspool_test.go` — handler test (fake builder).
- **Modify** `services/catalog/internal/transport/router.go` — register route + constructor param.
- **Modify** `services/catalog/cmd/catalog-api/main.go` — DI wiring.

---

## Task 1: Add `Franchise` field to the Anime model

**Files:**
- Modify: `services/catalog/internal/domain/anime.go` (after the `MaterialSource` field, ~line 25)
- Test: `services/catalog/internal/domain/anime_franchise_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/domain/anime_franchise_test.go`:

```go
package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnime_FranchiseSerialization(t *testing.T) {
	a := Anime{ID: "uuid-1", Franchise: "frieren"}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"franchise":"frieren"`) {
		t.Fatalf("expected franchise in JSON, got %s", string(b))
	}
}

func TestAnime_FranchiseOmitEmpty(t *testing.T) {
	a := Anime{ID: "uuid-1"}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "franchise") {
		t.Fatalf("expected franchise omitted when empty, got %s", string(b))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/domain/ -run TestAnime_Franchise -v`
Expected: FAIL — `a.Franchise` undefined (compile error).

- [ ] **Step 3: Add the field**

In `services/catalog/internal/domain/anime.go`, add this line immediately after the `MaterialSource` field:

```go
	Franchise       string         `gorm:"size:200;index" json:"franchise,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/domain/ -run TestAnime_Franchise -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/domain/anime.go services/catalog/internal/domain/anime_franchise_test.go
git commit -m "feat(catalog): add Franchise field to Anime model"
```

---

## Task 2: Shikimori REST `GetAnimeFranchise`

**Files:**
- Modify: `services/catalog/internal/parser/shikimori/client.go` (add near `GetRelatedAnime`, ~line 712)
- Test: `services/catalog/internal/parser/shikimori/franchise_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/parser/shikimori/franchise_test.go`:

```go
package shikimori

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
)

func newTestClient(srvURL string) *Client {
	return NewClient(config.ShikimoriConfig{
		BaseURL:    srvURL,
		GraphQLURL: srvURL + "/api/graphql",
		UserAgent:  "test-agent",
		Timeout:    5 * time.Second,
		RateLimit:  100,
	}, nil)
}

func TestGetAnimeFranchise_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/animes/52991" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":52991,"russian":"Фрирен","franchise":"frieren"}`))
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).GetAnimeFranchise(context.Background(), "52991")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "frieren" {
		t.Fatalf("want frieren, got %q", got)
	}
}

func TestGetAnimeFranchise_EmptyWhenMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1,"russian":"X","franchise":null}`))
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).GetAnimeFranchise(context.Background(), "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestGetAnimeFranchise_404IsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).GetAnimeFranchise(context.Background(), "999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("want empty on 404, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/shikimori/ -run TestGetAnimeFranchise -v`
Expected: FAIL — `GetAnimeFranchise` undefined.

- [ ] **Step 3: Add the method**

In `services/catalog/internal/parser/shikimori/client.go`, add after `GetRelatedAnime` (after ~line 712). It mirrors `GetRelatedAnime`'s exact request shape (rate limiter, User-Agent, decode):

```go
// GetAnimeFranchise fetches the franchise slug for an anime from the Shikimori
// REST API (GET /api/animes/{id}). Shikimori's GraphQL schema does NOT expose
// `franchise`, so this single-anime REST call is the only source. Returns an
// empty string when the anime has no franchise or does not exist (404).
func (c *Client) GetAnimeFranchise(ctx context.Context, shikimoriID string) (string, error) {
	c.rateLimiter.acquire()

	url := fmt.Sprintf("%s/api/animes/%s", c.config.BaseURL, shikimoriID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", errors.ExternalAPI("shikimori", err)
	}
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errors.ExternalAPI("shikimori", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.ExternalAPI("shikimori", fmt.Errorf("anime detail returned status %d", resp.StatusCode))
	}

	var detail struct {
		Franchise string `json:"franchise"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return "", errors.ExternalAPI("shikimori", fmt.Errorf("decode anime detail: %w", err))
	}
	return detail.Franchise, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/parser/shikimori/ -run TestGetAnimeFranchise -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/parser/shikimori/client.go services/catalog/internal/parser/shikimori/franchise_test.go
git commit -m "feat(catalog): Shikimori REST GetAnimeFranchise"
```

---

## Task 3: Repo — list pool candidates + set franchise

**Files:**
- Modify: `services/catalog/internal/repo/anime.go`

These are thin GORM queries. House style mocks repos with fakes at the service layer (Task 4), so they are not unit-tested in isolation; they are exercised by the service/integration path. Add the code only.

- [ ] **Step 1: Add the two methods**

Append to `services/catalog/internal/repo/anime.go`:

```go
// ListGuessPoolCandidates returns non-hidden anime with score strictly greater
// than minScore, ordered earliest-aired first (NULLs last) so a franchise's
// first member is encountered first during collapse. Genres/Studios/Tags are
// preloaded for attribute comparison.
func (r *AnimeRepository) ListGuessPoolCandidates(ctx context.Context, minScore float64) ([]*domain.Anime, error) {
	var animes []*domain.Anime
	err := r.db.WithContext(ctx).
		Where("score > ? AND (hidden = ? OR hidden IS NULL)", minScore, false).
		Preload("Genres").
		Preload("Studios").
		Preload("Tags").
		Order("aired_on ASC NULLS LAST").
		Find(&animes).Error
	if err != nil {
		return nil, fmt.Errorf("list guess pool candidates: %w", err)
	}
	return animes, nil
}

// SetFranchise persists a backfilled franchise slug onto an anime row.
func (r *AnimeRepository) SetFranchise(ctx context.Context, id, franchise string) error {
	return r.db.WithContext(ctx).
		Model(&domain.Anime{}).
		Where("id = ?", id).
		Update("franchise", franchise).Error
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd services/catalog && go build ./internal/repo/`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add services/catalog/internal/repo/anime.go
git commit -m "feat(catalog): repo ListGuessPoolCandidates + SetFranchise"
```

---

## Task 4a: Franchise-collapse pure function

**Files:**
- Create: `services/catalog/internal/service/guesspool.go`
- Test: `services/catalog/internal/service/guesspool_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/service/guesspool_test.go`:

```go
package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestCollapseByFranchise(t *testing.T) {
	in := []*domain.Anime{
		{ID: "jjk1", Franchise: "jujutsu_kaisen"}, // earliest first (repo orders aired_on ASC)
		{ID: "jjk2", Franchise: "jujutsu_kaisen"},
		{ID: "standalone-a", Franchise: ""},
		{ID: "standalone-b", Franchise: ""},
		{ID: "frieren", Franchise: "frieren"},
	}
	out := collapseByFranchise(in)

	var ids []string
	for _, a := range out {
		ids = append(ids, a.ID)
	}
	want := []string{"jjk1", "standalone-a", "standalone-b", "frieren"}
	if len(ids) != len(want) {
		t.Fatalf("want %v, got %v", want, ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("at %d want %s, got %s (full %v)", i, want[i], ids[i], ids)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run TestCollapseByFranchise -v`
Expected: FAIL — `collapseByFranchise` undefined.

- [ ] **Step 3: Create the file with the function**

Create `services/catalog/internal/service/guesspool.go`:

```go
package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// collapseByFranchise keeps one entry per non-empty franchise (the first one
// seen — callers pass earliest-aired first), and keeps every standalone
// (empty-franchise) anime individually.
func collapseByFranchise(animes []*domain.Anime) []*domain.Anime {
	seen := make(map[string]bool)
	out := make([]*domain.Anime, 0, len(animes))
	for _, a := range animes {
		if a.Franchise == "" {
			out = append(out, a)
			continue
		}
		if seen[a.Franchise] {
			continue
		}
		seen[a.Franchise] = true
		out = append(out, a)
	}
	return out
}

// --- placeholders filled in Task 4b (DTO, service) live below ---
var _ = context.Background
var _ logger.Logger
```

(The two `var _` lines keep the file compiling until Task 4b adds real uses; Task 4b deletes them.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/ -run TestCollapseByFranchise -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/guesspool.go services/catalog/internal/service/guesspool_test.go
git commit -m "feat(catalog): franchise-collapse function for guess pool"
```

---

## Task 4b: GuessPoolService (DTO, backfill, mapping)

**Files:**
- Modify: `services/catalog/internal/service/guesspool.go`
- Modify: `services/catalog/internal/service/guesspool_test.go`

- [ ] **Step 1: Write the failing test**

Append to `services/catalog/internal/service/guesspool_test.go`:

```go
type fakePoolRepo struct {
	candidates []*domain.Anime
	setCalls   map[string]string // animeID -> franchise
}

func (f *fakePoolRepo) ListGuessPoolCandidates(_ context.Context, _ float64) ([]*domain.Anime, error) {
	return f.candidates, nil
}
func (f *fakePoolRepo) SetFranchise(_ context.Context, id, franchise string) error {
	if f.setCalls == nil {
		f.setCalls = map[string]string{}
	}
	f.setCalls[id] = franchise
	return nil
}

type fakeFranchiseFetcher struct {
	byShikimori map[string]string
	calls       int
}

func (f *fakeFranchiseFetcher) GetAnimeFranchise(_ context.Context, sid string) (string, error) {
	f.calls++
	return f.byShikimori[sid], nil
}

func TestBuildPool_BackfillsAndCollapses(t *testing.T) {
	repo := &fakePoolRepo{candidates: []*domain.Anime{
		// jjk1 already has franchise; jjk2 missing -> backfilled to same franchise -> collapsed away
		{ID: "jjk1", ShikimoriID: "40748", Franchise: "jujutsu_kaisen", NameRU: "Маг. битва",
			Year: 2020, EpisodesCount: 24, Score: 8.6, Status: domain.StatusReleased, Rating: "pg_13",
			Genres:  []domain.Genre{{ID: "1", NameRU: "Экшен"}},
			Studios: []domain.Studio{{ID: "s1", Name: "MAPPA"}},
			Tags:    []domain.Tag{{ID: "t1", Name: "Магия"}}},
		{ID: "jjk2", ShikimoriID: "51009", Franchise: "", NameRU: "Маг. битва 2"},
		{ID: "frieren", ShikimoriID: "52991", Franchise: "", NameRU: "Фрирен", Year: 2023,
			EpisodesCount: 28, Score: 9.3, Status: domain.StatusReleased},
	}}
	fetcher := &fakeFranchiseFetcher{byShikimori: map[string]string{
		"51009": "jujutsu_kaisen", // jjk2 belongs to same franchise
		"52991": "",               // frieren standalone
	}}
	svc := NewGuessPoolService(repo, fetcher, nil)

	entries, err := svc.BuildPool(context.Background())
	if err != nil {
		t.Fatalf("BuildPool: %v", err)
	}
	// jjk2 collapsed into jjk1; frieren stays -> 2 entries
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d (%+v)", len(entries), entries)
	}
	if entries[0].ID != "jjk1" || entries[1].ID != "frieren" {
		t.Fatalf("unexpected entry ids: %s, %s", entries[0].ID, entries[1].ID)
	}
	// jjk2 franchise was backfilled & persisted
	if repo.setCalls["jjk2"] != "jujutsu_kaisen" {
		t.Fatalf("expected jjk2 franchise persisted, got %v", repo.setCalls)
	}
	// attribute mapping check on jjk1
	e := entries[0]
	if e.Status != "released" || e.Rating != "pg_13" || e.Score != 8.6 {
		t.Fatalf("bad scalar mapping: %+v", e)
	}
	if len(e.Genres) != 1 || e.Genres[0].Name != "Экшен" {
		t.Fatalf("bad genre mapping: %+v", e.Genres)
	}
	if len(e.Studios) != 1 || e.Studios[0].Name != "MAPPA" {
		t.Fatalf("bad studio mapping: %+v", e.Studios)
	}
	if len(e.Tags) != 1 || e.Tags[0].Name != "Магия" {
		t.Fatalf("bad tag mapping: %+v", e.Tags)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/ -run TestBuildPool -v`
Expected: FAIL — `NewGuessPoolService` / `GuessPoolEntry` undefined.

- [ ] **Step 3: Replace the placeholder tail of `guesspool.go`**

In `services/catalog/internal/service/guesspool.go`, delete the two `var _` placeholder lines and the unused imports, then append the real implementation. The final file imports become:

```go
import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)
```

Add below `collapseByFranchise`:

```go
// PoolTaxon is an id+name pair for a genre/studio/tag (anidle compares by id,
// displays by name).
type PoolTaxon struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GuessPoolEntry is one guessable anime with the 8 comparison attributes.
type GuessPoolEntry struct {
	ID        string      `json:"id"`
	NameRU    string      `json:"name_ru"`
	NameEN    string      `json:"name_en"`
	NameJP    string      `json:"name_jp"`
	PosterURL string      `json:"poster_url"`
	Year      int         `json:"year"`
	Episodes  int         `json:"episodes"`
	Score     float64     `json:"score"`
	Status    string      `json:"status"`
	Rating    string      `json:"rating"`
	Genres    []PoolTaxon `json:"genres"`
	Studios   []PoolTaxon `json:"studios"`
	Tags      []PoolTaxon `json:"tags"`
}

type guessPoolRepo interface {
	ListGuessPoolCandidates(ctx context.Context, minScore float64) ([]*domain.Anime, error)
	SetFranchise(ctx context.Context, id, franchise string) error
}

type franchiseFetcher interface {
	GetAnimeFranchise(ctx context.Context, shikimoriID string) (string, error)
}

// GuessPoolService builds the franchise-collapsed score>minScore pool.
type GuessPoolService struct {
	repo     guessPoolRepo
	fetcher  franchiseFetcher
	log      *logger.Logger
	minScore float64
}

func NewGuessPoolService(repo guessPoolRepo, fetcher franchiseFetcher, log *logger.Logger) *GuessPoolService {
	return &GuessPoolService{repo: repo, fetcher: fetcher, log: log, minScore: 8.0}
}

// BuildPool lists candidates, backfills any missing franchise via REST (persisted
// once), collapses by franchise, and maps to the DTO.
func (s *GuessPoolService) BuildPool(ctx context.Context) ([]GuessPoolEntry, error) {
	candidates, err := s.repo.ListGuessPoolCandidates(ctx, s.minScore)
	if err != nil {
		return nil, err
	}

	for _, a := range candidates {
		if a.Franchise != "" || a.ShikimoriID == "" {
			continue
		}
		fr, ferr := s.fetcher.GetAnimeFranchise(ctx, a.ShikimoriID)
		if ferr != nil {
			if s.log != nil {
				s.log.Debugw("franchise backfill failed; treating as standalone",
					"anime_id", a.ID, "shikimori_id", a.ShikimoriID, "error", ferr)
			}
			continue // leave standalone
		}
		if fr == "" {
			continue
		}
		a.Franchise = fr
		if serr := s.repo.SetFranchise(ctx, a.ID, fr); serr != nil && s.log != nil {
			s.log.Warnw("persist franchise failed", "anime_id", a.ID, "error", serr)
		}
	}

	collapsed := collapseByFranchise(candidates)

	out := make([]GuessPoolEntry, 0, len(collapsed))
	for _, a := range collapsed {
		out = append(out, toPoolEntry(a))
	}
	return out, nil
}

func toPoolEntry(a *domain.Anime) GuessPoolEntry {
	e := GuessPoolEntry{
		ID:        a.ID,
		NameRU:    a.NameRU,
		NameEN:    a.NameEN,
		NameJP:    a.NameJP,
		PosterURL: a.PosterURL,
		Year:      a.Year,
		Episodes:  a.EpisodesCount,
		Score:     a.Score,
		Status:    string(a.Status),
		Rating:    a.Rating,
	}
	for _, g := range a.Genres {
		name := g.NameRU
		if name == "" {
			name = g.Name
		}
		e.Genres = append(e.Genres, PoolTaxon{ID: g.ID, Name: name})
	}
	for _, st := range a.Studios {
		e.Studios = append(e.Studios, PoolTaxon{ID: st.ID, Name: st.Name})
	}
	for _, tg := range a.Tags {
		e.Tags = append(e.Tags, PoolTaxon{ID: tg.ID, Name: tg.Name})
	}
	return e
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd services/catalog && go test ./internal/service/ -run 'TestCollapseByFranchise|TestBuildPool' -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/guesspool.go services/catalog/internal/service/guesspool_test.go
git commit -m "feat(catalog): GuessPoolService with franchise backfill + DTO"
```

---

## Task 5: Internal handler + route + DI wiring

**Files:**
- Create: `services/catalog/internal/handler/internal_guesspool.go`
- Test: `services/catalog/internal/handler/internal_guesspool_test.go`
- Modify: `services/catalog/internal/transport/router.go`
- Modify: `services/catalog/cmd/catalog-api/main.go`

- [ ] **Step 1: Write the failing handler test**

Create `services/catalog/internal/handler/internal_guesspool_test.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

type fakePoolBuilder struct {
	entries []service.GuessPoolEntry
}

func (f *fakePoolBuilder) BuildPool(_ context.Context) ([]service.GuessPoolEntry, error) {
	return f.entries, nil
}

func TestInternalGuessPool_GetPool(t *testing.T) {
	builder := &fakePoolBuilder{entries: []service.GuessPoolEntry{
		{ID: "frieren", NameRU: "Фрирен", Score: 9.3},
	}}
	h := NewInternalGuessPoolHandler(builder, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/guessgame/pool", nil)
	h.GetPool(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp struct {
		Success bool `json:"success"`
		Data    []service.GuessPoolEntry `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success || len(resp.Data) != 1 || resp.Data[0].ID != "frieren" {
		t.Fatalf("unexpected body: %+v", resp)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/handler/ -run TestInternalGuessPool -v`
Expected: FAIL — `NewInternalGuessPoolHandler` undefined.

- [ ] **Step 3: Create the handler**

Create `services/catalog/internal/handler/internal_guesspool.go`:

```go
package handler

import (
	"context"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

// poolBuilder is the subset of GuessPoolService the handler needs (interface
// for testability).
type poolBuilder interface {
	BuildPool(ctx context.Context) ([]service.GuessPoolEntry, error)
}

// InternalGuessPoolHandler serves GET /internal/guessgame/pool (Docker-network
// only; NOT proxied by the gateway).
type InternalGuessPoolHandler struct {
	svc poolBuilder
	log *logger.Logger
}

func NewInternalGuessPoolHandler(svc poolBuilder, log *logger.Logger) *InternalGuessPoolHandler {
	return &InternalGuessPoolHandler{svc: svc, log: log}
}

func (h *InternalGuessPoolHandler) GetPool(w http.ResponseWriter, r *http.Request) {
	entries, err := h.svc.BuildPool(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, entries)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/handler/ -run TestInternalGuessPool -v`
Expected: PASS.

- [ ] **Step 5: Register the route**

In `services/catalog/internal/transport/router.go`, add a constructor parameter
`internalGuessPoolHandler *handler.InternalGuessPoolHandler` to `NewRouter(...)`
(place it next to the other `internal*Handler` params), and add this inside the
internal-route block (near the existing `/internal/anime/...` registrations):

```go
	// Anidle guess-game pool (spec 2026-06-15) — Docker-network only.
	if internalGuessPoolHandler != nil {
		r.Get("/internal/guessgame/pool", internalGuessPoolHandler.GetPool)
	}
```

- [ ] **Step 6: Wire DI in main.go**

In `services/catalog/cmd/catalog-api/main.go`, after the existing internal-handler
construction block (near `internalEpisodesHandler := ...`), add:

```go
	guessPoolService := service.NewGuessPoolService(animeRepo, shikimoriClient, log)
	internalGuessPoolHandler := handler.NewInternalGuessPoolHandler(guessPoolService, log)
```

Then add `internalGuessPoolHandler` as the matching argument in the
`transport.NewRouter(...)` call (same position you added the parameter in Step 5).

- [ ] **Step 7: Build the whole service**

Run: `cd services/catalog && go build ./...`
Expected: no output (success). If `animeRepo` does not satisfy `guessPoolRepo` or `shikimoriClient` does not satisfy `franchiseFetcher`, the compiler will say so — both implement the required methods after Tasks 2–3.

- [ ] **Step 8: Run all catalog tests**

Run: `cd services/catalog && go test ./internal/... -count=1`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add services/catalog/internal/handler/internal_guesspool.go \
        services/catalog/internal/handler/internal_guesspool_test.go \
        services/catalog/internal/transport/router.go \
        services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): internal /internal/guessgame/pool endpoint"
```

---

## Task 6: Deploy + live smoke

- [ ] **Step 1: Redeploy catalog (applies the franchise automigrate)**

Run: `make redeploy-catalog`
Expected: build + restart succeed; `make health` shows catalog healthy. The new
`franchise` column is added by GORM automigrate on startup.

- [ ] **Step 2: Smoke the endpoint from inside the Docker network**

The route is NOT gateway-proxied, so curl from within the network:

Run:
```bash
docker compose -f docker/docker-compose.yml exec -T catalog \
  wget -qO- http://localhost:8081/internal/guessgame/pool | head -c 600
```
Expected: JSON `{"success":true,"data":[ ... ]}` with entries carrying `id`,
`name_ru`, `poster_url`, `year`, `episodes`, `score`, `status`, `rating`,
`genres`, `studios`, `tags`. (First call may take longer while franchise backfill
runs; subsequent calls are fast.)

- [ ] **Step 3: Commit (no-op if nothing changed)**

No code change in this task. If `make redeploy-catalog` modified any state file
(e.g. `.claude/maintenance-state.json`), do NOT commit it here — leave it to the
after-update flow.

---

## Self-Review

**Spec coverage (§7 Catalog changes):**
- ✅ `franchise` field on Anime (Task 1) — sourced from REST since GraphQL lacks it (verified 2026-06-15).
- ✅ `GET /internal/guessgame/pool`, Docker-network-only, score>8, franchise-collapsed, 8 attrs + names + poster_url (Tasks 4b, 5).
- ✅ First-aired representative (Task 3 ordering + Task 4a collapse).
- ✅ Reusable franchise field (plain column; not anidle-specific).

**Placeholder scan:** The only intentional placeholder is the `var _` pair in Task 4a, explicitly deleted in Task 4b Step 3. No "TBD"/"handle errors"/vague steps remain; every code step has complete code.

**Type consistency:** `GuessPoolEntry`, `PoolTaxon`, `NewGuessPoolService`, `BuildPool`, `collapseByFranchise`, `ListGuessPoolCandidates`, `SetFranchise`, `GetAnimeFranchise`, `NewInternalGuessPoolHandler`, `GetPool`, `poolBuilder` are used identically across tasks. Handler depends on the `poolBuilder` interface; `GuessPoolService` satisfies it (`BuildPool` signature matches). `animeRepo` satisfies `guessPoolRepo` (Task 3 methods); `shikimoriClient` satisfies `franchiseFetcher` (Task 2 method).

**Known interface assumption:** `config.ShikimoriConfig` exposes `BaseURL`, `GraphQLURL`, `UserAgent`, `Timeout`, `RateLimit` (confirmed used in `client.go`). If a field name differs, adjust `newTestClient` in Task 2 accordingly.
