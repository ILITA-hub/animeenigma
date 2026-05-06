package recs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
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
const (
	UserTopNKeyPrefix = "recs:user:"
	UserTopNKeySuffix = ":topN"
	DebounceKeyPrefix = "recs:debounce:"

	// debounceTTL is the per-user debounce window: at most one
	// TriggerForUser-driven precompute will run every 5 minutes per user
	// across all replicas, regardless of how many MarkEpisodeWatched events
	// fire. Spec §13 / CONTEXT.md decisions §Debounced On-Write Trigger.
	debounceTTL = 5 * time.Minute

	// triggerPrecomputeTimeout bounds a fire-and-forget precompute goroutine
	// so a runaway query doesn't survive past the next debounce window.
	triggerPrecomputeTimeout = 5 * time.Minute
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
}

// NewUserOrchestrator wires the orchestrator with a precompute Orchestrator
// (carrying the user-scope SignalModules — S1, S2 in Phase 11), the player DB
// handle, a Redis-backed cache, and a structured logger.
func NewUserOrchestrator(precompute *Orchestrator, db *gorm.DB, cache userOrchestratorCache, log *logger.Logger) *UserOrchestrator {
	return &UserOrchestrator{
		precompute: precompute,
		db:         db,
		cache:      cache,
		log:        log,
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

	var errs []error
	for _, uid := range userIDs {
		if err := o.precompute.RunForUser(ctx, uid); err != nil {
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

// Start spawns a goroutine that fires RunOnce immediately (boot tick) and
// then once every `interval` thereafter. Cancelling ctx exits the goroutine.
//
// A failing tick is logged via Errorw and the goroutine continues — cron
// failure must NOT crash the service. Mirrors PopulationOrchestrator.Start.
func (o *UserOrchestrator) Start(ctx context.Context, interval time.Duration) {
	go func() {
		// Boot tick — get fresh signals within seconds of redeploy so logged-in
		// users see personalised recs without waiting 6 hours.
		if err := o.RunOnce(ctx); err != nil {
			o.log.Errorw("user precompute failed (boot tick)", "error", err)
		} else {
			o.log.Infow("user precompute boot tick complete")
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				o.log.Infow("user precompute cron stopped")
				return
			case <-ticker.C:
				if err := o.RunOnce(ctx); err != nil {
					o.log.Errorw("user precompute failed (tick)", "error", err)
					continue
				}
				o.log.Infow("user precompute tick complete")
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
