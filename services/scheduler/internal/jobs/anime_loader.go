package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/repo"
	"gorm.io/gorm"
)

// AnimeLoaderJob is the background worker that processes anime load tasks
type AnimeLoaderJob struct {
	taskRepo      *repo.TaskRepository
	exportJobRepo *repo.ExportJobRepository
	processor     domain.TaskProcessor
	log           *logger.Logger

	// Rate limiting: 3 requests per second to Shikimori
	rateLimiter *rateLimiter

	// Control
	stopCh  chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	running bool
}

// rateLimiter implements token bucket rate limiting
type rateLimiter struct {
	mu         sync.Mutex
	tokens     int
	maxTokens  int
	lastRefill time.Time
	interval   time.Duration
}

// NewAnimeLoaderJob creates a new anime loader job
func NewAnimeLoaderJob(
	taskRepo *repo.TaskRepository,
	exportJobRepo *repo.ExportJobRepository,
	processor domain.TaskProcessor,
	log *logger.Logger,
) *AnimeLoaderJob {
	return &AnimeLoaderJob{
		taskRepo:      taskRepo,
		exportJobRepo: exportJobRepo,
		processor:     processor,
		log:           log,
		rateLimiter: &rateLimiter{
			tokens:     3,
			maxTokens:  3,
			lastRefill: time.Now(),
			interval:   time.Second,
		},
		stopCh: make(chan struct{}),
	}
}

// Start starts the background worker
func (j *AnimeLoaderJob) Start(ctx context.Context) error {
	j.mu.Lock()
	if j.running {
		j.mu.Unlock()
		return nil
	}
	j.running = true
	j.mu.Unlock()

	// Reset stuck tasks on startup
	resetCount, err := j.taskRepo.ResetStuckTasks(ctx, 5*time.Minute)
	if err != nil {
		j.log.Warnw("failed to reset stuck tasks", "error", err)
	} else if resetCount > 0 {
		j.log.Infow("reset stuck tasks", "count", resetCount)
	}

	j.wg.Add(1)
	go j.runWorker()

	j.log.Info("anime loader worker started")
	return nil
}

// Stop stops the background worker gracefully
func (j *AnimeLoaderJob) Stop() {
	j.mu.Lock()
	if !j.running {
		j.mu.Unlock()
		return
	}
	j.running = false
	j.mu.Unlock()

	close(j.stopCh)
	j.wg.Wait()
	j.log.Info("anime loader worker stopped")
}

// runWorker is the main worker loop
func (j *AnimeLoaderJob) runWorker() {
	defer j.wg.Done()

	// Process tasks every 333ms (3 per second max)
	ticker := time.NewTicker(333 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-j.stopCh:
			return
		case <-ticker.C:
			j.processNextTask()
		}
	}
}

// processNextTask fetches and processes the next pending task
func (j *AnimeLoaderJob) processNextTask() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Wait for rate limiter
	j.rateLimiter.acquire()

	// Get next pending task
	task, err := j.taskRepo.GetNextPending(ctx)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// No tasks to process - this is normal
			return
		}
		j.log.Warnw("failed to get next pending task", "error", err)
		return
	}

	// Claim the task
	if err := j.taskRepo.ClaimTask(ctx, task.ID); err != nil {
		j.log.Warnw("failed to claim task", "task_id", task.ID, "error", err)
		return
	}

	// Process the task
	if err := j.processor.ProcessTask(ctx, task); err != nil {
		j.log.Warnw("failed to process task", "task_id", task.ID, "error", err)
		return
	}

	// Check if export job is complete
	if task.ExportJobID != "" {
		if err := j.processor.CheckExportJobCompletion(ctx, task.ExportJobID); err != nil {
			j.log.Warnw("failed to check export job completion", "export_job_id", task.ExportJobID, "error", err)
		}
	}
}

// GetPendingCount returns the number of pending tasks
func (j *AnimeLoaderJob) GetPendingCount(ctx context.Context) (int64, error) {
	return j.taskRepo.GetPendingCount(ctx)
}

// GetStatus returns the worker status
func (j *AnimeLoaderJob) GetStatus() map[string]interface{} {
	j.mu.Lock()
	running := j.running
	j.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pendingCount, _ := j.taskRepo.GetPendingCount(ctx)

	return map[string]interface{}{
		"running":       running,
		"pending_tasks": pendingCount,
	}
}

// acquire waits for a rate limit token
func (rl *rateLimiter) acquire() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)

	// Refill tokens
	if elapsed >= rl.interval {
		rl.tokens = rl.maxTokens
		rl.lastRefill = now
	}

	// Wait if no tokens available
	if rl.tokens <= 0 {
		waitTime := rl.interval - elapsed
		time.Sleep(waitTime)
		rl.tokens = rl.maxTokens
		rl.lastRefill = time.Now()
	}

	rl.tokens--
}
