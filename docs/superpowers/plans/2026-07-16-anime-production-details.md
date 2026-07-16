# Anime Production Details Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface who made an anime and when — production metadata (studio, aired→released dates, source, age rating, duration), key crew (director & co.), and the voice cast — on the anime page, behind a "Details" disclosure, with the cast attached to existing characters.

**Architecture:** Backend adds one nullable date column (`released_on`), one flat denormalized `anime_person_roles` table (role as a scalar column, person data inline — no Person entity, no join table), and an inline `seyu` JSON field on the existing `Character`. Three read paths: `GET /api/anime/{id}` (metadata, already served), a new `GET /api/anime/{id}/staff`, and the existing `GET /api/characters/{id}` (now carries seyu). Frontend renders metadata + staff inside a collapsible Details section on `Anime.vue`, and the voice cast on `Character.vue`.

**Tech Stack:** Go 1.x + GORM v2 + chi (catalog service), Vue 3 + TypeScript + Tailwind v4 + vue-i18n (frontend), Shikimori GraphQL (`personRoles`) + REST (`/api/characters/{id}` → `seyu`).

## Global Constraints

- **Worktree only.** All edits happen in `/data/animeenigma/.claude/worktrees/anime-production-details` (branch `feat/anime-production-details`). NEVER edit the base tree `/data/animeenigma`. Use relative paths or the worktree-absolute path — never a `/data/animeenigma/services/...` path (that writes the base tree).
- **Every commit carries all three co-authors**, verbatim:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **i18n parity gate:** every new key exists in `locales/en.json`, `locales/ru.json`, AND `locales/ja.json`. Missing any = build/verify failure.
- **DS-lint gate:** no off-palette color classes, no raw hex/rgba, no arbitrary spacing. Bind to semantic tokens. Brand hues `cyan pink orange rose indigo teal lime` are exempt. A `PostToolUse` hook runs the DS gate on every `frontend/web/src/**/*.{vue,ts}` edit.
- **Shikimori base URL is `https://shikimori.io`** (already configured — `config.go:178-179`). REST calls use `c.config.BaseURL`; GraphQL uses `c.config.GraphQLURL`.
- **GORM `AutoMigrate` is additive** — it creates new tables and adds new columns on restart but never drops/alters. `released_on`, the `seyu` column, and the `anime_person_roles` table all appear automatically on the next catalog boot.
- **Staff role whitelist** (canonical EN, ordered — this order is the display rank):
  `Director`, `Original Creator`, `Series Composition`, `Script`, `Character Design`, `Chief Animation Director`, `Art Director`, `Music`, `Sound Director`, `Director of Photography`, `Producer`, `Executive Producer`. Roles outside this set are dropped at parse time.
- **Voice cast costs one REST call per character** — surfaced on the character detail page only (`GetCharacterByID` path). Do NOT enrich the anime-page character rail.
- **No time-effort units** in any doc/changelog. Metrics are UXΔ / CDI / MVQ (see spec).
- Build the catalog service from the worktree: `cd services/catalog && go build ./... && go test ./...`. Frontend gate: `cd frontend/web && bash ../../bin/ae-fe-verify.sh <files>` (or `bun run build`).

---

## Task 1: Backend — `released_on` end date (metadata)

Adds the one metadata field not already on the wire, on all anime-fetch paths.

**Files:**
- Modify: `services/catalog/internal/domain/anime.go` (Anime struct, after `AiredOn`)
- Modify: `services/catalog/internal/parser/shikimori/client.go` (typed struct + raw struct + 4 raw query strings + both mappers)

**Interfaces:**
- Produces: `domain.Anime.ReleasedOn *time.Time` (json `released_on`), populated by `mapAnime` (typed) and `mapRawAnimeList` (raw).

- [ ] **Step 1: Add the domain field**

In `services/catalog/internal/domain/anime.go`, immediately after the `AiredOn` field (line ~87):

```go
	AiredOn           *time.Time `gorm:"index" json:"aired_on,omitempty"`
	// ReleasedOn is the air END date (Shikimori releasedOn). Nullable — ongoing
	// and single-cour titles often have none. Surfaced in the anime-page Details
	// block as the "aired → released" range.
	ReleasedOn        *time.Time `gorm:"index" json:"released_on,omitempty"`
```

- [ ] **Step 2: Add `releasedOn` to the typed GraphQL struct**

In `client.go`, in `type shikimoriAnime struct` (after `AiredOn` line ~133):

```go
	AiredOn       *shikimoriDate   `graphql:"airedOn"`
	ReleasedOn    *shikimoriDate   `graphql:"releasedOn"`
```

- [ ] **Step 3: Add `releasedOn` to the raw struct + all four raw query strings**

In `client.go`, in `type rawAnime struct` (after the `AiredOn` anonymous struct, line ~257):

```go
	AiredOn       *struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Day   int `json:"day"`
	} `json:"airedOn"`
	ReleasedOn    *struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Day   int `json:"day"`
	} `json:"releasedOn"`
```

Then in EACH of the four raw query strings (lines ~183, ~365, ~384, and the fourth — find every `airedOn { year month day }` in this file), add a `releasedOn` line right after `airedOn`:

```
			airedOn { year month day }
			releasedOn { year month day }
```

Run `grep -n "airedOn { year month day }" services/catalog/internal/parser/shikimori/client.go` first to find every occurrence; edit all of them.

- [ ] **Step 4: Map `releasedOn` in the raw mapper**

In `mapRawAnimeList` (client.go ~296), right after the `if a.AiredOn != nil { ... }` block:

```go
		if a.ReleasedOn != nil && a.ReleasedOn.Year > 0 && a.ReleasedOn.Month > 0 && a.ReleasedOn.Day > 0 {
			relDate := time.Date(a.ReleasedOn.Year, time.Month(a.ReleasedOn.Month), a.ReleasedOn.Day, 0, 0, 0, 0, time.UTC)
			anime.ReleasedOn = &relDate
		}
```

- [ ] **Step 5: Map `releasedOn` in the typed mapper**

In `mapAnime` (client.go ~467), right after the `if sa.AiredOn != nil { ... }` block:

```go
	if sa.ReleasedOn != nil {
		ry := int(sa.ReleasedOn.Year)
		rm := int(sa.ReleasedOn.Month)
		rd := int(sa.ReleasedOn.Day)
		if ry > 0 && rm > 0 && rd > 0 {
			relDate := time.Date(ry, time.Month(rm), rd, 0, 0, 0, 0, time.UTC)
			anime.ReleasedOn = &relDate
		}
	}
```

- [ ] **Step 6: Build**

Run: `cd services/catalog && go build ./...`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/domain/anime.go services/catalog/internal/parser/shikimori/client.go
git commit -F - <<'EOF'
feat(catalog): fetch and store anime releasedOn (air end date)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 2: Backend — `anime_person_roles` flat table (domain + repo)

The one denormalized staff table: role as a scalar column, person data inline, no Person entity, no join table.

**Files:**
- Create: `services/catalog/internal/domain/person_role.go`
- Create: `services/catalog/internal/repo/person_role.go`
- Create (test): `services/catalog/internal/repo/person_role_test.go`
- Modify: `services/catalog/cmd/catalog-api/main.go` (AutoMigrate list + wiring later)

**Interfaces:**
- Produces: `domain.AnimePersonRole` (the flat row & wire model).
- Produces: `repo.PersonRoleRepository` with `ReplaceAnimeStaff(ctx, animeID string, rows []domain.AnimePersonRole) error` and `GetStaffByAnimeID(ctx, animeID string) ([]domain.AnimePersonRole, error)`.

