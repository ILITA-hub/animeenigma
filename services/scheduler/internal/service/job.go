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
	cron                       *cron.Cron
	shikimoriJob               *jobs.ShikimoriSyncJob
	cleanupJob                 *jobs.CleanupJob
	topAnimeJob                *jobs.TopAnimeSyncJob
	calendarJob                *jobs.CalendarSyncJob
	probeTriggerJob            *jobs.ProbeTriggerJob
	readThresholdJob           *jobs.ReadThresholdJob
	providerRankingJob         *jobs.ProviderRankingJob
	subtitleProbeJob           *jobs.SubtitleProbeTriggerJob
	autocacheLogicAJob         *jobs.AutocacheLogicAJob
	autocachePredictionJob     *jobs.AutocachePredictionJob
	log                        *logger.Logger
	lastShikimoriRun           time.Time
	lastCleanupRun             time.Time
	lastTopAnimeRun            time.Time
	lastCalendarRun            time.Time
	lastProbeRun               time.Time
	lastReadThresholdRun       time.Time
	lastProviderRankingRun     time.Time
	lastSubtitleProbeRun       time.Time
	lastAutocacheLogicARun     time.Time
	lastAutocachePredictionRun time.Time
}

func NewJobService(
	shikimoriJob *jobs.ShikimoriSyncJob,
	cleanupJob *jobs.CleanupJob,
	topAnimeJob *jobs.TopAnimeSyncJob,
	calendarJob *jobs.CalendarSyncJob,
	probeTriggerJob *jobs.ProbeTriggerJob,
	readThresholdJob *jobs.ReadThresholdJob,
	providerRankingJob *jobs.ProviderRankingJob,
	subtitleProbeJob *jobs.SubtitleProbeTriggerJob,
	autocacheLogicAJob *jobs.AutocacheLogicAJob,
	autocachePredictionJob *jobs.AutocachePredictionJob,
	log *logger.Logger,
) *JobService {
	return &JobService{
		// WithChain(SkipIfStillRunning) makes a still-running scheduled entry skip
		// its next due tick instead of launching a concurrent goroutine — closes
		// the cron-overlap case where a slow run (e.g. Logic A every 20 min)
		// overlaps the next tick. (audit M669)
		cron:                   cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger))),
		shikimoriJob:           shikimoriJob,
		cleanupJob:             cleanupJob,
		topAnimeJob:            topAnimeJob,
		calendarJob:            calendarJob,
		probeTriggerJob:        probeTriggerJob,
		readThresholdJob:       readThresholdJob,
		providerRankingJob:     providerRankingJob,
		subtitleProbeJob:       subtitleProbeJob,
		autocacheLogicAJob:     autocacheLogicAJob,
		autocachePredictionJob: autocachePredictionJob,
		log:                    log,
	}
}

