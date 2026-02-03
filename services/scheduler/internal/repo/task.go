package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TaskRepository handles database operations for anime load tasks
type TaskRepository struct {
	db *gorm.DB
}

// NewTaskRepository creates a new task repository
func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// Create creates a new task
func (r *TaskRepository) Create(ctx context.Context, task *domain.AnimeLoadTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// CreateBatch creates multiple tasks in a batch
func (r *TaskRepository) CreateBatch(ctx context.Context, tasks []*domain.AnimeLoadTask) error {
	if len(tasks) == 0 {
		return nil
	}
	// Use ON CONFLICT DO NOTHING for deduplication
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(tasks, 100).Error
}

// GetByID retrieves a task by ID
func (r *TaskRepository) GetByID(ctx context.Context, id string) (*domain.AnimeLoadTask, error) {
	var task domain.AnimeLoadTask
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// GetByMALID retrieves a task by MAL ID
func (r *TaskRepository) GetByMALID(ctx context.Context, malID int) (*domain.AnimeLoadTask, error) {
	var task domain.AnimeLoadTask
	if err := r.db.WithContext(ctx).
		Where("mal_id = ? AND status IN ?", malID, []domain.TaskStatus{
			domain.TaskStatusPending,
			domain.TaskStatusProcessing,
		}).
		First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// GetByExportJobID retrieves all tasks for an export job
func (r *TaskRepository) GetByExportJobID(ctx context.Context, exportJobID string) ([]*domain.AnimeLoadTask, error) {
	var tasks []*domain.AnimeLoadTask
	if err := r.db.WithContext(ctx).
		Where("export_job_id = ?", exportJobID).
		Order("created_at ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// GetNextPending retrieves the next pending task for processing
// Uses FOR UPDATE SKIP LOCKED for concurrent worker safety
// Implements fair round-robin scheduling by user
func (r *TaskRepository) GetNextPending(ctx context.Context) (*domain.AnimeLoadTask, error) {
	var task domain.AnimeLoadTask

	// Use raw SQL for FOR UPDATE SKIP LOCKED with fair scheduling
	// Priority ordering (higher first), then round-robin by user based on least recent processing
	err := r.db.WithContext(ctx).Raw(`
		SELECT * FROM anime_load_tasks
		WHERE status = 'pending'
		  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		ORDER BY priority DESC, updated_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(&task).Error

	if err != nil {
		return nil, err
	}

	if task.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}

	return &task, nil
}

// ClaimTask atomically claims a task for processing
func (r *TaskRepository) ClaimTask(ctx context.Context, taskID string) error {
	return r.db.WithContext(ctx).
		Model(&domain.AnimeLoadTask{}).
		Where("id = ? AND status = ?", taskID, domain.TaskStatusPending).
		Updates(map[string]interface{}{
			"status":        domain.TaskStatusProcessing,
			"attempt_count": gorm.Expr("attempt_count + 1"),
			"updated_at":    time.Now(),
		}).Error
}

// Update updates a task
func (r *TaskRepository) Update(ctx context.Context, task *domain.AnimeLoadTask) error {
	task.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(task).Error
}

// UpdateStatus updates the status of a task
func (r *TaskRepository) UpdateStatus(ctx context.Context, id string, status domain.TaskStatus) error {
	return r.db.WithContext(ctx).
		Model(&domain.AnimeLoadTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

// MarkCompleted marks a task as completed with resolution details
func (r *TaskRepository) MarkCompleted(ctx context.Context, id, shikimoriID, animeID string, method domain.ResolutionMethod) error {
	return r.db.WithContext(ctx).
		Model(&domain.AnimeLoadTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":                domain.TaskStatusCompleted,
			"resolved_shikimori_id": shikimoriID,
			"resolved_anime_id":     animeID,
			"resolution_method":     method,
			"updated_at":            time.Now(),
		}).Error
}

// MarkFailed marks a task as failed with error and retry scheduling
func (r *TaskRepository) MarkFailed(ctx context.Context, id, errMsg string, retryAfter *time.Time) error {
	updates := map[string]interface{}{
		"last_error": errMsg,
		"updated_at": time.Now(),
	}

	// Check if we should retry
	var task domain.AnimeLoadTask
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&task).Error; err != nil {
		return err
	}

	if task.AttemptCount >= task.MaxAttempts {
		updates["status"] = domain.TaskStatusFailed
	} else {
		updates["status"] = domain.TaskStatusPending
		if retryAfter != nil {
			updates["next_retry_at"] = *retryAfter
		}
	}

	return r.db.WithContext(ctx).
		Model(&domain.AnimeLoadTask{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// MarkSkipped marks a task as skipped (e.g., already exists)
func (r *TaskRepository) MarkSkipped(ctx context.Context, id, animeID string) error {
	return r.db.WithContext(ctx).
		Model(&domain.AnimeLoadTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":            domain.TaskStatusSkipped,
			"resolved_anime_id": animeID,
			"updated_at":        time.Now(),
		}).Error
}

// MarkForManualResolution marks a task for manual user resolution
func (r *TaskRepository) MarkForManualResolution(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&domain.AnimeLoadTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     domain.TaskStatusManual,
			"updated_at": time.Now(),
		}).Error
}

// Delete deletes a task
func (r *TaskRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&domain.AnimeLoadTask{}, "id = ?", id).Error
}

// DeleteByMALID deletes a task by MAL ID
func (r *TaskRepository) DeleteByMALID(ctx context.Context, malID int) error {
	return r.db.WithContext(ctx).
		Delete(&domain.AnimeLoadTask{}, "mal_id = ? AND status IN ?", malID, []domain.TaskStatus{
			domain.TaskStatusPending,
			domain.TaskStatusProcessing,
		}).Error
}

// GetStats retrieves task statistics for an export job
func (r *TaskRepository) GetStats(ctx context.Context, exportJobID string) (*domain.TaskStats, error) {
	var stats domain.TaskStats

	// Count total
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&domain.AnimeLoadTask{}).
		Where("export_job_id = ?", exportJobID).
		Count(&total).Error; err != nil {
		return nil, err
	}
	stats.Total = int(total)

	// Count by status
	rows, err := r.db.WithContext(ctx).
		Model(&domain.AnimeLoadTask{}).
		Select("status, count(*) as count").
		Where("export_job_id = ?", exportJobID).
		Group("status").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		switch domain.TaskStatus(status) {
		case domain.TaskStatusPending:
			stats.Pending = count
		case domain.TaskStatusProcessing:
			stats.Processing = count
		case domain.TaskStatusCompleted:
			stats.Completed = count
		case domain.TaskStatusFailed:
			stats.Failed = count
		case domain.TaskStatusSkipped:
			stats.Skipped = count
		}
	}

	return &stats, nil
}

// GetPendingCount returns the number of pending tasks
func (r *TaskRepository) GetPendingCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.AnimeLoadTask{}).
		Where("status = ?", domain.TaskStatusPending).
		Count(&count).Error
	return count, err
}

// ResetStuckTasks resets tasks that are stuck in processing state
// Used on service startup to recover from crashes
func (r *TaskRepository) ResetStuckTasks(ctx context.Context, stuckDuration time.Duration) (int64, error) {
	cutoff := time.Now().Add(-stuckDuration)
	result := r.db.WithContext(ctx).
		Model(&domain.AnimeLoadTask{}).
		Where("status = ? AND updated_at < ?", domain.TaskStatusProcessing, cutoff).
		Updates(map[string]interface{}{
			"status":     domain.TaskStatusPending,
			"updated_at": time.Now(),
		})
	return result.RowsAffected, result.Error
}
