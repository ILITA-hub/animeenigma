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
//
// Phase 14 added FilterAudit for the admin debug page — returns the items
// the per-user filter would exclude with a reason string per category.

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

// FilteredOutEntry is one row in the admin-debug filter audit panel.
// Reason ∈ {"status=completed", "status=dropped", "hidden=true"}.
// Phase 14 (REC-ADMIN-01).
type FilteredOutEntry struct {
	AnimeID string `json:"anime_id"`
	Reason  string `json:"reason"`
}

// FilterAudit returns every anime the per-user S11 filter would exclude,
// with a reason string per applicable category. An anime that triggers
// multiple categories (e.g. hidden=true AND in the user's completed list)
// emits multiple rows — admins see every reason that applied.
//
// Soft-deleted anime (deleted_at IS NOT NULL) are EXCLUDED from the audit
// — they're never surfaced anywhere in the admin debug surface.
//
// Output is sorted by (reason ASC, anime_id ASC) for deterministic test
// snapshots and stable rendering. Phase 14 (REC-ADMIN-01).
func (f *S11Filter) FilterAudit(ctx context.Context, userID recs.UserID) ([]FilteredOutEntry, error) {
	type row struct {
		AnimeID string
		Reason  string
	}
	var rows []row
	const q = `
		SELECT a.id AS anime_id, 'hidden=true' AS reason
		FROM animes a
		WHERE a.hidden = TRUE AND a.deleted_at IS NULL
		UNION ALL
		SELECT al.anime_id, 'status=completed' AS reason
		FROM anime_list al
		JOIN animes a ON a.id = al.anime_id
		WHERE al.user_id = ? AND al.status = 'completed' AND a.deleted_at IS NULL
		UNION ALL
		SELECT al.anime_id, 'status=dropped' AS reason
		FROM anime_list al
		JOIN animes a ON a.id = al.anime_id
		WHERE al.user_id = ? AND al.status = 'dropped' AND a.deleted_at IS NULL
		ORDER BY reason ASC, anime_id ASC
	`
	if err := f.db.WithContext(ctx).Raw(q, string(userID), string(userID)).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]FilteredOutEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, FilteredOutEntry{AnimeID: r.AnimeID, Reason: r.Reason})
	}
	return out, nil
}
