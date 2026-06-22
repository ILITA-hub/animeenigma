package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

const poolCacheKey = "anidle:pool"

// errCacheMiss lets tests assert a miss without importing libs/cache; the real
// cache returns cache.ErrNotFound which we treat identically.
var errCacheMiss = errors.New("cache miss")

type poolCache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
}

type poolFetcher interface {
	Fetch(ctx context.Context) ([]domain.PoolAnime, error)
}

// PoolStore caches the guess pool (Redis + in-memory index).
type PoolStore struct {
	cache   poolCache
	fetcher poolFetcher
	ttl     time.Duration
	log     *logger.Logger

	mu       sync.RWMutex
	byID     map[string]domain.PoolAnime
	all      []domain.PoolAnime
	loaded   bool
	loadedAt time.Time
	now      func() time.Time // injectable for tests
}

func NewPoolStore(c poolCache, f poolFetcher, ttl time.Duration, log *logger.Logger) *PoolStore {
	return &PoolStore{cache: c, fetcher: f, ttl: ttl, log: log, now: time.Now}
}

// freshLocked reports whether the in-memory pool is loaded and still within its
// TTL. The caller must hold at least the read lock. A non-positive ttl disables
// in-memory expiry (load once — the historical behavior).
func (s *PoolStore) freshLocked() bool {
	return s.loaded && (s.ttl <= 0 || s.now().Before(s.loadedAt.Add(s.ttl)))
}

// All returns the full pool, loading it (Redis → catalog) on first use and
// RELOADING once the in-memory copy is older than ttl. Previously the in-memory
// index loaded once and never refreshed — ANIDLE_POOL_TTL only bounded the Redis
// entry, so a long-lived process served a stale pool until restart (audit #21).
func (s *PoolStore) All(ctx context.Context) ([]domain.PoolAnime, error) {
	s.mu.RLock()
	if s.freshLocked() {
		all := s.all
		s.mu.RUnlock()
		return all, nil
	}
	s.mu.RUnlock()

	pool, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func (s *PoolStore) load(ctx context.Context) ([]domain.PoolAnime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.freshLocked() {
		// Another goroutine refreshed while we waited for the write lock.
		return s.all, nil
	}

	var pool []domain.PoolAnime
	if err := s.cache.Get(ctx, poolCacheKey, &pool); err != nil {
		if !errors.Is(err, cache.ErrNotFound) && !errors.Is(err, errCacheMiss) {
			if s.log != nil {
				s.log.Warnw("pool cache get failed; refetching", "error", err)
			}
		}
		fetched, ferr := s.fetcher.Fetch(ctx)
		if ferr != nil {
			return nil, ferr
		}
		pool = fetched
		if serr := s.cache.Set(ctx, poolCacheKey, pool, s.ttl); serr != nil && s.log != nil {
			s.log.Warnw("pool cache set failed", "error", serr)
		}
	}

	s.byID = make(map[string]domain.PoolAnime, len(pool))
	for _, a := range pool {
		s.byID[a.ID] = a
	}
	s.all = pool
	s.loaded = true
	s.loadedAt = s.now()
	return pool, nil
}

// Lookup returns the pool anime by id (after All has loaded the pool).
func (s *PoolStore) Lookup(id string) (domain.PoolAnime, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.byID[id]
	return a, ok
}

// Search returns up to limit entries whose RU/EN/JP name contains q (case-insensitive).
func (s *PoolStore) Search(ctx context.Context, q string, limit int) []domain.PoolAnime {
	if _, err := s.All(ctx); err != nil {
		return nil
	}
	q = strings.ToLower(strings.TrimSpace(q))
	s.mu.RLock()
	defer s.mu.RUnlock()
	if q == "" {
		return nil
	}
	out := make([]domain.PoolAnime, 0, limit)
	for _, a := range s.all {
		if strings.Contains(strings.ToLower(a.NameRU), q) ||
			strings.Contains(strings.ToLower(a.NameEN), q) ||
			strings.Contains(strings.ToLower(a.NameJP), q) {
			out = append(out, a)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
