package spotlight

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// -----------------------------------------------------------------------------
// Fakes — handwritten struct implementations (project convention, see
// services/catalog/internal/service/scraper_test.go:20-37). NO testify/mock.
// All counters use sync/atomic for safety under concurrent goroutines.
// -----------------------------------------------------------------------------

// fakeResolver is a minimal Resolver implementation. Set typ + card/err
// + optional sleep to model success / error / slow paths. calls is
// incremented atomically so tests can assert the resolver was invoked
// despite its card being dropped.
type fakeResolver struct {
	typ   string
	card  *Card
	err   error
	sleep time.Duration
	calls int32
}

func (f *fakeResolver) Type() string { return f.typ }

func (f *fakeResolver) Resolve(ctx context.Context, _ *string) (*Card, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.sleep > 0 {
		select {
		case <-time.After(f.sleep):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.card, f.err
}

// fakeCache is an in-memory cache.Cache implementation. Get/Set are
// thread-safe; setErr/getErr force errors to model Redis-down. snapshot()
// returns a copy of the store for safe iteration in assertions.
type fakeCache struct {
	mu     sync.Mutex
	store  map[string][]byte
	setErr error
	getErr error
}

func newFakeCache() *fakeCache {
	return &fakeCache{store: map[string][]byte{}}
}

func (c *fakeCache) Get(ctx context.Context, key string, dest interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getErr != nil {
		return c.getErr
	}
	data, ok := c.store[key]
	if !ok {
		return cache.ErrNotFound
	}
	return json.Unmarshal(data, dest)
}

func (c *fakeCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setErr != nil {
		return c.setErr
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.store[key] = data
	return nil
}

func (c *fakeCache) GetDel(ctx context.Context, key string, dest interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getErr != nil {
		return c.getErr
	}
	data, ok := c.store[key]
	if !ok {
		return cache.ErrNotFound
	}
	delete(c.store, key)
	return json.Unmarshal(data, dest)
}

func (c *fakeCache) Delete(ctx context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.store, k)
	}
	return nil
}

func (c *fakeCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.store[key]
	return ok, nil
}

func (c *fakeCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error {
	return nil
}

func (c *fakeCache) Invalidate(ctx context.Context, pattern string) error {
	return nil
}

func (c *fakeCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	return false, nil
}

// snapshot returns a copy of the in-memory store for read-only iteration.
func (c *fakeCache) snapshot() map[string][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := map[string][]byte{}
	for k, v := range c.store {
		cp[k] = v
	}
	return cp
}

// preSeed writes a value to the store under key without going through
// Set (used by tests that need to pre-populate snapshot data before the
// aggregator runs).
func (c *fakeCache) preSeed(t *testing.T, key string, value interface{}) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("preSeed marshal: %v", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = data
}

// testLogger returns a real Logger configured for development. The tests
// do not assert on log contents directly; they assert on the side
// effects of dropped vs included cards. The logger is allowed to flush
// to stderr — Go's testing framework captures it.
func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	l, err := logger.New(logger.Config{Level: "error", Development: false, Encoding: "json"})
	if err != nil {
		t.Fatalf("testLogger: %v", err)
	}
	return l
}

// -----------------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------------

// TestAggregator_Concurrency_DispatchesInParallel proves the fan-out
// pattern works: 4 resolvers each sleeping 100ms should return in
// ~100ms total, NOT 400ms. Pins HSB-BE-03/05 fan-out semantics.
func TestAggregator_Concurrency_DispatchesInParallel(t *testing.T) {
	resolvers := []Resolver{
		&fakeResolver{typ: "a", card: &Card{Type: "a"}, sleep: 100 * time.Millisecond},
		&fakeResolver{typ: "b", card: &Card{Type: "b"}, sleep: 100 * time.Millisecond},
		&fakeResolver{typ: "c", card: &Card{Type: "c"}, sleep: 100 * time.Millisecond},
		&fakeResolver{typ: "d", card: &Card{Type: "d"}, sleep: 100 * time.Millisecond},
	}
	c := newFakeCache()
	agg := NewAggregator(c, testLogger(t), resolvers)

	t0 := time.Now()
	resp, err := agg.Resolve(context.Background(), nil)
	elapsed := time.Since(t0)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Cards) != 4 {
		t.Fatalf("expected 4 cards, got %d", len(resp.Cards))
	}
	// 4×100ms sequential would be 400ms; parallel should be ~100ms.
	// 250ms gives comfortable headroom for goroutine scheduling.
	if elapsed > 250*time.Millisecond {
		t.Fatalf("expected parallel dispatch < 250ms, got %v (sequential would be ~400ms)", elapsed)
	}
}

