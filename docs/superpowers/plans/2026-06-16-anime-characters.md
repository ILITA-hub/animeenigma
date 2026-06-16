# Anime Characters Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Characters section to the anime page and a dedicated character detail page, sourced from Shikimori, stored in Postgres and hot-cached in Redis.

**Architecture:** New `Character` + `AnimeCharacter` Postgres tables in catalog. A `CharacterService` fetches from Shikimori GraphQL (`characterRoles` for an anime's list, `characters(ids:)` for one character), upserts to Postgres (durable), caches in Redis (6h hot, populate-if-absent, Shikimori-down → serve from Postgres). Two new endpoints proxied gateway → catalog. Frontend adds a `#section-characters` carousel on `Anime.vue`, a `CharacterCard.vue`, and a `/characters/:id` view.

**Tech Stack:** Go (GORM, chi, `hasura/go-graphql-client` raw POST), Vue 3 `<script setup>` + TypeScript, Vitest, Tailwind v4 semantic tokens.

**Out of scope (YAGNI):** voice actors/seiyu (Shikimori can't pair them), character image galleries, "all anime featuring this character", character search, standalone all-characters index, background re-sync scheduler. See spec: `docs/superpowers/specs/2026-06-16-anime-characters-design.md`.

---

## File Structure

**Backend (catalog):**
- Create `services/catalog/internal/domain/character.go` — `Character`, `AnimeCharacter`, `AnimeCharacterView` models.
- Create `services/catalog/internal/parser/shikimori/characters.go` — `GetAnimeCharacters`, `GetCharacterByID`, `sanitizeDescription`, `postRaw` helper.
- Create `services/catalog/internal/parser/shikimori/characters_test.go` — sanitizer + role-mapping tests.
- Create `services/catalog/internal/repo/character.go` — `CharacterRepository`.
- Create `services/catalog/internal/repo/character_test.go` — ordering + upsert tests (testcontainers, build tag).
- Create `services/catalog/internal/service/character.go` — `CharacterService`.
- Create `services/catalog/internal/service/character_test.go` — cache/populate/fallback tests with fakes.
- Create `services/catalog/internal/handler/character.go` — `CharacterHandler`.
- Modify `services/catalog/internal/transport/router.go` — `NewRouter` signature + routes.
- Modify `services/catalog/cmd/catalog-api/main.go` — AutoMigrate, repo/service/handler wiring, `NewRouter` call.
- Modify `services/catalog/internal/domain/anime.go` — (none needed; `AnimeCharacter` is a standalone table, no m2m field on `Anime`).
- Modify `libs/cache/ttl.go` — `PrefixCharacter`, `KeyAnimeCharacters`, `KeyCharacter`.
- Modify `services/gateway/internal/transport/router.go` — `/characters` + `/characters/*` proxy routes.

**Frontend:**
- Modify `frontend/web/src/api/client.ts` — `charactersApi`.
- Create `frontend/web/src/types/character.ts` — `Character`, `AnimeCharacter`, `CharacterCardModel`.
- Create `frontend/web/src/composables/useCharacters.ts` — `useCharacters()` + `useCharacter()`.
- Create `frontend/web/src/components/anime/CharacterCard.vue` + `.spec.ts`.
- Create `frontend/web/src/views/Character.vue`.
- Modify `frontend/web/src/router/index.ts` — `/characters/:id` route.
- Modify `frontend/web/src/views/Anime.vue` — `#section-characters`.
- Modify `frontend/web/src/locales/{en,ru,ja}.json` — `characters.*` namespace.

---

## Task 1: Cache keys + TTL

**Files:**
- Modify: `libs/cache/ttl.go` (const block ~`:40`, key builders ~`:56`)

- [ ] **Step 1: Add the character prefix**

In `libs/cache/ttl.go`, inside the `const (...)` prefix block (after `PrefixStudio = "studio:"`), add:

```go
	PrefixCharacter    = "character:"
```

- [ ] **Step 2: Add the key builders**

After the `KeySimilarAnime` function (~`:115`), add:

```go
// KeyAnimeCharacters is the cache key for an anime's character list.
// TTL = TTLAnimeDetails (6h). Mirrors KeyRelatedAnime / KeySimilarAnime.
func KeyAnimeCharacters(animeID string) string {
	return PrefixAnime + "characters:" + animeID
}

// KeyCharacter is the cache key for a single character's detail row,
// keyed by Shikimori character id. TTL = TTLAnimeDetails (6h).
func KeyCharacter(shikimoriID string) string {
	return PrefixCharacter + shikimoriID
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /data/animeenigma && go build ./libs/cache/`
Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git commit libs/cache/ttl.go -m "feat(cache): character cache keys + prefix"
```

---

## Task 2: Domain models

**Files:**
- Create: `services/catalog/internal/domain/character.go`

- [ ] **Step 1: Write the models**

Create `services/catalog/internal/domain/character.go`:

```go
package domain

import (
	"time"

	"gorm.io/gorm"
)

// Character is an anime character sourced from Shikimori GraphQL.
// Stored durably in Postgres; the catalog service hot-caches it in Redis.
// Synonyms are stored as a single " / "-joined string to avoid a pq array
// dependency (the frontend never splits them — display only).
type Character struct {
	ID          string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ShikimoriID string         `gorm:"size:50;uniqueIndex" json:"shikimori_id"`
	MalID       string         `gorm:"size:50;index" json:"mal_id,omitempty"`
	Name        string         `gorm:"size:500;index" json:"name"`              // English/romaji
	NameRU      string         `gorm:"size:500" json:"name_ru,omitempty"`
	NameJP      string         `gorm:"size:500" json:"name_jp,omitempty"`
	Synonyms    string         `gorm:"size:1000" json:"synonyms,omitempty"`
	PosterURL   string         `gorm:"size:1000" json:"poster_url,omitempty"`
	Description string         `gorm:"type:text" json:"description,omitempty"`  // sanitized plain text
	URL         string         `gorm:"size:1000" json:"url,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// AnimeCharacter is the explicit join row for the anime <-> character
// relation. Managed directly by CharacterRepository (NOT a GORM m2m
// association on Anime) so we control ordering (Main before Supporting,
// then Position). Composite PK (AnimeID, CharacterID) prevents dup joins.
type AnimeCharacter struct {
	AnimeID     string    `gorm:"type:uuid;primaryKey" json:"anime_id"`
	CharacterID string    `gorm:"type:uuid;primaryKey" json:"character_id"`
	Role        string    `gorm:"size:20;index" json:"role"` // "main" / "supporting"
	Position    int       `gorm:"default:0" json:"position"`
	CreatedAt   time.Time `json:"created_at"`
}

// AnimeCharacterView is the flattened read model returned by
// CharacterRepository.GetByAnimeID — a Character plus its per-anime
// role/position. Populated via a raw JOIN scan.
type AnimeCharacterView struct {
	Character
	Role     string `json:"role" gorm:"column:role"`
	Position int    `json:"position" gorm:"column:position"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /data/animeenigma/services/catalog && go build ./internal/domain/`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git commit services/catalog/internal/domain/character.go -m "feat(catalog): Character + AnimeCharacter domain models"
```

---

## Task 3: Shikimori description sanitizer (TDD)

**Files:**
- Create: `services/catalog/internal/parser/shikimori/characters.go`
- Test: `services/catalog/internal/parser/shikimori/characters_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/catalog/internal/parser/shikimori/characters_test.go`:

```go
package shikimori

import "testing"

func TestSanitizeDescription(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "A strong mage.", "A strong mage."},
		{"empty", "", ""},
		{
			"character link",
			"Приёмная дочь [character=196826]Хайтера[/character].",
			"Приёмная дочь Хайтера.",
		},
		{
			"multiple tags",
			"[b]Fern[/b] is a pupil of [character=184947]Frieren[/character].",
			"Fern is a pupil of Frieren.",
		},
		{
			"url tag",
			"See [url=https://x.test]source[/url] here.",
			"See source here.",
		},
		{"trim", "  spaced  ", "spaced"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeDescription(tc.in); got != tc.want {
				t.Fatalf("sanitizeDescription(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeRole(t *testing.T) {
	cases := map[string]string{
		"Main":       "main",
		"Supporting": "supporting",
		"":           "supporting",
		"Background":  "supporting",
	}
	for in, want := range cases {
		if got := normalizeRole([]string{in}); got != want {
			t.Fatalf("normalizeRole([%q]) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeRole(nil); got != "supporting" {
		t.Fatalf("normalizeRole(nil) = %q, want supporting", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/services/catalog && go test ./internal/parser/shikimori/ -run 'TestSanitizeDescription|TestNormalizeRole' -v`
Expected: FAIL — `undefined: sanitizeDescription` / `undefined: normalizeRole`.

- [ ] **Step 3: Write the helpers**

Create `services/catalog/internal/parser/shikimori/characters.go`:

```go
package shikimori

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// bbcodeTagRe matches a single Shikimori bbcode tag, e.g. "[character=196826]"
// or "[/character]" or "[b]" — anything bracketed with no nested brackets.
// Removing every tag while keeping inner text turns
// "[character=1]Name[/character]" into "Name".
var bbcodeTagRe = regexp.MustCompile(`\[[^\[\]]*\]`)

// sanitizeDescription strips Shikimori bbcode/anchor markup to plain text.
// We store/return ONLY plain text — never raw descriptionHtml — so there is
// no external link or XSS surface.
func sanitizeDescription(s string) string {
	return strings.TrimSpace(bbcodeTagRe.ReplaceAllString(s, ""))
}

// normalizeRole collapses Shikimori's rolesEn (e.g. ["Main"], ["Supporting"])
// to our two-value role: "main" for Main, "supporting" for everything else.
func normalizeRole(rolesEn []string) string {
	if len(rolesEn) > 0 && strings.EqualFold(rolesEn[0], "Main") {
		return "main"
	}
	return "supporting"
}

// postRaw POSTs a raw GraphQL query and unmarshals the `data` field into out.
// Mirrors executeRawQuery (client.go) but is generic over the data shape.
func (c *Client) postRaw(ctx context.Context, query string, out interface{}) error {
	reqBody := map[string]string{"query": query}
	jsonBody, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.GraphQLURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return errors.ExternalAPI("shikimori", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.ExternalAPI("shikimori", err)
	}
	defer resp.Body.Close()

	envelope := struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return errors.ExternalAPI("shikimori", err)
	}
	if len(envelope.Errors) > 0 {
		return errors.ExternalAPI("shikimori", fmt.Errorf("%s", envelope.Errors[0].Message))
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return errors.ExternalAPI("shikimori", err)
	}
	return nil
}

// CharacterRoleResult is one character + its role on a given anime.
type CharacterRoleResult struct {
	Character domain.Character
	Role      string // "main" / "supporting"
}

// GetAnimeCharacters fetches an anime's character roles from Shikimori.
// characterRoles returns characters only (not staff). Description is NOT
// fetched here (list view doesn't need it) — see GetCharacterByID.
func (c *Client) GetAnimeCharacters(ctx context.Context, shikimoriID string) ([]CharacterRoleResult, error) {
	c.rateLimiter.acquire()

	query := fmt.Sprintf(`{
		animes(ids: "%s", limit: 1) {
			characterRoles {
				rolesEn
				rolesRu
				character {
					id malId name russian japanese
					poster { originalUrl }
				}
			}
		}
	}`, shikimoriID)

	var data struct {
		Animes []struct {
			CharacterRoles []struct {
				RolesEn   []string `json:"rolesEn"`
				RolesRu   []string `json:"rolesRu"`
				Character struct {
					ID       string `json:"id"`
					MalID    string `json:"malId"`
					Name     string `json:"name"`
					Russian  string `json:"russian"`
					Japanese string `json:"japanese"`
					Poster   *struct {
						OriginalURL string `json:"originalUrl"`
					} `json:"poster"`
				} `json:"character"`
			} `json:"characterRoles"`
		} `json:"animes"`
	}

	if err := c.postRaw(ctx, query, &data); err != nil {
		return nil, err
	}
	if len(data.Animes) == 0 {
		return nil, errors.NotFound("anime")
	}

	roles := data.Animes[0].CharacterRoles
	results := make([]CharacterRoleResult, 0, len(roles))
	for _, r := range roles {
		ch := domain.Character{
			ShikimoriID: r.Character.ID,
			MalID:       r.Character.MalID,
			Name:        r.Character.Name,
			NameRU:      r.Character.Russian,
			NameJP:      r.Character.Japanese,
		}
		if r.Character.Poster != nil {
			ch.PosterURL = r.Character.Poster.OriginalURL
		}
		results = append(results, CharacterRoleResult{Character: ch, Role: normalizeRole(r.RolesEn)})
	}
	return results, nil
}

// GetCharacterByID fetches a single character's detail (with sanitized
// description) from Shikimori GraphQL.
func (c *Client) GetCharacterByID(ctx context.Context, shikimoriID string) (*domain.Character, error) {
	c.rateLimiter.acquire()

	query := fmt.Sprintf(`{
		characters(ids: "%s") {
			id malId name russian japanese synonyms url
			poster { originalUrl }
			description
		}
	}`, shikimoriID)

	var data struct {
		Characters []struct {
			ID          string   `json:"id"`
			MalID       string   `json:"malId"`
			Name        string   `json:"name"`
			Russian     string   `json:"russian"`
			Japanese    string   `json:"japanese"`
			Synonyms    []string `json:"synonyms"`
			URL         string   `json:"url"`
			Description string   `json:"description"`
			Poster      *struct {
				OriginalURL string `json:"originalUrl"`
			} `json:"poster"`
		} `json:"characters"`
	}

	if err := c.postRaw(ctx, query, &data); err != nil {
		return nil, err
	}
	if len(data.Characters) == 0 {
		return nil, errors.NotFound("character")
	}

	src := data.Characters[0]
	ch := &domain.Character{
		ShikimoriID: src.ID,
		MalID:       src.MalID,
		Name:        src.Name,
		NameRU:      src.Russian,
		NameJP:      src.Japanese,
		Synonyms:    strings.Join(src.Synonyms, " / "),
		URL:         src.URL,
		Description: sanitizeDescription(src.Description),
	}
	if src.Poster != nil {
		ch.PosterURL = src.Poster.OriginalURL
	}
	return ch, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /data/animeenigma/services/catalog && go test ./internal/parser/shikimori/ -run 'TestSanitizeDescription|TestNormalizeRole' -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Verify the package builds**

Run: `cd /data/animeenigma/services/catalog && go build ./internal/parser/shikimori/`
Expected: no output (success).

- [ ] **Step 6: Commit**

```bash
git commit services/catalog/internal/parser/shikimori/characters.go services/catalog/internal/parser/shikimori/characters_test.go -m "feat(catalog): Shikimori character queries + bbcode sanitizer"
```

---

## Task 4: Character repository (TDD with testcontainers)

**Files:**
- Create: `services/catalog/internal/repo/character.go`
- Test: `services/catalog/internal/repo/character_test.go`

> The repo tests follow the existing testcontainers pattern in this package. Check an existing `*_test.go` in `services/catalog/internal/repo/` first to copy the exact DB-bootstrap helper name and build tag (e.g. `//go:build integration`). The steps below assume a helper `newTestDB(t)` returning `*gorm.DB`; rename to match the package's actual helper.

- [ ] **Step 1: Write the repository**

Create `services/catalog/internal/repo/character.go`:

```go
package repo

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// CharacterRepository persists characters and the anime<->character join.
type CharacterRepository struct {
	db *gorm.DB
}

func NewCharacterRepository(db *gorm.DB) *CharacterRepository {
	return &CharacterRepository{db: db}
}

// UpsertCharacter inserts or updates a character by shikimori_id and returns
// the stored row (with its generated UUID id populated).
func (r *CharacterRepository) UpsertCharacter(ctx context.Context, ch *domain.Character) (*domain.Character, error) {
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "shikimori_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"mal_id", "name", "name_ru", "name_jp", "synonyms", "poster_url", "description", "url", "updated_at"}),
		}).
		Create(ch).Error; err != nil {
		return nil, err
	}
	// On conflict the returned ch.ID may be empty (no RETURNING for the
	// existing row) — re-read by shikimori_id to get the canonical UUID.
	var stored domain.Character
	if err := r.db.WithContext(ctx).Where("shikimori_id = ?", ch.ShikimoriID).First(&stored).Error; err != nil {
		return nil, err
	}
	return &stored, nil
}

// GetByShikimoriID returns a single stored character, or gorm.ErrRecordNotFound.
func (r *CharacterRepository) GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Character, error) {
	var ch domain.Character
	if err := r.db.WithContext(ctx).Where("shikimori_id = ?", shikimoriID).First(&ch).Error; err != nil {
		return nil, err
	}
	return &ch, nil
}

// ReplaceAnimeCharacters upserts every character and rebuilds the anime's
// join rows in one transaction. Position is the index within roles as
// passed (Shikimori order). Role comes from each CharacterRoleResult.
func (r *CharacterRepository) ReplaceAnimeCharacters(ctx context.Context, animeID string, rows []domain.AnimeCharacter, chars []domain.Character) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range chars {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "shikimori_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"mal_id", "name", "name_ru", "name_jp", "poster_url", "updated_at"}),
			}).Create(&chars[i]).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("anime_id = ?", animeID).Delete(&domain.AnimeCharacter{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

// GetByAnimeID returns the anime's characters ordered Main-first then Position.
func (r *CharacterRepository) GetByAnimeID(ctx context.Context, animeID string) ([]domain.AnimeCharacterView, error) {
	var views []domain.AnimeCharacterView
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.*, ac.role, ac.position
		FROM characters c
		JOIN anime_characters ac ON ac.character_id = c.id
		WHERE ac.anime_id = ? AND c.deleted_at IS NULL
		ORDER BY CASE WHEN ac.role = 'main' THEN 0 ELSE 1 END, ac.position
	`, animeID).Scan(&views).Error
	if err != nil {
		return nil, err
	}
	return views, nil
}
```

- [ ] **Step 2: Write the failing test**

Create `services/catalog/internal/repo/character_test.go` (adjust build tag + `newTestDB` to match the package):

```go
//go:build integration

package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestCharacterRepo_UpsertAndGetByAnime_OrdersMainFirst(t *testing.T) {
	db := newTestDB(t)
	if err := db.AutoMigrate(&domain.Anime{}, &domain.Character{}, &domain.AnimeCharacter{}); err != nil {
		t.Fatal(err)
	}
	repo := NewCharacterRepository(db)
	ctx := context.Background()

	anime := domain.Anime{Name: "Frieren"}
	if err := db.Create(&anime).Error; err != nil {
		t.Fatal(err)
	}

	chars := []domain.Character{
		{ShikimoriID: "1", Name: "Stark", NameRU: "Штарк"},      // supporting, pos 0
		{ShikimoriID: "2", Name: "Frieren", NameRU: "Фрирен"},   // main, pos 1
	}
	// Upsert first to get UUIDs, then build join rows.
	c0, err := repo.UpsertCharacter(ctx, &chars[0])
	if err != nil {
		t.Fatal(err)
	}
	c1, err := repo.UpsertCharacter(ctx, &chars[1])
	if err != nil {
		t.Fatal(err)
	}
	rows := []domain.AnimeCharacter{
		{AnimeID: anime.ID, CharacterID: c0.ID, Role: "supporting", Position: 0},
		{AnimeID: anime.ID, CharacterID: c1.ID, Role: "main", Position: 1},
	}
	if err := repo.ReplaceAnimeCharacters(ctx, anime.ID, rows, nil); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByAnimeID(ctx, anime.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d characters, want 2", len(got))
	}
	if got[0].Name != "Frieren" || got[0].Role != "main" {
		t.Fatalf("first should be the main character Frieren, got %q (%s)", got[0].Name, got[0].Role)
	}
	if got[1].Name != "Stark" {
		t.Fatalf("second should be Stark, got %q", got[1].Name)
	}
}
```

- [ ] **Step 3: Run test to verify it fails, then passes**

Run: `cd /data/animeenigma/services/catalog && go test -tags=integration ./internal/repo/ -run TestCharacterRepo -v`
Expected: PASS (after Step 1 is in place). If your machine can't run testcontainers, at minimum verify the build: `go vet -tags=integration ./internal/repo/`.

- [ ] **Step 4: Verify the package builds**

Run: `cd /data/animeenigma/services/catalog && go build ./internal/repo/`
Expected: no output (success).

- [ ] **Step 5: Commit**

```bash
git commit services/catalog/internal/repo/character.go services/catalog/internal/repo/character_test.go -m "feat(catalog): CharacterRepository with main-first ordering"
```

---

## Task 5: Character service with cache + fallback (TDD with fakes)

**Files:**
- Create: `services/catalog/internal/service/character.go`
- Test: `services/catalog/internal/service/character_test.go`

> Uses handwritten fakes (no testify/mock), per project convention. The service depends on small interfaces it declares itself so the test can fake them.

- [ ] **Step 1: Write the service**

Create `services/catalog/internal/service/character.go`:

```go
package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
)

