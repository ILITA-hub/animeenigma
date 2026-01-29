package jobs

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

type CleanupJob struct {
	db     *database.Database
	cache  *cache.RedisCache
	config *config.JobsConfig
	log    *logger.Logger
}

func NewCleanupJob(
	db *database.Database,
	cache *cache.RedisCache,
	config *config.JobsConfig,
	log *logger.Logger,
) *CleanupJob {
	return &CleanupJob{
		db:     db,
		cache:  cache,
		config: config,
		log:    log,
	}
}

// Run executes the cleanup job
func (j *CleanupJob) Run(ctx context.Context) error {
	j.log.Info("starting cleanup job")

	// Clean up expired sessions
	if err := j.cleanupExpiredSessions(ctx); err != nil {
		j.log.Errorw("failed to cleanup expired sessions", "error", err)
	}

	// Clean up old watch history
	if err := j.cleanupOldWatchHistory(ctx); err != nil {
		j.log.Errorw("failed to cleanup old watch history", "error", err)
	}

	// Clean up orphaned room data
	if err := j.cleanupOrphanedRooms(ctx); err != nil {
		j.log.Errorw("failed to cleanup orphaned rooms", "error", err)
	}

	j.log.Info("cleanup job completed")
	return nil
}

func (j *CleanupJob) cleanupExpiredSessions(ctx context.Context) error {
	j.log.Info("cleaning up expired sessions")

	// In a real implementation, this would:
	// 1. Remove expired JWT refresh tokens from cache
	// 2. Clean up session data older than retention period

	return nil
}

func (j *CleanupJob) cleanupOldWatchHistory(ctx context.Context) error {
	j.log.Infow("cleaning up old watch history", "retention_days", j.config.DataRetentionDays)

	cutoffDate := time.Now().AddDate(0, 0, -j.config.DataRetentionDays)

	// In a real implementation, this would:
	// 1. Delete watch history entries older than retention period
	// 2. Archive important data before deletion

	j.log.Infow("watch history cleanup completed", "cutoff_date", cutoffDate)
	return nil
}

func (j *CleanupJob) cleanupOrphanedRooms(ctx context.Context) error {
	j.log.Info("cleaning up orphaned rooms")

	// In a real implementation, this would:
	// 1. Find rooms that have been inactive for > 24 hours
	// 2. Remove room data from cache and database
	// 3. Clean up associated player data

	return nil
}
