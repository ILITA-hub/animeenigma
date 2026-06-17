# Profile Showcase Wall — v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the shipped v1 profile showcase with per-block layout **variants**, four **new block types** (`continue_watching`, `op_ed`, `anime_dna`, `compatibility`), an Auto-fill curation affordance, and one new backend endpoint (compatibility).

**Architecture:** v1 is a pure config store on `services/player` (JSONB `blocks`, structural validation, FE-resolved content). v2 adds an optional `variant` string to each block (validated against a per-type allowlist), new block-type constants, FE sub-dispatch per variant, FE-resolved new blocks via existing public APIs, and a single new pairwise endpoint `GET /api/users/{userId}/compatibility` that stays inside the player service (it already owns watchlists + genres).

**Tech Stack:** Go (chi, GORM, SQLite tests), Vue 3 + TypeScript, Vitest.

## Global Constraints

Apply to **every** task:

- **Visual source of truth:** the committed mockups at `docs/superpowers/specs/assets/2026-06-17-showcase-v2/` (`about.html`, `anime.html`, `stats.html`, `characters.html`, `cards.html`, `new.html`, `base.css`). When a task says "port variant X", open the named asset file, find the matching CSS/markup, and translate it into the Vue SFC using **semantic DS tokens** (NOT the asset's raw hex/rgba — the asset uses literal values for standalone preview; the real components must bind to tokens per `frontend/web/src/styles/DESIGN-SYSTEM.md`).
- **Variant field is additive + backward compatible.** An existing v1 block with no `variant` renders the type's **default** variant. Never break the v1 stored shape.
- **Backend stays a structural config store.** `ValidateBlocks` only checks: known type, `(type,variant)` in allowlist, config size limits. NO content/ownership/visibility resolution (except the existing inline `stats` is already FE-resolved — leave as is). The ONLY new backend data path is the compatibility endpoint.
- **400 errors** use `errors.InvalidInput(msg)`; internal wrap `errors.Wrap(err, errors.CodeInternal, msg)`. (`errors.BadRequest` does NOT exist.)
- **Dark-ship: same gate, no new flag.** Compatibility route sits under the existing `PROFILE_WALL_ADMIN_ONLY` gateway gate.
- **DS lint (build-enforced):** semantic tokens only; only `font-medium`/`font-semibold`; no hardcoded hex / off-palette Tailwind colors in `.vue`; padding scale. `bash frontend/web/scripts/design-system-lint.sh` must show ZERO errors attributable to new/changed files.
- **i18n:** every new key in ALL THREE locales (en/ru/ja); additive edits only at the `showcase` namespace append point (do NOT rewrite surrounding keys — v1 hazard clobbered `player.sources`). Run `bash frontend/web/scripts/i18n-lint.sh`.
- **Commits:** path-scoped (`git commit <pathspec>`), never `git add -A`/bare/amend/reset/stash (shared tree). `git show --stat HEAD` after each. Co-author trailers:
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Tests:** Go service/repo tests use in-memory SQLite + raw-SQL schema (Postgres `jsonb`/defaults break SQLite AutoMigrate); mirror `service/showcase_test.go` / `service/comment_test.go`.

### Scope cuts (deliberate, documented)

- **`stats` `heatmap` variant is DEFERRED** — it needs a per-day watch-count aggregate that doesn't exist yet. v2 stats variants = `tiles | rings | bars | strip`. (Follow-up: add a daily-activity endpoint, then the heatmap variant.)
- **`continue_watching` uses the public watchlist** (`status=watching`) for all viewers — titles + posters, NO precise per-episode progress bar (that data isn't public). No new endpoint. (Follow-up: public progress endpoint for exact bars.)
- **`compatibility`** is the only new backend endpoint.

### Variant allowlist (canonical — used by Task 1 backend AND Task 4 frontend)

```
about              : quote(default) | bio | terminal | minimal | vn
favorite_anime     : row(default)   | podium | grid | list | banner
favorite_character : circles(default) | portraits | hero | hex
card_collection    : row(default)   | fan | grid | hero | tilt3d
stats              : tiles(default) | rings | bars | strip
continue_watching  : cards(default)
op_ed              : grid(default)
anime_dna          : bars(default)
compatibility      : ring(default)
```

## File Structure

**Backend (`services/player`):**
- `internal/domain/showcase.go` — add `Variant` to `Block`, new type consts, `VariantAllowlist` map, extend `ValidateBlocks` (modify).
- `internal/domain/compatibility.go` — `CompatibilityResult` struct (create).
- `internal/repo/compatibility.go` — fetch a user's `(animeID, score, genreIDs)` list (create).
- `internal/service/compatibility.go` — blend computation (create).
- `internal/handler/compatibility.go` — `GET /users/{userId}/compatibility` (create).
- `internal/transport/router.go` — register compatibility route (modify).
- `cmd/player-api/main.go` — wire compatibility repo→service→handler (modify).

**Gateway:**
- `internal/transport/router.go` — route `GET /users/{userId}/compatibility` under the `PROFILE_WALL_ADMIN_ONLY` gate (modify).

**Frontend (`frontend/web`):**
- `src/types/showcase.ts` — `variant`, new types/configs, allowlist (modify).
- `src/api/client.ts` — `showcaseApi.getCompatibility` (modify).
- `src/components/profile/showcase/blocks/*.vue` — each existing block gains a variant sub-dispatch (modify ×5); new `ContinueWatchingBlock.vue`, `OpEdBlock.vue`, `AnimeDnaBlock.vue`, `CompatibilityBlock.vue` (create ×4).
- `src/components/profile/showcase/ProfileShowcase.vue` — pass `variant`, dispatch new types (modify).
- `src/components/profile/showcase/ShowcaseEditor.vue` — variant picker, Auto buttons, new add-block entries, op_ed picker (modify).
- `src/locales/{en,ru,ja}.json` — new keys (modify).

---

## Task 1: Backend — `Variant` field, new types, allowlist validation

**Files:**
- Modify: `services/player/internal/domain/showcase.go`
- Test: `services/player/internal/domain/showcase_test.go`

**Interfaces:**
- Produces: `Block` gains `Variant string \`json:"variant,omitempty"\``; consts `BlockContinueWatching="continue_watching"`, `BlockOpEd="op_ed"`, `BlockAnimeDNA="anime_dna"`, `BlockCompatibility="compatibility"`; `var VariantAllowlist map[string][]string`; `ValidateBlocks` validates variant + new types. `MaxBlockItems` (12) reused for `op_ed` theme_ids.

- [ ] **Step 1: Write failing tests**

Append to `services/player/internal/domain/showcase_test.go`:

```go
func TestValidateBlocks_VariantOK(t *testing.T) {
	b := []Block{{Type: BlockFavoriteAnime, Variant: "podium", Config: cfg(t, map[string][]string{"anime_ids": {"a"}})}}
	if err := ValidateBlocks(b); err != nil {
		t.Fatalf("expected valid variant, got %v", err)
	}
}

func TestValidateBlocks_EmptyVariantOK(t *testing.T) {
	b := []Block{{Type: BlockAbout, Config: cfg(t, map[string]string{"text": "hi"})}}
	if err := ValidateBlocks(b); err != nil {
		t.Fatalf("empty variant must be allowed (defaults), got %v", err)
	}
}

func TestValidateBlocks_UnknownVariant(t *testing.T) {
	b := []Block{{Type: BlockStats, Variant: "bogus", Config: cfg(t, map[string]any{})}}
	if err := ValidateBlocks(b); err == nil {
		t.Fatal("expected error for unknown variant")
	}
}

func TestValidateBlocks_NewTypesAccepted(t *testing.T) {
	for _, ty := range []string{BlockContinueWatching, BlockAnimeDNA, BlockCompatibility} {
		if err := ValidateBlocks([]Block{{Type: ty, Config: cfg(t, map[string]any{})}}); err != nil {
			t.Fatalf("auto type %s should validate, got %v", ty, err)
		}
	}
}

func TestValidateBlocks_OpEdLimit(t *testing.T) {
	ids := make([]string, MaxBlockItems+1)
	for i := range ids { ids[i] = "t" }
	b := []Block{{Type: BlockOpEd, Config: cfg(t, map[string][]string{"theme_ids": ids})}}
	if err := ValidateBlocks(b); err == nil {
		t.Fatal("expected error: too many op_ed theme_ids")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd services/player && go test ./internal/domain/ -run 'Variant|NewTypes|OpEd' -v`
Expected: FAIL (compile: `BlockContinueWatching` etc. undefined / variant not validated).

- [ ] **Step 3: Implement**

In `services/player/internal/domain/showcase.go`:

(a) Add `Variant` to the `Block` struct:
```go
type Block struct {
	Type    string          `json:"type"`
	Variant string          `json:"variant,omitempty"`
	Order   int             `json:"order"`
	Config  json.RawMessage `json:"config"`
}
```

(b) Add new type consts to the existing `const (...)` block:
```go
	BlockContinueWatching = "continue_watching"
	BlockOpEd             = "op_ed"
	BlockAnimeDNA         = "anime_dna"
	BlockCompatibility    = "compatibility"
```

(c) Add the allowlist + an `opEdConfig` type:
```go
// VariantAllowlist maps each block type to its permitted variants. The first
// entry is the default (used when Block.Variant is empty). Keep in sync with
// frontend src/types/showcase.ts.
var VariantAllowlist = map[string][]string{
	BlockAbout:             {"quote", "bio", "terminal", "minimal", "vn"},
	BlockFavoriteAnime:     {"row", "podium", "grid", "list", "banner"},
	BlockFavoriteCharacter: {"circles", "portraits", "hero", "hex"},
	BlockCardCollection:    {"row", "fan", "grid", "hero", "tilt3d"},
	BlockStats:             {"tiles", "rings", "bars", "strip"},
	BlockContinueWatching:  {"cards"},
	BlockOpEd:              {"grid"},
	BlockAnimeDNA:          {"bars"},
	BlockCompatibility:     {"ring"},
}

type opEdConfig struct {
	ThemeIDs []string `json:"theme_ids"`
}

func variantAllowed(blockType, variant string) bool {
	if variant == "" {
		return true // empty ⇒ default
	}
	for _, v := range VariantAllowlist[blockType] {
		if v == variant {
			return true
		}
	}
	return false
}
```

(d) Extend `ValidateBlocks`. Add the variant check at the top of the loop body, add the new type cases, and keep the existing cases:
```go
	for _, b := range blocks {
		if _, known := VariantAllowlist[b.Type]; !known {
			return errors.InvalidInput("unknown showcase block type")
		}
		if !variantAllowed(b.Type, b.Variant) {
			return errors.InvalidInput("unknown variant for block type")
		}
		switch b.Type {
		case BlockStats, BlockContinueWatching, BlockAnimeDNA, BlockCompatibility:
			// auto blocks: no config required
		case BlockAbout:
			// ... existing about validation unchanged ...
		case BlockFavoriteAnime:
			// ... existing ...
		case BlockCardCollection:
			// ... existing ...
		case BlockFavoriteCharacter:
			// ... existing ...
		case BlockOpEd:
			var c opEdConfig
			if err := json.Unmarshal(b.Config, &c); err != nil {
				return errors.InvalidInput("invalid op_ed config")
			}
			if len(c.ThemeIDs) > MaxBlockItems {
				return errors.InvalidInput("too many op_ed themes")
			}
		}
	}
	return nil
```
> Keep the existing `default:` removed — the `known` check above replaces it (the `switch` no longer needs a default since unknown types are rejected before it). Preserve the existing about/anime/card/character case bodies exactly as they are today.

- [ ] **Step 4: Run — expect PASS**

Run: `cd services/player && go test ./internal/domain/ -v`
Expected: PASS (new + all existing v1 domain tests).

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/domain/showcase.go services/player/internal/domain/showcase_test.go
git commit services/player/internal/domain/showcase.go services/player/internal/domain/showcase_test.go -m "feat(player): showcase v2 — variant field, new block types, allowlist validation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -6
```

---

## Task 2: Backend — compatibility repo + service (blend math)

**Files:**
- Create: `services/player/internal/domain/compatibility.go`
- Create: `services/player/internal/repo/compatibility.go`
- Create: `services/player/internal/service/compatibility.go`
- Test: `services/player/internal/service/compatibility_test.go`

**Interfaces:**
- Produces:
  - `domain.CompatibilityResult struct { Percent int; SharedCount int; SharedSample []string }`
  - `domain.UserListEntry struct { AnimeID string; Score int; GenreIDs []string }`
  - `repo.CompatibilityRepository` with `ListEntries(ctx, userID string) ([]domain.UserListEntry, error)`
  - `service.CompatibilityService` with `NewCompatibilityService(repo)` and
    `Compute(ctx, viewerID, ownerID string) (*domain.CompatibilityResult, error)`
- Consumes: nothing from earlier tasks (independent).

- [ ] **Step 1: Write the failing test** (pure blend math via a fake repo)

Create `services/player/internal/service/compatibility_test.go`:

```go
package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/require"
)

type fakeCompatRepo struct{ data map[string][]domain.UserListEntry }

func (f *fakeCompatRepo) ListEntries(_ context.Context, uid string) ([]domain.UserListEntry, error) {
	return f.data[uid], nil
}

func TestCompatibility_IdenticalLists100(t *testing.T) {
	e := []domain.UserListEntry{{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}, {AnimeID: "b", Score: 9, GenreIDs: []string{"g1"}}}
	svc := NewCompatibilityService(&fakeCompatRepo{data: map[string][]domain.UserListEntry{"v": e, "o": e}})
	r, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	require.Equal(t, 100, r.Percent)
	require.Equal(t, 2, r.SharedCount)
}

func TestCompatibility_NoOverlap0(t *testing.T) {
	svc := NewCompatibilityService(&fakeCompatRepo{data: map[string][]domain.UserListEntry{
		"v": {{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}},
		"o": {{AnimeID: "z", Score: 8, GenreIDs: []string{"g9"}}},
	}})
	r, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	require.Equal(t, 0, r.Percent)
	require.Equal(t, 0, r.SharedCount)
}

func TestCompatibility_PartialBlend(t *testing.T) {
	// overlap 1/3 titles, identical scores on shared, identical genre vectors
	svc := NewCompatibilityService(&fakeCompatRepo{data: map[string][]domain.UserListEntry{
		"v": {{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}, {AnimeID: "b", Score: 7, GenreIDs: []string{"g1"}}},
		"o": {{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}, {AnimeID: "c", Score: 6, GenreIDs: []string{"g1"}}},
	}})
	r, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	// overlap = 1/3 ; scoreAgreement = 1 (identical on shared "a") ; genreSim = 1
	// score = 0.5*0.333 + 0.4*1 + 0.1*1 = 0.6667 -> 67
	require.InDelta(t, 67, r.Percent, 1)
	require.Equal(t, 1, r.SharedCount)
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `cd services/player && go test ./internal/service/ -run TestCompatibility -v`
Expected: FAIL (undefined `NewCompatibilityService` / domain types).

- [ ] **Step 3: Implement domain + service**

Create `services/player/internal/domain/compatibility.go`:
```go
package domain

// UserListEntry is the minimal projection used by the compatibility blend.
type UserListEntry struct {
	AnimeID  string
	Score    int      // 0 = unrated
	GenreIDs []string
}

// CompatibilityResult is returned by GET /users/{userId}/compatibility.
type CompatibilityResult struct {
	Percent      int      `json:"percent"`
	SharedCount  int      `json:"shared_count"`
	SharedSample []string `json:"shared_sample"` // up to 8 shared anime IDs
}
```

Create `services/player/internal/service/compatibility.go`:
```go
package service

import (
	"context"
	"math"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
)

// compatRepo is the data dependency (real repo or a test fake).
type compatRepo interface {
	ListEntries(ctx context.Context, userID string) ([]domain.UserListEntry, error)
}

type CompatibilityService struct{ repo compatRepo }

func NewCompatibilityService(r compatRepo) *CompatibilityService {
	return &CompatibilityService{repo: r}
}

// Compute blends list overlap (0.5), score agreement (0.4) and genre
// similarity (0.1) into a 0..100 percent.
func (s *CompatibilityService) Compute(ctx context.Context, viewerID, ownerID string) (*domain.CompatibilityResult, error) {
	ve, err := s.repo.ListEntries(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	oe, err := s.repo.ListEntries(ctx, ownerID)
	if err != nil {
		return nil, err
	}

	vm := map[string]domain.UserListEntry{}
	for _, e := range ve {
		vm[e.AnimeID] = e
	}
	om := map[string]domain.UserListEntry{}
	for _, e := range oe {
		om[e.AnimeID] = e
	}

	// overlap = Jaccard of title sets
	shared := []string{}
	for id := range vm {
		if _, ok := om[id]; ok {
			shared = append(shared, id)
		}
	}
	union := len(vm) + len(om) - len(shared)
	overlap := 0.0
	if union > 0 {
		overlap = float64(len(shared)) / float64(union)
	}

	// scoreAgreement on commonly-rated titles (both scores > 0)
	var diffSum float64
	var rated int
	for _, id := range shared {
		if vm[id].Score > 0 && om[id].Score > 0 {
			diffSum += math.Abs(float64(vm[id].Score - om[id].Score))
			rated++
		}
	}
	scoreAgreement := 1.0 // neutral when nothing co-rated
	if rated > 0 {
		scoreAgreement = 1.0 - (diffSum/float64(rated))/10.0 // scores are 1..10
		if scoreAgreement < 0 {
			scoreAgreement = 0
		}
	}

	genreSim := cosineGenre(ve, oe)

	score := 0.5*overlap + 0.4*scoreAgreement + 0.1*genreSim
	sample := shared
	if len(sample) > 8 {
		sample = sample[:8]
	}
	return &domain.CompatibilityResult{
		Percent:      int(math.Round(score * 100)),
		SharedCount:  len(shared),
		SharedSample: sample,
	}, nil
}

func cosineGenre(a, b []domain.UserListEntry) float64 {
	av, bv := genreVec(a), genreVec(b)
	if len(av) == 0 || len(bv) == 0 {
		return 0
	}
	var dot, na, nb float64
	for g, c := range av {
		dot += c * bv[g]
		na += c * c
	}
	for _, c := range bv {
		nb += c * c
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func genreVec(entries []domain.UserListEntry) map[string]float64 {
	v := map[string]float64{}
	for _, e := range entries {
		for _, g := range e.GenreIDs {
			v[g]++
		}
	}
	return v
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd services/player && go test ./internal/service/ -run TestCompatibility -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Implement the real repo**

Create `services/player/internal/repo/compatibility.go`. Use GORM to load the user's anime list with genres. Mirror how `repo/list.go` preloads `Anime.Genres` (lines ~141/154 use `Preload("Anime").Preload("Anime.Genres")`). The watchlist row type + table are defined in `internal/domain/watch.go` (the list entry has `UserID`, `AnimeID`, `Score`; `Anime.Genres` is the many2many).

```go
package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
)

type CompatibilityRepository struct{ db *gorm.DB }

func NewCompatibilityRepository(db *gorm.DB) *CompatibilityRepository {
	return &CompatibilityRepository{db: db}
}

// ListEntries returns the user's list as compatibility projections.
func (r *CompatibilityRepository) ListEntries(ctx context.Context, userID string) ([]domain.UserListEntry, error) {
	var rows []domain.AnimeListItem // the GORM model backing anime_list; confirm the exact type name in domain/watch.go
	err := r.db.WithContext(ctx).
		Preload("Anime").Preload("Anime.Genres").
		Where("user_id = ?", userID).
		Find(&rows).Error
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to load list for compatibility")
	}
	out := make([]domain.UserListEntry, 0, len(rows))
	for _, row := range rows {
		genreIDs := make([]string, 0, len(row.Anime.Genres))
		for _, g := range row.Anime.Genres {
			genreIDs = append(genreIDs, g.ID)
		}
		out = append(out, domain.UserListEntry{AnimeID: row.AnimeID, Score: row.Score, GenreIDs: genreIDs})
	}
	return out, nil
}
```
> CONFIRM the real list model type name + fields by reading `services/player/internal/domain/watch.go` and `repo/list.go` (the struct may be `AnimeListItem`, `WatchlistItem`, or similar; `Score` may be `*int` — handle nil as 0; `Genres[].ID` is the genre PK string). Adjust the projection to the real names. Repo has no unit test (it's exercised via the handler smoke + the service tests use the fake); the build must compile.

- [ ] **Step 6: Build + commit**

Run: `cd services/player && go build ./... && go test ./internal/service/ -run TestCompatibility`
Expected: build OK, tests PASS.

```bash
git add services/player/internal/domain/compatibility.go services/player/internal/repo/compatibility.go services/player/internal/service/compatibility.go services/player/internal/service/compatibility_test.go
git commit services/player/internal/domain/compatibility.go services/player/internal/repo/compatibility.go services/player/internal/service/compatibility.go services/player/internal/service/compatibility_test.go -m "feat(player): showcase v2 — compatibility blend (overlap+scores+genre)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -8
```

---

## Task 3: Backend — compatibility handler + routes (player + gateway)

**Files:**
- Create: `services/player/internal/handler/compatibility.go`
- Modify: `services/player/internal/transport/router.go`
- Modify: `services/player/cmd/player-api/main.go`
- Modify: `services/gateway/internal/transport/router.go`

**Interfaces:**
- Consumes: `service.CompatibilityService.Compute(ctx, viewerID, ownerID)` (Task 2).
- Produces: `GET /api/users/{userId}/compatibility` → `{ percent, shared_count, shared_sample }`.

- [ ] **Step 1: Implement the handler**

Create `services/player/internal/handler/compatibility.go` (mirror `handler/showcase.go` helpers — `httputil.OK`, `authz.ClaimsFromContext`):
```go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

