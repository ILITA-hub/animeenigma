package recs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	"gorm.io/gorm"
)

// userOrchestratorCache is the narrow cache surface the user orchestrator
// depends on (Go interface-from-the-consumer). *libs/cache.RedisCache satisfies
// this in production; tests inject a fake. We keep the dependency local so the
// recs package doesn't import libs/cache at build time.
type userOrchestratorCache interface {
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
}

// Cache key prefixes for the user-scope rec engine. Exported so handler
// and main.go can construct the same keys without re-defining magic strings.
//
// :v5 — S8 franchise signal joined the logged-in ensemble (2026-07-17);
// v4 was S12-without-S8.
const (
	UserTopNKeyPrefix = "recs:user:"
	UserTopNKeySuffix = ":topN:v5"
	DebounceKeyPrefix = "recs:debounce:"

	// debounceTTL is the per-user debounce window: at most one
	// TriggerForUser-driven precompute will run every 5 minutes per user
	// across all replicas, regardless of how many MarkEpisodeWatched events
	// fire. Spec §13 / CONTEXT.md decisions §Debounced On-Write Trigger.
	debounceTTL = 5 * time.Minute

	// triggerPrecomputeTimeout bounds a fire-and-forget precompute goroutine
	// so a runaway query doesn't survive past the next debounce window.
	triggerPrecomputeTimeout = 5 * time.Minute

	// userTickTimeout bounds a whole RunOnce sweep under Start. Well under the
	// 6h production cadence so a hung sweep aborts and the next tick fires
	// instead of stalling the ticker forever (audit L641).
	userTickTimeout = 30 * time.Minute

	// userPerUserTimeout bounds a single user's precompute inside RunOnce so
	// one slow user cannot starve the rest of the sweep (audit L648).
	userPerUserTimeout = 2 * time.Minute
)

// UserTopNKey returns the per-user topN cache key in the canonical shape so
// handler / cron / trigger paths all agree.
func UserTopNKey(userID UserID) string {
	return UserTopNKeyPrefix + userID + UserTopNKeySuffix
}

// debounceKey returns the per-user SetNX lock key.
func debounceKey(userID UserID) string {
	return DebounceKeyPrefix + userID
}

// UserOrchestrator runs per-user Precompute on a fixed cadence (6 hours in
// production) and supports debounced on-write triggers from
// ListService.MarkEpisodeWatched. It delegates the actual per-user work to a
// recs.Orchestrator (precompute.go) — this type owns the iteration / cron /
// debounce concerns only.
//
// Spec ref: docs/superpowers/specs/2026-05-03-rec-engine-design.md §5; phase
// plan REC-INFRA-02.
type UserOrchestrator struct {
	precompute *Orchestrator
	db         *gorm.DB
	cache      userOrchestratorCache
	log        *logger.Logger

	// tickTimeout bounds each Start-driven RunOnce sweep; perUserTimeout
	// bounds a single user's precompute inside RunOnce. Default to the
	// userTickTimeout / userPerUserTimeout consts; tests override with tiny
	// budgets.
	tickTimeout    time.Duration
	perUserTimeout time.Duration
}

// NewUserOrchestrator wires the orchestrator with a precompute Orchestrator
// (carrying the user-scope SignalModules — S1, S2 in Phase 11), the player DB
// handle, a Redis-backed cache, and a structured logger.
func NewUserOrchestrator(precompute *Orchestrator, db *gorm.DB, cache userOrchestratorCache, log *logger.Logger) *UserOrchestrator {
	return &UserOrchestrator{
		precompute:     precompute,
		db:             db,
		cache:          cache,
		log:            log,
		tickTimeout:    userTickTimeout,
		perUserTimeout: userPerUserTimeout,
	}
}

