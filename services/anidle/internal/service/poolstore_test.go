package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

// fakeCache implements the subset of libs/cache.Cache that PoolStore uses.
type fakeCache struct {
	mu    sync.Mutex
	store map[string][]byte
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string][]byte{}} }

func (f *fakeCache) Get(_ context.Context, key string, dest interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.store[key]
	if !ok {
		return errCacheMiss
	}
	return json.Unmarshal(b, dest)
}

func (f *fakeCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	b, _ := json.Marshal(value)
	f.mu.Lock()
	f.store[key] = b
	f.mu.Unlock()
	return nil
}

// fakeFetcher implements poolFetcher.
type fakeFetcher struct {
	pool  []domain.PoolAnime
	calls int
}

func (f *fakeFetcher) Fetch(_ context.Context) ([]domain.PoolAnime, error) {
	f.calls++
	return f.pool, nil
}

func samplePool() []domain.PoolAnime {
	return []domain.PoolAnime{
		{ID: "frieren", NameRU: "Фрирен", NameEN: "Frieren"},
		{ID: "jjk", NameRU: "Магическая битва", NameEN: "Jujutsu Kaisen"},
	}
}

func TestPoolStore_FetchesOnceThenServesFromCache(t *testing.T) {
	fetch := &fakeFetcher{pool: samplePool()}
	store := NewPoolStore(newFakeCache(), fetch, time.Hour, nil)

	p1, err := store.All(context.Background())
	require.NoError(t, err)
	require.Len(t, p1, 2)

	_, err = store.All(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, fetch.calls, "second All() must hit the in-memory/Redis cache, not refetch")
}

func TestPoolStore_LookupAndSearch(t *testing.T) {
	store := NewPoolStore(newFakeCache(), &fakeFetcher{pool: samplePool()}, time.Hour, nil)
	_, err := store.All(context.Background())
	require.NoError(t, err)

	a, ok := store.Lookup("jjk")
	require.True(t, ok)
	assert.Equal(t, "Магическая битва", a.NameRU)

	_, ok = store.Lookup("missing")
	assert.False(t, ok)

	res := store.Search(context.Background(), "маг", 10)
	require.Len(t, res, 1)
	assert.Equal(t, "jjk", res[0].ID)

	res = store.Search(context.Background(), "frie", 10)
	require.Len(t, res, 1)
	assert.Equal(t, "frieren", res[0].ID)
}
