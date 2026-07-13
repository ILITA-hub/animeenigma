package service

import (
	"context"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

// progressCacheTTL bounds how long a job's last-known download progress lives in
// Redis without a refresh. The download worker re-writes it every progressTick
// (~5s), so this only needs to outlast a brief tick gap; once a job stops
// ticking (finished, failed, or the service went down) the key self-expires well
// before the admin UI could show a phantom in-flight percentage.
const progressCacheTTL = 5 * time.Minute

// progressKeyPrefix namespaces the per-job live-progress keys in Redis.
const progressKeyPrefix = "lib:job:progress:"

func progressKey(jobID string) string { return progressKeyPrefix + jobID }

// ProgressStore is the live download-progress side channel. In-flight progress
// is written here every tick instead of to Postgres, so library_jobs only takes
// a write at a job's start and end (status transitions), never on every ~5s
// progress sample. The admin API reads it back via GetProgressMany and overlays
// it onto the persisted rows.
type ProgressStore interface {
	SetProgress(ctx context.Context, jobID string, pct int) error
	GetProgressMany(ctx context.Context, jobIDs []string) (map[string]int, error)
	DeleteProgress(ctx context.Context, jobID string) error
}

// RedisProgressCache implements ProgressStore over the shared Redis handle the
// library service already holds (added v4.0 Phase 3 for the read-gate).
//
// The write paths deliberately drop to the raw client (rc.Client().Set/Del/MGet)
// rather than the instrumented RedisCache.Set/Delete wrappers: this cache exists
// precisely to keep the ~5s-per-download progress writes OUT of the analytics
// effect pipeline. Routing them through the wrapper would emit a cache-effect
// event every tick and re-create the exact write churn this side channel removes.
// Do NOT "fix" these to use the wrapper methods.
type RedisProgressCache struct {
	rc  *cache.RedisCache
	ttl time.Duration
}

// NewRedisProgressCache builds a RedisProgressCache with the default TTL.
func NewRedisProgressCache(rc *cache.RedisCache) *RedisProgressCache {
	return &RedisProgressCache{rc: rc, ttl: progressCacheTTL}
}

// SetProgress records a job's latest download percentage (0..100), refreshing
// the key's TTL. Cheap SET; no DB write.
func (c *RedisProgressCache) SetProgress(ctx context.Context, jobID string, pct int) error {
	return c.rc.Client().Set(ctx, progressKey(jobID), pct, c.ttl).Err()
}

// DeleteProgress drops a job's live-progress key (called at terminal transitions
// so a finished job's bar doesn't linger).
func (c *RedisProgressCache) DeleteProgress(ctx context.Context, jobID string) error {
	return c.rc.Client().Del(ctx, progressKey(jobID)).Err()
}

// GetProgressMany returns the cached pct for each job that currently has one.
// Jobs without a live entry are simply absent from the map, so the caller falls
// back to the persisted progress_pct. An empty input returns an empty map.
func (c *RedisProgressCache) GetProgressMany(ctx context.Context, jobIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(jobIDs))
	if len(jobIDs) == 0 {
		return out, nil
	}
	keys := make([]string, len(jobIDs))
	for i, id := range jobIDs {
		keys[i] = progressKey(id)
	}
	vals, err := c.rc.Client().MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	for i, v := range vals {
		s, ok := v.(string)
		if !ok {
			continue // nil (missing key) or unexpected type
		}
		if pct, perr := strconv.Atoi(s); perr == nil {
			out[jobIDs[i]] = pct
		}
	}
	return out, nil
}
