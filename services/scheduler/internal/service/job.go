package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/jobs"
	"github.com/robfig/cron/v3"
)

type JobService struct {
	cron              *cron.Cron
	shikimoriJob      *jobs.ShikimoriSyncJob
	cleanupJob        *jobs.CleanupJob
	topAnimeJob       *jobs.TopAnimeSyncJob
	calendarJob       *jobs.CalendarSyncJob
	log               *logger.Logger
	lastShikimoriRun  time.Time
	lastCleanupRun    time.Time
	lastTopAnimeRun   time.Time
	lastCalendarRun   time.Time
}

func NewJobService(
	shikimoriJob *jobs.ShikimoriSyncJob,
	cleanupJob *jobs.CleanupJob,
	topAnimeJob *jobs.TopAnimeSyncJob,
	calendarJob *jobs.CalendarSyncJob,
	log *logger.Logger,
) *JobService {
	return &JobService{
		cron:         cron.New(),
		shikimoriJob: shikimoriJob,
		cleanupJob:   cleanupJob,
		topAnimeJob:  topAnimeJob,
		calendarJob:  calendarJob,
		log:          log,
	}
}

// Start starts the job scheduler
func (s *JobService) Start(shikimoriCron, cleanupCron, topAnimeCron, calendarCron string) error {
	// Schedule Shikimori sync job
	_, err := s.cron.AddFunc(shikimoriCron, func() {
		ctx := context.Background()
		s.log.Info("starting scheduled Shikimori sync")
		start := time.Now()
		if err := s.shikimoriJob.Run(ctx); err != nil {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("shikimori_sync", "error").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("shikimori_sync").Observe(time.Since(start).Seconds())
			s.log.Errorw("Shikimori sync failed", "error", err)
		} else {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("shikimori_sync", "success").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("shikimori_sync").Observe(time.Since(start).Seconds())
			metrics.SchedulerJobLastSuccess.WithLabelValues("shikimori_sync").SetToCurrentTime()
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
		start := time.Now()
		if err := s.cleanupJob.Run(ctx); err != nil {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("cleanup", "error").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("cleanup").Observe(time.Since(start).Seconds())
			s.log.Errorw("cleanup failed", "error", err)
		} else {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("cleanup", "success").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("cleanup").Observe(time.Since(start).Seconds())
			metrics.SchedulerJobLastSuccess.WithLabelValues("cleanup").SetToCurrentTime()
			s.lastCleanupRun = time.Now()
			s.log.Info("cleanup completed successfully")
		}
	})
	if err != nil {
		return err
	}

	// Schedule top anime sync job
	_, err = s.cron.AddFunc(topAnimeCron, func() {
		ctx := context.Background()
		s.log.Info("starting scheduled top anime sync")
		start := time.Now()
		if err := s.topAnimeJob.Run(ctx); err != nil {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("top_anime_sync", "error").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("top_anime_sync").Observe(time.Since(start).Seconds())
			s.log.Errorw("top anime sync failed", "error", err)
		} else {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("top_anime_sync", "success").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("top_anime_sync").Observe(time.Since(start).Seconds())
			metrics.SchedulerJobLastSuccess.WithLabelValues("top_anime_sync").SetToCurrentTime()
			s.lastTopAnimeRun = time.Now()
			s.log.Info("top anime sync completed successfully")
		}
	})
	if err != nil {
		return err
	}

	// Schedule calendar sync job
	_, err = s.cron.AddFunc(calendarCron, func() {
		ctx := context.Background()
		s.log.Info("starting scheduled calendar sync")
		start := time.Now()
		if err := s.calendarJob.Run(ctx); err != nil {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("calendar_sync", "error").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("calendar_sync").Observe(time.Since(start).Seconds())
			s.log.Errorw("calendar sync failed", "error", err)
		} else {
			metrics.SchedulerJobExecutionsTotal.WithLabelValues("calendar_sync", "success").Inc()
			metrics.SchedulerJobDuration.WithLabelValues("calendar_sync").Observe(time.Since(start).Seconds())
			metrics.SchedulerJobLastSuccess.WithLabelValues("calendar_sync").SetToCurrentTime()
			s.lastCalendarRun = time.Now()
			s.log.Info("calendar sync completed successfully")
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
		metrics.SchedulerJobLastSuccess.WithLabelValues("shikimori_sync").SetToCurrentTime()
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
		metrics.SchedulerJobLastSuccess.WithLabelValues("cleanup").SetToCurrentTime()
		s.lastCleanupRun = time.Now()
		s.log.Info("cleanup completed successfully")
	}
}

// TriggerTopAnimeSync manually triggers the top anime sync job
func (s *JobService) TriggerTopAnimeSync(ctx context.Context) {
	s.log.Info("manually triggering top anime sync")
	if err := s.topAnimeJob.Run(ctx); err != nil {
		s.log.Errorw("top anime sync failed", "error", err)
	} else {
		metrics.SchedulerJobLastSuccess.WithLabelValues("top_anime_sync").SetToCurrentTime()
		s.lastTopAnimeRun = time.Now()
		s.log.Info("top anime sync completed successfully")
	}
}

// TriggerCalendarSync manually triggers the calendar sync job
func (s *JobService) TriggerCalendarSync(ctx context.Context) {
	s.log.Info("manually triggering calendar sync")
	if err := s.calendarJob.Run(ctx); err != nil {
		s.log.Errorw("calendar sync failed", "error", err)
	} else {
		metrics.SchedulerJobLastSuccess.WithLabelValues("calendar_sync").SetToCurrentTime()
		s.lastCalendarRun = time.Now()
		s.log.Info("calendar sync completed successfully")
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
		"top_anime_sync": map[string]interface{}{
			"last_run": s.lastTopAnimeRun,
		},
		"calendar_sync": map[string]interface{}{
			"last_run": s.lastCalendarRun,
		},
	}
}
