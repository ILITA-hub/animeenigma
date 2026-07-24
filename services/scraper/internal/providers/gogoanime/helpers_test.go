package gogoanime

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

// fakeCache is an in-memory cache.Cache implementation for tests. Mirrors
// animepahe/malsync_test.go fakeCache so the test rig is identical across
// providers. Order-sensitive logs (getLog/setLog) capture every Get/Set call
// so assertions can verify the cache key shapes.
type fakeCache struct {
	mu      sync.Mutex
	data    map[string][]byte
	expiry  map[string]time.Time
	getLog  []string
	setLog  []string
	deleted []string
}

func newFakeCache() *fakeCache {
	return &fakeCache{
		data:   make(map[string][]byte),
		expiry: make(map[string]time.Time),
	}
}

func (f *fakeCache) Get(ctx context.Context, key string, dest interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getLog = append(f.getLog, key)
	data, ok := f.data[key]
	if !ok {
		return cache.ErrNotFound
	}
	if exp, ok := f.expiry[key]; ok && time.Now().After(exp) {
		delete(f.data, key)
		delete(f.expiry, key)
		return cache.ErrNotFound
	}
	return json.Unmarshal(data, dest)
}

func (f *fakeCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setLog = append(f.setLog, key)
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.data[key] = b
	if ttl > 0 {
		f.expiry[key] = time.Now().Add(ttl)
	}
	return nil
}

func (f *fakeCache) Delete(ctx context.Context, keys ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, keys...)
	for _, k := range keys {
		delete(f.data, k)
		delete(f.expiry, k)
	}
	return nil
}

func (f *fakeCache) GetDel(ctx context.Context, key string, dest interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getLog = append(f.getLog, key)
	data, ok := f.data[key]
	if !ok {
		return cache.ErrNotFound
	}
	if exp, ok := f.expiry[key]; ok && time.Now().After(exp) {
		delete(f.data, key)
		delete(f.expiry, key)
		return cache.ErrNotFound
	}
	f.deleted = append(f.deleted, key)
	delete(f.data, key)
	delete(f.expiry, key)
	return json.Unmarshal(data, dest)
}

func (f *fakeCache) Exists(ctx context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.data[key]
	return ok, nil
}

func (f *fakeCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error {
	if err := f.Get(ctx, key, dest); err == nil {
		return nil
	}
	v, err := fn()
	if err != nil {
		return err
	}
	if err := f.Set(ctx, key, v, ttl); err != nil {
		return err
	}
	return f.Get(ctx, key, dest)
}

func (f *fakeCache) Invalidate(ctx context.Context, pattern string) error { return nil }

func (f *fakeCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	if _, exists := f.data[key]; exists {
		f.mu.Unlock()
		return false, nil
	}
	f.mu.Unlock()
	return true, f.Set(ctx, key, value, ttl)
}

func (f *fakeCache) snapshotSetLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.setLog))
	copy(cp, f.setLog)
	return cp
}

func (f *fakeCache) snapshotGetLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.getLog))
	copy(cp, f.getLog)
	return cp
}

func (f *fakeCache) snapshotDeleted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.deleted))
	copy(cp, f.deleted)
	return cp
}

// Compile-time assertion: fakeCache satisfies cache.Cache.
var _ cache.Cache = (*fakeCache)(nil)
