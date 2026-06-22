package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/require"
)

type fakeCompatRepo struct {
	data  map[string][]domain.UserListEntry
	calls int // total ListEntries invocations (cache-effectiveness assertion)
}

func (f *fakeCompatRepo) ListEntries(_ context.Context, uid string) ([]domain.UserListEntry, error) {
	f.calls++
	return f.data[uid], nil
}

// fakeCache is a minimal in-memory cache.Cache used to assert the compatibility
// result cache. Only Get/Set are exercised by CompatibilityService; the rest
// satisfy the interface. JSON round-trips the value so it mirrors RedisCache's
// marshal semantics (and proves the cached struct survives serialization).
type fakeCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newFakeCache() *fakeCache { return &fakeCache{data: map[string][]byte{}} }

func (c *fakeCache) Get(_ context.Context, key string, dest interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.data[key]
	if !ok {
		return cache.ErrNotFound
	}
	return json.Unmarshal(b, dest)
}

func (c *fakeCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.data[key] = b
	return nil
}

func (c *fakeCache) Delete(_ context.Context, _ ...string) error      { return nil }
func (c *fakeCache) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (c *fakeCache) Invalidate(_ context.Context, _ string) error     { return nil }
func (c *fakeCache) SetNX(_ context.Context, _ string, _ interface{}, _ time.Duration) (bool, error) {
	return true, nil
}
func (c *fakeCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error {
	if err := c.Get(ctx, key, dest); err == nil {
		return nil
	}
	v, err := fn()
	if err != nil {
		return err
	}
	if err := c.Set(ctx, key, v, ttl); err != nil {
		return err
	}
	return c.Get(ctx, key, dest)
}

func TestCompatibility_IdenticalLists100(t *testing.T) {
	e := []domain.UserListEntry{{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}, {AnimeID: "b", Score: 9, GenreIDs: []string{"g1"}}}
	svc := NewCompatibilityService(&fakeCompatRepo{data: map[string][]domain.UserListEntry{"v": e, "o": e}})
	r, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	require.Equal(t, 100, r.Percent)
	require.Equal(t, 2, r.SharedCount)
}

func TestCompatibility_NoOverlap0(t *testing.T) {
	svc := NewCompatibilityService(&fakeCompatRepo{data: map[string][]domain.UserListEntry{
		"v": {{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}},
		"o": {{AnimeID: "z", Score: 8, GenreIDs: []string{"g9"}}},
	}})
	r, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	require.Equal(t, 0, r.Percent)
	require.Equal(t, 0, r.SharedCount)
}

func TestCompatibility_PartialBlend(t *testing.T) {
	// overlap 1/3 titles, identical scores on shared, identical genre vectors
	svc := NewCompatibilityService(&fakeCompatRepo{data: map[string][]domain.UserListEntry{
		"v": {{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}, {AnimeID: "b", Score: 7, GenreIDs: []string{"g1"}}},
		"o": {{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}, {AnimeID: "c", Score: 6, GenreIDs: []string{"g1"}}},
	}})
	r, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	// overlap = 1/3 ; scoreAgreement = 1 (identical on shared "a") ; genreSim = 1
	// score = 0.5*0.333 + 0.4*1 + 0.1*1 = 0.6667 -> 67
	require.InDelta(t, 67, r.Percent, 1)
	require.Equal(t, 1, r.SharedCount)
}

// TestCompatibility_CachesResult (audit L606) — the second Compute for the same
// pair must be served from cache: ListEntries is invoked exactly twice TOTAL
// (viewer+owner on the cold call, ZERO on the warm call) and both results are
// byte-equal. Before the cache lookup is wired this fails (4 calls).
func TestCompatibility_CachesResult(t *testing.T) {
	e := []domain.UserListEntry{{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}}
	repo := &fakeCompatRepo{data: map[string][]domain.UserListEntry{"v": e, "o": e}}
	svc := NewCompatibilityService(repo)
	svc.SetCache(newFakeCache())

	first, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	require.Equal(t, 2, repo.calls, "cold call loads viewer + owner")

	second, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	require.Equal(t, 2, repo.calls, "warm call must hit cache, not the repo")

	require.Equal(t, *first, *second, "cached result must equal the computed result")
}

// TestCompatibility_CacheKeyIsOrderCanonical (audit L606) — a swapped pair
// (owner,viewer) must hit the SAME canonical cache key as (viewer,owner), so
// the second Compute is still a cache hit. (Compatibility is symmetric, so the
// percent is identical regardless of argument order; only the canonical key
// prevents a duplicate recompute.)
func TestCompatibility_CacheKeyIsOrderCanonical(t *testing.T) {
	e := []domain.UserListEntry{{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}}
	repo := &fakeCompatRepo{data: map[string][]domain.UserListEntry{"v": e, "o": e}}
	svc := NewCompatibilityService(repo)
	svc.SetCache(newFakeCache())

	_, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	require.Equal(t, 2, repo.calls)

	// Swapped order — must resolve to the same cached entry, no new repo loads.
	_, err = svc.Compute(context.Background(), "o", "v")
	require.NoError(t, err)
	require.Equal(t, 2, repo.calls, "swapped pair must reuse the canonical cache key")
}

// TestCompatibility_NoCacheStillComputes (audit L606) — when no cache is wired
// (the unit-test/DI-optional path), Compute still works and recomputes each
// time (no panic on the nil cache).
func TestCompatibility_NoCacheStillComputes(t *testing.T) {
	e := []domain.UserListEntry{{AnimeID: "a", Score: 8, GenreIDs: []string{"g1"}}}
	repo := &fakeCompatRepo{data: map[string][]domain.UserListEntry{"v": e, "o": e}}
	svc := NewCompatibilityService(repo) // no SetCache

	_, err := svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	_, err = svc.Compute(context.Background(), "v", "o")
	require.NoError(t, err)
	require.Equal(t, 4, repo.calls, "without a cache every call recomputes (2 loads each)")
}