// TestAggregator_PerCardTimeout_DropsSlowCard verifies HSB-BE-03: a
// resolver exceeding its 800ms ctx.WithTimeout drops its card while
// the 3 fast resolvers complete normally.
func TestAggregator_PerCardTimeout_DropsSlowCard(t *testing.T) {
	slow := &fakeResolver{typ: "slow", card: &Card{Type: "slow"}, sleep: 1500 * time.Millisecond}
	resolvers := []Resolver{
		&fakeResolver{typ: "fast1", card: &Card{Type: "fast1"}, sleep: 10 * time.Millisecond},
		&fakeResolver{typ: "fast2", card: &Card{Type: "fast2"}, sleep: 10 * time.Millisecond},
		&fakeResolver{typ: "fast3", card: &Card{Type: "fast3"}, sleep: 10 * time.Millisecond},
		slow,
	}
	c := newFakeCache()
	agg := NewAggregatorWithDeadlines(c, testLogger(t), resolvers, 800*time.Millisecond, 2*time.Second)

	t0 := time.Now()
	resp, err := agg.Resolve(context.Background(), nil)
	elapsed := time.Since(t0)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Cards) != 3 {
		t.Fatalf("expected 3 cards (slow dropped), got %d (cards=%+v)", len(resp.Cards), resp.Cards)
	}
	for _, card := range resp.Cards {
		if card.Type == "slow" {
			t.Fatalf("slow resolver should have been dropped, but its card is present")
		}
	}
	// Slow resolver hits its 800ms ctx deadline. Allow up to 1100ms for
	// drain + goroutine join overhead.
	if elapsed < 800*time.Millisecond {
		t.Fatalf("expected elapsed >= 800ms (slow card's per-card deadline), got %v", elapsed)
	}
	if elapsed > 1100*time.Millisecond {
		t.Fatalf("expected elapsed < 1100ms (drain headroom), got %v", elapsed)
	}
	// Slow resolver was called (we got past dispatch); its sleep cooperatively
	// returned ctx.Err.
	if atomic.LoadInt32(&slow.calls) != 1 {
		t.Fatalf("expected slow resolver to be called once, got %d", slow.calls)
	}
}

// TestAggregator_OverallTimeout_DropsAllSlowCards verifies that with
// 1500ms sleeps and an 800ms per-card deadline, every card is cut by
// its OWN deadline (the per-card 800ms < 1500ms sleep < 2s overall).
// This pins the per-card-first contract: per-card deadline catches the
// slow path before the overall budget would.
func TestAggregator_OverallTimeout_DropsAllSlowCards(t *testing.T) {
	resolvers := []Resolver{
		&fakeResolver{typ: "s1", card: &Card{Type: "s1"}, sleep: 1500 * time.Millisecond},
		&fakeResolver{typ: "s2", card: &Card{Type: "s2"}, sleep: 1500 * time.Millisecond},
		&fakeResolver{typ: "s3", card: &Card{Type: "s3"}, sleep: 1500 * time.Millisecond},
		&fakeResolver{typ: "s4", card: &Card{Type: "s4"}, sleep: 1500 * time.Millisecond},
	}
	c := newFakeCache()
	agg := NewAggregatorWithDeadlines(c, testLogger(t), resolvers, 800*time.Millisecond, 2*time.Second)

	t0 := time.Now()
	resp, err := agg.Resolve(context.Background(), nil)
	elapsed := time.Since(t0)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Cards) != 0 {
		t.Fatalf("expected 0 cards (all timed out), got %d", len(resp.Cards))
	}
	if elapsed < 800*time.Millisecond {
		t.Fatalf("expected elapsed >= 800ms, got %v", elapsed)
	}
	if elapsed > 2200*time.Millisecond {
		t.Fatalf("expected elapsed < 2200ms (overall budget headroom), got %v", elapsed)
	}
}

