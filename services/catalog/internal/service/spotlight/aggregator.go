package spotlight

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
)

// Aggregator-level constants — HSB-BE-03 (per-card) and HSB-BE-04 (overall).
// These are the default values pinned to the production deadlines from the
// design doc. Tests that need to exercise the timeout branches with
// shorter budgets use NewAggregatorWithDeadlines.
const (
	// perCardDeadline gates each resolver — HSB-BE-03.
	perCardDeadline = 800 * time.Millisecond
	// overallBudget is the aggregator-level cap — HSB-BE-04.
	overallBudget = 2 * time.Second
	// snapshotTTL is the per-day fallback retention — HSB-BE-04.
	snapshotTTL = 24 * time.Hour
)

// Aggregator dispatches per-card resolvers concurrently and assembles
// the spotlight response. Each resolver runs in its own goroutine under a
// per-card context.WithTimeout (default 800ms) and the parent context
// carries an overall 2s budget. Failed or timed-out resolvers drop their
// card and emit one structured Errorw log line; resolvers returning
// (nil, nil) drop silently. On a zero-card outcome the aggregator
// attempts a snapshot fallback via Redis; on success it writes the
// snapshot best-effort via a detached context.Background() goroutine.
//
// Pattern lifted from services/catalog/internal/service/subs_aggregator.go
// lines 109-156 (sync.WaitGroup + buffered chan + drop-on-error). Three
// deliberate adaptations for spotlight (per 01-PATTERNS.md):
//   1. Per-card ctx.WithTimeout instead of trusting the outer ctx.
//   2. (nil, nil) is a silent-drop, not a log (eligibility=false contract).
//   3. Detached snapshot write because the request ctx is about to be
//      cancelled by the caller when Resolve returns.
type Aggregator struct {
	cache     cache.Cache
	log       *logger.Logger
	resolvers []Resolver
	perCard   time.Duration // default perCardDeadline
	overall   time.Duration // default overallBudget
}

// NewAggregator constructs an Aggregator with the production deadlines
// pinned to perCardDeadline (800ms) and overallBudget (2s). resolvers
// may be nil — the constructor normalises it to an empty (non-nil)
// slice so callers can range over Resolvers() without an NPE guard.
func NewAggregator(c cache.Cache, log *logger.Logger, resolvers []Resolver) *Aggregator {
	if resolvers == nil {
		resolvers = []Resolver{}
	}
	return &Aggregator{
		cache:     c,
		log:       log,
		resolvers: resolvers,
		perCard:   perCardDeadline,
		overall:   overallBudget,
	}
}

// NewAggregatorWithDeadlines is the test-friendly constructor that lets
// a caller override the per-card and overall deadlines. Production code
// uses NewAggregator; tests use this to drive the timeout branches with
// shorter / aggressive budgets without inflating test runtime.
func NewAggregatorWithDeadlines(c cache.Cache, log *logger.Logger, resolvers []Resolver, perCard, overall time.Duration) *Aggregator {
	a := NewAggregator(c, log, resolvers)
	a.perCard = perCard
	a.overall = overall
	return a
}

// Resolvers returns the configured resolver slice. Exported for tests
// only — the field itself stays unexported per Go convention.
func (a *Aggregator) Resolvers() []Resolver {
	return a.resolvers
}

// resolveResult is the per-goroutine collection envelope. card == nil
// with err == nil means "eligible=false, drop silently". card == nil
// with err != nil means "resolver failed, drop + log one Errorw". A
// non-nil card is always included in the response.
type resolveResult struct {
	name string
	card *Card
	err  error
}

