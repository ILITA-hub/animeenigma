package signals

// S11Filter — candidate-pool gate (anonymous + per-user paths).
//
// S11 is described in the spec as a "candidate-pool filter" rather than a
// SignalModule. It runs BEFORE the ensemble: the handler asks S11 for the set
// of valid candidate anime IDs, then passes that slice to Ensemble.Rank.
//
// Two paths:
//
//   - CandidatePool(ctx)              — anonymous (Phase 10). Filters out
//     admin-hidden anime and soft-deleted rows.
//   - CandidatePoolForUser(ctx, uid)  — logged-in (Phase 11). Anonymous filter
//     plus exclusion of anime the user has marked completed or dropped
//     (REC-UX-04). The anonymous path is intentionally untouched so the
//     anonymous trending row keeps its existing semantics.

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

// CandidatePoolForUser returns the IDs of every anime eligible for ranking
// in the logged-in "Up Next for you" row: the anonymous CandidatePool minus
// anime that the caller has marked 'completed' or 'dropped' in their
// watchlist. Phase 11 — REC-UX-04.
//
// Active statuses ('watching', 'planned', 'on_hold') and anime with no
// anime_list row at all are kept — the LEFT JOIN treats al.status IS NULL
// as a pass.
func (f *S11Filter) CandidatePoolForUser(ctx context.Context, userID recs.UserID) ([]recs.AnimeID, error) {
	var ids []recs.AnimeID
	err := f.db.WithContext(ctx).
		Table("animes AS a").
		Select("a.id").
		Joins("LEFT JOIN anime_list al ON al.anime_id = a.id AND al.user_id = ?", userID).
		Where("a.hidden = ?", false).
		Where("a.deleted_at IS NULL").
		Where("(al.status IS NULL OR al.status NOT IN ?)", []string{"completed", "dropped"}).
		Pluck("a.id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}