// characterShikimori is the slice of the Shikimori client this service needs.
type characterShikimori interface {
	GetAnimeCharacters(ctx context.Context, shikimoriID string) ([]shikimori.CharacterRoleResult, error)
	GetCharacterByID(ctx context.Context, shikimoriID string) (*domain.Character, error)
}

// characterRepo is the slice of CharacterRepository this service needs.
type characterRepo interface {
	UpsertCharacter(ctx context.Context, ch *domain.Character) (*domain.Character, error)
	GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Character, error)
	ReplaceAnimeCharacters(ctx context.Context, animeID string, rows []domain.AnimeCharacter, chars []domain.Character) error
	GetByAnimeID(ctx context.Context, animeID string) ([]domain.AnimeCharacterView, error)
}

// animeShikimoriIDLookup resolves a catalog anime UUID to its Shikimori id.
type animeShikimoriIDLookup interface {
	GetByID(ctx context.Context, id string) (*domain.Anime, error)
}

// CharacterService orchestrates Shikimori fetch → Postgres upsert → Redis cache.
type CharacterService struct {
	animeRepo animeShikimoriIDLookup
	chars     characterRepo
	shikimori characterShikimori
	cache     *cache.RedisCache
	log       *logger.Logger
}

func NewCharacterService(
	animeRepo animeShikimoriIDLookup,
	chars characterRepo,
	shiki characterShikimori,
	c *cache.RedisCache,
	log *logger.Logger,
) *CharacterService {
	return &CharacterService{animeRepo: animeRepo, chars: chars, shikimori: shiki, cache: c, log: log}
}

