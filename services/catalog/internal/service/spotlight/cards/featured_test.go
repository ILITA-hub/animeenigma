package cards

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

func TestFeaturedResolver_Type(t *testing.T) {
	r := &FeaturedResolver{}
	if got := r.Type(); got != "featured" {
		t.Errorf("Type() = %q, want %q", got, "featured")
	}
}

func TestFeaturedResolver_CacheMiss_PicksDeterministically(t *testing.T) {
	// No pinned anime (SortPriority==0 on all items), so falls through to
	// the daily-seeded pick. The fake returns the same items on every call.
	repo := &fakeAnimeSearcher{items: makeAnimes(10)}
	c := newFakeCache()
	r := NewFeaturedResolver(repo, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil Card on non-empty repo")
	}
	if card.Type != "featured" {
		t.Errorf("Card.Type = %q, want featured", card.Type)
	}

	data, ok := card.Data.(spotlight.FeaturedData)
	if !ok {
		t.Fatalf("Card.Data is not FeaturedData: %T", card.Data)
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
	if !strings.HasPrefix(keys[0], "spotlight:featured:") {
		t.Errorf("cache key prefix wrong: %q", keys[0])
	}
}

func TestFeaturedResolver_CacheHit_SkipsRepo(t *testing.T) {
	repo := &fakeAnimeSearcher{items: makeAnimes(5)}
	c := newFakeCache()

	// Pre-seed cache with a marshaled FeaturedData for today's key.
	seeded := spotlight.FeaturedData{Anime: *makeAnimes(1)[0]}
	seeded.Anime.Name = "PRE_SEEDED"
	key := "spotlight:featured:" + spotlight.DateKeyUTC(time.Now())
	data, _ := json.Marshal(seeded)
	c.store[key] = data

	r := NewFeaturedResolver(repo, c, testLogger())
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
	got, ok := card.Data.(spotlight.FeaturedData)
	if !ok {
		t.Fatalf("Card.Data wrong type: %T", card.Data)
	}
	if got.Anime.Name != "PRE_SEEDED" {
		t.Errorf("expected pre-seeded anime, got %q", got.Anime.Name)
	}
}

func TestFeaturedResolver_EmptyRepo_ReturnsNilNil(t *testing.T) {
	// Both pin query and daily-pool query return empty.
	repo := &fakeAnimeSearcher{items: nil}
	c := newFakeCache()
	r := NewFeaturedResolver(repo, c, testLogger())

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

func TestFeaturedResolver_RepoError_PropagatesAsError(t *testing.T) {
	// The pin query errors (same err field); resolver falls through to the
	// daily pick which also errors. The error must be wrapped with "featured".
	repo := &fakeAnimeSearcher{err: errors.New("db down")}
	c := newFakeCache()
	r := NewFeaturedResolver(repo, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err == nil {
		t.Fatal("expected non-nil error when repo errors")
	}
	if card != nil {
		t.Errorf("expected nil Card on repo error, got: %+v", card)
	}
	if !strings.Contains(err.Error(), "featured") {
		t.Errorf("error should be wrapped with resolver name, got: %v", err)
	}
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set calls on repo error, got %d", c.sets)
	}
}

func TestFeaturedResolver_CacheGetError_FallsThroughToCompute(t *testing.T) {
	// Simulate Redis hard-down on Get — resolver must NOT fail; instead
	// fall through to compute path. The pin query returns an unpinned item
	// (SortPriority=0), so the daily pick is used. 2 repo calls total.
	repo := &fakeAnimeSearcher{items: makeAnimes(3)}
	c := newFakeCache()
	c.getErr = errors.New("redis down")

	r := NewFeaturedResolver(repo, c, testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve should tolerate cache-get error, got: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil Card despite cache outage")
	}
	// 2 calls: pin query + daily-pool query.
	if repo.calls != 2 {
		t.Errorf("expected 2 repo calls when cache.Get errors, got %d", repo.calls)
	}
}

func TestFeaturedResolver_DailyPickSearchFiltersCorrect(t *testing.T) {
	// Uses byCall: first call (pin query) returns empty, second (daily pool)
	// returns items so we can inspect the daily-pick filters.
	repo := &fakeAnimeSearcher{byCall: [][]*domain.Anime{{}, makeAnimes(2)}}
	c := newFakeCache()
	r := NewFeaturedResolver(repo, c, testLogger())

	_, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// snapshotFilters returns the LAST captured filter set — the daily pick.
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

// --- Curated-pin tests (TDD: Step 3) ------------------------------------

func TestFeaturedResolver_CuratedPinWins(t *testing.T) {
	pin := &domain.Anime{ID: "pin-1", Name: "Pinned Hero", Score: 7.0, SortPriority: 5}
	daily := &domain.Anime{ID: "day-1", Name: "Daily Pick", Score: 9.5}
	// 1st Search call (pin query) returns the pin;
	// 2nd Search call (daily pool) returns the daily candidate.
	repo := &fakeAnimeSearcher{
		byCall: [][]*domain.Anime{{pin}, {daily}},
	}
	r := NewFeaturedResolver(repo, newFakeCache(), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	got, ok := card.Data.(spotlight.FeaturedData)
	if !ok {
		t.Fatalf("Card.Data is not FeaturedData: %T", card.Data)
	}
	if got.Anime.ID != "pin-1" {
		t.Fatalf("expected curated pin to win, got %q", got.Anime.ID)
	}
}

func TestFeaturedResolver_FallsBackToDailyWhenNoPin(t *testing.T) {
	daily := &domain.Anime{ID: "day-1", Name: "Daily Pick", Score: 9.5}
	// 1st call (pin query) returns empty; 2nd call (daily pool) returns daily.
	repo := &fakeAnimeSearcher{byCall: [][]*domain.Anime{{}, {daily}}}
	r := NewFeaturedResolver(repo, newFakeCache(), testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	got, ok := card.Data.(spotlight.FeaturedData)
	if !ok {
		t.Fatalf("Card.Data is not FeaturedData: %T", card.Data)
	}
	if got.Anime.ID != "day-1" {
		t.Fatalf("expected daily fallback, got %q", got.Anime.ID)
	}
}
