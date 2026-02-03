package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/repo"
	"gorm.io/gorm"
)

// ExportService handles export job creation and management
type ExportService struct {
	exportJobRepo *repo.ExportJobRepository
	taskRepo      *repo.TaskRepository
	log           *logger.Logger
}

// NewExportService creates a new export service
func NewExportService(
	exportJobRepo *repo.ExportJobRepository,
	taskRepo *repo.TaskRepository,
	log *logger.Logger,
) *ExportService {
	return &ExportService{
		exportJobRepo: exportJobRepo,
		taskRepo:      taskRepo,
		log:           log,
	}
}

// CreateExportJob creates a new export job
func (s *ExportService) CreateExportJob(ctx context.Context, req *domain.CreateExportJobRequest) (*domain.ExportJob, error) {
	// Check for existing active export
	existing, err := s.exportJobRepo.GetActiveByUserID(ctx, req.UserID)
	if err == nil && existing != nil {
		return existing, nil // Return existing active export
	}

	job := &domain.ExportJob{
		UserID:      req.UserID,
		MALUsername: req.MALUsername,
		Status:      domain.ExportStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.exportJobRepo.Create(ctx, job); err != nil {
		return nil, err
	}

	s.log.Infow("created export job",
		"job_id", job.ID,
		"user_id", req.UserID,
		"mal_username", req.MALUsername,
	)

	return job, nil
}

// CreateTasks creates anime load tasks for an export job
func (s *ExportService) CreateTasks(ctx context.Context, req *domain.CreateTasksRequest) error {
	tasks := make([]*domain.AnimeLoadTask, 0, len(req.Tasks))
	now := time.Now()

	for _, input := range req.Tasks {
		task := &domain.AnimeLoadTask{
			ExportJobID:      req.ExportJobID,
			UserID:           req.UserID,
			MALID:            input.MALID,
			MALTitle:         input.Title,
			MALTitleJapanese: input.TitleJapanese,
			MALTitleEnglish:  input.TitleEnglish,
			Status:           domain.TaskStatusPending,
			Priority:         req.Priority,
			MaxAttempts:      3,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		tasks = append(tasks, task)
	}

	if err := s.taskRepo.CreateBatch(ctx, tasks); err != nil {
		return err
	}

	// Update export job total count
	if err := s.exportJobRepo.SetTotalAnime(ctx, req.ExportJobID, len(req.Tasks)); err != nil {
		return err
	}

	// Mark job as processing
	if err := s.exportJobRepo.UpdateStatus(ctx, req.ExportJobID, domain.ExportStatusProcessing); err != nil {
		return err
	}

	s.log.Infow("created anime load tasks",
		"export_job_id", req.ExportJobID,
		"task_count", len(tasks),
	)

	return nil
}

// GetExportJob retrieves an export job by ID
func (s *ExportService) GetExportJob(ctx context.Context, id string) (*domain.ExportJob, error) {
	return s.exportJobRepo.GetByID(ctx, id)
}

// GetUserExports retrieves all export jobs for a user
func (s *ExportService) GetUserExports(ctx context.Context, userID string) ([]*domain.ExportJob, error) {
	return s.exportJobRepo.GetByUserID(ctx, userID)
}

// CancelExport cancels an active export job
func (s *ExportService) CancelExport(ctx context.Context, userID, exportID string) error {
	job, err := s.exportJobRepo.GetByID(ctx, exportID)
	if err != nil {
		return err
	}

	// Verify ownership
	if job.UserID != userID {
		return gorm.ErrRecordNotFound
	}

	// Only cancel active exports
	if !job.IsActive() {
		return nil
	}

	if err := s.exportJobRepo.UpdateStatus(ctx, exportID, domain.ExportStatusCancelled); err != nil {
		return err
	}

	s.log.Infow("cancelled export job",
		"job_id", exportID,
		"user_id", userID,
	)

	return nil
}

// GetTaskStats retrieves task statistics for an export job
func (s *ExportService) GetTaskStats(ctx context.Context, exportJobID string) (*domain.TaskStats, error) {
	return s.taskRepo.GetStats(ctx, exportJobID)
}

// DeletePendingTask removes a task from the queue (for "Load Now" feature)
func (s *ExportService) DeletePendingTask(ctx context.Context, malID int) error {
	return s.taskRepo.DeleteByMALID(ctx, malID)
}
