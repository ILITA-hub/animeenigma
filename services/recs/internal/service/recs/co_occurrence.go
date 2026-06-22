// Package recs — co_occurrence.go: nightly cron that materializes
// rec_completion_co_occurrence at score>=7.
//
// Phase 13 (REC-SIG-06). The S6 pin cascade reads from this materialized
// table on every recs request — a fresh INSERT...ON CONFLICT DO UPDATE
// run every 24h is enough at production scale (~2k completed-with-score>=7
// rows produces millisecond-scale wall time; at 100k users the query is
// minute-scale).
//
// Mirrors PopulationOrchestrator (population.go) — same boot-tick semantics,
// same stale-serves-on-failure contract.
package recs

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"gorm.io/gorm"
)

// CoOccurrenceOrchestrator materializes rec_completion_co_occurrence.
// Populated nightly at score>=7. The score>=5 fallback path is queried
// on-demand by the S6 cascade and is NOT pre-materialized — it's rare
// enough that the storage cost would not be worth it.
type CoOccurrenceOrchestrator struct {
	db  *gorm.DB
	log *logger.Logger

	// tickTimeout bounds each Start-driven RunOnce so a hung materialize
	// aborts instead of stalling the 24h ticker forever (audit L641).
	// Defaults to coOccurrenceTickTimeout; tests override it.
	tickTimeout time.Duration

	// lastTickHadDeadline records (for tests) whether the most recent
	// runTick passed a deadlined context into RunOnce.
	lastTickHadDeadline bool
}

// coOccurrenceTickTimeout bounds a single materialize run. Well under the 24h
// production cadence (audit L641).
const coOccurrenceTickTimeout = 1 * time.Hour

func NewCoOccurrenceOrchestrator(db *gorm.DB, log *logger.Logger) *CoOccurrenceOrchestrator {
	return &CoOccurrenceOrchestrator{db: db, log: log, tickTimeout: coOccurrenceTickTimeout}
}

// coOccurrenceMaterializeSQL is the binding cron query (Decision §B4).
// Both Postgres and SQLite support every clause used here:
//   - INSERT...ON CONFLICT (composite PK) DO UPDATE: Postgres + SQLite >=3.24
//   - COUNT(DISTINCT): Postgres + SQLite
//
// last_computed is stamped with a single run-generation timestamp (the bind
// param) rather than CURRENT_TIMESTAMP so it exactly equals the run-start
// boundary the reap DELETE compares against — refreshed/inserted rows survive
// the reap, dropped pairs (which keep their older stamp) do not.
//
// The query is idempotent — re-running on a stable anime_list state produces
// the same rows with refreshed last_computed timestamps.
const coOccurrenceMaterializeSQL = `
	INSERT INTO rec_completion_co_occurrence (seed_anime_id, candidate_anime_id, co_count, last_computed)
	SELECT a.anime_id, b.anime_id, COUNT(DISTINCT a.user_id) AS co_count, ?
	FROM anime_list a
	JOIN anime_list b ON a.user_id = b.user_id AND a.anime_id <> b.anime_id
	WHERE a.status = 'completed' AND a.score >= 7
	  AND b.status = 'completed' AND b.score >= 7
	GROUP BY a.anime_id, b.anime_id
	HAVING COUNT(DISTINCT a.user_id) >= 1
	ON CONFLICT (seed_anime_id, candidate_anime_id) DO UPDATE
	  SET co_count = EXCLUDED.co_count, last_computed = EXCLUDED.last_computed`

// coOccurrenceReapSQL deletes any row whose last_computed predates this run's
// generation timestamp. Because the upsert refreshes last_computed for every
// still-co-occurring pair to exactly the run-start boundary, only pairs that
// dropped out (or pre-existing stale rows) keep an older stamp and get reaped
// (audit L634). This makes the cron authoritative for the materialized set.
const coOccurrenceReapSQL = `DELETE FROM rec_completion_co_occurrence WHERE last_computed < ?`

// RunOnce materializes the co-occurrence matrix and reaps stale rows in a
// single transaction. Returns nil on success, a wrapped error on failure.
// Caller (Start, or production main) is responsible for logging — RunOnce
// returns the error so unit tests can assert on it.
//
// Authoritative full-rebuild-by-generation (audit L634): capture a run-start
// boundary, upsert every currently-co-occurring pair stamped with that
// boundary, then DELETE every row whose last_computed predates it. The
// delete-stale-after-upsert ordering keeps the table continuously non-empty
// (no TRUNCATE window) while still reaping pairs that stopped co-occurring.
func (o *CoOccurrenceOrchestrator) RunOnce(ctx context.Context) error {
	// Generation boundary stamped into the upsert AND used as the reap cutoff.
	// Truncated to seconds so it round-trips identically across the SQLite
	// DATETIME ('YYYY-MM-DD HH:MM:SS') and Postgres timestamp text formats —
	// sub-second precision could make a refreshed row compare as strictly less
	// than the cutoff and be wrongly reaped.
	runStart := time.Now().UTC().Truncate(time.Second)

	return o.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(coOccurrenceMaterializeSQL, runStart).Error; err != nil {
			return fmt.Errorf("recs: co-occurrence materialize: %w", err)
		}
		if err := tx.Exec(coOccurrenceReapSQL, runStart).Error; err != nil {
			return fmt.Errorf("recs: co-occurrence reap stale: %w", err)
		}
		return nil
	})
}

// runTick executes one RunOnce under a per-tick timeout derived from ctx
// (audit L641) so a hung materialize aborts instead of stalling the ticker. On
// success it advances the recs_cron_last_success_unixtime{cron="co_occurrence"}
// gauge so a frozen cron is observable in Grafana.
func (o *CoOccurrenceOrchestrator) runTick(ctx context.Context, phase string) {
	budget := o.tickTimeout
	if budget <= 0 {
		budget = coOccurrenceTickTimeout
	}
	tickCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	_, o.lastTickHadDeadline = tickCtx.Deadline()
	if err := o.RunOnce(tickCtx); err != nil {
		o.log.Errorw("co-occurrence cron failed ("+phase+")", "error", err)
		// Do NOT return up the goroutine — the caller continues ticking. Stale
		// data continues serving until the next successful tick.
		return
	}
	metrics.RecsCronLastSuccessUnixtime.WithLabelValues("co_occurrence").SetToCurrentTime()
	o.log.Infow("co-occurrence cron " + phase + " complete")
}

// Start spawns a goroutine that fires RunOnce immediately (boot tick) and
// then once every `interval` thereafter. Cancelling ctx exits the goroutine.
//
// A failing tick is logged via o.log.Errorw and the goroutine continues — the
// stale-serves-on-failure contract identical to PopulationOrchestrator.
func (o *CoOccurrenceOrchestrator) Start(ctx context.Context, interval time.Duration) {
	go func() {
		// Boot tick — populate within seconds of redeploy so the S6 cascade
		// works immediately on cold start without waiting 24h.
		o.runTick(ctx, "boot tick")

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				o.log.Infow("co-occurrence cron stopped")
				return
			case <-ticker.C:
				o.runTick(ctx, "tick")
			}
		}
	}()
}
