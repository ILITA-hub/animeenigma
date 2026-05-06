package repo

import (
	"context"
	"errors"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
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
