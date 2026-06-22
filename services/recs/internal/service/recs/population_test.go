package recs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCache is an in-memory implementation of the populationCache surface
// (just Get/Set/Exists for the lastcomputed timestamp). This keeps the test
// dependency local and avoids dragging libs/cache into the recs package.
type fakeCache struct {
	mu    sync.Mutex
	store map[string]string
}

func newFakeCache() *fakeCache {
	return &fakeCache{store: make(map[string]string)}
}

func (c *fakeCache) Get(_ context.Context, key string, dest interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.store[key]
	if !ok {
		return errors.New("not found")
	}
	if dp, ok := dest.(*string); ok {
		*dp = v
		return nil
	}
	return errors.New("dest unsupported")
}

func (c *fakeCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := value.(string); ok {
		c.store[key] = s
		return nil
	}
	c.store[key] = "set"
	return nil
}

func (c *fakeCache) Exists(_ context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.store[key]
	return ok, nil
}

// signalRunCounter counts Precompute invocations for assertion.
type signalRunCounter struct {
	id      SignalID
	calls   int64
	failErr error
}

func (s *signalRunCounter) ID() SignalID { return s.id }
func (s *signalRunCounter) Precompute(_ context.Context, _ UserID) error {
	atomic.AddInt64(&s.calls, 1)
	return s.failErr
}
func (s *signalRunCounter) Score(_ context.Context, _ UserID, _ []AnimeID) (map[AnimeID]RawScore, error) {
	return nil, nil
}

func TestPopulationOrchestrator_RunOnceCallsAllModules(t *testing.T) {
	a := &signalRunCounter{id: "s3"}
	b := &signalRunCounter{id: "s4"}
	cache := newFakeCache()
	o := NewPopulationOrchestrator([]SignalModule{a, b}, cache, logger.Default())

	require.NoError(t, o.RunOnce(context.Background()))
	assert.Equal(t, int64(1), atomic.LoadInt64(&a.calls))
	assert.Equal(t, int64(1), atomic.LoadInt64(&b.calls))
}

func TestPopulationOrchestrator_RunOnceWritesLastComputed(t *testing.T) {
	a := &signalRunCounter{id: "s3"}
	cache := newFakeCache()
	o := NewPopulationOrchestrator([]SignalModule{a}, cache, logger.Default())

	require.NoError(t, o.RunOnce(context.Background()))
	exists, err := cache.Exists(context.Background(), "recs:popsignal:lastcomputed")
	require.NoError(t, err)
	assert.True(t, exists, "RunOnce must write the cache-buster timestamp")
}

func TestPopulationOrchestrator_RunOnceContinuesOnFailure(t *testing.T) {
	wantErr := errors.New("boom")
	a := &signalRunCounter{id: "s3", failErr: wantErr}
	b := &signalRunCounter{id: "s4"}
	cache := newFakeCache()
	o := NewPopulationOrchestrator([]SignalModule{a, b}, cache, logger.Default())

	err := o.RunOnce(context.Background())
	assert.Error(t, err, "RunOnce returns joined errors so the caller can log")
	assert.Equal(t, int64(1), atomic.LoadInt64(&a.calls))
	assert.Equal(t, int64(1), atomic.LoadInt64(&b.calls), "subsequent modules must still run after a failure")

	// Cache-buster timestamp must STILL be written on partial failure (stale serves)
	exists, err2 := cache.Exists(context.Background(), "recs:popsignal:lastcomputed")
	require.NoError(t, err2)
	assert.True(t, exists, "lastcomputed timestamp must be written even on partial failure (stale data continues serving)")
}

func TestPopulationOrchestrator_StartFiresImmediatelyAndPeriodically(t *testing.T) {
	a := &signalRunCounter{id: "s3"}
	cache := newFakeCache()
	o := NewPopulationOrchestrator([]SignalModule{a}, cache, logger.Default())

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx, 50*time.Millisecond)

	// Wait for at least 2 ticks: the boot tick + 1 ticker tick.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&a.calls) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()

	got := atomic.LoadInt64(&a.calls)
	assert.GreaterOrEqual(t, got, int64(2),
		"Start must fire RunOnce immediately on launch, then again on every interval tick")

	// Ensure the goroutine actually exits when ctx is canceled.
	time.Sleep(120 * time.Millisecond)
	stable := atomic.LoadInt64(&a.calls)
	time.Sleep(120 * time.Millisecond)
	assert.Equal(t, stable, atomic.LoadInt64(&a.calls), "Start goroutine must exit when ctx is canceled")
}

func TestPopulationOrchestrator_StartContinuesAfterFailedTick(t *testing.T) {
	// Failing signal that "heals" after the first call so we can verify the
	// ticker keeps running through transient errors.
	heal := &healingSignal{id: "s3"}
	cache := newFakeCache()
	o := NewPopulationOrchestrator([]SignalModule{heal}, cache, logger.Default())

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx, 30*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&heal.calls) >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()

	calls := atomic.LoadInt64(&heal.calls)
	assert.GreaterOrEqual(t, calls, int64(3),
		"Start must NOT exit on a failed tick; it must keep ticking. Calls=%d", calls)
}

type healingSignal struct {
	id    SignalID
	calls int64
}

func (h *healingSignal) ID() SignalID { return h.id }
func (h *healingSignal) Precompute(_ context.Context, _ UserID) error {
	n := atomic.AddInt64(&h.calls, 1)
	if n == 1 {
		return errors.New("first-tick failure")
	}
	return nil
}
func (h *healingSignal) Score(_ context.Context, _ UserID, _ []AnimeID) (map[AnimeID]RawScore, error) {
	return nil, nil
}

// blockingSignal blocks on ctx.Done() and records whether the per-call ctx
// carried a deadline. Used to prove the per-tick timeout (audit L641): a hung
// Precompute must NOT stall the ticker forever.
type blockingSignal struct {
	id          SignalID
	calls       int64
	sawDeadline int64 // atomic bool: 1 if any call's ctx had a deadline
}

func (b *blockingSignal) ID() SignalID { return b.id }
func (b *blockingSignal) Precompute(ctx context.Context, _ UserID) error {
	atomic.AddInt64(&b.calls, 1)
	if _, ok := ctx.Deadline(); ok {
		atomic.StoreInt64(&b.sawDeadline, 1)
	}
	<-ctx.Done() // block until the per-tick timeout (or parent cancel) fires
	return ctx.Err()
}
func (b *blockingSignal) Score(_ context.Context, _ UserID, _ []AnimeID) (map[AnimeID]RawScore, error) {
	return nil, nil
}

func TestPopulationOrchestrator_StartPerTickTimeout(t *testing.T) {
	// A signal whose Precompute hangs forever. Without a per-tick timeout the
	// ticker stalls at 1 call; with the timeout each tick aborts and the next
	// fires, so the call count advances past 1.
	block := &blockingSignal{id: "s3"}
	cache := newFakeCache()
	o := NewPopulationOrchestrator([]SignalModule{block}, cache, logger.Default())
	o.tickTimeout = 40 * time.Millisecond // tiny budget for the test

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx, 30*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&block.calls) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()

	assert.GreaterOrEqual(t, atomic.LoadInt64(&block.calls), int64(2),
		"per-tick timeout must abort a hung Precompute so the next tick fires; without it calls stays stuck at 1")
	assert.Equal(t, int64(1), atomic.LoadInt64(&block.sawDeadline),
		"each per-tick RunOnce must carry a deadline so a hung query can't run forever")
}