// GetAnimeCharacters returns an anime's characters. Flow: Redis → (miss)
// fetch Shikimori → upsert Postgres → cache → return; on Shikimori failure,
// serve last-known-good from Postgres.
func (s *CharacterService) GetAnimeCharacters(ctx context.Context, animeID string) ([]domain.AnimeCharacterView, error) {
	cacheKey := cache.KeyAnimeCharacters(animeID)
	var cached []domain.AnimeCharacterView
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return cached, nil
	}

	anime, err := s.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	roles, ferr := s.shikimori.GetAnimeCharacters(ctx, anime.ShikimoriID)
	if ferr != nil {
		// Shikimori down — serve last-known-good from Postgres.
		s.log.Warnw("shikimori characters fetch failed, serving from db", "anime_id", animeID, "error", ferr)
		return s.chars.GetByAnimeID(ctx, animeID)
	}

	chars := make([]domain.Character, 0, len(roles))
	rows := make([]domain.AnimeCharacter, 0, len(roles))
	for i, r := range roles {
		stored, uerr := s.chars.UpsertCharacter(ctx, &r.Character)
		if uerr != nil {
			return nil, uerr
		}
		rows = append(rows, domain.AnimeCharacter{
			AnimeID:     animeID,
			CharacterID: stored.ID,
			Role:        r.Role,
			Position:    i,
		})
	}
	if err := s.chars.ReplaceAnimeCharacters(ctx, animeID, rows, chars); err != nil {
		return nil, err
	}

	views, err := s.chars.GetByAnimeID(ctx, animeID)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Set(ctx, cacheKey, views, cache.TTLAnimeDetails)
	return views, nil
}

