# Profile Showcase Wall Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Steam-style, owner-editable profile showcase ("стена") with drag-and-drop blocks, dark-shipped admin-only so it releases bundled with Gacha.

**Architecture:** A thin CRUD slice on `services/player` stores the showcase **layout config** (an ordered list of blocks) as a single `jsonb` column, one row per user. The backend is a pure config store with structural validation only; **all block content (anime posters, characters, gacha cards, stats) is resolved on the frontend** via existing public APIs — no new internal player→catalog/gacha coupling. A gateway env flag plus a frontend gate hide the feature behind admin-only access (mirror of the Gacha «Лудка» dark-ship).

**Tech Stack:** Go (chi, GORM, SQLite for tests), Vue 3 + TypeScript, `vuedraggable` (SortableJS), Vitest.

## Global Constraints

These apply to **every** task implicitly:

- **Backend is a pure config store.** The showcase GET/PUT endpoints store and return the block array only. Stats and all referenced content resolve on the FRONTEND via existing public APIs. Backend validation is **structural only** (type, counts, lengths, UUID format) — NO ownership/visibility checks, NO cross-service calls. This is the locked resolution of the spec's coupling-vs-validation tension.
- **No new Go dependency for JSON.** `gorm.io/datatypes` is NOT a player dependency. Store `Blocks` as a Go `string` with GORM tag `gorm:"type:jsonb"`; marshal/unmarshal with `encoding/json`. Block `Config` is `json.RawMessage`.
- **Dark-ship env flags (default hidden):**
  - Gateway: `PROFILE_WALL_ADMIN_ONLY` (bool, default `true`).
  - Frontend: `VITE_PROFILE_WALL_ADMIN_ONLY` (string, default = hidden unless literal `'false'`).
- **Block limits:** ≤ 12 blocks total; ≤ 12 ids per `favorite_anime`/`favorite_character`/`card_collection`; `about` title ≤ 64 runes, text ≤ 2000 runes. Duplicate block types allowed.
- **i18n:** every new key added to ALL THREE locales — `en.json`, `ru.json`, `ja.json`. `i18n-lint.sh` is a hard prereq of `make redeploy-web`.
- **Design system:** bind to semantic tokens; only `font-medium`/`font-semibold`; no off-palette Tailwind color classes; no hardcoded hex in `.vue`. `frontend/web/scripts/design-system-lint.sh` fails the build otherwise.
- **Commits:** path-scoped (`git commit <pathspec>`, never bare `git commit` — shared tree). Always append co-authors:
  ```
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
  Run `git show --stat HEAD` after every commit to confirm only your files landed.
- **Tests:** Go service/repo tests use in-memory SQLite with a raw-SQL schema (Postgres `jsonb`/`gen_random_uuid()`/`now()` defaults break SQLite AutoMigrate — mirror `service/comment_test.go`).

## File Structure

**Backend (`services/player`):**
- `internal/domain/showcase.go` — `ProfileShowcase`, `Block`, per-type config structs, constants, `ValidateBlocks`.
- `internal/repo/showcase.go` — `ShowcaseRepository` with `Get` / `Upsert`.
- `internal/service/showcase.go` — `ShowcaseService` with `GetShowcase` / `SaveShowcase`.
- `internal/handler/showcase.go` — `ShowcaseHandler` with `GetShowcase` / `SaveShowcase`.
- `internal/transport/router.go` — register routes (modify).
- `cmd/player-api/main.go` — AutoMigrate + DI + pass handler to `NewRouter` (modify).
- Tests: `internal/domain/showcase_test.go`, `internal/repo/showcase_test.go`, `internal/service/showcase_test.go`.

**Gateway (`services/gateway`):**
- `internal/config/config.go` — `ProfileWallAdminOnly` (modify).
- `internal/transport/router.go` — showcase route group (modify).
- Test: `internal/config/config_test.go` (modify/add).

**Frontend (`frontend/web`):**
- `src/utils/profileWallGate.ts` — gate (mirror `gachaGate.ts`).
- `src/types/showcase.ts` — block TypeScript types.
- `src/api/client.ts` — `showcaseApi` methods (modify).
- `src/components/profile/showcase/ProfileShowcase.vue` — read container.
- `src/components/profile/showcase/blocks/AboutBlock.vue`
- `src/components/profile/showcase/blocks/FavoriteAnimeBlock.vue`
- `src/components/profile/showcase/blocks/StatsBlock.vue`
- `src/components/profile/showcase/blocks/FavoriteCharacterBlock.vue`
- `src/components/profile/showcase/blocks/CardCollectionBlock.vue`
- `src/components/profile/showcase/ShowcaseEditor.vue` — owner editor.
- `src/views/Profile.vue` — embed (modify).
- `src/locales/{en,ru,ja}.json` — `showcase.*` namespace (modify).
- `vite.config.ts` / package — add `vuedraggable` + manualChunks (modify).
- Tests: `.spec.ts` co-located per component + `src/utils/__tests__/profileWallGate.spec.ts`.

---

## Task 1: Backend domain model + block validator

**Files:**
- Create: `services/player/internal/domain/showcase.go`
- Test: `services/player/internal/domain/showcase_test.go`

**Interfaces:**
- Produces:
  - `type ProfileShowcase struct { UserID string; Blocks string; UpdatedAt time.Time }`, `func (ProfileShowcase) TableName() string`
  - `type Block struct { Type string; Order int; Config json.RawMessage }`
  - Constants: `BlockAbout`, `BlockFavoriteAnime`, `BlockStats`, `BlockFavoriteCharacter`, `BlockCardCollection` (string values `"about"`, `"favorite_anime"`, `"stats"`, `"favorite_character"`, `"card_collection"`); `MaxBlocks = 12`, `MaxBlockItems = 12`, `MaxAboutTitle = 64`, `MaxAboutText = 2000`.
  - `func ValidateBlocks(blocks []Block) error` — returns a `libs/errors` BadRequest on any violation, `nil` otherwise.

- [ ] **Step 1: Write the failing test**

Create `services/player/internal/domain/showcase_test.go`:

```go
package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func cfg(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return b
}

