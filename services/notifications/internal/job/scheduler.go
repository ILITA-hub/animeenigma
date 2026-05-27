package job

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/config"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/repo"
	"github.com/robfig/cron/v3"
)

// Scheduler wires the detector + relevance-invalidation + cleanup +
// unread-gauge poller into a single robfig/cron instance with ±5min
// boot-time jitter. Start is called once at boot from
// cmd/notifications-api/main.go; Stop is called BEFORE srv.Shutdown so
// in-flight cron callbacks finish cleanly. On each detector tick the
// relevance-invalidation job runs immediately after, retiring notifications
// made stale by watch-list or progress changes since the last run.
//
// Cron expressions come from config.DetectorConfig (defaults: detector
// "0 * * * *", cleanup "30 3 * * *"). Boot-time jitter is computed once in
// the constructor — same value for the lifetime of the process — so log
// lines can attribute "ran at minute X" to a specific jitter value.
type Scheduler struct {
	cron        *cron.Cron
	detector    *NewEpisodeDetectorJob
	invalidator *RelevanceInvalidationJob
	cleanup     *DismissedRetentionCleanupJob
	gaugeRepo   *repo.UnreadGaugeRepository
	cfg         *config.DetectorConfig
	log         *logger.Logger

	jitter   time.Duration
	pollerWG sync.WaitGroup
	cancel   context.CancelFunc
}

// NewScheduler constructs the scheduler. The Detector + Invalidator + Cleanup + GaugeRepo
// args are required; cfg drives schedule + worker limits.
func NewScheduler(
	detector *NewEpisodeDetectorJob,
	invalidator *RelevanceInvalidationJob,
	cleanup *DismissedRetentionCleanupJob,
	gaugeRepo *repo.UnreadGaugeRepository,
	cfg *config.DetectorConfig,
	log *logger.Logger,
) *Scheduler {
	// Boot-time jitter: -5..+5 minutes. Pre-computed so log lines can
	// reference the exact value the scheduler is using.
	//
	// Negative values are honoured by adding |jitter| to the first
	// tick — effectively a random +0..+5min offset that re-anchors the
	// detector to the hour boundary. Positive values delay the first
	// tick by jitter, then re-anchor identically.
	//
	// Net effect: simultaneous boots across replicas (e.g. a rolling
	// restart) randomise the first hourly tick by up to 5 minutes, so
	// upstream parsers are not hit simultaneously.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	jitter := time.Duration(rng.Intn(11)-5) * time.Minute
	if jitter < 0 {
		jitter = -jitter
	}
	return &Scheduler{
		detector:    detector,
		invalidator: invalidator,
		cleanup:     cleanup,
		gaugeRepo:   gaugeRepo,
		cfg:         cfg,
		log:         log,
		jitter:      jitter,
	}
}

// Start registers the two cron expressions + launches the gauge poller
// goroutine. Returns an error if either cron expression fails to parse —
// main.go's caller is expected to Fatalw on error so the service refuses
// to boot rather than running with a silent disabled cron.
func (s *Scheduler) Start(ctx context.Context) error {
	s.cron = cron.New()

	if _, err := s.cron.AddFunc(s.cfg.Cron, func() {
		// First tick respects the boot-time jitter; subsequent ticks
		// re-anchor to the cron expression. The sleep happens INSIDE
		// the cron callback so the cron's tick scheduler stays aligned
		// to the hour boundary.
		if s.jitter > 0 {
			time.Sleep(s.jitter)
			// One-shot: after the first delayed tick, zero out the jitter
			// so subsequent ticks fire on time.
			s.jitter = 0
		}
		s.runDetector(ctx)
	}); err != nil {
		return err
	}

	if _, err := s.cron.AddFunc(s.cfg.CleanupCron, func() {
		s.runCleanup(ctx)
	}); err != nil {
		return err
	}

	// Unread-gauge poller goroutine. Cancellable via Stop.
	pollerCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.pollerWG.Add(1)
	go s.pollUnreadGauge(pollerCtx)

	s.cron.Start()
	if s.log != nil {
		s.log.Infow("scheduler started",
			"detector_cron", s.cfg.Cron,
			"cleanup_cron", s.cfg.CleanupCron,
			"jitter_seconds", int(s.jitter.Seconds()),
			"worker_limit", s.cfg.WorkerLimit,
		)
	}
	return nil
}

// Stop tears down the scheduler: cron.Stop() waits for in-flight jobs;
// the gauge poller goroutine listens to its context and exits cleanly.
func (s *Scheduler) Stop() {
	if s.cron != nil {
		// cron.Stop() returns a context that completes when in-flight
		// jobs finish. We block on it so the caller (main.go) can
		// reliably sequence srv.Shutdown after scheduler shutdown.
		<-s.cron.Stop().Done()
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.pollerWG.Wait()
	if s.log != nil {
		s.log.Info("scheduler stopped")
	}
}

// runDetector is the cron callback for the detector. Records metrics in
// the same shape as services/scheduler/internal/service/job.go.
// After the detector completes, the relevance invalidation job runs on the
// same tick to retire notifications made stale since the last run.
// Invalidation fires even if the detector errored — it is independent.
func (s *Scheduler) runDetector(ctx context.Context) {
	if s.log != nil {
		s.log.Info("scheduled detector run starting")
	}
	if _, err := s.detector.Run(ctx); err != nil {
		// Detector logs its own structured error; nothing more to add.
		_ = err
	}
	// Retire notifications made stale by watches / list changes since the
	// last tick. Runs even if the detector errored — invalidation is
	// independent of new-episode detection.
	if s.invalidator != nil {
		if _, err := s.invalidator.Run(ctx); err != nil && s.log != nil {
			s.log.Errorw("relevance invalidation failed", "error", err)
		}
	}
}

// runCleanup is the cron callback for the retention cleanup.
func (s *Scheduler) runCleanup(ctx context.Context) {
	if s.log != nil {
		s.log.Info("scheduled cleanup run starting")
	}
	if _, err := s.cleanup.Run(ctx); err != nil && s.log != nil {
		s.log.Errorw("scheduled cleanup failed", "error", err)
	}
}

// pollUnreadGauge updates notifications_active_unread_gauge every
// UnreadGaugeEvery. Exits on ctx cancellation. The first sample is taken
// immediately on goroutine entry so the gauge is non-zero shortly after
// boot (rather than waiting one full interval).
func (s *Scheduler) pollUnreadGauge(ctx context.Context) {
	defer s.pollerWG.Done()
	interval := s.cfg.UnreadGaugeEvery
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	sample := func() {
		n, err := s.gaugeRepo.ActiveUnreadCount(ctx)
		if err != nil {
			if s.log != nil {
				s.log.Warnw("unread gauge sample failed", "error", err)
			}
			return
		}
		NotificationsActiveUnreadGauge.Set(float64(n))
	}

	sample()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sample()
		}
	}
}
