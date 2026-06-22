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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// userOrchFakeCache extends fakeCache with the surface UserOrchestrator needs:
// Delete + SetNX. We define this as a separate struct so the existing
// fakeCache (population_test.go) is untouched.
type userOrchFakeCache struct {
	mu       sync.Mutex
	store    map[string]string
	deletes  []string
	setnxFn  func(key string) (bool, error) // optional override per-test
	setnxErr error
}

func newUserOrchFakeCache() *userOrchFakeCache {
	return &userOrchFakeCache{store: make(map[string]string)}
}

func (c *userOrchFakeCache) Get(_ context.Context, key string, dest interface{}) error {
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

func (c *userOrchFakeCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := value.(string); ok {
		c.store[key] = s
		return nil
	}
	c.store[key] = "set"
	return nil
}

func (c *userOrchFakeCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.store, k)
		c.deletes = append(c.deletes, k)
	}
	return nil
}

func (c *userOrchFakeCache) SetNX(_ context.Context, key string, _ interface{}, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setnxErr != nil {
		return false, c.setnxErr
	}
	if c.setnxFn != nil {
		return c.setnxFn(key)
	}
	if _, exists := c.store[key]; exists {
		return false, nil
	}
	c.store[key] = "1"
	return true, nil
}

func (c *userOrchFakeCache) deletesContains(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range c.deletes {
		if d == key {
			return true
		}
	}
	return false
}

// setupUserOrchTestDB seeds the watch_history table the orchestrator queries
// in RunOnce.
func setupUserOrchTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE watch_history (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		anime_id TEXT NOT NULL,
		watched_at DATETIME NOT NULL
	)`).Error)
	return db
}

func seedWH(t *testing.T, db *gorm.DB, id, userID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO watch_history (id, user_id, anime_id, watched_at) VALUES (?, ?, ?, datetime('now'))`,
		id, userID, "anime-X",
	).Error)
}

// userPrecomputeTracker is a SignalModule recorder for assertion. It records
// every userID Precompute is called with.
type userPrecomputeTracker struct {
	id   SignalID
	mu   sync.Mutex
	seen []UserID
	fail map[UserID]error // optional per-user failure injection
}

func (t *userPrecomputeTracker) ID() SignalID { return t.id }
func (t *userPrecomputeTracker) Precompute(_ context.Context, userID UserID) error {
	t.mu.Lock()
	t.seen = append(t.seen, userID)
	t.mu.Unlock()
	if t.fail != nil {
		if err, ok := t.fail[userID]; ok {
			return err
		}
	}
	return nil
}
func (t *userPrecomputeTracker) Score(_ context.Context, _ UserID, _ []AnimeID) (map[AnimeID]RawScore, error) {
	return nil, nil
}
func (t *userPrecomputeTracker) seenUsers() []UserID {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]UserID, len(t.seen))
	copy(out, t.seen)
	return out
}
func (t *userPrecomputeTracker) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.seen)
}

func TestUserOrchestrator_RunOnceIteratesDistinctUsers(t *testing.T) {
	db := setupUserOrchTestDB(t)
	// 2 users, 3 history rows total (user-A has 2, user-B has 1).
	seedWH(t, db, "wh1", "user-A")
	seedWH(t, db, "wh2", "user-A")
	seedWH(t, db, "wh3", "user-B")

	tracker := &userPrecomputeTracker{id: "s1"}
	pre := NewOrchestrator([]SignalModule{tracker})
	cache := newUserOrchFakeCache()
	o := NewUserOrchestrator(pre, db, cache, logger.Default())

	require.NoError(t, o.RunOnce(context.Background()))

	users := tracker.seenUsers()
	// Each user must appear exactly once (DISTINCT).
	counts := map[UserID]int{}
	for _, u := range users {
		counts[u]++
	}
	assert.Equal(t, 1, counts["user-A"])
	assert.Equal(t, 1, counts["user-B"])
	assert.Len(t, users, 2)
}