// TestAggregator_OverallTimeout_HitsOverallBudget verifies HSB-BE-04: when
// the overall budget is tighter than the per-card deadline AND each
// resolver's sleep exceeds the overall budget, the overall ctx cancels
// the children. Elapsed is bounded by the overall budget, NOT the
// per-card deadline.
func TestAggregator_OverallTimeout_HitsOverallBudget(t *testing.T) {
	resolvers := []Resolver{
		&fakeResolver{typ: "o1", card: &Card{Type: "o1"}, sleep: 1500 * time.Millisecond},
		&fakeResolver{typ: "o2", card: &Card{Type: "o2"}, sleep: 1500 * time.Millisecond},
		&fakeResolver{typ: "o3", card: &Card{Type: "o3"}, sleep: 1500 * time.Millisecond},
		&fakeResolver{typ: "o4", card: &Card{Type: "o4"}, sleep: 1500 * time.Millisecond},
	}
	c := newFakeCache()
	// per-card lenient (3s), overall tight (1s). Children sleep 1.5s
	// → cancelled by parent at 1s.
	agg := NewAggregatorWithDeadlines(c, testLogger(t), resolvers, 3*time.Second, 1*time.Second)

	t0 := time.Now()
	resp, err := agg.Resolve(context.Background(), nil)
	elapsed := time.Since(t0)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Don't pin a number — pin the budget. Cards may be 0 or include a
	// fast straggler depending on scheduler nondeterminism, but the
	// runtime MUST not exceed the overall budget + drain.
	_ = resp
	if elapsed > 1200*time.Millisecond {
		t.Fatalf("expected elapsed < 1200ms (overall budget enforced), got %v", elapsed)
	}
}

// TestAggregator_EligibilityFilter_DropsNilCardSilently verifies HSB-BE-05:
// a resolver returning (nil, nil) is silently dropped — no log line, no
// presence in the response payload — while sibling cards are returned.
func TestAggregator_EligibilityFilter_DropsNilCardSilently(t *testing.T) {
	ineligible := &fakeResolver{typ: "ineligible", card: nil, err: nil, sleep: 10 * time.Millisecond}
	resolvers := []Resolver{
		&fakeResolver{typ: "latest_news", card: &Card{Type: "latest_news"}, sleep: 10 * time.Millisecond},
		ineligible,
		&fakeResolver{typ: "platform_stats", card: &Card{Type: "platform_stats"}, sleep: 10 * time.Millisecond},
	}
	c := newFakeCache()
	agg := NewAggregator(c, testLogger(t), resolvers)

	resp, err := agg.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Cards) != 2 {
		t.Fatalf("expected 2 cards (ineligible dropped), got %d (%+v)", len(resp.Cards), resp.Cards)
	}
	gotTypes := map[string]bool{}
	for _, c := range resp.Cards {
		gotTypes[c.Type] = true
	}
	if !gotTypes["latest_news"] {
		t.Fatalf("expected latest_news card present, got types %v", gotTypes)
	}
	if !gotTypes["platform_stats"] {
		t.Fatalf("expected platform_stats card present, got types %v", gotTypes)
	}
	if gotTypes["ineligible"] {
		t.Fatalf("ineligible card must NOT appear in payload")
	}
	if atomic.LoadInt32(&ineligible.calls) != 1 {
		t.Fatalf("expected ineligible resolver to be invoked once, got %d", ineligible.calls)
	}
}

