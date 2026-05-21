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

// fakeChangelogFetcher implements changelogFetcher for tests.
type fakeChangelogFetcher struct {
	entries []spotlight.ChangelogEntry
	err     error
	calls   int32
}

func (f *fakeChangelogFetcher) GetChangelog(_ context.Context) ([]spotlight.ChangelogEntry, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.entries, f.err
}

func TestLatestNews_Type(t *testing.T) {
	r := &LatestNewsResolver{}
	if got := r.Type(); got != "latest_news" {
		t.Errorf("Type() = %q, want %q", got, "latest_news")
	}
}

func TestLatestNews_Resolve_CacheMiss_FetchesAndCaches(t *testing.T) {
	web := &fakeChangelogFetcher{entries: []spotlight.ChangelogEntry{
		{Date: "2026-05-21", Type: "feature", Message: "a"},
		{Date: "2026-05-21", Type: "feature", Message: "b"},
		{Date: "2026-05-20", Type: "fix", Message: "c"},
	}}
	c := newFakeCache()
	r := NewLatestNewsResolver(web, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	if card.Type != "latest_news" {
		t.Errorf("Card.Type = %q, want latest_news", card.Type)
	}
	data, ok := card.Data.(spotlight.LatestNewsData)
	if !ok {
		t.Fatalf("Card.Data not LatestNewsData: %T", card.Data)
	}
	if len(data.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(data.Entries))
	}
	if web.calls != 1 {
		t.Errorf("expected 1 web.GetChangelog call, got %d", web.calls)
	}
	if c.sets != 1 {
		t.Errorf("expected 1 cache.Set, got %d", c.sets)
	}
	keys := c.keys()
	if len(keys) != 1 || !strings.HasPrefix(keys[0], "spotlight:changelog:") {
		t.Errorf("cache key wrong: %v", keys)
	}
}

func TestLatestNews_Resolve_CacheHit_SkipsFetcher(t *testing.T) {
	web := &fakeChangelogFetcher{entries: []spotlight.ChangelogEntry{
		{Date: "2026-05-21", Message: "fresh"},
	}}
	c := newFakeCache()

	seeded := spotlight.LatestNewsData{Entries: []spotlight.ChangelogEntry{
		{Date: "2026-05-21", Message: "CACHED"},
	}}
	key := "spotlight:changelog:" + spotlight.DateKeyUTC(time.Now())
	data, _ := json.Marshal(seeded)
	c.store[key] = data

	r := NewLatestNewsResolver(web, c, testLogger())
	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if card == nil {
		t.Fatal("expected non-nil card from cache hit")
	}
	if web.calls != 0 {
		t.Errorf("expected 0 web calls on cache hit, got %d", web.calls)
	}
	got, _ := card.Data.(spotlight.LatestNewsData)
	if len(got.Entries) != 1 || got.Entries[0].Message != "CACHED" {
		t.Errorf("expected cached payload, got: %+v", got)
	}
}

func TestLatestNews_Resolve_FetcherError_ReturnsError(t *testing.T) {
	web := &fakeChangelogFetcher{err: errors.New("web down")}
	c := newFakeCache()
	r := NewLatestNewsResolver(web, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error from fetcher")
	}
	if card != nil {
		t.Errorf("expected nil card on error, got: %+v", card)
	}
	if !strings.Contains(err.Error(), "latest_news") {
		t.Errorf("error not wrapped with resolver name: %v", err)
	}
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set on error, got %d", c.sets)
	}
}

func TestLatestNews_Resolve_EmptyEntries_ReturnsNilNil(t *testing.T) {
	web := &fakeChangelogFetcher{entries: nil}
	c := newFakeCache()
	r := NewLatestNewsResolver(web, c, testLogger())

	card, err := r.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil err on empty entries, got: %v", err)
	}
	if card != nil {
		t.Errorf("expected nil card on empty entries, got: %+v", card)
	}
	if c.sets != 0 {
		t.Errorf("expected 0 cache.Set on empty entries, got %d", c.sets)
	}
}