func TestUserOrchestrator_RunOnceContinuesOnPerUserFailure(t *testing.T) {
	db := setupUserOrchTestDB(t)
	seedWH(t, db, "wh1", "user-A")
	seedWH(t, db, "wh2", "user-B")

	tracker := &userPrecomputeTracker{
		id:   "s1",
		fail: map[UserID]error{"user-A": errors.New("boom")},
	}
	pre := NewOrchestrator([]SignalModule{tracker})
	cache := newUserOrchFakeCache()
	o := NewUserOrchestrator(pre, db, cache, logger.Default())

	err := o.RunOnce(context.Background())
	assert.Error(t, err, "joined error so caller can log")
	// Both users must still get a Precompute call (failure does NOT halt the tick).
	assert.Equal(t, 2, tracker.callCount())
}

func TestUserOrchestrator_RunOnceDeletesCacheForSucceededUsersOnly(t *testing.T) {
	db := setupUserOrchTestDB(t)
	seedWH(t, db, "wh1", "user-A")
	seedWH(t, db, "wh2", "user-B")

	tracker := &userPrecomputeTracker{
		id:   "s1",
		fail: map[UserID]error{"user-A": errors.New("boom")},
	}
	pre := NewOrchestrator([]SignalModule{tracker})
	cache := newUserOrchFakeCache()
	o := NewUserOrchestrator(pre, db, cache, logger.Default())

	_ = o.RunOnce(context.Background())

	// user-B succeeded -> cache key must be Deleted (so next request recomputes).
	assert.True(t, cache.deletesContains("recs:user:user-B:topN:v4"),
		"Delete must be called for users whose precompute succeeded")
	// user-A failed -> cache key must NOT be Deleted (stale-serves contract).
	assert.False(t, cache.deletesContains("recs:user:user-A:topN:v4"),
		"Delete must NOT be called for users whose precompute failed")
}

func TestUserOrchestrator_StartFiresBootTickAndPeriodically(t *testing.T) {
	db := setupUserOrchTestDB(t)
	seedWH(t, db, "wh1", "user-A")

	tracker := &userPrecomputeTracker{id: "s1"}
	pre := NewOrchestrator([]SignalModule{tracker})
	cache := newUserOrchFakeCache()
	o := NewUserOrchestrator(pre, db, cache, logger.Default())

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx, 50*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if tracker.callCount() >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()

	got := tracker.callCount()
	assert.GreaterOrEqual(t, got, 2, "Start must fire boot tick + at least 1 ticker tick")

	// Confirm the goroutine exits when ctx cancels.
	time.Sleep(120 * time.Millisecond)
	stable := tracker.callCount()
	time.Sleep(120 * time.Millisecond)
	assert.Equal(t, stable, tracker.callCount(), "Start goroutine must exit on ctx.Done")
}

