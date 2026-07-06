package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrQuotaExceeded = errors.New("daily generation quota exceeded")
	ErrBusy          = errors.New("a generation is already in progress")
)

// quotaStore is the minimal Redis surface the quota needs (adapter in main.go).
type quotaStore interface {
	Incr(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Del(ctx context.Context, key string) error
}

type Quota struct {
	store    quotaStore
	dailyCap int
	now      func() time.Time
}

func NewQuota(store quotaStore, dailyCap int, now func() time.Time) *Quota {
	return &Quota{store: store, dailyCap: dailyCap, now: now}
}

// Acquire enforces (a) one concurrent generation per user and (b) a daily cap.
// On any Redis error it FAILS OPEN (returns a no-op release, nil error) so a
// Redis blip never blocks an admin. The returned release must always be called.
func (q *Quota) Acquire(ctx context.Context, userID string) (func(), error) {
	noop := func() {}
	lockKey := "fanfic:lock:" + userID
	ok, err := q.store.SetNX(ctx, lockKey, 3*time.Minute)
	if err != nil {
		return noop, nil // fail open
	}
	if !ok {
		return noop, ErrBusy
	}
	release := func() { _ = q.store.Del(context.Background(), lockKey) }

	dayKey := fmt.Sprintf("fanfic:quota:%s:%s", userID, q.now().UTC().Format("20060102"))
	n, err := q.store.Incr(ctx, dayKey)
	if err != nil {
		return release, nil // fail open, but still release the lock
	}
	if n == 1 {
		_ = q.store.Expire(ctx, dayKey, 48*time.Hour)
	}
	if int(n) > q.dailyCap {
		release()
		return noop, ErrQuotaExceeded
	}
	return release, nil
}