type CompatibilityHandler struct {
	svc *service.CompatibilityService
	log *logger.Logger
}

func NewCompatibilityHandler(s *service.CompatibilityService, log *logger.Logger) *CompatibilityHandler {
	return &CompatibilityHandler{svc: s, log: log}
}

// GetCompatibility handles GET /api/users/{userId}/compatibility.
// JWT required; computes the viewer (claims) vs {userId} (the profile owner).
func (h *CompatibilityHandler) GetCompatibility(w http.ResponseWriter, r *http.Request) {
	ownerID := chi.URLParam(r, "userId")
	if ownerID == "" {
		httputil.BadRequest(w, "userId is required")
		return
	}
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	if claims.UserID == ownerID {
		// own profile: compatibility is meaningless — return 100% / no sample so the FE can hide it
		httputil.OK(w, map[string]any{"percent": 100, "shared_count": 0, "shared_sample": []string{}, "self": true})
		return
	}
	res, err := h.svc.Compute(r.Context(), claims.UserID, ownerID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, res)
}
```
> Confirm `httputil.OK`/`BadRequest`/`Unauthorized`/`Error` against `handler/showcase.go` (Task v1 used exactly these).

- [ ] **Step 2: Wire route in player `transport/router.go`**

Add `compatibilityHandler *handler.CompatibilityHandler` to the `NewRouter` signature (right after `showcaseHandler`). Register the route **inside** the JWT-protected `/users` group (it requires auth), as a specific path. Mirror where `r.Put("/me/showcase", ...)` was added; add:
```go
			r.Get("/{userId}/compatibility", compatibilityHandler.GetCompatibility)
