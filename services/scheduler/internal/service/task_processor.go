package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/repo"
)

// TaskProcessor handles processing of anime load tasks
type TaskProcessor struct {
	taskRepo      *repo.TaskRepository
	exportJobRepo *repo.ExportJobRepository
	resolver      *MALResolver
	log           *logger.Logger
}

// NewTaskProcessor creates a new task processor
func NewTaskProcessor(
	taskRepo *repo.TaskRepository,
	exportJobRepo *repo.ExportJobRepository,
	resolver *MALResolver,
	log *logger.Logger,
) *TaskProcessor {
	return &TaskProcessor{
		taskRepo:      taskRepo,
		exportJobRepo: exportJobRepo,
		resolver:      resolver,
		log:           log,
	}
}

// ProcessTask processes a single anime load task
func (p *TaskProcessor) ProcessTask(ctx context.Context, task *domain.AnimeLoadTask) error {
	p.log.Infow("processing task",
		"task_id", task.ID,
		"mal_id", task.MALID,
		"mal_title", task.MALTitle,
		"attempt", task.AttemptCount,
	)

	// Resolve MAL ID to Shikimori ID
	result := p.resolver.Resolve(ctx, task)

	if result.Error != nil {
		return p.handleError(ctx, task, result.Error)
	}

	switch result.Method {
	case domain.ResolutionCached:
		// Already exists in database
		if result.AnimeID != "" {
			return p.markSkipped(ctx, task, result.AnimeID)
		}
		// Have Shikimori ID but no local anime - load it
		return p.loadAnime(ctx, task, result.ShikimoriID)

	case domain.ResolutionExactJapanese, domain.ResolutionExactRomanized:
		// Found exact match on Shikimori - load it
		return p.loadAnime(ctx, task, result.ShikimoriID)

	case domain.ResolutionNotFound:
		// No match found - mark for manual resolution
		return p.markForManualResolution(ctx, task)

	default:
		return p.handleError(ctx, task, nil)
	}
}

// loadAnime loads an anime from Shikimori and marks the task as completed
func (p *TaskProcessor) loadAnime(ctx context.Context, task *domain.AnimeLoadTask, shikimoriID string) error {
	animeID, err := p.resolver.LoadAnimeFromShikimori(ctx, shikimoriID, task.MALID)
	if err != nil {
		return p.handleError(ctx, task, err)
	}

	p.log.Infow("loaded anime from Shikimori",
		"task_id", task.ID,
		"mal_id", task.MALID,
		"shikimori_id", shikimoriID,
		"anime_id", animeID,
	)

	// Mark task as completed
	if err := p.taskRepo.MarkCompleted(ctx, task.ID, shikimoriID, animeID, domain.ResolutionExactRomanized); err != nil {
		return err
	}

	// Update export job counters
	if task.ExportJobID != "" {
		return p.exportJobRepo.IncrementCounters(ctx, task.ExportJobID, 1, 0, 0)
	}

	return nil
}

// markSkipped marks a task as skipped (anime already exists)
func (p *TaskProcessor) markSkipped(ctx context.Context, task *domain.AnimeLoadTask, animeID string) error {
	p.log.Infow("anime already exists, skipping",
		"task_id", task.ID,
		"mal_id", task.MALID,
		"anime_id", animeID,
	)

	if err := p.taskRepo.MarkSkipped(ctx, task.ID, animeID); err != nil {
		return err
	}

	// Update export job counters
	if task.ExportJobID != "" {
		return p.exportJobRepo.IncrementCounters(ctx, task.ExportJobID, 0, 1, 0)
	}

	return nil
}

// markForManualResolution marks a task for manual user resolution
func (p *TaskProcessor) markForManualResolution(ctx context.Context, task *domain.AnimeLoadTask) error {
	p.log.Infow("marking for manual resolution",
		"task_id", task.ID,
		"mal_id", task.MALID,
		"mal_title", task.MALTitle,
	)

	if err := p.taskRepo.MarkForManualResolution(ctx, task.ID); err != nil {
		return err
	}

	// Update export job counters - count as skipped for now
	if task.ExportJobID != "" {
		return p.exportJobRepo.IncrementCounters(ctx, task.ExportJobID, 0, 1, 0)
	}

	return nil
}

// handleError handles task errors with retry logic
func (p *TaskProcessor) handleError(ctx context.Context, task *domain.AnimeLoadTask, err error) error {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}

	p.log.Warnw("task processing failed",
		"task_id", task.ID,
		"mal_id", task.MALID,
		"attempt", task.AttemptCount,
		"error", errMsg,
	)

	// Calculate retry backoff: 30s, 2m, 8m
	var retryAfter *time.Time
	if task.AttemptCount < task.MaxAttempts {
		backoff := calculateBackoff(task.AttemptCount)
		retryTime := time.Now().Add(backoff)
		retryAfter = &retryTime

		p.log.Infow("scheduling retry",
			"task_id", task.ID,
			"attempt", task.AttemptCount,
			"retry_after", backoff,
		)
	}

	if err := p.taskRepo.MarkFailed(ctx, task.ID, errMsg, retryAfter); err != nil {
		return err
	}

	// If max attempts reached, update export job failed counter
	if task.AttemptCount >= task.MaxAttempts && task.ExportJobID != "" {
		return p.exportJobRepo.IncrementCounters(ctx, task.ExportJobID, 0, 0, 1)
	}

	return nil
}

// calculateBackoff calculates exponential backoff duration
// Attempts: 1 -> 30s, 2 -> 2m, 3 -> 8m
func calculateBackoff(attempt int) time.Duration {
	baseDelay := 30 * time.Second
	multiplier := 1 << uint(attempt) // 2^attempt
	return baseDelay * time.Duration(multiplier)
}

// CheckExportJobCompletion checks if an export job is complete and updates its status
func (p *TaskProcessor) CheckExportJobCompletion(ctx context.Context, exportJobID string) error {
	stats, err := p.taskRepo.GetStats(ctx, exportJobID)
	if err != nil {
		return err
	}

	// If all tasks are processed, mark job as completed
	if stats.Pending == 0 && stats.Processing == 0 {
		return p.exportJobRepo.UpdateStatus(ctx, exportJobID, domain.ExportStatusCompleted)
	}

	return nil
}
