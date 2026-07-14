// Task 11 (Daily Fanfic Spotlight) — DailyFanficResolver tests.
//
// Mirrors featured_test.go's cache-discipline coverage: cache-miss fetch +
// cache-set, cache-hit skips the source entirely, and the (nil, nil)
// no-cache-on-empty eligibility path (Pitfall 5 from 01-RESEARCH.md).

package cards

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// --- fake dailyFanficSource ----------------------------------------------

type fakeFanficSource struct {
	data  *spotlight.DailyFanficData
	err   error
	calls int32
}

func (f *fakeFanficSource) FetchDaily(_ context.Context) (*spotlight.DailyFanficData, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.data, f.err
}

// --- tests ----------------------------------------------------------------

func TestDailyFanficResolver_Type(t *testing.T) {
	r := &DailyFanficResolver{}
	if got := r.Type(); got != "daily_fanfic" {
		t.Errorf("Type() = %q, want %q", got, "daily_fanfic")
	}
}

func TestDailyFanficResolver_CacheMiss_DataFetched_CachesAndReturnsCard(t *testing.T) {
	daily := &spotlight.DailyFanficData{
		ID:          "fic-1",
		FanficTitle: "The Last Sakura",
		AnimeTitle:  "Bocchi the Rock!",
		Rating:      "T",
		Language:    "ru",
	}
	src := &fakeFanficSource{data: daily}
	c := newFakeCache()
	r := NewDailyFanficResolver(src, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil Card when source returns data")
	}
	if card.Type != "daily_fanfic" {
		t.Errorf("Card.Type = %q, want daily_fanfic", card.Type)
	}
	got, ok := card.Data.(spotlight.DailyFanficData)
	if !ok {
		t.Fatalf("Card.Data is not DailyFanficData: %T", card.Data)
	}
	if got.ID != "fic-1" {
		t.Errorf("Card.Data.ID = %q, want fic-1", got.ID)
	}

	// Cache write happened exactly once, with the right key prefix.
	if c.sets != 1 {
		t.Errorf("expected exactly 1 cache.Set call, got %d", c.sets)
	}
	keys := c.keys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 cache key, got %d", len(keys))
	}
	if !strings.HasPrefix(keys[0], "spotlight:daily_fanfic:") {
		t.Errorf("cache key prefix wrong: %q", keys[0])
	}
	if atomic.LoadInt32(&src.calls) != 1 {
		t.Errorf("expected 1 source call, got %d", src.calls)
	}
}

func TestDailyFanficResolver_SourceReturnsNil_ReturnsNilNil_NotCached(t *testing.T) {
	src := &fakeFanficSource{data: nil} // e.g. client saw a 404 (nil, nil)
	c := newFakeCache()
	r := NewDailyFanficResolver(src, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil err when source is ineligible, got: %v", err)
	}
	if card != nil {
		t.Errorf("expected nil Card when source returns nil data, got: %+v", card)
	}
	// Pitfall 5: must NOT cache empty.
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set calls on nil data, got %d", c.sets)
	}
}

func TestDailyFanficResolver_CacheHit_SkipsSource(t *testing.T) {
	src := &fakeFanficSource{data: &spotlight.DailyFanficData{ID: "should-not-be-fetched"}}
	c := newFakeCache()

	seeded := spotlight.DailyFanficData{ID: "PRE_SEEDED", FanficTitle: "Cached Fic"}
	key := "spotlight:daily_fanfic:" + spotlight.DateKeyUTC(time.Now())
	data, _ := json.Marshal(seeded)
	c.store[key] = data

	r := NewDailyFanficResolver(src, c, testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card from cache hit")
	}

	// Source must NOT have been touched.
	if atomic.LoadInt32(&src.calls) != 0 {
		t.Errorf("expected 0 source calls on cache hit, got %d", src.calls)
	}

	got, ok := card.Data.(spotlight.DailyFanficData)
	if !ok {
		t.Fatalf("Card.Data wrong type: %T", card.Data)
	}
	if got.ID != "PRE_SEEDED" {
		t.Errorf("expected pre-seeded fanfic, got %q", got.ID)
	}
}

func TestDailyFanficResolver_SourceError_PropagatesAsError(t *testing.T) {
	src := &fakeFanficSource{err: errors.New("fanfic service down")}
	c := newFakeCache()
	r := NewDailyFanficResolver(src, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err == nil {
		t.Fatal("expected non-nil error when source errors")
	}
	if card != nil {
		t.Errorf("expected nil Card on source error, got: %+v", card)
	}
	if !strings.Contains(err.Error(), "daily_fanfic") {
		t.Errorf("error should be wrapped with resolver name, got: %v", err)
	}
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set calls on source error, got %d", c.sets)
	}
}

func TestDailyFanficResolver_CacheGetError_FallsThroughToSource(t *testing.T) {
	daily := &spotlight.DailyFanficData{ID: "fic-2"}
	src := &fakeFanficSource{data: daily}
	c := newFakeCache()
	c.getErr = errors.New("redis down")

	r := NewDailyFanficResolver(src, c, testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve should tolerate cache-get error, got: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil Card despite cache outage")
	}
	if atomic.LoadInt32(&src.calls) != 1 {
		t.Errorf("expected 1 source call when cache.Get errors, got %d", src.calls)
	}
}