- [ ] **Step 1: Create the domain model**

Create `services/catalog/internal/domain/person_role.go`:

```go
package domain

import "time"

// AnimePersonRole is ONE flat, denormalized staff/crew credit for an anime.
//
// Deliberately NOT normalized (owner directive): there is no separate Person
// entity and no m2m join table. Person identity (name/poster) lives inline and
// repeats per role — one row per (anime, person, role). Role is a scalar
// column so the read model can group/sort by it directly. Sourced from
// Shikimori GraphQL personRoles, filtered to a headline whitelist at parse
// time (see parser/shikimori/staff.go).
type AnimePersonRole struct {
	ID                string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AnimeID           string    `gorm:"type:uuid;index:idx_person_roles_anime" json:"anime_id"`
	ShikimoriPersonID string    `gorm:"size:50;index" json:"shikimori_person_id,omitempty"`
	Name              string    `gorm:"size:500" json:"name"`
	NameRU            string    `gorm:"size:500" json:"name_ru,omitempty"`
	NameJP            string    `gorm:"size:500" json:"name_jp,omitempty"`
	PosterURL         string    `gorm:"size:1000" json:"poster_url,omitempty"`
	Role              string    `gorm:"size:100;index" json:"role"`          // canonical EN, scalar
	RoleRU            string    `gorm:"size:100" json:"role_ru,omitempty"`   // Shikimori rolesRu (free)
	IsProducer        bool      `json:"is_producer,omitempty"`
	IsMangaka         bool      `json:"is_mangaka,omitempty"`
	Position          int       `gorm:"default:0" json:"position"`           // whitelist rank
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
```

- [ ] **Step 2: Write the failing repo test**

Create `services/catalog/internal/repo/person_role_test.go`. Mirror the SQLite setup used by `character_test.go` in this package (open `sqlite` in-memory, `AutoMigrate(&domain.AnimePersonRole{})`):

```go
package repo

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func newPersonRoleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.AnimePersonRole{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestReplaceAnimeStaff_ReplacesAndOrders(t *testing.T) {
	db := newPersonRoleDB(t)
	r := NewPersonRoleRepository(db)
	ctx := context.Background()
	const animeID = "11111111-1111-1111-1111-111111111111"

	// First write: two rows.
	rows := []domain.AnimePersonRole{
		{AnimeID: animeID, ShikimoriPersonID: "1", Name: "B Person", Role: "Script", Position: 3},
		{AnimeID: animeID, ShikimoriPersonID: "2", Name: "A Person", Role: "Director", Position: 0},
	}
	if err := r.ReplaceAnimeStaff(ctx, animeID, rows); err != nil {
		t.Fatalf("replace: %v", err)
	}

	got, err := r.GetStaffByAnimeID(ctx, animeID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	// Ordered by position ASC → Director (0) before Script (3).
	if got[0].Role != "Director" || got[1].Role != "Script" {
		t.Fatalf("bad order: %s, %s", got[0].Role, got[1].Role)
	}

	// Second write REPLACES (not appends): one row.
	if err := r.ReplaceAnimeStaff(ctx, animeID, []domain.AnimePersonRole{
		{AnimeID: animeID, ShikimoriPersonID: "9", Name: "Solo", Role: "Music", Position: 0},
	}); err != nil {
		t.Fatalf("replace2: %v", err)
	}
	got, _ = r.GetStaffByAnimeID(ctx, animeID)
	if len(got) != 1 || got[0].Role != "Music" {
		t.Fatalf("replace did not overwrite: %+v", got)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd services/catalog && go test ./internal/repo/ -run TestReplaceAnimeStaff -v`
Expected: FAIL — `undefined: NewPersonRoleRepository`.

- [ ] **Step 4: Implement the repo**

Create `services/catalog/internal/repo/person_role.go`:

```go
package repo

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// PersonRoleRepository persists the flat anime staff/crew credits.
type PersonRoleRepository struct {
	db *gorm.DB
}

func NewPersonRoleRepository(db *gorm.DB) *PersonRoleRepository {
	return &PersonRoleRepository{db: db}
}

// ReplaceAnimeStaff deletes the anime's existing staff rows and inserts the
// given set, in one transaction. The flat table has no join to resolve, so
// this is a straight delete-then-insert (mirrors the ReplaceAnimeCharacters
// resilience contract without the id-remap dance). IDs are assigned Go-side
// when blank — Postgres's gen_random_uuid() default works in prod, but
// generating here keeps the repo portable to the SQLite test DB (same reason
// CharacterRepository does it).
func (r *PersonRoleRepository) ReplaceAnimeStaff(ctx context.Context, animeID string, rows []domain.AnimePersonRole) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("anime_id = ?", animeID).Delete(&domain.AnimePersonRole{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for i := range rows {
			if rows[i].ID == "" {
				rows[i].ID = uuid.NewString()
			}
		}
		return tx.Create(&rows).Error
	})
}

// GetStaffByAnimeID returns the anime's staff ordered by whitelist rank then name.
func (r *PersonRoleRepository) GetStaffByAnimeID(ctx context.Context, animeID string) ([]domain.AnimePersonRole, error) {
	var rows []domain.AnimePersonRole
	err := r.db.WithContext(ctx).
		Where("anime_id = ?", animeID).
		Order("position ASC, name ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd services/catalog && go test ./internal/repo/ -run TestReplaceAnimeStaff -v`
Expected: PASS.

- [ ] **Step 6: Register the table in AutoMigrate**

In `services/catalog/cmd/catalog-api/main.go`, in the `db.AutoMigrate(...)` list (after `&domain.AnimeCharacter{}`, line ~110):

```go
		&domain.Character{},
		&domain.AnimeCharacter{},
		// Flat denormalized staff/crew credits (2026-07-16).
		&domain.AnimePersonRole{},
```

- [ ] **Step 7: Build + commit**

```bash
cd services/catalog && go build ./... && cd ../..
git add services/catalog/internal/domain/person_role.go services/catalog/internal/repo/person_role.go services/catalog/internal/repo/person_role_test.go services/catalog/cmd/catalog-api/main.go
git commit -F - <<'EOF'
feat(catalog): anime_person_roles flat staff table + repo

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 3: Backend — Shikimori `GetAnimeStaff` parser + whitelist

Fetches `personRoles`, flattens to one-row-per-(person,role), filters to the whitelist.

**Files:**
- Create: `services/catalog/internal/parser/shikimori/staff.go`
- Create (test): append to a new `services/catalog/internal/parser/shikimori/staff_test.go`

**Interfaces:**
- Consumes: `Client.postRaw` (existing, `characters.go`), `Client.rateLimiter`, `Client.config`.
- Produces: `Client.GetAnimeStaff(ctx, shikimoriID string) ([]domain.AnimePersonRole, error)` — rows have every field EXCEPT `ID`/`AnimeID`/timestamps (the service fills `AnimeID`).
- Produces (pure, tested): `filterStaffRoles(rolesEn, rolesRu []string) []staffRole` where `type staffRole struct { Role, RoleRU string; Rank int }`.

- [ ] **Step 1: Write the failing pure-function test**

Create `services/catalog/internal/parser/shikimori/staff_test.go`:

```go
package shikimori

import "testing"