// RunOnce iterates every distinct user_id in watch_history and calls the
// precompute orchestrator for each. Per-user errors are collected (via
// errors.Join) but do NOT halt the iteration — one user's failure does not
// starve the others. On per-user success the topN cache key is Deleted so
// the next request rebuilds the row with fresh signals; on failure the cache
// is left intact (stale-serves contract, mirrors PopulationOrchestrator).
func (o *UserOrchestrator) RunOnce(ctx context.Context) error {
	var userIDs []string
	if err := o.db.WithContext(ctx).
		Table("watch_history").
		Distinct("user_id").
		Pluck("user_id", &userIDs).Error; err != nil {
		return fmt.Errorf("recs: load distinct watch_history users: %w", err)
	}

	// Build the once-per-tick shared precompute context (audit L648): signals
	// that implement SharedPrecomputer (e.g. S5's population-scope IDF) compute
	// their cross-user state once here and seed it into sharedCtx, so the
	// per-user loop reuses it instead of recomputing it for every user. A
	// failed shared step is non-fatal — each signal falls back to inline
	// per-user computation — so we log and continue with the best-effort ctx.
	sharedCtx, sharedErr := o.precompute.BuildSharedContext(ctx)
	if sharedErr != nil {
		o.log.Warnw("recs shared precompute step failed; falling back to per-user inline", "error", sharedErr)
	}

	// Resolve the per-user budget once (tests may leave the field zero when
	// constructing the struct directly; fall back to the prod default).
	perUser := o.perUserTimeout
	if perUser <= 0 {
		perUser = userPerUserTimeout
	}

	var errs []error
	for _, uid := range userIDs {
		// Bound each user so one slow user can't starve the rest of the sweep
		// (audit L648). Composes with the per-tick timeout in Start (L641).
		// Derive from sharedCtx so per-user work reuses the hoisted IDF.
		uCtx, cancel := context.WithTimeout(sharedCtx, perUser)
		err := o.precompute.RunForUser(uCtx, uid)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("recs: user precompute %q: %w", uid, err))
			// Stale-serves: do NOT delete cache on failure.
			continue
		}
		if err := o.cache.Delete(ctx, UserTopNKey(uid)); err != nil {
			// Cache delete failure is not catastrophic — the cache will hit
			// natural expiry within the 6h TTL — but log so we notice if it
			// becomes systemic.
			o.log.Warnw("recs user cache delete failed", "user_id", uid, "error", err)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// runTick executes one RunOnce sweep under a per-tick timeout derived from ctx
// (audit L641) so a hung sweep aborts instead of stalling the ticker. On
// success it advances the recs_cron_last_success_unixtime{cron="user"} gauge
// so a frozen cron is observable in Grafana.
func (o *UserOrchestrator) runTick(ctx context.Context, phase string) {
	budget := o.tickTimeout
	if budget <= 0 {
		budget = userTickTimeout
	}
	tickCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if err := o.RunOnce(tickCtx); err != nil {
		o.log.Errorw("user precompute failed ("+phase+")", "error", err)
		return
	}
	metrics.RecsCronLastSuccessUnixtime.WithLabelValues("user").SetToCurrentTime()
	o.log.Infow("user precompute " + phase + " complete")
}

// Start spawns a goroutine that fires RunOnce immediately (boot tick) and
// then once every `interval` thereafter. Cancelling ctx exits the goroutine.
//
// A failing tick is logged via Errorw and the goroutine continues — cron
// failure must NOT crash the service. Mirrors PopulationOrchestrator.Start.
func (o *UserOrchestrator) Start(ctx context.Context, interval time.Duration) {
	go func() {
		// Boot tick — get fresh signals within seconds of redeploy so logged-in
		// users see personalised recs without waiting 6 hours.
		o.runTick(ctx, "boot tick")

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				o.log.Infow("user precompute cron stopped")
				return
			case <-ticker.C:
				o.runTick(ctx, "tick")
			}
		}
	}()
}

// TriggerForUser fires a debounced per-user precompute. Called from
// ListService.MarkEpisodeWatched (in a fire-and-forget goroutine). The Redis
// SetNX lock guarantees at most one precompute per user per debounceTTL
// window across all replicas; subsequent triggers within the window are
// silent no-ops.
//
// On a successful acquire, a goroutine runs the precompute under a 5-minute
// timeout context (so a runaway query doesn't outlive the next debounce
// window) and Deletes the per-user topN cache on success. On failure the
// cache is left intact — same stale-serves contract as RunOnce.
//
// Always returns nil. The caller (list.go) cannot do anything useful with an
// error here: this trigger is best-effort, the cron picks up the same user
// within 6 hours regardless. We log and move on.
func (o *UserOrchestrator) TriggerForUser(ctx context.Context, userID UserID) error {
	key := debounceKey(userID)
	acquired, err := o.cache.SetNX(ctx, key, "1", debounceTTL)
	if err != nil {
		o.log.Errorw("recs debounce SetNX failed", "user_id", userID, "error", err)
		return nil
	}
	if !acquired {
		o.log.Debugw("recs debounce hit; skipping trigger", "user_id", userID)
		return nil
	}

	// Fire-and-forget. Use context.Background() so a cancelled request
	// context (e.g. the HTTP handler's request returning) doesn't cancel the
	// precompute mid-flight.
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), triggerPrecomputeTimeout)
		defer cancel()
		// Seed a named origin so frame-less db_writes from this detached
		// fire-and-forget precompute attribute to "goroutine/recs-trigger"
		// instead of "goroutine/unknown".
		bgCtx = tracing.SeedBaggage(bgCtx, "recs-trigger", "")

		if err := o.precompute.RunForUser(bgCtx, userID); err != nil {
			o.log.Errorw("recs trigger precompute failed", "user_id", userID, "error", err)
			// Stale-serves: do NOT delete cache on failure.
			return
		}
		if err := o.cache.Delete(bgCtx, UserTopNKey(userID)); err != nil {
			o.log.Warnw("recs trigger cache delete failed", "user_id", userID, "error", err)
		}
	}()
	return nil
}
