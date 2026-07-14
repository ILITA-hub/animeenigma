package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// JobSuccessRepository persists per-job last-success timestamps.
type JobSuccessRepository struct {
	db *gorm.DB
}

// NewJobSuccessRepository creates a new job-success repository.
func NewJobSuccessRepository(db *gorm.DB) *JobSuccessRepository {
	return &JobSuccessRepository{db: db}
}

// Upsert records the latest successful run for a job.
func (r *JobSuccessRepository) Upsert(ctx context.Context, job string, at time.Time) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "job"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_success_at"}),
		}).
		Create(&domain.JobSuccess{Job: job, LastSuccessAt: at}).Error
}

// All returns every persisted job-success row.
func (r *JobSuccessRepository) All(ctx context.Context) ([]domain.JobSuccess, error) {
	var rows []domain.JobSuccess
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
