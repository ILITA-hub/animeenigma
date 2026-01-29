package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/jobs"
	"github.com/robfig/cron/v3"
)

type JobService struct {
	cron             *cron.Cron
	shikimoriJob     *jobs.ShikimoriSyncJob
	cleanupJob       *jobs.CleanupJob
	log              *logger.Logger
	lastShikimoriRun time.Time
	lastCleanupRun   time.Time
}

func NewJobService(
	shikimoriJob *jobs.ShikimoriSyncJob,
	cleanupJob *jobs.CleanupJob,
	log *logger.Logger,
) *JobService {
	return &JobService{
		cron:         cron.New(),
		shikimoriJob: shikimoriJob,
		cleanupJob:   cleanupJob,
		log:          log,
	}
}

// Start starts the job scheduler
func (s *JobService) Start(shikimoriCron, cleanupCron string) error {
	// Schedule Shikimori sync job
	_, err := s.cron.AddFunc(shikimoriCron, func() {
		ctx := context.Background()
		s.log.Info("starting scheduled Shikimori sync")
		if err := s.shikimoriJob.Run(ctx); err != nil {
			s.log.Errorw("Shikimori sync failed", "error", err)
		} else {
			s.lastShikimoriRun = time.Now()
			s.log.Info("Shikimori sync completed successfully")
		}
	})
	if err != nil {
		return err
	}

	// Schedule cleanup job
	_, err = s.cron.AddFunc(cleanupCron, func() {
		ctx := context.Background()
		s.log.Info("starting scheduled cleanup")
		if err := s.cleanupJob.Run(ctx); err != nil {
			s.log.Errorw("cleanup failed", "error", err)
		} else {
			s.lastCleanupRun = time.Now()
			s.log.Info("cleanup completed successfully")
		}
	})
	if err != nil {
		return err
	}

	s.cron.Start()
	s.log.Info("job scheduler started")
	return nil
}

// Stop stops the job scheduler
func (s *JobService) Stop() {
	s.cron.Stop()
	s.log.Info("job scheduler stopped")
}

// TriggerShikimoriSync manually triggers the Shikimori sync job
func (s *JobService) TriggerShikimoriSync(ctx context.Context) {
	s.log.Info("manually triggering Shikimori sync")
	if err := s.shikimoriJob.Run(ctx); err != nil {
		s.log.Errorw("Shikimori sync failed", "error", err)
	} else {
		s.lastShikimoriRun = time.Now()
		s.log.Info("Shikimori sync completed successfully")
	}
}

// TriggerCleanup manually triggers the cleanup job
func (s *JobService) TriggerCleanup(ctx context.Context) {
	s.log.Info("manually triggering cleanup")
	if err := s.cleanupJob.Run(ctx); err != nil {
		s.log.Errorw("cleanup failed", "error", err)
	} else {
		s.lastCleanupRun = time.Now()
		s.log.Info("cleanup completed successfully")
	}
}

// GetStatus returns the status of all jobs
func (s *JobService) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"scheduler_running": s.cron != nil,
		"shikimori_sync": map[string]interface{}{
			"last_run": s.lastShikimoriRun,
		},
		"cleanup": map[string]interface{}{
			"last_run": s.lastCleanupRun,
		},
	}
}