```
> This lands at `/api/users/{userId}/compatibility` inside the `AuthMiddleware`-protected group, so `claims` is always present.

- [ ] **Step 3: Wire DI in `cmd/player-api/main.go`**

After the showcase wiring, add:
```go
	compatRepo := repo.NewCompatibilityRepository(db.DB)
	compatService := service.NewCompatibilityService(compatRepo)
	compatibilityHandler := handler.NewCompatibilityHandler(compatService, log)
```
Pass `compatibilityHandler` into `transport.NewRouter(...)` in the position matching the signature (right after `showcaseHandler`). Update the existing `router_internal_list_test.go` `NewRouter(...)` stub call with a `nil` in the new position if it constructs the router.

- [ ] **Step 4: Wire gateway route under the gate**

In `services/gateway/internal/transport/router.go`, the v1 showcase gate block branches on `cfg.ProfileWallAdminOnly`. Add the compatibility route to BOTH branches (it always needs JWT; when dark-shipped also admin). In the `if cfg.ProfileWallAdminOnly` group (JWT + admin) add:
```go
			r.Get("/users/{userId}/compatibility", proxyHandler.ProxyToPlayer)
```
In the `else` (released) branch, compatibility needs JWT (not public) — it falls through to the existing protected `/users/*` group automatically, so **no line needed there**. (Add a one-line comment noting that.)

- [ ] **Step 5: Build both services + commit**

Run: `cd services/player && go build ./... && go test ./internal/... ; cd ../gateway && go build ./...`
Expected: builds OK; player tests PASS (pre-existing unrelated `TestMALExportHandler_GetUserExports_Authorized` network failure may persist — ignore, it predates this work).

```bash
git add services/player/internal/handler/compatibility.go services/player/internal/transport/router.go services/player/cmd/player-api/main.go services/gateway/internal/transport/router.go
# add the test file too if you modified the NewRouter stub
git commit services/player/internal/handler/compatibility.go services/player/internal/transport/router.go services/player/cmd/player-api/main.go services/gateway/internal/transport/router.go -m "feat(player+gateway): showcase v2 — compatibility endpoint under dark-ship gate

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -10
```

---

## Task 4: Frontend — types, api, container dispatch foundation

**Files:**
- Modify: `frontend/web/src/types/showcase.ts`
- Modify: `frontend/web/src/api/client.ts`
- Modify: `frontend/web/src/components/profile/showcase/ProfileShowcase.vue`
- Test: `frontend/web/src/components/profile/showcase/__tests__/ProfileShowcase.spec.ts` (extend)

**Interfaces:**
- Produces: `ShowcaseBlock` gains `variant?: string`; new type literals + config interfaces; `SHOWCASE_VARIANTS` allowlist const; `showcaseApi.getCompatibility(userId)`. Container imports + dispatches the 4 new block components and passes `:variant`.
- Consumes: Task 1 type names (string values must match the Go allowlist exactly).

- [ ] **Step 1: Extend `src/types/showcase.ts`**

```ts
export type ShowcaseBlockType =
  | 'about' | 'favorite_anime' | 'stats' | 'favorite_character' | 'card_collection'
  | 'continue_watching' | 'op_ed' | 'anime_dna' | 'compatibility'

export interface OpEdConfig { theme_ids: string[] }
// continue_watching / anime_dna / compatibility carry no config:
export type AutoConfig = Record<string, never>

export interface ShowcaseBlock {
  type: ShowcaseBlockType
  variant?: string
  order: number
  config: AboutConfig | FavoriteAnimeConfig | FavoriteCharacterConfig
        | CardCollectionConfig | StatsConfig | OpEdConfig | AutoConfig
}

// Allowlist — MUST mirror domain.VariantAllowlist (Go). First entry = default.
export const SHOWCASE_VARIANTS: Record<ShowcaseBlockType, string[]> = {
  about: ['quote', 'bio', 'terminal', 'minimal', 'vn'],
  favorite_anime: ['row', 'podium', 'grid', 'list', 'banner'],
  favorite_character: ['circles', 'portraits', 'hero', 'hex'],
  card_collection: ['row', 'fan', 'grid', 'hero', 'tilt3d'],
  stats: ['tiles', 'rings', 'bars', 'strip'],
  continue_watching: ['cards'],
  op_ed: ['grid'],
  anime_dna: ['bars'],
  compatibility: ['ring'],
}
export const defaultVariant = (t: ShowcaseBlockType) => SHOWCASE_VARIANTS[t][0]
```
(Keep the existing `AboutConfig`/`FavoriteAnimeConfig`/etc. and `MAX_*` consts.)

- [ ] **Step 2: Add the compatibility API method** in `src/api/client.ts` `showcaseApi`:
```ts
  getCompatibility: (userId: string) =>
    apiClient.get<{ percent: number; shared_count: number; shared_sample: string[]; self?: boolean }
      | { data: { percent: number; shared_count: number; shared_sample: string[]; self?: boolean } }>(
      `/users/${userId}/compatibility`,
    ),
```

- [ ] **Step 3: Update container dispatch** in `ProfileShowcase.vue`

Import the 4 new components (created in Tasks 10–11 — for now create empty stub SFCs so this compiles, OR sequence Task 4 after 10–11; RECOMMENDED: create the 4 files as one-line stubs here and flesh them out in 10–11). Add `:variant="b.variant"` to every block in the dispatch chain, and add the new `v-else-if` branches:
```vue
        <AboutBlock v-if="b.type === 'about'" :config="b.config as never" :variant="b.variant" />
        <FavoriteAnimeBlock v-else-if="b.type === 'favorite_anime'" :config="b.config as never" :variant="b.variant" :user-id="userId" />
        <StatsBlock v-else-if="b.type === 'stats'" :user-id="userId" :variant="b.variant" />
        <FavoriteCharacterBlock v-else-if="b.type === 'favorite_character'" :config="b.config as never" :variant="b.variant" />
        <CardCollectionBlock v-else-if="b.type === 'card_collection'" :config="b.config as never" :user-id="userId" :variant="b.variant" />
        <ContinueWatchingBlock v-else-if="b.type === 'continue_watching'" :user-id="userId" />
        <OpEdBlock v-else-if="b.type === 'op_ed'" :config="b.config as never" />
        <AnimeDnaBlock v-else-if="b.type === 'anime_dna'" :user-id="userId" />
        <CompatibilityBlock v-else-if="b.type === 'compatibility'" :user-id="userId" :is-owner="isOwner" />
```
> Note FavoriteAnimeBlock/CardCollectionBlock now also receive `:user-id` (needed by some variants). Create the 4 new SFCs as stubs now:
```vue
<script setup lang="ts">defineProps<{ userId?: string; config?: unknown; isOwner?: boolean }>()</script>
<template><div /></template>
```

- [ ] **Step 4: Run gates + commit**

Run (from `frontend/web`): `bunx tsc --noEmit && bunx vitest run src/components/profile/showcase/`
Expected: tsc 0 errors; existing showcase specs still PASS.

```bash
git commit frontend/web/src/types/showcase.ts frontend/web/src/api/client.ts frontend/web/src/components/profile/showcase/ProfileShowcase.vue frontend/web/src/components/profile/showcase/blocks/ContinueWatchingBlock.vue frontend/web/src/components/profile/showcase/blocks/OpEdBlock.vue frontend/web/src/components/profile/showcase/blocks/AnimeDnaBlock.vue frontend/web/src/components/profile/showcase/blocks/CompatibilityBlock.vue -m "feat(web): showcase v2 — variant types, compatibility api, container dispatch

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -10
```

---

## Tasks 5–9: Variant sub-dispatch for the five existing block components

Each existing block SFC becomes a **dispatcher on `props.variant`** (falling back to the type's default). The structural pattern is identical across all five; the per-variant markup + styles are **ported from the committed asset file** named in each task, rebinding raw colors to DS tokens.

**Shared pattern (apply in every task 5–9):**
```vue
<script setup lang="ts">
import { computed } from 'vue'
import { defaultVariant } from '@/types/showcase'
const props = defineProps<{ /* config / userId per block */ variant?: string }>()
const v = computed(() => props.variant || defaultVariant('<type>'))
</script>
<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <!-- header (existing) -->
    <div v-if="v === 'variantA'"> … </div>
    <div v-else-if="v === 'variantB'"> … </div>
    <!-- … -->
  </div>
