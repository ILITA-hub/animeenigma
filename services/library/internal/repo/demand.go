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
// ON CONFLICT (mal_id, episode) DO UPDATE upsert via clause.OnConflict, so
// concurrent demand for the same episode collapses to a single row by the
// composite primary key. Phase 09 wires the three producers: backfill (catalog
// serve MISS), next_ep (player Logic B), ongoing (scheduler Logic A).
//
// ON CONFLICT we refresh `reason` (last-writer-wins) but DELIBERATELY DO NOT
// refresh `requested_at`:
//
//   - WR-02 (reason refresh): a (mal,ep) first seen as 'backfill' that Logic A
//     later re-asserts as 'ongoing' (or Logic B as 'next_ep') MUST update the
//     stored reason — otherwise the Planner derives the wrong OBS-04
//     downloads_total{trigger} label, mis-attributing an A/B-driven download to
//     'backfill'. Keeping A/B/backfill separable is the whole point of CONTEXT
//     decision 7 (and the 'ongoing' enum value migration 010 added). Last-
//     writer-wins is correct: the most recent producer is the one actually
//     driving the still-unsatisfied download right now.
//
//   - WR-01 (NO requested_at bump): Drain orders requested_at ASC (FIFO). Logic A
//     re-asserts every ongoing demand each sweep; bumping requested_at=now() on
//     every re-assert kept stamping those rows fresh and sinking them behind
//     static backfill demands (whose requested_at never refreshes) — a starvation
//     hazard against the fanout cap. Preserving the ORIGINAL first-seen
//     requested_at makes the ASC ordering stable, so a frequently-re-asserted
//     ongoing demand holds its queue position rather than being pushed to the back.
//
// RequestedAt is set explicitly to time.Now() on INSERT rather than relying on
// the SQL `DEFAULT now()`: GORM only omits a zero-value column from the INSERT
// (letting the SQL default fire) when the field carries a `default:` tag, and
// RequestedAt is neither a magic CreatedAt/UpdatedAt name nor tagged — so without
// this the FIRST insert would land a zero-value 0001-01-01 timestamp and break
// the Planner's recency ordering. The ON CONFLICT path no longer touches
// requested_at (WR-01), so the first-seen time is the durable FIFO key.
func (r *DemandRepository) Record(ctx context.Context, malID string, episode int, reason domain.DemandReason, titles []string) error {
	joined := domain.JoinTitles(titles)
	row := &domain.AutocacheDemand{MALID: malID, Episode: episode, Reason: reason, RequestedAt: time.Now(), Titles: joined}
	// WR-02: refresh reason (last-writer-wins). Refresh titles too — but only when
	// the incoming set is non-empty, so a later title-less re-assert (e.g. a legacy
	// caller) can't blank a populated column. WR-01: never refresh requested_at.
	updateCols := []string{"reason"}
	if joined != "" {
		updateCols = append(updateCols, "titles")
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "mal_id"}, {Name: "episode"}},
			DoUpdates: clause.AssignmentColumns(updateCols),
		}).
		Create(row).Error
	if err != nil {
		return liberrors.Wrap(err, liberrors.CodeInternal, "record demand")
	}
	return nil
}

// Drain returns up to `limit` autocache_demand rows ordered by requested_at ASC
// (oldest first), the FIFO order the Phase-09 Planner consumes. Since Record no
// longer bumps requested_at on a re-assert (WR-01), requested_at is the durable
// FIRST-SEEN time, so this ASC ordering is stable: a frequently-re-asserted
// ongoing demand keeps its position instead of starving behind static backfill
// rows. The `limit` bound is load-bearing (T-09-02): the Planner caps the batch
// so an unbounded autocache_demand table can never be loaded into memory in one
// sweep. The repo
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