// Resolve assembles the spotlight Response for the given user (nil =
// anonymous). Fans out across all configured resolvers concurrently
// under a per-card and overall deadline; collects whichever cards
// completed within budget; falls back to a stored snapshot when zero
// cards resolved; writes the response back as a new snapshot
// (detached, best-effort) on a non-empty result.
//
// Error contract: returns (resp, nil) for all partial-success and
// total-failure paths — the caller (handler) never sees a non-nil error
// here. Only catastrophic mis-use (cache nil, log nil) would panic,
// which is a programming error, not a runtime one.
func (a *Aggregator) Resolve(ctx context.Context, userID *string) (*Response, error) {
	started := time.Now()

	// Overall budget — HSB-BE-04. Wraps the caller's ctx so a long-running
	// resolver cannot outlive the request. The defer cancel() releases the
	// internal timer goroutine when Resolve returns.
	ctx, cancel := context.WithTimeout(ctx, a.overall)
	defer cancel()

	if len(a.resolvers) == 0 {
		// No resolvers wired (test path or feature-half-built) — try the
		// snapshot, else an empty response. Frontend treats empty cards as
		// "hide the block" (HSB-FE-02).
		if snap := a.loadSnapshot(ctx, userID); snap != nil {
			return snap, nil
		}
		return &Response{
			Cards:       []Card{},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	// Buffered channel — capacity equal to len(resolvers) so goroutines
	// never block on send even if the collector loop is delayed by a
	// scheduling hiccup.
	resultsCh := make(chan resolveResult, len(a.resolvers))
	var wg sync.WaitGroup

	for _, r := range a.resolvers {
		wg.Add(1)
		// Pass r as a goroutine parameter to defuse the pre-1.22 loop-var
		// capture bug; harmless under 1.22+ but keeps the pattern
		// canonical (matches subs_aggregator.go).
		go func(r Resolver) {
			defer wg.Done()
			// Per-card deadline — HSB-BE-03. Each resolver gets its own
			// 800ms ctx; a slow resolver drops its card without
			// affecting siblings. Carved out of the parent overall ctx.
			cctx, cancel := context.WithTimeout(ctx, a.perCard)
			defer cancel()
			card, err := r.Resolve(cctx, userID)
			resultsCh <- resolveResult{name: r.Type(), card: card, err: err}
		}(r)
	}

	// Close the channel once all goroutines have published their result.
	// This is what lets the range loop terminate cleanly.
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// CRITICAL: cards := []Card{} (NOT `var cards []Card`) so the empty
	// case marshals to JSON `[]` not `null`. Phase 2 frontend treats
	// `null` as a parse failure (see types.go Response comment).
	cards := []Card{}
	for res := range resultsCh {
		if res.err != nil {
			// Per HSB-BE-03: dropped card gets exactly ONE structured log
			// line. Errorw (NOT Warnw) per the design doc's logging
			// contract — Warnw is reserved for snapshot infra failures.
			a.log.Errorw("spotlight.card_failed", "type", res.name, "error", res.err)
			continue
		}
		if res.card == nil {
			// Eligible=false — silent drop, no log line (Plan 01
			// Resolver contract). The resolver decided there is no
			// data to surface today; that is a legitimate signal,
			// not an error.
			continue
		}
		cards = append(cards, *res.card)
	}

	// Zero-card outcome — try snapshot fallback (HSB-BE-04). When every
	// resolver was either ineligible or failed, return the last-known-good
	// payload if Redis still has one.
	if len(cards) == 0 {
		if snap := a.loadSnapshot(ctx, userID); snap != nil {
			return snap, nil
		}
	}

	resp := &Response{
		Cards:       cards,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Best-effort snapshot write — detached ctx because the request ctx
	// is about to be cancelled by the caller when this function returns.
	// Fire-and-forget; never blocks the response path. Only write on
	// ≥1 card to avoid baking an empty result into a 24h snapshot that
	// would mask a transient zero-card outage.
	if len(cards) > 0 {
		// Detached ctx (request ctx is about to be cancelled), but seed a named
		// origin so the snapshot's cache effect attributes to
		// "goroutine/spotlight-snapshot [success]" instead of the bare
		// "unknown [success]".
		go a.saveSnapshot(tracing.SeedBaggage(context.Background(), "spotlight-snapshot", ""), userID, resp)
	}

	a.log.Infow("spotlight.aggregated",
		"cards_returned", len(cards),
		"ms_total", time.Since(started).Milliseconds(),
	)

	return resp, nil
}

// loadSnapshot returns the cached fallback Response for the given userID,
// or nil if none exists / Redis is down. Only errors.Is(err, cache.ErrNotFound)
// is the "no snapshot exists" path that emits no log line — other errors
// get a Warnw (snapshot infra problem, not a card-level failure) and
// return nil so the request continues with whatever cards resolved.
//
// DELIBERATE DIVERGENCE 2 (per Plan 01-03): the sentinel check uses
// errors.Is(err, cache.ErrNotFound), NOT a generic err != nil check.
// This is intentional so a hard Redis failure stays distinguishable from
// a clean cache miss in logs.
func (a *Aggregator) loadSnapshot(ctx context.Context, userID *string) *Response {
	// Defensive: a nil cache (test wiring or feature-half-built) means
	// snapshot infra is unreachable. Treat as a clean miss — no log,
	// no panic — so the caller continues with empty cards.
	if a.cache == nil {
		return nil
	}
	var snap Response
	err := a.cache.Get(ctx, SnapshotKey(userID), &snap)
	if err == nil {
		return &snap
	}
	if !errors.Is(err, cache.ErrNotFound) {
		a.log.Warnw("spotlight.snapshot_load_failed", "error", err)
	}
	return nil
}

// saveSnapshot writes the best-known-good Response to Redis under the
// per-day snapshot key. The caller is expected to invoke this in a
// detached goroutine with context.Background() because the request ctx
// is cancelled as soon as Resolve returns. Errors are Warnw and dropped —
// a snapshot-write failure must never affect the response that has
// already been built.
func (a *Aggregator) saveSnapshot(ctx context.Context, userID *string, resp *Response) {
	// Defensive: nil cache (test wiring or feature-half-built) means
	// the write is silently a no-op — symmetric with loadSnapshot.
	if a.cache == nil {
		return
	}
	if err := a.cache.Set(ctx, SnapshotKey(userID), resp, snapshotTTL); err != nil {
		a.log.Warnw("spotlight.snapshot_save_failed", "error", err)
	}
}