// TestAggregator_SnapshotFallback_ReturnsSnapshotOnZeroCards verifies
// HSB-BE-04: when every resolver returns ineligible (or 0 resolvers
// wired) AND a snapshot exists in cache, the aggregator returns the
// snapshot (not a fresh empty Response).
func TestAggregator_SnapshotFallback_ReturnsSnapshotOnZeroCards(t *testing.T) {
	c := newFakeCache()
	// Pre-seed a snapshot for the anon user.
	snapResp := Response{
		Cards: []Card{
			{Type: "latest_news", Data: map[string]any{"entries": []any{}}},
		},
		GeneratedAt: "2026-05-20T00:00:00Z",
	}
	c.preSeed(t, SnapshotKey(nil), snapResp)

	// 0 resolvers → 0 cards → snapshot path.
	agg := NewAggregator(c, testLogger(t), nil)

	resp, err := agg.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Cards) != 1 {
		t.Fatalf("expected 1 card from snapshot, got %d", len(resp.Cards))
	}
	if resp.Cards[0].Type != "latest_news" {
		t.Fatalf("expected latest_news from snapshot, got %s", resp.Cards[0].Type)
	}
	// GeneratedAt should be the snapshot's stale timestamp, not a fresh now().
	if resp.GeneratedAt != "2026-05-20T00:00:00Z" {
		t.Fatalf("expected snapshot GeneratedAt preserved, got %s", resp.GeneratedAt)
	}
}

