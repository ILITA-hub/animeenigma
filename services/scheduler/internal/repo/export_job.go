package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"gorm.io/gorm"
)

// ExportJobRepository handles database operations for export jobs
type ExportJobRepository struct {
	db *gorm.DB
}

// NewExportJobRepository creates a new export job repository
func NewExportJobRepository(db *gorm.DB) *ExportJobRepository {
	return &ExportJobRepository{db: db}
}

// Create creates a new export job
func (r *ExportJobRepository) Create(ctx context.Context, job *domain.ExportJob) error {
	return r.db.WithContext(ctx).Create(job).Error
}

// GetByID retrieves an export job by ID
func (r *ExportJobRepository) GetByID(ctx context.Context, id string) (*domain.ExportJob, error) {
	var job domain.ExportJob
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// GetByUserID retrieves all export jobs for a user
func (r *ExportJobRepository) GetByUserID(ctx context.Context, userID string) ([]*domain.ExportJob, error) {
	var jobs []*domain.ExportJob
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&jobs).Error; err != nil {
		return nil, err
	}
	return jobs, nil
}

// GetActiveByUserID retrieves active export jobs for a user
func (r *ExportJobRepository) GetActiveByUserID(ctx context.Context, userID string) (*domain.ExportJob, error) {
	var job domain.ExportJob
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND status IN ?", userID, []domain.ExportJobStatus{
			domain.ExportStatusPending,
			domain.ExportStatusProcessing,
		}).
		Order("created_at DESC").
		First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// Update updates an export job
func (r *ExportJobRepository) Update(ctx context.Context, job *domain.ExportJob) error {
	job.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(job).Error
}

// UpdateStatus updates the status of an export job
func (r *ExportJobRepository) UpdateStatus(ctx context.Context, id string, status domain.ExportJobStatus) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}

	if status == domain.ExportStatusProcessing {
		now := time.Now()
		updates["started_at"] = now
	} else if status == domain.ExportStatusCompleted || status == domain.ExportStatusFailed || status == domain.ExportStatusCancelled {
		now := time.Now()
		updates["completed_at"] = now
	}

	return r.db.WithContext(ctx).
		Model(&domain.ExportJob{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// IncrementCounters atomically increments the counters for an export job
func (r *ExportJobRepository) IncrementCounters(ctx context.Context, id string, loaded, skipped, failed int) error {
	return r.db.WithContext(ctx).
		Model(&domain.ExportJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"processed_anime": gorm.Expr("processed_anime + ?", loaded+skipped+failed),
			"loaded_anime":    gorm.Expr("loaded_anime + ?", loaded),
			"skipped_anime":   gorm.Expr("skipped_anime + ?", skipped),
			"failed_anime":    gorm.Expr("failed_anime + ?", failed),
			"updated_at":      time.Now(),
		}).Error
}

// SetTotalAnime sets the total anime count for an export job
func (r *ExportJobRepository) SetTotalAnime(ctx context.Context, id string, total int) error {
	return r.db.WithContext(ctx).
		Model(&domain.ExportJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"total_anime": total,
			"updated_at":  time.Now(),
		}).Error
}

// SetError sets the error message for an export job
func (r *ExportJobRepository) SetError(ctx context.Context, id string, errMsg string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.ExportJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        domain.ExportStatusFailed,
			"error_message": errMsg,
			"completed_at":  now,
			"updated_at":    now,
		}).Error
}

// Delete deletes an export job
func (r *ExportJobRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&domain.ExportJob{}, "id = ?", id).Error
}

// DeleteByUserID deletes all export jobs for a user
func (r *ExportJobRepository) DeleteByUserID(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Delete(&domain.ExportJob{}, "user_id = ?", userID).Error
}
