package signals

// S11Filter — Phase 10 scope.
//
// S11 in the spec is described as a "candidate-pool filter" rather than a
// SignalModule. It runs BEFORE the ensemble: the handler asks S11 for the set
// of valid candidate anime IDs, then passes that slice to Ensemble.Rank. Score
// values are not part of S11's job.
//
// Phase 10 scope is narrow on purpose:
//
//   - Filter out admin-hidden anime (animes.hidden = true)
//   - Filter out soft-deleted rows (animes.deleted_at IS NOT NULL)
//
// Phase 11 (per CONTEXT.md decisions §S11) extends this with the user-specific
// layer: anime that the caller has already marked as "completed" or "dropped"
// in their watchlist are also excluded. That layer requires a userID, so it
// can't ship in the anonymous-only Phase 10 surface.
//
// When Phase 11 lands, S11Filter will likely sprout a second method
// (CandidatePoolForUser(ctx, userID)) and the existing CandidatePool will
// remain the anonymous/population path. Plan accordingly when extending.

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/player/internal/service/recs"
	"gorm.io/gorm"
)

// S11Filter exposes the candidate pool used by the ensemble.
type S11Filter struct {
	db *gorm.DB
}

// NewS11Filter wires S11 with the player DB handle.
func NewS11Filter(db *gorm.DB) *S11Filter {
	return &S11Filter{db: db}
}

// CandidatePool returns the IDs of every anime eligible for ranking in the
// anonymous trending row. Order is unspecified (the ensemble sorts by score
// after running). On empty animes table, returns an empty slice and nil error.
func (f *S11Filter) CandidatePool(ctx context.Context) ([]recs.AnimeID, error) {
	var ids []recs.AnimeID
	err := f.db.WithContext(ctx).
		Table("animes").
		Where("hidden = ?", false).
		Where("deleted_at IS NULL").
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}
