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
	"gorm.io/gorm"
)

// CoOccurrenceOrchestrator materializes rec_completion_co_occurrence.
// Populated nightly at score>=7. The score>=5 fallback path is queried
// on-demand by the S6 cascade and is NOT pre-materialized — it's rare
// enough that the storage cost would not be worth it.
type CoOccurrenceOrchestrator struct {
	db  *gorm.DB
	log *logger.Logger
}

func NewCoOccurrenceOrchestrator(db *gorm.DB, log *logger.Logger) *CoOccurrenceOrchestrator {
	return &CoOccurrenceOrchestrator{db: db, log: log}
}

// coOccurrenceMaterializeSQL is the binding cron query (Decision §B4).
// Both Postgres and SQLite support every clause used here:
//   - INSERT...ON CONFLICT (composite PK) DO UPDATE: Postgres + SQLite >=3.24
//   - COUNT(DISTINCT): Postgres + SQLite
//   - CURRENT_TIMESTAMP: ANSI-standard, both dialects
//
// The query is idempotent — re-running on a stable anime_list state produces
// the same rows with refreshed last_computed timestamps.
const coOccurrenceMaterializeSQL = `
	INSERT INTO rec_completion_co_occurrence (seed_anime_id, candidate_anime_id, co_count, last_computed)
	SELECT a.anime_id, b.anime_id, COUNT(DISTINCT a.user_id) AS co_count, CURRENT_TIMESTAMP
	FROM anime_list a
	JOIN anime_list b ON a.user_id = b.user_id AND a.anime_id <> b.anime_id
	WHERE a.status = 'completed' AND a.score >= 7
	  AND b.status = 'completed' AND b.score >= 7
	GROUP BY a.anime_id, b.anime_id
	HAVING COUNT(DISTINCT a.user_id) >= 1
	ON CONFLICT (seed_anime_id, candidate_anime_id) DO UPDATE
	  SET co_count = EXCLUDED.co_count, last_computed = EXCLUDED.last_computed`

// RunOnce executes the materialization SQL. Returns nil on success, a
// wrapped error on failure. Caller (Start, or production main) is
// responsible for logging — RunOnce returns the error so unit tests can
// assert on it.
func (o *CoOccurrenceOrchestrator) RunOnce(ctx context.Context) error {
	if err := o.db.WithContext(ctx).Exec(coOccurrenceMaterializeSQL).Error; err != nil {
		return fmt.Errorf("recs: co-occurrence materialize: %w", err)
	}
	return nil
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
		if err := o.RunOnce(ctx); err != nil {
			o.log.Errorw("co-occurrence cron failed (boot tick)", "error", err)
		} else {
			o.log.Infow("co-occurrence cron boot tick complete")
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				o.log.Infow("co-occurrence cron stopped")
				return
			case <-ticker.C:
				if err := o.RunOnce(ctx); err != nil {
					o.log.Errorw("co-occurrence cron failed (tick)", "error", err)
					// Do NOT return — continue ticking. Stale data continues
					// serving until the next successful tick.
					continue
				}
				o.log.Infow("co-occurrence cron tick complete")
			}
		}
	}()
}