func TestFilterStaffRoles(t *testing.T) {
	// Director is whitelisted (rank 0); Key Animation is not → dropped.
	got := filterStaffRoles(
		[]string{"Key Animation", "Director"},
		[]string{"Аниматор", "Режиссёр"},
	)
	if len(got) != 1 {
		t.Fatalf("want 1 kept role, got %d (%+v)", len(got), got)
	}
	if got[0].Role != "Director" || got[0].RoleRU != "Режиссёр" || got[0].Rank != 0 {
		t.Fatalf("bad mapping: %+v", got[0])
	}

	// A person with two whitelisted roles yields two entries.
	got = filterStaffRoles(
		[]string{"Script", "Series Composition"},
		[]string{"Сценарий", "Компоновка серий"},
	)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}

	// rolesRu shorter than rolesEn → RoleRU is empty, no panic.
	got = filterStaffRoles([]string{"Music"}, nil)
	if len(got) != 1 || got[0].RoleRU != "" {
		t.Fatalf("nil rolesRu handling: %+v", got)
	}

	// Nothing whitelisted → empty.
	if got = filterStaffRoles([]string{"In-Between Animation"}, []string{"x"}); len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/shikimori/ -run TestFilterStaffRoles -v`
Expected: FAIL — `undefined: filterStaffRoles`.

- [ ] **Step 3: Implement the parser**

Create `services/catalog/internal/parser/shikimori/staff.go`:

```go
package shikimori

