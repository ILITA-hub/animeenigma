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
//   - CandidatePoolForUser(ctx, uid)  — logged-in. Anonymous filter plus
//     exclusion of any anime present in the user's anime_list, regardless of
//     status. Recs are for things the user has NOT already added to their list;
//     the ranking signals (S1/S2/S5) still read anime_list independently to
//     compute affinity scores, so the user's history continues to shape the
//     ordering — it just isn't recommended back at them.
//
// FilterAudit (admin debug page) returns the items the per-user filter would
// exclude with a reason string per category.

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
// any anime the caller has in their anime_list (any status — watching,
// planned, on_hold, completed, dropped). Recs are for things the user has
// NOT already added to their list; the ranking signals (S1/S2/S5) still
// read anime_list independently to compute affinity, so the user's history
// continues to shape the ordering — it just isn't recommended back at them.
//
// Only anime with no anime_list row at all (al.status IS NULL on the LEFT
// JOIN) survive.
func (f *S11Filter) CandidatePoolForUser(ctx context.Context, userID recs.UserID) ([]recs.AnimeID, error) {
	var ids []recs.AnimeID
	err := f.db.WithContext(ctx).
		Table("animes AS a").
		Select("a.id").
		Joins("LEFT JOIN anime_list al ON al.anime_id = a.id AND al.user_id = ?", userID).
		Where("a.hidden = ?", false).
		Where("a.deleted_at IS NULL").
		Where("al.status IS NULL").
		Pluck("a.id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// FilteredOutEntry is one row in the admin-debug filter audit panel.
// Reason is one of {"hidden=true", "status=<watching|planned|on_hold|completed|dropped>"}.
type FilteredOutEntry struct {
	AnimeID string `json:"anime_id"`
	Reason  string `json:"reason"`
}

// FilterAudit returns every anime the per-user S11 filter would exclude,
// with a reason string per applicable category. An anime that triggers
// multiple categories (e.g. hidden=true AND in the user's list) emits
// multiple rows — admins see every reason that applied.
//
// Any anime_list row excludes the anime now (recs are "things not yet in
// your list"), so the audit emits one status=<value> row per anime_list
// entry regardless of status.
//
// Soft-deleted anime (deleted_at IS NOT NULL) are EXCLUDED from the audit
// — they're never surfaced anywhere in the admin debug surface.
//
// Output is sorted by (reason ASC, anime_id ASC) for deterministic test
// snapshots and stable rendering.
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
		SELECT al.anime_id, 'status=' || al.status AS reason
		FROM anime_list al
		JOIN animes a ON a.id = al.anime_id
		WHERE al.user_id = ? AND a.deleted_at IS NULL
		ORDER BY reason ASC, anime_id ASC
	`
	if err := f.db.WithContext(ctx).Raw(q, string(userID)).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]FilteredOutEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, FilteredOutEntry{AnimeID: r.AnimeID, Reason: r.Reason})
	}
	return out, nil
}