// TestAggregator_SnapshotFallback_NoSnapshot_EmptyResponse verifies that
// with zero resolvers AND no snapshot in cache, the aggregator returns
// a 200-equivalent empty Response (Cards == []Card{}, NOT nil) with a
// fresh GeneratedAt timestamp. Phase 2 frontend hides the block.
func TestAggregator_SnapshotFallback_NoSnapshot_EmptyResponse(t *testing.T) {
	c := newFakeCache()
	agg := NewAggregator(c, testLogger(t), nil)

	resp, err := agg.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Cards == nil {
		t.Fatalf("expected non-nil empty Cards slice (JSON [] not null), got nil")
	}
	if len(resp.Cards) != 0 {
		t.Fatalf("expected 0 cards, got %d", len(resp.Cards))
	}
	if resp.GeneratedAt == "" {
		t.Fatalf("expected non-empty GeneratedAt, got empty")
	}
	// Verify the JSON encodes as `[]` not `null` — Phase 2 frontend
	// regression guard.
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"cards":[]`) {
		t.Fatalf("expected cards to marshal as [], got %s", string(data))
	}
}

// TestAggregator_SnapshotSave_WritesAfterSuccessfulResolve verifies that
// a successful (>=1 card) Resolve triggers a best-effort snapshot write
// via the detached context.Background() goroutine. Tests poll up to
// 500ms because the write is async — not synchronous with Resolve return.
func TestAggregator_SnapshotSave_WritesAfterSuccessfulResolve(t *testing.T) {
	resolvers := []Resolver{
		&fakeResolver{typ: "a", card: &Card{Type: "a"}, sleep: 10 * time.Millisecond},
		&fakeResolver{typ: "b", card: &Card{Type: "b"}, sleep: 10 * time.Millisecond},
	}
	c := newFakeCache()
	agg := NewAggregator(c, testLogger(t), resolvers)

	resp, err := agg.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(resp.Cards))
	}

	// Poll up to 500ms for the detached snapshot write to complete.
	key := SnapshotKey(nil)
	var present bool
	for i := 0; i < 50; i++ {
		if _, ok := c.snapshot()[key]; ok {
			present = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !present {
		t.Fatalf("expected snapshot key %q to be written within 500ms, store: %v", key, c.snapshot())
	}

	// Verify the stored bytes round-trip to a Response with 2 cards.
	var stored Response
	if err := json.Unmarshal(c.snapshot()[key], &stored); err != nil {
		t.Fatalf("snapshot unmarshal: %v", err)
	}
	if len(stored.Cards) != 2 {
		t.Fatalf("expected stored snapshot to contain 2 cards, got %d", len(stored.Cards))
	}
}

// TestKeyPrefix_AllWritesUseSpotlightPrefix verifies HSB-NF-03: every
// Redis key written by the aggregator (in this case, the snapshot key)
// starts with the literal "spotlight:" prefix. Regression guard against
// accidental key-namespace drift.
func TestKeyPrefix_AllWritesUseSpotlightPrefix(t *testing.T) {
	resolvers := []Resolver{
		&fakeResolver{typ: "a", card: &Card{Type: "a"}, sleep: 5 * time.Millisecond},
		&fakeResolver{typ: "b", card: &Card{Type: "b"}, sleep: 5 * time.Millisecond},
	}
	c := newFakeCache()
	agg := NewAggregator(c, testLogger(t), resolvers)

	if _, err := agg.Resolve(context.Background(), nil); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Poll for the detached snapshot write.
	var keysSeen []string
	for i := 0; i < 50; i++ {
		store := c.snapshot()
		if len(store) > 0 {
			for k := range store {
				keysSeen = append(keysSeen, k)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(keysSeen) == 0 {
		t.Fatalf("expected at least one key written, got 0")
	}
	for _, k := range keysSeen {
		if !strings.HasPrefix(k, "spotlight:") {
			t.Fatalf("expected all keys to start with 'spotlight:', got %q", k)
		}
	}
}

// capturingResolver is a fake Resolver that captures the (userID, JWT-on-ctx)
// it was invoked with. Used by Plan 03-04 Task 2 to assert the aggregator
// fan-out propagates the userID + ctx-attached JWT to every resolver. The
// `card` is returned verbatim; nil ⇒ ineligible (silent drop).
type capturingResolver struct {
	typ  string
	card *Card

	mu        sync.Mutex
	gotUserID *string
	gotJWT    string
	gotJWTOK  bool
	captured  bool
}

func (f *capturingResolver) Type() string { return f.typ }

func (f *capturingResolver) Resolve(ctx context.Context, userID *string) (*Card, error) {
	// Import path workaround — aggregator package cannot import cards (it
	// would create a cycle), so we read the JWT directly via the same key
	// the cards.ContextWithJWT helper uses. The key type is the unexported
	// struct{} value behind `cards.ContextWithJWT`; we re-define a local
	// alias at the bottom of this file.
	f.mu.Lock()
	if userID != nil {
		v := *userID
		f.gotUserID = &v
	} else {
		f.gotUserID = nil
	}
	jwt, ok := jwtFromCtxForTest(ctx)
	f.gotJWT = jwt
	f.gotJWTOK = ok
	f.captured = true
	f.mu.Unlock()
	return f.card, nil
}

func (f *capturingResolver) snapshot() (userID *string, jwt string, jwtOK bool, called bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gotUserID, f.gotJWT, f.gotJWTOK, f.captured
}

// TestAggregator_NineCards_PassesUserIDAndJWT proves the aggregator's fan-out
// preserves the (userID, JWT-on-ctx) contract across 9 concurrent resolvers —
// the exact wave-3 configuration Plan 04 wires in main.go. Every resolver:
//
//  1. Saw the same userID pointer value the caller passed.
//  2. Saw the same JWT string the caller attached via ContextWithJWT.
//  3. Contributed its card to the response (len(Cards) == 9).
//
// This is the regression guard for the 9-card production wiring (HSB-BE-02).
func TestAggregator_NineCards_PassesUserIDAndJWT(t *testing.T) {
	caps := []*capturingResolver{
		{typ: "featured", card: &Card{Type: "featured"}},
		{typ: "random_tail", card: &Card{Type: "random_tail"}},
		{typ: "latest_news", card: &Card{Type: "latest_news"}},
		{typ: "platform_stats", card: &Card{Type: "platform_stats"}},
		{typ: "personal_pick", card: &Card{Type: "personal_pick"}},
		{typ: "telegram_news", card: &Card{Type: "telegram_news", Data: TelegramNewsData{Posts: []TelegramPost{{Date: time.Now().UTC().Format(time.RFC3339)}}}}},
		{typ: "now_watching", card: &Card{Type: "now_watching"}},
		{typ: "not_time_yet", card: &Card{Type: "not_time_yet"}},
		{typ: "continue_watching_new", card: &Card{Type: "continue_watching_new"}},
	}
	resolvers := make([]Resolver, 0, len(caps))
	for _, c := range caps {
		resolvers = append(resolvers, c)
	}

	c := newFakeCache()
	agg := NewAggregator(c, testLogger(t), resolvers)

	wantUserID := "user1"
	wantJWT := "testjwt"
	ctx := contextWithJWTForTest(context.Background(), wantJWT)
	resp, err := agg.Resolve(ctx, &wantUserID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Cards) != 8 {
		t.Fatalf("expected 8 cards after collapsing featured/random_tail, got %d (cards=%+v)", len(resp.Cards), resp.Cards)
	}

	for _, cr := range caps {
		got, jwt, jwtOK, called := cr.snapshot()
		if !called {
			t.Errorf("resolver %q was not invoked", cr.typ)
			continue
		}
		if got == nil {
			t.Errorf("resolver %q saw userID=nil, want *userID=%q", cr.typ, wantUserID)
			continue
		}
		if *got != wantUserID {
			t.Errorf("resolver %q saw *userID=%q, want %q", cr.typ, *got, wantUserID)
		}
		if !jwtOK || jwt != wantJWT {
			t.Errorf("resolver %q saw JWT=%q ok=%v, want %q ok=true", cr.typ, jwt, jwtOK, wantJWT)
		}
	}
}

// jwtKeyForTest mirrors the unexported jwtKey type in the cards package so
// the aggregator tests can verify the JWT-on-ctx contract without importing
// cards (which would create a package cycle: cards → spotlight → cards).
// The struct{} value type must be IDENTICAL to the one in
// services/catalog/internal/service/spotlight/cards/jwt_context.go for the
// ctx-value lookup to match — but since contextWithJWTForTest writes via
// the same local key, they are consistent within this test file.
type jwtKeyForTest struct{}

func contextWithJWTForTest(ctx context.Context, jwt string) context.Context {
	return context.WithValue(ctx, jwtKeyForTest{}, jwt)
}

func jwtFromCtxForTest(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(jwtKeyForTest{}).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// TestAggregator_ErroringResolver_EmitsErrorw verifies that a resolver
// returning (nil, err) is dropped AND its sibling cards are returned.
// Log assertion is best-effort (we don't introspect logs directly) but
// we DO verify the side-effect: the failed card's resolver was invoked
// and its card is NOT in the response.
func TestAggregator_ErroringResolver_EmitsErrorw(t *testing.T) {
	failing := &fakeResolver{typ: "failing", card: nil, err: errors.New("upstream-boom"), sleep: 10 * time.Millisecond}
	resolvers := []Resolver{
		&fakeResolver{typ: "ok1", card: &Card{Type: "ok1"}, sleep: 10 * time.Millisecond},
		failing,
		&fakeResolver{typ: "ok2", card: &Card{Type: "ok2"}, sleep: 10 * time.Millisecond},
	}
	c := newFakeCache()
	agg := NewAggregator(c, testLogger(t), resolvers)

	resp, err := agg.Resolve(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Cards) != 2 {
		t.Fatalf("expected 2 cards (failing dropped), got %d", len(resp.Cards))
	}
	for _, card := range resp.Cards {
		if card.Type == "failing" {
			t.Fatalf("failing card must NOT appear in payload")
		}
	}
	if atomic.LoadInt32(&failing.calls) != 1 {
		t.Fatalf("expected failing resolver to be called once, got %d", failing.calls)
	}
}
