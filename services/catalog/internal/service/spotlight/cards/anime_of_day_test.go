package cards

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

func TestAnimeOfDay_Type(t *testing.T) {
	r := &AnimeOfDayResolver{}
	if got := r.Type(); got != "anime_of_day" {
		t.Errorf("Type() = %q, want %q", got, "anime_of_day")
	}
}

func TestAnimeOfDay_Resolve_CacheMiss_PicksDeterministically(t *testing.T) {
	repo := &fakeAnimeSearcher{items: makeAnimes(10)}
	c := newFakeCache()
	r := NewAnimeOfDayResolver(repo, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil Card on non-empty repo")
	}
	if card.Type != "anime_of_day" {
		t.Errorf("Card.Type = %q, want anime_of_day", card.Type)
	}

	data, ok := card.Data.(spotlight.AnimeOfDayData)
	if !ok {
		t.Fatalf("Card.Data is not AnimeOfDayData: %T", card.Data)
	}

	// Deterministic pick: items[DateSeedUTC(now) % 10].
	want := repo.items[spotlight.DateSeedUTC(time.Now())%10]
	if data.Anime.ID != want.ID {
		t.Errorf("picked anime ID = %q, want %q (DateSeedUTC=%d)", data.Anime.ID, want.ID, spotlight.DateSeedUTC(time.Now()))
	}

	// Cache write happened exactly once.
	if c.sets != 1 {
		t.Errorf("expected exactly 1 cache.Set call, got %d", c.sets)
	}
	// And the key has the right prefix.
	keys := c.keys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 cache key, got %d", len(keys))
	}
	if !strings.HasPrefix(keys[0], "spotlight:anime_of_day:") {
		t.Errorf("cache key prefix wrong: %q", keys[0])
	}
}

func TestAnimeOfDay_Resolve_CacheHit_SkipsRepo(t *testing.T) {
	repo := &fakeAnimeSearcher{items: makeAnimes(5)}
	c := newFakeCache()

	// Pre-seed cache with a marshaled AnimeOfDayData for today's key.
	seeded := spotlight.AnimeOfDayData{Anime: *makeAnimes(1)[0]}
	seeded.Anime.Name = "PRE_SEEDED"
	key := "spotlight:anime_of_day:" + spotlight.DateKeyUTC(time.Now())
	data, _ := json.Marshal(seeded)
	c.store[key] = data

	r := NewAnimeOfDayResolver(repo, c, testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card from cache hit")
	}

	// Repo must NOT have been touched.
	if repo.calls != 0 {
		t.Errorf("expected 0 repo calls on cache hit, got %d", repo.calls)
	}

	// Cache returned the seeded value.
	got, ok := card.Data.(spotlight.AnimeOfDayData)
	if !ok {
		t.Fatalf("Card.Data wrong type: %T", card.Data)
	}
	if got.Anime.Name != "PRE_SEEDED" {
		t.Errorf("expected pre-seeded anime, got %q", got.Anime.Name)
	}
}

func TestAnimeOfDay_Resolve_EmptyRepo_ReturnsNilNil(t *testing.T) {
	repo := &fakeAnimeSearcher{items: nil}
	c := newFakeCache()
	r := NewAnimeOfDayResolver(repo, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil err on empty repo, got: %v", err)
	}
	if card != nil {
		t.Errorf("expected nil Card on empty pool, got: %+v", card)
	}
	// Pitfall 5: must NOT cache empty.
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set calls on empty pool, got %d", c.sets)
	}
}

func TestAnimeOfDay_Resolve_RepoError_PropagatesAsError(t *testing.T) {
	repo := &fakeAnimeSearcher{err: errors.New("db down")}
	c := newFakeCache()
	r := NewAnimeOfDayResolver(repo, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err == nil {
		t.Fatal("expected non-nil error when repo errors")
	}
	if card != nil {
		t.Errorf("expected nil Card on repo error, got: %+v", card)
	}
	if !strings.Contains(err.Error(), "anime_of_day") {
		t.Errorf("error should be wrapped with resolver name, got: %v", err)
	}
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set calls on repo error, got %d", c.sets)
	}
}

func TestAnimeOfDay_Resolve_CacheGetError_FallsThroughToCompute(t *testing.T) {
	// Simulate Redis hard-down on Get — resolver must NOT fail; instead
	// fall through to compute path.
	repo := &fakeAnimeSearcher{items: makeAnimes(3)}
	c := newFakeCache()
	c.getErr = errors.New("redis down")

	r := NewAnimeOfDayResolver(repo, c, testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve should tolerate cache-get error, got: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil Card despite cache outage")
	}
	if repo.calls != 1 {
		t.Errorf("expected 1 repo call when cache.Get errors, got %d", repo.calls)
	}
}

func TestAnimeOfDay_Resolve_SearchFiltersCorrect(t *testing.T) {
	repo := &fakeAnimeSearcher{items: makeAnimes(2)}
	c := newFakeCache()
	r := NewAnimeOfDayResolver(repo, c, testLogger())

	_, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	f := repo.snapshotFilters()
	if f.Sort != "score" {
		t.Errorf("Sort = %q, want score", f.Sort)
	}
	if f.Order != "desc" {
		t.Errorf("Order = %q, want desc", f.Order)
	}
	if f.ScoreMin == nil || *f.ScoreMin != 8.0 {
		t.Errorf("ScoreMin = %v, want &8.0", f.ScoreMin)
	}
	if f.Page != 1 {
		t.Errorf("Page = %d, want 1", f.Page)
	}
	if f.PageSize != 200 {
		t.Errorf("PageSize = %d, want 200", f.PageSize)
	}
}
