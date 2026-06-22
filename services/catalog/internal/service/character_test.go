package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/shikimori"
)

// realMissCache returns a *cache.RedisCache backed by a real (local) Redis
// using DB 13, which is flushed to empty before each test so every Get is a
// cache miss. Mirrors the newTestRedis / resolveTestRedis helpers in this
// package (raw_resolver_test.go, subs_aggregator_resolve_test.go). Skips the
// test if Redis is not reachable.
func realMissCache(t *testing.T) *cache.RedisCache {
	t.Helper()
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := 6379
	if p := os.Getenv("REDIS_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	c, err := cache.New(cache.Config{Host: host, Port: port, DB: 13})
	if err != nil {
		t.Skipf("redis unreachable at %s:%d (%v); skipping character service test", host, port, err)
	}
	_ = c.Client().FlushDB(context.Background()).Err()
	t.Cleanup(func() {
		_ = c.Client().FlushDB(context.Background()).Err()
		_ = c.Close()
	})
	return c
}

// ---- handwritten fakes (no testify/mock) -----------------------------------

type fakeAnimeRepo struct{ anime *domain.Anime }

func (f *fakeAnimeRepo) GetByID(_ context.Context, _ string) (*domain.Anime, error) {
	return f.anime, nil
}

type fakeShikimori struct {
	roles    []shikimori.CharacterRoleResult
	rolesErr error
}

func (f *fakeShikimori) GetAnimeCharacters(_ context.Context, _ string) ([]shikimori.CharacterRoleResult, error) {
	return f.roles, f.rolesErr
}
func (f *fakeShikimori) GetCharacterByID(_ context.Context, _ string) (*domain.Character, error) {
	return nil, errors.New("not used")
}

type fakeCharRepo struct {
	upserted      []domain.Character
	byAnime       []domain.AnimeCharacterView
	replaced      bool
	replaceCalls  int
	replacedChars []domain.Character
	replacedRows  []domain.AnimeCharacter
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
func (f *fakeCharRepo) ReplaceAnimeCharacters(_ context.Context, _ string, rows []domain.AnimeCharacter, chars []domain.Character) error {
	f.replaced = true
	f.replaceCalls++
	f.replacedChars = chars
	f.replacedRows = rows
	return nil
}
func (f *fakeCharRepo) GetByAnimeID(_ context.Context, _ string) ([]domain.AnimeCharacterView, error) {
	return f.byAnime, nil
}

func newSvc(t *testing.T, animeRepo animeShikimoriIDLookup, repo characterRepo, shiki characterShikimori) *CharacterService {
	t.Helper()
	return NewCharacterService(animeRepo, repo, shiki, realMissCache(t), logger.Default())
}

// ---- tests -----------------------------------------------------------------

func TestGetAnimeCharacters_FetchesUpsertsAndReturns(t *testing.T) {
	animeRepo := &fakeAnimeRepo{anime: &domain.Anime{ID: "anime-uuid", ShikimoriID: "52991"}}
	shiki := &fakeShikimori{roles: []shikimori.CharacterRoleResult{
		{Character: domain.Character{ShikimoriID: "2", Name: "Frieren"}, Role: "main"},
		{Character: domain.Character{ShikimoriID: "1", Name: "Stark"}, Role: "supporting"},
	}}
	repo := &fakeCharRepo{byAnime: []domain.AnimeCharacterView{
		{Character: domain.Character{Name: "Frieren"}, Role: "main"},
	}}
	svc := newSvc(t, animeRepo, repo, shiki)

	got, err := svc.GetAnimeCharacters(context.Background(), "anime-uuid")
	if err != nil {
		t.Fatal(err)
	}
	if !repo.replaced {
		t.Fatal("expected join rows to be replaced")
	}
	// N+1 fix (L403): characters are bulk-upserted via ReplaceAnimeCharacters,
	// NOT via a per-character UpsertCharacter loop.
	if len(repo.upserted) != 0 {
		t.Fatalf("expected 0 per-character UpsertCharacter calls, got %d", len(repo.upserted))
	}
	if len(got) != 1 || got[0].Role != "main" {
		t.Fatalf("expected db read-back, got %+v", got)
	}
}

// TestGetAnimeCharacters_BulkUpsert_NoNPlus1 is the L403 regression guard: on a
// cache miss the service must NOT loop UpsertCharacter (2 queries each) — it
// must make a SINGLE ReplaceAnimeCharacters call carrying the full chars slice
// (index-aligned with the join rows) so the repo bulk-upserts in one tx.
func TestGetAnimeCharacters_BulkUpsert_NoNPlus1(t *testing.T) {
	animeRepo := &fakeAnimeRepo{anime: &domain.Anime{ID: "anime-uuid", ShikimoriID: "52991"}}
	roles := []shikimori.CharacterRoleResult{
		{Character: domain.Character{ShikimoriID: "2", Name: "Frieren"}, Role: "main"},
		{Character: domain.Character{ShikimoriID: "1", Name: "Stark"}, Role: "supporting"},
		{Character: domain.Character{ShikimoriID: "3", Name: "Fern"}, Role: "supporting"},
	}
	shiki := &fakeShikimori{roles: roles}
	repo := &fakeCharRepo{byAnime: []domain.AnimeCharacterView{
		{Character: domain.Character{Name: "Frieren"}, Role: "main"},
	}}
	svc := newSvc(t, animeRepo, repo, shiki)

	if _, err := svc.GetAnimeCharacters(context.Background(), "anime-uuid"); err != nil {
		t.Fatal(err)
	}

	if len(repo.upserted) != 0 {
		t.Fatalf("expected 0 UpsertCharacter calls (no N+1), got %d", len(repo.upserted))
	}
	if repo.replaceCalls != 1 {
		t.Fatalf("expected exactly 1 ReplaceAnimeCharacters call, got %d", repo.replaceCalls)
	}
	if len(repo.replacedChars) != len(roles) {
		t.Fatalf("expected chars slice of len %d, got %d", len(roles), len(repo.replacedChars))
	}
	if len(repo.replacedRows) != len(roles) {
		t.Fatalf("expected rows slice of len %d, got %d", len(roles), len(repo.replacedRows))
	}
	// Rows must be index-aligned with chars (same shikimori per index) and carry
	// the role + position the repo needs to resolve canonical ids + ordering.
	for i := range roles {
		if repo.replacedChars[i].ShikimoriID != roles[i].Character.ShikimoriID {
			t.Fatalf("chars[%d] shikimori=%q, want %q", i, repo.replacedChars[i].ShikimoriID, roles[i].Character.ShikimoriID)
		}
		if repo.replacedChars[i].ID == "" {
			t.Fatalf("chars[%d] must have a Go-side UUID assigned", i)
		}
		if repo.replacedRows[i].CharacterID != repo.replacedChars[i].ID {
			t.Fatalf("rows[%d].CharacterID=%q must equal chars[%d].ID=%q", i, repo.replacedRows[i].CharacterID, i, repo.replacedChars[i].ID)
		}
		if repo.replacedRows[i].Role != roles[i].Role {
			t.Fatalf("rows[%d].Role=%q, want %q", i, repo.replacedRows[i].Role, roles[i].Role)
		}
		if repo.replacedRows[i].Position != i {
			t.Fatalf("rows[%d].Position=%d, want %d", i, repo.replacedRows[i].Position, i)
		}
	}
}

func TestGetAnimeCharacters_ShikimoriDown_ServesFromDB(t *testing.T) {
	animeRepo := &fakeAnimeRepo{anime: &domain.Anime{ID: "anime-uuid", ShikimoriID: "52991"}}
	shiki := &fakeShikimori{rolesErr: errors.New("shikimori 503")}
	repo := &fakeCharRepo{byAnime: []domain.AnimeCharacterView{
		{Character: domain.Character{Name: "StaleFrieren"}, Role: "main"},
	}}
	svc := newSvc(t, animeRepo, repo, shiki)

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
