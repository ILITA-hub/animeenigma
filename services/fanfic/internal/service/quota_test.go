package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeQuotaStore struct {
	counts map[string]int64
	locks  map[string]bool
	fail   bool
}

func newFakeQuotaStore() *fakeQuotaStore {
	return &fakeQuotaStore{counts: map[string]int64{}, locks: map[string]bool{}}
}

func (f *fakeQuotaStore) Incr(_ context.Context, key string) (int64, error) {
	if f.fail {
		return 0, errors.New("redis down")
	}
	f.counts[key]++
	return f.counts[key], nil
}
func (f *fakeQuotaStore) Expire(_ context.Context, _ string, _ time.Duration) error { return nil }
func (f *fakeQuotaStore) SetNX(_ context.Context, key string, _ time.Duration) (bool, error) {
	if f.fail {
		return false, errors.New("redis down")
	}
	if f.locks[key] {
		return false, nil
	}
	f.locks[key] = true
	return true, nil
}
func (f *fakeQuotaStore) Del(_ context.Context, key string) error { delete(f.locks, key); return nil }

func TestQuota_AllowsUnderCapThenBlocks(t *testing.T) {
	store := newFakeQuotaStore()
	q := NewQuota(store, 2, func() time.Time { return time.Unix(0, 0) })
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		rel, err := q.Acquire(ctx, "u")
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		rel()
	}
	if _, err := q.Acquire(ctx, "u"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}
}

func TestQuota_ConcurrencyLock(t *testing.T) {
	store := newFakeQuotaStore()
	q := NewQuota(store, 100, func() time.Time { return time.Unix(0, 0) })
	rel, err := q.Acquire(context.Background(), "u")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := q.Acquire(context.Background(), "u"); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy while locked, got %v", err)
	}
	rel()
}

func TestQuota_FailsOpenOnRedisError(t *testing.T) {
	store := newFakeQuotaStore()
	store.fail = true
	q := NewQuota(store, 1, func() time.Time { return time.Unix(0, 0) })
	rel, err := q.Acquire(context.Background(), "u")
	if err != nil {
		t.Fatalf("expected fail-open (nil err), got %v", err)
	}
	rel() // no-op release must be safe
}