</template>
<style scoped> /* ported per-variant CSS, DS tokens */ </style>
```
Keep the v-if/v-else-if chain contiguous (no element between branches). Reuse the existing data-resolution logic already in each v1 component (PosterCard `:model`, gacha `cardImageUrl`, `CharacterCard`, `publicApi.getPublicWatchlistStats`) — only the **layout** changes per variant.

### Task 5: `AboutBlock.vue` variants — `quote|bio|terminal|minimal|vn`
**Files:** Modify `blocks/AboutBlock.vue`; Test `blocks/__tests__/AboutBlock.spec.ts`. **Asset:** `assets/2026-06-17-showcase-v2/about.html` (variants A–E).
- [ ] Step 1: Add a failing test asserting `vn` variant renders the name-tag + text and `terminal` renders the prompt lines.
```ts
it('renders vn variant with name tag', () => {
  const w = mount(AboutBlock, { props: { config: { title:'', text:'hello' }, variant:'vn' }, global:{ mocks:{ $t:(k:string)=>k } } })
  expect(w.text()).toContain('hello')
})
```
- [ ] Step 2: Run → FAIL. `bunx vitest run src/components/profile/showcase/blocks/__tests__/AboutBlock.spec.ts`
- [ ] Step 3: Implement the 5-variant dispatch, porting markup/CSS from `about.html` (`.a-quote`, `.a-bio`, `.a-term`, `.a-min`, `.a-vn`), binding colors to tokens (`text-foreground`, `text-muted-foreground`, `border-border`, brand cyan/pink via tokens). Keep `config.title`/`config.text`.
- [ ] Step 4: Run → PASS (new + existing AboutBlock tests).
- [ ] Step 5: Commit (`blocks/AboutBlock.vue` + its spec, path-scoped, co-authors).

### Task 6: `FavoriteAnimeBlock.vue` variants — `row|podium|grid|list|banner`
**Files:** Modify `blocks/FavoriteAnimeBlock.vue`; Test its spec. **Asset:** `anime.html` (A–E). Props gain `variant?` + `userId?`.
- [ ] Step 1: Failing test: `podium` variant renders 3 ranked posters; `list` renders rows with scores.
- [ ] Step 2: Run → FAIL.
- [ ] Step 3: Implement dispatch porting `.row/.podium/.podrest→podium/.grid6/.list/.bstrip` from `anime.html`; reuse the existing PosterCard `:model` resolution + `fromHomeAnime`.
- [ ] Step 4: Run → PASS.
- [ ] Step 5: Commit.

### Task 7: `StatsBlock.vue` variants — `tiles|rings|bars|strip`
**Files:** Modify `blocks/StatsBlock.vue`; Test its spec. **Asset:** `stats.html` (A–D; **skip heatmap/E** — deferred per Global Constraints).
- [ ] Step 1: Failing test: `rings` variant renders 4 conic rings; `strip` renders inline stats.
- [ ] Step 2: Run → FAIL.
- [ ] Step 3: Implement dispatch porting `.tiles/.rings/.bars/.strip`; reuse the existing `publicApi.getPublicWatchlistStats` resolution + the real stat keys (`profile.stats.*`, fields `total_entries`/`avg_score`/`total_episodes`/`completed`). For `bars` (per-status counts) reuse the stats payload's status counts if present, else render the four headline stats as bars.
- [ ] Step 4: Run → PASS.
- [ ] Step 5: Commit.

### Task 8: `FavoriteCharacterBlock.vue` variants — `circles|portraits|hero|hex`
**Files:** Modify `blocks/FavoriteCharacterBlock.vue`; Test its spec. **Asset:** `characters.html` (A–D).
- [ ] Step 1: Failing test: `hero` variant renders the big card + the ranked list rows; `portraits` renders name overlays.
- [ ] Step 2: Run → FAIL.
- [ ] Step 3: Implement dispatch porting `.circs/.pcards/.hero(+.li)/.hexes`; reuse the existing character resolution + `CharacterCard` where it fits (circles), custom markup for the others.
- [ ] Step 4: Run → PASS.
- [ ] Step 5: Commit.

### Task 9: `CardCollectionBlock.vue` variants — `row|fan|grid|hero|tilt3d`
**Files:** Modify `blocks/CardCollectionBlock.vue`; Test its spec. **Asset:** `cards.html` (A–E, incl. dialog + tilt JS). Props gain `variant?` + keep `userId`.
- [ ] Step 1: Failing test: `fan` renders 5 overlapped cards; `hero` renders the feature card + info panel.
- [ ] Step 2: Run → FAIL.
- [ ] Step 3: Implement dispatch porting `.row/.fan/.gridc/.heror/.tilt`; reuse the existing gacha `cardImageUrl` + owned-filter resolution. Port the card-detail **dialog** (open on click, ease-out/in, dimmed backdrop, Esc/✕/backdrop close) and the **3D tilt** mousemove handler for the `tilt3d` variant. Respect `prefers-reduced-motion`.
- [ ] Step 4: Run → PASS.
- [ ] Step 5: Commit.

---

## Task 10: New FE blocks — `ContinueWatchingBlock` + `AnimeDnaBlock`

**Files:**
- Modify (flesh out stubs): `blocks/ContinueWatchingBlock.vue`, `blocks/AnimeDnaBlock.vue`
- Test: co-located specs for each.
- Asset: `new.html` (`.cw`/`.cwc` and `.dna`/`.drow`).

**Interfaces:** Consume `publicApi.getPublicWatchlist(userId, {status:'watching'})` and `publicApi.getPublicWatchlistFacets(userId)` (both exist in `client.ts`).

- [ ] Step 1: Failing tests — ContinueWatching renders a card per watching title (mock the API to return 2 titles); AnimeDna renders a bar per genre facet.
- [ ] Step 2: Run → FAIL.
- [ ] Step 3: Implement:
  - `ContinueWatchingBlock.vue` — `onMounted` fetch `getPublicWatchlist(userId, {status:'watching', per_page:12})`, unwrap envelope, render `.cw` landscape cards (poster + title); NO precise progress bar (deferred — Global Constraints). Reuse `getLocalizedTitle` if other blocks do.
  - `AnimeDnaBlock.vue` — `onMounted` fetch `getPublicWatchlistFacets(userId)`, take top genres by count, compute % of max, render `.dna` neon bars. Empty facets ⇒ render nothing.
  - Bind all colors to DS tokens.
- [ ] Step 4: Run → PASS (+ tsc).
- [ ] Step 5: Commit (both SFCs + specs + any i18n keys `showcase.block.continue_watching`/`anime_dna` in en/ru/ja).

---

## Task 11: New FE blocks — `OpEdBlock` + `CompatibilityBlock`

**Files:**
- Modify (flesh out stubs): `blocks/OpEdBlock.vue`, `blocks/CompatibilityBlock.vue`
- Test: co-located specs.
- Asset: `new.html` (`.th3`/`.tc` and `.compat`/`.ring2`/`.shared`).

**Interfaces:** Consume `themesApi.get(id)` / `themesApi.list` (exist, `client.ts:877`) and `showcaseApi.getCompatibility(userId)` (Task 4).

- [ ] Step 1: Failing tests — OpEd renders a theme card per configured `theme_ids` (mock themesApi); Compatibility renders the percent ring (mock getCompatibility → 73), and renders nothing when `isOwner` (or `self:true`).
- [ ] Step 2: Run → FAIL.
- [ ] Step 3: Implement:
  - `OpEdBlock.vue` — props `{ config: OpEdConfig }`; resolve each `theme_ids[i]` via `themesApi.get`, render `.tc` cards (cover + OP/ED badge + play + song/anime). Missing/failed themes skipped.
  - `CompatibilityBlock.vue` — props `{ userId, isOwner }`; if `isOwner` render nothing; else `onMounted` `getCompatibility(userId)`, unwrap; if `self` or error ⇒ render nothing; render `.compat` ring (conic by `percent`) + shared count. (`shared_sample` poster thumbs optional — render if cheap via existing anime fetch, else show count only.)
  - DS tokens; honor reduced-motion.
- [ ] Step 4: Run → PASS (+ tsc).
- [ ] Step 5: Commit (both SFCs + specs + i18n keys `showcase.block.op_ed`/`compatibility` + labels in en/ru/ja).

---

## Task 12: Editor — variant picker, Auto buttons, new add-block entries

**Files:**
- Modify: `frontend/web/src/components/profile/showcase/ShowcaseEditor.vue`
- Test: `ShowcaseEditor.spec.ts` (extend)

**Interfaces:** Consumes `SHOWCASE_VARIANTS`, `defaultVariant` (Task 4). Emits the same `save`(ShowcaseBlock[]) — now each block carries `variant`.

- [ ] Step 1: Failing tests:
  - Adding a block sets `variant = defaultVariant(type)`.
  - Changing the variant control updates `block.variant` and the saved payload reflects it.
  - The Auto button on a `favorite_anime` block fills `config.anime_ids` (mock the owner's top-rated list source) and on `card_collection` fills `config.card_ids`.
  - The add-block menu lists the 4 new types.
- [ ] Step 2: Run → FAIL.
- [ ] Step 3: Implement:
  - Add the 4 new types to the `ADDABLE` list; for auto types (`continue_watching`/`anime_dna`/`compatibility`) push a block with empty config + default variant and NO picker; for `op_ed` show a theme picker (reuse a simple id-list input or the themes search if available).
  - Add a **variant `<select>`/segmented control** per block row showing `SHOWCASE_VARIANTS[block.type]` (skip when only one variant).
  - Add an **"Авто" button** on `favorite_anime` and `card_collection` rows: anime → fill `anime_ids` from the owner's top-N by score (reuse `userApi.getWatchlist` sorted by score, take top 12 ids); cards → fill `card_ids` from `gachaApi.getCollection` (rarest/newest first, top 12).
  - On save, ensure each block has a `variant` (default when unset).
  - DS tokens; respect the player-picker native-control DS exemption only if inside `components/player/` (this is not — use the `Select` primitive or a `bespoke-keep` justification if a native select is needed).
- [ ] Step 4: Run → PASS (+ tsc + DS-lint + i18n-lint).
- [ ] Step 5: Commit (editor + spec + i18n keys for variant labels + "Авто" in en/ru/ja).

---

## Task 13: Full sweep, deploy, verify, document

**Files:** none (deploy + changelog).

- [ ] Step 1: Backend sweep — `cd services/player && go test ./... && cd ../gateway && go test ./...` (the pre-existing MAL-export network test failure is unrelated). Expected: PASS.
- [ ] Step 2: Frontend sweep (from `frontend/web`) — `bunx vitest run && bunx tsc --noEmit && bash scripts/design-system-lint.sh && bash scripts/i18n-lint.sh`. Expected: all PASS / DS-lint ERRORS=0 for showcase files.
- [ ] Step 3: Deploy from a CLEAN `origin/main` worktree (project practice — never the dirty shared tree): `bun install` in the worktree, then `make redeploy-player`, `make redeploy-gateway`, `make redeploy-web`. Verify health.
- [ ] Step 4: Live gate check — unauthenticated `GET /api/users/<uuid>/compatibility` → 401 (route wired + JWT-gated); showcase still admin-only (dark-shipped). v2 ships hidden, releases with the same 4 flags as v1.
- [ ] Step 5: Invoke `/animeenigma-after-update` for the changelog entry (note: feature still dark-shipped — keep the entry internal/omit public-facing announcement, consistent with v1). Commit/push handled per task.

---

## Self-Review

**Spec coverage (v2 section):**
- `variant` field + per-type allowlist → Task 1 (BE) + Task 4 (FE). ✓
- All listed variants → Tasks 5–9 (about/anime/characters/cards/stats), minus `heatmap` (deferred, documented). ✓
- New blocks `continue_watching`/`anime_dna` → Task 10; `op_ed`/`compatibility` → Task 11. ✓
- Curation model (manual + Auto buttons; auto blocks) → Task 12 (Auto buttons, add-block menu) + per-block resolution (Tasks 10/11). ✓
- Compatibility blend endpoint (0.5/0.4/0.1, pairwise, privacy, own-profile hidden) → Tasks 2–3 (math + endpoint) + Task 11 (own-profile hide). ✓
- Dark-ship same gate → Task 3 Step 4. ✓
- Motion contract + card dialog + tilt → Task 9 + Tasks 10/11 (reduced-motion). ✓
- Editor variant picker + Auto + new types → Task 12. ✓
- i18n tri-locale → folded into Tasks 10/11/12 + verified Task 13. ✓

**Deliberate deviations (documented in Global Constraints):**
- `stats.heatmap` deferred (needs daily-watch aggregate). ✓
- `continue_watching` uses public watching list (no precise progress bar; no new endpoint). ✓

**Placeholder scan:** Variant CSS is referenced from committed asset files (real, complete source — not a placeholder). All structural/backend code is inline. Tasks 5–9 share one explicit pattern (repeated intentionally per the no-"similar to Task N" rule the pattern is restated, and each task names its asset + variants).

**Type consistency:** `variant` string values identical in Go `VariantAllowlist` (Task 1) and TS `SHOWCASE_VARIANTS` (Task 4). New block-type strings identical across Go consts, TS literals, container dispatch, editor. `getCompatibility` shape matches the handler's `{percent,shared_count,shared_sample,self?}`. `CompatibilityService.Compute(viewerID, ownerID)` consistent across Tasks 2/3.
