package repo

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/services/recs/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecsRepository provides access to rec_user_signals and rec_population_signals.
// Phase 9 ships only the methods the orchestrator and population/user precompute
// jobs need. Phase 13 (S6) adds rec_completion_co_occurrence access.
type RecsRepository struct {
	db *gorm.DB
}

func NewRecsRepository(db *gorm.DB) *RecsRepository {
	return &RecsRepository{db: db}
}

// GetUserSignals returns the row for a user, or (nil, nil) if no row exists.
// Callers treat a nil result as "no precomputed signals yet" — that's the
// normal cold-start state for new users until the first precompute pass.
func (r *RecsRepository) GetUserSignals(ctx context.Context, userID string) (*domain.RecUserSignals, error) {
	var row domain.RecUserSignals
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// UpsertUserSignals inserts or updates the row for a user, keyed by user_id.
// Caller is responsible for setting LastComputed before the call.
func (r *RecsRepository) UpsertUserSignals(ctx context.Context, row *domain.RecUserSignals) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"s1_vector", "s5_affinity",
				"s6_seed_anime_id", "s6_seed_completed_at", "s6_seed_score",
				"last_computed",
			}),
		}).
		Create(row).Error
}

// UpdateS6Seed performs a NARROW upsert that touches ONLY the four S6
// columns + last_computed — s1_vector and s5_affinity are deliberately
// omitted from the DoUpdates list so the synchronous Phase-13 seed-update
// path inside MarkEpisodeWatched cannot race with the user-orchestrator
// cron that writes those JSONB columns.
//
// Phase 13 (REC-INFRA-03). Spec ref: docs/superpowers/specs/2026-05-03-rec-engine-design.md §5.
//
// Postgres semantics: ON CONFLICT (user_id) DO UPDATE only assigns the listed
// columns; pre-existing s1_vector / s5_affinity values are preserved verbatim.
// On a fresh insert the column DEFAULTs ('{}') fill them in.
func (r *RecsRepository) UpdateS6Seed(ctx context.Context, userID, animeID string, completedAt time.Time, score int) error {
	row := &domain.RecUserSignals{
		UserID:            userID,
		S6SeedAnimeID:     &animeID,
		S6SeedCompletedAt: &completedAt,
		S6SeedScore:       &score,
		LastComputed:      time.Now().UTC(),
		// S1Vector + S5Affinity intentionally left as zero-value "" — the
		// Phase 9 column DEFAULT '{}' fills them on INSERT. The DoUpdates
		// list below intentionally OMITS these two columns so existing
		// values are preserved on UPDATE.
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"s6_seed_anime_id", "s6_seed_completed_at", "s6_seed_score", "last_computed",
			}),
		}).
		Create(row).Error
}

// GetTopCoOccurrences returns candidate anime IDs sorted by descending
// co-count for the given seed. Phase 13 (REC-SIG-06).
//
// At scoreThreshold == 7, reads from the materialized
// rec_completion_co_occurrence table (populated nightly by the new
// CoOccurrenceOrchestrator). At scoreThreshold == 5 (or any other value),
// runs an on-demand JOIN against anime_list — the score=5 fallback is rare
// enough that materializing it would waste storage; per spec §3.2 we never
// fall to score > 0 so the only other expected value is 5.
//
// Returns an empty slice (not an error) when the seed has no matching
// candidates.
func (r *RecsRepository) GetTopCoOccurrences(ctx context.Context, seedAnimeID string, scoreThreshold, limit int) ([]string, error) {
	if scoreThreshold == 7 {
		var ids []string
		err := r.db.WithContext(ctx).
			Table("rec_completion_co_occurrence").
			Where("seed_anime_id = ?", seedAnimeID).
			Order("co_count DESC").
			Limit(limit).
			Pluck("candidate_anime_id", &ids).Error
		if err != nil {
			return nil, err
		}
		return ids, nil
	}
	// On-demand live query for the score=5 fallback path.
	var ids []string
	err := r.db.WithContext(ctx).
		Raw(`
			SELECT b.anime_id AS candidate_anime_id
			FROM anime_list a
			JOIN anime_list b ON a.user_id = b.user_id AND a.anime_id <> b.anime_id
			WHERE a.anime_id = ?
			  AND a.status = 'completed' AND a.score >= ?
			  AND b.status = 'completed' AND b.score >= ?
			GROUP BY b.anime_id
			ORDER BY COUNT(DISTINCT a.user_id) DESC
			LIMIT ?
		`, seedAnimeID, scoreThreshold, scoreThreshold, limit).
		Pluck("candidate_anime_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// ListPopulationSignals returns every row in rec_population_signals.
// Population is small (~few thousand rows) — full-scan is acceptable.
func (r *RecsRepository) ListPopulationSignals(ctx context.Context) ([]domain.RecPopulationSignals, error) {
	var rows []domain.RecPopulationSignals
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpsertPopulationSignal upserts a single anime_id row.
func (r *RecsRepository) UpsertPopulationSignal(ctx context.Context, row *domain.RecPopulationSignals) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "anime_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"s3_trending_score", "s4_recency_score", "last_computed",
			}),
		}).
		Create(row).Error
}
