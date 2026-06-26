package service

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisLogAdapter adapts *redis.Client to the logRedis interface.
type redisLogAdapter struct {
	c *redis.Client
}

// NewRedisLogAdapter wraps a *redis.Client for use with LogBuffer.
func NewRedisLogAdapter(c *redis.Client) logRedis {
	return &redisLogAdapter{c: c}
}

func (a *redisLogAdapter) appendLog(ctx context.Context, key, val string, cap int, ttl time.Duration) error {
	pipe := a.c.Pipeline()
	pipe.RPush(ctx, key, val)
	// Trim to keep only the last `cap` entries (0-indexed: -cap to -1).
	pipe.LTrim(ctx, key, int64(-cap), -1)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (a *redisLogAdapter) rangeLogs(ctx context.Context, key string, n int) ([]string, error) {
	return a.c.LRange(ctx, key, int64(-n), -1).Result()
}