// Start starts the job scheduler
func (s *JobService) Start(shikimoriCron, cleanupCron, topAnimeCron, calendarCron, playbackProbeCron, readThresholdCron, providerRankingCron, subtitleProbeCron, autocacheLogicACron, autocachePredictionCron string) error {
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

	// Schedule daily playback-health probe trigger (Phase A). The scheduler
	// has no ClickHouse connection — this job POSTs analytics'
	// /internal/probe/run endpoint, which runs the catalog-signed resolve →
	// HLS proxy validation chain and persists results. Replaces the Phase 23
	// scraper playability canary.
	if s.probeTriggerJob != nil {
		_, err = s.cron.AddFunc(playbackProbeCron, func() {
			ctx := context.Background()
			s.log.Info("starting scheduled playback-health probe")
			start := time.Now()
			if err := s.probeTriggerJob.Run(ctx); err != nil {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("playback_probe", "error").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("playback_probe").Observe(time.Since(start).Seconds())
				s.log.Errorw("playback-health probe failed", "error", err)
			} else {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("playback_probe", "success").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("playback_probe").Observe(time.Since(start).Seconds())
				metrics.SchedulerJobLastSuccess.WithLabelValues("playback_probe").SetToCurrentTime()
				s.lastProbeRun = time.Now()
				s.log.Info("playback-health probe completed successfully")
			}
		})
		if err != nil {
			return err
		}
		s.log.Info("registered job: playback_probe")
	}

	// Schedule daily read-threshold recompute trigger (Phase 03 / D-03 /
	// AR-EFFECT-01). The scheduler has no ClickHouse connection — this job
	// POSTs analytics' /internal recompute endpoint, which runs the
	// quantile(0.95)(duration_ms) query and publishes the read_thresholds
	// Redis hash. Skipped if no job was wired (analytics URL unset).
	if s.readThresholdJob != nil {
		_, err = s.cron.AddFunc(readThresholdCron, func() {
			ctx := context.Background()
			s.log.Info("starting scheduled read-threshold recompute")
			start := time.Now()
			if err := s.readThresholdJob.Run(ctx); err != nil {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("read_threshold_recompute", "error").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("read_threshold_recompute").Observe(time.Since(start).Seconds())
				s.log.Errorw("read-threshold recompute failed", "error", err)
			} else {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("read_threshold_recompute", "success").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("read_threshold_recompute").Observe(time.Since(start).Seconds())
				metrics.SchedulerJobLastSuccess.WithLabelValues("read_threshold_recompute").SetToCurrentTime()
				s.lastReadThresholdRun = time.Now()
				s.log.Info("read-threshold recompute completed successfully")
			}
		})
		if err != nil {
			return err
		}
		s.log.Info("registered job: read_threshold_recompute")
	}

	// Schedule daily provider-ranking recompute trigger (Stage 2b — Smart
	// Source Selection). Like the read-threshold job, the scheduler has no
	// ClickHouse connection — this job POSTs analytics' /internal recompute
	// endpoint, which runs the aggregate and publishes the player_ranking:*
	// Redis keys the catalog serves. Skipped if no job was wired.
	if s.providerRankingJob != nil {
		_, err = s.cron.AddFunc(providerRankingCron, func() {
			ctx := context.Background()
			s.log.Info("starting scheduled provider-ranking recompute")
			start := time.Now()
			if err := s.providerRankingJob.Run(ctx); err != nil {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("provider_ranking_recompute", "error").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("provider_ranking_recompute").Observe(time.Since(start).Seconds())
				s.log.Errorw("provider-ranking recompute failed", "error", err)
			} else {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("provider_ranking_recompute", "success").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("provider_ranking_recompute").Observe(time.Since(start).Seconds())
				metrics.SchedulerJobLastSuccess.WithLabelValues("provider_ranking_recompute").SetToCurrentTime()
				s.lastProviderRankingRun = time.Now()
				s.log.Info("provider-ranking recompute completed successfully")
			}
		})
		if err != nil {
			return err
		}
		s.log.Info("registered job: provider_ranking_recompute")
	}

	// Schedule the active subtitle-provider health probe (every 5 min). The probe
	// lives in catalog (owns the Jimaku/OpenSubtitles clients + keys), so this job
	// just POSTs catalog's /internal/subtitle-probe/run. Nil-guarded for symmetry.
	if s.subtitleProbeJob != nil {
		_, err = s.cron.AddFunc(subtitleProbeCron, func() {
			ctx := context.Background()
			s.log.Info("starting scheduled subtitle-health probe")
			start := time.Now()
			if err := s.subtitleProbeJob.Run(ctx); err != nil {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("subtitle_probe", "error").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("subtitle_probe").Observe(time.Since(start).Seconds())
				s.log.Errorw("subtitle-health probe failed", "error", err)
			} else {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("subtitle_probe", "success").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("subtitle_probe").Observe(time.Since(start).Seconds())
				metrics.SchedulerJobLastSuccess.WithLabelValues("subtitle_probe").SetToCurrentTime()
				s.lastSubtitleProbeRun = time.Now()
				s.log.Info("subtitle-health probe completed successfully")
			}
		})
		if err != nil {
			return err
		}
		s.log.Info("registered job: subtitle_probe")
	}

	// Schedule autocache Logic A ongoing-push producer (Phase 09 — TRIG-01).
	// Unlike the analytics-trigger jobs above, this one runs the enumeration
	// join itself (shared animeenigma DB) and fires per-ongoing demand POSTs to
	// the library /internal endpoint. Nil-guarded: a missing library URL (empty
	// LibraryInternalURL → nil job from main.go) disables it cleanly.
	if s.autocacheLogicAJob != nil {
		_, err = s.cron.AddFunc(autocacheLogicACron, func() {
			ctx := context.Background()
			s.log.Info("starting scheduled autocache Logic A sweep")
			start := time.Now()
			if err := s.autocacheLogicAJob.Run(ctx); err != nil {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("autocache_logic_a", "error").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("autocache_logic_a").Observe(time.Since(start).Seconds())
				s.log.Errorw("autocache Logic A sweep failed", "error", err)
			} else {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("autocache_logic_a", "success").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("autocache_logic_a").Observe(time.Since(start).Seconds())
				metrics.SchedulerJobLastSuccess.WithLabelValues("autocache_logic_a").SetToCurrentTime()
				s.lastAutocacheLogicARun = time.Now()
				s.log.Info("autocache Logic A sweep completed successfully")
			}
		})
		if err != nil {
			return err
		}
		s.log.Info("registered job: autocache_logic_a")
	}

	// Schedule autocache prediction producer (Phase 11 — OBS-05). Unlike Logic A
	// it has NO external dependency (it only reads the shared animeenigma DB the
	// scheduler already owns and sets a Prometheus gauge), so it is registered
	// UNCONDITIONALLY (always on) and constructed unconditionally in main.go.
	if s.autocachePredictionJob != nil {
		_, err = s.cron.AddFunc(autocachePredictionCron, func() {
			ctx := context.Background()
			s.log.Info("starting scheduled autocache prediction sweep")
			start := time.Now()
			if err := s.autocachePredictionJob.Run(ctx); err != nil {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("autocache_prediction", "error").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("autocache_prediction").Observe(time.Since(start).Seconds())
				s.log.Errorw("autocache prediction sweep failed", "error", err)
			} else {
				metrics.SchedulerJobExecutionsTotal.WithLabelValues("autocache_prediction", "success").Inc()
				metrics.SchedulerJobDuration.WithLabelValues("autocache_prediction").Observe(time.Since(start).Seconds())
				metrics.SchedulerJobLastSuccess.WithLabelValues("autocache_prediction").SetToCurrentTime()
				s.lastAutocachePredictionRun = time.Now()
				s.log.Info("autocache prediction sweep completed successfully")
			}
		})
		if err != nil {
			return err
		}
		s.log.Info("registered job: autocache_prediction")
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

// TriggerPlaybackProbe manually triggers the playback-health probe job.
// Used by the manual-trigger HTTP handler (POST /api/v1/jobs/playback_probe).
func (s *JobService) TriggerPlaybackProbe(ctx context.Context) {
	s.log.Info("manually triggering playback-health probe")
	if s.probeTriggerJob == nil {
		s.log.Warn("playback probe job not configured")
		return
	}
	if err := s.probeTriggerJob.Run(ctx); err != nil {
		s.log.Errorw("playback-health probe failed", "error", err)
	} else {
		metrics.SchedulerJobLastSuccess.WithLabelValues("playback_probe").SetToCurrentTime()
		s.lastProbeRun = time.Now()
		s.log.Info("playback-health probe completed successfully")
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
		"playback_probe": map[string]interface{}{
			"last_run": s.lastProbeRun,
		},
		"read_threshold_recompute": map[string]interface{}{
			"last_run": s.lastReadThresholdRun,
		},
		"provider_ranking_recompute": map[string]interface{}{
			"last_run": s.lastProviderRankingRun,
		},
		"subtitle_probe": map[string]interface{}{
			"last_run": s.lastSubtitleProbeRun,
		},
		"autocache_logic_a": map[string]interface{}{
			"last_run": s.lastAutocacheLogicARun,
		},
		"autocache_prediction": map[string]interface{}{
			"last_run": s.lastAutocachePredictionRun,
		},
	}
}
