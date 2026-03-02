package repo

import (
	"context"
	"errors"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
)

type SyncRepository struct {
	db *gorm.DB
}

func NewSyncRepository(db *gorm.DB) *SyncRepository {
	return &SyncRepository{db: db}
}

func (r *SyncRepository) Create(ctx context.Context, job *domain.SyncJob) error {
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *SyncRepository) GetByID(ctx context.Context, id string) (*domain.SyncJob, error) {
	var job domain.SyncJob
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}

func (r *SyncRepository) GetActiveByUserAndSource(ctx context.Context, userID, source string) (*domain.SyncJob, error) {
	var job domain.SyncJob
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND source = ? AND status = 'processing'", userID, source).
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}

func (r *SyncRepository) GetLatestByUserAndSource(ctx context.Context, userID, source string) (*domain.SyncJob, error) {
	var job domain.SyncJob
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND source = ? AND status IN ('completed','failed')", userID, source).
		Order("completed_at DESC").
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}

func (r *SyncRepository) UpdateProgress(ctx context.Context, id string, imported, skipped int) error {
	return r.db.WithContext(ctx).
		Model(&domain.SyncJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"imported":   imported,
			"skipped":    skipped,
			"updated_at": time.Now(),
		}).Error
}

func (r *SyncRepository) Complete(ctx context.Context, id, status, errorMsg string, imported, skipped int) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.SyncJob{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        status,
			"error_message": errorMsg,
			"imported":      imported,
			"skipped":       skipped,
			"completed_at":  now,
			"updated_at":    now,
		}).Error
}

func (r *SyncRepository) MarkStaleJobsFailed(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	return r.db.WithContext(ctx).
		Model(&domain.SyncJob{}).
		Where("status = 'processing' AND started_at < ?", cutoff).
		Updates(map[string]interface{}{
			"status":        "failed",
			"error_message": "stale job cleaned up on startup",
			"completed_at":  time.Now(),
			"updated_at":    time.Now(),
		}).Error
}
