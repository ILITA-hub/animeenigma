package cards

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"go.uber.org/zap"
)

// testLogger returns a no-op *logger.Logger suitable for tests. The
// zap.NewNop() base swallows every log line so we don't pollute test
// output.
func testLogger() *logger.Logger {
	return &logger.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

// --- fake animeSearcher -------------------------------------------------

// fakeAnimeSearcher implements animeSearcher with handwritten state
// recording. Pattern: services/catalog/internal/service/scraper_test.go.
type fakeAnimeSearcher struct {
	items       []*domain.Anime
	err         error
	calls       int32
	lastFilters domain.SearchFilters
	mu          sync.Mutex
}

func (f *fakeAnimeSearcher) Search(_ context.Context, filters domain.SearchFilters) ([]*domain.Anime, int64, error) {
	atomic.AddInt32(&f.calls, 1)
	f.mu.Lock()
	f.lastFilters = filters
	f.mu.Unlock()
	return f.items, int64(len(f.items)), f.err
}

func (f *fakeAnimeSearcher) snapshotFilters() domain.SearchFilters {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastFilters
}

// --- fake cache ---------------------------------------------------------

// fakeCache implements the full cache.Cache interface. Only Get/Set
// carry real state; the others no-op. JSON round-trip mirrors the real
// RedisCache (libs/cache/cache.go:69 / :82) so test fixtures behave
// identically to production.
type fakeCache struct {
	mu     sync.Mutex
	store  map[string][]byte
	getErr error // if non-nil, Get returns this (overrides store lookup)
	setErr error // if non-nil, Set returns this and skips the write
	gets   int32
	sets   int32
}

func newFakeCache() *fakeCache {
	return &fakeCache{store: map[string][]byte{}}
}

func (f *fakeCache) Get(_ context.Context, key string, dest interface{}) error {
	atomic.AddInt32(&f.gets, 1)
	if f.getErr != nil {
		return f.getErr
	}
	f.mu.Lock()
	data, ok := f.store[key]
	f.mu.Unlock()
	if !ok {
		return cache.ErrNotFound
	}
	return json.Unmarshal(data, dest)
}

func (f *fakeCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	atomic.AddInt32(&f.sets, 1)
	if f.setErr != nil {
		return f.setErr
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.store[key] = data
	f.mu.Unlock()
	return nil
}

func (f *fakeCache) Delete(_ context.Context, _ ...string) error             { return nil }
func (f *fakeCache) Exists(_ context.Context, _ string) (bool, error)        { return false, nil }
func (f *fakeCache) Invalidate(_ context.Context, _ string) error            { return nil }
func (f *fakeCache) GetOrSet(_ context.Context, _ string, _ interface{}, _ time.Duration, _ func() (interface{}, error)) error {
	panic("spotlight resolvers must NOT use GetOrSet — deliberate divergence (Pitfall 5)")
}
func (f *fakeCache) SetNX(_ context.Context, _ string, _ interface{}, _ time.Duration) (bool, error) {
	return false, nil
}

func (f *fakeCache) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.store))
	for k := range f.store {
		out = append(out, k)
	}
	return out
}

// makeAnimes produces n stubbed *domain.Anime with predictable IDs and
// scores for deterministic-pick assertions.
func makeAnimes(n int) []*domain.Anime {
	out := make([]*domain.Anime, n)
	for i := 0; i < n; i++ {
		out[i] = &domain.Anime{
			ID:    "id-" + string(rune('a'+i%26)) + "-" + string(rune('0'+i/26)),
			Name:  "anime-" + string(rune('a'+i%26)),
			Score: 9.5 - float64(i)*0.01,
		}
	}
	return out
}

// fakeAnimeWithID returns a domain.Anime value carrying the given ID. Used
// by Plan 03-03 resolver tests that need a populated Anime value (not a
// pointer) inside cache fixtures.
func fakeAnimeWithID(id string) domain.Anime {
	return domain.Anime{ID: id, Name: "fake-" + id}
}