// GetCharacter returns a single character by Shikimori id. Flow: Redis →
// (miss) Shikimori → upsert Postgres → cache; on Shikimori failure, Postgres.
func (s *CharacterService) GetCharacter(ctx context.Context, shikimoriID string) (*domain.Character, error) {
	cacheKey := cache.KeyCharacter(shikimoriID)
	var cached domain.Character
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil {
		return &cached, nil
	}

	ch, ferr := s.shikimori.GetCharacterByID(ctx, shikimoriID)
	if ferr != nil {
		s.log.Warnw("shikimori character fetch failed, serving from db", "shikimori_id", shikimoriID, "error", ferr)
		return s.chars.GetByShikimoriID(ctx, shikimoriID)
	}

	stored, err := s.chars.UpsertCharacter(ctx, ch)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Set(ctx, cacheKey, stored, cache.TTLAnimeDetails)
	return stored, nil
}
```

> Note: `ReplaceAnimeCharacters` is called with an empty `chars` slice here because each character was already upserted individually in the loop (to obtain its UUID for the join rows). The `chars` param exists for callers that batch-upsert; passing `nil`/empty is valid (the repo loop just no-ops).

- [ ] **Step 2: Write the failing test**

Create `services/catalog/internal/service/character_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
)

type fakeAnimeRepo struct{ anime *domain.Anime }

func (f *fakeAnimeRepo) GetByID(_ context.Context, _ string) (*domain.Anime, error) {
	return f.anime, nil
}

type fakeShikimori struct {
	roles   []shikimori.CharacterRoleResult
	rolesErr error
}

func (f *fakeShikimori) GetAnimeCharacters(_ context.Context, _ string) ([]shikimori.CharacterRoleResult, error) {
	return f.roles, f.rolesErr
}
func (f *fakeShikimori) GetCharacterByID(_ context.Context, _ string) (*domain.Character, error) {
	return nil, errors.New("not used")
}

type fakeCharRepo struct {
	upserted []domain.Character
	byAnime  []domain.AnimeCharacterView
	replaced bool
}

func (f *fakeCharRepo) UpsertCharacter(_ context.Context, ch *domain.Character) (*domain.Character, error) {
	out := *ch
	out.ID = "uuid-" + ch.ShikimoriID
	f.upserted = append(f.upserted, out)
	return &out, nil
}
func (f *fakeCharRepo) GetByShikimoriID(_ context.Context, _ string) (*domain.Character, error) {
	return nil, errors.New("not used")
}
func (f *fakeCharRepo) ReplaceAnimeCharacters(_ context.Context, _ string, _ []domain.AnimeCharacter, _ []domain.Character) error {
	f.replaced = true
	return nil
}
func (f *fakeCharRepo) GetByAnimeID(_ context.Context, _ string) ([]domain.AnimeCharacterView, error) {
	return f.byAnime, nil
}