func TestUserOrchestrator_StartContinuesAfterFailedTick(t *testing.T) {
	db := setupUserOrchTestDB(t)
	seedWH(t, db, "wh1", "user-A")

	// Always-failing tracker — Start must NOT exit when ticks fail.
	tracker := &userPrecomputeTracker{
		id:   "s1",
		fail: map[UserID]error{"user-A": errors.New("permanent")},
	}
	pre := NewOrchestrator([]SignalModule{tracker})
	cache := newUserOrchFakeCache()
	o := NewUserOrchestrator(pre, db, cache, logger.Default())

	ctx, cancel := context.WithCancel(context.Background())
	o.Start(ctx, 30*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if tracker.callCount() >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()

	assert.GreaterOrEqual(t, tracker.callCount(), 3,
		"Start must NOT exit on failed ticks; the ticker keeps going")
}

func TestUserOrchestrator_TriggerForUser_AcquiresAndSpawns(t *testing.T) {
	db := setupUserOrchTestDB(t)
	tracker := &userPrecomputeTracker{id: "s1"}
	pre := NewOrchestrator([]SignalModule{tracker})
	cache := newUserOrchFakeCache()
	o := NewUserOrchestrator(pre, db, cache, logger.Default())

	require.NoError(t, o.TriggerForUser(context.Background(), "user-A"))

	// Wait for the spawned goroutine to call Precompute.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if tracker.callCount() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, 1, tracker.callCount(), "first acquire must spawn the precompute")
	// Also assert that the per-user topN cache key was Deleted on success.
	deadline = time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if cache.deletesContains("recs:user:user-A:topN:v4") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.True(t, cache.deletesContains("recs:user:user-A:topN:v4"),
		"successful trigger must Delete the per-user topN cache")
}

func TestUserOrchestrator_TriggerForUser_DebounceSkipsSecondCall(t *testing.T) {
	db := setupUserOrchTestDB(t)
	tracker := &userPrecomputeTracker{id: "s1"}
	pre := NewOrchestrator([]SignalModule{tracker})
	cache := newUserOrchFakeCache()
	o := NewUserOrchestrator(pre, db, cache, logger.Default())

	// First call acquires.
	require.NoError(t, o.TriggerForUser(context.Background(), "user-A"))
	// Wait for the first goroutine to finish.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if tracker.callCount() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Second call within the debounce window must NOT spawn another precompute.
	require.NoError(t, o.TriggerForUser(context.Background(), "user-A"))
	time.Sleep(150 * time.Millisecond) // give a hypothetical goroutine time to fire

	assert.Equal(t, 1, tracker.callCount(),
		"second TriggerForUser within debounce window must NOT spawn another precompute")
}

func TestUserOrchestrator_TriggerForUser_SetNXErrorReturnsNil(t *testing.T) {
	db := setupUserOrchTestDB(t)
	tracker := &userPrecomputeTracker{id: "s1"}
	pre := NewOrchestrator([]SignalModule{tracker})
	cache := newUserOrchFakeCache()
	cache.setnxErr = errors.New("redis down")
	o := NewUserOrchestrator(pre, db, cache, logger.Default())

	// MUST NOT propagate the error to the caller — list.go calls in a goroutine
	// and the trigger is best-effort.
	assert.NoError(t, o.TriggerForUser(context.Background(), "user-A"))

	// Must NOT have spawned the precompute goroutine.
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, 0, tracker.callCount(),
		"SetNX error must skip the precompute spawn (no acquire = no work)")
}

func TestUserOrchestrator_TriggerForUser_ReturnsImmediately(t *testing.T) {
	db := setupUserOrchTestDB(t)
	// Tracker that blocks for 200ms on Precompute — proves that TriggerForUser
	// returns long before Precompute completes.
	tracker := &slowSignalRecorder{id: "s1", delay: 200 * time.Millisecond}
	pre := NewOrchestrator([]SignalModule{tracker})
	cache := newUserOrchFakeCache()
	o := NewUserOrchestrator(pre, db, cache, logger.Default())

	start := time.Now()
	require.NoError(t, o.TriggerForUser(context.Background(), "user-A"))
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 50*time.Millisecond,
		"TriggerForUser must return immediately (< 50ms) — the precompute happens in a goroutine")
}

type slowSignalRecorder struct {
	id    SignalID
	delay time.Duration
	calls int64
}

func (s *slowSignalRecorder) ID() SignalID { return s.id }
func (s *slowSignalRecorder) Precompute(_ context.Context, _ UserID) error {
	atomic.AddInt64(&s.calls, 1)
	time.Sleep(s.delay)
	return nil
}
func (s *slowSignalRecorder) Score(_ context.Context, _ UserID, _ []AnimeID) (map[AnimeID]RawScore, error) {
	return nil, nil
}

