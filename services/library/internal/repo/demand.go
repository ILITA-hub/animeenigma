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