func newSvc(animeRepo animeShikimoriIDLookup, repo characterRepo, shiki characterShikimori) *CharacterService {
	// cache nil-safe: pass a real RedisCache only in integration; here we
	// rely on Get returning an error (miss) for a nil-less fake. Use the
	// project's in-memory cache helper if available; otherwise this test
	// targets the fetch path and a miss is simulated by an empty cache.
	return &CharacterService{animeRepo: animeRepo, chars: repo, shikimori: shiki, cache: newMissCache(), log: logger.Default()}
}

func TestGetAnimeCharacters_FetchesUpsertsAndReturns(t *testing.T) {
	animeRepo := &fakeAnimeRepo{anime: &domain.Anime{ID: "anime-uuid", ShikimoriID: "52991"}}
	shiki := &fakeShikimori{roles: []shikimori.CharacterRoleResult{
		{Character: domain.Character{ShikimoriID: "2", Name: "Frieren"}, Role: "main"},
		{Character: domain.Character{ShikimoriID: "1", Name: "Stark"}, Role: "supporting"},
	}}
	repo := &fakeCharRepo{byAnime: []domain.AnimeCharacterView{
		{Character: domain.Character{Name: "Frieren"}, Role: "main"},
	}}
	svc := newSvc(animeRepo, repo, shiki)

	got, err := svc.GetAnimeCharacters(context.Background(), "anime-uuid")
	if err != nil {
		t.Fatal(err)
	}
	if !repo.replaced {
		t.Fatal("expected join rows to be replaced")
	}
	if len(repo.upserted) != 2 {
		t.Fatalf("expected 2 upserts, got %d", len(repo.upserted))
	}
	if len(got) != 1 || got[0].Role != "main" {
		t.Fatalf("expected db read-back, got %+v", got)
	}
}

func TestGetAnimeCharacters_ShikimoriDown_ServesFromDB(t *testing.T) {
	animeRepo := &fakeAnimeRepo{anime: &domain.Anime{ID: "anime-uuid", ShikimoriID: "52991"}}
	shiki := &fakeShikimori{rolesErr: errors.New("shikimori 503")}
	repo := &fakeCharRepo{byAnime: []domain.AnimeCharacterView{
		{Character: domain.Character{Name: "StaleFrieren"}, Role: "main"},
	}}
	svc := newSvc(animeRepo, repo, shiki)

	got, err := svc.GetAnimeCharacters(context.Background(), "anime-uuid")
	if err != nil {
		t.Fatal(err)
	}
	if repo.replaced {
		t.Fatal("should NOT replace when Shikimori is down")
	}
	if len(got) != 1 || got[0].Name != "StaleFrieren" {
		t.Fatalf("expected stale db rows, got %+v", got)
	}
}
```

> **Cache fake:** this test needs a `cache.RedisCache` whose `Get` always misses. Check `libs/cache` for an existing in-memory/miniredis test helper (e.g. `cache.NewNoop()` or a miniredis constructor used elsewhere in catalog tests) and replace `newMissCache()` with it. If none exists, the smallest path is to use the miniredis-backed `RedisCache` already used by other catalog service tests — copy that setup. Do not invent a new cache type.

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd /data/animeenigma/services/catalog && go test ./internal/service/ -run TestGetAnimeCharacters -v`
Expected: PASS (both cases).

- [ ] **Step 4: Commit**

```bash
git commit services/catalog/internal/service/character.go services/catalog/internal/service/character_test.go -m "feat(catalog): CharacterService with Redis cache + Postgres fallback"
```

---

## Task 6: Character HTTP handler

**Files:**
- Create: `services/catalog/internal/handler/character.go`

- [ ] **Step 1: Write the handler**

Create `services/catalog/internal/handler/character.go`:

```go
package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service"
)

// CharacterHandler serves anime-character endpoints.
type CharacterHandler struct {
	svc *service.CharacterService
	log *logger.Logger
}

func NewCharacterHandler(svc *service.CharacterService, log *logger.Logger) *CharacterHandler {
	return &CharacterHandler{svc: svc, log: log}
}

// GetAnimeCharacters handles GET /api/anime/{animeId}/characters.
func (h *CharacterHandler) GetAnimeCharacters(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		httputil.BadRequest(w, "anime ID is required")
		return
	}
	list, err := h.svc.GetAnimeCharacters(r.Context(), animeID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, list)
}

// GetCharacter handles GET /api/characters/{characterId} (Shikimori id).
func (h *CharacterHandler) GetCharacter(w http.ResponseWriter, r *http.Request) {
	characterID := chi.URLParam(r, "characterId")
	if characterID == "" {
		httputil.BadRequest(w, "character ID is required")
		return
	}
	ch, err := h.svc.GetCharacter(r.Context(), characterID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, ch)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /data/animeenigma/services/catalog && go build ./internal/handler/`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git commit services/catalog/internal/handler/character.go -m "feat(catalog): character HTTP handler"
```

---

## Task 7: Wire routes + DI in catalog

**Files:**
- Modify: `services/catalog/internal/transport/router.go` (`NewRouter` signature ~`:17`, `/anime` block ~`:116`)
- Modify: `services/catalog/cmd/catalog-api/main.go` (AutoMigrate ~`:80`, wiring ~`:177`, `NewRouter` call ~`:379`)

- [ ] **Step 1: Add `characterHandler` to `NewRouter` signature**

In `services/catalog/internal/transport/router.go`, add `characterHandler *handler.CharacterHandler` to the `NewRouter(...)` parameter list (place it right after `catalogHandler`).

- [ ] **Step 2: Register the routes**

Inside the existing `r.Route("/anime", func(r chi.Router) { ... })` block, add (literal/static segments already precede `/{animeId}`; this nested static path is safe after `/{animeId}`):

```go
			r.Get("/{animeId}/characters", characterHandler.GetAnimeCharacters)
```

Then, as a SIBLING of the `r.Route("/anime", ...)` block (inside the same `/api` route group), add:

```go
		// Public character routes
		r.Route("/characters", func(r chi.Router) {
			r.Get("/{characterId}", characterHandler.GetCharacter)
		})
```

- [ ] **Step 3: Add models to AutoMigrate**

In `services/catalog/cmd/catalog-api/main.go`, inside the `db.AutoMigrate(...)` call (after `&domain.ScraperProvider{}`), add:

```go
		&domain.Character{},
		&domain.AnimeCharacter{},
```

> No `SetupJoinTable` / `db.DB.AutoMigrate(&domain.Anime{})` needed — `AnimeCharacter` is a standalone table managed by `CharacterRepository`, not a GORM m2m association on `Anime`.

- [ ] **Step 4: Construct repo, service, handler**

In `main.go`, after `videoRepo := repo.NewVideoRepository(db.DB)`, add:

```go
	characterRepo := repo.NewCharacterRepository(db.DB)
```

After `catalogService := service.NewCatalogService(...)`, add:

```go
	characterService := service.NewCharacterService(animeRepo, characterRepo, shikimoriClient, redisCache, log)
```

After `catalogHandler := handler.NewCatalogHandler(catalogService, log)`, add:

```go
	characterHandler := handler.NewCharacterHandler(characterService, log)
