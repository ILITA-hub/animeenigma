package cards

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

func TestRandomTail_Type(t *testing.T) {
	r := &RandomTailResolver{}
	if got := r.Type(); got != "random_tail" {
		t.Errorf("Type() = %q, want %q", got, "random_tail")
	}
}

func TestRandomTail_Resolve_CallsSearch_WithPage3PageSize100(t *testing.T) {
	repo := &fakeAnimeSearcher{items: makeAnimes(50)}
	c := newFakeCache()
	r := NewRandomTailResolver(repo, c, testLogger())

	_, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	f := repo.snapshotFilters()
	if f.Page != 3 {
		t.Errorf("Page = %d, want 3 (ranks 201..300, outside featured's top-200 pool)", f.Page)
	}
	if f.PageSize != 100 {
		t.Errorf("PageSize = %d, want 100", f.PageSize)
	}
	if f.Sort != "score" {
		t.Errorf("Sort = %q, want score", f.Sort)
	}
	if f.Order != "desc" {
		t.Errorf("Order = %q, want desc", f.Order)
	}
	if f.ScoreMin != nil {
		t.Errorf("ScoreMin should be nil for random_tail (no min score), got: %v", f.ScoreMin)
	}
}

func TestRandomTail_Resolve_CacheKeyPrefix(t *testing.T) {
	repo := &fakeAnimeSearcher{items: makeAnimes(3)}
	c := newFakeCache()
	r := NewRandomTailResolver(repo, c, testLogger())

	_, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	keys := c.keys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 cache key, got %d", len(keys))
	}
	if !strings.HasPrefix(keys[0], "spotlight:random_tail:") {
		t.Errorf("cache key prefix wrong: %q", keys[0])
	}
	dateSuffix := strings.TrimPrefix(keys[0], "spotlight:random_tail:")
	if matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}$`, dateSuffix); !matched {
		t.Errorf("date suffix not YYYY-MM-DD: %q", dateSuffix)
	}
}

func TestRandomTail_Resolve_EmptyRepo_ReturnsNilNil(t *testing.T) {
	repo := &fakeAnimeSearcher{items: nil}
	c := newFakeCache()
	r := NewRandomTailResolver(repo, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil err on empty repo, got: %v", err)
	}
	if card != nil {
		t.Errorf("expected nil Card on empty pool, got: %+v", card)
	}
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set calls on empty pool, got %d", c.sets)
	}
}

func TestRandomTail_Resolve_ReturnsTypedData(t *testing.T) {
	repo := &fakeAnimeSearcher{items: makeAnimes(8)}
	c := newFakeCache()
	r := NewRandomTailResolver(repo, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	if card.Type != "random_tail" {
		t.Errorf("Card.Type = %q, want random_tail", card.Type)
	}
	data, ok := card.Data.(spotlight.RandomTailData)
	if !ok {
		t.Fatalf("Card.Data not RandomTailData: %T", card.Data)
	}
	if data.Anime.ID == "" {
		t.Error("picked anime should have a non-empty ID")
	}
}
