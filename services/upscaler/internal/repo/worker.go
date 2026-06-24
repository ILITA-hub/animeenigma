package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkerRepository provides access to the upscale_workers table.
type WorkerRepository struct {
	db *gorm.DB
}

// NewWorkerRepository constructs a WorkerRepository backed by db.
func NewWorkerRepository(db *gorm.DB) *WorkerRepository {
	return &WorkerRepository{db: db}
}

// Upsert inserts or updates a worker record. On conflict on worker_id, all
// mutable fields are refreshed.
func (r *WorkerRepository) Upsert(ctx context.Context, w *domain.UpscaleWorker) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "worker_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"gpu_info", "image_version", "models_available",
			"status", "current_job_id", "current_segment",
			"session_expires_at", "last_heartbeat_at",
		}),
	}).Create(w).Error
}

// Heartbeat updates the worker's current assignment and heartbeat timestamp.
func (r *WorkerRepository) Heartbeat(ctx context.Context, workerID, jobID string, seg int, now time.Time) error {
	return r.db.WithContext(ctx).
		Model(&domain.UpscaleWorker{}).
		Where("worker_id = ?", workerID).
		Updates(map[string]interface{}{
			"current_job_id":    jobID,
			"current_segment":   seg,
			"last_heartbeat_at": now,
		}).Error
}

// MarkGone sets a worker's status to 'gone'.
func (r *WorkerRepository) MarkGone(ctx context.Context, workerID string) error {
	return r.db.WithContext(ctx).
		Model(&domain.UpscaleWorker{}).
		Where("worker_id = ?", workerID).
		Update("status", "gone").Error
}

// FindByJob returns the non-gone workers whose current_job_id matches jobID.
// Used by the admin cancel path to deliver an in-flight cancel command to the
// worker(s) actively processing a job. Returns an empty slice (not an error)
// when no worker is currently bound to the job.
func (r *WorkerRepository) FindByJob(ctx context.Context, jobID string) ([]domain.UpscaleWorker, error) {
	var workers []domain.UpscaleWorker
	if err := r.db.WithContext(ctx).
		Where("current_job_id = ? AND status != ?", jobID, "gone").
		Order("worker_id ASC").
		Find(&workers).Error; err != nil {
		return nil, err
	}
	return workers, nil
}

// ListConnected returns workers whose last_heartbeat_at is at or after since
// and whose status is not 'gone'.
func (r *WorkerRepository) ListConnected(ctx context.Context, since time.Time) ([]domain.UpscaleWorker, error) {
	var workers []domain.UpscaleWorker
	if err := r.db.WithContext(ctx).
		Where("last_heartbeat_at >= ? AND status != ?", since, "gone").
		Order("worker_id ASC").
		Find(&workers).Error; err != nil {
		return nil, err
	}
	return workers, nil
}
