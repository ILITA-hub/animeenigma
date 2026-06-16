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