import (
	"context"
	"fmt"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// staffRoleWhitelist is the ordered set of headline crew roles we keep from
// Shikimori personRoles. Order IS the display rank (index). Everything else
// (Key Animation, In-Between, etc.) is dropped — it is 90%+ of the payload and
// pure noise for a viewer. Match is exact against Shikimori's rolesEn strings.
var staffRoleWhitelist = []string{
	"Director",
	"Original Creator",
	"Series Composition",
	"Script",
	"Character Design",
	"Chief Animation Director",
	"Art Director",
	"Music",
	"Sound Director",
	"Director of Photography",
	"Producer",
	"Executive Producer",
}

// staffRoleRank maps a whitelisted role to its display rank. Built once.
var staffRoleRank = func() map[string]int {
	m := make(map[string]int, len(staffRoleWhitelist))
	for i, r := range staffRoleWhitelist {
		m[r] = i
	}
	return m
}()

type staffRole struct {
	Role   string // canonical EN
	RoleRU string // Shikimori's parallel rolesRu entry, "" if absent
	Rank   int
}

// filterStaffRoles keeps only whitelisted roles from a person's parallel
// rolesEn/rolesRu arrays, one staffRole per kept role. Pure — unit tested.
func filterStaffRoles(rolesEn, rolesRu []string) []staffRole {
	out := make([]staffRole, 0, len(rolesEn))
	for i, en := range rolesEn {
		rank, ok := staffRoleRank[en]
		if !ok {
			continue
		}
		ru := ""
		if i < len(rolesRu) {
			ru = rolesRu[i]
		}
		out = append(out, staffRole{Role: en, RoleRU: ru, Rank: rank})
	}
	return out
}

// GetAnimeStaff fetches an anime's crew from Shikimori personRoles, flattened
// to one domain.AnimePersonRole per (person, whitelisted role). AnimeID is left
// blank for the service to fill. personRoles is the STAFF list — it does NOT
// contain the voice cast (that lives on each character; see GetCharacterByID).
func (c *Client) GetAnimeStaff(ctx context.Context, shikimoriID string) ([]domain.AnimePersonRole, error) {
	c.rateLimiter.acquire()

	query := fmt.Sprintf(`{
		animes(ids: "%s", limit: 1) {
			personRoles {
				rolesEn
				rolesRu
				person {
					id name russian japanese isProducer isMangaka
					poster { originalUrl }
				}
			}
		}
	}`, shikimoriID)

	var data struct {
		Animes []struct {
			PersonRoles []struct {
				RolesEn []string `json:"rolesEn"`
				RolesRu []string `json:"rolesRu"`
				Person  struct {
					ID         string `json:"id"`
					Name       string `json:"name"`
					Russian    string `json:"russian"`
					Japanese   string `json:"japanese"`
					IsProducer bool   `json:"isProducer"`
					IsMangaka  bool   `json:"isMangaka"`
					Poster     *struct {
						OriginalURL string `json:"originalUrl"`
					} `json:"poster"`
				} `json:"person"`
			} `json:"personRoles"`
		} `json:"animes"`
	}

	if err := c.postRaw(ctx, query, &data); err != nil {
		return nil, err
	}
	if len(data.Animes) == 0 {
		return nil, errors.NotFound("anime")
	}

	rolesRaw := data.Animes[0].PersonRoles
	out := make([]domain.AnimePersonRole, 0, len(rolesRaw))
	for _, pr := range rolesRaw {
		kept := filterStaffRoles(pr.RolesEn, pr.RolesRu)
		if len(kept) == 0 {
			continue
		}
		poster := ""
		if pr.Person.Poster != nil {
			poster = pr.Person.Poster.OriginalURL
		}
		for _, k := range kept {
			out = append(out, domain.AnimePersonRole{
				ShikimoriPersonID: pr.Person.ID,
				Name:              pr.Person.Name,
				NameRU:            pr.Person.Russian,
				NameJP:            pr.Person.Japanese,
				PosterURL:         poster,
				Role:              k.Role,
				RoleRU:            k.RoleRU,
				IsProducer:        pr.Person.IsProducer,
				IsMangaka:         pr.Person.IsMangaka,
				Position:          k.Rank,
			})
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd services/catalog && go test ./internal/parser/shikimori/ -run TestFilterStaffRoles -v`
Expected: PASS.

- [ ] **Step 5: Build + commit**

```bash
cd services/catalog && go build ./... && cd ../..
git add services/catalog/internal/parser/shikimori/staff.go services/catalog/internal/parser/shikimori/staff_test.go
git commit -F - <<'EOF'
feat(catalog): Shikimori GetAnimeStaff personRoles parser + whitelist

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 4: Backend — StaffService + handler + route + wiring

Wires the parser + repo behind `GET /api/anime/{animeId}/staff` with the Redis→Shikimori→Postgres last-known-good flow.

**Files:**
- Create: `services/catalog/internal/service/staff.go`
- Create: `services/catalog/internal/handler/staff.go`
- Modify: `libs/cache/ttl.go` (add `KeyAnimeStaff`)
- Modify: `services/catalog/internal/transport/router.go` (signature + route)
- Modify: `services/catalog/cmd/catalog-api/main.go` (wiring + NewRouter call)

**Interfaces:**
- Consumes: `repo.PersonRoleRepository` (Task 2), `Client.GetAnimeStaff` (Task 3), `animeShikimoriIDLookup` (existing in `service/character.go`), `*cache.RedisCache`.
- Produces: `service.StaffService.GetAnimeStaff(ctx, animeID string) ([]domain.AnimePersonRole, error)`.
- Produces: `handler.StaffHandler.GetAnimeStaff(w, r)`.
- Produces: `cache.KeyAnimeStaff(animeID string) string`.

- [ ] **Step 1: Add the cache key**

In `libs/cache/ttl.go`, after `KeyAnimeCharacters` (line ~127):

```go
// KeyAnimeStaff is the cache key for an anime's staff/crew list.
// TTL = TTLAnimeDetails (6h). Mirrors KeyAnimeCharacters.
func KeyAnimeStaff(animeID string) string {
	return PrefixAnime + "staff:" + animeID
}
```

- [ ] **Step 2: Implement the service**

Create `services/catalog/internal/service/staff.go`:

```go
package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// staffShikimori is the slice of the Shikimori client this service needs.
type staffShikimori interface {
	GetAnimeStaff(ctx context.Context, shikimoriID string) ([]domain.AnimePersonRole, error)
}

// staffRepo is the slice of PersonRoleRepository this service needs.
type staffRepo interface {
	ReplaceAnimeStaff(ctx context.Context, animeID string, rows []domain.AnimePersonRole) error
	GetStaffByAnimeID(ctx context.Context, animeID string) ([]domain.AnimePersonRole, error)
}

// StaffService orchestrates Shikimori fetch → Postgres replace → Redis cache
// for an anime's crew. Same resilience contract as CharacterService: on a
// Shikimori failure it serves the last-known-good rows from Postgres.
type StaffService struct {
	animeRepo animeShikimoriIDLookup // defined in character.go (same package)
	staff     staffRepo
	shikimori staffShikimori
	cache     *cache.RedisCache
	log       *logger.Logger
}

func NewStaffService(
	animeRepo animeShikimoriIDLookup,
	staff staffRepo,
	shiki staffShikimori,
	c *cache.RedisCache,
	log *logger.Logger,
) *StaffService {
	return &StaffService{animeRepo: animeRepo, staff: staff, shikimori: shiki, cache: c, log: log}
}

// GetAnimeStaff returns an anime's crew. Flow: Redis → (miss) resolve
// shikimori id → fetch → replace Postgres → cache → return; on Shikimori
// failure, serve last-known-good from Postgres.
func (s *StaffService) GetAnimeStaff(ctx context.Context, animeID string) ([]domain.AnimePersonRole, error) {
	cacheKey := cache.KeyAnimeStaff(animeID)
	var cached []domain.AnimePersonRole
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	rows, ferr := s.shikimori.GetAnimeStaff(ctx, anime.ShikimoriID)
	if ferr != nil {
		s.log.Warnw("shikimori staff fetch failed, serving from db", "anime_id", animeID, "error", ferr)
		return s.staff.GetStaffByAnimeID(ctx, animeID)
	}

	for i := range rows {
		rows[i].AnimeID = animeID
	}
	if err := s.staff.ReplaceAnimeStaff(ctx, animeID, rows); err != nil {
		return nil, err
	}

	stored, err := s.staff.GetStaffByAnimeID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Set(ctx, cacheKey, stored, cache.TTLAnimeDetails)
	return stored, nil
}
```

- [ ] **Step 3: Implement the handler**

Create `services/catalog/internal/handler/staff.go`:

```go
package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

// StaffHandler serves the anime staff/crew endpoint.
type StaffHandler struct {
	svc *service.StaffService
	log *logger.Logger
}

func NewStaffHandler(svc *service.StaffService, log *logger.Logger) *StaffHandler {
	return &StaffHandler{svc: svc, log: log}
}

// GetAnimeStaff handles GET /api/anime/{animeId}/staff.
func (h *StaffHandler) GetAnimeStaff(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}
	list, err := h.svc.GetAnimeStaff(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, list)
}
```

- [ ] **Step 4: Add the route (signature + registration)**

In `services/catalog/internal/transport/router.go`:
- Add `staffHandler *handler.StaffHandler,` to the `NewRouter(...)` parameter list, right after `characterHandler *handler.CharacterHandler,` (line ~19).
- Register the route next to the characters route inside the `/anime` block. Find `r.Get("/{animeId}/related", ...)` (line ~157) and add after it:

```go
			r.Get("/{animeId}/staff", staffHandler.GetAnimeStaff)
```

- [ ] **Step 5: Wire it in main.go**

In `services/catalog/cmd/catalog-api/main.go`:
- After `characterRepo := repo.NewCharacterRepository(db.DB)` (line ~458):

```go
	personRoleRepo := repo.NewPersonRoleRepository(db.DB)
```

- After `characterService := service.NewCharacterService(...)` (line ~483):

```go
	staffService := service.NewStaffService(animeRepo, personRoleRepo, shikimoriClient, redisCache, log)
```

- After `characterHandler := handler.NewCharacterHandler(characterService, log)` (line ~490):

```go
	staffHandler := handler.NewStaffHandler(staffService, log)
```

- In the `transport.NewRouter(...)` call (line ~704), add `staffHandler,` right after `characterHandler,`.

- [ ] **Step 6: Build + test**

Run: `cd services/catalog && go build ./... && go test ./... 2>&1 | tail -20`
Expected: build clean, all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add libs/cache/ttl.go services/catalog/internal/service/staff.go services/catalog/internal/handler/staff.go services/catalog/internal/transport/router.go services/catalog/cmd/catalog-api/main.go
git commit -F - <<'EOF'
feat(catalog): GET /api/anime/{id}/staff — crew service, handler, route

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 5: Backend — voice cast (`seyu`) inline on Character

Extends the existing per-character path to capture `seyu[]` from Shikimori REST and persist it inline.

**Files:**
- Modify: `services/catalog/internal/domain/character.go` (Seyu field + CharacterSeyu type)
- Modify: `services/catalog/internal/parser/shikimori/characters.go` (REST fetch + absImageURL)
- Create (test): `services/catalog/internal/parser/shikimori/seyu_test.go`
- Modify: `services/catalog/internal/repo/character.go` (UpsertCharacter DoUpdates + seyu)

**Interfaces:**
- Produces: `domain.CharacterSeyu` and `domain.Character.Seyu []CharacterSeyu`.
- Produces (pure, tested): `absImageURL(base, path string) string`.
- Modifies: `Client.GetCharacterByID` now also populates `ch.Seyu` (fails soft).

- [ ] **Step 1: Add the domain type + field**

In `services/catalog/internal/domain/character.go`, add after the `Character` struct:

```go
// CharacterSeyu is one voice actor for a character. Stored inline on Character
// (owner directive: wire the cast onto existing characters, no separate seiyu
// table). Sourced from Shikimori REST /api/characters/{id} → seyu[]. The list
// mixes JP seiyu and localized dub actors with no language flag from Shikimori.
type CharacterSeyu struct {
	ShikimoriID string `json:"shikimori_id"`
	Name        string `json:"name"`
	NameRU      string `json:"name_ru,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	URL         string `json:"url,omitempty"`
}
```

And add a field to the `Character` struct, after `URL` (line ~24):

```go
	URL         string         `gorm:"size:1000" json:"url,omitempty"`
	// Seyu — the character's voice cast, stored inline as JSON (serializer:json,
	// portable across Postgres + the SQLite test DB). Populated by the
	// per-character REST fetch in GetCharacterByID.
	Seyu        []CharacterSeyu `gorm:"serializer:json" json:"seyu,omitempty"`
```

- [ ] **Step 2: Write the failing pure-helper test**

Create `services/catalog/internal/parser/shikimori/seyu_test.go`:

```go
package shikimori

import "testing"

func TestAbsImageURL(t *testing.T) {
	const base = "https://shikimori.io"
	cases := map[string]string{
		"/system/people/original/47918.jpg": "https://shikimori.io/system/people/original/47918.jpg",
		"https://cdn.example/x.jpg":         "https://cdn.example/x.jpg", // already absolute
		"":                                  "",                          // empty stays empty
	}
	for in, want := range cases {
		if got := absImageURL(base, in); got != want {
			t.Fatalf("absImageURL(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `cd services/catalog && go test ./internal/parser/shikimori/ -run TestAbsImageURL -v`
Expected: FAIL — `undefined: absImageURL`.

- [ ] **Step 4: Implement the REST seyu fetch + helper**

In `services/catalog/internal/parser/shikimori/characters.go`:

Add imports if missing — the file already imports `bytes`, `context`, `encoding/json`, `fmt`, `net/http`, `regexp`, `strings`, `errors`, `domain`. Add `"strings"` is already present.

Add the helper + fetch function at the end of the file:

```go
// absImageURL turns a Shikimori-relative image path into an absolute URL.
// REST /api/characters/{id} returns seyu image paths relative to the site
// root (e.g. "/system/people/original/1.jpg"); GraphQL poster originalUrls are
// already absolute. Pure — unit tested.
func absImageURL(base, path string) string {
	if path == "" || strings.HasPrefix(path, "http") {
		return path
	}
	return strings.TrimRight(base, "/") + path
}

// fetchCharacterSeyu pulls a character's voice cast from Shikimori REST
// (GraphQL's Character type has no seiyu field, and the anime-level /roles
// endpoint never pairs a character with a person — only this per-character
// endpoint does). Returns an empty slice (never an error) on failure so the
// caller can still return the character.
func (c *Client) fetchCharacterSeyu(ctx context.Context, shikimoriID string) []domain.CharacterSeyu {
	c.rateLimiter.acquire()

	url := fmt.Sprintf("%s/api/characters/%s", c.config.BaseURL, shikimoriID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Warnw("shikimori seyu fetch failed", "shikimori_id", shikimoriID, "error", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var body struct {
		Seyu []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Russian string `json:"russian"`
			URL     string `json:"url"`
			Image   *struct {
				Original string `json:"original"`
			} `json:"image"`
		} `json:"seyu"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}

	out := make([]domain.CharacterSeyu, 0, len(body.Seyu))
	for _, s := range body.Seyu {
		img := ""
		if s.Image != nil {
			img = absImageURL(c.config.BaseURL, s.Image.Original)
		}
		out = append(out, domain.CharacterSeyu{
			ShikimoriID: fmt.Sprintf("%d", s.ID),
			Name:        s.Name,
			NameRU:      s.Russian,
			ImageURL:    img,
			URL:         s.URL,
		})
	}
	return out
}
```

Then, in `GetCharacterByID`, right before `return ch, nil` (end of the function), populate seyu:

```go
	if src.Poster != nil {
		ch.PosterURL = src.Poster.OriginalURL
	}
	ch.Seyu = c.fetchCharacterSeyu(ctx, shikimoriID)
	return ch, nil
```

- [ ] **Step 5: Run to verify the helper test passes**

Run: `cd services/catalog && go test ./internal/parser/shikimori/ -run TestAbsImageURL -v`
Expected: PASS.

- [ ] **Step 6: Persist seyu on upsert**

In `services/catalog/internal/repo/character.go`, `UpsertCharacter`, add `"seyu"` to the `DoUpdates` column list (line ~34):

```go
			DoUpdates: clause.AssignmentColumns([]string{"mal_id", "name", "name_ru", "name_jp", "synonyms", "poster_url", "description", "url", "seyu", "updated_at"}),
```

(Leave `ReplaceAnimeCharacters`'s DoUpdates unchanged — the list path never fetches seyu, so it must not blank it.)

- [ ] **Step 7: Build + test + commit**

Run: `cd services/catalog && go build ./... && go test ./... 2>&1 | tail -20`
Expected: build clean, all PASS.

```bash
cd ../..
git add services/catalog/internal/domain/character.go services/catalog/internal/parser/shikimori/characters.go services/catalog/internal/parser/shikimori/seyu_test.go services/catalog/internal/repo/character.go
git commit -F - <<'EOF'
feat(catalog): fetch and store character voice cast (seyu) inline

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 6: Frontend — anime metadata data layer

Types + transform so `Anime.vue` can read studios, released date, source, and age rating.

**Files:**
- Modify: `frontend/web/src/composables/useAnime.ts` (ApiAnime, Anime, transformAnime)
- Create: `frontend/web/src/utils/animeMeta.ts` (pure rating/source display maps)
- Create (test): `frontend/web/src/utils/animeMeta.spec.ts`

**Interfaces:**
- Produces: `Anime.studios?: { id: string; name: string }[]`, `Anime.releasedOn?: string`, `Anime.materialSource?: string`, `Anime.ageRating?: string`.
- Produces: `ratingLabel(raw?: string): string`, `sourceLabelKey(raw?: string): string | null` in `utils/animeMeta.ts`.

- [ ] **Step 1: Write the failing util test**

Create `frontend/web/src/utils/animeMeta.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { ratingLabel, sourceLabelKey } from './animeMeta'

describe('ratingLabel', () => {
  it('maps Shikimori rating codes to display badges', () => {
    expect(ratingLabel('r_plus')).toBe('R+')
    expect(ratingLabel('pg_13')).toBe('PG-13')
    expect(ratingLabel('rx')).toBe('Rx')
  })
  it('returns empty string for unknown/empty', () => {
    expect(ratingLabel(undefined)).toBe('')
    expect(ratingLabel('none')).toBe('')
  })
})

describe('sourceLabelKey', () => {
  it('maps known sources to an i18n key suffix', () => {
    expect(sourceLabelKey('manga')).toBe('manga')
    expect(sourceLabelKey('light_novel')).toBe('light_novel')
  })
  it('returns null for empty', () => {
    expect(sourceLabelKey(undefined)).toBeNull()
    expect(sourceLabelKey('')).toBeNull()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd frontend/web && bunx vitest run src/utils/animeMeta.spec.ts`
Expected: FAIL — cannot resolve `./animeMeta`.

- [ ] **Step 3: Implement the util**

Create `frontend/web/src/utils/animeMeta.ts`:

```ts
// Display helpers for anime production metadata.

// Shikimori age-rating codes → classification badge. These are codes, not
// translated prose, so they are the same in every locale.
const RATING_LABELS: Record<string, string> = {
  g: 'G',
  pg: 'PG',
  pg_13: 'PG-13',
  r: 'R-17',
  r_plus: 'R+',
  rx: 'Rx',
}

export function ratingLabel(raw?: string): string {
  if (!raw) return ''
  return RATING_LABELS[raw] ?? ''
}

// Known Shikimori adaptation sources. Returns the i18n key suffix under
// anime.sources.*, or null when unknown/empty (caller falls back to the raw
// value or hides the row).
const KNOWN_SOURCES = new Set([
  'original',
  'manga',
  'web_manga',
  'novel',
  'light_novel',
  'visual_novel',
  'game',
  'card_game',
  'music',
  'book',
  'other',
])

export function sourceLabelKey(raw?: string): string | null {
  if (!raw) return null
  return KNOWN_SOURCES.has(raw) ? raw : null
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd frontend/web && bunx vitest run src/utils/animeMeta.spec.ts`
Expected: PASS.

- [ ] **Step 5: Extend the API + view types in useAnime.ts**

In `frontend/web/src/composables/useAnime.ts`:

Add to `interface ApiAnime` (after `aired_on`, line ~28):

```ts
  aired_on?: string | null
  released_on?: string | null
  material_source?: string
  rating?: string
  studios?: { id: string; name: string }[]
```

Add to `export interface Anime` (after `airedOn`, line ~57):

```ts
  airedOn?: string
  releasedOn?: string
  materialSource?: string
  ageRating?: string
  studios?: { id: string; name: string }[]
```

Add to `transformAnime` return object (after `airedOn:`, line ~94):

```ts
    airedOn: apiAnime.aired_on || undefined,
    releasedOn: apiAnime.released_on || undefined,
    materialSource: apiAnime.material_source || undefined,
    ageRating: apiAnime.rating || undefined,
    studios: apiAnime.studios || undefined,
```

- [ ] **Step 6: Type-check + commit**

Run: `cd frontend/web && bunx vue-tsc --noEmit -p tsconfig.app.json 2>&1 | tail -5` (note: vue-tsc `--noEmit` can false-pass; the real gate is `bun run build` in Task 10).
Expected: no new errors from these files.

```bash
cd ../..
git add frontend/web/src/composables/useAnime.ts frontend/web/src/utils/animeMeta.ts frontend/web/src/utils/animeMeta.spec.ts
git commit -F - <<'EOF'
feat(web): anime production metadata types + display helpers

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 7: Frontend — staff data layer (`useStaff` + api)

**Files:**
- Modify: `frontend/web/src/api/client.ts` (add `staffApi`)
- Create: `frontend/web/src/composables/useStaff.ts`
- Create: `frontend/web/src/types/staff.ts`

**Interfaces:**
- Consumes: `GET /api/anime/{id}/staff` returning `ApiPersonRole[]`.
- Produces: `useStaff()` → `{ staff: Ref<StaffMember[]>, fetchStaff(animeId): Promise<StaffMember[]> }`.
- Produces: `StaffMember = { id: string; name: string; roleKey: string; roleRu?: string; roleEn: string; image: string }`.

- [ ] **Step 1: Add the API method**

In `frontend/web/src/api/client.ts`, right after the `charactersApi` block (line ~1010):

```ts
export const staffApi = {
  getAnimeStaff: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/staff`),
}
```

- [ ] **Step 2: Add the types**

Create `frontend/web/src/types/staff.ts`:

```ts
// Raw API shape (snake_case, mirrors Go json tags on domain.AnimePersonRole).
export interface ApiPersonRole {
  id: string
  shikimori_person_id?: string
  name: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  role: string        // canonical EN, e.g. "Director"
  role_ru?: string
  is_producer?: boolean
  is_mangaka?: boolean
  position: number
}

// Frontend view-model for one staff row in the Details table.
export interface StaffMember {
  id: string          // shikimori_person_id (may be "")
  name: string        // localized (RU fallback EN)
  roleEn: string      // canonical EN role
  roleKey: string     // i18n key suffix under anime.roles.* (camelCased EN)
  roleRu?: string     // Shikimori RU label, used as fallback if no i18n key
  image: string       // proxied poster url
}
```

- [ ] **Step 3: Implement the composable**

Create `frontend/web/src/composables/useStaff.ts`:

```ts
import { ref } from 'vue'
import { staffApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import type { ApiPersonRole, StaffMember } from '@/types/staff'

// Canonical EN role → i18n key suffix under anime.roles.*.
function roleKeyFromEn(en: string): string {
  const map: Record<string, string> = {
    'Director': 'director',
    'Original Creator': 'originalCreator',
    'Series Composition': 'seriesComposition',
    'Script': 'script',
    'Character Design': 'characterDesign',
    'Chief Animation Director': 'chiefAnimationDirector',
    'Art Director': 'artDirector',
    'Music': 'music',
    'Sound Director': 'soundDirector',
    'Director of Photography': 'directorOfPhotography',
    'Producer': 'producer',
    'Executive Producer': 'executiveProducer',
  }
  return map[en] ?? ''
}

function toMember(r: ApiPersonRole): StaffMember {
  return {
    id: r.shikimori_person_id || '',
    name: getLocalizedTitle(r.name, r.name_ru, r.name_jp),
    roleEn: r.role,
    roleKey: roleKeyFromEn(r.role),
    roleRu: r.role_ru || undefined,
    image: getImageUrl(r.poster_url),
  }
}

// Staff/crew for an anime. Fails soft (empty on error) like useCharacters.
export function useStaff() {
  const staff = ref<StaffMember[]>([])

  const fetchStaff = async (animeId: string) => {
    try {
      const resp = await staffApi.getAnimeStaff(animeId)
      const data = (resp.data?.data || resp.data) as ApiPersonRole[]
      staff.value = Array.isArray(data) ? data.map(toMember) : []
      return staff.value
    } catch (err) {
      console.warn('Failed to fetch staff:', err)
      staff.value = []
      return []
    }
  }

  return { staff, fetchStaff }
}
```

- [ ] **Step 4: Type-check + commit**

Run: `cd frontend/web && bunx eslint src/composables/useStaff.ts src/types/staff.ts src/api/client.ts`
Expected: no errors.

```bash
cd ../..
git add frontend/web/src/api/client.ts frontend/web/src/composables/useStaff.ts frontend/web/src/types/staff.ts
git commit -F - <<'EOF'
feat(web): staff data layer — staffApi + useStaff composable

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 8: Frontend — Details disclosure on Anime.vue + i18n

Renders the metadata block + staff table behind a collapsible, lazy-fetching staff on first expand.

**Files:**
- Modify: `frontend/web/src/views/Anime.vue` (template section + script)
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`

**Interfaces:**
- Consumes: `Anime` fields from Task 6, `useStaff` from Task 7, `ratingLabel`/`sourceLabelKey` from Task 6.

- [ ] **Step 1: Add i18n keys (all three locales)**

Add an `anime.details` block, an `anime.roles` block, and an `anime.sources` block. In `frontend/web/src/locales/en.json`, inside the `"anime"` object (e.g. after `"synopsis": "Synopsis",`):

```json
    "details": {
      "title": "Details",
      "show": "Show details",
      "hide": "Hide details",
      "studio": "Studio",
      "aired": "Aired",
      "airedRange": "{start} — {end}",
      "source": "Source",
      "rating": "Rating",
      "duration": "Duration",
      "durationMin": "{n} min",
      "staff": "Staff",
      "empty": "No production details available."
    },
    "roles": {
      "director": "Director",
      "originalCreator": "Original creator",
      "seriesComposition": "Series composition",
      "script": "Script",
      "characterDesign": "Character design",
      "chiefAnimationDirector": "Chief animation director",
      "artDirector": "Art director",
      "music": "Music",
      "soundDirector": "Sound director",
      "directorOfPhotography": "Director of photography",
      "producer": "Producer",
      "executiveProducer": "Executive producer"
    },
    "sources": {
      "original": "Original",
      "manga": "Manga",
      "web_manga": "Web manga",
      "novel": "Novel",
      "light_novel": "Light novel",
      "visual_novel": "Visual novel",
      "game": "Game",
      "card_game": "Card game",
      "music": "Music",
      "book": "Book",
      "other": "Other"
    },
```

In `ru.json` (same location under `"anime"`):

```json
    "details": {
      "title": "Подробности",
      "show": "Показать подробности",
      "hide": "Скрыть подробности",
      "studio": "Студия",
      "aired": "Выходил",
      "airedRange": "{start} — {end}",
      "source": "Первоисточник",
      "rating": "Рейтинг",
      "duration": "Длительность",
      "durationMin": "{n} мин",
      "staff": "Над аниме работали",
      "empty": "Нет данных о производстве."
    },
    "roles": {
      "director": "Режиссёр",
      "originalCreator": "Автор оригинала",
      "seriesComposition": "Компоновка серий",
      "script": "Сценарий",
      "characterDesign": "Дизайн персонажей",
      "chiefAnimationDirector": "Главный режиссёр анимации",
      "artDirector": "Арт-директор",
      "music": "Музыка",
      "soundDirector": "Звукорежиссёр",
      "directorOfPhotography": "Оператор-постановщик",
      "producer": "Продюсер",
      "executiveProducer": "Исполнительный продюсер"
    },
    "sources": {
      "original": "Оригинал",
      "manga": "Манга",
      "web_manga": "Веб-манга",
      "novel": "Роман",
      "light_novel": "Ранобэ",
      "visual_novel": "Визуальный роман",
      "game": "Игра",
      "card_game": "Карточная игра",
      "music": "Музыка",
      "book": "Книга",
      "other": "Другое"
    },
```

In `ja.json` (same location under `"anime"`):

```json
    "details": {
      "title": "詳細",
      "show": "詳細を表示",
      "hide": "詳細を隠す",
      "studio": "制作",
      "aired": "放送",
      "airedRange": "{start} — {end}",
      "source": "原作",
      "rating": "レーティング",
      "duration": "話数の長さ",
      "durationMin": "{n}分",
      "staff": "スタッフ",
      "empty": "制作情報がありません。"
    },
    "roles": {
      "director": "監督",
      "originalCreator": "原作",
      "seriesComposition": "シリーズ構成",
      "script": "脚本",
      "characterDesign": "キャラクターデザイン",
      "chiefAnimationDirector": "総作画監督",
      "artDirector": "美術監督",
      "music": "音楽",
      "soundDirector": "音響監督",
      "directorOfPhotography": "撮影監督",
      "producer": "プロデューサー",
      "executiveProducer": "製作総指揮"
    },
    "sources": {
      "original": "オリジナル",
      "manga": "漫画",
      "web_manga": "ウェブ漫画",
      "novel": "小説",
      "light_novel": "ライトノベル",
      "visual_novel": "ビジュアルノベル",
      "game": "ゲーム",
      "card_game": "カードゲーム",
      "music": "音楽",
      "book": "書籍",
      "other": "その他"
    },
```

- [ ] **Step 2: Add the disclosure template**

In `frontend/web/src/views/Anime.vue`, immediately AFTER the closing `</section>` of `section-overview` (the synopsis section ends at line ~381) and BEFORE the `<!-- Video Player Section -->` comment, insert:

```vue
      <!-- Production Details disclosure (feedback 2026-07-13) -->
      <section id="section-details" class="mt-8">
        <button
          type="button"
          class="flex items-center gap-2 text-xl font-semibold text-white mb-3"
          :aria-expanded="detailsExpanded"
          @click="toggleDetails"
        >
          <Info class="size-6 text-cyan-400" aria-hidden="true" />
          {{ $t('anime.details.title') }}
          <ChevronDown
            class="size-5 text-white/50 transition-transform"
            :class="{ 'rotate-180': detailsExpanded }"
            aria-hidden="true"
          />
        </button>

        <div v-show="detailsExpanded" class="glass-card p-4 space-y-5">
          <!-- Metadata key/value grid -->
          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <template v-if="anime.studios && anime.studios.length">
              <dt class="text-white/50">{{ $t('anime.details.studio') }}</dt>
              <dd class="text-white/90">{{ anime.studios.map(s => s.name).join(', ') }}</dd>
            </template>
            <template v-if="airedRange">
              <dt class="text-white/50">{{ $t('anime.details.aired') }}</dt>
              <dd class="text-white/90">{{ airedRange }}</dd>
            </template>
            <template v-if="sourceLabel">
              <dt class="text-white/50">{{ $t('anime.details.source') }}</dt>
              <dd class="text-white/90">{{ sourceLabel }}</dd>
            </template>
            <template v-if="ratingBadge">
              <dt class="text-white/50">{{ $t('anime.details.rating') }}</dt>
              <dd class="text-white/90">{{ ratingBadge }}</dd>
            </template>
            <template v-if="anime.episodeDuration">
              <dt class="text-white/50">{{ $t('anime.details.duration') }}</dt>
              <dd class="text-white/90">{{ $t('anime.details.durationMin', { n: anime.episodeDuration }) }}</dd>
            </template>
          </dl>

          <!-- Staff table grouped by role -->
          <div v-if="staff.length">
            <h3 class="text-sm font-semibold text-white/70 mb-2">{{ $t('anime.details.staff') }}</h3>
            <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
              <template v-for="row in staff" :key="row.id + row.roleEn">
                <dt class="text-white/50">{{ row.roleKey ? $t('anime.roles.' + row.roleKey) : (row.roleRu || row.roleEn) }}</dt>
                <dd class="text-white/90">{{ row.name }}</dd>
              </template>
            </dl>
          </div>
        </div>
      </section>
```

- [ ] **Step 3: Wire the script**

In `frontend/web/src/views/Anime.vue` `<script setup>`:

Add to the lucide import (find the existing `lucide-vue-next` import; it already imports `Play`) — add `Info, ChevronDown`:

```ts
import { Play, Info, ChevronDown } from 'lucide-vue-next'
```
(Merge into the existing lucide import line — do not add a second import from the same module.)

Add near the other composable/state declarations (after `const { characters, fetchCharacters } = useCharacters()`, line ~1084):

```ts
import { useStaff } from '@/composables/useStaff'
import { ratingLabel, sourceLabelKey } from '@/utils/animeMeta'
// ...
const { staff, fetchStaff } = useStaff()
const staffFetched = ref(false)
const detailsExpanded = ref(false)

function toggleDetails() {
  detailsExpanded.value = !detailsExpanded.value
  if (detailsExpanded.value && !staffFetched.value && anime.value?.id) {
    staffFetched.value = true
    void fetchStaff(String(anime.value.id))
  }
}
```
(Place the two `import` lines with the other top-of-script imports, not inside the body.)

Add computed helpers (near other `computed` declarations):

```ts
const ratingBadge = computed(() => ratingLabel(anime.value?.ageRating))

const sourceLabel = computed(() => {
  const key = sourceLabelKey(anime.value?.materialSource)
  return key ? t('anime.sources.' + key) : (anime.value?.materialSource || '')
})

const airedRange = computed(() => {
  const a = anime.value?.airedOn
  const r = anime.value?.releasedOn
  const fmt = (iso?: string) => {
    if (!iso) return ''
    const d = new Date(iso)
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(locale.value, { year: 'numeric', month: 'short', day: 'numeric' })
  }
  const start = fmt(a)
  const end = fmt(r)
  if (start && end) return t('anime.details.airedRange', { start, end })
  return start || end || ''
})
```

Ensure `computed`, `t`, and `locale` are available: `computed` from `vue` (likely already imported); `const { t, locale } = useI18n()` — check the existing `useI18n()` destructure and add `locale` if missing. Reset `staffFetched`/`detailsExpanded` where the page resets other per-anime state (near line ~1135 `characters.value = []`):

```ts
  characters.value = []
  staff.value = []
  staffFetched.value = false
  detailsExpanded.value = false
```

- [ ] **Step 4: Run the DS + i18n + build gate**

Run: `cd frontend/web && bash ../../bin/ae-fe-verify.sh src/views/Anime.vue && bash scripts/i18n-lint.sh`
Expected: DS-lint 0 errors, i18n parity OK, `bun run build` succeeds.
(If `ae-fe-verify.sh` doesn't run i18n parity, run `scripts/i18n-lint.sh` explicitly since locale JSON changed.)

- [ ] **Step 5: Commit**

```bash
cd ../..
git add frontend/web/src/views/Anime.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -F - <<'EOF'
feat(web): production Details disclosure on the anime page

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 9: Frontend — voice cast on the character page

**Files:**
- Modify: `frontend/web/src/types/character.ts` (Seyu + CharacterDetail)
- Modify: `frontend/web/src/composables/useCharacters.ts` (`toDetail` maps seyu)
- Modify: `frontend/web/src/views/Character.vue` (voice cast section)
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (`characters.seyu`)

**Interfaces:**
- Consumes: `ApiCharacter.seyu` (added), `CharacterDetail.seyu`.

- [ ] **Step 1: Extend the types**

In `frontend/web/src/types/character.ts`:

Add a seyu shape + field on `ApiCharacter`:

```ts
export interface ApiCharacterSeyu {
  shikimori_id: string
  name: string
  name_ru?: string
  image_url?: string
  url?: string
}

export interface ApiCharacter {
  id: string
  shikimori_id: string
  mal_id?: string
  name: string
  name_ru?: string
  name_jp?: string
  synonyms?: string
  poster_url?: string
  description?: string
  url?: string
  seyu?: ApiCharacterSeyu[]
}
```

Add a view-model + field on `CharacterDetail`:

```ts
export interface SeyuModel {
  id: string
  name: string       // localized
  image: string      // proxied
}

export interface CharacterDetail {
  shikimoriId: string
  name: string
  nameRu?: string
  nameJp?: string
  synonyms?: string
  image: string
  description?: string
  seyu: SeyuModel[]
}
```

- [ ] **Step 2: Map seyu in `toDetail`**

In `frontend/web/src/composables/useCharacters.ts`, extend `toDetail`:

```ts
function toDetail(c: ApiCharacter): CharacterDetail {
  return {
    shikimoriId: c.shikimori_id,
    name: getLocalizedTitle(c.name, c.name_ru, c.name_jp),
    nameRu: c.name_ru || undefined,
    nameJp: c.name_jp || undefined,
    synonyms: c.synonyms || undefined,
    image: getImageUrl(c.poster_url),
    description: c.description || undefined,
    seyu: (c.seyu || []).map((s) => ({
      id: s.shikimori_id,
      name: getLocalizedTitle(s.name, s.name_ru, undefined),
      image: getImageUrl(s.image_url),
    })),
  }
}
```

(Update the `CharacterDetail`/`ApiCharacter` type imports at the top of the file if TS flags them — they already import from `@/types/character`.)

- [ ] **Step 3: Add i18n key (all three locales)**

Add `"seyu"` inside the `"characters"` object of each locale, after `"description"`:
- `en.json`: `"seyu": "Voice cast",`
- `ru.json`: `"seyu": "Озвучивание",`
- `ja.json`: `"seyu": "声優",`

- [ ] **Step 4: Render the voice cast section**

In `frontend/web/src/views/Character.vue`, inside the `v-else-if="character"` info column, after the description block (`<p v-else ...>{{ $t('characters.noDescription') }}</p>`) and before the closing `</div>` of the info column, add:

```vue
        <template v-if="character.seyu.length">
          <h2 class="text-sm font-semibold text-white/70 mt-6 mb-2">{{ $t('characters.seyu') }}</h2>
          <ul class="flex flex-col gap-2">
            <li v-for="va in character.seyu" :key="va.id" class="flex items-center gap-3">
              <CharacterImage
                :src="va.image || '/placeholder.svg'"
                :alt="va.name"
                ratio="1/1"
                rounded="full"
                :proxy-width="64"
                class="size-10 shrink-0 border border-white/10"
              />
              <span class="text-white/85 text-sm">{{ va.name }}</span>
            </li>
          </ul>
        </template>
```

(`CharacterImage` is already imported in `Character.vue`.)

- [ ] **Step 5: Gate + commit**

Run: `cd frontend/web && bash ../../bin/ae-fe-verify.sh src/views/Character.vue src/composables/useCharacters.ts && bash scripts/i18n-lint.sh`
Expected: DS-lint 0 errors, i18n parity OK, build succeeds.

```bash
cd ../..
git add frontend/web/src/types/character.ts frontend/web/src/composables/useCharacters.ts frontend/web/src/views/Character.vue frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -F - <<'EOF'
feat(web): voice cast (seyu) on the character page

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 10: Full verification gate

Final build/test across backend + frontend before landing.

**Files:** none (verification only).

- [ ] **Step 1: Backend build + full test**

Run: `cd services/catalog && go build ./... && go test ./... 2>&1 | tail -30`
Expected: build clean; all tests PASS. If `go vet`/`gofmt` complaints appear, fix by hand — NEVER run `gofmt -w` or `make fmt` (smart-quote landmine).

- [ ] **Step 2: Frontend full verify**

Run: `cd frontend/web && bash ../../bin/ae-fe-verify.sh && bash scripts/i18n-lint.sh`
Expected: DS-lint 0 errors, ESLint clean, `bun run build` succeeds, touched vitest specs (`animeMeta.spec.ts`) PASS, i18n en/ru/ja parity OK.

- [ ] **Step 3: Manual smoke against a real anime (verify streams/data, per project rule)**

The design was validated against FMA:Brotherhood (Shikimori id 5114) and Rimuru (character 131549). After deploy (handled by `/animeenigma-after-update`), spot-check the live endpoints:

```bash
# staff endpoint returns whitelisted crew (Director present, Key Animation absent)
curl -s "http://localhost:8081/api/anime/<local-uuid>/staff" | python3 -m json.tool | head -40
# character endpoint carries seyu
curl -s "http://localhost:8081/api/characters/131549" | python3 -c "import sys,json;print(len(json.load(sys.stdin).get('data',{}).get('seyu',[])),'seyu')"
```
Expected: staff list contains a `"role":"Director"` row and no `"Key Animation"`; character carries a non-empty `seyu` array.

- [ ] **Step 4: Land + after-update**

Do NOT hand-run the land/deploy here — hand off to `/animeenigma-after-update`, which runs `/simplify`, lint/build, redeploys `catalog` + `web`, health-checks, writes the changelog (Russian Trump-mode), and commits/pushes. The design doc + this plan are already committed on the branch.

---

## Self-Review

**Spec coverage:**
- Piece A metadata (studio/aired→released/source/rating/duration) → Tasks 1, 6, 8. `releasedOn` backend = Task 1. ✅
- Piece B flat `anime_person_roles`, role-as-column, whitelist, no Person entity → Tasks 2, 3, 4, 7, 8. ✅
- Piece C seyu inline on Character via per-character REST, character page only → Tasks 5, 9. ✅
- "Details" disclosure presentation → Task 8. ✅
- i18n en/ru/ja parity → Tasks 8, 9 + gate in 10. ✅
- Non-goal (no anime-rail cast enrichment) respected — Task 9 touches only `Character.vue`. ✅

**Placeholder scan:** no TBD/TODO; every code step shows full code; no "similar to Task N" — repo/service/handler code written out in full. ✅

**Type consistency:**
- `domain.AnimePersonRole` fields (Task 2) match the parser output (Task 3), repo (Task 2), service (Task 4), and `ApiPersonRole` (Task 7). ✅
- `ReplaceAnimeStaff`/`GetStaffByAnimeID` signatures identical across repo (Task 2), `staffRepo` interface (Task 4). ✅
- `domain.CharacterSeyu` (Task 5) ↔ `ApiCharacterSeyu` (Task 9) field names align (`shikimori_id`, `name`, `name_ru`, `image_url`, `url`). ✅
- `Anime` fields added in Task 6 (`studios`, `releasedOn`, `materialSource`, `ageRating`) are exactly those read in Task 8. ✅
- `StaffMember.roleKey` (Task 7) ↔ `anime.roles.*` keys (Task 8) use the same camelCase suffixes. ✅
