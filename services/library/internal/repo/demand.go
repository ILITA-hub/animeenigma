package repo

import (
	"context"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DemandRepository handles persistence for autocache_demand rows — the durable
// backfill-demand sink the ae serve MISS path writes into (Phase 08) and the
// Phase-09 Planner later drains. Mirrors the EpisodeRepository style:
// context-first, wrap GORM errors via liberrors so the HTTP layer maps cleanly.
type DemandRepository struct {
	db *gorm.DB
}

// NewDemandRepository constructs a DemandRepository over the provided *gorm.DB.
func NewDemandRepository(db *gorm.DB) *DemandRepository {
	return &DemandRepository{db: db}
}

// Record upserts a wanted (mal_id, episode) demand row. It is an
// ON CONFLICT (mal_id, episode) DO UPDATE SET requested_at = now() upsert via
// clause.OnConflict, so concurrent demand for the same episode collapses to a
// single row by the composite primary key and the row always reflects the
// most-recent want (recency refresh). Phase 08 always passes
// domain.DemandReasonBackfill; 'next_ep' is reserved for Phase 09.
//
// RequestedAt is set explicitly to time.Now() rather than relying on the SQL
// `DEFAULT now()` (CR-01): GORM only omits a zero-value column from the INSERT
// (letting the SQL default fire) when the field carries a `default:` tag, and
// RequestedAt is not a magic CreatedAt/UpdatedAt name nor tagged — so without
// this the FIRST insert would land a zero-value 0001-01-01 timestamp and break
// the Phase-09 Planner's recency ordering. The ON CONFLICT path keeps using
// gorm.Expr("now()") so a re-demand refreshes server-side.
func (r *DemandRepository) Record(ctx context.Context, malID string, episode int, reason domain.DemandReason) error {
	row := &domain.AutocacheDemand{MALID: malID, Episode: episode, Reason: reason, RequestedAt: time.Now()}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "mal_id"}, {Name: "episode"}},
			DoUpdates: clause.Assignments(map[string]any{"requested_at": gorm.Expr("now()")}),
		}).
		Create(row).Error
	if err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "record demand")
	}
	return nil
}

// Drain returns up to `limit` autocache_demand rows ordered by requested_at ASC
// (oldest first), the FIFO order the Phase-09 Planner consumes. The `limit` bound
// is load-bearing (T-09-02): the Planner caps the batch so an unbounded
// autocache_demand table can never be loaded into memory in one sweep. The repo
// method is lifecycle-agnostic — Drain only READS; the Planner decides when to
// Delete a satisfied row (per RESEARCH Pitfall 6, on confirmed presence, not
// speculatively). A non-positive limit returns no rows. Errors wrap CodeInternal.
func (r *DemandRepository) Drain(ctx context.Context, limit int) ([]domain.AutocacheDemand, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []domain.AutocacheDemand
	err := r.db.WithContext(ctx).
		Order("requested_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, liberrors.Wrap(err, liberrors.CodeInternal, "drain demand")
	}
	return rows, nil
}

// Delete removes the (mal_id, episode) demand row. Deleting an absent row is a
// no-op (returns nil) — the Planner calls this once an episode is confirmed
// present in the pool so the demand stops being re-drained. Errors wrap
// CodeInternal.
func (r *DemandRepository) Delete(ctx context.Context, malID string, episode int) error {
	err := r.db.WithContext(ctx).
		Where("mal_id = ? AND episode = ?", malID, episode).
		Delete(&domain.AutocacheDemand{}).Error
	if err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "delete demand")
	}
	return nil
}
