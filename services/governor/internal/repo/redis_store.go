// Package repo implements the governor's two IO edges: the Redis level store
// consumers poll, and the analytics transition sink that persists the durable
// what/when/why history.
package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ILITA-hub/animeenigma/services/governor/internal/domain"
)

// RedisStore publishes the degradation level and reads the owner override.
// Values are plain strings ("0"|"1"|"2"), NOT JSON, so shell consumers
// (bin/degradation-override.sh, redis-cli) stay trivial.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore wraps an existing go-redis client (from libs/cache Client()).
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

// PublishLevel refreshes the level + reasons keys with ttl. Called every tick;
// the TTL is the fail-open mechanism (dead governor ⇒ keys expire ⇒ consumers
// read "no key" ⇒ LevelNormal).
func (s *RedisStore) PublishLevel(ctx context.Context, level domain.Level, reasons []domain.Reason, ttl time.Duration) error {
	if err := s.client.Set(ctx, domain.RedisLevelKey, strconv.Itoa(int(level)), ttl).Err(); err != nil {
		return fmt.Errorf("set level: %w", err)
	}
	blob, err := json.Marshal(reasons)
	if err != nil {
		return fmt.Errorf("marshal reasons: %w", err)
	}
	if err := s.client.Set(ctx, domain.RedisReasonsKey, blob, ttl).Err(); err != nil {
		return fmt.Errorf("set reasons: %w", err)
	}
	return nil
}

// Override returns the pinned level when ae:degradation:override holds a valid
// "0"|"1"|"2", nil when the key is absent, and an error only on transport
// failure. Garbage values are treated as absent (and logged by the caller via
// the nil return — a bad pin must never wedge the governor).
func (s *RedisStore) Override(ctx context.Context) (*domain.Level, error) {
	raw, err := s.client.Get(ctx, domain.RedisOverrideKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 2 {
		return nil, nil
	}
	lvl := domain.Level(n)
	return &lvl, nil
}