func TestValidateBlocks_OK(t *testing.T) {
	blocks := []Block{
		{Type: BlockAbout, Order: 0, Config: cfg(t, map[string]string{"title": "Hi", "text": "about me"})},
		{Type: BlockStats, Order: 1, Config: cfg(t, map[string]any{})},
		{Type: BlockFavoriteAnime, Order: 2, Config: cfg(t, map[string][]string{"anime_ids": {"a1", "a2"}})},
	}
	if err := ValidateBlocks(blocks); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateBlocks_UnknownType(t *testing.T) {
	if err := ValidateBlocks([]Block{{Type: "bogus", Order: 0, Config: cfg(t, map[string]any{})}}); err == nil {
		t.Fatal("expected error for unknown block type")
	}
}

func TestValidateBlocks_TooManyBlocks(t *testing.T) {
	blocks := make([]Block, MaxBlocks+1)
	for i := range blocks {
		blocks[i] = Block{Type: BlockStats, Order: i, Config: cfg(t, map[string]any{})}
	}
	if err := ValidateBlocks(blocks); err == nil {
		t.Fatal("expected error when exceeding MaxBlocks")
	}
}

func TestValidateBlocks_TooManyItems(t *testing.T) {
	ids := make([]string, MaxBlockItems+1)
	for i := range ids {
		ids[i] = "id"
	}
	b := []Block{{Type: BlockFavoriteAnime, Order: 0, Config: cfg(t, map[string][]string{"anime_ids": ids})}}
	if err := ValidateBlocks(b); err == nil {
		t.Fatal("expected error when exceeding MaxBlockItems")
	}
}

func TestValidateBlocks_AboutTooLong(t *testing.T) {
	b := []Block{{Type: BlockAbout, Order: 0, Config: cfg(t, map[string]string{"text": strings.Repeat("x", MaxAboutText+1)})}}
	if err := ValidateBlocks(b); err == nil {
		t.Fatal("expected error for over-long about text")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/player && go test ./internal/domain/ -run TestValidateBlocks -v`
Expected: FAIL — compile error (`ProfileShowcase`/`Block`/`ValidateBlocks` undefined).

- [ ] **Step 3: Write minimal implementation**

Create `services/player/internal/domain/showcase.go`:

```go
package domain

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/ILITA-hub/animeenigma/libs/errors"
)

// ProfileShowcase is the Steam-style customizable profile "wall". One row
// per user. Blocks holds an ordered JSON array of Block (jsonb on Postgres,
// TEXT on SQLite in tests). The backend is a pure config store: content
// (posters, characters, cards, stats) is resolved on the frontend.
type ProfileShowcase struct {
	UserID    string    `gorm:"type:uuid;primaryKey" json:"user_id"`
	Blocks    string    `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	UpdatedAt time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (ProfileShowcase) TableName() string { return "profile_showcases" }

// Block is one showcase block. Config is type-specific raw JSON, validated
// structurally by ValidateBlocks.
type Block struct {
	Type   string          `json:"type"`
	Order  int             `json:"order"`
	Config json.RawMessage `json:"config"`
}

const (
	BlockAbout             = "about"
	BlockFavoriteAnime     = "favorite_anime"
	BlockStats             = "stats"
	BlockFavoriteCharacter = "favorite_character"
	BlockCardCollection    = "card_collection"

	MaxBlocks     = 12
	MaxBlockItems = 12
	MaxAboutTitle = 64
	MaxAboutText  = 2000
)

type aboutConfig struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}
type idListConfig struct {
	AnimeIDs     []string `json:"anime_ids"`
	CardIDs      []string `json:"card_ids"`
	CharacterIDs []int    `json:"character_ids"`
}

// ValidateBlocks enforces structural limits only (no ownership/visibility,
// no cross-service calls — see plan Global Constraints).
func ValidateBlocks(blocks []Block) error {
	if len(blocks) > MaxBlocks {
		return errors.BadRequest("too many showcase blocks")
	}
	for _, b := range blocks {
		switch b.Type {
		case BlockStats:
			// no config required
		case BlockAbout:
			var c aboutConfig
			if err := json.Unmarshal(b.Config, &c); err != nil {
				return errors.BadRequest("invalid about config")
			}
			if utf8.RuneCountInString(c.Title) > MaxAboutTitle {
				return errors.BadRequest("about title too long")
			}
			if utf8.RuneCountInString(c.Text) > MaxAboutText {
				return errors.BadRequest("about text too long")
			}
		case BlockFavoriteAnime, BlockCardCollection, BlockFavoriteCharacter:
			var c idListConfig
			if err := json.Unmarshal(b.Config, &c); err != nil {
				return errors.BadRequest("invalid block config")
			}
			n := len(c.AnimeIDs) + len(c.CardIDs) + len(c.CharacterIDs)
			if n > MaxBlockItems {
				return errors.BadRequest("too many items in showcase block")
			}
		default:
			return errors.BadRequest("unknown showcase block type")
		}
	}
	return nil
}
```

> Note: confirm `errors.BadRequest` exists in `libs/errors` (it is used across the codebase). If the helper is named differently, use the project's canonical 400 constructor (`errors.New(errors.CodeBadRequest, msg)`).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/player && go test ./internal/domain/ -run TestValidateBlocks -v`
Expected: PASS (all 5 cases).

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/domain/showcase.go services/player/internal/domain/showcase_test.go
git commit services/player/internal/domain/showcase.go services/player/internal/domain/showcase_test.go -m "feat(player): profile showcase domain model + block validator

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -8
```

---

## Task 2: Backend repository (Get / Upsert)

**Files:**
- Create: `services/player/internal/repo/showcase.go`
- Test: `services/player/internal/repo/showcase_test.go`

**Interfaces:**
- Consumes: `domain.ProfileShowcase` (Task 1).
- Produces:
  - `type ShowcaseRepository struct{ db *gorm.DB }`
  - `func NewShowcaseRepository(db *gorm.DB) *ShowcaseRepository`
  - `func (r *ShowcaseRepository) Get(ctx context.Context, userID string) (*domain.ProfileShowcase, error)` — returns `&{UserID, Blocks: "[]"}` (NOT an error) when no row exists.
  - `func (r *ShowcaseRepository) Upsert(ctx context.Context, userID, blocksJSON string) error`

- [ ] **Step 1: Write the failing test**

Create `services/player/internal/repo/showcase_test.go`:

```go
package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupShowcaseRepoDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE profile_showcases (
		user_id TEXT PRIMARY KEY,
		blocks TEXT NOT NULL DEFAULT '[]',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`).Error)
	return db
}

func TestShowcaseRepo_GetEmpty(t *testing.T) {
	repo := NewShowcaseRepository(setupShowcaseRepoDB(t))
	got, err := repo.Get(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, "u1", got.UserID)
	require.Equal(t, "[]", got.Blocks)
}

func TestShowcaseRepo_UpsertThenGet(t *testing.T) {
	repo := NewShowcaseRepository(setupShowcaseRepoDB(t))
	ctx := context.Background()
	require.NoError(t, repo.Upsert(ctx, "u1", `[{"type":"about","order":0,"config":{}}]`))
	got, err := repo.Get(ctx, "u1")
	require.NoError(t, err)
	require.Contains(t, got.Blocks, `"about"`)

	// second upsert replaces
	require.NoError(t, repo.Upsert(ctx, "u1", `[{"type":"stats","order":0,"config":{}}]`))
	got2, err := repo.Get(ctx, "u1")
	require.NoError(t, err)
	require.Contains(t, got2.Blocks, `"stats"`)
	require.NotContains(t, got2.Blocks, `"about"`)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/player && go test ./internal/repo/ -run TestShowcaseRepo -v`
Expected: FAIL — `NewShowcaseRepository` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `services/player/internal/repo/showcase.go`:

```go
package repo

import (
	"context"
	stderrors "errors"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ShowcaseRepository is the data-access layer for profile_showcases.
type ShowcaseRepository struct {
	db *gorm.DB
}

func NewShowcaseRepository(db *gorm.DB) *ShowcaseRepository {
	return &ShowcaseRepository{db: db}
}

// Get returns the user's showcase, or an empty (Blocks="[]") showcase when
// no row exists yet — never a NotFound error.
func (r *ShowcaseRepository) Get(ctx context.Context, userID string) (*domain.ProfileShowcase, error) {
	var s domain.ProfileShowcase
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&s).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return &domain.ProfileShowcase{UserID: userID, Blocks: "[]"}, nil
		}
		return nil, errors.Wrap(err, errors.CodeInternal, "failed to load showcase")
	}
	return &s, nil
}

// Upsert writes the full blocks JSON for a user (insert or replace).
func (r *ShowcaseRepository) Upsert(ctx context.Context, userID, blocksJSON string) error {
	row := domain.ProfileShowcase{UserID: userID, Blocks: blocksJSON}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"blocks", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "failed to upsert showcase")
	}
	return nil
}
```

> Note: `updated_at` is set by the DB default on insert. On conflict the
> `updated_at` column is in `DoUpdates` but its value comes from the struct
> zero-time unless refreshed — acceptable for this feature (the frontend does
> not display showcase mtime in v1). If a fresh timestamp is required later,
> add `row.UpdatedAt = time.Now()` before Create.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/player && go test ./internal/repo/ -run TestShowcaseRepo -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/repo/showcase.go services/player/internal/repo/showcase_test.go
git commit services/player/internal/repo/showcase.go services/player/internal/repo/showcase_test.go -m "feat(player): profile showcase repository (Get/Upsert)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -8
```

---

## Task 3: Backend service (GetShowcase / SaveShowcase)

**Files:**
- Create: `services/player/internal/service/showcase.go`
- Test: `services/player/internal/service/showcase_test.go`

**Interfaces:**
- Consumes: `domain.Block`, `domain.ValidateBlocks` (Task 1); `ShowcaseRepository` (Task 2).
- Produces:
  - `type ShowcaseService struct{ ... }`
  - `func NewShowcaseService(repo *repo.ShowcaseRepository, log *logger.Logger) *ShowcaseService`
  - `func (s *ShowcaseService) GetShowcase(ctx context.Context, userID string) ([]domain.Block, error)` — returns parsed blocks sorted by `Order` ascending; empty slice when none.
  - `func (s *ShowcaseService) SaveShowcase(ctx context.Context, userID string, blocks []domain.Block) error` — validates then persists; re-numbers `Order` to the array index for canonical ordering.

- [ ] **Step 1: Write the failing test**

Create `services/player/internal/service/showcase_test.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupShowcaseService(t *testing.T) *ShowcaseService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE profile_showcases (
		user_id TEXT PRIMARY KEY,
		blocks TEXT NOT NULL DEFAULT '[]',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`).Error)
	return NewShowcaseService(repo.NewShowcaseRepository(db), logger.NewNop())
}

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestShowcaseService_SaveAndGet_SortsByOrder(t *testing.T) {
	svc := setupShowcaseService(t)
	ctx := context.Background()
	blocks := []domain.Block{
		{Type: domain.BlockStats, Order: 5, Config: raw(t, map[string]any{})},
		{Type: domain.BlockAbout, Order: 1, Config: raw(t, map[string]string{"text": "hi"})},
	}
	require.NoError(t, svc.SaveShowcase(ctx, "u1", blocks))

	got, err := svc.GetShowcase(ctx, "u1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	// re-numbered + sorted: about(0) before stats(1)
	require.Equal(t, domain.BlockAbout, got[0].Type)
	require.Equal(t, 0, got[0].Order)
	require.Equal(t, domain.BlockStats, got[1].Type)
	require.Equal(t, 1, got[1].Order)
}

func TestShowcaseService_GetEmpty(t *testing.T) {
	svc := setupShowcaseService(t)
	got, err := svc.GetShowcase(context.Background(), "nobody")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestShowcaseService_SaveRejectsInvalid(t *testing.T) {
	svc := setupShowcaseService(t)
	err := svc.SaveShowcase(context.Background(), "u1", []domain.Block{
		{Type: "bogus", Order: 0, Config: raw(t, map[string]any{})},
	})
	require.Error(t, err)
}
```

> If `logger.NewNop()` does not exist, use the package's standard test logger constructor (grep `logger.New` usages in existing `*_test.go`).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/player && go test ./internal/service/ -run TestShowcaseService -v`
Expected: FAIL — `NewShowcaseService` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `services/player/internal/service/showcase.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

// ShowcaseService is the business layer for the profile showcase. It is a
// pure config store: validation is structural only and no content is
// resolved here (the frontend resolves posters/characters/cards/stats).
type ShowcaseService struct {
	repo *repo.ShowcaseRepository
	log  *logger.Logger
}

func NewShowcaseService(r *repo.ShowcaseRepository, log *logger.Logger) *ShowcaseService {
	return &ShowcaseService{repo: r, log: log}
}

// GetShowcase returns the user's blocks sorted by Order ascending.
func (s *ShowcaseService) GetShowcase(ctx context.Context, userID string) ([]domain.Block, error) {
	row, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	var blocks []domain.Block
	if row.Blocks != "" {
		if err := json.Unmarshal([]byte(row.Blocks), &blocks); err != nil {
			// Corrupt config should not 500 the public profile — log + return empty.
			s.log.Errorw("failed to parse showcase blocks", "user_id", userID, "error", err)
			return []domain.Block{}, nil
		}
	}
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].Order < blocks[j].Order })
	return blocks, nil
}

// SaveShowcase validates, re-numbers Order to the array index, and persists.
func (s *ShowcaseService) SaveShowcase(ctx context.Context, userID string, blocks []domain.Block) error {
	if err := domain.ValidateBlocks(blocks); err != nil {
		return err
	}
	for i := range blocks {
		blocks[i].Order = i
	}
	encoded, err := json.Marshal(blocks)
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "failed to encode showcase blocks")
	}
	return s.repo.Upsert(ctx, userID, string(encoded))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/player && go test ./internal/service/ -run TestShowcaseService -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/service/showcase.go services/player/internal/service/showcase_test.go
git commit services/player/internal/service/showcase.go services/player/internal/service/showcase_test.go -m "feat(player): profile showcase service (get/save, validate, order)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -8
```

---

## Task 4: Backend handler + router + DI + migration

**Files:**
- Create: `services/player/internal/handler/showcase.go`
- Modify: `services/player/internal/transport/router.go`
- Modify: `services/player/cmd/player-api/main.go`

**Interfaces:**
- Consumes: `ShowcaseService` (Task 3).
- Produces:
  - `type ShowcaseHandler struct{ ... }`, `func NewShowcaseHandler(s *service.ShowcaseService, log *logger.Logger) *ShowcaseHandler`
  - `func (h *ShowcaseHandler) GetShowcase(w, r)` — `GET /api/users/{userId}/showcase` → `{ "blocks": [...] }`
  - `func (h *ShowcaseHandler) SaveShowcase(w, r)` — `PUT /api/users/me/showcase`, body `{ "blocks": [...] }`, owner from claims.
  - `NewRouter` gains a `showcaseHandler *handler.ShowcaseHandler` parameter.

- [ ] **Step 1: Write the handler**

Create `services/player/internal/handler/showcase.go`:

```go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
	"github.com/go-chi/chi/v5"
)

// ShowcaseHandler serves the profile showcase (Steam-style wall):
//
//	GET /api/users/{userId}/showcase   (public read)
//	PUT /api/users/me/showcase         (owner write, JWT)
type ShowcaseHandler struct {
	svc *service.ShowcaseService
	log *logger.Logger
}

func NewShowcaseHandler(s *service.ShowcaseService, log *logger.Logger) *ShowcaseHandler {
	return &ShowcaseHandler{svc: s, log: log}
}

type showcaseResponse struct {
	Blocks []domain.Block `json:"blocks"`
}

type saveShowcaseRequest struct {
	Blocks []domain.Block `json:"blocks"`
}

func (h *ShowcaseHandler) GetShowcase(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		httputil.BadRequest(w, "userId is required")
		return
	}
	blocks, err := h.svc.GetShowcase(r.Context(), userID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	if blocks == nil {
		blocks = []domain.Block{}
	}
	httputil.JSON(w, http.StatusOK, showcaseResponse{Blocks: blocks})
}

func (h *ShowcaseHandler) SaveShowcase(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}
	var req saveShowcaseRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if err := h.svc.SaveShowcase(r.Context(), claims.UserID, req.Blocks); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, showcaseResponse{Blocks: req.Blocks})
}
```

> Confirm the exact `httputil` response helpers (`JSON`, `Bind`, `BadRequest`, `Unauthorized`, `Error`) against `internal/handler/comment.go` — match whatever it uses. If `httputil.JSON` is named `httputil.OK`/`httputil.Respond`, use that.

- [ ] **Step 2: Wire the routes in `transport/router.go`**

Add `showcaseHandler *handler.ShowcaseHandler` to the `NewRouter` signature (place it right after `commentHandler *handler.CommentHandler` on line ~20).

Inside the protected `/users` group (after `r.Post("/watchlist/{animeId}/rewatch", ...)`, around line 97), add the owner write route:

```go
			// Profile showcase (Steam-style wall) — owner write. "me" resolves
			// to the JWT claims user id in the handler.
			r.Put("/me/showcase", showcaseHandler.SaveShowcase)
```

Next to the other public user routes (after `r.Get("/users/{userId}/watchlist/facets", ...)`, around line 191), add the public read route:

```go
		// Profile showcase public read (mirrors watchlist/public — lives
		// OUTSIDE the JWT-protected /users group so anonymous viewers can
		// read a profile's showcase once the dark-ship gate is lifted).
		r.Get("/users/{userId}/showcase", showcaseHandler.GetShowcase)
```

- [ ] **Step 3: Wire DI + migration in `cmd/player-api/main.go`**

Add to the `db.AutoMigrate(...)` list (next to `&domain.Comment{}`, line ~83):

```go
		&domain.ProfileShowcase{},
```

After the comment wiring (line ~347), add:

```go
	// Profile showcase (Steam-style wall, dark-shipped via gateway
	// PROFILE_WALL_ADMIN_ONLY). Pure config store; content resolved on FE.
	showcaseRepo := repo.NewShowcaseRepository(db.DB)
	showcaseService := service.NewShowcaseService(showcaseRepo, log)
	showcaseHandler := handler.NewShowcaseHandler(showcaseService, log)
```

Update the `transport.NewRouter(...)` call (line ~392) to pass `showcaseHandler` in the same position you added it to the signature (right after `commentHandler`):

```go
	router := transport.NewRouter(progressHandler, listHandler, historyHandler, reviewHandler, commentHandler, showcaseHandler, malImportHandler, malExportHandler, shikimoriImportHandler, reportHandler, syncHandler, activityHandler, exportHandler, prefHandler, overrideHandler, adminReportsHandler, internalListHandler, viewerContextHandler, cfg.JWT, log, metricsCollector)
```

- [ ] **Step 4: Build + run the full player test suite**

Run: `cd services/player && go build ./... && go test ./internal/...`
Expected: build OK; all tests PASS (domain/repo/service showcase tests included).

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/handler/showcase.go services/player/internal/transport/router.go services/player/cmd/player-api/main.go
git commit services/player/internal/handler/showcase.go services/player/internal/transport/router.go services/player/cmd/player-api/main.go -m "feat(player): wire profile showcase handler, routes, migration

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -10
```

---

## Task 5: Gateway dark-ship gate (config flag + routing)

**Files:**
- Modify: `services/gateway/internal/config/config.go`
- Modify: `services/gateway/internal/transport/router.go`
- Test: `services/gateway/internal/config/config_test.go` (add a case)

**Interfaces:**
- Produces: `Config.ProfileWallAdminOnly bool` (env `PROFILE_WALL_ADMIN_ONLY`, default `true`).
- Consumes: existing `JWTValidationMiddleware`, `OptionalJWTValidationMiddleware`, `AdminRoleMiddleware`, `userRateLimit`, `proxyHandler.ProxyToPlayer`.

- [ ] **Step 1: Add the config field + loader**

In `services/gateway/internal/config/config.go`, mirror `GachaAdminOnly`. Add to the `Config` struct (next to `GachaAdminOnly`):

```go
	// ProfileWallAdminOnly is the profile-showcase ("стена") dark-ship gate.
	// When true, the /api/users/{id}/showcase routes additionally require the
	// admin role, so the showcase is invisible to regular users until the
	// bundled release. Flip GACHA_ADMIN_ONLY + PROFILE_WALL_ADMIN_ONLY=false
	// together to reveal both. Default true.
	ProfileWallAdminOnly bool
```

In `config.Load()`, next to where `GachaAdminOnly` is parsed, add (match the existing bool-env helper used for `GachaAdminOnly`; the default must be `true` when unset):

```go
	cfg.ProfileWallAdminOnly = getEnvBool("PROFILE_WALL_ADMIN_ONLY", true)
```

> Use the SAME helper `GachaAdminOnly` uses (grep `GACHA_ADMIN_ONLY` in config.go to copy the exact parse call — it may be `getEnvBool`, `parseBoolEnv`, or inline). Default `true`.

- [ ] **Step 2: Write the failing config test**

In `services/gateway/internal/config/config_test.go`, add:

```go
func TestProfileWallAdminOnly_DefaultsTrue(t *testing.T) {
	t.Setenv("PROFILE_WALL_ADMIN_ONLY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.ProfileWallAdminOnly {
		t.Fatal("expected ProfileWallAdminOnly to default true")
	}
}

func TestProfileWallAdminOnly_FalseWhenSet(t *testing.T) {
	t.Setenv("PROFILE_WALL_ADMIN_ONLY", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProfileWallAdminOnly {
		t.Fatal("expected ProfileWallAdminOnly false when env=false")
	}
}
```

> If `Load()` requires other mandatory env vars (JWT secret etc.), copy the existing `GachaAdminOnly` test's setup so `Load()` succeeds.

- [ ] **Step 3: Run test to verify it passes (after Step 1)**

Run: `cd services/gateway && go test ./internal/config/ -run TestProfileWallAdminOnly -v`
Expected: PASS (the field + loader from Step 1 satisfy it).

- [ ] **Step 4: Register the routes in `transport/router.go`**

Add this BEFORE the protected `/users/*` group (currently lines ~412-418), so chi matches the specific showcase routes first. Mirror the Gacha gate branch:

```go
		// Profile showcase ("стена") — dark-shipped behind PROFILE_WALL_ADMIN_ONLY
		// (mirror of the Gacha gate). When admin-only, BOTH read and write require
		// JWT + admin. When open, read is public (OptionalJWT) and write falls
		// through to the protected /users/* group below. The player handler also
		// enforces owner-only writes from JWT claims (defense-in-depth).
		if cfg.ProfileWallAdminOnly {
			r.Group(func(r chi.Router) {
				r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Use(AdminRoleMiddleware)
				r.Get("/users/{userId}/showcase", proxyHandler.ProxyToPlayer)
				r.Put("/users/me/showcase", proxyHandler.ProxyToPlayer)
			})
		} else {
			r.Group(func(r chi.Router) {
				r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
				r.Use(userRateLimit)
				r.Get("/users/{userId}/showcase", proxyHandler.ProxyToPlayer)
			})
			// PUT /users/me/showcase falls through to the protected /users/* group.
		}
```

> Confirm the exact middleware constructor signatures by copying from the
> adjacent Gacha block (lines ~513-556) and the recs `OptionalJWTValidationMiddleware`
> usage (line ~379). Match them verbatim.

- [ ] **Step 5: Build + commit**

Run: `cd services/gateway && go build ./... && go test ./internal/config/`
Expected: build OK, config tests PASS.

```bash
git add services/gateway/internal/config/config.go services/gateway/internal/config/config_test.go services/gateway/internal/transport/router.go
git commit services/gateway/internal/config/config.go services/gateway/internal/config/config_test.go services/gateway/internal/transport/router.go -m "feat(gateway): PROFILE_WALL_ADMIN_ONLY dark-ship gate for profile showcase

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -8
```

---

## Task 6: Frontend gate + API client + types

**Files:**
- Create: `src/utils/profileWallGate.ts`
- Create: `src/types/showcase.ts`
- Modify: `src/api/client.ts`
- Test: `src/utils/__tests__/profileWallGate.spec.ts`

**Interfaces:**
- Produces:
  - `export const PROFILE_WALL_ADMIN_ONLY: boolean`
  - `export function useProfileWallVisible(): ComputedRef<boolean>`
  - `export type ShowcaseBlock = { type: ShowcaseBlockType; order: number; config: Record<string, unknown> }` and per-type config types.
  - `showcaseApi.getShowcase(userId)`, `showcaseApi.saveShowcase(blocks)`.

- [ ] **Step 1: Write the gate (mirror gachaGate.ts)**

Create `src/utils/profileWallGate.ts`:

```ts
/**
 * Profile showcase ("стена") visibility gate.
 *
 * VITE_PROFILE_WALL_ADMIN_ONLY defaults to TRUE (unset = admin-only dark-ship).
 * The bundled release flips it to 'false' to expose the feature to all users.
 * Mirror of utils/gachaGate.ts.
 */
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

/** True ⟹ only admins see the showcase; false ⟹ every authenticated user does. */
export const PROFILE_WALL_ADMIN_ONLY =
  (import.meta.env.VITE_PROFILE_WALL_ADMIN_ONLY as string | undefined) !== 'false'

/**
 * Reactive boolean: whether the current user should see the profile showcase
 * feature (read + the owner editor entry). When released this means "any
 * authenticated user"; during dark-ship it means "admins only".
 */
export function useProfileWallVisible() {
  const authStore = useAuthStore()
  return computed(() => {
    if (PROFILE_WALL_ADMIN_ONLY) return authStore.isAdmin
    return authStore.isAuthenticated
  })
}
```

- [ ] **Step 2: Write the types**

Create `src/types/showcase.ts`:

```ts
export type ShowcaseBlockType =
  | 'about'
  | 'favorite_anime'
  | 'stats'
  | 'favorite_character'
  | 'card_collection'

export interface AboutConfig {
  title?: string
  text?: string
}
export interface FavoriteAnimeConfig {
  anime_ids: string[]
}
export interface FavoriteCharacterConfig {
  character_ids: number[]
}
export interface CardCollectionConfig {
  card_ids: string[]
}
export type StatsConfig = Record<string, never>

export interface ShowcaseBlock {
  type: ShowcaseBlockType
  order: number
  config: AboutConfig | FavoriteAnimeConfig | FavoriteCharacterConfig | CardCollectionConfig | StatsConfig
}

export const MAX_SHOWCASE_BLOCKS = 12
export const MAX_SHOWCASE_ITEMS = 12
```

- [ ] **Step 3: Add the API client methods**

In `src/api/client.ts`, after the `publicApi` object (line ~566), add:

```ts
export const showcaseApi = {
  // Public read of a user's profile showcase blocks.
  getShowcase: (userId: string) =>
    apiClient.get<{ blocks: ShowcaseBlock[] } | { data: { blocks: ShowcaseBlock[] } }>(
      `/users/${userId}/showcase`,
    ),
  // Owner save (replaces the whole block array). "me" resolves to the JWT user.
  saveShowcase: (blocks: ShowcaseBlock[]) =>
    apiClient.put<{ blocks: ShowcaseBlock[] } | { data: { blocks: ShowcaseBlock[] } }>(
      `/users/me/showcase`,
      { blocks },
    ),
}
```

Add the import at the top of `client.ts` (with the other type imports):

```ts
import type { ShowcaseBlock } from '@/types/showcase'
```

> The player handler returns a bare `{ blocks }` (not the `{success,data}`
> envelope used by catalog/auth). The union return type above lets the
> consumer unwrap either shape — in components, read
> `('data' in res.data ? res.data.data : res.data).blocks`.

- [ ] **Step 4: Write the gate test**

Create `src/utils/__tests__/profileWallGate.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

describe('profileWallGate', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('hides for non-admin when admin-only (default)', async () => {
    vi.resetModules()
    const { useProfileWallVisible } = await import('@/utils/profileWallGate')
    const { useAuthStore } = await import('@/stores/auth')
    const auth = useAuthStore()
    auth.user = { is_admin: false, public_id: 'x' } as never
    const visible = useProfileWallVisible()
    // PROFILE_WALL_ADMIN_ONLY defaults true ⇒ non-admin sees nothing
    expect(visible.value).toBe(false)
  })
})
```

> Match `useAuthStore`'s admin getter (`isAdmin`) to how the store actually
> derives admin — copy the shape from `gachaGate`'s existing test if one
> exists, or set `auth.user` per the store's real fields. Keep the assertion
> on the default (admin-only) path, which is deterministic regardless of env.

- [ ] **Step 5: Run + commit**

Run: `bunx vitest run src/utils/__tests__/profileWallGate.spec.ts && bunx tsc --noEmit`
Expected: PASS, no type errors.

```bash
git add src/utils/profileWallGate.ts src/types/showcase.ts src/api/client.ts src/utils/__tests__/profileWallGate.spec.ts
git commit src/utils/profileWallGate.ts src/types/showcase.ts src/api/client.ts src/utils/__tests__/profileWallGate.spec.ts -m "feat(web): profile showcase gate, types, api client

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -8
```

> All `bunx`/`git` commands in frontend tasks run from `frontend/web`.

---

## Task 7: Read-view block components + i18n

**Files:**
- Create: `src/components/profile/showcase/blocks/AboutBlock.vue`
- Create: `src/components/profile/showcase/blocks/FavoriteAnimeBlock.vue`
- Create: `src/components/profile/showcase/blocks/StatsBlock.vue`
- Create: `src/components/profile/showcase/blocks/FavoriteCharacterBlock.vue`
- Create: `src/components/profile/showcase/blocks/CardCollectionBlock.vue`
- Modify: `src/locales/en.json`, `src/locales/ru.json`, `src/locales/ja.json`
- Test: `src/components/profile/showcase/blocks/__tests__/AboutBlock.spec.ts` (+ one per block, minimum render assertions)

**Interfaces:**
- Consumes: block config types (Task 6); existing `PosterCard` (anime), character card UI, gacha card display.
- Produces: 5 SFCs each accepting a typed `config` prop (and `userId` where needed for content resolution).

- [ ] **Step 1: Add i18n keys to all three locales**

Add a `showcase` namespace to `en.json`, `ru.json`, `ja.json`. Keys (same set, translated values):

en.json:
```json
"showcase": {
  "title": "Showcase",
  "edit": "Edit showcase",
  "save": "Save",
  "cancel": "Cancel",
  "add_block": "Add block",
  "remove_block": "Remove",
  "empty": "Nothing here yet.",
  "block": {
    "about": "About me",
    "favorite_anime": "Favorite anime",
    "stats": "Stats",
    "favorite_character": "Favorite characters",
    "card_collection": "Card collection"
  },
  "about_placeholder": "Tell others about yourself…",
  "about_title_placeholder": "Title",
  "pick_anime": "Pick anime",
  "pick_character": "Pick characters",
  "pick_cards": "Pick cards"
}
```

ru.json (Trump-mode NOT required here — plain UI copy):
```json
"showcase": {
  "title": "Витрина",
  "edit": "Редактировать витрину",
  "save": "Сохранить",
  "cancel": "Отмена",
  "add_block": "Добавить блок",
  "remove_block": "Убрать",
  "empty": "Здесь пока пусто.",
  "block": {
    "about": "Обо мне",
    "favorite_anime": "Любимое аниме",
    "stats": "Статистика",
    "favorite_character": "Любимые персонажи",
    "card_collection": "Коллекция карточек"
  },
  "about_placeholder": "Расскажите о себе…",
  "about_title_placeholder": "Заголовок",
  "pick_anime": "Выбрать аниме",
  "pick_character": "Выбрать персонажей",
  "pick_cards": "Выбрать карточки"
}
```

ja.json:
```json
"showcase": {
  "title": "ショーケース",
  "edit": "ショーケースを編集",
  "save": "保存",
  "cancel": "キャンセル",
  "add_block": "ブロックを追加",
  "remove_block": "削除",
  "empty": "まだ何もありません。",
  "block": {
    "about": "自己紹介",
    "favorite_anime": "お気に入りのアニメ",
    "stats": "統計",
    "favorite_character": "お気に入りのキャラクター",
    "card_collection": "カードコレクション"
  },
  "about_placeholder": "自己紹介を書いてください…",
  "about_title_placeholder": "タイトル",
  "pick_anime": "アニメを選ぶ",
  "pick_character": "キャラクターを選ぶ",
  "pick_cards": "カードを選ぶ"
}
```

Run: `bash scripts/i18n-lint.sh`
Expected: no missing-key errors for `showcase.*`.

- [ ] **Step 2: Write the AboutBlock test**

Create `src/components/profile/showcase/blocks/__tests__/AboutBlock.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AboutBlock from '../AboutBlock.vue'

const mountWith = (config: object) =>
  mount(AboutBlock, {
    props: { config },
    global: { mocks: { $t: (k: string) => k } },
  })

describe('AboutBlock', () => {
  it('renders the title and text', () => {
    const w = mountWith({ title: 'Hello', text: 'I like anime' })
    expect(w.text()).toContain('Hello')
    expect(w.text()).toContain('I like anime')
  })

  it('renders nothing fatal when empty', () => {
    const w = mountWith({})
    expect(w.exists()).toBe(true)
  })
})
```

- [ ] **Step 3: Implement AboutBlock**

Create `src/components/profile/showcase/blocks/AboutBlock.vue`:

```vue
<script setup lang="ts">
import type { AboutConfig } from '@/types/showcase'

defineProps<{ config: AboutConfig }>()
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 v-if="config.title" class="mb-2 text-lg font-semibold text-foreground">
      {{ config.title }}
    </h3>
    <p class="whitespace-pre-wrap text-sm text-muted-foreground">
      {{ config.text }}
    </p>
  </div>
</template>
```

- [ ] **Step 4: Implement the other four blocks**

`StatsBlock.vue` (resolves stats on the FE via existing public stats API):

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { publicApi } from '@/api/client'

const props = defineProps<{ userId: string }>()
const stats = ref<{ total?: number; avg_score?: number; episodes?: number; completed?: number }>({})

onMounted(async () => {
  try {
    const res = await publicApi.getPublicWatchlistStats(props.userId)
    stats.value = ('data' in res.data ? (res.data as { data: typeof stats.value }).data : res.data) as typeof stats.value
  } catch {
    stats.value = {}
  }
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.stats') }}</h3>
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <div class="text-center">
        <div class="text-xl font-semibold text-foreground">{{ stats.total ?? 0 }}</div>
        <div class="text-xs text-muted-foreground">{{ $t('profile.stats.total') }}</div>
      </div>
      <div class="text-center">
        <div class="text-xl font-semibold text-foreground">{{ stats.avg_score ?? 0 }}</div>
        <div class="text-xs text-muted-foreground">{{ $t('profile.stats.avg_score') }}</div>
      </div>
      <div class="text-center">
        <div class="text-xl font-semibold text-foreground">{{ stats.episodes ?? 0 }}</div>
        <div class="text-xs text-muted-foreground">{{ $t('profile.stats.episodes') }}</div>
      </div>
      <div class="text-center">
        <div class="text-xl font-semibold text-foreground">{{ stats.completed ?? 0 }}</div>
        <div class="text-xs text-muted-foreground">{{ $t('profile.stats.completed') }}</div>
      </div>
    </div>
  </div>
</template>
```

> Reuse the EXACT i18n keys Profile.vue already uses for these stat labels
> (grep `profile.stats` in `Profile.vue`/locales; if the keys differ, use the
> real ones — do NOT invent new stat-label keys).

`FavoriteAnimeBlock.vue` — resolve posters via the public watchlist/anime API the profile already uses, render with the existing `PosterCard`:

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { FavoriteAnimeConfig } from '@/types/showcase'
import { animeApi } from '@/api/client'
import PosterCard from '@/components/anime/PosterCard.vue'

const props = defineProps<{ config: FavoriteAnimeConfig }>()
const items = ref<Array<{ id: string }>>([])

onMounted(async () => {
  const ids = props.config.anime_ids ?? []
  if (!ids.length) return
  try {
    // Resolve each id to a card via the existing public anime endpoint.
    const results = await Promise.all(
      ids.map((id) => animeApi.getById(id).then((r) => ('data' in r.data ? r.data.data : r.data)).catch(() => null)),
    )
    items.value = results.filter(Boolean) as Array<{ id: string }>
  } catch {
    items.value = []
  }
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.favorite_anime') }}</h3>
    <div class="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6">
      <PosterCard v-for="a in items" :key="a.id" :anime="a" />
    </div>
  </div>
</template>
```

> Confirm `animeApi.getById` (or the equivalent the anime page uses) and the
> `PosterCard` prop name (`:anime`) from `components/anime/PosterCard.vue`.
> Match them exactly. If a batch endpoint exists (e.g. an ids→anime fetch),
> prefer it over N requests.

`FavoriteCharacterBlock.vue` — resolve via the characters feature endpoint (`/api/characters/{shikimoriId}`), reuse the character card UI from the characters page:

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { FavoriteCharacterConfig } from '@/types/showcase'
import { apiClient } from '@/api/client'

const props = defineProps<{ config: FavoriteCharacterConfig }>()
const items = ref<Array<{ id: number; name: string; image?: string }>>([])

onMounted(async () => {
  const ids = props.config.character_ids ?? []
  if (!ids.length) return
  const results = await Promise.all(
    ids.map((id) =>
      apiClient
        .get(`/characters/${id}`)
        .then((r) => ('data' in r.data ? r.data.data : r.data))
        .catch(() => null),
    ),
  )
  items.value = results.filter(Boolean) as typeof items.value
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.favorite_character') }}</h3>
    <div class="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6">
      <RouterLink
        v-for="c in items"
        :key="c.id"
        :to="`/characters/${c.id}`"
        class="block text-center"
      >
        <img v-if="c.image" :src="c.image" :alt="c.name" class="aspect-[2/3] w-full rounded-lg object-cover" />
        <span class="mt-1 block truncate text-xs text-muted-foreground">{{ c.name }}</span>
      </RouterLink>
    </div>
  </div>
</template>
```

> Match the real character endpoint response field names (grep the characters
> page component, e.g. `FavoriteCharacterBlock` should reuse the SAME card
> component the `/characters/:id` and anime characters section render — prefer
> importing that component over re-implementing the markup).

`CardCollectionBlock.vue` — resolve gacha cards via the existing gacha API used by `GachaCollection.vue`:

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { CardCollectionConfig } from '@/types/showcase'
import { gachaApi } from '@/api/gacha'

const props = defineProps<{ config: CardCollectionConfig; userId: string }>()
const cards = ref<Array<{ id: string; image_url?: string; name?: string }>>([])

onMounted(async () => {
  const ids = new Set(props.config.card_ids ?? [])
  if (!ids.size) return
  try {
    const res = await gachaApi.getCollection()
    const all = ('data' in res.data ? res.data.data : res.data) as Array<{ id: string; image_url?: string; name?: string }>
    cards.value = all.filter((c) => ids.has(c.id))
  } catch {
    cards.value = []
  }
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.card_collection') }}</h3>
    <div class="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6">
      <div v-for="c in cards" :key="c.id" class="text-center">
        <img v-if="c.image_url" :src="c.image_url" :alt="c.name" class="aspect-[2/3] w-full rounded-lg object-cover" />
        <span class="mt-1 block truncate text-xs text-muted-foreground">{{ c.name }}</span>
      </div>
    </div>
  </div>
</template>
```

> Confirm `gachaApi.getCollection` (or the method `GachaCollection.vue`
> calls) and the card field names (`id`, `image_url`, `name`) against
> `src/api/gacha.ts` + `components/profile/GachaCollection.vue`. Reuse the
> existing card display component if `GachaCollection.vue` has an extractable
> card SFC. Note: when viewing ANOTHER user's profile during dark-ship,
> `gachaApi.getCollection()` returns the VIEWER's cards, not the owner's — for
> v1 the card block is only meaningful on your own profile; the editor's card
> picker (Task 8) only runs for the owner. If a per-user public collection
> endpoint is needed later, add it then (YAGNI for v1).

Add a co-located minimal `.spec.ts` for each of the other four blocks asserting it mounts and renders its title key (mirror the AboutBlock test structure; mock network calls with `vi.mock` or rely on empty configs so `onMounted` no-ops).

- [ ] **Step 5: Run + commit**

Run: `bash scripts/i18n-lint.sh && bunx vitest run src/components/profile/showcase/ && bunx tsc --noEmit`
Expected: i18n OK, specs PASS, no type errors.

```bash
git add src/components/profile/showcase/blocks src/locales/en.json src/locales/ru.json src/locales/ja.json
git commit src/components/profile/showcase/blocks src/locales/en.json src/locales/ru.json src/locales/ja.json -m "feat(web): profile showcase read-view block components + i18n

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -12
```

---

## Task 8: Read container + drag-and-drop editor

**Files:**
- Create: `src/components/profile/showcase/ProfileShowcase.vue`
- Create: `src/components/profile/showcase/ShowcaseEditor.vue`
- Modify: `package.json` (add `vuedraggable`), `vite.config.ts` (manualChunks)
- Test: `src/components/profile/showcase/__tests__/ProfileShowcase.spec.ts`, `.../ShowcaseEditor.spec.ts`

**Interfaces:**
- Consumes: block components (Task 7); `showcaseApi`, `ShowcaseBlock` (Task 6); `useProfileWallVisible` (Task 6).
- Produces:
  - `ProfileShowcase.vue` — props `{ userId: string; isOwner: boolean }`; fetches + renders blocks; shows "Edit showcase" for owners; toggles `ShowcaseEditor`.
  - `ShowcaseEditor.vue` — props `{ userId: string; modelValue: ShowcaseBlock[] }`; emits `save` (ShowcaseBlock[]) and `cancel`; drag-reorders via `vuedraggable`.

- [ ] **Step 1: Add the dependency**

Run (from `frontend/web`): `bun add vuedraggable@next`
Expected: `vuedraggable` (Vue 3 build, wraps SortableJS) added to `package.json` dependencies.

In `vite.config.ts` `manualChunks`, pin it so it lazy-loads with the editor and doesn't bloat the main bundle (mirror how `icons`/`ui-vendor` are pinned):

```ts
// inside manualChunks
if (id.includes('vuedraggable') || id.includes('sortablejs')) return 'showcase-editor'
```

- [ ] **Step 2: Write the editor test**

Create `src/components/profile/showcase/__tests__/ShowcaseEditor.spec.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ShowcaseEditor from '../ShowcaseEditor.vue'
import type { ShowcaseBlock } from '@/types/showcase'

vi.mock('vuedraggable', () => ({
  default: {
    name: 'draggable',
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<div><slot v-for="el in modelValue" :element="el" name="item" /></div>',
  },
}))

const blocks: ShowcaseBlock[] = [
  { type: 'about', order: 0, config: { text: 'hi' } },
  { type: 'stats', order: 1, config: {} },
]

const mountEditor = () =>
  mount(ShowcaseEditor, {
    props: { userId: 'u1', modelValue: blocks },
    global: { mocks: { $t: (k: string) => k }, stubs: { teleport: true } },
  })

describe('ShowcaseEditor', () => {
  it('renders one row per block', () => {
    const w = mountEditor()
    expect(w.text()).toContain('showcase.block.about')
    expect(w.text()).toContain('showcase.block.stats')
  })

  it('emits save with re-numbered order on save', async () => {
    const w = mountEditor()
    await w.find('[data-test="showcase-save"]').trigger('click')
    const emitted = w.emitted('save')
    expect(emitted).toBeTruthy()
    const payload = emitted![0][0] as ShowcaseBlock[]
    expect(payload.map((b) => b.order)).toEqual([0, 1])
  })

  it('removes a block', async () => {
    const w = mountEditor()
    await w.find('[data-test="showcase-remove-0"]').trigger('click')
    await w.find('[data-test="showcase-save"]').trigger('click')
    const payload = w.emitted('save')![0][0] as ShowcaseBlock[]
    expect(payload).toHaveLength(1)
  })
})
```

- [ ] **Step 3: Implement the editor**

Create `src/components/profile/showcase/ShowcaseEditor.vue`:

```vue
<script setup lang="ts">
import { ref } from 'vue'
import draggable from 'vuedraggable'
import type { ShowcaseBlock, ShowcaseBlockType } from '@/types/showcase'
import { MAX_SHOWCASE_BLOCKS } from '@/types/showcase'

const props = defineProps<{ userId: string; modelValue: ShowcaseBlock[] }>()
const emit = defineEmits<{ save: [ShowcaseBlock[]]; cancel: [] }>()

const local = ref<ShowcaseBlock[]>(props.modelValue.map((b) => ({ ...b })))

const ADDABLE: ShowcaseBlockType[] = ['about', 'favorite_anime', 'stats', 'favorite_character', 'card_collection']

function addBlock(type: ShowcaseBlockType) {
  if (local.value.length >= MAX_SHOWCASE_BLOCKS) return
  const config = type === 'about' ? { title: '', text: '' } : {}
  local.value.push({ type, order: local.value.length, config })
}
function removeBlock(i: number) {
  local.value.splice(i, 1)
}
function save() {
  const renumbered = local.value.map((b, i) => ({ ...b, order: i }))
  emit('save', renumbered)
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center gap-2">
      <button
        v-for="t in ADDABLE"
        :key="t"
        type="button"
        class="rounded-lg border border-border px-3 py-1 text-sm font-medium text-foreground hover:bg-accent"
        @click="addBlock(t)"
      >
        + {{ $t(`showcase.block.${t}`) }}
      </button>
    </div>

    <draggable v-model="local" item-key="order" handle=".showcase-drag-handle">
      <template #item="{ element, index }">
        <div class="mb-3 rounded-xl border border-border bg-card p-3">
          <div class="mb-2 flex items-center justify-between">
            <span class="showcase-drag-handle cursor-grab text-sm font-semibold text-foreground">
              ⠿ {{ $t(`showcase.block.${element.type}`) }}
            </span>
            <button
              type="button"
              :data-test="`showcase-remove-${index}`"
              class="text-sm font-medium text-destructive"
              @click="removeBlock(index)"
            >
              {{ $t('showcase.remove_block') }}
            </button>
          </div>

          <!-- About block inline editor -->
          <div v-if="element.type === 'about'" class="space-y-2">
            <input
              v-model="(element.config as { title?: string }).title"
              :placeholder="$t('showcase.about_title_placeholder')"
              class="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
            />
            <textarea
              v-model="(element.config as { text?: string }).text"
              :placeholder="$t('showcase.about_placeholder')"
              rows="4"
              class="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
            />
          </div>
          <!-- Other block types: picker stubs wired to existing pickers -->
          <p v-else class="text-xs text-muted-foreground">
            {{ $t(`showcase.pick_${element.type === 'favorite_anime' ? 'anime' : element.type === 'favorite_character' ? 'character' : element.type === 'card_collection' ? 'cards' : 'anime'}`) }}
          </p>
        </div>
      </template>
    </draggable>

    <div class="flex gap-2">
      <button
        type="button"
        data-test="showcase-save"
        class="rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground"
        @click="save"
      >
        {{ $t('showcase.save') }}
      </button>
      <button
        type="button"
        class="rounded-lg border border-border px-4 py-2 text-sm font-medium text-foreground"
        @click="emit('cancel')"
      >
        {{ $t('showcase.cancel') }}
      </button>
    </div>
  </div>
</template>
```

> The anime/character/card pickers are stubbed as text hints in v1's editor.
> Implementing the actual multi-select pickers (reusing the watchlist search,
> the characters search, and the gacha collection grid as selectable lists)
> is the natural follow-up; the data model + save path are fully wired, so a
> picker just needs to mutate `element.config.{anime_ids|character_ids|card_ids}`.
> If you implement pickers now, reuse existing search components — do NOT build
> new search UIs. Keep this task's scope to drag-reorder + add/remove + about
> editing if the pickers risk blowing the task budget; note the deferral in
> the commit message.

- [ ] **Step 4: Implement the read container**

Create `src/components/profile/showcase/ProfileShowcase.vue`:

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { ShowcaseBlock } from '@/types/showcase'
import { showcaseApi } from '@/api/client'
import AboutBlock from './blocks/AboutBlock.vue'
import FavoriteAnimeBlock from './blocks/FavoriteAnimeBlock.vue'
import StatsBlock from './blocks/StatsBlock.vue'
import FavoriteCharacterBlock from './blocks/FavoriteCharacterBlock.vue'
import CardCollectionBlock from './blocks/CardCollectionBlock.vue'
import ShowcaseEditor from './ShowcaseEditor.vue'

const props = defineProps<{ userId: string; isOwner: boolean }>()

const blocks = ref<ShowcaseBlock[]>([])
const editing = ref(false)
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    const res = await showcaseApi.getShowcase(props.userId)
    const data = 'data' in res.data ? (res.data as { data: { blocks: ShowcaseBlock[] } }).data : res.data
    blocks.value = data.blocks ?? []
  } catch {
    blocks.value = []
  } finally {
    loading.value = false
  }
}

async function onSave(next: ShowcaseBlock[]) {
  await showcaseApi.saveShowcase(next)
  blocks.value = next
  editing.value = false
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-semibold text-foreground">{{ $t('showcase.title') }}</h2>
      <button
        v-if="isOwner && !editing"
        type="button"
        class="rounded-lg border border-border px-3 py-1 text-sm font-medium text-foreground hover:bg-accent"
        @click="editing = true"
      >
        {{ $t('showcase.edit') }}
      </button>
    </div>

    <ShowcaseEditor
      v-if="editing"
      :user-id="userId"
      :model-value="blocks"
      @save="onSave"
      @cancel="editing = false"
    />

    <template v-else>
      <p v-if="!loading && !blocks.length" class="text-sm text-muted-foreground">
        {{ $t('showcase.empty') }}
      </p>
      <template v-for="(b, i) in blocks" :key="i">
        <AboutBlock v-if="b.type === 'about'" :config="b.config as never" />
        <FavoriteAnimeBlock v-else-if="b.type === 'favorite_anime'" :config="b.config as never" />
        <StatsBlock v-else-if="b.type === 'stats'" :user-id="userId" />
        <FavoriteCharacterBlock v-else-if="b.type === 'favorite_character'" :config="b.config as never" />
        <CardCollectionBlock v-else-if="b.type === 'card_collection'" :config="b.config as never" :user-id="userId" />
      </template>
    </template>
  </section>
</template>
```

> Vue template rule (project memory): the `v-if`/`v-else-if` chain must have
> NO non-conditional elements between branches. The chain above is contiguous —
> keep it that way.

- [ ] **Step 5: Write the container test**

Create `src/components/profile/showcase/__tests__/ProfileShowcase.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

vi.mock('@/api/client', () => ({
  showcaseApi: {
    getShowcase: vi.fn().mockResolvedValue({ data: { blocks: [{ type: 'about', order: 0, config: { text: 'hi' } }] } }),
    saveShowcase: vi.fn().mockResolvedValue({ data: { blocks: [] } }),
  },
  publicApi: { getPublicWatchlistStats: vi.fn().mockResolvedValue({ data: {} }) },
  animeApi: { getById: vi.fn().mockResolvedValue({ data: {} }) },
  apiClient: { get: vi.fn().mockResolvedValue({ data: {} }) },
}))

import ProfileShowcase from '../ProfileShowcase.vue'

const mountSc = (isOwner: boolean) =>
  mount(ProfileShowcase, {
    props: { userId: 'u1', isOwner },
    global: { mocks: { $t: (k: string) => k }, stubs: { ShowcaseEditor: true } },
  })

describe('ProfileShowcase', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders fetched blocks', async () => {
    const w = mountSc(false)
    await flushPromises()
    expect(w.text()).toContain('hi')
  })

  it('shows edit button only for owner', async () => {
    const owner = mountSc(true)
    await flushPromises()
    expect(owner.text()).toContain('showcase.edit')
    const visitor = mountSc(false)
    await flushPromises()
    expect(visitor.text()).not.toContain('showcase.edit')
  })
})
```

- [ ] **Step 6: Run + commit**

Run: `bunx vitest run src/components/profile/showcase/ && bunx tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: specs PASS, no type errors, DS-lint ERRORS=0.

```bash
git add src/components/profile/showcase/ProfileShowcase.vue src/components/profile/showcase/ShowcaseEditor.vue src/components/profile/showcase/__tests__ package.json bun.lockb vite.config.ts
git commit src/components/profile/showcase/ProfileShowcase.vue src/components/profile/showcase/ShowcaseEditor.vue src/components/profile/showcase/__tests__ package.json bun.lockb vite.config.ts -m "feat(web): profile showcase container + drag-and-drop editor

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -12
```

> `bun.lockb` may be named `bun.lock` — add whichever the repo tracks.

---

## Task 9: Embed in Profile.vue behind the gate

**Files:**
- Modify: `src/views/Profile.vue`
- Test: extend an existing Profile spec OR add a focused mount assertion (see Step 2).

**Interfaces:**
- Consumes: `ProfileShowcase.vue` (Task 8); `useProfileWallVisible` (Task 6); the existing `isOwnProfile` computed and the viewed user's id in `Profile.vue`.

- [ ] **Step 1: Add the gate + component to Profile.vue**

In the `<script setup>` of `src/views/Profile.vue`, import and wire the gate (mirror the existing `useGachaVisible` usage on line ~853):

```ts
import ProfileShowcase from '@/components/profile/showcase/ProfileShowcase.vue'
import { useProfileWallVisible } from '@/utils/profileWallGate'
const profileWallVisible = useProfileWallVisible()
```

In the template, place the showcase prominently on the profile (e.g. directly under the profile header, before the tab strip). Use the id the page already resolves for public watchlist calls (grep the variable passed to `publicApi.getPublicWatchlist` — it is the viewed user's id; reuse it verbatim as `:user-id`):

```vue
        <ProfileShowcase
          v-if="profileWallVisible"
          :user-id="viewedUserId"
          :is-owner="isOwnProfile"
          class="mt-6"
        />
```

> Replace `viewedUserId` with the ACTUAL variable name Profile.vue uses for
> the viewed user's id (the one feeding `getPublicWatchlist`/`getPublicWatchlistStats`).
> Do NOT use `route.params.publicId` if that is a public_id distinct from the
> player-service user id — match what the existing public watchlist calls pass.

- [ ] **Step 2: Add a render-gate assertion**

If a `Profile.spec.ts` exists, add a case; otherwise create `src/views/__tests__/Profile.showcase.spec.ts` with a minimal mount that stubs heavy children and asserts `ProfileShowcase` is absent when the gate is closed. Because the gate depends on `import.meta.env` + admin state, the deterministic assertion is: with a non-admin user and default env, `ProfileShowcase` does not render.

```ts
import { describe, it, expect, vi } from 'vitest'
// Mock the gate to closed to keep the assertion deterministic.
vi.mock('@/utils/profileWallGate', () => ({
  PROFILE_WALL_ADMIN_ONLY: true,
  useProfileWallVisible: () => ({ value: false }),
}))
// ...mount Profile with the project's standard stubs and assert
// w.findComponent({ name: 'ProfileShowcase' }).exists() === false
```

> Mounting `Profile.vue` pulls a large dependency graph. If a canonical
> Profile test setup exists, copy its stubs/mocks. If standing up the full
> mount is disproportionate, assert at the `ProfileShowcase` level instead
> (already covered in Task 8) and verify the gate wiring manually in Task 10's
> deploy smoke — note the choice in the commit message.

- [ ] **Step 3: Run gates + commit**

Run: `bunx vitest run && bunx tsc --noEmit && bash scripts/design-system-lint.sh && bash scripts/i18n-lint.sh`
Expected: all PASS / ERRORS=0.

```bash
git add src/views/Profile.vue src/views/__tests__/Profile.showcase.spec.ts
git commit src/views/Profile.vue src/views/__tests__/Profile.showcase.spec.ts -m "feat(web): embed profile showcase in Profile.vue behind dark-ship gate

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -8
```

---

## Task 10: Deploy, verify, document

**Files:** none (deploy + changelog via the after-update skill).

- [ ] **Step 1: Full backend test sweep**

Run: `cd services/player && go test ./... && cd ../gateway && go test ./...`
Expected: all PASS.

- [ ] **Step 2: Full frontend gate sweep**

Run (from `frontend/web`): `bunx vitest run && bunx tsc --noEmit && bash scripts/design-system-lint.sh && bash scripts/i18n-lint.sh`
Expected: all PASS / ERRORS=0.

- [ ] **Step 3: Invoke the after-update skill**

Invoke `/animeenigma-after-update`. It redeploys `player`, `gateway`, and `web` (deploy from a clean `origin/main` worktree per project practice), runs health checks, prepends a Russian Trump-mode changelog entry, and pushes. The showcase ships **admin-only** (both env defaults are `true`) — verify on the live site that a non-admin profile shows NO showcase and an admin profile shows it with a working editor.

- [ ] **Step 4: Confirm the bundled-release toggle is documented**

Ensure the release runbook lists all four flags to flip together when revealing both features:
```
GACHA_ADMIN_ONLY=false
VITE_GACHA_ADMIN_ONLY=false
PROFILE_WALL_ADMIN_ONLY=false
VITE_PROFILE_WALL_ADMIN_ONLY=false
```
then `make restart-gateway` + `make redeploy-web`.

---

## Self-Review

**Spec coverage:**
- Steam-style owner-edited showcase, visitors read-only → Tasks 7–9 (read components + owner-only editor + `isOwner` gating). ✓
- 5 block types (about, favorite_anime, stats, favorite_character, card_collection) → Task 1 (domain), Task 7 (components). ✓
- Drag-and-drop reorder → Task 8 (`vuedraggable`). ✓
- Backend = player slice, JSONB config, content resolved on FE → Tasks 1–4 + Global Constraints. ✓
- Dark-ship gate (gateway flag + FE gate, default hidden) → Task 5 + Task 6. ✓
- Release bundled with Gacha → Task 10 Step 4. ✓
- i18n all three locales → Task 7 Step 1. ✓
- Tests (Go service/repo + Vitest per component + editor) → Tasks 1–3, 6–9. ✓

**Spec deviations (intentional, noted):**
- Spec said the service validates "ownership of cards, visibility of anime"; the plan drops these to keep the backend coupling-free (the spec ALSO prioritized "avoid new internal player→catalog/gacha coupling"). Ownership/visibility is enforced naturally at resolve time (pickers only offer owned/visible items; resolvers only render what the public APIs return). Documented in Global Constraints. ✓
- Spec said backend resolves `stats` inline; the plan resolves stats on the FE via the existing public stats API (Task 7 StatsBlock), making the backend a pure config store. Documented in Global Constraints. ✓

**Placeholder scan:** Editor pickers for anime/character/card are intentionally stubbed in Task 8 with an explicit deferral note and a fully-wired save path (not a hidden TODO) — the data contract is complete; pickers are a scoped follow-up. All other steps contain complete code.

**Type consistency:** `ShowcaseBlock`/`Block` shape (`type`/`order`/`config`) is consistent across domain (Go), types (TS), API, and components. `showcaseApi.getShowcase`/`saveShowcase` names match between Task 6 (def) and Tasks 7–9 (use). `useProfileWallVisible` consistent. `ValidateBlocks` consistent.