// blockingUserSignal blocks on ctx.Done() for a configurable subset of users
// (or all). Records whether the per-call ctx carried a deadline. Used to prove
// (a) the per-tick timeout (audit L641) and (b) the per-user timeout (L648).
type blockingUserSignal struct {
	id          SignalID
	mu          sync.Mutex
	calls       int64
	blockUsers  map[UserID]bool // if non-nil, only these users block; else all block
	seen        []UserID
	sawDeadline int64 // atomic bool: 1 if any blocked call's ctx had a deadline
}

func (b *blockingUserSignal) ID() SignalID { return b.id }
func (b *blockingUserSignal) Precompute(ctx context.Context, userID UserID) error {
	atomic.AddInt64(&b.calls, 1)
	b.mu.Lock()
	b.seen = append(b.seen, userID)
	b.mu.Unlock()
	shouldBlock := b.blockUsers == nil || b.blockUsers[userID]
	if !shouldBlock {
		return nil
	}
	if _, ok := ctx.Deadline(); ok {
		atomic.StoreInt64(&b.sawDeadline, 1)
	}
	<-ctx.Done()
	return ctx.Err()
}
func (b *blockingUserSignal) Score(_ context.Context, _ UserID, _ []AnimeID) (map[AnimeID]RawScore, error) {
	return nil, nil
}
func (b *blockingUserSignal) seenUsers() []UserID {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]UserID, len(b.seen))
	copy(out, b.seen)
	return out
}

// L641: a hung per-user precompute under the cron's shared ctx stalls the
// ticker forever. The per-tick timeout must abort it so the next tick fires.
func TestUserOrchestrator_StartPerTickTimeout(t *testing.T) {
	db := setupUserOrchTestDB(t)
	seedWH(t, db, "wh1", "user-A")

	block := &blockingUserSignal{id: "s1"} // all users block forever
	pre := NewOrchestrator([]SignalModule{block})
	cache := newUserOrchFakeCache()
	o := NewUserOrchestrator(pre, db, cache, logger.Default())
	o.tickTimeout = 60 * time.Millisecond
	o.perUserTimeout = 40 * time.Millisecond

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
		"per-tick timeout must abort a hung precompute so the next tick fires")
}

// L648: one slow user must not starve the rest. With a per-user timeout, the
// 'slow' user's precompute aborts and 'fast' still gets its cache.Delete.
func TestUserOrchestrator_RunOncePerUserTimeoutDoesNotStarveOthers(t *testing.T) {
	db := setupUserOrchTestDB(t)
	seedWH(t, db, "wh1", "slow")
	seedWH(t, db, "wh2", "fast")

	block := &blockingUserSignal{
		id:         "s1",
		blockUsers: map[UserID]bool{"slow": true}, // only 'slow' hangs
	}
	pre := NewOrchestrator([]SignalModule{block})
	cache := newUserOrchFakeCache()
	o := NewUserOrchestrator(pre, db, cache, logger.Default())
	o.perUserTimeout = 50 * time.Millisecond

	start := time.Now()
	err := o.RunOnce(context.Background())
	elapsed := time.Since(start)

	// 'slow' blocks until its per-user budget fires; without the per-user
	// timeout RunOnce would hang forever.
	assert.Less(t, elapsed, 1*time.Second,
		"per-user timeout must bound the slow user so RunOnce returns")
	assert.Error(t, err, "the slow user's DeadlineExceeded is joined into the error")

	// 'fast' must still have been processed AND had its cache deleted —
	// the slow user did not starve it.
	assert.Equal(t, int64(1), atomic.LoadInt64(&block.sawDeadline),
		"the blocked (slow) precompute must have carried a deadline")
	assert.True(t, cache.deletesContains("recs:user:fast:topN:v4"),
		"fast user must still get its cache.Delete despite the slow user hanging")
	assert.False(t, cache.deletesContains("recs:user:slow:topN:v4"),
		"slow user failed (timed out) -> stale-serves, no cache delete")

	seen := block.seenUsers()
	assert.Len(t, seen, 2, "both users must be visited even though one hung")
}