```

- [ ] **Step 5: Pass `characterHandler` to the `NewRouter` call**

In the `transport.NewRouter(catalogHandler, adminHandler, ...)` call (~`:379`), insert `characterHandler` right after `catalogHandler`.

> `NewCharacterService` takes `animeRepo` as its `animeShikimoriIDLookup` — `*repo.AnimeRepository` must have a `GetByID(ctx, id) (*domain.Anime, error)` method. Confirm by grep: `grep -n "func (r \*AnimeRepository) GetByID" services/catalog/internal/repo/anime.go`. It does (CatalogService.GetAnime uses it). If its signature differs, adapt the `animeShikimoriIDLookup` interface to match.

- [ ] **Step 6: Build the whole service**

Run: `cd /data/animeenigma/services/catalog && go build ./...`
Expected: no output (success).

- [ ] **Step 7: Run the full catalog test suite**

Run: `cd /data/animeenigma/services/catalog && go test ./... 2>&1 | tail -20`
Expected: all packages PASS (or `ok`/`no test files`).

- [ ] **Step 8: Commit**

```bash
git commit services/catalog/internal/transport/router.go services/catalog/cmd/catalog-api/main.go -m "feat(catalog): wire character routes + DI + migrations"
```

---

## Task 8: Gateway proxy routes

**Files:**
- Modify: `services/gateway/internal/transport/router.go` (catalog public block ~`:278`)

- [ ] **Step 1: Add the proxy routes**

In `services/gateway/internal/transport/router.go`, in the public catalog block (next to `/collections` / `/collections/*`), add:

```go
		r.HandleFunc("/characters", proxyHandler.ProxyToCatalog)
		r.HandleFunc("/characters/*", proxyHandler.ProxyToCatalog)
```

> `/anime/*` is already a catch-all to catalog, so `/api/anime/{id}/characters` needs no new gateway line. Only the top-level `/api/characters/*` family needs these.

- [ ] **Step 2: Build the gateway**

Run: `cd /data/animeenigma/services/gateway && go build ./...`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git commit services/gateway/internal/transport/router.go -m "feat(gateway): proxy /api/characters/* to catalog"
```

---

## Task 9: Frontend types + API client

**Files:**
- Create: `frontend/web/src/types/character.ts`
- Modify: `frontend/web/src/api/client.ts` (near other anime-scoped groups)

- [ ] **Step 1: Write the types**

Create `frontend/web/src/types/character.ts`:

```ts
// Raw API shapes (snake_case, mirror the Go json tags).
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
}

export interface ApiAnimeCharacter extends ApiCharacter {
  role: 'main' | 'supporting'
  position: number
}

// Frontend view-model for a character card on the anime page.
export interface CharacterCardModel {
  id: string          // shikimori_id — used in /characters/:id
  name: string        // already localized (RU fallback EN)
  image: string       // proxied poster url
  role: 'main' | 'supporting'
}

// Frontend model for the character detail page.
export interface CharacterDetail {
  shikimoriId: string
  name: string
  nameRu?: string
  nameJp?: string
  synonyms?: string
  image: string
  description?: string
}
```

- [ ] **Step 2: Add the API group**

In `frontend/web/src/api/client.ts`, near the other anime-scoped groups (e.g. after `jimakuApi`), add:

```ts
export const charactersApi = {
  getAnimeCharacters: (animeId: string) =>
    apiClient.get(`/anime/${animeId}/characters`),
  getCharacter: (id: string) =>
    apiClient.get(`/characters/${id}`),
}
```

- [ ] **Step 3: Type-check**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git commit frontend/web/src/types/character.ts frontend/web/src/api/client.ts -m "feat(web): character types + charactersApi"
```

---

## Task 10: Composables `useCharacters` / `useCharacter`

**Files:**
- Create: `frontend/web/src/composables/useCharacters.ts`

- [ ] **Step 1: Write the composables**

Create `frontend/web/src/composables/useCharacters.ts`:

```ts
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { charactersApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import type {
  ApiAnimeCharacter,
  ApiCharacter,
  CharacterCardModel,
  CharacterDetail,
} from '@/types/character'

function toCardModel(c: ApiAnimeCharacter): CharacterCardModel {
  return {
    id: c.shikimori_id,
    name: getLocalizedTitle(c.name, c.name_ru, c.name_jp),
    image: getImageUrl(c.poster_url),
    role: c.role === 'main' ? 'main' : 'supporting',
  }
}

function toDetail(c: ApiCharacter): CharacterDetail {
  return {
    shikimoriId: c.shikimori_id,
    name: getLocalizedTitle(c.name, c.name_ru, c.name_jp),
    nameRu: c.name_ru || undefined,
    nameJp: c.name_jp || undefined,
    synonyms: c.synonyms || undefined,
    image: getImageUrl(c.poster_url),
    description: c.description || undefined,
  }
}

// List of an anime's characters. Fails soft (no global error) — the section
// just stays empty if the fetch fails, like the related rail.
export function useCharacters() {
  const characters = ref<CharacterCardModel[]>([])

  const fetchCharacters = async (animeId: string) => {
    try {
      const resp = await charactersApi.getAnimeCharacters(animeId)
      const data = (resp.data?.data || resp.data) as ApiAnimeCharacter[]
      characters.value = Array.isArray(data) ? data.map(toCardModel) : []
      return characters.value
    } catch (err) {
      console.warn('Failed to fetch characters:', err)
      characters.value = []
      return []
    }
  }

  return { characters, fetchCharacters }
}

// Single character detail (for /characters/:id).
export function useCharacter() {
  const { t } = useI18n()
  const character = ref<CharacterDetail | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)

  const fetchCharacter = async (id: string) => {
    loading.value = true
    error.value = null
    try {
      const resp = await charactersApi.getCharacter(id)
      const data = (resp.data?.data || resp.data) as ApiCharacter
      character.value = toDetail(data)
      return character.value
    } catch (err: unknown) {
      const e = err as { response?: { data?: { message?: string } } }
      error.value = e.response?.data?.message || t('characters.fetchError')
      throw err
    } finally {
      loading.value = false
    }
  }

  return { character, loading, error, fetchCharacter }
}
```

> Confirm `getLocalizedTitle(name, nameRu, nameJp)` is exported from `@/utils/title` (used by `useAnime.ts`). If its arg order differs, match the call in `useAnime.ts:transformAnime`.

- [ ] **Step 2: Type-check**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git commit frontend/web/src/composables/useCharacters.ts -m "feat(web): useCharacters + useCharacter composables"
```

---

## Task 11: `CharacterCard.vue` (TDD)

**Files:**
- Create: `frontend/web/src/components/anime/CharacterCard.vue`
- Test: `frontend/web/src/components/anime/CharacterCard.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/components/anime/CharacterCard.spec.ts`:

```ts
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}))

import CharacterCard from './CharacterCard.vue'
import type { CharacterCardModel } from '@/types/character'

const RouterLinkStub = { name: 'RouterLink', props: ['to'], template: '<a :href="to"><slot /></a>' }

function mountCard(model: Partial<CharacterCardModel> = {}) {
  const full: CharacterCardModel = {
    id: '188176', name: 'Ферн', image: 'http://x/p.jpg', role: 'main', ...model,
  }
  return mount(CharacterCard, {
    props: { model: full },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
}

describe('CharacterCard', () => {
  it('renders the character name', () => {
    expect(mountCard().text()).toContain('Ферн')
  })

  it('links to the character page by shikimori id', () => {
    expect(mountCard().find('a').attributes('href')).toBe('/characters/188176')
  })

  it('shows a Main role badge for main characters', () => {
    const w = mountCard({ role: 'main' })
    expect(w.find('[data-testid="role-badge"]').exists()).toBe(true)
    expect(w.text()).toContain('characters.main')
  })

  it('shows a Supporting role badge for supporting characters', () => {
    expect(mountCard({ role: 'supporting' }).text()).toContain('characters.supporting')
  })

  it('renders the portrait image with the character name as alt', () => {
    const img = mountCard().find('img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('alt')).toBe('Ферн')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/anime/CharacterCard.spec.ts`
Expected: FAIL — cannot resolve `./CharacterCard.vue`.

- [ ] **Step 3: Write the component**

Create `frontend/web/src/components/anime/CharacterCard.vue`:

```vue
<template>
  <router-link :to="`/characters/${model.id}`" class="group block" :aria-label="model.name">
    <div class="rounded-xl overflow-hidden bg-white/5 border border-white/10 transition-[border-color,box-shadow] duration-200 group-hover:border-white/20 group-hover:shadow-[0_10px_30px_rgba(0,0,0,0.4)]">
      <div class="relative aspect-[2/3] bg-white/5">
        <img
          :src="model.image"
          :alt="model.name"
          loading="lazy"
          class="absolute inset-0 w-full h-full object-cover"
        />
        <div class="absolute top-2 left-2">
          <Badge
            data-testid="role-badge"
            :variant="model.role === 'main' ? 'primary' : 'default'"
            size="sm"
            :overlay="true"
          >
            {{ model.role === 'main' ? $t('characters.main') : $t('characters.supporting') }}
          </Badge>
        </div>
      </div>
      <div class="px-2.5 pt-2.5 pb-3">
        <h3 class="font-medium text-white line-clamp-2 text-[13px] leading-[1.3] min-h-[2.6em] group-hover:text-cyan-400 transition-colors">
          {{ model.name }}
        </h3>
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import Badge from '@/components/ui/Badge.vue'
import type { CharacterCardModel } from '@/types/character'

defineProps<{ model: CharacterCardModel }>()
</script>
```

> DS-lint note: `group-hover:text-cyan-400` is allowed (cyan is an exempt brand hue). `Badge` `variant` values must be ones it actually supports — confirm `primary`/`default` exist in `Badge.vue` (PosterCard uses `default`/`success`/`warning`). If `primary` isn't a variant, use `success` for main.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/anime/CharacterCard.spec.ts`
Expected: PASS (5 assertions).

- [ ] **Step 5: Type-check**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git commit frontend/web/src/components/anime/CharacterCard.vue frontend/web/src/components/anime/CharacterCard.spec.ts -m "feat(web): CharacterCard component"
```

---

## Task 12: i18n keys (en/ru/ja — all three)

**Files:**
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`
- Modify: `frontend/web/src/locales/ja.json`

> All three MUST get the identical key tree or `i18n-lint.sh` fails the redeploy build.

- [ ] **Step 1: Add the `characters` namespace to en.json**

Add a top-level `"characters"` key to `frontend/web/src/locales/en.json`:

```json
  "characters": {
    "detailsTitle": "Character",
    "heading": "Characters",
    "showAll": "Show all",
    "showLess": "Show less",
    "main": "Main",
    "supporting": "Supporting",
    "synonyms": "Also known as",
    "description": "Description",
    "noDescription": "No description available.",
    "empty": "No characters listed",
    "back": "Back",
    "fetchError": "Failed to load character"
  },
```

- [ ] **Step 2: Add the `characters` namespace to ru.json**

```json
  "characters": {
    "detailsTitle": "Персонаж",
    "heading": "Персонажи",
    "showAll": "Показать всех",
    "showLess": "Свернуть",
    "main": "Главный",
    "supporting": "Второстепенный",
    "synonyms": "Также известен как",
    "description": "Описание",
    "noDescription": "Описание отсутствует.",
    "empty": "Список персонажей пуст",
    "back": "Назад",
    "fetchError": "Не удалось загрузить персонажа"
  },
```

- [ ] **Step 3: Add the `characters` namespace to ja.json**

```json
  "characters": {
    "detailsTitle": "キャラクター",
    "heading": "キャラクター",
    "showAll": "すべて表示",
    "showLess": "折りたたむ",
    "main": "主要",
    "supporting": "脇役",
    "synonyms": "別名",
    "description": "説明",
    "noDescription": "説明はありません。",
    "empty": "キャラクターがありません",
    "back": "戻る",
    "fetchError": "キャラクターの読み込みに失敗しました"
  },
```

- [ ] **Step 4: Run the i18n lint gate**

Run: `cd /data/animeenigma && bash frontend/web/scripts/i18n-lint.sh`
Expected: PASS (no missing-key errors).

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json -m "feat(web): characters i18n keys (en/ru/ja)"
```

---

## Task 13: `#section-characters` on Anime.vue

**Files:**
- Modify: `frontend/web/src/views/Anime.vue` (section ~after `:1014`; refs ~`:1335`; lazy wiring ~`:2006-2047`; reset ~`:2460`; fetch fn ~`:2616`)

- [ ] **Step 1: Import the composable and card**

In `Anime.vue`'s `<script setup>`, add near the other component imports:

```ts
import CharacterCard from '@/components/anime/CharacterCard.vue'
import { useCharacters } from '@/composables/useCharacters'
```

And near the other composable instantiations:

```ts
const { characters, fetchCharacters } = useCharacters()
const characterSentinelEl = ref<HTMLElement | null>(null)
```

- [ ] **Step 2: Add the fetch wiring to the lazy observer**

In the IntersectionObserver `observe` setup block (where `relatedSentinelEl` is observed, ~`:2040`), add:

```ts
  if (characterSentinelEl.value) lazySectionObserver.observe(characterSentinelEl.value)
```

And in the observer callback (the `if/else if` chain on `entry.target`, ~`:2010`), add a branch:

```ts
        } else if (entry.target === characterSentinelEl.value) {
          lazySectionObserver.unobserve(entry.target)
          void fetchCharacters(String(anime.value?.id))
```

- [ ] **Step 3: Reset on route change**

Where `relatedAnime.value = []` resets (~`:2460`), add:

```ts
    characters.value = []
```

- [ ] **Step 4: Add the template section**

In the template, right after the `#section-similar` `</section>` (~`:1014`), add:

```vue
      <div ref="characterSentinelEl" aria-hidden="true" />
      <section
        v-if="characters.length > 0"
        id="section-characters"
        class="mt-8 non-player-content"
      >
        <Carousel
          :items="characters"
          :title="$t('characters.heading')"
          item-key="id"
          :item-width="{ mobile: 110, tablet: 128, desktop: 140, large: 150 }"
        >
          <template #default="{ item }">
            <CharacterCard :model="item as CharacterCardModel" />
          </template>
        </Carousel>
      </section>
```

Add the type import if not already present:

```ts
import type { CharacterCardModel } from '@/types/character'
```

> Insertion ordering: placing `#section-characters` after `#section-similar` keeps it the last section. If you'd rather have characters before similar/comments, move the block accordingly — keep it OUTSIDE any `v-if/v-else-if` page-state chain (per project Vue rule: independent sections go after the chain).

- [ ] **Step 5: (Optional) Add to AnimeQuickNav**

If `AnimeQuickNav` lists `#section-*` anchors via `anime.nav.*`, add an `anime.nav.characters` key to all three locales and a nav entry. Skip if the nav is data-driven off present sections. (Not required for the feature to work.)

- [ ] **Step 6: Type-check + build**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/anime/CharacterCard.spec.ts`
Expected: no type errors; card spec PASS.

- [ ] **Step 7: Commit**

```bash
git commit frontend/web/src/views/Anime.vue -m "feat(web): characters section on the anime page"
```

---

## Task 14: `Character.vue` detail view + route

**Files:**
- Create: `frontend/web/src/views/Character.vue`
- Modify: `frontend/web/src/router/index.ts` (after the `/anime/:id` block ~`:81`)

- [ ] **Step 1: Write the view**

Create `frontend/web/src/views/Character.vue`:

```vue
<template>
  <div class="max-w-4xl mx-auto px-4 py-6">
    <router-link to="" class="text-sm text-white/60 hover:text-white" @click.prevent="goBack">
      ← {{ $t('characters.back') }}
    </router-link>

    <div v-if="loading" class="mt-6 flex justify-center">
      <Spinner />
    </div>

    <div v-else-if="error" class="mt-6 text-center text-destructive">
      {{ error }}
    </div>

    <div v-else-if="character" class="mt-6 flex flex-col md:flex-row gap-6">
      <div class="w-48 shrink-0 mx-auto md:mx-0">
        <div class="rounded-xl overflow-hidden bg-white/5 border border-white/10 aspect-[2/3]">
          <img :src="character.image" :alt="character.name" class="w-full h-full object-cover" />
        </div>
      </div>

      <div class="flex-1 min-w-0">
        <h1 class="text-2xl font-semibold text-white">{{ character.name }}</h1>
        <p v-if="character.nameJp" class="text-white/50 mt-1">{{ character.nameJp }}</p>
        <p v-if="character.synonyms" class="text-sm text-white/40 mt-2">
          {{ $t('characters.synonyms') }}: {{ character.synonyms }}
        </p>

        <h2 class="text-sm font-semibold text-white/70 mt-6 mb-2">{{ $t('characters.description') }}</h2>
        <p v-if="character.description" class="text-white/80 whitespace-pre-line leading-relaxed">
          {{ character.description }}
        </p>
        <p v-else class="text-white/40">{{ $t('characters.noDescription') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Spinner from '@/components/ui/Spinner.vue'
import { useCharacter } from '@/composables/useCharacters'

const route = useRoute()
const router = useRouter()
const { character, loading, error, fetchCharacter } = useCharacter()

function goBack() {
  if (window.history.length > 1) router.back()
  else void router.push('/')
}

function load() {
  const id = String(route.params.id ?? '')
  if (id) void fetchCharacter(id)
}

onMounted(load)
watch(() => route.params.id, load)
</script>
```

> Confirm `Spinner.vue` exists at `@/components/ui/Spinner.vue` (it does — per the UI primitives inventory). `text-destructive` is the semantic error token.

- [ ] **Step 2: Add the route**

In `frontend/web/src/router/index.ts`, after the `/anime/:id` route object (~`:81`), add:

```ts
  {
    path: '/characters/:id',
    name: 'character',
    component: () => import('@/views/Character.vue'),
    meta: { titleKey: 'characters.detailsTitle' }
  },
```

- [ ] **Step 3: Type-check**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Run the frontend lint gates**

Run: `cd /data/animeenigma && bash frontend/web/scripts/design-system-lint.sh && bash frontend/web/scripts/i18n-lint.sh`
Expected: both PASS (ERRORS=0).

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/views/Character.vue frontend/web/src/router/index.ts -m "feat(web): character detail page + route"
```

---

## Task 15: Full verification

- [ ] **Step 1: Backend — build + test all touched services**

Run: `cd /data/animeenigma && go build ./services/catalog/... ./services/gateway/... ./libs/cache/... && (cd services/catalog && go test ./... 2>&1 | tail -20)`
Expected: builds clean; catalog tests PASS.

- [ ] **Step 2: Frontend — type-check, unit tests, lints**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/anime/CharacterCard.spec.ts && bash scripts/design-system-lint.sh && bash scripts/i18n-lint.sh`
Expected: all green.

- [ ] **Step 3: Deploy + changelog via the after-update skill**

Invoke `/animeenigma-after-update` to redeploy `catalog`, `gateway`, and `web`, run health checks, update the Russian Trump-mode changelog, and push. (This is the project's mandatory post-implementation step — covers redeploy of the three changed services.)

> Runtime smoke (manual, optional): open an anime page → scroll to Characters → click a card → verify the character page loads name + description. Posters should load through `/api/streaming/image-proxy`.

---

## Self-Review notes (resolved during authoring)

- **Spec coverage:** every spec section maps to a task — data source (T3), Postgres tables (T2/T4), endpoints (T6/T7), gateway route (T8), caching+fallback (T5), sanitization (T3), anime section (T13), character page (T14), card (T11), composables (T10), image proxy (T9/T10 via `getImageUrl`), i18n×3 (T12), tests (T3/T4/T5/T11). Out-of-scope items remain out.
- **Type consistency:** `CharacterRoleResult`, `AnimeCharacterView`, `CharacterCardModel`, `ApiAnimeCharacter` names are used identically across backend↔frontend tasks. Role is the two-value `"main"`/`"supporting"` everywhere (normalized in T3, ordered in T4, badged in T11).
- **Known confirm-before-coding points (flagged inline, not placeholders):** the repo test helper name + build tag (T4), the miniredis/noop cache helper for the service test (T5), `Badge` variant names (T11), `getLocalizedTitle` arg order (T10), `AnimeRepository.GetByID` signature (T7). Each step says exactly what to grep and what to do if it differs.
